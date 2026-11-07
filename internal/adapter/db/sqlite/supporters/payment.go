// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/supportersgen"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
)

// PaymentRepository implements supporters.PaymentRepository (data-model §4.13, ADR-0074 §4).
type PaymentRepository struct {
	db *moduleDB
}

var _ supporters.PaymentRepository = (*PaymentRepository)(nil)

// NewPaymentRepository returns the repository of the payment records on this database.
func NewPaymentRepository(d *sqlite.DB) *PaymentRepository {
	return &PaymentRepository{db: &moduleDB{d}}
}

// InsertWebhookReceipt implements supporters.PaymentRepository.
func (r *PaymentRepository) InsertWebhookReceipt(ctx context.Context, tx uow.Tx, w supporters.WebhookReceipt) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertPaymentWebhookReceipt(ctx, supportersgen.InsertPaymentWebhookReceiptParams{
		ID: w.ID, Provider: w.Provider, EventID: w.EventID, EventType: w.EventType, ObjectID: w.ObjectID,
		ReceivedAt: sqlite.ToMillis(w.ReceivedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert payment webhook receipt")
}

// LastWebhookReceipt implements supporters.PaymentRepository.
func (r *PaymentRepository) LastWebhookReceipt(ctx context.Context, provider string) (supporters.WebhookReceipt, error) {
	row, err := r.db.reader().LastPaymentWebhookReceipt(ctx, provider)
	if err != nil {
		return supporters.WebhookReceipt{}, sqlite.NotFound(err, supporters.ErrNotFound, "payment webhook receipt of "+provider)
	}
	return supporters.WebhookReceipt{
		ID: row.ID, Provider: row.Provider, EventID: row.EventID, EventType: row.EventType, ObjectID: row.ObjectID,
		ReceivedAt: sqlite.FromMillis(row.ReceivedAt),
	}, nil
}

// CountUnsettledDuplicates implements supporters.PaymentRepository.
func (r *PaymentRepository) CountUnsettledDuplicates(ctx context.Context, provider string) (int, error) {
	n, err := r.db.reader().CountUnsettledSupporterDuplicates(ctx, provider)
	if err != nil {
		return 0, sqlite.Wrap(err, "count unsettled supporter duplicates")
	}
	return int(n), nil
}

// PurgeWebhookReceipts implements supporters.PaymentRepository.
func (r *PaymentRepository) PurgeWebhookReceipts(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.PurgePaymentWebhookReceipts(ctx, supportersgen.PurgePaymentWebhookReceiptsParams{BeforeAt: sqlite.ToMillis(before), Lim: int64(limit)})
	return n, sqlite.Wrap(err, "purge payment webhook receipts")
}

// InsertReconcileRun implements supporters.PaymentRepository.
func (r *PaymentRepository) InsertReconcileRun(ctx context.Context, tx uow.Tx, run supporters.ReconcileRun) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertPaymentReconcileRun(ctx, supportersgen.InsertPaymentReconcileRunParams{
		ID: run.ID, Provider: run.Provider, RunKey: run.RunKey, Outcome: string(run.Outcome), ErrorKind: string(run.ErrorKind),
		ChangesUntil: sqlite.ToMillis(run.ChangesUntil), ChangesCursor: run.ChangesCursor, MoneyCursor: run.MoneyCursor,
		SweepCursor: run.SweepCursor, Requests: int64(run.Requests), CheckedSubscriptions: int64(run.CheckedSubscriptions),
		CorrectedSubscriptions: int64(run.CorrectedSubscriptions), OccurredAt: sqlite.ToMillis(run.OccurredAt),
	})
	return n > 0, sqlite.Wrap(err, "insert payment reconcile run")
}

// ReconcileRunKeyUsed implements supporters.PaymentRepository.
func (r *PaymentRepository) ReconcileRunKeyUsed(ctx context.Context, provider, runKey string) (bool, error) {
	n, err := r.db.reader().CountPaymentReconcileRunsByKey(ctx, supportersgen.CountPaymentReconcileRunsByKeyParams{Provider: provider, RunKey: runKey})
	return n > 0, sqlite.Wrap(err, "count payment reconcile runs")
}

// LastReconcileRun implements supporters.PaymentRepository.
func (r *PaymentRepository) LastReconcileRun(ctx context.Context, provider string, resumable bool) (supporters.ReconcileRun, error) {
	var includeFailed int64
	if !resumable {
		includeFailed = 1
	}
	row, err := r.db.reader().LastPaymentReconcileRun(ctx, supportersgen.LastPaymentReconcileRunParams{Provider: provider, IncludeFailed: includeFailed})
	if err != nil {
		return supporters.ReconcileRun{}, sqlite.NotFound(err, supporters.ErrNotFound, "payment reconcile run of "+provider)
	}
	return supporters.ReconcileRun{
		ID: row.ID, Provider: row.Provider, RunKey: row.RunKey, Outcome: supporters.ReconcileOutcome(row.Outcome),
		ErrorKind: supporters.ErrorKind(row.ErrorKind), ChangesUntil: sqlite.FromMillis(row.ChangesUntil), ChangesCursor: row.ChangesCursor,
		MoneyCursor: row.MoneyCursor, SweepCursor: row.SweepCursor, Requests: int(row.Requests),
		CheckedSubscriptions: int(row.CheckedSubscriptions), CorrectedSubscriptions: int(row.CorrectedSubscriptions),
		OccurredAt: sqlite.FromMillis(row.OccurredAt),
	}, nil
}

// ClaimCheckout implements supporters.PaymentRepository.
func (r *PaymentRepository) ClaimCheckout(ctx context.Context, tx uow.Tx, c supporters.CheckoutClaim) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.ClaimPaymentCheckout(ctx, supportersgen.ClaimPaymentCheckoutParams{
		Provider: c.Provider, ProviderCheckoutID: c.CheckoutID, Holder: c.Holder, ClaimedAt: sqlite.ToMillis(c.ClaimedAt), ExpiresAt: sqlite.ToMillis(c.ExpiresAt),
	})
	return n > 0, sqlite.Wrap(err, "claim payment checkout")
}

// ReleaseCheckout implements supporters.PaymentRepository.
func (r *PaymentRepository) ReleaseCheckout(ctx context.Context, tx uow.Tx, provider, checkoutID, holder string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.ReleasePaymentCheckout(ctx, supportersgen.ReleasePaymentCheckoutParams{Provider: provider, ProviderCheckoutID: checkoutID, Holder: holder}),
		"release payment checkout")
}

// PurgeCheckoutClaims implements supporters.PaymentRepository.
func (r *PaymentRepository) PurgeCheckoutClaims(ctx context.Context, tx uow.Tx, before time.Time) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.PurgePaymentCheckoutClaims(ctx, sqlite.ToMillis(before))
	return n, sqlite.Wrap(err, "purge payment checkout claims")
}
