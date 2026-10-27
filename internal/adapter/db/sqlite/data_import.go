// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertDataImport implements identity.ImportRepository.
func (r *IdentityRepository) InsertDataImport(ctx context.Context, tx uow.Tx, i identity.DataImport) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertDataImport(ctx, sqlitegen.InsertDataImportParams{
		ID: i.ID, UserID: i.UserID, AccountID: i.AccountID, Kind: string(i.Kind), Mode: string(i.Mode), CreatedAt: toMillis(i.CreatedAt),
	}), "insert data import")
}

// InsertDataImportItems implements identity.ImportRepository.
func (r *IdentityRepository) InsertDataImportItems(ctx context.Context, tx uow.Tx, items []identity.DataImportItem) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, it := range items {
		err := q.InsertDataImportItem(ctx, sqlitegen.InsertDataImportItemParams{
			ImportID: it.ImportID, Position: int64(it.Position), Action: string(it.Action), Target: it.Target,
			ShowBoosts: boolToInt(it.ShowBoosts), Notify: boolToInt(it.Notify), HideNotifications: boolToInt(it.HideNotifications),
		})
		if err != nil {
			return wrap(err, "insert data import item")
		}
	}
	return nil
}

// InsertDataImportResult implements identity.ImportRepository.
func (r *IdentityRepository) InsertDataImportResult(ctx context.Context, tx uow.Tx, res identity.DataImportResult) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertDataImportItemResult(ctx, sqlitegen.InsertDataImportItemResultParams{
		ImportID: res.ImportID, Position: int64(res.Position), Outcome: string(res.Outcome),
		Failure: toNullString(string(res.Failure)), ProcessedAt: toMillis(res.ProcessedAt),
	}), "insert data import result")
}

// FindDataImport implements identity.ImportRepository.
func (r *IdentityRepository) FindDataImport(ctx context.Context, id string) (identity.DataImportProgress, error) {
	row, err := r.db.reader().GetDataImportProgress(ctx, id)
	if err != nil {
		return identity.DataImportProgress{}, notFound(err, appidentity.ErrNotFound, "data import")
	}
	return dataImportProgress(sqlitegen.ListDataImportProgressRow(row)), nil
}

// ListDataImports implements identity.ImportRepository.
func (r *IdentityRepository) ListDataImports(ctx context.Context, userID string, limit int) ([]identity.DataImportProgress, error) {
	rows, err := r.db.reader().ListDataImportProgress(ctx, sqlitegen.ListDataImportProgressParams{UserID: userID, BatchLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list data imports")
	}
	out := make([]identity.DataImportProgress, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataImportProgress(row))
	}
	return out, nil
}

// ListPendingDataImportItems implements identity.ImportRepository.
func (r *IdentityRepository) ListPendingDataImportItems(ctx context.Context, importID string, limit int) ([]identity.DataImportItem, error) {
	rows, err := r.db.reader().ListPendingDataImportItems(ctx, sqlitegen.ListPendingDataImportItemsParams{ImportID: importID, BatchLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list pending data import items")
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
func (r *IdentityRepository) ListDataImportsCreatedBefore(ctx context.Context, t time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListDataImportsCreatedBefore(ctx, sqlitegen.ListDataImportsCreatedBeforeParams{
		CreatedBefore: toMillis(t), BatchLimit: int64(limit),
	})
	return ids, wrap(err, "list old data imports")
}

// DeleteDataImport implements identity.ImportRepository.
func (r *IdentityRepository) DeleteDataImport(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteDataImportItemResults(ctx, id); err != nil {
		return wrap(err, "delete data import results")
	}
	if err := q.DeleteDataImportItems(ctx, id); err != nil {
		return wrap(err, "delete data import items")
	}
	return wrap(q.DeleteDataImport(ctx, id), "delete data import")
}

func dataImportProgress(row sqlitegen.ListDataImportProgressRow) identity.DataImportProgress {
	return identity.DataImportProgress{
		DataImport: identity.DataImport{
			ID: row.ID, UserID: row.UserID, AccountID: row.AccountID, Kind: identity.ImportKind(row.Kind),
			Mode: identity.ImportMode(row.Mode), CreatedAt: fromMillis(row.CreatedAt),
		},
		Total: int(row.Total), Processed: int(row.Processed), Failed: int(row.Failed),
	}
}
