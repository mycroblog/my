// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertDataImport implements identity.ImportRepository.
func (r *Repository) InsertDataImport(ctx context.Context, tx uow.Tx, i identity.DataImport) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertDataImport(ctx, identitygen.InsertDataImportParams{
		ID: i.ID, UserID: i.UserID, AccountID: i.AccountID, Kind: string(i.Kind), Mode: string(i.Mode), CreatedAt: sqlite.ToMillis(i.CreatedAt),
	}), "insert data import")
}

// InsertDataImportItems implements identity.ImportRepository.
func (r *Repository) InsertDataImportItems(ctx context.Context, tx uow.Tx, items []identity.DataImportItem) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, it := range items {
		err := q.InsertDataImportItem(ctx, identitygen.InsertDataImportItemParams{
			ImportID: it.ImportID, Position: int64(it.Position), Action: string(it.Action), Target: it.Target,
			ShowBoosts: sqlite.BoolToInt(it.ShowBoosts), Notify: sqlite.BoolToInt(it.Notify), HideNotifications: sqlite.BoolToInt(it.HideNotifications),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert data import item")
		}
	}
	return nil
}

// LockUnfinishedDataImports implements identity.ImportRepository. SQLite serializes write
// transactions (BEGIN IMMEDIATE), so reading inside tx is enough.
func (r *Repository) LockUnfinishedDataImports(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	unfinished, err := q.HasUnfinishedDataImport(ctx, userID)
	return unfinished, sqlite.Wrap(err, "find an unfinished data import")
}

// findDataImport fails with ErrNotFound when the import is gone. SQLite serializes write
// transactions, so an import that tx finds stays until tx ends.
func findDataImport(ctx context.Context, q *identitygen.Queries, importID string) error {
	present, err := q.DataImportExists(ctx, importID)
	if err != nil {
		return sqlite.Wrap(err, "find data import")
	}
	if !present {
		return fmt.Errorf("data import %s: %w", importID, appidentity.ErrNotFound)
	}
	return nil
}

// InsertDataImportItemFailure implements identity.ImportRepository.
func (r *Repository) InsertDataImportItemFailure(ctx context.Context, tx uow.Tx, importID string, position int, at time.Time) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	if err := findDataImport(ctx, q, importID); err != nil {
		return 0, err
	}
	n, err := q.InsertDataImportItemFailure(ctx, identitygen.InsertDataImportItemFailureParams{
		ImportID: importID, Position: int64(position), FailedAt: sqlite.ToMillis(at),
	})
	return int(n), sqlite.Wrap(err, "insert data import item failure")
}

// InsertDataImportResult implements identity.ImportRepository.
func (r *Repository) InsertDataImportResult(ctx context.Context, tx uow.Tx, res identity.DataImportResult) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := findDataImport(ctx, q, res.ImportID); err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertDataImportItemResult(ctx, identitygen.InsertDataImportItemResultParams{
		ImportID: res.ImportID, Position: int64(res.Position), Outcome: string(res.Outcome),
		Failure: sqlite.ToNullString(string(res.Failure)), ProcessedAt: sqlite.ToMillis(res.ProcessedAt),
	}), "insert data import result")
}

// FindDataImport implements identity.ImportRepository.
func (r *Repository) FindDataImport(ctx context.Context, id string) (identity.DataImportProgress, error) {
	row, err := r.db.reader().GetDataImportProgress(ctx, id)
	if err != nil {
		return identity.DataImportProgress{}, sqlite.NotFound(err, appidentity.ErrNotFound, "data import")
	}
	return dataImportProgress(identitygen.ListDataImportProgressRow(row)), nil
}

// ListDataImports implements identity.ImportRepository.
func (r *Repository) ListDataImports(ctx context.Context, userID string, limit int) ([]identity.DataImportProgress, error) {
	rows, err := r.db.reader().ListDataImportProgress(ctx, identitygen.ListDataImportProgressParams{UserID: userID, BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list data imports")
	}
	out := make([]identity.DataImportProgress, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataImportProgress(row))
	}
	return out, nil
}

// ListPendingDataImportItems implements identity.ImportRepository.
func (r *Repository) ListPendingDataImportItems(ctx context.Context, importID string, limit int) ([]identity.DataImportItem, error) {
	rows, err := r.db.reader().ListPendingDataImportItems(ctx, identitygen.ListPendingDataImportItemsParams{ImportID: importID, BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list pending data import items")
	}
	out := make([]identity.DataImportItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, identity.DataImportItem{
			ImportID: row.ImportID, Position: int(row.Position), Action: identity.ImportAction(row.Action), Target: row.Target,
			ShowBoosts: row.ShowBoosts != 0, Notify: row.Notify != 0, HideNotifications: row.HideNotifications != 0,
		})
	}
	return out, nil
}

// ListDataImportsCreatedBefore implements identity.ImportRepository.
func (r *Repository) ListDataImportsCreatedBefore(ctx context.Context, t time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListDataImportsCreatedBefore(ctx, identitygen.ListDataImportsCreatedBeforeParams{
		CreatedBefore: sqlite.ToMillis(t), BatchLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list old data imports")
}

// DeleteDataImport implements identity.ImportRepository.
func (r *Repository) DeleteDataImport(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteDataImport(ctx, id); err != nil {
		return sqlite.Wrap(err, "delete data import")
	}
	if err := q.DeleteDataImportItemResults(ctx, id); err != nil {
		return sqlite.Wrap(err, "delete data import results")
	}
	if err := q.DeleteDataImportItemFailures(ctx, id); err != nil {
		return sqlite.Wrap(err, "delete data import item failures")
	}
	return sqlite.Wrap(q.DeleteDataImportItems(ctx, id), "delete data import items")
}

func dataImportProgress(row identitygen.ListDataImportProgressRow) identity.DataImportProgress {
	return identity.DataImportProgress{
		DataImport: identity.DataImport{
			ID: row.ID, UserID: row.UserID, AccountID: row.AccountID, Kind: identity.ImportKind(row.Kind),
			Mode: identity.ImportMode(row.Mode), CreatedAt: sqlite.FromMillis(row.CreatedAt),
		},
		Total: int(row.Total), Processed: int(row.Processed), Failed: int(row.Failed),
	}
}
