// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/financegen"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// Repository implements appfinance.Repository (data-model §4.12, ADR-0067).
type Repository struct {
	db *moduleDB
}

var _ appfinance.Repository = (*Repository)(nil)

// NewRepository returns the finance module's repository on this database.
func NewRepository(d *postgres.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// Dates are date (data-model §1.2), which pgx reads as the start of the day in UTC.
func toDate(d finance.Date) pgtype.Date { return pgtype.Date{Time: d.Time(), Valid: true} }

func toNullDate(d *finance.Date) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return toDate(*d)
}

func fromDate(d pgtype.Date) (finance.Date, error) {
	if !d.Valid || d.InfinityModifier != pgtype.Finite {
		return finance.Date{}, errors.New("postgres: read date: not a finite date")
	}
	return finance.DateOf(d.Time.UTC()), nil
}

func fromNullDate(d pgtype.Date) (*finance.Date, error) {
	if !d.Valid {
		return nil, nil
	}
	out, err := fromDate(d)
	return &out, err
}

// InsertItem implements appfinance.Repository.
func (r *Repository) InsertItem(ctx context.Context, tx uow.Tx, item finance.CostItem) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	interval, err := postgres.ToInt32(item.Recurrence.Interval, "cost item interval")
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertCostItem(ctx, financegen.InsertCostItemParams{
		ID: item.ID, Schedule: string(item.Recurrence.Schedule), Interval: interval, StartsOn: toDate(item.StartsOn),
		CreatedByUserID: postgres.ToNullString(item.CreatedByUserID), CreatedAt: item.CreatedAt,
	}), "insert cost item")
}

// InsertLabelRevision implements appfinance.Repository.
func (r *Repository) InsertLabelRevision(ctx context.Context, tx uow.Tx, rev appfinance.LabelRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertCostItemLabelRevision(ctx, financegen.InsertCostItemLabelRevisionParams{
		ID: rev.ID, CostItemID: rev.CostItemID, Name: rev.Label.Name, Provider: rev.Label.Provider,
		ProviderNormalized: rev.ProviderNormalized, Category: string(rev.Label.Category), Memo: rev.Label.Memo,
		RevisedByUserID: postgres.ToNullString(rev.RevisedByUserID), RevisedAt: rev.RevisedAt,
	}), "insert cost item label revision")
}

// InsertPriceRevision implements appfinance.Repository.
func (r *Repository) InsertPriceRevision(ctx context.Context, tx uow.Tx, rev appfinance.PriceRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	p := rev.Revision
	return postgres.Wrap(q.InsertCostItemPriceRevision(ctx, financegen.InsertCostItemPriceRevisionParams{
		ID: p.ID, CostItemID: rev.CostItemID, EffectiveOn: toDate(p.EffectiveOn), AmountMinor: p.Price.Minor,
		Currency: string(p.Price.Currency), RevisedByUserID: postgres.ToNullString(rev.RevisedByUserID), RevisedAt: p.RevisedAt,
	}), "insert cost item price revision")
}

// InsertEndChange implements appfinance.Repository.
func (r *Repository) InsertEndChange(ctx context.Context, tx uow.Tx, c appfinance.EndChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertCostItemEndChange(ctx, financegen.InsertCostItemEndChangeParams{
		ID: c.ID, CostItemID: c.CostItemID, EndsOn: toNullDate(c.EndsOn), ChangedByUserID: postgres.ToNullString(c.ChangedByUserID),
		ChangedAt: c.ChangedAt,
	}), "insert cost item end change")
}

// InsertCancellation implements appfinance.Repository.
func (r *Repository) InsertCancellation(ctx context.Context, tx uow.Tx, c appfinance.Cancellation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertCostItemCancellation(ctx, financegen.InsertCostItemCancellationParams{
		ID: c.ID, CostItemID: c.CostItemID, CanceledByUserID: postgres.ToNullString(c.CanceledByUserID), CanceledAt: c.CanceledAt,
	}), "insert cost item cancellation")
}

// UpsertHead implements appfinance.Repository.
func (r *Repository) UpsertHead(ctx context.Context, tx uow.Tx, h appfinance.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertCostItemHead(ctx, financegen.UpsertCostItemHeadParams{
		CostItemID: h.CostItemID, LabelRevisionID: h.LabelRevisionID, EndsOn: toNullDate(h.EndsOn),
	}), "upsert cost item head")
}

// DeleteHead implements appfinance.Repository.
func (r *Repository) DeleteHead(ctx context.Context, tx uow.Tx, costItemID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteCostItemHead(ctx, costItemID), "delete cost item head")
}

// CurrentForUpdate implements appfinance.Repository; it locks the head until the transaction ends.
func (r *Repository) CurrentForUpdate(ctx context.Context, tx uow.Tx, costItemID string) (appfinance.Current, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appfinance.Current{}, err
	}
	row, err := q.GetCostItemCurrentForUpdate(ctx, costItemID)
	if err != nil {
		return appfinance.Current{}, postgres.NotFound(err, appfinance.ErrNotFound, "cost item "+costItemID)
	}
	starts, err := fromDate(row.StartsOn)
	if err != nil {
		return appfinance.Current{}, err
	}
	ends, err := fromNullDate(row.EndsOn)
	if err != nil {
		return appfinance.Current{}, err
	}
	return appfinance.Current{
		Head:     appfinance.Head{CostItemID: row.CostItemID, LabelRevisionID: row.LabelRevisionID, EndsOn: ends},
		StartsOn: starts,
		Name:     row.Name,
	}, nil
}

// FindItem implements appfinance.Repository.
func (r *Repository) FindItem(ctx context.Context, id string) (finance.CostItem, error) {
	if id == "" {
		return finance.CostItem{}, appfinance.ErrNotFound
	}
	out, err := r.items(ctx, id, "", 1)
	if err != nil {
		return finance.CostItem{}, err
	}
	if len(out) == 0 {
		return finance.CostItem{}, postgres.NotFound(pgx.ErrNoRows, appfinance.ErrNotFound, "cost item "+id)
	}
	return out[0], nil
}

// ListItems implements appfinance.Repository.
func (r *Repository) ListItems(ctx context.Context, before string, limit int) ([]finance.CostItem, error) {
	if limit <= 0 {
		return []finance.CostItem{}, nil
	}
	return r.items(ctx, "", before, int32(min(limit, math.MaxInt32)))
}

// AllItems implements appfinance.Repository.
func (r *Repository) AllItems(ctx context.Context) ([]finance.CostItem, error) {
	return r.items(ctx, "", "", math.MaxInt32)
}

func (r *Repository) items(ctx context.Context, id, before string, limit int32) ([]finance.CostItem, error) {
	q := r.db.reader()
	rows, err := q.ListCostItems(ctx, financegen.ListCostItemsParams{ID: postgres.ToNullString(id), Before: postgres.ToNullString(before), Lim: limit})
	if err != nil {
		return nil, postgres.Wrap(err, "list cost items")
	}
	out := make([]finance.CostItem, len(rows))
	ids := make([]string, len(rows))
	byID := make(map[string]int, len(rows))
	for i, row := range rows {
		starts, err := fromDate(row.StartsOn)
		if err != nil {
			return nil, err
		}
		ends, err := fromNullDate(row.EndsOn)
		if err != nil {
			return nil, err
		}
		out[i] = finance.CostItem{
			ID: row.ID, Recurrence: finance.Recurrence{Schedule: finance.Schedule(row.Schedule), Interval: int(row.Interval)},
			StartsOn: starts, EndsOn: ends,
			Label: finance.Label{
				Name: row.Name, Provider: row.Provider, Category: finance.Category(row.Category), Memo: row.Memo,
			},
			ProviderNormalized: row.ProviderNormalized, LabelRevisionID: row.LabelRevisionID, LabelRevisedAt: postgres.UTC(row.LabelRevisedAt),
			CreatedByUserID: postgres.FromNullString(row.CreatedByUserID), CreatedAt: postgres.UTC(row.CreatedAt),
		}
		ids[i] = row.ID
		byID[row.ID] = i
	}
	if len(rows) == 0 {
		return out, nil
	}
	prices, err := q.ListCostItemPriceRevisions(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list cost item price revisions")
	}
	for _, p := range prices {
		on, err := fromDate(p.EffectiveOn)
		if err != nil {
			return nil, fmt.Errorf("cost item %s: %w", p.CostItemID, err)
		}
		i := byID[p.CostItemID]
		out[i].Prices = append(out[i].Prices, finance.PriceRevision{
			ID: p.ID, EffectiveOn: on, Price: finance.Money{Minor: p.AmountMinor, Currency: finance.Currency(p.Currency)},
			RevisedAt: postgres.UTC(p.RevisedAt),
		})
	}
	return out, nil
}

// RebuildHeads implements appfinance.Repository.
func (r *Repository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild cost item heads",
		(*financegen.Queries).DeleteAllCostItemHeads, (*financegen.Queries).RebuildCostItemHeads)
}
