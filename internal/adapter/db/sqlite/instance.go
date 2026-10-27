// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InstanceRepository implements instance.Repository on SQLite.
type InstanceRepository struct {
	db *DB
}

var _ appinstance.Repository = (*InstanceRepository)(nil)

// InstanceRepository returns the instance module's repository on d.
func (d *DB) InstanceRepository() *InstanceRepository { return &InstanceRepository{db: d} }

// InsertSettingRevision implements instance.Repository.
func (r *InstanceRepository) InsertSettingRevision(ctx context.Context, tx uow.Tx, rev instance.SettingRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceSettingRevision(ctx, sqlitegen.InsertInstanceSettingRevisionParams{
		ID: rev.ID, Key: string(rev.Key), Value: rev.Value,
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(rev.RevisedAt),
	})
	return wrap(err, "insert instance setting revision")
}

// CurrentSetting implements instance.Repository.
func (r *InstanceRepository) CurrentSetting(ctx context.Context, tx uow.Tx, key instance.SettingKey) (instance.CurrentSetting, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.CurrentSetting{}, err
	}
	row, err := q.GetInstanceSetting(ctx, string(key))
	if err != nil {
		return instance.CurrentSetting{}, notFound(err, appinstance.ErrNotFound, "instance setting "+string(key))
	}
	return currentSetting(row), nil
}

// UpsertCurrentSetting implements instance.Repository.
func (r *InstanceRepository) UpsertCurrentSetting(ctx context.Context, tx uow.Tx, c instance.CurrentSetting) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertInstanceSetting(ctx, sqlitegen.UpsertInstanceSettingParams{Key: string(c.Key), RevisionID: c.RevisionID, Value: c.Value})
	return wrap(err, "upsert instance setting")
}

// DeleteAllCurrentSettings implements instance.Repository.
func (r *InstanceRepository) DeleteAllCurrentSettings(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllInstanceSettings(ctx), "delete instance settings")
}

// ListSettingRevisions implements instance.Repository.
func (r *InstanceRepository) ListSettingRevisions(ctx context.Context, tx uow.Tx) ([]instance.SettingRevision, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceSettingRevisions(ctx)
	if err != nil {
		return nil, wrap(err, "list instance setting revisions")
	}
	return settingRevisions(rows), nil
}

// InsertTextRevision implements instance.Repository.
func (r *InstanceRepository) InsertTextRevision(ctx context.Context, tx uow.Tx, rev instance.TextRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceTextRevision(ctx, sqlitegen.InsertInstanceTextRevisionParams{
		ID: rev.ID, Key: string(rev.Key), Body: rev.Body,
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(rev.RevisedAt),
	})
	return wrap(err, "insert instance text revision")
}

// CurrentText implements instance.Repository.
func (r *InstanceRepository) CurrentText(ctx context.Context, tx uow.Tx, key instance.TextKey) (instance.CurrentText, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.CurrentText{}, err
	}
	row, err := q.GetInstanceText(ctx, string(key))
	if err != nil {
		return instance.CurrentText{}, notFound(err, appinstance.ErrNotFound, "instance text "+string(key))
	}
	return instance.CurrentText{Key: instance.TextKey(row.Key), RevisionID: row.RevisionID}, nil
}

// UpsertCurrentText implements instance.Repository.
func (r *InstanceRepository) UpsertCurrentText(ctx context.Context, tx uow.Tx, c instance.CurrentText) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertInstanceText(ctx, sqlitegen.UpsertInstanceTextParams{Key: string(c.Key), RevisionID: c.RevisionID})
	return wrap(err, "upsert instance text")
}

// DeleteAllCurrentTexts implements instance.Repository.
func (r *InstanceRepository) DeleteAllCurrentTexts(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllInstanceTexts(ctx), "delete instance texts")
}

// ListTextRevisionHeads implements instance.Repository.
func (r *InstanceRepository) ListTextRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.CurrentText, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceTextRevisionHeads(ctx)
	if err != nil {
		return nil, wrap(err, "list instance text revisions")
	}
	out := make([]instance.CurrentText, len(rows))
	for i, row := range rows {
		out[i] = instance.CurrentText{Key: instance.TextKey(row.Key), RevisionID: row.ID}
	}
	return out, nil
}

// ListCurrentSettings implements instance.Repository.
func (r *InstanceRepository) ListCurrentSettings(ctx context.Context) ([]instance.CurrentSetting, error) {
	rows, err := r.db.reader().ListInstanceSettings(ctx)
	if err != nil {
		return nil, wrap(err, "list instance settings")
	}
	out := make([]instance.CurrentSetting, len(rows))
	for i, row := range rows {
		out[i] = currentSetting(row)
	}
	return out, nil
}

// ListSettingHistory implements instance.Repository.
func (r *InstanceRepository) ListSettingHistory(ctx context.Context, key instance.SettingKey, limit int) ([]instance.SettingRevision, error) {
	rows, err := r.db.reader().ListInstanceSettingHistory(ctx, sqlitegen.ListInstanceSettingHistoryParams{Key: string(key), Limit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list instance setting history")
	}
	return settingRevisions(rows), nil
}

// FindCurrentText implements instance.Repository.
func (r *InstanceRepository) FindCurrentText(ctx context.Context, key instance.TextKey) (instance.Text, error) {
	row, err := r.db.reader().GetCurrentInstanceText(ctx, string(key))
	if err != nil {
		return instance.Text{}, notFound(err, appinstance.ErrNotFound, "instance text "+string(key))
	}
	return currentText(sqlitegen.ListAllCurrentInstanceTextsRow(row)), nil
}

// ListAllCurrentTexts implements instance.Repository.
func (r *InstanceRepository) ListAllCurrentTexts(ctx context.Context) ([]instance.Text, error) {
	rows, err := r.db.reader().ListAllCurrentInstanceTexts(ctx)
	if err != nil {
		return nil, wrap(err, "list instance texts")
	}
	out := make([]instance.Text, len(rows))
	for i, row := range rows {
		out[i] = currentText(row)
	}
	return out, nil
}

// ListTextHistory implements instance.Repository.
func (r *InstanceRepository) ListTextHistory(ctx context.Context, key instance.TextKey, limit int) ([]instance.TextRevision, error) {
	rows, err := r.db.reader().ListInstanceTextHistory(ctx, sqlitegen.ListInstanceTextHistoryParams{
		Key: string(key), Limit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list instance text history")
	}
	out := make([]instance.TextRevision, len(rows))
	for i, row := range rows {
		out[i] = instance.TextRevision{
			ID: row.ID, Key: instance.TextKey(row.Key), Body: row.Body,
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: fromMillis(row.RevisedAt),
		}
	}
	return out, nil
}

func currentSetting(row sqlitegen.InstanceSetting) instance.CurrentSetting {
	return instance.CurrentSetting{Key: instance.SettingKey(row.Key), RevisionID: row.RevisionID, Value: row.Value}
}

func currentText(row sqlitegen.ListAllCurrentInstanceTextsRow) instance.Text {
	return instance.Text{
		Key: instance.TextKey(row.Key), Body: row.Body,
		RevisionID: row.RevisionID, RevisedAt: fromMillis(row.RevisedAt),
	}
}

func settingRevisions(rows []sqlitegen.InstanceSettingRevision) []instance.SettingRevision {
	out := make([]instance.SettingRevision, len(rows))
	for i, row := range rows {
		out[i] = instance.SettingRevision{
			ID: row.ID, Key: instance.SettingKey(row.Key), Value: row.Value,
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: fromMillis(row.RevisedAt),
		}
	}
	return out
}

// IsSettingValueUsed implements instance.Repository.
func (r *InstanceRepository) IsSettingValueUsed(ctx context.Context, key instance.SettingKey, value string) (bool, error) {
	ok, err := r.db.reader().IsInstanceSettingValueUsed(ctx, sqlitegen.IsInstanceSettingValueUsedParams{Key: string(key), Value: value})
	return ok, wrap(err, "look up instance setting value")
}
