// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/supporters"
)

// SupporterRepository implements supporters.Repository (data-model §4.13, ADR-0072 §15).
type SupporterRepository struct {
	db *DB
}

var _ supporters.Repository = (*SupporterRepository)(nil)

// SupporterRepository returns the repository of the supporter program, its plans, and their
// subscriptions on this database.
func (d *DB) SupporterRepository() *SupporterRepository { return &SupporterRepository{db: d} }

// q returns the queries in tx, or on the reader when tx is nil.
func (r *SupporterRepository) q(tx uow.Tx) (*postgresgen.Queries, error) {
	if tx == nil {
		return r.db.reader(), nil
	}
	return r.db.queries(tx)
}

func toNullTimeValue(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
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
	return wrap(q.InsertSupporterProgramChange(ctx, postgresgen.InsertSupporterProgramChangeParams{
		ID: c.ID, State: state, Provider: c.Provider, ChangedByUserID: toNullString(c.ChangedByUserID), ChangedAt: c.ChangedAt,
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
		return supporters.ProgramChange{}, notFound(err, supporters.ErrNotFound, "supporter program change")
	}
	return supporters.ProgramChange{
		ID: row.ID, Enabled: row.State == "enabled", Provider: row.Provider, ChangedByUserID: fromNullString(row.ChangedByUserID),
		ChangedAt: utc(row.ChangedAt),
	}, nil
}

// InsertPlan implements supporters.Repository.
func (r *SupporterRepository) InsertPlan(ctx context.Context, tx uow.Tx, p domain.Plan) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSupporterPlan(ctx, postgresgen.InsertSupporterPlanParams{
		ID: p.ID, RoleID: p.RoleID, Currency: p.Currency, CreatedByUserID: toNullString(p.CreatedByUserID), CreatedAt: p.CreatedAt,
	})
	if isUniqueViolation(err, "supporter_plans_role_id") {
		return fmt.Errorf("plan %s: %w", p.ID, supporters.ErrPlanRoleTaken)
	}
	return wrap(err, "insert supporter plan")
}

// InsertPlanRevision implements supporters.Repository.
func (r *SupporterRepository) InsertPlanRevision(ctx context.Context, tx uow.Tx, rev domain.PlanRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	rank, err := toInt32(rev.Rank, "rank")
	if err != nil {
		return err
	}
	discount, err := toInt32(rev.AnnualDiscountPercent, "annual discount")
	if err != nil {
		return err
	}
	emojis, err := toInt32(rev.EmojiLimit, "emoji limit")
	if err != nil {
		return err
	}
	err = q.InsertSupporterPlanRevision(ctx, postgresgen.InsertSupporterPlanRevisionParams{
		ID: rev.ID, PlanID: rev.PlanID, Name: rev.Name, Description: rev.Description, Rank: rank,
		MonthlyAmountMinor: rev.MonthlyAmountMinor, AnnualDiscountPercent: discount,
		EmojiLimit: emojis, Visibility: string(rev.Visibility), RevisedByUserID: toNullString(rev.RevisedByUserID),
		RevisedAt: rev.RevisedAt,
	})
	if err != nil {
		return wrap(err, "insert supporter plan revision")
	}
	err = q.UpsertSupporterPlanHead(ctx, postgresgen.UpsertSupporterPlanHeadParams{
		PlanID: rev.PlanID, RevisionID: rev.ID, Rank: rank, Visibility: string(rev.Visibility),
	})
	if isUniqueViolation(err, "supporter_plan_heads_rank") {
		return fmt.Errorf("plan %s rank %d: %w", rev.PlanID, rev.Rank, supporters.ErrRankTaken)
	}
	return wrap(err, "upsert supporter plan head")
}

// InsertPlanRetirement implements supporters.Repository.
func (r *SupporterRepository) InsertPlanRetirement(ctx context.Context, tx uow.Tx, planID, byUserID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSupporterPlanRetirement(ctx, postgresgen.InsertSupporterPlanRetirementParams{
		PlanID: planID, RetiredByUserID: toNullString(byUserID), RetiredAt: at,
	})
	if err != nil {
		return wrap(err, "insert supporter plan retirement")
	}
	return wrap(q.DeleteSupporterPlanHead(ctx, planID), "delete supporter plan head")
}

// plans reads every plan, or the one of planID.
func (r *SupporterRepository) plans(ctx context.Context, tx uow.Tx, planID string) ([]supporters.PlanRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSupporterPlans(ctx, toNullString(planID))
	if err != nil {
		return nil, wrap(err, "list supporter plans")
	}
	heads, err := q.ListSupporterPriceHeads(ctx, toNullString(planID))
	if err != nil {
		return nil, wrap(err, "list supporter price heads")
	}
	prices := map[string]map[domain.Interval]supporters.Price{}
	for _, h := range heads {
		if prices[h.PlanID] == nil {
			prices[h.PlanID] = map[domain.Interval]supporters.Price{}
		}
		prices[h.PlanID][domain.Interval(h.Interval)] = priceFrom(postgresgen.GetSupporterPriceRow{
			ID: h.ID, PlanID: h.PlanID, Interval: h.Interval, AmountMinor: h.AmountMinor, Currency: h.Currency,
			Provider: h.Provider, ProviderPriceID: h.ProviderPriceID, CreatedAt: h.CreatedAt, Archived: h.Archived,
		})
	}
	out := make([]supporters.PlanRecord, len(rows))
	for i, row := range rows {
		p := supporters.PlanRecord{
			Plan: domain.Plan{
				ID: row.ID, RoleID: row.RoleID, Currency: row.Currency, CreatedByUserID: fromNullString(row.CreatedByUserID),
				CreatedAt: utc(row.CreatedAt),
			},
			Revision: domain.PlanRevision{
				ID: row.RevisionID, PlanID: row.ID,
				PlanTerms: domain.PlanTerms{
					Name: row.Name, Description: row.Description, MonthlyAmountMinor: row.MonthlyAmountMinor,
					AnnualDiscountPercent: int(row.AnnualDiscountPercent), EmojiLimit: int(row.EmojiLimit), Rank: int(row.Rank),
					Visibility: domain.Visibility(row.Visibility),
				},
				RevisedByUserID: fromNullString(row.RevisedByUserID), RevisedAt: utc(row.RevisedAt),
			},
			Retired: row.Retired,
			Prices:  prices[row.ID],
		}
		if p.Retired && row.RetiredAt != nil {
			p.RetiredAt = utc(*row.RetiredAt)
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

func priceFrom(row postgresgen.GetSupporterPriceRow) supporters.Price {
	return supporters.Price{
		ID: row.ID, PlanID: row.PlanID, Interval: domain.Interval(row.Interval), AmountMinor: row.AmountMinor,
		Currency: row.Currency, Provider: row.Provider, Ref: supporters.PriceRef(row.ProviderPriceID),
		CreatedAt: utc(row.CreatedAt), Archived: row.Archived,
	}
}

// InsertPrice implements supporters.Repository.
func (r *SupporterRepository) InsertPrice(ctx context.Context, tx uow.Tx, p supporters.Price) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertSupporterPrice(ctx, postgresgen.InsertSupporterPriceParams{
		ID: p.ID, PlanID: p.PlanID, Interval: string(p.Interval), AmountMinor: p.AmountMinor, Currency: p.Currency,
		Provider: p.Provider, ProviderPriceID: string(p.Ref), CreatedAt: p.CreatedAt,
	}), "insert supporter price")
}

// SetPriceHead implements supporters.Repository.
func (r *SupporterRepository) SetPriceHead(ctx context.Context, tx uow.Tx, planID string, interval domain.Interval, priceID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertSupporterPriceHead(ctx, postgresgen.UpsertSupporterPriceHeadParams{
		PlanID: planID, Interval: string(interval), PriceID: priceID,
	}), "upsert supporter price head")
}

// InsertPriceArchival implements supporters.Repository.
func (r *SupporterRepository) InsertPriceArchival(ctx context.Context, tx uow.Tx, priceID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertSupporterPriceArchival(ctx, postgresgen.InsertSupporterPriceArchivalParams{
		PriceID: priceID, ArchivedAt: at,
	}), "insert supporter price archival")
}

// FindPriceByRef implements supporters.Repository.
func (r *SupporterRepository) FindPriceByRef(ctx context.Context, tx uow.Tx, provider string, ref supporters.PriceRef) (supporters.Price, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.Price{}, err
	}
	row, err := q.GetSupporterPriceByRef(ctx, postgresgen.GetSupporterPriceByRefParams{Provider: provider, ProviderPriceID: string(ref)})
	if err != nil {
		return supporters.Price{}, notFound(err, supporters.ErrNotFound, "supporter price "+string(ref))
	}
	return priceFrom(postgresgen.GetSupporterPriceRow(row)), nil
}

// FindPrice implements supporters.Repository.
func (r *SupporterRepository) FindPrice(ctx context.Context, tx uow.Tx, id string) (supporters.Price, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.Price{}, err
	}
	row, err := q.GetSupporterPrice(ctx, id)
	if err != nil {
		return supporters.Price{}, notFound(err, supporters.ErrNotFound, "supporter price "+id)
	}
	return priceFrom(row), nil
}

// InsertCustomer implements supporters.Repository.
func (r *SupporterRepository) InsertCustomer(ctx context.Context, tx uow.Tx, c supporters.Customer) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertSupporterCustomer(ctx, postgresgen.InsertSupporterCustomerParams{
		ID: c.ID, UserID: c.UserID, Provider: c.Provider, ProviderCustomerID: string(c.Ref), CreatedAt: c.CreatedAt,
	}), "insert supporter customer")
}

// FindCustomer implements supporters.Repository.
func (r *SupporterRepository) FindCustomer(ctx context.Context, tx uow.Tx, provider, userID string) (supporters.Customer, error) {
	q, err := r.q(tx)
	if err != nil {
		return supporters.Customer{}, err
	}
	row, err := q.GetSupporterCustomer(ctx, postgresgen.GetSupporterCustomerParams{Provider: provider, UserID: userID})
	if err != nil {
		return supporters.Customer{}, notFound(err, supporters.ErrNotFound, "supporter customer of "+userID)
	}
	return supporters.Customer{
		ID: row.ID, UserID: row.UserID, Provider: row.Provider, Ref: supporters.CustomerRef(row.ProviderCustomerID),
		CreatedAt: utc(row.CreatedAt),
	}, nil
}

// insertTransition writes a transition with its grace and scheduled change and makes it the
// current one of its subscription.
func insertTransition(ctx context.Context, q *postgresgen.Queries, t supporters.TransitionRecord, userID string) error {
	err := q.InsertSupporterTransition(ctx, postgresgen.InsertSupporterTransitionParams{
		ID: t.ID, SubscriptionID: t.SubscriptionID, State: string(t.State), PlanID: t.PlanID, PriceID: t.PriceID,
		PeriodEndsAt: toNullTimeValue(t.PeriodEndsAt), Reason: string(t.Reason), OccurredAt: t.OccurredAt,
	})
	if err != nil {
		return wrap(err, "insert supporter transition")
	}
	if g := t.Grace; g != nil {
		err := q.InsertSupporterGracePeriod(ctx, postgresgen.InsertSupporterGracePeriodParams{
			TransitionID: t.ID, FailedAt: g.FailedAt, EndsAt: g.EndsAt,
		})
		if err != nil {
			return wrap(err, "insert supporter grace period")
		}
	}
	if s := t.Scheduled; s != nil {
		err := q.InsertSupporterScheduledChange(ctx, postgresgen.InsertSupporterScheduledChangeParams{
			TransitionID: t.ID, PriceID: s.PriceID, EffectiveAt: s.EffectiveAt,
		})
		if err != nil {
			return wrap(err, "insert supporter scheduled change")
		}
	}
	return wrap(q.UpsertSupporterSubscriptionHead(ctx, postgresgen.UpsertSupporterSubscriptionHeadParams{
		SubscriptionID: t.SubscriptionID, TransitionID: t.ID, UserID: userID, PlanID: t.PlanID, State: string(t.State),
	}), "upsert supporter subscription head")
}

// setDeadline writes the deadline of a subscription, or removes it when d is nil.
func setDeadline(ctx context.Context, q *postgresgen.Queries, subscriptionID string, d *supporters.Deadline) error {
	if d == nil {
		return wrap(q.DeleteSupporterDeadline(ctx, subscriptionID), "delete supporter deadline")
	}
	return wrap(q.UpsertSupporterDeadline(ctx, postgresgen.UpsertSupporterDeadlineParams{
		SubscriptionID: subscriptionID, Kind: string(d.Kind), DueAt: d.DueAt,
	}), "upsert supporter deadline")
}

// InsertSubscription implements supporters.Repository.
func (r *SupporterRepository) InsertSubscription(ctx context.Context, tx uow.Tx, s supporters.Subscription, first supporters.TransitionRecord, d *supporters.Deadline) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertSupporterLiveSubscription(ctx, postgresgen.InsertSupporterLiveSubscriptionParams{UserID: s.UserID, SubscriptionID: s.ID})
	if isUniqueViolation(err, "supporter_live_subscriptions_pkey") {
		return fmt.Errorf("user %s: %w", s.UserID, supporters.ErrLiveSubscription)
	}
	if err != nil {
		return wrap(err, "insert supporter live subscription")
	}
	err = q.InsertSupporterSubscription(ctx, postgresgen.InsertSupporterSubscriptionParams{
		ID: s.ID, UserID: s.UserID, PlanID: s.PlanID, PriceID: s.PriceID, Provider: s.Provider, CreatedAt: s.CreatedAt,
	})
	if err != nil {
		return wrap(err, "insert supporter subscription")
	}
	if err := insertTransition(ctx, q, first, s.UserID); err != nil {
		return err
	}
	if !first.State.Live() {
		if err := q.DeleteSupporterLiveSubscription(ctx, s.ID); err != nil {
			return wrap(err, "delete supporter live subscription")
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
	return wrap(q.InsertSupporterCheckout(ctx, postgresgen.InsertSupporterCheckoutParams{
		ID: c.ID, SubscriptionID: c.SubscriptionID, Provider: c.Provider, ProviderCheckoutID: string(c.Ref),
		ExpiresAt: c.ExpiresAt, CreatedAt: c.CreatedAt,
	}), "insert supporter checkout")
}

// subscription reads a subscription with its current transition, link, and latest checkout.
func subscription(ctx context.Context, q *postgresgen.Queries, id string) (supporters.SubscriptionRecord, error) {
	row, err := q.GetSupporterSubscription(ctx, id)
	if err != nil {
		return supporters.SubscriptionRecord{}, notFound(err, supporters.ErrNotFound, "supporter subscription "+id)
	}
	rec := supporters.SubscriptionRecord{
		Subscription: supporters.Subscription{
			ID: row.ID, UserID: row.UserID, PlanID: row.PlanID, PriceID: row.PriceID, Provider: row.Provider,
			CreatedAt: utc(row.CreatedAt),
		},
		Current: supporters.TransitionRecord{
			ID: row.TransitionID, SubscriptionID: row.ID,
			Transition: domain.Transition{
				State: domain.State(row.State), PlanID: row.TransitionPlanID, PriceID: row.TransitionPriceID,
			},
			Reason: domain.Reason(row.Reason), OccurredAt: utc(row.OccurredAt),
		},
	}
	if row.PeriodEndsAt != nil {
		rec.Current.PeriodEndsAt = utc(*row.PeriodEndsAt)
	}
	switch g, err := q.GetSupporterGracePeriod(ctx, row.TransitionID); {
	case err == nil:
		rec.Current.Grace = &domain.Grace{FailedAt: utc(g.FailedAt), EndsAt: utc(g.EndsAt)}
	case !errors.Is(err, pgx.ErrNoRows):
		return supporters.SubscriptionRecord{}, wrap(err, "get supporter grace period")
	}
	switch s, err := q.GetSupporterScheduledChange(ctx, row.TransitionID); {
	case err == nil:
		rec.Current.Scheduled = &domain.ScheduledChange{PriceID: s.PriceID, EffectiveAt: utc(s.EffectiveAt)}
	case !errors.Is(err, pgx.ErrNoRows):
		return supporters.SubscriptionRecord{}, wrap(err, "get supporter scheduled change")
	}
	switch link, err := q.GetSupporterLink(ctx, row.ID); {
	case err == nil:
		rec.Link = supporters.SubscriptionRef(link)
	case !errors.Is(err, pgx.ErrNoRows):
		return supporters.SubscriptionRecord{}, wrap(err, "get supporter link")
	}
	switch c, err := q.GetLatestSupporterCheckout(ctx, row.ID); {
	case err == nil:
		rec.Checkout = &supporters.CheckoutRecord{
			ID: c.ID, SubscriptionID: c.SubscriptionID, Provider: c.Provider, Ref: supporters.CheckoutRef(c.ProviderCheckoutID),
			ExpiresAt: utc(c.ExpiresAt), CreatedAt: utc(c.CreatedAt),
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return supporters.SubscriptionRecord{}, wrap(err, "get supporter checkout")
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

// LockSubscription implements supporters.Repository with SELECT ... FOR UPDATE.
func (r *SupporterRepository) LockSubscription(ctx context.Context, tx uow.Tx, id string) (supporters.SubscriptionRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return supporters.SubscriptionRecord{}, err
	}
	if _, err := q.GetSupporterSubscriptionForUpdate(ctx, id); err != nil {
		return supporters.SubscriptionRecord{}, notFound(err, supporters.ErrNotFound, "supporter subscription "+id)
	}
	return subscription(ctx, q, id)
}

// FindSubscriptionIDByRef implements supporters.Repository.
func (r *SupporterRepository) FindSubscriptionIDByRef(ctx context.Context, tx uow.Tx, provider string, ref supporters.SubscriptionRef) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetSupporterSubscriptionIDByRef(ctx, postgresgen.GetSupporterSubscriptionIDByRefParams{
		Provider: provider, ProviderSubscriptionID: string(ref),
	})
	if err != nil {
		return "", notFound(err, supporters.ErrNotFound, "supporter subscription of "+string(ref))
	}
	return id, nil
}

// FindSubscriptionIDByCheckout implements supporters.Repository.
func (r *SupporterRepository) FindSubscriptionIDByCheckout(ctx context.Context, tx uow.Tx, provider string, ref supporters.CheckoutRef) (string, error) {
	q, err := r.q(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetSupporterSubscriptionIDByCheckout(ctx, postgresgen.GetSupporterSubscriptionIDByCheckoutParams{
		Provider: provider, ProviderCheckoutID: string(ref),
	})
	if err != nil {
		return "", notFound(err, supporters.ErrNotFound, "supporter subscription of checkout "+string(ref))
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
		return "", notFound(err, supporters.ErrNotFound, "live supporter subscription of "+userID)
	}
	return id, nil
}

// InsertLink implements supporters.Repository.
func (r *SupporterRepository) InsertLink(ctx context.Context, tx uow.Tx, subscriptionID, provider string, ref supporters.SubscriptionRef, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertSupporterLink(ctx, postgresgen.InsertSupporterLinkParams{
		SubscriptionID: subscriptionID, Provider: provider, ProviderSubscriptionID: string(ref), LinkedAt: at,
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
		err = q.EnsureSupporterLiveSubscription(ctx, postgresgen.EnsureSupporterLiveSubscriptionParams{UserID: userID, SubscriptionID: t.SubscriptionID})
	} else {
		err = q.DeleteSupporterLiveSubscription(ctx, t.SubscriptionID)
	}
	if err != nil {
		return wrap(err, "update supporter live subscription")
	}
	return setDeadline(ctx, q, t.SubscriptionID, d)
}

// InsertReversal implements supporters.Repository.
func (r *SupporterRepository) InsertReversal(ctx context.Context, tx uow.Tx, rev supporters.Reversal) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertSupporterReversal(ctx, postgresgen.InsertSupporterReversalParams{
		ID: rev.ID, SubscriptionID: rev.SubscriptionID, Kind: string(rev.Kind), Provider: rev.Provider,
		ProviderReversalID: rev.ProviderID, OccurredAt: rev.OccurredAt,
	})
	return n > 0, wrap(err, "insert supporter reversal")
}

// HasRevokingReversal implements supporters.Repository.
func (r *SupporterRepository) HasRevokingReversal(ctx context.Context, tx uow.Tx, subscriptionID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	found, err := q.HasRevokingSupporterReversal(ctx, subscriptionID)
	return found, wrap(err, "find supporter reversals")
}

// KnownReversals implements supporters.Repository.
func (r *SupporterRepository) KnownReversals(ctx context.Context, provider string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := r.db.reader().ListKnownSupporterReversals(ctx, postgresgen.ListKnownSupporterReversalsParams{Provider: provider, Ids: ids})
	return out, wrap(err, "list known supporter reversals")
}

// OpenCheckouts implements supporters.Repository.
func (r *SupporterRepository) OpenCheckouts(ctx context.Context, provider string, now time.Time) ([]supporters.CheckoutRef, error) {
	rows, err := r.db.reader().ListOpenSupporterCheckouts(ctx, postgresgen.ListOpenSupporterCheckoutsParams{Provider: provider, Now: now})
	if err != nil {
		return nil, wrap(err, "list open supporter checkouts")
	}
	out := make([]supporters.CheckoutRef, len(rows))
	for i, ref := range rows {
		out[i] = supporters.CheckoutRef(ref)
	}
	return out, nil
}

// DueDeadlines implements supporters.Repository.
func (r *SupporterRepository) DueDeadlines(ctx context.Context, now time.Time, limit int) ([]supporters.Deadline, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDueSupporterDeadlines(ctx, postgresgen.ListDueSupporterDeadlinesParams{Now: now, Lim: lim})
	if err != nil {
		return nil, wrap(err, "list due supporter deadlines")
	}
	out := make([]supporters.Deadline, len(rows))
	for i, row := range rows {
		out[i] = supporters.Deadline{SubscriptionID: row.SubscriptionID, Kind: domain.Deadline(row.Kind), DueAt: utc(row.DueAt)}
	}
	return out, nil
}

// ListSubscriptions implements supporters.Repository.
func (r *SupporterRepository) ListSubscriptions(ctx context.Context, f supporters.SubscriptionFilter) ([]supporters.SubscriptionRecord, error) {
	q := r.db.reader()
	lim, err := toInt32(f.Limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := q.ListSupporterSubscriptionIDs(ctx, postgresgen.ListSupporterSubscriptionIDsParams{
		PlanID: toNullString(f.PlanID), State: string(f.State), Before: toNullString(f.Before), Lim: lim,
	})
	if err != nil {
		return nil, wrap(err, "list supporter subscriptions")
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
	return ids, wrap(err, "list live supporter subscriptions of a plan")
}

// LiveSubscriptionIDs implements supporters.Repository.
func (r *SupporterRepository) LiveSubscriptionIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListLiveSupporterSubscriptionIDs(ctx)
	return ids, wrap(err, "list live supporter subscriptions")
}

// CountLiveSubscriptions implements supporters.Repository.
func (r *SupporterRepository) CountLiveSubscriptions(ctx context.Context, tx uow.Tx) (int64, error) {
	q, err := r.q(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountSupporterLiveSubscriptions(ctx)
	return n, wrap(err, "count live supporter subscriptions")
}

// CountMembers implements supporters.Repository.
func (r *SupporterRepository) CountMembers(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.reader().CountSupporterMembers(ctx)
	if err != nil {
		return nil, wrap(err, "count supporter members")
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
	rows, err := r.db.reader().ListSupporterSubscriptionUsers(ctx, ids)
	if err != nil {
		return nil, wrap(err, "list supporter subscription users")
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
	return wrap(q.InsertSupporterListing(ctx, postgresgen.InsertSupporterListingParams{UserID: userID, CreatedAt: at}), "insert supporter listing")
}

// HasListing implements supporters.Repository.
func (r *SupporterRepository) HasListing(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return false, err
	}
	found, err := q.HasSupporterListing(ctx, userID)
	return found, wrap(err, "find supporter listing")
}

// ListListings implements supporters.Repository.
func (r *SupporterRepository) ListListings(ctx context.Context, f supporters.ListingFilter) ([]supporters.Listing, error) {
	lim, err := toInt32(f.Limit, "limit")
	if err != nil {
		return nil, err
	}
	params := postgresgen.ListSupporterListingsParams{PlanID: toNullString(f.PlanID), Lim: lim}
	if f.After != nil {
		if params.AfterRank, err = toInt32(f.After.Rank, "rank"); err != nil {
			return nil, err
		}
		params.AfterUserID, params.AfterAt = toNullString(f.After.UserID), f.After.ListedAt
	}
	rows, err := r.db.reader().ListSupporterListings(ctx, params)
	if err != nil {
		return nil, wrap(err, "list supporter listings")
	}
	out := make([]supporters.Listing, len(rows))
	for i, row := range rows {
		out[i] = supporters.Listing{UserID: row.UserID, PlanID: row.PlanID, Rank: int(row.Rank), ListedAt: utc(row.CreatedAt)}
	}
	return out, nil
}

// DeleteListing implements supporters.Repository.
func (r *SupporterRepository) DeleteListing(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteSupporterListing(ctx, userID), "delete supporter listing")
}

// EraseUser implements supporters.Repository.
func (r *SupporterRepository) EraseUser(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteSupporterListing(ctx, userID); err != nil {
		return wrap(err, "delete supporter listing")
	}
	return wrap(q.DeleteSupporterCustomersOfUser(ctx, userID), "delete supporter customers")
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
			return wrap(err, step.what)
		}
	}
	return nil
}
