// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// PaymentProvider is the port of the payment provider (ADR-0074 §1). Its types are of the
// standard library and this package only, never of the provider's SDK. It reports the
// provider's facts only; the state of our subscriptions, the grace period, and the roles are
// derived by this module. One adapter is active on a server (payments_provider).
//
// Every method that talks to the provider counts as one request to it, a page of a list
// included; the reconciliation counts them against payments_reconcile_max_requests (§4). The
// adapters bound their own request rate. The errors are the ones of ErrorKindOf.
type PaymentProvider interface {
	// Name is the provider's identifier, such as "stripe": the provider column of the tables and
	// the {provider} of the webhook path.
	Name() string
	// Location is the time zone of the provider's reports; MoneyMovement.OccurredOn uses it
	// (ADR-0073 §3).
	Location() *time.Location
	// Mode reports whether the credentials are for test payments (no real charges) or live ones.
	Mode() Mode
	// MinimumAmount is the smallest recurring charge the provider accepts in currency (ISO
	// 4217), in minor units; false for a currency it does not take.
	MinimumAmount(currency string) (int64, bool)

	// EnsurePlan creates or reuses the product of a plan and one recurring price per entry of
	// spec.Prices, archives the plan's prices that spec no longer lists, and returns the refs.
	// Idempotent. Archived prices keep renewing the subscriptions already on them.
	EnsurePlan(ctx context.Context, spec PlanSpec) (PlanRefs, error)

	// StartCheckout creates a hosted checkout and returns the URL to redirect the browser to.
	// The same Key returns the same checkout.
	StartCheckout(ctx context.Context, req CheckoutRequest) (Checkout, error)
	// GetCheckout reports whether a checkout completed and, if so, what it created.
	GetCheckout(ctx context.Context, ref CheckoutRef) (CheckoutResult, error)
	// PortalURL returns a short-lived URL of the provider's customer portal (payment methods,
	// invoices, cancellation at the period's end).
	PortalURL(ctx context.Context, customer CustomerRef, returnURL string) (string, error)

	// The commands that our own screens and jobs issue. Each is safe to repeat: a second call
	// with the subscription already in the asked state changes nothing. key is the caller's
	// idempotency key (the job's key and the operation's name).
	ChangePrice(ctx context.Context, sub SubscriptionRef, to PriceRef, when ChangeTiming, key string) error
	// CancelScheduledChange withdraws the change waiting for the period's end; nothing without one.
	CancelScheduledChange(ctx context.Context, sub SubscriptionRef, key string) error
	CancelAtPeriodEnd(ctx context.Context, sub SubscriptionRef, key string) error
	ResumeCancellation(ctx context.Context, sub SubscriptionRef, key string) error
	// CancelNow ends the subscription without a refund; nothing when it already ended.
	CancelNow(ctx context.Context, sub SubscriptionRef, key string) error

	// ParseWebhook verifies a delivery (its signature, its time within WebhookTolerance of now,
	// and its mode against Mode) and returns what it points at. It does no I/O and fails with
	// ErrInvalidSignature, ErrStaleSignature, or ErrModeMismatch. An event of a type that this
	// module does not act on is a Notice of NoticeIgnored.
	ParseWebhook(header http.Header, body []byte, now time.Time) (Notice, error)
	// Resolve turns a notice into the objects to re-read: subscriptions, a checkout, and money.
	Resolve(ctx context.Context, n Notice) (Change, error)

	// GetSubscription re-reads the provider's current state of one subscription.
	GetSubscription(ctx context.Context, sub SubscriptionRef) (SubscriptionSnapshot, error)
	// ListSubscriptions pages through every subscription of the account, at most PageSize at a
	// time, from cursor ("" for the first page).
	ListSubscriptions(ctx context.Context, cursor string) (SubscriptionPage, error)
	// ListChanges pages through the notices of changes since a time, oldest first, from cursor
	// ("" for the first page); it fails with ErrChangesWindow when since is older than the
	// provider keeps them.
	ListChanges(ctx context.Context, since time.Time, cursor string) (ChangePage, error)
	// ListMoney pages through the final money movements booked at or after since, oldest first,
	// after cursor ("" reads from since).
	ListMoney(ctx context.Context, since time.Time, cursor string) (MoneyPage, error)
	// GetMoney re-reads one final money movement (a refund's original payment, for example); it
	// fails with ErrPaymentNotFound for one that is not final yet.
	GetMoney(ctx context.Context, id string) (MoneyMovement, error)
}

// PageServer is implemented by a provider without hosted checkout and portal pages: our server
// serves the adapter's own pages instead, under PagesPath of its Name, and StartCheckout and
// PortalURL return URLs of them (ADR-0099 §1, §3).
type PageServer interface {
	// Pages returns the handler of the GET and POST requests under PagesPath(Name()), which
	// see their whole path, rendered with env.
	Pages(env PageEnv) http.Handler
}

// PageEnv is what the pages of a PageServer render with: the server's own pages' locales and
// stylesheet (ADR-0009, ADR-0030).
type PageEnv struct {
	// Locales resolves the locale of a request.
	Locales *i18n.Resolver
	// Stylesheet returns the URL of the server pages' stylesheet; "" leaves the pages unstyled.
	Stylesheet func() string
	// Logger logs what a page could not do.
	Logger *slog.Logger
	// Checkouts claims a checkout across the server's processes; Payments.Pages fills it.
	Checkouts CheckoutClaims
}

// CheckoutClaims claims a checkout of the provider's own pages across the server's processes, for
// a provider that takes no idempotency key on the creation of a subscription (PAY.JP v1), so
// that the posts of one checkout on several replicas create one subscription.
type CheckoutClaims interface {
	// ClaimCheckout claims checkoutID for CheckoutClaimHold and reports whether it did; false
	// means that another post holds it. release gives the claim back and is nil when the claim
	// was not taken. A page keeps its calls of the provider within CheckoutCallBudget, so that its
	// claim does not expire while it runs.
	ClaimCheckout(ctx context.Context, checkoutID string) (release func(), claimed bool, err error)
}

// CheckoutClaimHold is how long a claim of a checkout lasts unless it is released: longer than
// CheckoutCallBudget, so that a process that stopped holds the checkout for a while only.
const CheckoutClaimHold = 5 * time.Minute

// CheckoutCallBudget is how long a page that holds the claim of a checkout may call the provider.
const CheckoutCallBudget = 3 * time.Minute

// PagesPathPrefix is the path of the pages of a PageServer, followed by the provider's name.
const PagesPathPrefix = "/payments/pages/"

// PagesPath returns the path under which the pages of provider are served.
func PagesPath(provider string) string { return PagesPathPrefix + provider + "/" }

// PageSize is the most items a page of the provider's lists holds (ADR-0074 §4).
const PageSize = 100

// WebhookTolerance is how far the time of a webhook's signature may be from now (ADR-0074 §3).
const WebhookTolerance = 5 * time.Minute

// Mode is the kind of the provider's credentials (ADR-0074 §5).
type Mode string

// The modes.
const (
	// ModeTest: test credentials; nothing is charged.
	ModeTest Mode = "test"
	// ModeLive: live credentials.
	ModeLive Mode = "live"
)

// Interval is how often a price renews.
type Interval string

// The intervals of the plans (ADR-0072 §3).
const (
	IntervalMonth Interval = "month"
	IntervalYear  Interval = "year"
)

// ChangeTiming is when ChangePrice switches a subscription to the new price (ADR-0072 §8).
type ChangeTiming string

// The timings.
const (
	// ChangeNowProrated switches now and charges the prorated difference now.
	ChangeNowProrated ChangeTiming = "now_prorated"
	// ChangeAtPeriodEnd switches at the end of the current period.
	ChangeAtPeriodEnd ChangeTiming = "period_end"
)

// The provider's identifiers of its objects. The zero value is none.
type (
	// ProductRef is the provider's product of a plan.
	ProductRef string
	// PriceRef is a recurring price.
	PriceRef string
	// CustomerRef is a customer.
	CustomerRef string
	// SubscriptionRef is a subscription.
	SubscriptionRef string
	// CheckoutRef is a hosted checkout.
	CheckoutRef string
)

// PlanSpec is a plan as the provider should have it.
type PlanSpec struct {
	// PlanID is our plan's ID; the adapter stores it with the product.
	PlanID string
	// Name and Description show on the hosted checkout and the receipts.
	Name        string
	Description string
	// Prices are the prices to keep active.
	Prices []PriceSpec
}

// PriceSpec is a recurring price.
type PriceSpec struct {
	Interval Interval
	// AmountMinor is in minor units of Currency (ADR-0067 §6).
	AmountMinor int64
	// Currency is an ISO 4217 code.
	Currency string
}

// PlanRefs is what EnsurePlan made of a PlanSpec.
type PlanRefs struct {
	Product ProductRef
	// Prices are the active prices, one per entry of PlanSpec.Prices, in its order.
	Prices []PlanPrice
	// Archived are the prices of the plan that this call archived.
	Archived []PriceRef
}

// PlanPrice is the provider's price of a PriceSpec.
type PlanPrice struct {
	Spec PriceSpec
	Ref  PriceRef
}

// CheckoutRequest asks for a hosted checkout of a new subscription.
type CheckoutRequest struct {
	// LocalID is our subscription's ID (ADR-0072); the provider keeps it on the checkout and the
	// subscription, and returns it in CheckoutResult and SubscriptionSnapshot.
	LocalID string
	Price   PriceRef
	// Customer is the provider's customer of the user; zero lets the provider create one.
	Customer CustomerRef
	// Email prefills the checkout when Customer is zero.
	Email string
	// Locale is the BCP 47 tag of the user's language.
	Locale string
	// SuccessURL and CancelURL are on our origin; the provider appends its checkout's ID to
	// SuccessURL.
	SuccessURL string
	CancelURL  string
	// Key is the idempotency key.
	Key string
}

// Checkout is a hosted checkout that the browser is redirected to.
type Checkout struct {
	Ref CheckoutRef
	// RedirectURL is https on a host the adapter knows (ADR-0074 §7).
	RedirectURL string
	ExpiresAt   time.Time
}

// CheckoutStatus is the state of a hosted checkout.
type CheckoutStatus string

// The states of a checkout.
const (
	CheckoutOpen      CheckoutStatus = "open"
	CheckoutCompleted CheckoutStatus = "completed"
	CheckoutExpired   CheckoutStatus = "expired"
)

// CheckoutResult is what GetCheckout reports.
type CheckoutResult struct {
	Ref     CheckoutRef
	Status  CheckoutStatus
	LocalID string
	// Customer and Subscription are what a completed checkout created; zero otherwise.
	Customer     CustomerRef
	Subscription SubscriptionRef
}

// SubscriptionStatus is the provider's state of a subscription, in the words of Stripe with a
// meaning that does not depend on it (ADR-0074 §1). A trial is StatusActive, and a state that the
// adapter does not know is StatusUnpaid (no role), logged as a warning.
type SubscriptionStatus string

// The states of a subscription.
const (
	StatusIncomplete        SubscriptionStatus = "incomplete"
	StatusIncompleteExpired SubscriptionStatus = "incomplete_expired"
	StatusActive            SubscriptionStatus = "active"
	StatusPastDue           SubscriptionStatus = "past_due"
	StatusUnpaid            SubscriptionStatus = "unpaid"
	StatusPaused            SubscriptionStatus = "paused"
	StatusCanceled          SubscriptionStatus = "canceled"
)

// SubscriptionSnapshot is the provider's current state of a subscription.
type SubscriptionSnapshot struct {
	Ref      SubscriptionRef
	Customer CustomerRef
	// LocalID is our subscription's ID from the provider's subscription; "" when it is not ours.
	LocalID string
	Price   PriceRef
	Status  SubscriptionStatus
	// PeriodStart and PeriodEnd bound the current billing period.
	PeriodStart time.Time
	PeriodEnd   time.Time
	// CancelAtPeriodEnd reports a cancellation scheduled for PeriodEnd.
	CancelAtPeriodEnd bool
	// ScheduledPrice is the price that a change waiting for PeriodEnd switches to; zero without one.
	ScheduledPrice PriceRef
	// LatestPaymentFailed is when the latest invoice's payment failed while it is still unpaid;
	// zero otherwise.
	LatestPaymentFailed time.Time
	// ReadAt is when the supporters module began to read the snapshot from the provider; the
	// adapters leave it zero, which means the time it is applied. A snapshot read before the one
	// applied last to its subscription is not applied (ADR-0074 §3).
	ReadAt time.Time
}

// readSubscription reads a subscription from prov and stamps the snapshot with the time the
// read began.
func readSubscription(ctx context.Context, prov PaymentProvider, clock kernel.Clock, ref SubscriptionRef) (SubscriptionSnapshot, error) {
	at := kernel.Normalize(clock.Now())
	snap, err := prov.GetSubscription(ctx, ref)
	snap.ReadAt = at
	return snap, err
}

// MoneyKind is the kind of a money movement (ADR-0073 §1).
type MoneyKind string

// The kinds of money movements.
const (
	MoneyPayment            MoneyKind = "payment"
	MoneyRefund             MoneyKind = "refund"
	MoneyChargeback         MoneyKind = "chargeback"
	MoneyChargebackReversal MoneyKind = "chargeback_reversal"
	MoneyFee                MoneyKind = "fee"
)

// HasParent reports whether movements of the kind belong to a payment (MoneyMovement.ParentID).
func (k MoneyKind) HasParent() bool {
	return k == MoneyRefund || k == MoneyChargeback || k == MoneyChargebackReversal
}

// Date is a calendar date.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the date of t in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// IsZero reports whether d is the zero Date, which means none.
func (d Date) IsZero() bool { return d == Date{} }

// String returns d as YYYY-MM-DD.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }

// DateRange is the days from Start through End, both included; the zero value means none.
type DateRange struct {
	Start, End Date
}

// IsZero reports whether r is the zero DateRange.
func (r DateRange) IsZero() bool { return r == DateRange{} }

// MoneyMovement is a movement of money on the provider's account (ADR-0074 §1), which
// finance.RecordIncome takes as an IncomeMovement (ADR-0073 §1). The provider returns only the
// movements whose amounts are final.
type MoneyMovement struct {
	// ID is the provider's ID, unique per provider (Stripe: the balance transaction).
	ID   string
	Kind MoneyKind
	// ParentID is the ID of the payment of a refund, a chargeback, or its reversal; "" otherwise.
	ParentID string
	// ParentGross is the Gross of the payment of ParentID, which tells a full refund from a
	// partial one; "" without a parent.
	ParentGross string
	// Subscription is the subscription that the movement belongs to; zero for a fee that belongs
	// to none.
	Subscription SubscriptionRef
	OccurredAt   time.Time
	// OccurredOn is OccurredAt as a date in the provider's Location.
	OccurredOn Date
	// Covers is the billing period that a payment pays for; zero for the other kinds.
	Covers DateRange
	// Gross, Fee, and Net are decimals in major units of Currency, Net = Gross - Fee, negative
	// for money out (refunds, chargebacks, fees).
	Gross, Fee, Net string
	// Currency is an ISO 4217 code.
	Currency string
	// Final reports that the provider will not change the amounts any more; always true of the
	// movements the port returns.
	Final bool
}

// NoticeKind is what a provider's notice points at.
type NoticeKind string

// The kinds of notices.
const (
	NoticeSubscription NoticeKind = "subscription"
	NoticeCheckout     NoticeKind = "checkout"
	NoticeMoney        NoticeKind = "money"
	// NoticeIgnored is an event of a type that this module does not act on.
	NoticeIgnored NoticeKind = "ignored"
)

// Notice is a provider's event: a signal to re-read an object, never its state (ADR-0074 T1).
type Notice struct {
	// EventID is the provider's ID of the event, unique per provider.
	EventID string
	// Type is the provider's name of the event's type, such as "invoice.paid".
	Type     string
	Kind     NoticeKind
	ObjectID string
	// Subscription is the subscription the event is about when the provider names it without
	// another request (ListChanges may fill it); zero otherwise.
	Subscription SubscriptionRef
	// OccurredAt is when the provider created the event.
	OccurredAt time.Time
}

// Change is what a notice resolves to: the objects to re-read.
type Change struct {
	Subscriptions []SubscriptionRef
	Checkout      CheckoutRef
	// Money are the final movements of the notice.
	Money []MoneyMovement
}

// SubscriptionPage is a page of ListSubscriptions.
type SubscriptionPage struct {
	Subscriptions []SubscriptionSnapshot
	// Next is the cursor of the next page; "" after the last one.
	Next string
}

// ChangePage is a page of ListChanges.
type ChangePage struct {
	Notices []Notice
	// Next is the cursor of the next page; "" after the last one.
	Next string
	// Until is the time through which the listing is complete once Next is "": the time the
	// next listing may start from.
	Until time.Time
}

// MoneyPage is a page of ListMoney.
type MoneyPage struct {
	Movements []MoneyMovement
	// Next is the cursor to pass to read what follows this page; it stays valid when the
	// provider books more movements later. It is the cursor passed in when the page is empty.
	Next string
	// More reports that movements after Next exist now.
	More bool
}
