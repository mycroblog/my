// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// The income records (data-model §4.12, ADR-0073 §8).

func toNullRange(r *finance.DateRange) (sql.NullString, sql.NullString) {
	if r == nil {
		return sql.NullString{}, sql.NullString{}
	}
	return toNullDate(&r.Start), toNullDate(&r.End)
}

func fromNullRange(start, end sql.NullString) (*finance.DateRange, error) {
	s, err := fromNullDate(start)
	if err != nil || s == nil {
		return nil, err
	}
	e, err := fromNullDate(end)
	if err != nil || e == nil {
		return nil, err
	}
	return &finance.DateRange{Start: *s, End: *e}, nil
}

func incomeMoney(gross, fee, net int64, currency string) (finance.Money, finance.Money, finance.Money) {
	c := finance.Currency(currency)
	return finance.Money{Minor: gross, Currency: c}, finance.Money{Minor: fee, Currency: c}, finance.Money{Minor: net, Currency: c}
}

// InsertIncome implements appfinance.Repository.
func (r *FinanceRepository) InsertIncome(ctx context.Context, tx uow.Tx, rec appfinance.IncomeRecord) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	starts, ends := toNullRange(rec.Covers)
	n, err := q.InsertIncomeMovement(ctx, sqlitegen.InsertIncomeMovementParams{
		ID: rec.ID, Provider: rec.Provider, ProviderID: rec.ProviderID, Kind: string(rec.Kind),
		ParentMovementID: toNullString(rec.ParentMovementID), SubscriptionID: toNullString(rec.SubscriptionID),
		OccurredAt: toMillis(rec.OccurredAt), OccurredOn: rec.OccurredOn.String(), CoversStartsOn: starts, CoversEndsOn: ends,
		GrossMinor: rec.Gross.Minor, FeeMinor: rec.Fee.Minor, NetMinor: rec.Net.Minor, Currency: string(rec.Net.Currency),
		RecordedAt: toMillis(rec.RecordedAt),
	})
	return n > 0, wrap(err, "insert income movement")
}

// FindIncomeID implements appfinance.Repository.
func (r *FinanceRepository) FindIncomeID(ctx context.Context, tx uow.Tx, provider, providerID string) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.FindIncomeMovementID(ctx, sqlitegen.FindIncomeMovementIDParams{Provider: provider, ProviderID: providerID})
	if err != nil {
		return "", notFound(err, appfinance.ErrNotFound, "income movement "+providerID)
	}
	return id, nil
}

// KnownIncome implements appfinance.Repository.
func (r *FinanceRepository) KnownIncome(ctx context.Context, provider string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().KnownIncomeMovements(ctx, sqlitegen.KnownIncomeMovementsParams{Provider: provider, Ids: list})
	return out, wrap(err, "list known income movements")
}

// IncomeInWindow implements appfinance.Repository.
func (r *FinanceRepository) IncomeInWindow(ctx context.Context, window finance.DateRange) ([]finance.IncomeMovement, error) {
	rows, err := r.db.reader().ListIncomeMovementsInWindow(ctx, sqlitegen.ListIncomeMovementsInWindowParams{
		StartsOn: window.Start.String(), EndsOn: window.End.String(),
	})
	if err != nil {
		return nil, wrap(err, "list income movements in a window")
	}
	out := make([]finance.IncomeMovement, len(rows))
	for i, row := range rows {
		on, err := fromDate(row.OccurredOn)
		if err != nil {
			return nil, err
		}
		covers, err := fromNullRange(row.CoversStartsOn, row.CoversEndsOn)
		if err != nil {
			return nil, err
		}
		parent, err := fromNullRange(row.ParentCoversStartsOn, row.ParentCoversEndsOn)
		if err != nil {
			return nil, err
		}
		gross, fee, net := incomeMoney(row.GrossMinor, row.FeeMinor, row.NetMinor, row.Currency)
		out[i] = finance.IncomeMovement{
			Kind: finance.IncomeKind(row.Kind), OccurredOn: on, Covers: covers, ParentCovers: parent, Gross: gross, Fee: fee, Net: net,
		}
	}
	return out, nil
}

// ListIncome implements appfinance.Repository.
func (r *FinanceRepository) ListIncome(ctx context.Context, f appfinance.IncomeFilter, before string, limit int) ([]appfinance.IncomeRecord, error) {
	rows, err := r.db.reader().ListIncomeMovements(ctx, sqlitegen.ListIncomeMovementsParams{
		StartsOn: f.From.String(), EndsOn: f.To.String(), Kind: string(f.Kind), Before: before, Lim: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list income movements")
	}
	out := make([]appfinance.IncomeRecord, len(rows))
	for i, row := range rows {
		on, err := fromDate(row.OccurredOn)
		if err != nil {
			return nil, err
		}
		covers, err := fromNullRange(row.CoversStartsOn, row.CoversEndsOn)
		if err != nil {
			return nil, err
		}
		gross, fee, net := incomeMoney(row.GrossMinor, row.FeeMinor, row.NetMinor, row.Currency)
		out[i] = appfinance.IncomeRecord{
			ID: row.ID, Provider: row.Provider, ProviderID: row.ProviderID, Kind: finance.IncomeKind(row.Kind),
			ParentProviderID: row.ParentProviderID.String, SubscriptionID: row.SubscriptionID.String,
			OccurredAt: fromMillis(row.OccurredAt), OccurredOn: on, Covers: covers, Gross: gross, Fee: fee, Net: net,
			RecordedAt: fromMillis(row.RecordedAt),
		}
	}
	return out, nil
}
