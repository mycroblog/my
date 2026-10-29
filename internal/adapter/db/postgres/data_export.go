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

// LockPendingDataExports implements identity.ExportRepository with SELECT ... FOR UPDATE on the
// user's row, which the requests of the user's exports and imports lock first.
func (r *IdentityRepository) LockPendingDataExports(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	if err := q.LockUserDataTransfers(ctx, userID); err != nil {
		return false, wrap(err, "lock the data transfers of a user")
	}
	pending, err := q.HasPendingDataExport(ctx, userID)
	return pending, wrap(err, "find a pending data export")
}

// InsertDataExportResult implements identity.ExportRepository: SELECT ... FOR KEY SHARE on the
// export keeps DeleteDataExport, which deletes the export's row first, from deleting it until tx
// ends.
func (r *IdentityRepository) InsertDataExportResult(ctx context.Context, tx uow.Tx, res identity.DataExportResult) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	if _, err := q.LockDataExport(ctx, res.ExportID); err != nil {
		return false, notFound(err, appidentity.ErrNotFound, "data export")
	}
	p := postgresgen.InsertDataExportResultParams{
		ExportID: res.ExportID, Outcome: string(res.Outcome), BlobKey: toNullString(res.BlobKey),
		ExpiresAt: res.ExpiresAt, CompletedAt: res.CompletedAt,
	}
	if res.Outcome == identity.ExportReady {
		size := res.FileSize
		p.FileSize = &size
	}
	n, err := q.InsertDataExportResult(ctx, p)
	return n > 0, wrap(err, "insert data export result")
}

// ListStalePendingDataExports implements identity.ExportRepository.
func (r *IdentityRepository) ListStalePendingDataExports(ctx context.Context, t time.Time, limit int) ([]string, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListStalePendingDataExports(ctx, postgresgen.ListStalePendingDataExportsParams{RequestedBefore: t, BatchLimit: n})
	return ids, wrap(err, "list stale pending data exports")
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

// DeleteDataExport implements identity.ExportRepository. It deletes the export's row first, so
// that it waits for a transaction that locked the row to record a result, and the deletion of
// the result, a statement of its own, sees that result.
func (r *IdentityRepository) DeleteDataExport(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteDataExport(ctx, id); err != nil {
		return wrap(err, "delete data export")
	}
	return wrap(q.DeleteDataExportResult(ctx, id), "delete data export result")
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
