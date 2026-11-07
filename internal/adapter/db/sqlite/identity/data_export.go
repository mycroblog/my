// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertDataExport implements identity.ExportRepository.
func (r *Repository) InsertDataExport(ctx context.Context, tx uow.Tx, e identity.DataExport) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertDataExport(ctx, identitygen.InsertDataExportParams{ID: e.ID, UserID: e.UserID, RequestedAt: sqlite.ToMillis(e.RequestedAt)})
	return sqlite.Wrap(err, "insert data export")
}

// LockPendingDataExports implements identity.ExportRepository. SQLite serializes write
// transactions (BEGIN IMMEDIATE), so reading inside tx is enough.
func (r *Repository) LockPendingDataExports(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	pending, err := q.HasPendingDataExport(ctx, userID)
	return pending, sqlite.Wrap(err, "find a pending data export")
}

// InsertDataExportResult implements identity.ExportRepository. SQLite serializes write
// transactions, so the export that tx finds stays until tx ends.
func (r *Repository) InsertDataExportResult(ctx context.Context, tx uow.Tx, res identity.DataExportResult) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	present, err := q.DataExportExists(ctx, res.ExportID)
	if err != nil {
		return false, sqlite.Wrap(err, "find data export")
	}
	if !present {
		return false, fmt.Errorf("data export %s: %w", res.ExportID, appidentity.ErrNotFound)
	}
	p := identitygen.InsertDataExportResultParams{
		ExportID: res.ExportID, Outcome: string(res.Outcome), BlobKey: sqlite.ToNullString(res.BlobKey),
		ExpiresAt: sqlite.ToMillis(res.ExpiresAt), CompletedAt: sqlite.ToMillis(res.CompletedAt),
	}
	if res.Outcome == identity.ExportReady {
		p.FileSize = sql.NullInt64{Int64: res.FileSize, Valid: true}
	}
	n, err := q.InsertDataExportResult(ctx, p)
	return n > 0, sqlite.Wrap(err, "insert data export result")
}

// ListStalePendingDataExports implements identity.ExportRepository.
func (r *Repository) ListStalePendingDataExports(ctx context.Context, t time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListStalePendingDataExports(ctx, identitygen.ListStalePendingDataExportsParams{
		RequestedBefore: sqlite.ToMillis(t), BatchLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list stale pending data exports")
}

// FindDataExport implements identity.ExportRepository.
func (r *Repository) FindDataExport(ctx context.Context, id string) (identity.DataExport, error) {
	row, err := r.db.reader().GetDataExport(ctx, id)
	if err != nil {
		return identity.DataExport{}, sqlite.NotFound(err, appidentity.ErrNotFound, "data export")
	}
	return dataExport(identitygen.ListDataExportsRow(row)), nil
}

// ListDataExports implements identity.ExportRepository.
func (r *Repository) ListDataExports(ctx context.Context, userID string, limit int) ([]identity.DataExport, error) {
	rows, err := r.db.reader().ListDataExports(ctx, identitygen.ListDataExportsParams{UserID: userID, BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list data exports")
	}
	out := make([]identity.DataExport, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataExport(row))
	}
	return out, nil
}

// ListExpiredDataExports implements identity.ExportRepository.
func (r *Repository) ListExpiredDataExports(ctx context.Context, t time.Time, limit int) ([]identity.DataExport, error) {
	rows, err := r.db.reader().ListExpiredDataExports(ctx, identitygen.ListExpiredDataExportsParams{
		ExpiresBefore: sqlite.ToMillis(t), BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list expired data exports")
	}
	out := make([]identity.DataExport, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataExport(identitygen.ListDataExportsRow{
			ID: row.ID, UserID: row.UserID, RequestedAt: row.RequestedAt,
			Outcome: sql.NullString{String: row.Outcome, Valid: true}, BlobKey: row.BlobKey, FileSize: row.FileSize,
			ExpiresAt: sql.NullInt64{Int64: row.ExpiresAt, Valid: true}, CompletedAt: sql.NullInt64{Int64: row.CompletedAt, Valid: true},
		}))
	}
	return out, nil
}

// DeleteDataExport implements identity.ExportRepository.
func (r *Repository) DeleteDataExport(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteDataExport(ctx, id); err != nil {
		return sqlite.Wrap(err, "delete data export")
	}
	return sqlite.Wrap(q.DeleteDataExportResult(ctx, id), "delete data export result")
}

func dataExport(row identitygen.ListDataExportsRow) identity.DataExport {
	e := identity.DataExport{ID: row.ID, UserID: row.UserID, RequestedAt: sqlite.FromMillis(row.RequestedAt)}
	if row.Outcome.Valid {
		e.Result = &identity.DataExportResult{
			ExportID: row.ID, Outcome: identity.ExportOutcome(row.Outcome.String), BlobKey: row.BlobKey.String,
			FileSize: row.FileSize.Int64, ExpiresAt: sqlite.FromMillis(row.ExpiresAt.Int64), CompletedAt: sqlite.FromMillis(row.CompletedAt.Int64),
		}
	}
	return e
}
