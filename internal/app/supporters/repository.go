// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

// ErrNotFound reports a record that does not exist.
var ErrNotFound = errors.New("supporters: not found")

// WebhookReceipt is the record of a provider's webhook delivery (payment_webhook_receipts,
// ADR-0074 §4): the IDs and the type only, never the body (§5).
type WebhookReceipt struct {
	ID         string
	Provider   string
	EventID    string
	EventType  string
	ObjectID   string
	ReceivedAt time.Time
}

// ReconcileOutcome is how an attempt of the reconciliation ended.
type ReconcileOutcome string

// The outcomes of a reconciliation.
const (
	// ReconcileSucceeded: every stage finished.
	ReconcileSucceeded ReconcileOutcome = "succeeded"
	// ReconcilePartial: the attempt used its requests; a continuation job takes over from its
	// cursors.
	ReconcilePartial ReconcileOutcome = "partial"
	// ReconcileFailed: the provider failed; the next reconciliation starts from the cursors of
	// the latest succeeded or partial attempt.
	ReconcileFailed ReconcileOutcome = "failed"
)

// ReconcileRun is one attempt of the reconciliation (payment_reconcile_runs, ADR-0074 §4). The
// cursors are where the attempt stopped, "" for the start of a list.
type ReconcileRun struct {
	ID       string
	Provider string
	// RunKey is the idempotency key of the attempt's job: one row per job.
	RunKey    string
	Outcome   ReconcileOutcome
	ErrorKind ErrorKind
	// ChangesUntil is the time through which the changes are read; the next listing starts
	// ChangesOverlap before it.
	ChangesUntil time.Time
	// ChangesCursor is the page of the changes since ChangesUntil - ChangesOverlap to read next.
	ChangesCursor string
	MoneyCursor   string
	SweepCursor   string
	// Requests is how many requests the attempt sent to the provider.
	Requests int
	// CheckedSubscriptions is how many snapshots it handed to the Syncer, and
	// CorrectedSubscriptions how many of them changed our state.
	CheckedSubscriptions   int
	CorrectedSubscriptions int
	OccurredAt             time.Time
}

// PaymentRepository is the persistence port of the payment records of the supporters module
// (ADR-0074 §4).
type PaymentRepository interface {
	// InsertWebhookReceipt records a delivery and reports whether it did: a delivery of a
	// (provider, event ID) already recorded writes nothing.
	InsertWebhookReceipt(ctx context.Context, tx uow.Tx, r WebhookReceipt) (bool, error)
	// LastWebhookReceipt returns the newest delivery of provider; ErrNotFound without one.
	LastWebhookReceipt(ctx context.Context, provider string) (WebhookReceipt, error)
	// PurgeWebhookReceipts deletes up to limit deliveries received before before and returns
	// how many it deleted.
	PurgeWebhookReceipts(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error)

	// InsertReconcileRun records an attempt and reports whether it did: an attempt of a
	// (provider, run key) already recorded writes nothing.
	InsertReconcileRun(ctx context.Context, tx uow.Tx, r ReconcileRun) (bool, error)
	// ReconcileRunKeyUsed reports whether an attempt of provider recorded runKey.
	ReconcileRunKeyUsed(ctx context.Context, provider, runKey string) (bool, error)
	// LastReconcileRun returns the newest attempt of provider, only among the succeeded and
	// partial ones when resumable is true; ErrNotFound without one.
	LastReconcileRun(ctx context.Context, provider string, resumable bool) (ReconcileRun, error)

	// ClaimCheckout takes the claim c of a checkout and reports whether it did: it does when no
	// one holds the checkout or the claim of its holder expired at or before c.ClaimedAt.
	ClaimCheckout(ctx context.Context, tx uow.Tx, c CheckoutClaim) (bool, error)
	// ReleaseCheckout deletes the claim of the checkout of provider while holder holds it.
	ReleaseCheckout(ctx context.Context, tx uow.Tx, provider, checkoutID, holder string) error
	// PurgeCheckoutClaims deletes the claims that expired before before and returns how many.
	PurgeCheckoutClaims(ctx context.Context, tx uow.Tx, before time.Time) (int64, error)
}

// CheckoutClaim is the claim of a checkout of the provider's own pages (payment_checkout_claims):
// while Holder holds it, no other post of the checkout creates its subscription.
type CheckoutClaim struct {
	Provider   string
	CheckoutID string
	Holder     string
	ClaimedAt  time.Time
	ExpiresAt  time.Time
}
