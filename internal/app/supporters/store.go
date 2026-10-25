// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// The records of the supporter program, its plans, and their subscriptions (ADR-0072 §15,
// data-model §4.13), which Repository keeps.

// ErrRankTaken reports a plan whose rank another plan that is not retired has.
var ErrRankTaken = errors.New("supporters: another plan has the rank")

// ErrPlanRoleTaken reports a plan of a role that another plan has.
var ErrPlanRoleTaken = errors.New("supporters: another plan has the role")

// ErrLiveSubscription reports a subscription of a user who has a live one already.
var ErrLiveSubscription = errors.New("supporters: the user has a subscription already")

// ProgramChange is the supporter program enabled or disabled (supporter_program_changes).
type ProgramChange struct {
	ID      string
	Enabled bool
	// Provider is the payment provider of an enabled program, "" for a disabled one.
	Provider string
	// ChangedByUserID is "" for the CLI.
	ChangedByUserID string
	ChangedAt       time.Time
}

// Price is a recurring price of a plan at the provider (supporter_plan_prices).
type Price struct {
	ID          string
	PlanID      string
	Interval    supporters.Interval
	AmountMinor int64
	Currency    string
	Provider    string
	Ref         PriceRef
	CreatedAt   time.Time
	// Archived reports that the price was archived at the provider.
	Archived bool
}

// PlanRecord is a plan with its current terms, its retirement, and the prices that new
// subscriptions use.
type PlanRecord struct {
	supporters.Plan
	Revision supporters.PlanRevision
	// Retired reports a retirement, at RetiredAt.
	Retired   bool
	RetiredAt time.Time
	// Prices are the current prices by interval; an interval is missing until
	// JobEnsurePrices makes its price.
	Prices map[supporters.Interval]Price
}

// Customer is the provider's customer of a user (supporter_customers).
type Customer struct {
	ID        string
	UserID    string
	Provider  string
	Ref       CustomerRef
	CreatedAt time.Time
}

// Subscription is a subscription (supporter_subscriptions): the plan and the price it started
// with.
type Subscription struct {
	ID        string
	UserID    string
	PlanID    string
	PriceID   string
	Provider  string
	CreatedAt time.Time
}

// TransitionRecord is a transition of a subscription (supporter_subscription_transitions and
// its grace and scheduled change).
type TransitionRecord struct {
	ID             string
	SubscriptionID string
	supporters.Transition
	Reason     supporters.Reason
	OccurredAt time.Time
}

// CheckoutRecord is a hosted checkout of a pending subscription (supporter_checkouts).
type CheckoutRecord struct {
	ID             string
	SubscriptionID string
	Provider       string
	Ref            CheckoutRef
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

// SubscriptionRecord is a subscription with its current transition, the provider's
// subscription, and its latest checkout.
type SubscriptionRecord struct {
	Subscription
	Current TransitionRecord
	// Link is the provider's subscription; "" until the provider has one.
	Link SubscriptionRef
	// Checkout is the latest checkout; nil without one.
	Checkout *CheckoutRecord
}

// ReversalKind is the kind of a refund or a chargeback (supporter_subscription_reversals.kind).
type ReversalKind string

// The kinds of reversals.
const (
	ReversalFullRefund    ReversalKind = "full_refund"
	ReversalPartialRefund ReversalKind = "partial_refund"
	ReversalChargeback    ReversalKind = "chargeback"
)

// Revokes reports whether a reversal of the kind revokes the subscription (ADR-0072 §7).
func (k ReversalKind) Revokes() bool { return k == ReversalFullRefund || k == ReversalChargeback }

// Reversal is a refund or a chargeback of a payment of a subscription.
type Reversal struct {
	ID             string
	SubscriptionID string
	Kind           ReversalKind
	Provider       string
	// ProviderID is the provider's ID of the money movement.
	ProviderID string
	OccurredAt time.Time
}

// Deadline is when the Scheduler re-reads a subscription (supporter_subscription_deadlines).
type Deadline struct {
	SubscriptionID string
	Kind           supporters.Deadline
	DueAt          time.Time
}

// SubscriptionFilter narrows the list of the subscriptions for the administrators.
type SubscriptionFilter struct {
	// PlanID and State narrow to a plan and a state; "" for any.
	PlanID string
	State  supporters.State
	// Before is the ID after which the page starts, newest first; "" for the first page.
	Before string
	Limit  int
}

// SubscriptionUser is the subscriber of a subscription (ADR-0072 §12).
type SubscriptionUser struct {
	SubscriptionID string
	UserID         string
	PlanID         string
}

// Repository is the persistence port of the supporter program, the plans, and the
// subscriptions (ADR-0072 §15). Methods that take a uow.Tx run inside it; a nil tx, where a
// method allows it, reads outside any transaction. Lookups that find nothing return an error
// wrapping ErrNotFound. The database adapters pass testkit.SupporterRepositoryContract.
type Repository interface {
	// InsertProgramChange records the program enabled or disabled.
	InsertProgramChange(ctx context.Context, tx uow.Tx, c ProgramChange) error
	// LastProgramChange returns the newest program change; ErrNotFound without one. tx may be nil.
	LastProgramChange(ctx context.Context, tx uow.Tx) (ProgramChange, error)

	// InsertPlan records a plan; ErrPlanRoleTaken when another plan has its role.
	InsertPlan(ctx context.Context, tx uow.Tx, p supporters.Plan) error
	// InsertPlanRevision records new terms of a plan that is not retired and makes them its
	// current terms; ErrRankTaken when another plan that is not retired has the rank.
	InsertPlanRevision(ctx context.Context, tx uow.Tx, r supporters.PlanRevision) error
	// InsertPlanRetirement records the retirement of a plan and removes it from the current
	// plans.
	InsertPlanRetirement(ctx context.Context, tx uow.Tx, planID, byUserID string, at time.Time) error
	// ListPlans returns every plan, retired ones too, by rank; tx may be nil.
	ListPlans(ctx context.Context, tx uow.Tx) ([]PlanRecord, error)
	// FindPlan returns a plan; tx may be nil.
	FindPlan(ctx context.Context, tx uow.Tx, id string) (PlanRecord, error)

	// InsertPrice records a price; a price whose provider's ID is recorded writes nothing.
	InsertPrice(ctx context.Context, tx uow.Tx, p Price) error
	// SetPriceHead makes a price the current price of its plan and interval.
	SetPriceHead(ctx context.Context, tx uow.Tx, planID string, interval supporters.Interval, priceID string) error
	// InsertPriceArchival records that a price was archived at the provider; again writes nothing.
	InsertPriceArchival(ctx context.Context, tx uow.Tx, priceID string, at time.Time) error
	// FindPriceByRef returns the price of the provider's ID; tx may be nil.
	FindPriceByRef(ctx context.Context, tx uow.Tx, provider string, ref PriceRef) (Price, error)
	// FindPrice returns a price; tx may be nil.
	FindPrice(ctx context.Context, tx uow.Tx, id string) (Price, error)

	// InsertCustomer records the provider's customer of a user; a user or a customer recorded
	// already writes nothing.
	InsertCustomer(ctx context.Context, tx uow.Tx, c Customer) error
	// FindCustomer returns the provider's customer of a user; tx may be nil.
	FindCustomer(ctx context.Context, tx uow.Tx, provider, userID string) (Customer, error)

	// InsertSubscription records a subscription and its first transition, with its deadline;
	// ErrLiveSubscription when its user has a live subscription.
	InsertSubscription(ctx context.Context, tx uow.Tx, s Subscription, first TransitionRecord, d *Deadline) error
	// InsertCheckout records a checkout of a subscription.
	InsertCheckout(ctx context.Context, tx uow.Tx, c CheckoutRecord) error
	// FindSubscription returns a subscription; tx may be nil.
	FindSubscription(ctx context.Context, tx uow.Tx, id string) (SubscriptionRecord, error)
	// LockSubscription returns a subscription inside tx, so that its changes run one after
	// another (SELECT ... FOR UPDATE where the database needs it).
	LockSubscription(ctx context.Context, tx uow.Tx, id string) (SubscriptionRecord, error)
	// FindSubscriptionIDByRef returns our subscription of the provider's one; tx may be nil.
	FindSubscriptionIDByRef(ctx context.Context, tx uow.Tx, provider string, ref SubscriptionRef) (string, error)
	// FindSubscriptionIDByCheckout returns our subscription of a checkout; tx may be nil.
	FindSubscriptionIDByCheckout(ctx context.Context, tx uow.Tx, provider string, ref CheckoutRef) (string, error)
	// LiveSubscriptionID returns the live subscription of a user; tx may be nil.
	LiveSubscriptionID(ctx context.Context, tx uow.Tx, userID string) (string, error)
	// InsertLink records the provider's subscription of ours; it writes nothing when the
	// subscription has one.
	InsertLink(ctx context.Context, tx uow.Tx, subscriptionID, provider string, ref SubscriptionRef, at time.Time) error
	// InsertTransition records a transition of a subscription of userID and makes it current,
	// with its deadline (nil for none).
	InsertTransition(ctx context.Context, tx uow.Tx, t TransitionRecord, userID string, d *Deadline) error

	// InsertReversal records a refund or a chargeback and reports whether it did; one of a
	// provider's ID recorded already writes nothing.
	InsertReversal(ctx context.Context, tx uow.Tx, r Reversal) (bool, error)
	// HasRevokingReversal reports inside tx whether a full refund or a chargeback of the
	// subscription is recorded.
	HasRevokingReversal(ctx context.Context, tx uow.Tx, subscriptionID string) (bool, error)
	// KnownReversals returns the IDs among ids of the reversals of provider that are recorded.
	KnownReversals(ctx context.Context, provider string, ids []string) ([]string, error)

	// OpenCheckouts returns the latest checkouts of the pending subscriptions of provider that
	// have not expired at now.
	OpenCheckouts(ctx context.Context, provider string, now time.Time) ([]CheckoutRef, error)
	// DueDeadlines returns up to limit deadlines due at now, oldest first.
	DueDeadlines(ctx context.Context, now time.Time, limit int) ([]Deadline, error)
	// ListSubscriptions returns a page of the subscriptions, newest first.
	ListSubscriptions(ctx context.Context, f SubscriptionFilter) ([]SubscriptionRecord, error)
	// LiveSubscriptionIDsOfPlan returns inside tx the live subscriptions of a plan.
	LiveSubscriptionIDsOfPlan(ctx context.Context, tx uow.Tx, planID string) ([]string, error)
	// LiveSubscriptionIDs returns inside tx every live subscription, in order.
	LiveSubscriptionIDs(ctx context.Context, tx uow.Tx) ([]string, error)
	// CountLiveSubscriptions counts the live subscriptions; tx may be nil.
	CountLiveSubscriptions(ctx context.Context, tx uow.Tx) (int64, error)
	// CountMembers counts the subscriptions in a member state, by plan.
	CountMembers(ctx context.Context) (map[string]int64, error)
	// SubscriptionUsers returns the subscribers of the subscriptions among ids that exist.
	SubscriptionUsers(ctx context.Context, ids []string) ([]SubscriptionUser, error)

	// InsertListing puts a user on the supporter list at at; a listed user stays as listed.
	InsertListing(ctx context.Context, tx uow.Tx, userID string, at time.Time) error
	// HasListing reports whether a user is on the supporter list; tx may be nil.
	HasListing(ctx context.Context, tx uow.Tx, userID string) (bool, error)
	// ListListings returns a page of the listed users whose live subscription is in a member
	// state, of the plans that are not retired: by the plan's rank, then newest first, then by
	// user ID from the last.
	ListListings(ctx context.Context, f ListingFilter) ([]Listing, error)
	// DeleteListing removes a user from the supporter list.
	DeleteListing(ctx context.Context, tx uow.Tx, userID string) error
	// EraseUser deletes the listing and the customers of an erased user (ADR-0072 §10).
	EraseUser(ctx context.Context, tx uow.Tx, userID string) error
	// RebuildProjections derives the plan heads, the price heads, and the subscription heads,
	// live subscriptions, and deadlines again from the records.
	RebuildProjections(ctx context.Context, tx uow.Tx) error
}
