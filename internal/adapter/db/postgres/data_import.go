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

// InsertDataImport implements identity.ImportRepository.
func (r *IdentityRepository) InsertDataImport(ctx context.Context, tx uow.Tx, i identity.DataImport) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertDataImport(ctx, postgresgen.InsertDataImportParams{
		ID: i.ID, UserID: i.UserID, AccountID: i.AccountID, Kind: string(i.Kind), Mode: string(i.Mode), CreatedAt: i.CreatedAt,
	}), "insert data import")
}

// InsertDataImportItems implements identity.ImportRepository: the items of each import in one
// statement.
func (r *IdentityRepository) InsertDataImportItems(ctx context.Context, tx uow.Tx, items []identity.DataImportItem) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	byImport := map[string]*postgresgen.InsertDataImportItemsParams{}
	var order []string
	for _, it := range items {
		p := byImport[it.ImportID]
		if p == nil {
			p = &postgresgen.InsertDataImportItemsParams{ImportID: it.ImportID}
			byImport[it.ImportID] = p
			order = append(order, it.ImportID)
		}
		pos, err := toInt32(it.Position, "position")
		if err != nil {
			return err
		}
		p.Positions = append(p.Positions, pos)
		p.Actions = append(p.Actions, string(it.Action))
		p.Targets = append(p.Targets, it.Target)
		p.ShowBoosts = append(p.ShowBoosts, it.ShowBoosts)
		p.Notifies = append(p.Notifies, it.Notify)
		p.HideNotifications = append(p.HideNotifications, it.HideNotifications)
	}
	for _, id := range order {
		if err := q.InsertDataImportItems(ctx, *byImport[id]); err != nil {
			return wrap(err, "insert data import items")
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
	pos, err := toInt32(res.Position, "position")
	if err != nil {
		return err
	}
	return wrap(q.InsertDataImportItemResult(ctx, postgresgen.InsertDataImportItemResultParams{
		ImportID: res.ImportID, Position: pos, Outcome: string(res.Outcome),
		Failure: toNullString(string(res.Failure)), ProcessedAt: res.ProcessedAt,
	}), "insert data import result")
}

// FindDataImport implements identity.ImportRepository.
func (r *IdentityRepository) FindDataImport(ctx context.Context, id string) (identity.DataImportProgress, error) {
	row, err := r.db.reader().GetDataImportProgress(ctx, id)
	if err != nil {
		return identity.DataImportProgress{}, notFound(err, appidentity.ErrNotFound, "data import")
	}
	return dataImportProgress(postgresgen.ListDataImportProgressRow(row)), nil
}

// ListDataImports implements identity.ImportRepository.
func (r *IdentityRepository) ListDataImports(ctx context.Context, userID string, limit int) ([]identity.DataImportProgress, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDataImportProgress(ctx, postgresgen.ListDataImportProgressParams{UserID: userID, BatchLimit: n})
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
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListPendingDataImportItems(ctx, postgresgen.ListPendingDataImportItemsParams{ImportID: importID, BatchLimit: n})
	if err != nil {
		return nil, wrap(err, "list pending data import items")
	}
	out := make([]identity.DataImportItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, identity.DataImportItem{
			ImportID: row.ImportID, Position: int(row.Position), Action: identity.ImportAction(row.Action), Target: row.Target,
			ShowBoosts: row.ShowBoosts, Notify: row.Notify, HideNotifications: row.HideNotifications,
		})
	}
	return out, nil
}

// ListDataImportsCreatedBefore implements identity.ImportRepository.
func (r *IdentityRepository) ListDataImportsCreatedBefore(ctx context.Context, t time.Time, limit int) ([]string, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListDataImportsCreatedBefore(ctx, postgresgen.ListDataImportsCreatedBeforeParams{CreatedBefore: t, BatchLimit: n})
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

func dataImportProgress(row postgresgen.ListDataImportProgressRow) identity.DataImportProgress {
	return identity.DataImportProgress{
		DataImport: identity.DataImport{
			ID: row.ID, UserID: row.UserID, AccountID: row.AccountID, Kind: identity.ImportKind(row.Kind),
			Mode: identity.ImportMode(row.Mode), CreatedAt: utc(row.CreatedAt),
		},
		Total: int(row.Total), Processed: int(row.Processed), Failed: int(row.Failed),
	}
}
