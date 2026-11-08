// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
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
	err = q.InsertDataExport(ctx, identitygen.InsertDataExportParams{ID: e.ID, UserID: e.UserID, RequestedAt: e.RequestedAt})
	return postgres.Wrap(err, "insert data export")
}

// LockPendingDataExports implements identity.ExportRepository with SELECT ... FOR UPDATE on the
// user's row, which the requests of the user's exports and imports lock first.
func (r *Repository) LockPendingDataExports(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	if err := q.LockUserDataTransfers(ctx, userID); err != nil {
		return false, postgres.Wrap(err, "lock the data transfers of a user")
	}
	pending, err := q.HasPendingDataExport(ctx, userID)
	return pending, postgres.Wrap(err, "find a pending data export")
}

// InsertDataExportResult implements identity.ExportRepository: SELECT ... FOR KEY SHARE on the
// export keeps DeleteDataExport, which deletes the export's row first, from deleting it until tx
// ends.
func (r *Repository) InsertDataExportResult(ctx context.Context, tx uow.Tx, res identity.DataExportResult) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	if _, err := q.LockDataExport(ctx, res.ExportID); err != nil {
		return false, postgres.NotFound(err, appidentity.ErrNotFound, "data export")
	}
	p := identitygen.InsertDataExportResultParams{
		ExportID: res.ExportID, Outcome: string(res.Outcome), BlobKey: postgres.ToNullString(res.BlobKey),
		ExpiresAt: res.ExpiresAt, CompletedAt: res.CompletedAt,
	}
	if res.Outcome == identity.ExportReady {
		size := res.FileSize
		p.FileSize = &size
	}
	n, err := q.InsertDataExportResult(ctx, p)
	return n > 0, postgres.Wrap(err, "insert data export result")
}

// ListStalePendingDataExports implements identity.ExportRepository.
func (r *Repository) ListStalePendingDataExports(ctx context.Context, t time.Time, limit int) ([]string, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListStalePendingDataExports(ctx, identitygen.ListStalePendingDataExportsParams{RequestedBefore: t, BatchLimit: n})
	return ids, postgres.Wrap(err, "list stale pending data exports")
}

// FindDataExport implements identity.ExportRepository.
func (r *Repository) FindDataExport(ctx context.Context, id string) (identity.DataExport, error) {
	row, err := r.db.reader().GetDataExport(ctx, id)
	if err != nil {
		return identity.DataExport{}, postgres.NotFound(err, appidentity.ErrNotFound, "data export")
	}
	return dataExport(identitygen.ListDataExportsRow(row)), nil
}

// ListDataExports implements identity.ExportRepository.
func (r *Repository) ListDataExports(ctx context.Context, userID string, limit int) ([]identity.DataExport, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDataExports(ctx, identitygen.ListDataExportsParams{UserID: userID, BatchLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "list data exports")
	}
	out := make([]identity.DataExport, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataExport(row))
	}
	return out, nil
}

// ListExpiredDataExports implements identity.ExportRepository.
func (r *Repository) ListExpiredDataExports(ctx context.Context, t time.Time, limit int) ([]identity.DataExport, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListExpiredDataExports(ctx, identitygen.ListExpiredDataExportsParams{ExpiresBefore: t, BatchLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "list expired data exports")
	}
	out := make([]identity.DataExport, 0, len(rows))
	for _, row := range rows {
		outcome, expires, completed := row.Outcome, row.ExpiresAt, row.CompletedAt
		out = append(out, dataExport(identitygen.ListDataExportsRow{
			ID: row.ID, UserID: row.UserID, RequestedAt: row.RequestedAt, Outcome: &outcome, BlobKey: row.BlobKey,
			FileSize: row.FileSize, ExpiresAt: &expires, CompletedAt: &completed,
		}))
	}
	return out, nil
}

// DeleteDataExport implements identity.ExportRepository. It deletes the export's row first, so
// that it waits for a transaction that locked the row to record a result, and the deletion of
// the result, a statement of its own, sees that result.
func (r *Repository) DeleteDataExport(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteDataExport(ctx, id); err != nil {
		return postgres.Wrap(err, "delete data export")
	}
	return postgres.Wrap(q.DeleteDataExportResult(ctx, id), "delete data export result")
}

func dataExport(row identitygen.ListDataExportsRow) identity.DataExport {
	e := identity.DataExport{ID: row.ID, UserID: row.UserID, RequestedAt: postgres.UTC(row.RequestedAt)}
	if row.Outcome != nil && row.ExpiresAt != nil && row.CompletedAt != nil {
		res := &identity.DataExportResult{
			ExportID: row.ID, Outcome: identity.ExportOutcome(*row.Outcome), BlobKey: postgres.FromNullString(row.BlobKey),
			ExpiresAt: postgres.UTC(*row.ExpiresAt), CompletedAt: postgres.UTC(*row.CompletedAt),
		}
		if row.FileSize != nil {
			res.FileSize = *row.FileSize
		}
		e.Result = res
	}
	return e
}
