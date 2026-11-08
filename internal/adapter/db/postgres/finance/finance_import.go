// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/financegen"
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
func (r *Repository) InsertRun(ctx context.Context, tx uow.Tx, run appfinance.Run) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	requests, err := postgres.ToInt32(run.Requests, "billing import requests")
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertBillingImportRun(ctx, financegen.InsertBillingImportRunParams{
		ID: run.ID, Source: run.Source, RunKey: run.RunKey, Outcome: runOutcome(run), ErrorKind: string(run.ErrorKind),
		Requests: requests, BytesBilled: run.BytesBilled, OccurredAt: run.OccurredAt,
	}), "insert billing import run")
}

// SumRuns implements appfinance.Repository.
func (r *Repository) SumRuns(ctx context.Context, source string, from, until time.Time) (int, int64, error) {
	row, err := r.db.reader().SumBillingImportRuns(ctx, financegen.SumBillingImportRunsParams{Source: source, FromAt: from, UntilAt: until})
	if err != nil {
		return 0, 0, postgres.Wrap(err, "sum billing import runs")
	}
	return int(row.Requests), row.BytesBilled, nil
}

// SumRunsTx implements appfinance.Repository.
func (r *Repository) SumRunsTx(ctx context.Context, tx uow.Tx, source string, from, until time.Time) (int, int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, 0, err
	}
	row, err := q.SumBillingImportRuns(ctx, financegen.SumBillingImportRunsParams{Source: source, FromAt: from, UntilAt: until})
	if err != nil {
		return 0, 0, postgres.Wrap(err, "sum billing import runs")
	}
	return int(row.Requests), row.BytesBilled, nil
}

// LockSource implements appfinance.Repository with a transaction-scoped advisory lock on the
// imports of the source.
func (r *Repository) LockSource(ctx context.Context, tx uow.Tx, source string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.LockBillingImportSource(ctx, "finance:billing_import:"+source), "lock billing import source")
}

// InsertReservation implements appfinance.Repository.
func (r *Repository) InsertReservation(ctx context.Context, tx uow.Tx, res appfinance.Reservation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	requests, err := postgres.ToInt32(res.Requests, "billing import reservation")
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertBillingImportReservation(ctx, financegen.InsertBillingImportReservationParams{
		ID: res.ID, Source: res.Source, Requests: requests, ReservedAt: res.ReservedAt,
	}), "insert billing import reservation")
}

// ImportRunning implements appfinance.Repository.
func (r *Repository) ImportRunning(ctx context.Context, tx uow.Tx, source string, since time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	v, err := q.PendingBillingImportReservation(ctx, financegen.PendingBillingImportReservationParams{Source: source, Since: since})
	return v, postgres.Wrap(err, "find a pending billing import reservation")
}

// RunKeySucceeded implements appfinance.Repository.
func (r *Repository) RunKeySucceeded(ctx context.Context, tx uow.Tx, source, runKey string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	v, err := q.BillingImportRunKeySucceeded(ctx, financegen.BillingImportRunKeySucceededParams{Source: source, RunKey: runKey})
	return v, postgres.Wrap(err, "find a succeeded billing import run")
}

// RunKeyUsed implements appfinance.Repository.
func (r *Repository) RunKeyUsed(ctx context.Context, source, runKey string) (bool, error) {
	n, err := r.db.reader().CountBillingImportRunsByKey(ctx, financegen.CountBillingImportRunsByKeyParams{Source: source, RunKey: runKey})
	return n > 0, postgres.Wrap(err, "count billing import runs")
}

// LastRun implements appfinance.Repository.
func (r *Repository) LastRun(ctx context.Context, source string, succeeded bool) (appfinance.Run, error) {
	row, err := r.db.reader().LastBillingImportRun(ctx, financegen.LastBillingImportRunParams{Source: source, IncludeFailed: !succeeded})
	if err != nil {
		return appfinance.Run{}, postgres.NotFound(err, appfinance.ErrNotFound, "billing import run of "+source)
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

func importedHeads(ctx context.Context, q *financegen.Queries, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	rows, err := q.ListImportedCostHeads(ctx, financegen.ListImportedCostHeadsParams{Source: source, Months: monthStrings(months)})
	if err != nil {
		return nil, postgres.Wrap(err, "list imported cost heads")
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
func (r *Repository) ImportedHeads(ctx context.Context, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	return importedHeads(ctx, r.db.reader(), source, months)
}

// ImportedHeadsForUpdate implements appfinance.Repository: it locks the heads first.
func (r *Repository) ImportedHeadsForUpdate(ctx context.Context, tx uow.Tx, source string, months []finance.YearMonth) ([]appfinance.ImportedRevision, error) {
	if len(months) == 0 {
		return []appfinance.ImportedRevision{}, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	if err := q.LockImportedCostHeads(ctx, financegen.LockImportedCostHeadsParams{Source: source, Months: monthStrings(months)}); err != nil {
		return nil, postgres.Wrap(err, "lock imported cost heads")
	}
	return importedHeads(ctx, q, source, months)
}

// InsertImportedRevision implements appfinance.Repository.
func (r *Repository) InsertImportedRevision(ctx context.Context, tx uow.Tx, rev appfinance.ImportedRevision) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertImportedCostRevision(ctx, financegen.InsertImportedCostRevisionParams{
		ID: rev.ID, Source: rev.Key.Source, BillingMonth: rev.Key.Month.String(), Service: rev.Key.Service,
		ChargeType: string(rev.Key.ChargeType), Category: string(rev.Category), StartsOn: toDate(rev.Covered.Start),
		EndsOn: toDate(rev.Covered.End), AmountMinor: rev.Amount.Minor, Currency: string(rev.Amount.Currency),
		Kind: string(rev.Kind), RunID: rev.RunID, RunKey: rev.RunKey, RecordedAt: rev.RecordedAt,
	})
	return n > 0, postgres.Wrap(err, "insert imported cost revision")
}

// UpsertImportedHead implements appfinance.Repository.
func (r *Repository) UpsertImportedHead(ctx context.Context, tx uow.Tx, rev appfinance.ImportedRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertImportedCostHead(ctx, financegen.UpsertImportedCostHeadParams{
		Source: rev.Key.Source, BillingMonth: rev.Key.Month.String(), Service: rev.Key.Service,
		ChargeType: string(rev.Key.ChargeType), RevisionID: rev.ID,
	}), "upsert imported cost head")
}

// RebuildImportedHeads implements appfinance.Repository.
func (r *Repository) RebuildImportedHeads(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild imported cost heads",
		(*financegen.Queries).DeleteAllImportedCostHeads, (*financegen.Queries).RebuildImportedCostHeads)
}
