// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	err = q.InsertDataExport(ctx, postgresgen.InsertDataExportParams{ID: e.ID, UserID: e.UserID, RequestedAt: e.RequestedAt})
	return wrap(err, "insert data export")
}

// InsertDataExportResult implements identity.ExportRepository.
func (r *IdentityRepository) InsertDataExportResult(ctx context.Context, tx uow.Tx, res identity.DataExportResult) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	p := postgresgen.InsertDataExportResultParams{
		ExportID: res.ExportID, Outcome: string(res.Outcome), BlobKey: toNullString(res.BlobKey),
		ExpiresAt: res.ExpiresAt, CompletedAt: res.CompletedAt,
	}
	if res.Outcome == identity.ExportReady {
		size := res.FileSize
		p.FileSize = &size
	}
	return wrap(q.InsertDataExportResult(ctx, p), "insert data export result")
}

// FindDataExport implements identity.ExportRepository.
func (r *IdentityRepository) FindDataExport(ctx context.Context, id string) (identity.DataExport, error) {
	row, err := r.db.reader().GetDataExport(ctx, id)
	if err != nil {
		return identity.DataExport{}, notFound(err, appidentity.ErrNotFound, "data export")
	}
	return dataExport(postgresgen.ListDataExportsRow(row)), nil
}

// ListDataExports implements identity.ExportRepository.
func (r *IdentityRepository) ListDataExports(ctx context.Context, userID string, limit int) ([]identity.DataExport, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDataExports(ctx, postgresgen.ListDataExportsParams{UserID: userID, BatchLimit: n})
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
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListExpiredDataExports(ctx, postgresgen.ListExpiredDataExportsParams{ExpiresBefore: t, BatchLimit: n})
	if err != nil {
		return nil, wrap(err, "list expired data exports")
	}
	out := make([]identity.DataExport, 0, len(rows))
	for _, row := range rows {
		outcome, expires, completed := row.Outcome, row.ExpiresAt, row.CompletedAt
		out = append(out, dataExport(postgresgen.ListDataExportsRow{
			ID: row.ID, UserID: row.UserID, RequestedAt: row.RequestedAt, Outcome: &outcome, BlobKey: row.BlobKey,
			FileSize: row.FileSize, ExpiresAt: &expires, CompletedAt: &completed,
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

func dataExport(row postgresgen.ListDataExportsRow) identity.DataExport {
	e := identity.DataExport{ID: row.ID, UserID: row.UserID, RequestedAt: utc(row.RequestedAt)}
	if row.Outcome != nil && row.ExpiresAt != nil && row.CompletedAt != nil {
		res := &identity.DataExportResult{
			ExportID: row.ID, Outcome: identity.ExportOutcome(*row.Outcome), BlobKey: fromNullString(row.BlobKey),
			ExpiresAt: utc(*row.ExpiresAt), CompletedAt: utc(*row.CompletedAt),
		}
		if row.FileSize != nil {
			res.FileSize = *row.FileSize
		}
		e.Result = res
	}
	return e
}
