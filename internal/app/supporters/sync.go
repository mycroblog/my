// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"time"
)

// Syncer is the subscription core of this module (ADR-0072 §7, §17): the use case
// supporters.sync, which derives our state of a subscription from the provider's facts and
// grants or revokes the plan's role. The webhook job and the reconciliation hand it what they
// re-read from the provider, in no particular order and possibly more than once, so each
// method is idempotent and does not depend on the order of the calls. Each call writes in its
// own short transactions and never calls the provider.
type Syncer interface {
	// SyncSubscription applies a snapshot just re-read from provider. A snapshot whose LocalID
	// is not one of our subscriptions is ignored. It reports whether our state changed.
	SyncSubscription(ctx context.Context, provider string, snap SubscriptionSnapshot) (bool, error)
	// SyncCheckout applies what GetCheckout reported of a checkout: a completed one links our
	// subscription (LocalID) to the provider's customer and subscription. It reports whether our
	// state changed.
	SyncCheckout(ctx context.Context, provider string, res CheckoutResult) (bool, error)
	// RecordMoney records final money movements of provider (finance.RecordIncome, ADR-0073)
	// and the refunds and chargebacks that revoke a subscription (ADR-0072 §7). The parent of
	// each refund, chargeback, and reversal is either recorded already or comes earlier in ms.
	RecordMoney(ctx context.Context, provider string, ms []MoneyMovement) error
	// KnownMoney reports which of the movement IDs of provider are recorded.
	KnownMoney(ctx context.Context, provider string, ids []string) (map[string]bool, error)
	// RetryDuplicates queues again the settling of the duplicates of provider that are not settled;
	// cause makes the keys of the jobs.
	RetryDuplicates(ctx context.Context, provider, cause string) error
	// OpenCheckouts returns the checkouts of provider that our subscriptions wait for and that
	// have not expired at now (ADR-0074 §4, stage 2).
	OpenCheckouts(ctx context.Context, provider string, now time.Time) ([]CheckoutRef, error)
}
