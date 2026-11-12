// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/supportersgen"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
)

// The tips (ADR-0162 §12).

// InsertTipChange implements supporters.TipRepository.
func (r *SupporterRepository) InsertTipChange(ctx context.Context, tx uow.Tx, c supporters.TipChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	state := "disabled"
	if c.Enabled {
		state = "enabled"
	}
	if err := q.InsertSupporterTipChange(ctx, supportersgen.InsertSupporterTipChangeParams{
		ID: c.ID, State: state, Currency: c.Currency, ChangedByUserID: postgres.ToNullString(c.ChangedByUserID), ChangedAt: c.ChangedAt,
	}); err != nil {
		return postgres.Wrap(err, "insert supporter tip change")
	}
	if !c.Enabled {
		return nil
	}
	for _, a := range c.Amounts {
		if err := q.InsertSupporterTipAmount(ctx, supportersgen.InsertSupporterTipAmountParams{ChangeID: c.ID, AmountMinor: a}); err != nil {
			return postgres.Wrap(err, "insert supporter tip amount")
		}
	}
	return nil
}

// LastTipChange implements supporters.TipRepository.
func (r *SupporterRepository) LastTipChange(ctx context.Context, tx uow.Tx) (supporters.TipChange, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.TipChange{}, err
	}
	row, err := q.GetLastSupporterTipChange(ctx)
	if err != nil {
		return supporters.TipChange{}, postgres.NotFound(err, supporters.ErrNotFound, "supporter tip change")
	}
	c := supporters.TipChange{
		ID: row.ID, Enabled: row.State == "enabled", Currency: row.Currency, ChangedByUserID: postgres.FromNullString(row.ChangedByUserID),
		ChangedAt: postgres.UTC(row.ChangedAt),
	}
	if c.Enabled {
		if c.Amounts, err = q.ListSupporterTipAmounts(ctx, row.ID); err != nil {
			return supporters.TipChange{}, postgres.Wrap(err, "list supporter tip amounts")
		}
	}
	return c, nil
}

// InsertTip implements supporters.TipRepository.
func (r *SupporterRepository) InsertTip(ctx context.Context, tx uow.Tx, t supporters.Tip, expiresAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.InsertSupporterTip(ctx, supportersgen.InsertSupporterTipParams{
		ID: t.ID, UserID: t.UserID, AmountMinor: t.AmountMinor, Currency: t.Currency, Provider: t.Provider, CreatedAt: t.CreatedAt,
	}); err != nil {
		return postgres.Wrap(err, "insert supporter tip")
	}
	err = q.InsertSupporterOpenTip(ctx, supportersgen.InsertSupporterOpenTipParams{UserID: t.UserID, TipID: t.ID, ExpiresAt: expiresAt})
	if postgres.IsUniqueViolation(err, "supporter_open_tips_pkey") {
		return supporters.ErrOpenTip
	}
	return postgres.Wrap(err, "insert supporter open tip")
}

// InsertTipCheckout implements supporters.TipRepository.
func (r *SupporterRepository) InsertTipCheckout(ctx context.Context, tx uow.Tx, c supporters.TipCheckout) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertSupporterTipCheckout(ctx, supportersgen.InsertSupporterTipCheckoutParams{
		TipID: c.TipID, Provider: c.Provider, ProviderCheckoutID: string(c.Ref), ExpiresAt: c.ExpiresAt, CreatedAt: c.CreatedAt,
	})
	if err != nil || n == 0 {
		return postgres.Wrap(err, "insert supporter tip checkout")
	}
	return postgres.Wrap(q.UpdateSupporterOpenTipExpiry(ctx, supportersgen.UpdateSupporterOpenTipExpiryParams{
		ExpiresAt: c.ExpiresAt, TipID: c.TipID,
	}), "update supporter open tip")
}

// InsertTipPayment implements supporters.TipRepository.
func (r *SupporterRepository) InsertTipPayment(ctx context.Context, tx uow.Tx, p supporters.TipPayment) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertSupporterTipPayment(ctx, supportersgen.InsertSupporterTipPaymentParams{
		TipID: p.TipID, Provider: p.Provider, ProviderPaymentID: string(p.Ref), AmountMinor: p.AmountMinor, Currency: p.Currency,
		PaidAt: p.PaidAt,
	})
	if err != nil || n == 0 {
		return false, postgres.Wrap(err, "insert supporter tip payment")
	}
	return true, postgres.Wrap(q.DeleteSupporterOpenTip(ctx, p.TipID), "delete supporter open tip")
}

// InsertTipOutcome implements supporters.TipRepository.
func (r *SupporterRepository) InsertTipOutcome(ctx context.Context, tx uow.Tx, o supporters.TipOutcome) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertSupporterTipOutcome(ctx, supportersgen.InsertSupporterTipOutcomeParams{
		ID: o.ID, TipID: o.TipID, Kind: string(o.Kind), OccurredAt: o.OccurredAt,
	})
	if err != nil || n == 0 {
		return false, postgres.Wrap(err, "insert supporter tip outcome")
	}
	if o.Kind == supporters.TipExpired {
		if err := q.DeleteSupporterOpenTip(ctx, o.TipID); err != nil {
			return false, postgres.Wrap(err, "delete supporter open tip")
		}
	}
	return true, nil
}

// FindTip implements supporters.TipRepository.
func (r *SupporterRepository) FindTip(ctx context.Context, tx uow.Tx, id string) (supporters.TipRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.TipRecord{}, err
	}
	row, err := q.GetSupporterTip(ctx, id)
	if err != nil {
		return supporters.TipRecord{}, postgres.NotFound(err, supporters.ErrNotFound, "supporter tip "+id)
	}
	rec := supporters.TipRecord{Tip: supporters.Tip{
		ID: row.ID, UserID: row.UserID, AmountMinor: row.AmountMinor, Currency: row.Currency, Provider: row.Provider,
		CreatedAt: postgres.UTC(row.CreatedAt),
	}}
	switch c, err := q.GetSupporterTipCheckout(ctx, id); {
	case err == nil:
		rec.Checkout = &supporters.TipCheckout{
			TipID: id, Provider: c.Provider, Ref: supporters.CheckoutRef(c.ProviderCheckoutID),
			ExpiresAt: postgres.UTC(c.ExpiresAt), CreatedAt: postgres.UTC(c.CreatedAt),
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return supporters.TipRecord{}, postgres.Wrap(err, "get supporter tip checkout")
	}
	switch p, err := q.GetSupporterTipPayment(ctx, id); {
	case err == nil:
		rec.Payment = &supporters.TipPayment{
			TipID: id, Provider: p.Provider, Ref: supporters.PaymentRef(p.ProviderPaymentID), AmountMinor: p.AmountMinor,
			Currency: p.Currency, PaidAt: postgres.UTC(p.PaidAt),
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return supporters.TipRecord{}, postgres.Wrap(err, "get supporter tip payment")
	}
	kinds, err := q.ListSupporterTipOutcomes(ctx, id)
	if err != nil {
		return supporters.TipRecord{}, postgres.Wrap(err, "list supporter tip outcomes")
	}
	for _, k := range kinds {
		rec.Outcomes = append(rec.Outcomes, supporters.TipOutcomeKind(k))
	}
	return rec, nil
}

// FindTipIDByCheckout implements supporters.TipRepository.
func (r *SupporterRepository) FindTipIDByCheckout(ctx context.Context, tx uow.Tx, provider string, ref supporters.CheckoutRef) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.FindSupporterTipByCheckout(ctx, supportersgen.FindSupporterTipByCheckoutParams{Provider: provider, ProviderCheckoutID: string(ref)})
	return id, tipNotFound(err, "supporter tip of checkout "+string(ref))
}

// FindTipIDByPayment implements supporters.TipRepository.
func (r *SupporterRepository) FindTipIDByPayment(ctx context.Context, tx uow.Tx, provider string, ref supporters.PaymentRef) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.FindSupporterTipByPayment(ctx, supportersgen.FindSupporterTipByPaymentParams{Provider: provider, ProviderPaymentID: string(ref)})
	return id, tipNotFound(err, "supporter tip of payment "+string(ref))
}

// OpenTipID implements supporters.TipRepository.
func (r *SupporterRepository) OpenTipID(ctx context.Context, tx uow.Tx, userID string) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.FindSupporterOpenTip(ctx, userID)
	return id, tipNotFound(err, "open supporter tip")
}

// DueOpenTips implements supporters.TipRepository.
func (r *SupporterRepository) DueOpenTips(ctx context.Context, now time.Time, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "due supporter open tips limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListDueSupporterOpenTips(ctx, supportersgen.ListDueSupporterOpenTipsParams{ExpiresAt: now, Limit: lim})
	return ids, postgres.Wrap(err, "list due supporter open tips")
}

// OpenTipCheckouts implements supporters.TipRepository.
func (r *SupporterRepository) OpenTipCheckouts(ctx context.Context, provider string, now time.Time) ([]supporters.CheckoutRef, error) {
	rows, err := r.db.reader().ListSupporterOpenTipCheckouts(ctx, supportersgen.ListSupporterOpenTipCheckoutsParams{Provider: provider, ExpiresAt: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list supporter open tip checkouts")
	}
	out := make([]supporters.CheckoutRef, len(rows))
	for i, ref := range rows {
		out[i] = supporters.CheckoutRef(ref)
	}
	return out, nil
}

// TipUsers implements supporters.TipRepository.
func (r *SupporterRepository) TipUsers(ctx context.Context, ids []string) ([]supporters.TipUser, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.reader().ListSupporterTipUsers(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list supporter tip users")
	}
	out := make([]supporters.TipUser, len(rows))
	for i, row := range rows {
		out[i] = supporters.TipUser{TipID: row.ID, UserID: row.UserID}
	}
	return out, nil
}

// tipNotFound is postgres.NotFound of a tip, nil for nil.
func tipNotFound(err error, what string) error {
	if err == nil {
		return nil
	}
	return postgres.NotFound(err, supporters.ErrNotFound, what)
}

// eraseTips deletes the tips of an erased user with their records.
func eraseTips(ctx context.Context, q *supportersgen.Queries, userID string) error {
	for _, step := range []struct {
		what string
		run  func(context.Context, string) error
	}{
		{"delete supporter tip outcomes", q.DeleteSupporterTipOutcomesOfUser},
		{"delete supporter tip payments", q.DeleteSupporterTipPaymentsOfUser},
		{"delete supporter tip checkouts", q.DeleteSupporterTipCheckoutsOfUser},
		{"delete supporter open tip", q.DeleteSupporterOpenTipOfUser},
		{"delete supporter tips", q.DeleteSupporterTipsOfUser},
	} {
		if err := step.run(ctx, userID); err != nil {
			return postgres.Wrap(err, step.what)
		}
	}
	return nil
}
