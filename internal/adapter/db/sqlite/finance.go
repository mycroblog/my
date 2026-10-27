// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// FinanceRepository implements appfinance.Repository (data-model §4.12, ADR-0067).
type FinanceRepository struct {
	db *DB
}

var _ appfinance.Repository = (*FinanceRepository)(nil)

// FinanceRepository returns the finance module's repository on this database.
func (d *DB) FinanceRepository() *FinanceRepository { return &FinanceRepository{db: d} }

// Dates are TEXT as YYYY-MM-DD (data-model §1.2).
func toNullDate(d *finance.Date) sql.NullString {
	if d == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: d.String(), Valid: true}
}

func fromDate(s string) (finance.Date, error) {
	d, err := finance.ParseDate(s)
	if err != nil {
		return finance.Date{}, fmt.Errorf("sqlite: read date: %w", err)
	}
	return d, nil
}

func fromNullDate(s sql.NullString) (*finance.Date, error) {
	if !s.Valid {
		return nil, nil
	}
	d, err := fromDate(s.String)
	return &d, err
}

// InsertItem implements appfinance.Repository.
func (r *FinanceRepository) InsertItem(ctx context.Context, tx uow.Tx, item finance.CostItem) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertCostItem(ctx, sqlitegen.InsertCostItemParams{
		ID: item.ID, Schedule: string(item.Recurrence.Schedule), Interval: int64(item.Recurrence.Interval),
		StartsOn: item.StartsOn.String(), CreatedByUserID: toNullString(item.CreatedByUserID), CreatedAt: toMillis(item.CreatedAt),
	}), "insert cost item")
}

// InsertLabelRevision implements appfinance.Repository.
func (r *FinanceRepository) InsertLabelRevision(ctx context.Context, tx uow.Tx, rev appfinance.LabelRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertCostItemLabelRevision(ctx, sqlitegen.InsertCostItemLabelRevisionParams{
		ID: rev.ID, CostItemID: rev.CostItemID, Name: rev.Label.Name, Provider: rev.Label.Provider,
		ProviderNormalized: rev.ProviderNormalized, Category: string(rev.Label.Category), Memo: rev.Label.Memo,
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(rev.RevisedAt),
	}), "insert cost item label revision")
}

// InsertPriceRevision implements appfinance.Repository.
func (r *FinanceRepository) InsertPriceRevision(ctx context.Context, tx uow.Tx, rev appfinance.PriceRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	p := rev.Revision
	return wrap(q.InsertCostItemPriceRevision(ctx, sqlitegen.InsertCostItemPriceRevisionParams{
		ID: p.ID, CostItemID: rev.CostItemID, EffectiveOn: p.EffectiveOn.String(), AmountMinor: p.Price.Minor,
		Currency: string(p.Price.Currency), RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(p.RevisedAt),
	}), "insert cost item price revision")
}

// InsertEndChange implements appfinance.Repository.
func (r *FinanceRepository) InsertEndChange(ctx context.Context, tx uow.Tx, c appfinance.EndChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertCostItemEndChange(ctx, sqlitegen.InsertCostItemEndChangeParams{
		ID: c.ID, CostItemID: c.CostItemID, EndsOn: toNullDate(c.EndsOn), ChangedByUserID: toNullString(c.ChangedByUserID),
		ChangedAt: toMillis(c.ChangedAt),
	}), "insert cost item end change")
}

// InsertCancellation implements appfinance.Repository.
func (r *FinanceRepository) InsertCancellation(ctx context.Context, tx uow.Tx, c appfinance.Cancellation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertCostItemCancellation(ctx, sqlitegen.InsertCostItemCancellationParams{
		ID: c.ID, CostItemID: c.CostItemID, CanceledByUserID: toNullString(c.CanceledByUserID), CanceledAt: toMillis(c.CanceledAt),
	}), "insert cost item cancellation")
}

// UpsertHead implements appfinance.Repository.
func (r *FinanceRepository) UpsertHead(ctx context.Context, tx uow.Tx, h appfinance.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertCostItemHead(ctx, sqlitegen.UpsertCostItemHeadParams{
		CostItemID: h.CostItemID, LabelRevisionID: h.LabelRevisionID, EndsOn: toNullDate(h.EndsOn),
	}), "upsert cost item head")
}

// DeleteHead implements appfinance.Repository.
func (r *FinanceRepository) DeleteHead(ctx context.Context, tx uow.Tx, costItemID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteCostItemHead(ctx, costItemID), "delete cost item head")
}

// CurrentForUpdate implements appfinance.Repository. A SQLite transaction holds the write lock,
// so the row needs no lock of its own.
func (r *FinanceRepository) CurrentForUpdate(ctx context.Context, tx uow.Tx, costItemID string) (appfinance.Current, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appfinance.Current{}, err
	}
	row, err := q.GetCostItemCurrent(ctx, costItemID)
	if err != nil {
		return appfinance.Current{}, notFound(err, appfinance.ErrNotFound, "cost item "+costItemID)
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
func (r *FinanceRepository) FindItem(ctx context.Context, id string) (finance.CostItem, error) {
	if id == "" {
		return finance.CostItem{}, appfinance.ErrNotFound
	}
	out, err := r.items(ctx, id, "", 1)
	if err != nil {
		return finance.CostItem{}, err
	}
	if len(out) == 0 {
		return finance.CostItem{}, notFound(sql.ErrNoRows, appfinance.ErrNotFound, "cost item "+id)
	}
	return out[0], nil
}

// ListItems implements appfinance.Repository.
func (r *FinanceRepository) ListItems(ctx context.Context, before string, limit int) ([]finance.CostItem, error) {
	if limit <= 0 {
		return []finance.CostItem{}, nil
	}
	return r.items(ctx, "", before, int64(limit))
}

// AllItems implements appfinance.Repository.
func (r *FinanceRepository) AllItems(ctx context.Context) ([]finance.CostItem, error) {
	return r.items(ctx, "", "", math.MaxInt64)
}

func (r *FinanceRepository) items(ctx context.Context, id, before string, limit int64) ([]finance.CostItem, error) {
	q := r.db.reader()
	rows, err := q.ListCostItems(ctx, sqlitegen.ListCostItemsParams{ID: id, Before: before, Lim: limit})
	if err != nil {
		return nil, wrap(err, "list cost items")
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
			ProviderNormalized: row.ProviderNormalized, LabelRevisionID: row.LabelRevisionID, LabelRevisedAt: fromMillis(row.LabelRevisedAt),
			CreatedByUserID: row.CreatedByUserID.String, CreatedAt: fromMillis(row.CreatedAt),
		}
		ids[i] = row.ID
		byID[row.ID] = i
	}
	if len(rows) == 0 {
		return out, nil
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return nil, wrap(err, "encode cost item ids")
	}
	prices, err := q.ListCostItemPriceRevisions(ctx, string(idsJSON))
	if err != nil {
		return nil, wrap(err, "list cost item price revisions")
	}
	for _, p := range prices {
		on, err := fromDate(p.EffectiveOn)
		if err != nil {
			return nil, err
		}
		i := byID[p.CostItemID]
		out[i].Prices = append(out[i].Prices, finance.PriceRevision{
			ID: p.ID, EffectiveOn: on, Price: finance.Money{Minor: p.AmountMinor, Currency: finance.Currency(p.Currency)},
			RevisedAt: fromMillis(p.RevisedAt),
		})
	}
	return out, nil
}

// RebuildHeads implements appfinance.Repository.
func (r *FinanceRepository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild cost item heads",
		(*sqlitegen.Queries).DeleteAllCostItemHeads, (*sqlitegen.Queries).RebuildCostItemHeads)
}
