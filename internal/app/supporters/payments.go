// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// Jobs and limits of the provider's facts (ADR-0074 §3, §4).
const (
	// JobPaymentEvent re-reads what a recorded webhook delivery points at and hands it to the
	// Syncer. Its idempotency key is "payment_event:<provider>:<event ID>" and its ordering key
	// the object that the event is about, so the jobs of one object run one at a time.
	JobPaymentEvent = "supporters.payment_event"
	// JobPaymentReconcile is the periodic reconciliation, every payments_reconcile_interval.
	JobPaymentReconcile = "supporters.payment_reconcile"
	// JobPaymentReceiptPurge deletes the webhook receipts older than ReceiptRetention.
	JobPaymentReceiptPurge = "supporters.payment_receipt_purge"

	// WebhookMaxBodyBytes bounds the body of a webhook delivery, read before its signature is
	// checked (413 beyond it).
	WebhookMaxBodyBytes = 1 << 20
	// ReceiptRetention is how long a webhook receipt is kept: the provider resends a delivery
	// for up to 3 days, and its CLI for up to 30.
	ReceiptRetention = 45 * 24 * time.Hour
	// ReceiptPurgeInterval is the period of JobPaymentReceiptPurge, and ReceiptPurgeBatch the
	// rows one statement deletes.
	ReceiptPurgeInterval = 24 * time.Hour
	ReceiptPurgeBatch    = 1000

	// DefaultReconcileInterval, MinReconcileInterval, and MaxReconcileInterval bound
	// payments_reconcile_interval.
	DefaultReconcileInterval = 6 * time.Hour
	MinReconcileInterval     = 15 * time.Minute
	MaxReconcileInterval     = 7 * 24 * time.Hour
	// DefaultReconcileMaxRequests is the default of payments_reconcile_max_requests.
	DefaultReconcileMaxRequests = 1000
	// ChangesOverlap is how far before the end of the previous listing the changes are read
	// again, for the events that the provider records late.
	ChangesOverlap = 5 * time.Minute
	// ContinuationDelay is how long after an attempt that used its requests its continuation runs.
	ContinuationDelay = time.Minute

	// The longest values that payment_webhook_receipts keeps.
	maxEventIDLength   = 255
	maxEventTypeLength = 100
	maxObjectIDLength  = 255
)

// WebhookIPLimit bounds the webhook deliveries from one client IP (ADR-0019 §5): far above what a
// provider sends, so that it only stops a flood of forged requests.
var WebhookIPLimit = kernel.Limit{Name: "payments.webhook.ip", Max: 3000, Window: 5 * time.Minute}

// PageIPLimit bounds the requests to the pages of a PageServer from one client IP (ADR-0099 §1):
// a checkout or a card change takes a handful, so it only stops a flood.
var PageIPLimit = kernel.Limit{Name: "payments.pages.ip", Max: 300, Window: 5 * time.Minute}

// PaymentsDeps are the ports of Payments.
type PaymentsDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository PaymentRepository
	Provider   PaymentProvider
	Syncer     Syncer
	Outbox     queue.Outbox
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// RateLimiter applies WebhookLimit and PageLimit; nil applies none.
	RateLimiter *kernel.RateLimiter
	// WebhookLimit defaults to WebhookIPLimit, and PageLimit to PageIPLimit.
	WebhookLimit kernel.Limit
	PageLimit    kernel.Limit
	// PageEnv is what the pages of a PageServer render with.
	PageEnv PageEnv
	// ReconcileInterval defaults to DefaultReconcileInterval, and ReconcileMaxRequests to
	// DefaultReconcileMaxRequests.
	ReconcileInterval    time.Duration
	ReconcileMaxRequests int
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// Payments carries the provider's facts to the Syncer (ADR-0074 §3, §4): the webhook deliveries,
// their jobs, and the reconciliation.
type Payments struct{ d PaymentsDeps }

// NewPayments returns Payments on d.
func NewPayments(d PaymentsDeps) *Payments {
	if d.WebhookLimit.Max == 0 {
		d.WebhookLimit = WebhookIPLimit
	}
	if d.PageLimit.Max == 0 {
		d.PageLimit = PageIPLimit
	}
	if d.ReconcileInterval <= 0 {
		d.ReconcileInterval = DefaultReconcileInterval
	}
	if d.ReconcileMaxRequests <= 0 {
		d.ReconcileMaxRequests = DefaultReconcileMaxRequests
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Payments{d: d}
}

// Provider returns the name of the provider.
func (p *Payments) Provider() string { return p.d.Provider.Name() }

func (p *Payments) now() time.Time { return kernel.Normalize(p.d.Clock.Now()) }

// PagesResult is what Pages decided about a request to the pages of a provider.
type PagesResult struct {
	// Handler serves the request; nil when the provider is another than the configured one or
	// serves no pages (404), or when the rate limit refused the request (429).
	Handler http.Handler
	// RateLimited reports that PageLimit refused the request, until RetryAfter.
	RateLimited bool
	RetryAfter  time.Duration
}

// Pages returns the handler of a request to the pages of provider (ADR-0099 §1): the
// configured provider's when it is a PageServer and the client IP is within PageLimit.
func (p *Payments) Pages(ctx context.Context, provider, clientIP string) (PagesResult, error) {
	ps, ok := p.d.Provider.(PageServer)
	if !ok || provider != p.d.Provider.Name() {
		return PagesResult{}, nil
	}
	if p.d.RateLimiter != nil {
		d, err := p.d.RateLimiter.Take(ctx, p.d.PageLimit, kernel.ClientKey(clientIP))
		if err != nil {
			return PagesResult{}, err
		}
		if !d.Allowed {
			return PagesResult{RateLimited: true, RetryAfter: d.RetryAfter}, nil
		}
	}
	env := p.d.PageEnv
	if env.Logger == nil {
		env.Logger = p.d.Logger
	}
	if env.Stylesheet == nil {
		env.Stylesheet = func() string { return "" }
	}
	if env.Checkouts == nil {
		env.Checkouts = p
	}
	return PagesResult{Handler: ps.Pages(env)}, nil
}

// ClaimCheckout implements CheckoutClaims on payment_checkout_claims: it takes the claim in one
// short transaction, and release deletes it in another, so that no call of the provider runs
// inside a transaction (ADR-0003). A claim that its process never releases expires after
// CheckoutClaimHold.
func (p *Payments) ClaimCheckout(ctx context.Context, checkoutID string) (func(), bool, error) {
	now := p.now()
	c := CheckoutClaim{
		Provider: p.d.Provider.Name(), CheckoutID: checkoutID, Holder: p.d.IDs.NewID(),
		ClaimedAt: now, ExpiresAt: now.Add(CheckoutClaimHold),
	}
	var claimed bool
	err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		claimed, err = p.d.Repository.ClaimCheckout(ctx, tx, c)
		return err
	})
	if err != nil {
		return nil, false, fmt.Errorf("supporters: claim checkout: %w", err)
	}
	if !claimed {
		return nil, false, nil
	}
	release := func() {
		// The claim is released even when the request was canceled.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return p.d.Repository.ReleaseCheckout(ctx, tx, c.Provider, c.CheckoutID, c.Holder)
		})
		if err != nil {
			p.d.Logger.WarnContext(ctx, "the claim of a checkout could not be released; it expires on its own",
				slog.String("provider", c.Provider), slog.Any("error", err))
		}
	}
	return release, true, nil
}

// WebhookRequest is a webhook delivery as the endpoint received it.
type WebhookRequest struct {
	// Provider is the {provider} of the path.
	Provider string
	Header   http.Header
	// Body is the raw body, at most WebhookMaxBodyBytes.
	Body     []byte
	ClientIP string
}

// WebhookOutcome is what ReceiveWebhook did with a delivery.
type WebhookOutcome string

// The outcomes of a delivery and the status that the endpoint answers with.
const (
	// WebhookRecorded: recorded with its job (2xx).
	WebhookRecorded WebhookOutcome = "recorded"
	// WebhookDuplicate: the event was recorded before (2xx).
	WebhookDuplicate WebhookOutcome = "duplicate"
	// WebhookIgnored: an event of a type this module does not act on, not recorded (2xx).
	WebhookIgnored WebhookOutcome = "ignored"
	// WebhookRejected: the signature, its time, the mode, or the body is wrong (400).
	WebhookRejected WebhookOutcome = "rejected"
	// WebhookRateLimited: WebhookLimit refused it (429).
	WebhookRateLimited WebhookOutcome = "rate_limited"
	// WebhookUnknownProvider: the path names another provider than the configured one (404).
	WebhookUnknownProvider WebhookOutcome = "unknown_provider"
)

// WebhookResult is the outcome of a delivery.
type WebhookResult struct {
	Outcome WebhookOutcome
	// ErrorKind is why a delivery was rejected.
	ErrorKind ErrorKind
	// RetryAfter is when the rate limit allows a delivery again.
	RetryAfter time.Duration
	// Notice is what a delivery that was not refused points at.
	Notice Notice
}

// ReceiveWebhook handles a webhook delivery (ADR-0074 §3) without any I/O but the database:
// the rate limit of its client IP, the signature through the provider's ParseWebhook, and, in
// one short transaction, its receipt (unique by provider and event ID) and its JobPaymentEvent.
// A delivery that was recorded before writes nothing. The error is the database's, which the
// endpoint answers with 503 for the provider to resend.
func (p *Payments) ReceiveWebhook(ctx context.Context, req WebhookRequest) (WebhookResult, error) {
	prov := p.d.Provider
	if req.Provider != prov.Name() {
		return WebhookResult{Outcome: WebhookUnknownProvider}, nil
	}
	if p.d.RateLimiter != nil {
		d, err := p.d.RateLimiter.Take(ctx, p.d.WebhookLimit, kernel.ClientKey(req.ClientIP))
		if err != nil {
			return WebhookResult{}, err
		}
		if !d.Allowed {
			return WebhookResult{Outcome: WebhookRateLimited, RetryAfter: d.RetryAfter}, nil
		}
	}
	now := p.now()
	if len(req.Body) > WebhookMaxBodyBytes {
		return WebhookResult{Outcome: WebhookRejected, ErrorKind: ErrorInvalidRequest}, nil
	}
	n, err := prov.ParseWebhook(req.Header, req.Body, now)
	if err != nil {
		return WebhookResult{Outcome: WebhookRejected, ErrorKind: ErrorKindOf(err)}, nil
	}
	if n.Kind == NoticeIgnored {
		return WebhookResult{Outcome: WebhookIgnored, Notice: n}, nil
	}
	if !validNotice(n) {
		p.d.Logger.WarnContext(ctx, "payment webhook with an unusable notice", slog.String("provider", prov.Name()),
			slog.String("event_type", n.Type))
		return WebhookResult{Outcome: WebhookRejected, ErrorKind: ErrorInvalidResponse}, nil
	}
	job, err := p.eventJob(n)
	if err != nil {
		return WebhookResult{}, err
	}
	receipt := WebhookReceipt{
		ID: p.d.IDs.NewID(), Provider: prov.Name(), EventID: n.EventID, EventType: n.Type, ObjectID: n.ObjectID, ReceivedAt: now,
	}
	recorded := false
	err = p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted, err := p.d.Repository.InsertWebhookReceipt(ctx, tx, receipt)
		if err != nil || !inserted {
			recorded = false
			return err
		}
		recorded = true
		return p.d.Outbox.EnqueueTx(ctx, tx, job)
	})
	if err != nil {
		return WebhookResult{}, fmt.Errorf("supporters: record the webhook of %s: %w", prov.Name(), err)
	}
	if !recorded {
		return WebhookResult{Outcome: WebhookDuplicate, Notice: n}, nil
	}
	return WebhookResult{Outcome: WebhookRecorded, Notice: n}, nil
}

func validNotice(n Notice) bool {
	ok := func(s string, maxLen int) bool {
		return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= maxLen
	}
	switch n.Kind {
	case NoticeSubscription, NoticeCheckout, NoticeMoney:
	default:
		return false
	}
	return ok(n.EventID, maxEventIDLength) && ok(n.Type, maxEventTypeLength) && ok(n.ObjectID, maxObjectIDLength)
}

// eventPayload is the payload of JobPaymentEvent: the notice of the delivery.
type eventPayload struct {
	Provider     string    `json:"provider"`
	EventID      string    `json:"event_id"`
	Type         string    `json:"type"`
	Kind         string    `json:"kind"`
	ObjectID     string    `json:"object_id"`
	Subscription string    `json:"subscription,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
}

// EventJobKey returns the idempotency key of the JobPaymentEvent of an event.
func EventJobKey(provider, eventID string) string { return "payment_event:" + provider + ":" + eventID }

func (p *Payments) eventJob(n Notice) (queue.NewJob, error) {
	name := p.d.Provider.Name()
	b, err := json.Marshal(eventPayload{
		Provider: name, EventID: n.EventID, Type: n.Type, Kind: string(n.Kind), ObjectID: n.ObjectID,
		Subscription: string(n.Subscription), OccurredAt: n.OccurredAt,
	})
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{
		Type: JobPaymentEvent, Payload: b, IdempotencyKey: EventJobKey(name, n.EventID),
		OrderingKey: "payment:" + name + ":" + n.ObjectID,
	}, nil
}

// reconcilePayload is the payload of JobPaymentReconcile.
type reconcilePayload struct {
	Provider string `json:"provider"`
}

func (p *Payments) reconcileJob(key string, runAt time.Time) queue.NewJob {
	b, err := json.Marshal(reconcilePayload{Provider: p.d.Provider.Name()})
	if err != nil {
		panic(err) // a string always marshals
	}
	return queue.NewJob{Type: JobPaymentReconcile, Payload: b, IdempotencyKey: key, RunAt: runAt}
}

// RegisterJobs registers the handlers of JobPaymentEvent, JobPaymentReconcile, and
// JobPaymentReceiptPurge and, when sc is not nil, the schedules of the reconciliation (every
// ReconcileInterval) and of the purge.
func (p *Payments) RegisterJobs(h *queue.JobHandlerRegistry, sc *queue.Scheduler) {
	h.Register(JobPaymentEvent, func(ctx context.Context, j queue.Job) queue.Result {
		var pl eventPayload
		if err := json.Unmarshal(j.Payload, &pl); err != nil || pl.EventID == "" {
			return queue.Fail(fmt.Errorf("supporters: the payload of %s: %w", j.ID, err))
		}
		if pl.Provider != p.d.Provider.Name() {
			p.d.Logger.WarnContext(ctx, "payment event of a provider that is not configured", slog.String("provider", pl.Provider))
			return queue.Done()
		}
		return p.HandleNotice(ctx, Notice{
			EventID: pl.EventID, Type: pl.Type, Kind: NoticeKind(pl.Kind), ObjectID: pl.ObjectID,
			Subscription: SubscriptionRef(pl.Subscription), OccurredAt: pl.OccurredAt,
		})
	})
	h.Register(JobPaymentReconcile, func(ctx context.Context, j queue.Job) queue.Result {
		var pl reconcilePayload
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("supporters: the payload of %s: %w", j.ID, err))
		}
		if pl.Provider != p.d.Provider.Name() {
			p.d.Logger.WarnContext(ctx, "reconciliation of a provider that is not configured", slog.String("provider", pl.Provider))
			return queue.Done()
		}
		key := j.IdempotencyKey
		if key == "" {
			key = "job:" + j.ID
		}
		return p.Reconcile(ctx, key)
	})
	h.Register(JobPaymentReceiptPurge, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := p.PurgeReceipts(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	if sc == nil {
		return
	}
	sc.Add(queue.Schedule{Name: JobPaymentReconcile, Every: p.d.ReconcileInterval, Job: p.reconcileJob("", time.Time{})})
	sc.Add(queue.Schedule{Name: JobPaymentReceiptPurge, Every: ReceiptPurgeInterval, Job: queue.NewJob{Type: JobPaymentReceiptPurge}})
}

// PurgeReceipts deletes the webhook receipts older than ReceiptRetention and returns how many.
// A delivery of a purged event that comes again is recorded again, and its job re-reads and
// applies the provider's state, which changes nothing. It also deletes the claims of checkouts
// that expired (ClaimCheckout), which it does not count.
func (p *Payments) PurgeReceipts(ctx context.Context) (int64, error) {
	now := p.now()
	if err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := p.d.Repository.PurgeCheckoutClaims(ctx, tx, now)
		return err
	}); err != nil {
		return 0, err
	}
	before := now.Add(-ReceiptRetention)
	var total int64
	for {
		var n int64
		err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = p.d.Repository.PurgeWebhookReceipts(ctx, tx, before, ReceiptPurgeBatch)
			return err
		})
		total += n
		if err != nil || n < ReceiptPurgeBatch {
			return total, err
		}
	}
}

// providerFailure returns the result of a job whose call of the provider failed: a retry for
// the failures that ErrorKind.Retried passes; the others are logged, and the reconciliation and
// the provider's state on the admin screen show them.
func (p *Payments) providerFailure(ctx context.Context, what string, err error) queue.Result {
	kind := ErrorKindOf(err)
	if kind.Retried() {
		return queue.Retry(0, fmt.Errorf("supporters: %s: %w", what, err))
	}
	p.d.Logger.WarnContext(ctx, "payment provider failed", slog.String("provider", p.d.Provider.Name()),
		slog.String("call", what), slog.String("error_kind", string(kind)), slog.Any("error", err))
	return queue.Done()
}

// HandleNotice is the work of JobPaymentEvent (ADR-0074 §3): it resolves the notice through the
// provider, re-reads the checkout and every subscription it points at, and hands what it read
// and the notice's final money movements to the Syncer. The state in the event's body is never
// used, so duplicates, reordering, and losses all come down to "re-read and apply".
func (p *Payments) HandleNotice(ctx context.Context, n Notice) queue.Result {
	prov, name := p.d.Provider, p.d.Provider.Name()
	ch, err := prov.Resolve(ctx, n)
	if err != nil {
		return p.providerFailure(ctx, "resolve "+n.Type, err)
	}
	subs := slices.Clone(ch.Subscriptions)
	if n.Subscription != "" {
		subs = append(subs, n.Subscription)
	}
	if ch.Checkout != "" {
		res, err := prov.GetCheckout(ctx, ch.Checkout)
		if err != nil {
			return p.providerFailure(ctx, "get checkout", err)
		}
		if _, err := p.d.Syncer.SyncCheckout(ctx, name, res); err != nil {
			return queue.Retry(0, err)
		}
		if res.Subscription != "" {
			subs = append(subs, res.Subscription)
		}
	}
	slices.Sort(subs)
	for _, ref := range slices.Compact(subs) {
		if ref == "" {
			continue
		}
		snap, err := readSubscription(ctx, prov, p.d.Clock, ref)
		if err != nil {
			return p.providerFailure(ctx, "get subscription", err)
		}
		if _, err := p.d.Syncer.SyncSubscription(ctx, name, snap); err != nil {
			return queue.Retry(0, err)
		}
	}
	if len(ch.Money) > 0 {
		if err := p.recordMoney(ctx, ch.Money, func() bool { return true }); err != nil {
			if errors.Is(err, errSyncer) {
				return queue.Retry(0, err)
			}
			return p.providerFailure(ctx, "get money", err)
		}
	}
	return queue.Done()
}

// errSyncer marks an error of the Syncer, which the jobs retry.
var errSyncer = errors.New("supporters: sync")

// errRequestCap stops a reconciliation that used its requests.
var errRequestCap = errors.New("supporters: the reconciliation used its requests")

// recordMoney hands final movements to the Syncer with the payments of their refunds,
// chargebacks, and reversals that are not recorded yet, re-read first with GetMoney (ADR-0073
// §1). take reports whether one more request may be sent; without one it fails with
// errRequestCap and records nothing.
func (p *Payments) recordMoney(ctx context.Context, ms []MoneyMovement, take func() bool) error {
	prov, name := p.d.Provider, p.d.Provider.Name()
	ms = slices.Clone(ms)
	// Payments and fees first, so that a parent in the batch comes before its children.
	slices.SortStableFunc(ms, func(a, b MoneyMovement) int {
		switch {
		case !a.Kind.HasParent() && b.Kind.HasParent():
			return -1
		case a.Kind.HasParent() && !b.Kind.HasParent():
			return 1
		}
		return 0
	})
	inBatch := map[string]bool{}
	var parents []string
	for _, m := range ms {
		inBatch[m.ID] = true
	}
	for _, m := range ms {
		if m.Kind.HasParent() && m.ParentID != "" && !inBatch[m.ParentID] && !slices.Contains(parents, m.ParentID) {
			parents = append(parents, m.ParentID)
		}
	}
	var fetched []MoneyMovement
	if len(parents) > 0 {
		known, err := p.d.Syncer.KnownMoney(ctx, name, parents)
		if err != nil {
			return fmt.Errorf("%w: %w", errSyncer, err)
		}
		for _, id := range parents {
			if known[id] {
				continue
			}
			if !take() {
				return errRequestCap
			}
			m, err := prov.GetMoney(ctx, id)
			if err != nil {
				return err
			}
			fetched = append(fetched, m)
		}
	}
	if err := p.d.Syncer.RecordMoney(ctx, name, append(fetched, ms...)); err != nil {
		return fmt.Errorf("%w: %w", errSyncer, err)
	}
	return nil
}

// PaymentStatus is the state of the provider for the administrators (ADR-0074 §4).
type PaymentStatus struct {
	Provider string
	Mode     Mode
	Location *time.Location
	// LastWebhook is the newest webhook receipt, and LastRun the newest attempt of the
	// reconciliation; nil without one.
	LastWebhook *WebhookReceipt
	LastRun     *ReconcileRun
}

// NoWebhook reports whether no webhook delivery is recorded (within ReceiptRetention): the
// provider's webhook may not be set up (ADR-0074 §4).
func (s PaymentStatus) NoWebhook() bool { return s.LastWebhook == nil }

// ReconcileCorrected reports whether the last reconciliation changed subscriptions that the
// webhook deliveries should have changed: the provider's webhook may miss events.
func (s PaymentStatus) ReconcileCorrected() bool {
	return s.LastRun != nil && s.LastRun.CorrectedSubscriptions > 0
}

// Status returns the state of the provider.
func (p *Payments) Status(ctx context.Context) (PaymentStatus, error) {
	prov := p.d.Provider
	st := PaymentStatus{Provider: prov.Name(), Mode: prov.Mode(), Location: prov.Location()}
	w, err := p.d.Repository.LastWebhookReceipt(ctx, prov.Name())
	switch {
	case err == nil:
		st.LastWebhook = &w
	case !errors.Is(err, ErrNotFound):
		return PaymentStatus{}, err
	}
	r, err := p.d.Repository.LastReconcileRun(ctx, prov.Name(), false)
	switch {
	case err == nil:
		st.LastRun = &r
	case !errors.Is(err, ErrNotFound):
		return PaymentStatus{}, err
	}
	return st, nil
}

// ErrReconcileTooSoon: a reconciliation by hand already ran in this UTC hour.
var ErrReconcileTooSoon = errors.New("supporters: the reconciliation of this hour already ran")

// ManualReconcileKey returns the idempotency key of a reconciliation by hand of provider at t:
// one per UTC hour (ADR-0074 §4).
func ManualReconcileKey(provider string, t time.Time) string {
	return "manual:" + provider + ":" + t.UTC().Format("2006-01-02T15")
}

// RequestReconcile queues a reconciliation now, besides the periodic one, at most once per UTC
// hour (ADR-0074 §4), and records the request in the audit log. byUserID is the administrator,
// "" for the CLI. It fails with ErrReconcileTooSoon when the reconciliation of the hour already
// ran. Its requests count toward ReconcileMaxRequests like the others.
func (p *Payments) RequestReconcile(ctx context.Context, byUserID string) error {
	name := p.d.Provider.Name()
	now := p.now()
	key := ManualReconcileKey(name, now)
	used, err := p.d.Repository.ReconcileRunKeyUsed(ctx, name, key)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("%w: %s", ErrReconcileTooSoon, name)
	}
	e := audit(p.d.IDs.NewID(), byUserID, appmoderation.AuditPaymentReconcileQueued, appmoderation.AuditTargetPaymentProvider,
		name, name, "", now)
	return p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := tx.Emit(ctx, e); err != nil {
			return err
		}
		return p.d.Outbox.EnqueueTx(ctx, tx, p.reconcileJob(key, time.Time{}))
	})
}
