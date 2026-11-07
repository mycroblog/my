// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/supportersgen"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/supporters"
)

// SupporterRepository implements supporters.Repository (data-model §4.13, ADR-0072 §15).
type SupporterRepository struct {
	db *moduleDB
}

var _ supporters.Repository = (*SupporterRepository)(nil)

// NewSupporterRepository returns the repository of the supporter program, its plans, and their
// subscriptions on this database.
func NewSupporterRepository(d *sqlite.DB) *SupporterRepository {
	return &SupporterRepository{db: &moduleDB{d}}
}

// q returns the queries in tx, or on the reader when tx is nil.
func (r *SupporterRepository) q(tx uow.Tx) (*supportersgen.Queries, error) {
	if tx == nil {
		return r.db.reader(), nil
	}
	return r.db.queries(tx)
}

func toNullMillisValue(t time.Time) sql.NullInt64 {
	if t.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: sqlite.ToMillis(t), Valid: true}
}

// InsertProgramChange implements supporters.Repository.
func (r *SupporterRepository) InsertProgramChange(ctx context.Context, tx uow.Tx, c supporters.ProgramChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	state := "disabled"
	if c.Enabled {
		state = "enabled"
	}
	return sqlite.Wrap(q.InsertSupporterProgramChange(ctx, supportersgen.InsertSupporterProgramChangeParams{
		ID: c.ID, State: state, Provider: c.Provider, ChangedByUserID: sqlite.ToNullString(c.ChangedByUserID), ChangedAt: sqlite.ToMillis(c.ChangedAt),
	}), "insert supporter program change")
}

// LastProgramChange implements supporters.Repository.
func (r *SupporterRepository) LastProgramChange(ctx context.Context, tx uow.Tx) (supporters.ProgramChange, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.ProgramChange{}, err
	}
	row, err := q.GetLastSupporterProgramChange(ctx)
	if err != nil {
		return supporters.ProgramChange{}, sqlite.NotFound(err, supporters.ErrNotFound, "supporter program change")
	}
	return supporters.ProgramChange{
		ID: row.ID, Enabled: row.State == "enabled", Provider: row.Provider, ChangedByUserID: row.ChangedByUserID.String,
		ChangedAt: sqlite.FromMillis(row.ChangedAt),
	}, nil
}

// InsertPlan implements supporters.Repository.
func (r *SupporterRepository) InsertPlan(ctx context.Context, tx uow.Tx, p domain.Plan) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSupporterPlan(ctx, supportersgen.InsertSupporterPlanParams{
		ID: p.ID, RoleID: p.RoleID, Currency: p.Currency, CreatedByUserID: sqlite.ToNullString(p.CreatedByUserID), CreatedAt: sqlite.ToMillis(p.CreatedAt),
	})
	if sqlite.IsUniqueViolation(err, "supporter_plans.role_id") {
		return fmt.Errorf("plan %s: %w", p.ID, supporters.ErrPlanRoleTaken)
	}
	return sqlite.Wrap(err, "insert supporter plan")
}

// InsertPlanRevision implements supporters.Repository.
func (r *SupporterRepository) InsertPlanRevision(ctx context.Context, tx uow.Tx, rev domain.PlanRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSupporterPlanRevision(ctx, supportersgen.InsertSupporterPlanRevisionParams{
		ID: rev.ID, PlanID: rev.PlanID, Name: rev.Name, Description: rev.Description, Rank: int64(rev.Rank),
		MonthlyAmountMinor: rev.MonthlyAmountMinor, AnnualDiscountPercent: int64(rev.AnnualDiscountPercent),
		EmojiLimit: int64(rev.EmojiLimit), Visibility: string(rev.Visibility), RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID),
		RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert supporter plan revision")
	}
	err = q.UpsertSupporterPlanHead(ctx, supportersgen.UpsertSupporterPlanHeadParams{
		PlanID: rev.PlanID, RevisionID: rev.ID, Rank: int64(rev.Rank), Visibility: string(rev.Visibility),
	})
	if sqlite.IsUniqueViolation(err, "supporter_plan_heads.rank") {
		return fmt.Errorf("plan %s rank %d: %w", rev.PlanID, rev.Rank, supporters.ErrRankTaken)
	}
	return sqlite.Wrap(err, "upsert supporter plan head")
}

// InsertPlanRetirement implements supporters.Repository.
func (r *SupporterRepository) InsertPlanRetirement(ctx context.Context, tx uow.Tx, planID, byUserID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSupporterPlanRetirement(ctx, supportersgen.InsertSupporterPlanRetirementParams{
		PlanID: planID, RetiredByUserID: sqlite.ToNullString(byUserID), RetiredAt: sqlite.ToMillis(at),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert supporter plan retirement")
	}
	return sqlite.Wrap(q.DeleteSupporterPlanHead(ctx, planID), "delete supporter plan head")
}

// plans reads every plan, or the one of planID.
func (r *SupporterRepository) plans(ctx context.Context, tx uow.Tx, planID string) ([]supporters.PlanRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSupporterPlans(ctx, planID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list supporter plans")
	}
	heads, err := q.ListSupporterPriceHeads(ctx, planID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list supporter price heads")
	}
	prices := map[string]map[domain.Interval]supporters.Price{}
	for _, h := range heads {
		if prices[h.PlanID] == nil {
			prices[h.PlanID] = map[domain.Interval]supporters.Price{}
		}
		prices[h.PlanID][domain.Interval(h.Interval)] = priceFrom(supportersgen.GetSupporterPriceRow{
			ID: h.ID, PlanID: h.PlanID, Interval: h.Interval, AmountMinor: h.AmountMinor, Currency: h.Currency,
			Provider: h.Provider, ProviderPriceID: h.ProviderPriceID, CreatedAt: h.CreatedAt, Archived: h.Archived,
		})
	}
	out := make([]supporters.PlanRecord, len(rows))
	for i, row := range rows {
		p := supporters.PlanRecord{
			Plan: domain.Plan{
				ID: row.ID, RoleID: row.RoleID, Currency: row.Currency, CreatedByUserID: row.CreatedByUserID.String,
				CreatedAt: sqlite.FromMillis(row.CreatedAt),
			},
			Revision: domain.PlanRevision{
				ID: row.RevisionID, PlanID: row.ID,
				PlanTerms: domain.PlanTerms{
					Name: row.Name, Description: row.Description, MonthlyAmountMinor: row.MonthlyAmountMinor,
					AnnualDiscountPercent: int(row.AnnualDiscountPercent), EmojiLimit: int(row.EmojiLimit), Rank: int(row.Rank),
					Visibility: domain.Visibility(row.Visibility),
				},
				RevisedByUserID: row.RevisedByUserID.String, RevisedAt: sqlite.FromMillis(row.RevisedAt),
			},
			Retired: row.Retired != 0,
			Prices:  prices[row.ID],
		}
		if p.Retired {
			p.RetiredAt = sqlite.FromMillis(row.RetiredAt)
		}
		if p.Prices == nil {
			p.Prices = map[domain.Interval]supporters.Price{}
		}
		out[i] = p
	}
	return out, nil
}

// ListPlans implements supporters.Repository.
func (r *SupporterRepository) ListPlans(ctx context.Context, tx uow.Tx) ([]supporters.PlanRecord, error) {
	return r.plans(ctx, tx, "")
}

// FindPlan implements supporters.Repository.
func (r *SupporterRepository) FindPlan(ctx context.Context, tx uow.Tx, id string) (supporters.PlanRecord, error) {
	if id == "" {
		return supporters.PlanRecord{}, fmt.Errorf("supporter plan: %w", supporters.ErrNotFound)
	}
	plans, err := r.plans(ctx, tx, id)
	if err != nil {
		return supporters.PlanRecord{}, err
	}
	if len(plans) == 0 {
		return supporters.PlanRecord{}, fmt.Errorf("supporter plan %s: %w", id, supporters.ErrNotFound)
	}
	return plans[0], nil
}

func priceFrom(row supportersgen.GetSupporterPriceRow) supporters.Price {
	return supporters.Price{
		ID: row.ID, PlanID: row.PlanID, Interval: domain.Interval(row.Interval), AmountMinor: row.AmountMinor,
		Currency: row.Currency, Provider: row.Provider, Ref: supporters.PriceRef(row.ProviderPriceID),
		CreatedAt: sqlite.FromMillis(row.CreatedAt), Archived: row.Archived != 0,
	}
}

// InsertPrice implements supporters.Repository.
func (r *SupporterRepository) InsertPrice(ctx context.Context, tx uow.Tx, p supporters.Price) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterPrice(ctx, supportersgen.InsertSupporterPriceParams{
		ID: p.ID, PlanID: p.PlanID, Interval: string(p.Interval), AmountMinor: p.AmountMinor, Currency: p.Currency,
		Provider: p.Provider, ProviderPriceID: string(p.Ref), CreatedAt: sqlite.ToMillis(p.CreatedAt),
	}), "insert supporter price")
}

// SetPriceHead implements supporters.Repository.
func (r *SupporterRepository) SetPriceHead(ctx context.Context, tx uow.Tx, planID string, interval domain.Interval, priceID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertSupporterPriceHead(ctx, supportersgen.UpsertSupporterPriceHeadParams{
		PlanID: planID, Interval: string(interval), PriceID: priceID,
	}), "upsert supporter price head")
}

// InsertPriceArchival implements supporters.Repository.
func (r *SupporterRepository) InsertPriceArchival(ctx context.Context, tx uow.Tx, priceID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterPriceArchival(ctx, supportersgen.InsertSupporterPriceArchivalParams{
		PriceID: priceID, ArchivedAt: sqlite.ToMillis(at),
	}), "insert supporter price archival")
}

// FindPriceByRef implements supporters.Repository.
func (r *SupporterRepository) FindPriceByRef(ctx context.Context, tx uow.Tx, provider string, ref supporters.PriceRef) (supporters.Price, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.Price{}, err
	}
	row, err := q.GetSupporterPriceByRef(ctx, supportersgen.GetSupporterPriceByRefParams{Provider: provider, ProviderPriceID: string(ref)})
	if err != nil {
		return supporters.Price{}, sqlite.NotFound(err, supporters.ErrNotFound, "supporter price "+string(ref))
	}
	return priceFrom(supportersgen.GetSupporterPriceRow(row)), nil
}

// FindPrice implements supporters.Repository.
func (r *SupporterRepository) FindPrice(ctx context.Context, tx uow.Tx, id string) (supporters.Price, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.Price{}, err
	}
	row, err := q.GetSupporterPrice(ctx, id)
	if err != nil {
		return supporters.Price{}, sqlite.NotFound(err, supporters.ErrNotFound, "supporter price "+id)
	}
	return priceFrom(row), nil
}

// InsertCustomer implements supporters.Repository.
func (r *SupporterRepository) InsertCustomer(ctx context.Context, tx uow.Tx, c supporters.Customer) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterCustomer(ctx, supportersgen.InsertSupporterCustomerParams{
		ID: c.ID, UserID: c.UserID, Provider: c.Provider, ProviderCustomerID: string(c.Ref), CreatedAt: sqlite.ToMillis(c.CreatedAt),
	}), "insert supporter customer")
}

// FindCustomer implements supporters.Repository.
func (r *SupporterRepository) FindCustomer(ctx context.Context, tx uow.Tx, provider, userID string) (supporters.Customer, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.Customer{}, err
	}
	row, err := q.GetSupporterCustomer(ctx, supportersgen.GetSupporterCustomerParams{Provider: provider, UserID: userID})
	if err != nil {
		return supporters.Customer{}, sqlite.NotFound(err, supporters.ErrNotFound, "supporter customer of "+userID)
	}
	return supporters.Customer{
		ID: row.ID, UserID: row.UserID, Provider: row.Provider, Ref: supporters.CustomerRef(row.ProviderCustomerID),
		CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}, nil
}

// insertTransition writes a transition with its grace and scheduled change and makes it the
// current one of its subscription.
func insertTransition(ctx context.Context, q *supportersgen.Queries, t supporters.TransitionRecord, userID string) error {
	err := q.InsertSupporterTransition(ctx, supportersgen.InsertSupporterTransitionParams{
		ID: t.ID, SubscriptionID: t.SubscriptionID, State: string(t.State), PlanID: t.PlanID, PriceID: t.PriceID,
		PeriodEndsAt: toNullMillisValue(t.PeriodEndsAt), Reason: string(t.Reason), OccurredAt: sqlite.ToMillis(t.OccurredAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert supporter transition")
	}
	if g := t.Grace; g != nil {
		err := q.InsertSupporterGracePeriod(ctx, supportersgen.InsertSupporterGracePeriodParams{
			TransitionID: t.ID, FailedAt: sqlite.ToMillis(g.FailedAt), EndsAt: sqlite.ToMillis(g.EndsAt),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert supporter grace period")
		}
	}
	if s := t.Scheduled; s != nil {
		err := q.InsertSupporterScheduledChange(ctx, supportersgen.InsertSupporterScheduledChangeParams{
			TransitionID: t.ID, PriceID: s.PriceID, EffectiveAt: sqlite.ToMillis(s.EffectiveAt),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert supporter scheduled change")
		}
	}
	return sqlite.Wrap(q.UpsertSupporterSubscriptionHead(ctx, supportersgen.UpsertSupporterSubscriptionHeadParams{
		SubscriptionID: t.SubscriptionID, TransitionID: t.ID, UserID: userID, PlanID: t.PlanID, State: string(t.State),
	}), "upsert supporter subscription head")
}

// setDeadline writes the deadline of a subscription, or removes it when d is nil.
func setDeadline(ctx context.Context, q *supportersgen.Queries, subscriptionID string, d *supporters.Deadline) error {
	if d == nil {
		return sqlite.Wrap(q.DeleteSupporterDeadline(ctx, subscriptionID), "delete supporter deadline")
	}
	return sqlite.Wrap(q.UpsertSupporterDeadline(ctx, supportersgen.UpsertSupporterDeadlineParams{
		SubscriptionID: subscriptionID, Kind: string(d.Kind), DueAt: sqlite.ToMillis(d.DueAt),
	}), "upsert supporter deadline")
}

// InsertSubscription implements supporters.Repository.
func (r *SupporterRepository) InsertSubscription(ctx context.Context, tx uow.Tx, s supporters.Subscription, first supporters.TransitionRecord, d *supporters.Deadline) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSupporterLiveSubscription(ctx, supportersgen.InsertSupporterLiveSubscriptionParams{UserID: s.UserID, SubscriptionID: s.ID})
	if sqlite.IsUniqueViolation(err, "supporter_live_subscriptions.user_id") {
		return fmt.Errorf("user %s: %w", s.UserID, supporters.ErrLiveSubscription)
	}
	if err != nil {
		return sqlite.Wrap(err, "insert supporter live subscription")
	}
	err = q.InsertSupporterSubscription(ctx, supportersgen.InsertSupporterSubscriptionParams{
		ID: s.ID, UserID: s.UserID, PlanID: s.PlanID, PriceID: s.PriceID, Provider: s.Provider, CreatedAt: sqlite.ToMillis(s.CreatedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert supporter subscription")
	}
	if err := insertTransition(ctx, q, first, s.UserID); err != nil {
		return err
	}
	if !first.State.Live() {
		if err := q.DeleteSupporterLiveSubscription(ctx, s.ID); err != nil {
			return sqlite.Wrap(err, "delete supporter live subscription")
		}
	}
	return setDeadline(ctx, q, s.ID, d)
}

// InsertCheckout implements supporters.Repository.
func (r *SupporterRepository) InsertCheckout(ctx context.Context, tx uow.Tx, c supporters.CheckoutRecord) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterCheckout(ctx, supportersgen.InsertSupporterCheckoutParams{
		ID: c.ID, SubscriptionID: c.SubscriptionID, Provider: c.Provider, ProviderCheckoutID: string(c.Ref),
		ExpiresAt: sqlite.ToMillis(c.ExpiresAt), CreatedAt: sqlite.ToMillis(c.CreatedAt),
	}), "insert supporter checkout")
}

// LiveCheckouts implements supporters.Repository.
func (r *SupporterRepository) LiveCheckouts(ctx context.Context, tx uow.Tx, subscriptionID string, now time.Time) ([]supporters.CheckoutRef, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLiveSupporterCheckouts(ctx, supportersgen.ListLiveSupporterCheckoutsParams{SubscriptionID: subscriptionID, Now: sqlite.ToMillis(now)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list live supporter checkouts")
	}
	out := make([]supporters.CheckoutRef, len(rows))
	for i, ref := range rows {
		out[i] = supporters.CheckoutRef(ref)
	}
	return out, nil
}

// InsertDuplicate implements supporters.Repository.
func (r *SupporterRepository) InsertDuplicate(ctx context.Context, tx uow.Tx, d supporters.Duplicate) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertSupporterDuplicate(ctx, supportersgen.InsertSupporterDuplicateParams{
		ID: d.ID, SubscriptionID: d.SubscriptionID, Provider: d.Provider, ProviderSubscriptionID: string(d.Ref), DetectedAt: sqlite.ToMillis(d.DetectedAt),
	})
	if err != nil {
		return false, sqlite.Wrap(err, "insert supporter duplicate")
	}
	return n > 0, nil
}

// CancelDuplicate implements supporters.Repository.
func (r *SupporterRepository) CancelDuplicate(ctx context.Context, tx uow.Tx, duplicateID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterDuplicateCancellation(ctx, supportersgen.InsertSupporterDuplicateCancellationParams{DuplicateID: duplicateID, DecidedAt: sqlite.ToMillis(at)}), "record the cancellation of a supporter duplicate")
}

// SettleDuplicate implements supporters.Repository.
func (r *SupporterRepository) SettleDuplicate(ctx context.Context, tx uow.Tx, duplicateID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterDuplicateSettlement(ctx, supportersgen.InsertSupporterDuplicateSettlementParams{DuplicateID: duplicateID, SettledAt: sqlite.ToMillis(at)}), "settle supporter duplicate")
}

// FindDuplicate implements supporters.Repository.
func (r *SupporterRepository) FindDuplicate(ctx context.Context, tx uow.Tx, provider string, ref supporters.SubscriptionRef) (supporters.Duplicate, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.Duplicate{}, err
	}
	row, err := q.GetSupporterDuplicate(ctx, supportersgen.GetSupporterDuplicateParams{Provider: provider, ProviderSubscriptionID: string(ref)})
	if err != nil {
		return supporters.Duplicate{}, sqlite.NotFound(err, supporters.ErrNotFound, "supporter duplicate "+string(ref))
	}
	return supporters.Duplicate{
		ID: row.ID, SubscriptionID: row.SubscriptionID, Provider: row.Provider, Ref: supporters.SubscriptionRef(row.ProviderSubscriptionID),
		DetectedAt: sqlite.FromMillis(row.DetectedAt), Canceling: row.Canceling, Settled: row.Settled,
	}, nil
}

// UnsettledDuplicates implements supporters.Repository.
func (r *SupporterRepository) UnsettledDuplicates(ctx context.Context, provider string) ([]supporters.Duplicate, error) {
	rows, err := r.db.reader().ListUnsettledSupporterDuplicates(ctx, provider)
	if err != nil {
		return nil, sqlite.Wrap(err, "list unsettled supporter duplicates")
	}
	out := make([]supporters.Duplicate, len(rows))
	for i, row := range rows {
		out[i] = supporters.Duplicate{
			ID: row.ID, SubscriptionID: row.SubscriptionID, Provider: row.Provider, Ref: supporters.SubscriptionRef(row.ProviderSubscriptionID),
			DetectedAt: sqlite.FromMillis(row.DetectedAt),
		}
	}
	return out, nil
}

// subscription reads a subscription with its current transition, link, and latest checkout.
func subscription(ctx context.Context, q *supportersgen.Queries, id string) (supporters.SubscriptionRecord, error) {
	row, err := q.GetSupporterSubscription(ctx, id)
	if err != nil {
		return supporters.SubscriptionRecord{}, sqlite.NotFound(err, supporters.ErrNotFound, "supporter subscription "+id)
	}
	rec := supporters.SubscriptionRecord{
		Subscription: supporters.Subscription{
			ID: row.ID, UserID: row.UserID, PlanID: row.PlanID, PriceID: row.PriceID, Provider: row.Provider,
			CreatedAt: sqlite.FromMillis(row.CreatedAt),
		},
		Current: supporters.TransitionRecord{
			ID: row.TransitionID, SubscriptionID: row.ID,
			Transition: domain.Transition{
				State: domain.State(row.State), PlanID: row.TransitionPlanID, PriceID: row.TransitionPriceID,
			},
			Reason: domain.Reason(row.Reason), OccurredAt: sqlite.FromMillis(row.OccurredAt),
		},
	}
	if row.PeriodEndsAt.Valid {
		rec.Current.PeriodEndsAt = sqlite.FromMillis(row.PeriodEndsAt.Int64)
	}
	switch g, err := q.GetSupporterGracePeriod(ctx, row.TransitionID); {
	case err == nil:
		rec.Current.Grace = &domain.Grace{FailedAt: sqlite.FromMillis(g.FailedAt), EndsAt: sqlite.FromMillis(g.EndsAt)}
	case !errors.Is(err, sql.ErrNoRows):
		return supporters.SubscriptionRecord{}, sqlite.Wrap(err, "get supporter grace period")
	}
	switch s, err := q.GetSupporterScheduledChange(ctx, row.TransitionID); {
	case err == nil:
		rec.Current.Scheduled = &domain.ScheduledChange{PriceID: s.PriceID, EffectiveAt: sqlite.FromMillis(s.EffectiveAt)}
	case !errors.Is(err, sql.ErrNoRows):
		return supporters.SubscriptionRecord{}, sqlite.Wrap(err, "get supporter scheduled change")
	}
	switch link, err := q.GetSupporterLink(ctx, row.ID); {
	case err == nil:
		rec.Link = supporters.SubscriptionRef(link)
	case !errors.Is(err, sql.ErrNoRows):
		return supporters.SubscriptionRecord{}, sqlite.Wrap(err, "get supporter link")
	}
	switch c, err := q.GetLatestSupporterCheckout(ctx, row.ID); {
	case err == nil:
		rec.Checkout = &supporters.CheckoutRecord{
			ID: c.ID, SubscriptionID: c.SubscriptionID, Provider: c.Provider, Ref: supporters.CheckoutRef(c.ProviderCheckoutID),
			ExpiresAt: sqlite.FromMillis(c.ExpiresAt), CreatedAt: sqlite.FromMillis(c.CreatedAt),
		}
	case !errors.Is(err, sql.ErrNoRows):
		return supporters.SubscriptionRecord{}, sqlite.Wrap(err, "get supporter checkout")
	}
	return rec, nil
}

// FindSubscription implements supporters.Repository.
func (r *SupporterRepository) FindSubscription(ctx context.Context, tx uow.Tx, id string) (supporters.SubscriptionRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.SubscriptionRecord{}, err
	}
	return subscription(ctx, q, id)
}

// LockSubscription implements supporters.Repository. SQLite serializes write transactions, so
// it needs no lock.
func (r *SupporterRepository) LockSubscription(ctx context.Context, tx uow.Tx, id string) (supporters.SubscriptionRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return supporters.SubscriptionRecord{}, err
	}
	return subscription(ctx, q, id)
}

// MarkSubscriptionRead implements supporters.Repository.
func (r *SupporterRepository) MarkSubscriptionRead(ctx context.Context, tx uow.Tx, id string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.MarkSupporterSubscriptionRead(ctx, supportersgen.MarkSupporterSubscriptionReadParams{SubscriptionID: id, ReadAt: sqlite.ToMillis(at)})
	return n > 0, sqlite.Wrap(err, "mark the read of a subscription")
}

// FindSubscriptionIDByRef implements supporters.Repository.
func (r *SupporterRepository) FindSubscriptionIDByRef(ctx context.Context, tx uow.Tx, provider string, ref supporters.SubscriptionRef) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetSupporterSubscriptionIDByRef(ctx, supportersgen.GetSupporterSubscriptionIDByRefParams{
		Provider: provider, ProviderSubscriptionID: string(ref),
	})
	if err != nil {
		return "", sqlite.NotFound(err, supporters.ErrNotFound, "supporter subscription of "+string(ref))
	}
	return id, nil
}

// FindSubscriptionIDByCheckout implements supporters.Repository.
func (r *SupporterRepository) FindSubscriptionIDByCheckout(ctx context.Context, tx uow.Tx, provider string, ref supporters.CheckoutRef) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetSupporterSubscriptionIDByCheckout(ctx, supportersgen.GetSupporterSubscriptionIDByCheckoutParams{
		Provider: provider, ProviderCheckoutID: string(ref),
	})
	if err != nil {
		return "", sqlite.NotFound(err, supporters.ErrNotFound, "supporter subscription of checkout "+string(ref))
	}
	return id, nil
}

// LiveSubscriptionID implements supporters.Repository.
func (r *SupporterRepository) LiveSubscriptionID(ctx context.Context, tx uow.Tx, userID string) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetSupporterLiveSubscriptionID(ctx, userID)
	if err != nil {
		return "", sqlite.NotFound(err, supporters.ErrNotFound, "live supporter subscription of "+userID)
	}
	return id, nil
}

// InsertLink implements supporters.Repository.
func (r *SupporterRepository) InsertLink(ctx context.Context, tx uow.Tx, subscriptionID, provider string, ref supporters.SubscriptionRef, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterLink(ctx, supportersgen.InsertSupporterLinkParams{
		SubscriptionID: subscriptionID, Provider: provider, ProviderSubscriptionID: string(ref), LinkedAt: sqlite.ToMillis(at),
	}), "insert supporter link")
}

// InsertTransition implements supporters.Repository.
func (r *SupporterRepository) InsertTransition(ctx context.Context, tx uow.Tx, t supporters.TransitionRecord, userID string, d *supporters.Deadline) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := insertTransition(ctx, q, t, userID); err != nil {
		return err
	}
	if t.State.Live() {
		err = q.EnsureSupporterLiveSubscription(ctx, supportersgen.EnsureSupporterLiveSubscriptionParams{UserID: userID, SubscriptionID: t.SubscriptionID})
	} else {
		err = q.DeleteSupporterLiveSubscription(ctx, t.SubscriptionID)
	}
	if err != nil {
		return sqlite.Wrap(err, "update supporter live subscription")
	}
	return setDeadline(ctx, q, t.SubscriptionID, d)
}

// InsertReversal implements supporters.Repository.
func (r *SupporterRepository) InsertReversal(ctx context.Context, tx uow.Tx, rev supporters.Reversal) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertSupporterReversal(ctx, supportersgen.InsertSupporterReversalParams{
		ID: rev.ID, SubscriptionID: rev.SubscriptionID, Kind: string(rev.Kind), Provider: rev.Provider,
		ProviderReversalID: rev.ProviderID, OccurredAt: sqlite.ToMillis(rev.OccurredAt),
	})
	return n > 0, sqlite.Wrap(err, "insert supporter reversal")
}

// HasRevokingReversal implements supporters.Repository.
func (r *SupporterRepository) HasRevokingReversal(ctx context.Context, tx uow.Tx, subscriptionID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	found, err := q.HasRevokingSupporterReversal(ctx, subscriptionID)
	return found != 0, sqlite.Wrap(err, "find supporter reversals")
}

// KnownReversals implements supporters.Repository.
func (r *SupporterRepository) KnownReversals(ctx context.Context, provider string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListKnownSupporterReversals(ctx, supportersgen.ListKnownSupporterReversalsParams{Provider: provider, Ids: list})
	return out, sqlite.Wrap(err, "list known supporter reversals")
}

// OpenCheckouts implements supporters.Repository.
func (r *SupporterRepository) OpenCheckouts(ctx context.Context, provider string, now time.Time) ([]supporters.CheckoutRef, error) {
	rows, err := r.db.reader().ListOpenSupporterCheckouts(ctx, supportersgen.ListOpenSupporterCheckoutsParams{Provider: provider, Now: sqlite.ToMillis(now)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list open supporter checkouts")
	}
	out := make([]supporters.CheckoutRef, len(rows))
	for i, ref := range rows {
		out[i] = supporters.CheckoutRef(ref)
	}
	return out, nil
}

// DueDeadlines implements supporters.Repository.
func (r *SupporterRepository) DueDeadlines(ctx context.Context, now time.Time, limit int) ([]supporters.Deadline, error) {
	rows, err := r.db.reader().ListDueSupporterDeadlines(ctx, supportersgen.ListDueSupporterDeadlinesParams{Now: sqlite.ToMillis(now), Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list due supporter deadlines")
	}
	out := make([]supporters.Deadline, len(rows))
	for i, row := range rows {
		out[i] = supporters.Deadline{SubscriptionID: row.SubscriptionID, Kind: domain.Deadline(row.Kind), DueAt: sqlite.FromMillis(row.DueAt)}
	}
	return out, nil
}

// ListSubscriptions implements supporters.Repository.
func (r *SupporterRepository) ListSubscriptions(ctx context.Context, f supporters.SubscriptionFilter) ([]supporters.SubscriptionRecord, error) {
	q := r.db.reader()
	ids, err := q.ListSupporterSubscriptionIDs(ctx, supportersgen.ListSupporterSubscriptionIDsParams{
		PlanID: f.PlanID, State: string(f.State), Before: f.Before, Lim: int64(f.Limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list supporter subscriptions")
	}
	out := make([]supporters.SubscriptionRecord, 0, len(ids))
	for _, id := range ids {
		rec, err := subscription(ctx, q, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// LiveSubscriptionIDsOfPlan implements supporters.Repository.
func (r *SupporterRepository) LiveSubscriptionIDsOfPlan(ctx context.Context, tx uow.Tx, planID string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListLiveSupporterSubscriptionIDsOfPlan(ctx, planID)
	return ids, sqlite.Wrap(err, "list live supporter subscriptions of a plan")
}

// LiveSubscriptionIDs implements supporters.Repository.
func (r *SupporterRepository) LiveSubscriptionIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListLiveSupporterSubscriptionIDs(ctx)
	return ids, sqlite.Wrap(err, "list live supporter subscriptions")
}

// CountLiveSubscriptions implements supporters.Repository.
func (r *SupporterRepository) CountLiveSubscriptions(ctx context.Context, tx uow.Tx) (int64, error) {
	q, err := r.q(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountSupporterLiveSubscriptions(ctx)
	return n, sqlite.Wrap(err, "count live supporter subscriptions")
}

// CountMembers implements supporters.Repository.
func (r *SupporterRepository) CountMembers(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.reader().CountSupporterMembers(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "count supporter members")
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.PlanID] = row.Members
	}
	return out, nil
}

// SubscriptionUsers implements supporters.Repository.
func (r *SupporterRepository) SubscriptionUsers(ctx context.Context, ids []string) ([]supporters.SubscriptionUser, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListSupporterSubscriptionUsers(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list supporter subscription users")
	}
	out := make([]supporters.SubscriptionUser, len(rows))
	for i, row := range rows {
		out[i] = supporters.SubscriptionUser{SubscriptionID: row.ID, UserID: row.UserID, PlanID: row.PlanID}
	}
	return out, nil
}

// InsertListing implements supporters.Repository.
func (r *SupporterRepository) InsertListing(ctx context.Context, tx uow.Tx, userID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSupporterListing(ctx, supportersgen.InsertSupporterListingParams{UserID: userID, CreatedAt: sqlite.ToMillis(at)}), "insert supporter listing")
}

// HasListing implements supporters.Repository.
func (r *SupporterRepository) HasListing(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return false, err
	}
	found, err := q.HasSupporterListing(ctx, userID)
	return found != 0, sqlite.Wrap(err, "find supporter listing")
}

// ListListings implements supporters.Repository.
func (r *SupporterRepository) ListListings(ctx context.Context, f supporters.ListingFilter) ([]supporters.Listing, error) {
	params := supportersgen.ListSupporterListingsParams{PlanID: f.PlanID, Lim: int64(f.Limit)}
	if f.After != nil {
		params.AfterUserID, params.AfterRank, params.AfterAt = f.After.UserID, int64(f.After.Rank), sqlite.ToMillis(f.After.ListedAt)
	}
	rows, err := r.db.reader().ListSupporterListings(ctx, params)
	if err != nil {
		return nil, sqlite.Wrap(err, "list supporter listings")
	}
	out := make([]supporters.Listing, len(rows))
	for i, row := range rows {
		out[i] = supporters.Listing{UserID: row.UserID, PlanID: row.PlanID, Rank: int(row.Rank), ListedAt: sqlite.FromMillis(row.CreatedAt)}
	}
	return out, nil
}

// DeleteListing implements supporters.Repository.
func (r *SupporterRepository) DeleteListing(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteSupporterListing(ctx, userID), "delete supporter listing")
}

// EraseUser implements supporters.Repository.
func (r *SupporterRepository) EraseUser(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteSupporterListing(ctx, userID); err != nil {
		return sqlite.Wrap(err, "delete supporter listing")
	}
	return sqlite.Wrap(q.DeleteSupporterCustomersOfUser(ctx, userID), "delete supporter customers")
}

// RebuildProjections implements supporters.Repository.
func (r *SupporterRepository) RebuildProjections(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range []struct {
		what string
		run  func(context.Context) error
	}{
		{"delete supporter plan heads", q.DeleteAllSupporterPlanHeads},
		{"rebuild supporter plan heads", q.RebuildSupporterPlanHeads},
		{"delete supporter price heads", q.DeleteAllSupporterPriceHeads},
		{"rebuild supporter price heads", q.RebuildSupporterPriceHeads},
		{"delete supporter subscription heads", q.DeleteAllSupporterSubscriptionHeads},
		{"rebuild supporter subscription heads", q.RebuildSupporterSubscriptionHeads},
		{"delete supporter live subscriptions", q.DeleteAllSupporterLiveSubscriptions},
		{"rebuild supporter live subscriptions", q.RebuildSupporterLiveSubscriptions},
		{"delete supporter deadlines", q.DeleteAllSupporterDeadlines},
		{"rebuild supporter deadlines", q.RebuildSupporterDeadlines},
	} {
		if err := step.run(ctx); err != nil {
			return sqlite.Wrap(err, step.what)
		}
	}
	return nil
}
