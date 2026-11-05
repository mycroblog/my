// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/app/uow"
)

// SearchRepository implements search.Repository.
type SearchRepository struct{ db *DB }

var _ search.Repository = (*SearchRepository)(nil)

// SearchRepository returns the repository of the search module.
func (d *DB) SearchRepository() *SearchRepository { return &SearchRepository{db: d} }

// StartRebuild implements search.Repository.
func (r *SearchRepository) StartRebuild(ctx context.Context, tx uow.Tx, b search.Rebuild) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rate, err := toInt32(b.Rate, "rate")
	if err != nil {
		return false, err
	}
	n, err := q.StartSearchRebuild(ctx, postgresgen.StartSearchRebuildParams{
		RebuildID: b.ID, Mode: string(b.Mode), Phase: string(b.Phase), Rate: rate, StartedAt: b.StartedAt,
	})
	if err != nil {
		return false, wrap(err, "start search rebuild")
	}
	if n == 0 {
		return false, nil
	}
	if err := q.DeleteOtherSearchRebuildFailures(ctx, b.ID); err != nil {
		return false, wrap(err, "delete failures of earlier search rebuilds")
	}
	return true, nil
}

// FindRebuild implements search.Repository.
func (r *SearchRepository) FindRebuild(ctx context.Context) (search.Rebuild, bool, error) {
	row, err := r.db.reader().GetSearchRebuild(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return search.Rebuild{}, false, nil
	}
	if err != nil {
		return search.Rebuild{}, false, wrap(err, "get search rebuild")
	}
	return search.Rebuild{
		ID: row.RebuildID, Mode: search.RebuildMode(row.Mode), Phase: search.RebuildPhase(row.Phase), After: row.AfterStatusID,
		Rate: int(row.Rate), Processed: row.Processed, Failed: row.Failed, LastError: row.LastError,
		StartedAt: row.StartedAt, UpdatedAt: row.UpdatedAt,
	}, true, nil
}

// StepRebuild implements search.Repository.
func (r *SearchRepository) StepRebuild(ctx context.Context, tx uow.Tx, step search.RebuildStep) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.StepSearchRebuild(ctx, postgresgen.StepSearchRebuildParams{
		Phase: string(step.Phase), AfterStatusID: step.After, Processed: step.Processed, Failed: step.Failed,
		LastError: step.LastError, UpdatedAt: step.UpdatedAt,
		RebuildID: step.ID, FromPhase: string(step.FromPhase), FromAfter: step.FromAfter,
	})
	if err != nil {
		return false, wrap(err, "step search rebuild")
	}
	return n > 0, nil
}

// SetRebuildRate implements search.Repository.
func (r *SearchRepository) SetRebuildRate(ctx context.Context, tx uow.Tx, id string, rate int) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	pgRate, err := toInt32(rate, "rate")
	if err != nil {
		return err
	}
	return wrap(q.SetSearchRebuildRate(ctx, postgresgen.SetSearchRebuildRateParams{Rate: pgRate, RebuildID: id}), "set search rebuild rate")
}

// SetRebuildError implements search.Repository.
func (r *SearchRepository) SetRebuildError(ctx context.Context, tx uow.Tx, id, message string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.SetSearchRebuildError(ctx, postgresgen.SetSearchRebuildErrorParams{LastError: message, UpdatedAt: at, RebuildID: id}), "set search rebuild error")
}

// AddRebuildFailures implements search.Repository.
func (r *SearchRepository) AddRebuildFailures(ctx context.Context, tx uow.Tx, id string, failures []search.RebuildFailure) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, f := range failures {
		err := q.AddSearchRebuildFailure(ctx, postgresgen.AddSearchRebuildFailureParams{
			RebuildID: id, StatusID: f.StatusID, Message: f.Message, FailedAt: f.FailedAt,
		})
		if err != nil {
			return wrap(err, "add search rebuild failure")
		}
	}
	return nil
}

// ListRebuildFailures implements search.Repository.
func (r *SearchRepository) ListRebuildFailures(ctx context.Context, id string, limit int) ([]search.RebuildFailure, error) {
	pageLimit, err := toInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListSearchRebuildFailures(ctx, postgresgen.ListSearchRebuildFailuresParams{RebuildID: id, PageLimit: pageLimit})
	if err != nil {
		return nil, wrap(err, "list search rebuild failures")
	}
	out := make([]search.RebuildFailure, 0, len(rows))
	for _, row := range rows {
		out = append(out, search.RebuildFailure{StatusID: row.StatusID, Message: row.Message, FailedAt: row.FailedAt})
	}
	return out, nil
}

// DeleteRebuild implements search.Repository.
func (r *SearchRepository) DeleteRebuild(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteSearchRebuildFailures(ctx); err != nil {
		return wrap(err, "delete search rebuild failures")
	}
	return wrap(q.DeleteSearchRebuild(ctx), "delete search rebuild")
}
