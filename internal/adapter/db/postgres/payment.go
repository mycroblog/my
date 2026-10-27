// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
)

// PaymentRepository implements supporters.PaymentRepository (data-model §4.13, ADR-0074 §4).
type PaymentRepository struct {
	db *DB
}

var _ supporters.PaymentRepository = (*PaymentRepository)(nil)

// PaymentRepository returns the repository of the payment records on this database.
func (d *DB) PaymentRepository() *PaymentRepository { return &PaymentRepository{db: d} }

// InsertWebhookReceipt implements supporters.PaymentRepository.
func (r *PaymentRepository) InsertWebhookReceipt(ctx context.Context, tx uow.Tx, w supporters.WebhookReceipt) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertPaymentWebhookReceipt(ctx, postgresgen.InsertPaymentWebhookReceiptParams{
		ID: w.ID, Provider: w.Provider, EventID: w.EventID, EventType: w.EventType, ObjectID: w.ObjectID, ReceivedAt: w.ReceivedAt,
	})
	return n > 0, wrap(err, "insert payment webhook receipt")
}

// LastWebhookReceipt implements supporters.PaymentRepository.
func (r *PaymentRepository) LastWebhookReceipt(ctx context.Context, provider string) (supporters.WebhookReceipt, error) {
	row, err := r.db.reader().LastPaymentWebhookReceipt(ctx, provider)
	if err != nil {
		return supporters.WebhookReceipt{}, notFound(err, supporters.ErrNotFound, "payment webhook receipt of "+provider)
	}
	return supporters.WebhookReceipt{
		ID: row.ID, Provider: row.Provider, EventID: row.EventID, EventType: row.EventType, ObjectID: row.ObjectID,
		ReceivedAt: row.ReceivedAt.UTC(),
	}, nil
}

// PurgeWebhookReceipts implements supporters.PaymentRepository.
func (r *PaymentRepository) PurgeWebhookReceipts(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error) {
	q, err := queries(tx)
	if err != nil {
		return 0, err
	}
	lim, err := toInt32(limit, "payment webhook receipt purge limit")
	if err != nil {
		return 0, err
	}
	n, err := q.PurgePaymentWebhookReceipts(ctx, postgresgen.PurgePaymentWebhookReceiptsParams{BeforeAt: before, Lim: lim})
	return n, wrap(err, "purge payment webhook receipts")
}

// InsertReconcileRun implements supporters.PaymentRepository.
func (r *PaymentRepository) InsertReconcileRun(ctx context.Context, tx uow.Tx, run supporters.ReconcileRun) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	counts := make([]int32, 3)
	for i, c := range []struct {
		n    int
		what string
	}{
		{run.Requests, "payment reconcile requests"}, {run.CheckedSubscriptions, "checked subscriptions"},
		{run.CorrectedSubscriptions, "corrected subscriptions"},
	} {
		if counts[i], err = toInt32(c.n, c.what); err != nil {
			return false, err
		}
	}
	n, err := q.InsertPaymentReconcileRun(ctx, postgresgen.InsertPaymentReconcileRunParams{
		ID: run.ID, Provider: run.Provider, RunKey: run.RunKey, Outcome: string(run.Outcome), ErrorKind: string(run.ErrorKind),
		ChangesUntil: run.ChangesUntil, ChangesCursor: run.ChangesCursor, MoneyCursor: run.MoneyCursor, SweepCursor: run.SweepCursor,
		Requests: counts[0], CheckedSubscriptions: counts[1], CorrectedSubscriptions: counts[2], OccurredAt: run.OccurredAt,
	})
	return n > 0, wrap(err, "insert payment reconcile run")
}

// ReconcileRunKeyUsed implements supporters.PaymentRepository.
func (r *PaymentRepository) ReconcileRunKeyUsed(ctx context.Context, provider, runKey string) (bool, error) {
	n, err := r.db.reader().CountPaymentReconcileRunsByKey(ctx, postgresgen.CountPaymentReconcileRunsByKeyParams{Provider: provider, RunKey: runKey})
	return n > 0, wrap(err, "count payment reconcile runs")
}

// LastReconcileRun implements supporters.PaymentRepository.
func (r *PaymentRepository) LastReconcileRun(ctx context.Context, provider string, resumable bool) (supporters.ReconcileRun, error) {
	row, err := r.db.reader().LastPaymentReconcileRun(ctx, postgresgen.LastPaymentReconcileRunParams{Provider: provider, IncludeFailed: !resumable})
	if err != nil {
		return supporters.ReconcileRun{}, notFound(err, supporters.ErrNotFound, "payment reconcile run of "+provider)
	}
	return supporters.ReconcileRun{
		ID: row.ID, Provider: row.Provider, RunKey: row.RunKey, Outcome: supporters.ReconcileOutcome(row.Outcome),
		ErrorKind: supporters.ErrorKind(row.ErrorKind), ChangesUntil: row.ChangesUntil.UTC(), ChangesCursor: row.ChangesCursor,
		MoneyCursor: row.MoneyCursor, SweepCursor: row.SweepCursor, Requests: int(row.Requests),
		CheckedSubscriptions: int(row.CheckedSubscriptions), CorrectedSubscriptions: int(row.CorrectedSubscriptions),
		OccurredAt: row.OccurredAt.UTC(),
	}, nil
}

// ClaimCheckout implements supporters.PaymentRepository.
func (r *PaymentRepository) ClaimCheckout(ctx context.Context, tx uow.Tx, c supporters.CheckoutClaim) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.ClaimPaymentCheckout(ctx, postgresgen.ClaimPaymentCheckoutParams{
		Provider: c.Provider, CheckoutID: c.CheckoutID, Holder: c.Holder, ClaimedAt: c.ClaimedAt, ExpiresAt: c.ExpiresAt,
	})
	return n > 0, wrap(err, "claim payment checkout")
}

// ReleaseCheckout implements supporters.PaymentRepository.
func (r *PaymentRepository) ReleaseCheckout(ctx context.Context, tx uow.Tx, provider, checkoutID, holder string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.ReleasePaymentCheckout(ctx, postgresgen.ReleasePaymentCheckoutParams{Provider: provider, CheckoutID: checkoutID, Holder: holder}),
		"release payment checkout")
}

// PurgeCheckoutClaims implements supporters.PaymentRepository.
func (r *PaymentRepository) PurgeCheckoutClaims(ctx context.Context, tx uow.Tx, before time.Time) (int64, error) {
	q, err := queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.PurgePaymentCheckoutClaims(ctx, before)
	return n, wrap(err, "purge payment checkout claims")
}
