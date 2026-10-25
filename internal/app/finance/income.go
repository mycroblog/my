// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// The income records (ADR-0073, FR-917): the payment provider's final money movements, which
// the supporters module hands over through RecordIncome, and the totals of their net by period,
// computed on read like the costs. The finance module reads the supporters through the
// SupporterDirectory port, which internal/platform implements on the supporters module.

// ErrIncomeParentMissing reports a refund, a chargeback, or a reversal whose payment is not
// recorded yet: the job that hands it over records the payment first and retries (ADR-0073 §8).
var ErrIncomeParentMissing = errors.New("finance: the payment of the movement is not recorded")

// IncomeMovement is a final money movement of a payment provider in the provider-neutral form
// that RecordIncome takes (ADR-0073 §1). Its fields are plain values, so that the module that
// hands it over needs nothing of the finance domain.
type IncomeMovement struct {
	// Provider is the provider's name ("stripe", "payjp"), and ProviderID its ID of the movement, unique
	// per provider (Stripe: the balance transaction; PAY.JP: ADR-0099 §5).
	Provider   string
	ProviderID string
	// Kind is a finance.IncomeKind: payment, refund, chargeback, chargeback_reversal, or fee.
	Kind string
	// ParentID is the ProviderID of the payment of a refund, a chargeback, or a reversal; ""
	// otherwise.
	ParentID string
	// SubscriptionID is the supporters module's ID of the subscription; "" for a fee that belongs
	// to none, and for a movement of a subscription that is not ours.
	SubscriptionID string
	// OccurredAt is when the provider booked the movement, and OccurredOn its date (YYYY-MM-DD)
	// in the provider's reporting time zone (§3).
	OccurredAt time.Time
	OccurredOn string
	// CoversStart and CoversEnd (YYYY-MM-DD, both included) are the billing period that a
	// payment pays for; "" for none.
	CoversStart, CoversEnd string
	// Gross, Fee, and Net are decimals in major units of Currency, Net = Gross - Fee.
	Gross, Fee, Net string
	Currency        string
}

// IncomeRecord is a row of income_movements.
type IncomeRecord struct {
	ID         string
	Provider   string
	ProviderID string
	Kind       finance.IncomeKind
	// ParentMovementID is the row of the payment of a refund, a chargeback, or a reversal; ""
	// otherwise. ParentProviderID is that payment's ProviderID, which the listing reads.
	ParentMovementID string
	ParentProviderID string
	SubscriptionID   string
	OccurredAt       time.Time
	OccurredOn       finance.Date
	Covers           *finance.DateRange
	Gross, Fee, Net  finance.Money
	RecordedAt       time.Time
}

// IncomeFilter narrows the listing of the movements: the dates they fall on, from From to To,
// and a kind unless Kind is "".
type IncomeFilter struct {
	From, To finance.Date
	Kind     finance.IncomeKind
}

// IncomeRepository is the part of Repository that records the income (ADR-0073 §8).
type IncomeRepository interface {
	// InsertIncome records a movement and reports whether it did: a movement already recorded
	// (the same provider and provider ID) writes nothing.
	InsertIncome(ctx context.Context, tx uow.Tx, r IncomeRecord) (bool, error)
	// FindIncomeID returns the row of a provider's movement in tx; it fails with ErrNotFound
	// when it is not recorded.
	FindIncomeID(ctx context.Context, tx uow.Tx, provider, providerID string) (string, error)
	// KnownIncome returns the provider IDs among ids that are recorded.
	KnownIncome(ctx context.Context, provider string, ids []string) ([]string, error)
	// IncomeInWindow returns the movements whose place in the totals can meet window: those on
	// its dates, the payments whose period meets it, and the movements on or before its end of a
	// payment whose period reaches it, with that payment's period (ParentCovers).
	IncomeInWindow(ctx context.Context, window finance.DateRange) ([]finance.IncomeMovement, error)
	// ListIncome returns up to limit movements of f, newest first by OccurredAt and ID, before
	// the movement before ("" for the first page).
	ListIncome(ctx context.Context, f IncomeFilter, before string, limit int) ([]IncomeRecord, error)
}

// PlanCount is the number of members of a supporter plan (ADR-0073 §7, N1).
type PlanCount struct {
	PlanID  string
	Name    string
	Members int64
}

// ProviderRule is the reporting time zone of a payment provider, in which its movements fall on
// their dates (ADR-0073 §3).
type ProviderRule struct {
	Provider string
	Location *time.Location
}

// Subscriber is the local account of the subscriber of a subscription: its ID and its username
// without the leading "@".
type Subscriber struct {
	AccountID string
	Username  string
}

// SupporterDirectory is what the finance module reads of the supporters module (ADR-0073 §1):
// internal/platform implements it on the supporters module. It reads nothing while the program
// is disabled.
type SupporterDirectory interface {
	// PlanCounts returns the members of each plan now: the plans that are not retired and the
	// retired plans that still have members.
	PlanCounts(ctx context.Context) ([]PlanCount, error)
	// SubscriptionAccounts returns the local account of the subscriber of each subscription
	// among ids, by subscription ID; an unknown subscription or an erased account is left out.
	SubscriptionAccounts(ctx context.Context, ids []string) (map[string]Subscriber, error)
	// ProviderRules returns the time zone of the configured payment provider; none without one.
	ProviderRules() []ProviderRule
}

// ParseIncome returns the row of m, rounding its decimals to the minor unit of its currency
// (finance.MinorUnitsOf), without its ID, parent row, and recording time. It fails with
// finance.ErrInvalidIncome, finance.ErrInvalidDate, and the errors of finance.MinorUnitsOf and
// finance.IncomeMovement.Validate.
func ParseIncome(m IncomeMovement) (IncomeRecord, error) {
	if m.Provider == "" || m.ProviderID == "" {
		return IncomeRecord{}, fmt.Errorf("%w: no provider or provider ID", finance.ErrInvalidIncome)
	}
	kind := finance.IncomeKind(m.Kind)
	if m.ParentID != "" && !kind.HasParent() {
		return IncomeRecord{}, fmt.Errorf("%w: a %s with a parent", finance.ErrInvalidIncome, m.Kind)
	}
	c, err := finance.ParseCurrency(m.Currency)
	if err != nil {
		return IncomeRecord{}, err
	}
	var minor [3]int64
	for i, s := range []string{m.Gross, m.Fee, m.Net} {
		if minor[i], err = finance.MinorUnitsOf(s, c); err != nil {
			return IncomeRecord{}, err
		}
	}
	on, err := finance.ParseDate(m.OccurredOn)
	if err != nil {
		return IncomeRecord{}, err
	}
	r := IncomeRecord{
		Provider: m.Provider, ProviderID: m.ProviderID, Kind: kind, SubscriptionID: m.SubscriptionID,
		OccurredAt: kernel.Normalize(m.OccurredAt), OccurredOn: on,
		Gross: finance.Money{Minor: minor[0], Currency: c}, Fee: finance.Money{Minor: minor[1], Currency: c},
		Net: finance.Money{Minor: minor[2], Currency: c},
	}
	if m.CoversStart != "" || m.CoversEnd != "" {
		start, err := finance.ParseDate(m.CoversStart)
		if err != nil {
			return IncomeRecord{}, err
		}
		end, err := finance.ParseDate(m.CoversEnd)
		if err != nil {
			return IncomeRecord{}, err
		}
		r.Covers = &finance.DateRange{Start: start, End: end}
	}
	return r, r.movement().Validate()
}

// movement returns the movement of r as the totals read it, without the period of its parent.
func (r IncomeRecord) movement() finance.IncomeMovement {
	return finance.IncomeMovement{Kind: r.Kind, OccurredOn: r.OccurredOn, Covers: r.Covers, Gross: r.Gross, Fee: r.Fee, Net: r.Net}
}

// RecordIncome records the final money movements of a payment provider, each once however
// often it comes (ADR-0073 §2), in order, one short transaction each: a refund, a chargeback,
// or a reversal after its payment, which the batch or an earlier one recorded. It fails with
// ErrIncomeParentMissing for a movement whose payment is not recorded, so that the job retries
// once it is. A movement that breaks the rules of the income records is logged and left out,
// since no retry would make it valid.
func (s *Service) RecordIncome(ctx context.Context, ms []IncomeMovement) error {
	for _, m := range ms {
		r, err := ParseIncome(m)
		if err != nil {
			s.logger().ErrorContext(ctx, "a money movement that the income records cannot hold",
				slog.String("provider", m.Provider), slog.String("movement", m.ProviderID), slog.Any("error", err))
			continue
		}
		r.ID = s.d.IDs.NewID()
		r.RecordedAt = kernel.Normalize(s.d.Clock.Now())
		err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			if m.ParentID != "" {
				parent, err := s.d.Repository.FindIncomeID(ctx, tx, m.Provider, m.ParentID)
				if errors.Is(err, ErrNotFound) {
					return fmt.Errorf("%w: %s %s of %s", ErrIncomeParentMissing, m.Kind, m.ProviderID, m.ParentID)
				}
				if err != nil {
					return err
				}
				r.ParentMovementID = parent
			}
			_, err := s.d.Repository.InsertIncome(ctx, tx, r)
			return err
		})
		if err != nil {
			return fmt.Errorf("finance: record the money movement %s: %w", m.ProviderID, err)
		}
	}
	return nil
}

// KnownIncome reports which of the movement IDs of provider are recorded.
func (s *Service) KnownIncome(ctx context.Context, provider string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	known, err := s.d.Repository.KnownIncome(ctx, provider, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range known {
		out[id] = true
	}
	return out, nil
}

// IncomePeriod is a period of the income totals, with its balance when the query asks for it.
type IncomePeriod struct {
	finance.IncomeTotal
	// Balance is the net less the costs of each currency (finance.Balance) over the period,
	// under the same basis; nil unless IncomeQuery.Balance.
	Balance []finance.Money
}

// IncomeQuery asks for the income totals of the periods of TotalsQuery, with the balance when
// Balance is true.
type IncomeQuery struct {
	TotalsQuery
	Balance bool
}

// IncomeTotals returns the totals of the recorded income over the periods of q under its basis
// (ADR-0073 §5), computed on read, and the balance against the costs (Totals) when q asks for
// it. It fails with finance.ErrInvalidBasis and the errors of finance.Granularity.Periods.
func (s *Service) IncomeTotals(ctx context.Context, q IncomeQuery) ([]IncomePeriod, error) {
	periods, err := s.periods(q.TotalsQuery)
	if err != nil || len(periods) == 0 {
		return []IncomePeriod{}, err
	}
	window := finance.DateRange{Start: periods[0].Range.Start, End: periods[len(periods)-1].Range.End}
	ms, err := s.d.Repository.IncomeInWindow(ctx, window)
	if err != nil {
		return nil, fmt.Errorf("finance: read the income: %w", err)
	}
	totals := finance.IncomeTotals(ms, q.Basis, periods)
	out := make([]IncomePeriod, len(totals))
	for i, t := range totals {
		out[i] = IncomePeriod{IncomeTotal: t}
	}
	if !q.Balance {
		return out, nil
	}
	costs, err := s.Totals(ctx, TotalsQuery{Granularity: q.Granularity, From: q.From, To: q.To, Basis: q.Basis, UntilToday: q.UntilToday})
	if err != nil {
		return nil, err
	}
	for i := range out {
		var c []finance.Money
		if i < len(costs) {
			c = costs[i].Totals
		}
		out[i].Balance = finance.Balance(out[i].Net, c)
	}
	return out, nil
}

// periods returns the periods of q, stopped at today (UTC) when q asks for it; none when q
// starts after today. It fails with finance.ErrInvalidBasis, finance.ErrInvalidPeriod, and the
// errors of finance.Granularity.Periods.
func (s *Service) periods(q TotalsQuery) ([]finance.Period, error) {
	if err := q.Basis.Validate(); err != nil {
		return nil, err
	}
	if q.To.Before(q.From) {
		return nil, fmt.Errorf("%w: %s to %s", finance.ErrInvalidPeriod, q.From, q.To)
	}
	if q.UntilToday {
		today := finance.DateOf(s.d.Clock.Now().UTC())
		if q.From.After(today) {
			return nil, nil
		}
		if q.To.After(today) {
			q.To = today
		}
	}
	return q.Granularity.Periods(q.From, q.To)
}

// PlanCounts returns the members of each supporter plan now (ADR-0073 §7); none without the
// supporters module.
func (s *Service) PlanCounts(ctx context.Context) ([]PlanCount, error) {
	if s.d.Supporters == nil {
		return []PlanCount{}, nil
	}
	out, err := s.d.Supporters.PlanCounts(ctx)
	if out == nil && err == nil {
		out = []PlanCount{}
	}
	return out, err
}

// IncomeRules returns the reporting time zones of the payment providers (ADR-0073 §3).
func (s *Service) IncomeRules() []ProviderRule {
	if s.d.Supporters == nil {
		return []ProviderRule{}
	}
	out := s.d.Supporters.ProviderRules()
	if out == nil {
		out = []ProviderRule{}
	}
	return out
}

// IncomeEntry is a recorded movement with the local account of its subscriber, as the
// administrators see it (ADR-0073 §6); Subscriber is the zero value without one.
type IncomeEntry struct {
	IncomeRecord
	Subscriber Subscriber
}

// ListIncome returns up to limit movements of f, newest first, before the movement before (""
// for the first page), with the accounts of their subscribers.
func (s *Service) ListIncome(ctx context.Context, f IncomeFilter, before string, limit int) ([]IncomeEntry, error) {
	rows, err := s.d.Repository.ListIncome(ctx, f, before, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, r := range rows {
		if r.SubscriptionID != "" {
			ids = append(ids, r.SubscriptionID)
		}
	}
	accounts := map[string]Subscriber{}
	if s.d.Supporters != nil && len(ids) > 0 {
		if accounts, err = s.d.Supporters.SubscriptionAccounts(ctx, ids); err != nil {
			return nil, fmt.Errorf("finance: read the subscribers: %w", err)
		}
	}
	out := make([]IncomeEntry, len(rows))
	for i, r := range rows {
		out[i] = IncomeEntry{IncomeRecord: r, Subscriber: accounts[r.SubscriptionID]}
	}
	return out, nil
}
