// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// The billing imports (data-model §4.12, ADR-0068 §5).

func runOutcome(r appfinance.Run) string {
	if r.Succeeded() {
		return "succeeded"
	}
	return "failed"
}

// InsertRun implements appfinance.Repository.
func (r *FinanceRepository) InsertRun(ctx context.Context, tx uow.Tx, run appfinance.Run) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertBillingImportRun(ctx, sqlitegen.InsertBillingImportRunParams{
		ID: run.ID, Source: run.Source, RunKey: run.RunKey, Outcome: runOutcome(run), ErrorKind: string(run.ErrorKind),
		Requests: int64(run.Requests), BytesBilled: run.BytesBilled, OccurredAt: toMillis(run.OccurredAt),
	}), "insert billing import run")
}

// SumRuns implements appfinance.Repository.
func (r *FinanceRepository) SumRuns(ctx context.Context, source string, from, until time.Time) (int, int64, error) {
	row, err := r.db.reader().SumBillingImportRuns(ctx, sqlitegen.SumBillingImportRunsParams{
		Source: source, FromAt: toMillis(from), UntilAt: toMillis(until),
	})
	if err != nil {
		return 0, 0, wrap(err, "sum billing import runs")
	}
	return int(row.Requests), row.BytesBilled, nil
}

// SumRunsTx implements appfinance.Repository.
func (r *FinanceRepository) SumRunsTx(ctx context.Context, tx uow.Tx, source string, from, until time.Time) (int, int64, error) {
	q, err := queries(tx)
	if err != nil {
		return 0, 0, err
	}
	row, err := q.SumBillingImportRuns(ctx, sqlitegen.SumBillingImportRunsParams{Source: source, FromAt: toMillis(from), UntilAt: toMillis(until)})
	if err != nil {
		return 0, 0, wrap(err, "sum billing import runs")
	}
	return int(row.Requests), row.BytesBilled, nil
}

// LockSource implements appfinance.Repository. SQLite runs one write transaction at a time, so
// there is nothing to lock.
func (r *FinanceRepository) LockSource(context.Context, uow.Tx, string) error { return nil }

// InsertReservation implements appfinance.Repository.
func (r *FinanceRepository) InsertReservation(ctx context.Context, tx uow.Tx, res appfinance.Reservation) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertBillingImportReservation(ctx, sqlitegen.InsertBillingImportReservationParams{
		ID: res.ID, Source: res.Source, RunKey: res.RunKey, Requests: int64(res.Requests), ReservedAt: toMillis(res.ReservedAt),
	}), "insert billing import reservation")
}

// ImportRunning implements appfinance.Repository.
func (r *FinanceRepository) ImportRunning(ctx context.Context, tx uow.Tx, source string, since time.Time) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	v, err := q.PendingBillingImportReservation(ctx, sqlitegen.PendingBillingImportReservationParams{Source: source, Since: toMillis(since)})
	return v != 0, wrap(err, "find a pending billing import reservation")
}

// RunKeySucceeded implements appfinance.Repository.
func (r *FinanceRepository) RunKeySucceeded(ctx context.Context, tx uow.Tx, source, runKey string) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	v, err := q.BillingImportRunKeySucceeded(ctx, sqlitegen.BillingImportRunKeySucceededParams{Source: source, RunKey: runKey})
	return v != 0, wrap(err, "find a succeeded billing import run")
}

// RunKeyUsed implements appfinance.Repository.
func (r *FinanceRepository) RunKeyUsed(ctx context.Context, source, runKey string) (bool, error) {
	n, err := r.db.reader().CountBillingImportRunsByKey(ctx, sqlitegen.CountBillingImportRunsByKeyParams{Source: source, RunKey: runKey})
	return n > 0, wrap(err, "count billing import runs")
}

// LastRun implements appfinance.Repository.
func (r *FinanceRepository) LastRun(ctx context.Context, source string, succeeded bool) (appfinance.Run, error) {
	var includeFailed int64
	if !succeeded {
		includeFailed = 1
	}
	row, err := r.db.reader().LastBillingImportRun(ctx, sqlitegen.LastBillingImportRunParams{Source: source, IncludeFailed: includeFailed})
	if err != nil {
		return appfinance.Run{}, notFound(err, appfinance.ErrNotFound, "billing import run of "+source)
	}
	return appfinance.Run{
		ID: row.ID, Source: row.Source, RunKey: row.RunKey, ErrorKind: appfinance.ImportErrorKind(row.ErrorKind),
		Requests: int(row.Requests), BytesBilled: row.BytesBilled, OccurredAt: fromMillis(row.OccurredAt),
	}, nil
}

func monthsJSON(months []finance.YearMonth) (string, error) {
	list := make([]string, len(months))
	for i, m := range months {
		list[i] = m.String()
	}
	b, err := json.Marshal(list)
	return string(b), err
}

func importedHeads(ctx context.Context, q *sqlitegen.Queries, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	list, err := monthsJSON(months)
	if err != nil {
		return nil, wrap(err, "encode billing months")
	}
	rows, err := q.ListImportedCostHeads(ctx, sqlitegen.ListImportedCostHeadsParams{Source: source, Months: list})
	if err != nil {
		return nil, wrap(err, "list imported cost heads")
	}
	out := make([]appfinance.ImportedRevision, len(rows))
	for i, row := range rows {
		month, err := finance.ParseYearMonth(row.BillingMonth)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read billing month: %w", err)
		}
		starts, err := fromDate(row.StartsOn)
		if err != nil {
			return nil, err
		}
		ends, err := fromDate(row.EndsOn)
		if err != nil {
			return nil, err
		}
		out[i] = appfinance.ImportedRevision{
			ID: row.ID,
			Key: appfinance.ImportedKey{
				Source: row.Source, Month: month, Service: row.Service, ChargeType: finance.ChargeType(row.ChargeType),
			},
			Category: finance.Category(row.Category), Covered: finance.DateRange{Start: starts, End: ends},
			Amount: finance.Money{Minor: row.AmountMinor, Currency: finance.Currency(row.Currency)},
			Kind:   finance.RevisionKind(row.Kind), RunID: row.RunID, RunKey: row.RunKey, RecordedAt: fromMillis(row.RecordedAt),
		}
	}
	return out, nil
}

// ImportedHeads implements appfinance.Repository.
func (r *FinanceRepository) ImportedHeads(ctx context.Context, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	return importedHeads(ctx, r.db.reader(), source, months)
}

// ImportedHeadsForUpdate implements appfinance.Repository. A SQLite transaction holds the write
// lock, so the rows need no lock of their own.
func (r *FinanceRepository) ImportedHeadsForUpdate(ctx context.Context, tx uow.Tx, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	if len(months) == 0 {
		return []appfinance.ImportedRevision{}, nil
	}
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	return importedHeads(ctx, q, source, months)
}

// InsertImportedRevision implements appfinance.Repository.
func (r *FinanceRepository) InsertImportedRevision(ctx context.Context, tx uow.Tx, rev appfinance.ImportedRevision) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertImportedCostRevision(ctx, sqlitegen.InsertImportedCostRevisionParams{
		ID: rev.ID, Source: rev.Key.Source, BillingMonth: rev.Key.Month.String(), Service: rev.Key.Service,
		ChargeType: string(rev.Key.ChargeType), Category: string(rev.Category), StartsOn: rev.Covered.Start.String(),
		EndsOn: rev.Covered.End.String(), AmountMinor: rev.Amount.Minor, Currency: string(rev.Amount.Currency),
		Kind: string(rev.Kind), RunID: rev.RunID, RunKey: rev.RunKey, RecordedAt: toMillis(rev.RecordedAt),
	})
	return n > 0, wrap(err, "insert imported cost revision")
}

// UpsertImportedHead implements appfinance.Repository.
func (r *FinanceRepository) UpsertImportedHead(ctx context.Context, tx uow.Tx, rev appfinance.ImportedRevision) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertImportedCostHead(ctx, sqlitegen.UpsertImportedCostHeadParams{
		Source: rev.Key.Source, BillingMonth: rev.Key.Month.String(), Service: rev.Key.Service,
		ChargeType: string(rev.Key.ChargeType), RevisionID: rev.ID,
	}), "upsert imported cost head")
}

// RebuildImportedHeads implements appfinance.Repository.
func (r *FinanceRepository) RebuildImportedHeads(ctx context.Context, tx uow.Tx) error {
	return execAll(ctx, tx, "rebuild imported cost heads",
		(*sqlitegen.Queries).DeleteAllImportedCostHeads, (*sqlitegen.Queries).RebuildImportedCostHeads)
}
