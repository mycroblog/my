// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/searchgen"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/app/uow"
)

// Repository implements search.Repository.
type Repository struct{ db *moduleDB }

var _ search.Repository = (*Repository)(nil)

// NewRepository returns the repository of the search module.
func NewRepository(d *sqlite.DB) *Repository { return &Repository{db: &moduleDB{d}} }

// StartRebuild implements search.Repository.
func (r *Repository) StartRebuild(ctx context.Context, tx uow.Tx, b search.Rebuild) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.StartSearchRebuild(ctx, searchgen.StartSearchRebuildParams{
		RebuildID: b.ID, Mode: string(b.Mode), Phase: string(b.Phase), Rate: int64(b.Rate), StartedAt: sqlite.ToMillis(b.StartedAt),
	})
	if err != nil {
		return false, sqlite.Wrap(err, "start search rebuild")
	}
	if n == 0 {
		return false, nil
	}
	if err := q.DeleteOtherSearchRebuildFailures(ctx, b.ID); err != nil {
		return false, sqlite.Wrap(err, "delete failures of earlier search rebuilds")
	}
	return true, nil
}

// FindRebuild implements search.Repository.
func (r *Repository) FindRebuild(ctx context.Context) (search.Rebuild, bool, error) {
	row, err := r.db.reader().GetSearchRebuild(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return search.Rebuild{}, false, nil
	}
	if err != nil {
		return search.Rebuild{}, false, sqlite.Wrap(err, "get search rebuild")
	}
	return search.Rebuild{
		ID: row.RebuildID, Mode: search.RebuildMode(row.Mode), Phase: search.RebuildPhase(row.Phase), After: row.AfterStatusID,
		Rate: int(row.Rate), Processed: row.Processed, Failed: row.Failed, LastError: row.LastError,
		StartedAt: sqlite.FromMillis(row.StartedAt), UpdatedAt: sqlite.FromMillis(row.UpdatedAt),
	}, true, nil
}

// StepRebuild implements search.Repository.
func (r *Repository) StepRebuild(ctx context.Context, tx uow.Tx, step search.RebuildStep) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.StepSearchRebuild(ctx, searchgen.StepSearchRebuildParams{
		Phase: string(step.Phase), AfterStatusID: step.After, Processed: step.Processed, Failed: step.Failed,
		LastError: step.LastError, UpdatedAt: sqlite.ToMillis(step.UpdatedAt),
		RebuildID: step.ID, FromPhase: string(step.FromPhase), FromAfter: step.FromAfter,
	})
	if err != nil {
		return false, sqlite.Wrap(err, "step search rebuild")
	}
	return n > 0, nil
}

// SetRebuildRate implements search.Repository.
func (r *Repository) SetRebuildRate(ctx context.Context, tx uow.Tx, id string, rate int) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.SetSearchRebuildRate(ctx, searchgen.SetSearchRebuildRateParams{Rate: int64(rate), RebuildID: id}), "set search rebuild rate")
}

// SetRebuildError implements search.Repository.
func (r *Repository) SetRebuildError(ctx context.Context, tx uow.Tx, id, message string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.SetSearchRebuildError(ctx, searchgen.SetSearchRebuildErrorParams{LastError: message, UpdatedAt: sqlite.ToMillis(at), RebuildID: id}), "set search rebuild error")
}

// AddRebuildFailures implements search.Repository.
func (r *Repository) AddRebuildFailures(ctx context.Context, tx uow.Tx, id string, failures []search.RebuildFailure) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, f := range failures {
		err := q.AddSearchRebuildFailure(ctx, searchgen.AddSearchRebuildFailureParams{
			RebuildID: id, StatusID: f.StatusID, Message: f.Message, FailedAt: sqlite.ToMillis(f.FailedAt),
		})
		if err != nil {
			return sqlite.Wrap(err, "add search rebuild failure")
		}
	}
	return nil
}

// ListRebuildFailures implements search.Repository.
func (r *Repository) ListRebuildFailures(ctx context.Context, id string, limit int) ([]search.RebuildFailure, error) {
	rows, err := r.db.reader().ListSearchRebuildFailures(ctx, searchgen.ListSearchRebuildFailuresParams{RebuildID: id, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list search rebuild failures")
	}
	out := make([]search.RebuildFailure, 0, len(rows))
	for _, row := range rows {
		out = append(out, search.RebuildFailure{StatusID: row.StatusID, Message: row.Message, FailedAt: sqlite.FromMillis(row.FailedAt)})
	}
	return out, nil
}

// DeleteRebuild implements search.Repository.
func (r *Repository) DeleteRebuild(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteSearchRebuildFailures(ctx); err != nil {
		return sqlite.Wrap(err, "delete search rebuild failures")
	}
	return sqlite.Wrap(q.DeleteSearchRebuild(ctx), "delete search rebuild")
}
