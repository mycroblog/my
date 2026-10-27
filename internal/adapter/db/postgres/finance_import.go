// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	requests, err := toInt32(run.Requests, "billing import requests")
	if err != nil {
		return err
	}
	return wrap(q.InsertBillingImportRun(ctx, postgresgen.InsertBillingImportRunParams{
		ID: run.ID, Source: run.Source, RunKey: run.RunKey, Outcome: runOutcome(run), ErrorKind: string(run.ErrorKind),
		Requests: requests, BytesBilled: run.BytesBilled, OccurredAt: run.OccurredAt,
	}), "insert billing import run")
}

// SumRuns implements appfinance.Repository.
func (r *FinanceRepository) SumRuns(ctx context.Context, source string, from, until time.Time) (int, int64, error) {
	row, err := r.db.reader().SumBillingImportRuns(ctx, postgresgen.SumBillingImportRunsParams{Source: source, FromAt: from, UntilAt: until})
	if err != nil {
		return 0, 0, wrap(err, "sum billing import runs")
	}
	return int(row.Requests), row.BytesBilled, nil
}

// RunKeyUsed implements appfinance.Repository.
func (r *FinanceRepository) RunKeyUsed(ctx context.Context, source, runKey string) (bool, error) {
	n, err := r.db.reader().CountBillingImportRunsByKey(ctx, postgresgen.CountBillingImportRunsByKeyParams{Source: source, RunKey: runKey})
	return n > 0, wrap(err, "count billing import runs")
}

// LastRun implements appfinance.Repository.
func (r *FinanceRepository) LastRun(ctx context.Context, source string, succeeded bool) (appfinance.Run, error) {
	row, err := r.db.reader().LastBillingImportRun(ctx, postgresgen.LastBillingImportRunParams{Source: source, IncludeFailed: !succeeded})
	if err != nil {
		return appfinance.Run{}, notFound(err, appfinance.ErrNotFound, "billing import run of "+source)
	}
	return appfinance.Run{
		ID: row.ID, Source: row.Source, RunKey: row.RunKey, ErrorKind: appfinance.ImportErrorKind(row.ErrorKind),
		Requests: int(row.Requests), BytesBilled: row.BytesBilled, OccurredAt: row.OccurredAt.UTC(),
	}, nil
}

func monthStrings(months []finance.YearMonth) []string {
	out := make([]string, len(months))
	for i, m := range months {
		out[i] = m.String()
	}
	return out
}

func importedHeads(ctx context.Context, q *postgresgen.Queries, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	rows, err := q.ListImportedCostHeads(ctx, postgresgen.ListImportedCostHeadsParams{Source: source, Months: monthStrings(months)})
	if err != nil {
		return nil, wrap(err, "list imported cost heads")
	}
	out := make([]appfinance.ImportedRevision, len(rows))
	for i, row := range rows {
		month, err := finance.ParseYearMonth(row.BillingMonth)
		if err != nil {
			return nil, fmt.Errorf("postgres: read billing month: %w", err)
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
			Kind:   finance.RevisionKind(row.Kind), RunID: row.RunID, RunKey: row.RunKey, RecordedAt: row.RecordedAt.UTC(),
		}
	}
	return out, nil
}

// ImportedHeads implements appfinance.Repository.
func (r *FinanceRepository) ImportedHeads(ctx context.Context, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	return importedHeads(ctx, r.db.reader(), source, months)
}

// ImportedHeadsForUpdate implements appfinance.Repository: it locks the heads first.
func (r *FinanceRepository) ImportedHeadsForUpdate(ctx context.Context, tx uow.Tx, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	if len(months) == 0 {
		return []appfinance.ImportedRevision{}, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	if err := q.LockImportedCostHeads(ctx, postgresgen.LockImportedCostHeadsParams{Source: source, Months: monthStrings(months)}); err != nil {
		return nil, wrap(err, "lock imported cost heads")
	}
	return importedHeads(ctx, q, source, months)
}

// InsertImportedRevision implements appfinance.Repository.
func (r *FinanceRepository) InsertImportedRevision(ctx context.Context, tx uow.Tx, rev appfinance.ImportedRevision) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertImportedCostRevision(ctx, postgresgen.InsertImportedCostRevisionParams{
		ID: rev.ID, Source: rev.Key.Source, BillingMonth: rev.Key.Month.String(), Service: rev.Key.Service,
		ChargeType: string(rev.Key.ChargeType), Category: string(rev.Category), StartsOn: toDate(rev.Covered.Start),
		EndsOn: toDate(rev.Covered.End), AmountMinor: rev.Amount.Minor, Currency: string(rev.Amount.Currency),
		Kind: string(rev.Kind), RunID: rev.RunID, RunKey: rev.RunKey, RecordedAt: rev.RecordedAt,
	})
	return n > 0, wrap(err, "insert imported cost revision")
}

// UpsertImportedHead implements appfinance.Repository.
func (r *FinanceRepository) UpsertImportedHead(ctx context.Context, tx uow.Tx, rev appfinance.ImportedRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertImportedCostHead(ctx, postgresgen.UpsertImportedCostHeadParams{
		Source: rev.Key.Source, BillingMonth: rev.Key.Month.String(), Service: rev.Key.Service,
		ChargeType: string(rev.Key.ChargeType), RevisionID: rev.ID,
	}), "upsert imported cost head")
}

// RebuildImportedHeads implements appfinance.Repository.
func (r *FinanceRepository) RebuildImportedHeads(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild imported cost heads",
		(*postgresgen.Queries).DeleteAllImportedCostHeads, (*postgresgen.Queries).RebuildImportedCostHeads)
}
