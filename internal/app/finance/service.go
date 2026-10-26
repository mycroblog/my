// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// Deps are the ports of a Service.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// Sources are the billing sources of billing_sources, in its order (ADR-0068); none imports
	// nothing.
	Sources []BillingSource
	// Caps bounds the billed requests of the sources it names (the AWS caps of ADR-0068 §4).
	Caps map[string]RequestCaps
	// Outbox enqueues the manual imports; nil refuses them.
	Outbox queue.Outbox
	// Supporters reads the supporters module for the income (ADR-0073 §1); nil reads no plans,
	// subscribers, or providers.
	Supporters SupporterDirectory
	// Logger logs the imports and the income records; nil means slog.Default().
	Logger *slog.Logger
	// Meter records my.billing.imports; nil records nothing.
	Meter kernel.Meter
}

// Service runs the use cases of the instance's costs.
type Service struct {
	d       Deps
	imports kernel.Counter
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	return &Service{d: d, imports: kernel.MeterOrNop(d.Meter).Counter(BillingImports)}
}

// NormalizeProvider returns the form of a provider that groups the breakdown
// (cost_item_label_revisions.provider_normalized): NFKC, then lower case.
func NormalizeProvider(provider string) string {
	return strings.ToLower(norm.NFKC.String(provider))
}

// CreateInput is a new cost item: its schedule, start date, first label, and first price, which
// starts on the start date.
type CreateInput struct {
	Recurrence finance.Recurrence
	StartsOn   finance.Date
	// EndsOn is the last date that a payment can fall on; nil for no end.
	EndsOn *finance.Date
	Label  finance.Label
	Price  finance.Money
}

// audit returns the audit log entry of an operation on a cost item (ADR-0067 §7), labeled with
// the item's name (at most finance.MaxName characters); sourceID is the row that the
// operation recorded.
func (s *Service) audit(action appmoderation.AuditAction, itemID, name, sourceID, byUserID string, at time.Time) appmoderation.AuditRecorded {
	actor, user := appmoderation.AuditActor(byUserID)
	return appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: actor, ActorUserID: user, Action: action, TargetKind: appmoderation.AuditTargetCostItem,
		TargetID: itemID, TargetLabel: name, SourceEventID: sourceID, OccurredAt: at,
	}}
}

// Create records a cost item and its audit log entry. byUserID is the administrator, "" for the
// CLI. It fails with the errors of finance.Recurrence.Validate, finance.Label.Validate,
// finance.ValidatePrice, and finance.ValidateDates.
func (s *Service) Create(ctx context.Context, in CreateInput, byUserID string) (finance.CostItem, error) {
	label := in.Label.Normalize()
	if err := in.Recurrence.Validate(); err != nil {
		return finance.CostItem{}, err
	}
	if err := label.Validate(); err != nil {
		return finance.CostItem{}, err
	}
	if err := finance.ValidatePrice(in.Price); err != nil {
		return finance.CostItem{}, err
	}
	if in.EndsOn != nil {
		if err := finance.ValidateDates(in.StartsOn, *in.EndsOn); err != nil {
			return finance.CostItem{}, err
		}
	}
	now := kernel.Normalize(s.d.Clock.Now())
	item := finance.CostItem{
		ID: s.d.IDs.NewID(), Recurrence: in.Recurrence, StartsOn: in.StartsOn, CreatedByUserID: byUserID, CreatedAt: now,
	}
	rev := LabelRevision{
		ID: s.d.IDs.NewID(), CostItemID: item.ID, Label: label, ProviderNormalized: NormalizeProvider(label.Provider),
		RevisedByUserID: byUserID, RevisedAt: now,
	}
	price := PriceRevision{
		CostItemID:      item.ID,
		Revision:        finance.PriceRevision{ID: s.d.IDs.NewID(), EffectiveOn: in.StartsOn, Price: in.Price, RevisedAt: now},
		RevisedByUserID: byUserID,
	}
	var end *EndChange
	if in.EndsOn != nil {
		end = &EndChange{ID: s.d.IDs.NewID(), CostItemID: item.ID, EndsOn: in.EndsOn, ChangedByUserID: byUserID, ChangedAt: now}
	}
	audit := s.audit(appmoderation.AuditCostItemCreated, item.ID, label.Name, item.ID, byUserID, now)
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.InsertItem(ctx, tx, item); err != nil {
			return err
		}
		if err := repo.InsertLabelRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := repo.InsertPriceRevision(ctx, tx, price); err != nil {
			return err
		}
		if end != nil {
			if err := repo.InsertEndChange(ctx, tx, *end); err != nil {
				return err
			}
		}
		if err := tx.Emit(ctx, CostItemChanged{Head: Head{CostItemID: item.ID, LabelRevisionID: rev.ID, EndsOn: in.EndsOn}}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return finance.CostItem{}, err
	}
	return s.Get(ctx, item.ID)
}

// change runs write on the current head of a cost item in one transaction and emits the head it
// returns and the audit log entry that audit makes from the current state; it fails with
// ErrNotFound for an unknown or canceled item.
func (s *Service) change(ctx context.Context, id string, audit func(cur Current) appmoderation.AuditRecorded,
	write func(tx uow.Tx, cur Current) (Head, error),
) (finance.CostItem, error) {
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.d.Repository.CurrentForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		head, err := write(tx, cur)
		if err != nil {
			return err
		}
		if err := tx.Emit(ctx, CostItemChanged{Head: head}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit(cur))
	})
	if err != nil {
		return finance.CostItem{}, err
	}
	return s.Get(ctx, id)
}

// labeled returns e with the current name of the item as its label: the entry of a change that
// keeps the name. The entry's ID is made before the transaction, so a retry writes the same row.
func labeled(e appmoderation.AuditRecorded) func(cur Current) appmoderation.AuditRecorded {
	return func(cur Current) appmoderation.AuditRecorded {
		e.Entry.TargetLabel = cur.Name
		return e
	}
}

// Relabel records a new label of a cost item. It fails with ErrNotFound and the errors of
// finance.Label.Validate.
func (s *Service) Relabel(ctx context.Context, id string, label finance.Label, byUserID string) (finance.CostItem, error) {
	label = label.Normalize()
	if err := label.Validate(); err != nil {
		return finance.CostItem{}, err
	}
	rev := LabelRevision{
		ID: s.d.IDs.NewID(), CostItemID: id, Label: label, ProviderNormalized: NormalizeProvider(label.Provider),
		RevisedByUserID: byUserID, RevisedAt: kernel.Normalize(s.d.Clock.Now()),
	}
	audit := s.audit(appmoderation.AuditCostItemRelabeled, id, label.Name, rev.ID, byUserID, rev.RevisedAt)
	return s.change(ctx, id, func(Current) appmoderation.AuditRecorded { return audit }, func(tx uow.Tx, cur Current) (Head, error) {
		if err := s.d.Repository.InsertLabelRevision(ctx, tx, rev); err != nil {
			return Head{}, err
		}
		head := cur.Head
		head.LabelRevisionID = rev.ID
		return head, nil
	})
}

// RevisePrice records the price of the payments of a cost item on or after effectiveOn: a raise,
// a cut, or the correction of a mistake (effectiveOn = the start date corrects every payment).
// It fails with ErrNotFound and the errors of finance.ValidatePrice and finance.ValidateDates.
func (s *Service) RevisePrice(ctx context.Context, id string, effectiveOn finance.Date, price finance.Money, byUserID string) (finance.CostItem, error) {
	if err := finance.ValidatePrice(price); err != nil {
		return finance.CostItem{}, err
	}
	rev := PriceRevision{
		CostItemID: id,
		Revision: finance.PriceRevision{
			ID: s.d.IDs.NewID(), EffectiveOn: effectiveOn, Price: price, RevisedAt: kernel.Normalize(s.d.Clock.Now()),
		},
		RevisedByUserID: byUserID,
	}
	audit := s.audit(appmoderation.AuditCostItemPriceRevised, id, "", rev.Revision.ID, byUserID, rev.Revision.RevisedAt)
	return s.change(ctx, id, labeled(audit), func(tx uow.Tx, cur Current) (Head, error) {
		if err := finance.ValidateDates(cur.StartsOn, effectiveOn); err != nil {
			return Head{}, err
		}
		return cur.Head, s.d.Repository.InsertPriceRevision(ctx, tx, rev)
	})
}

// SetEnd records the end of a cost item: no payment after endsOn (included), or no end for nil.
// It fails with ErrNotFound and the errors of finance.ValidateDates.
func (s *Service) SetEnd(ctx context.Context, id string, endsOn *finance.Date, byUserID string) (finance.CostItem, error) {
	change := EndChange{ID: s.d.IDs.NewID(), CostItemID: id, EndsOn: endsOn, ChangedByUserID: byUserID, ChangedAt: kernel.Normalize(s.d.Clock.Now())}
	audit := s.audit(appmoderation.AuditCostItemEndChanged, id, "", change.ID, byUserID, change.ChangedAt)
	return s.change(ctx, id, labeled(audit), func(tx uow.Tx, cur Current) (Head, error) {
		if endsOn != nil {
			if err := finance.ValidateDates(cur.StartsOn, *endsOn); err != nil {
				return Head{}, err
			}
		}
		if err := s.d.Repository.InsertEndChange(ctx, tx, change); err != nil {
			return Head{}, err
		}
		head := cur.Head
		head.EndsOn = endsOn
		return head, nil
	})
}

// Cancel removes a cost item from every period, past ones included; there is no undo. It fails
// with ErrNotFound for an unknown or already canceled item.
func (s *Service) Cancel(ctx context.Context, id, byUserID string) error {
	c := Cancellation{ID: s.d.IDs.NewID(), CostItemID: id, CanceledByUserID: byUserID, CanceledAt: kernel.Normalize(s.d.Clock.Now())}
	audit := labeled(s.audit(appmoderation.AuditCostItemCanceled, id, "", c.ID, byUserID, c.CanceledAt))
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.d.Repository.CurrentForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.d.Repository.InsertCancellation(ctx, tx, c); err != nil {
			return err
		}
		if err := tx.Emit(ctx, CostItemCanceled{CostItemID: id, CancellationID: c.ID, At: c.CanceledAt}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit(cur))
	})
}

// Get returns a cost item; it fails with ErrNotFound for an unknown or canceled one.
func (s *Service) Get(ctx context.Context, id string) (finance.CostItem, error) {
	return s.d.Repository.FindItem(ctx, id)
}

// List returns up to limit cost items that are not canceled, newest first, before the item
// before ("" for the first page).
func (s *Service) List(ctx context.Context, before string, limit int) ([]finance.CostItem, error) {
	return s.d.Repository.ListItems(ctx, before, limit)
}

// TotalsQuery asks for the totals of the weeks, months, or years from the one that holds From to
// the one that holds To, under Basis.
type TotalsQuery struct {
	Granularity finance.Granularity
	From, To    finance.Date
	Basis       finance.Basis
	// UntilToday stops the periods at the one that holds today in UTC, for the public totals
	// (ADR-0067 §8): a To after today reads as today, and a From after today gives no period.
	UntilToday bool
	// Items adds the item lines to each period (finance.ItemizedTotals, ADR-0070 §5).
	Items bool
}

// Totals returns the totals of the cost items that are not canceled and of the newest revision of
// every imported cost over the periods of q, computed on read (ADR-0067 §1); a period that an
// estimated imported cost meets is Estimated (ADR-0068 §3). It fails with finance.ErrInvalidBasis
// and the errors of finance.Granularity.Periods.
func (s *Service) Totals(ctx context.Context, q TotalsQuery) ([]finance.PeriodTotal, error) {
	periods, err := s.periods(q)
	if err != nil || len(periods) == 0 {
		return []finance.PeriodTotal{}, err
	}
	items, err := s.d.Repository.AllItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("finance: read the cost items: %w", err)
	}
	imported, err := s.importedEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("finance: read the imported costs: %w", err)
	}
	if q.Items {
		return finance.ItemizedTotals(items, imported, q.Basis, periods), nil
	}
	return finance.Totals(items, imported, q.Basis, periods), nil
}
