// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertDataExport implements identity.ExportRepository.
func (r *IdentityRepository) InsertDataExport(ctx context.Context, tx uow.Tx, e identity.DataExport) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertDataExport(ctx, sqlitegen.InsertDataExportParams{ID: e.ID, UserID: e.UserID, RequestedAt: toMillis(e.RequestedAt)})
	return wrap(err, "insert data export")
}

// InsertDataExportResult implements identity.ExportRepository.
func (r *IdentityRepository) InsertDataExportResult(ctx context.Context, tx uow.Tx, res identity.DataExportResult) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	p := sqlitegen.InsertDataExportResultParams{
		ExportID: res.ExportID, Outcome: string(res.Outcome), BlobKey: toNullString(res.BlobKey),
		ExpiresAt: toMillis(res.ExpiresAt), CompletedAt: toMillis(res.CompletedAt),
	}
	if res.Outcome == identity.ExportReady {
		p.FileSize = sql.NullInt64{Int64: res.FileSize, Valid: true}
	}
	return wrap(q.InsertDataExportResult(ctx, p), "insert data export result")
}

// FindDataExport implements identity.ExportRepository.
func (r *IdentityRepository) FindDataExport(ctx context.Context, id string) (identity.DataExport, error) {
	row, err := r.db.reader().GetDataExport(ctx, id)
	if err != nil {
		return identity.DataExport{}, notFound(err, appidentity.ErrNotFound, "data export")
	}
	return dataExport(sqlitegen.ListDataExportsRow(row)), nil
}

// ListDataExports implements identity.ExportRepository.
func (r *IdentityRepository) ListDataExports(ctx context.Context, userID string, limit int) ([]identity.DataExport, error) {
	rows, err := r.db.reader().ListDataExports(ctx, sqlitegen.ListDataExportsParams{UserID: userID, BatchLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list data exports")
	}
	out := make([]identity.DataExport, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataExport(row))
	}
	return out, nil
}

// ListExpiredDataExports implements identity.ExportRepository.
func (r *IdentityRepository) ListExpiredDataExports(ctx context.Context, t time.Time, limit int) ([]identity.DataExport, error) {
	rows, err := r.db.reader().ListExpiredDataExports(ctx, sqlitegen.ListExpiredDataExportsParams{
		ExpiresBefore: toMillis(t), BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list expired data exports")
	}
	out := make([]identity.DataExport, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataExport(sqlitegen.ListDataExportsRow{
			ID: row.ID, UserID: row.UserID, RequestedAt: row.RequestedAt,
			Outcome: sql.NullString{String: row.Outcome, Valid: true}, BlobKey: row.BlobKey, FileSize: row.FileSize,
			ExpiresAt: sql.NullInt64{Int64: row.ExpiresAt, Valid: true}, CompletedAt: sql.NullInt64{Int64: row.CompletedAt, Valid: true},
		}))
	}
	return out, nil
}

// DeleteDataExport implements identity.ExportRepository.
func (r *IdentityRepository) DeleteDataExport(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteDataExportResult(ctx, id); err != nil {
		return wrap(err, "delete data export result")
	}
	return wrap(q.DeleteDataExport(ctx, id), "delete data export")
}

func dataExport(row sqlitegen.ListDataExportsRow) identity.DataExport {
	e := identity.DataExport{ID: row.ID, UserID: row.UserID, RequestedAt: fromMillis(row.RequestedAt)}
	if row.Outcome.Valid {
		e.Result = &identity.DataExportResult{
			ExportID: row.ID, Outcome: identity.ExportOutcome(row.Outcome.String), BlobKey: row.BlobKey.String,
			FileSize: row.FileSize.Int64, ExpiresAt: fromMillis(row.ExpiresAt.Int64), CompletedAt: fromMillis(row.CompletedAt.Int64),
		}
	}
	return e
}
