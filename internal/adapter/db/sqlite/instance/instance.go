// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// Repository implements instance.Repository on SQLite.
type Repository struct {
	db *moduleDB
}

var _ appinstance.Repository = (*Repository)(nil)

// NewRepository returns the instance module's repository on d.
func NewRepository(d *sqlite.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertSettingRevision implements instance.Repository.
func (r *Repository) InsertSettingRevision(ctx context.Context, tx uow.Tx, rev instance.SettingRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceSettingRevision(ctx, instancegen.InsertInstanceSettingRevisionParams{
		ID: rev.ID, Key: string(rev.Key), Value: rev.Value,
		RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID), RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	return sqlite.Wrap(err, "insert instance setting revision")
}

// CurrentSetting implements instance.Repository.
func (r *Repository) CurrentSetting(ctx context.Context, tx uow.Tx, key instance.SettingKey) (instance.CurrentSetting, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.CurrentSetting{}, err
	}
	row, err := q.GetInstanceSetting(ctx, string(key))
	if err != nil {
		return instance.CurrentSetting{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance setting "+string(key))
	}
	return currentSetting(row), nil
}

// UpsertCurrentSetting implements instance.Repository.
func (r *Repository) UpsertCurrentSetting(ctx context.Context, tx uow.Tx, c instance.CurrentSetting) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertInstanceSetting(ctx, instancegen.UpsertInstanceSettingParams{Key: string(c.Key), RevisionID: c.RevisionID, Value: c.Value})
	return sqlite.Wrap(err, "upsert instance setting")
}

// DeleteAllCurrentSettings implements instance.Repository.
func (r *Repository) DeleteAllCurrentSettings(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllInstanceSettings(ctx), "delete instance settings")
}

// ListSettingRevisions implements instance.Repository.
func (r *Repository) ListSettingRevisions(ctx context.Context, tx uow.Tx) ([]instance.SettingRevision, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceSettingRevisions(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance setting revisions")
	}
	return settingRevisions(rows), nil
}

// InsertTextRevision implements instance.Repository.
func (r *Repository) InsertTextRevision(ctx context.Context, tx uow.Tx, rev instance.TextRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceTextRevision(ctx, instancegen.InsertInstanceTextRevisionParams{
		ID: rev.ID, Key: string(rev.Key), Body: rev.Body,
		RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID), RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	return sqlite.Wrap(err, "insert instance text revision")
}

// CurrentText implements instance.Repository.
func (r *Repository) CurrentText(ctx context.Context, tx uow.Tx, key instance.TextKey) (instance.CurrentText, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.CurrentText{}, err
	}
	row, err := q.GetInstanceText(ctx, string(key))
	if err != nil {
		return instance.CurrentText{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance text "+string(key))
	}
	return instance.CurrentText{Key: instance.TextKey(row.Key), RevisionID: row.RevisionID}, nil
}

// UpsertCurrentText implements instance.Repository.
func (r *Repository) UpsertCurrentText(ctx context.Context, tx uow.Tx, c instance.CurrentText) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertInstanceText(ctx, instancegen.UpsertInstanceTextParams{Key: string(c.Key), RevisionID: c.RevisionID})
	return sqlite.Wrap(err, "upsert instance text")
}

// DeleteAllCurrentTexts implements instance.Repository.
func (r *Repository) DeleteAllCurrentTexts(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllInstanceTexts(ctx), "delete instance texts")
}

// ListTextRevisionHeads implements instance.Repository.
func (r *Repository) ListTextRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.CurrentText, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceTextRevisionHeads(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance text revisions")
	}
	out := make([]instance.CurrentText, len(rows))
	for i, row := range rows {
		out[i] = instance.CurrentText{Key: instance.TextKey(row.Key), RevisionID: row.ID}
	}
	return out, nil
}

// ListCurrentSettings implements instance.Repository.
func (r *Repository) ListCurrentSettings(ctx context.Context) ([]instance.CurrentSetting, error) {
	rows, err := r.db.reader().ListInstanceSettings(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance settings")
	}
	out := make([]instance.CurrentSetting, len(rows))
	for i, row := range rows {
		out[i] = currentSetting(row)
	}
	return out, nil
}

// ListSettingHistory implements instance.Repository.
func (r *Repository) ListSettingHistory(ctx context.Context, key instance.SettingKey, limit int) ([]instance.SettingRevision, error) {
	rows, err := r.db.reader().ListInstanceSettingHistory(ctx, instancegen.ListInstanceSettingHistoryParams{Key: string(key), Limit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance setting history")
	}
	return settingRevisions(rows), nil
}

// FindCurrentText implements instance.Repository.
func (r *Repository) FindCurrentText(ctx context.Context, key instance.TextKey) (instance.Text, error) {
	row, err := r.db.reader().GetCurrentInstanceText(ctx, string(key))
	if err != nil {
		return instance.Text{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance text "+string(key))
	}
	return currentText(instancegen.ListAllCurrentInstanceTextsRow(row)), nil
}

// ListAllCurrentTexts implements instance.Repository.
func (r *Repository) ListAllCurrentTexts(ctx context.Context) ([]instance.Text, error) {
	rows, err := r.db.reader().ListAllCurrentInstanceTexts(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance texts")
	}
	out := make([]instance.Text, len(rows))
	for i, row := range rows {
		out[i] = currentText(row)
	}
	return out, nil
}

// ListTextHistory implements instance.Repository.
func (r *Repository) ListTextHistory(ctx context.Context, key instance.TextKey, limit int) ([]instance.TextRevision, error) {
	rows, err := r.db.reader().ListInstanceTextHistory(ctx, instancegen.ListInstanceTextHistoryParams{
		Key: string(key), Limit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance text history")
	}
	out := make([]instance.TextRevision, len(rows))
	for i, row := range rows {
		out[i] = instance.TextRevision{
			ID: row.ID, Key: instance.TextKey(row.Key), Body: row.Body,
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: sqlite.FromMillis(row.RevisedAt),
		}
	}
	return out, nil
}

func currentSetting(row instancegen.InstanceSetting) instance.CurrentSetting {
	return instance.CurrentSetting{Key: instance.SettingKey(row.Key), RevisionID: row.RevisionID, Value: row.Value}
}

func currentText(row instancegen.ListAllCurrentInstanceTextsRow) instance.Text {
	return instance.Text{
		Key: instance.TextKey(row.Key), Body: row.Body,
		RevisionID: row.RevisionID, RevisedAt: sqlite.FromMillis(row.RevisedAt),
	}
}

func settingRevisions(rows []instancegen.InstanceSettingRevision) []instance.SettingRevision {
	out := make([]instance.SettingRevision, len(rows))
	for i, row := range rows {
		out[i] = instance.SettingRevision{
			ID: row.ID, Key: instance.SettingKey(row.Key), Value: row.Value,
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: sqlite.FromMillis(row.RevisedAt),
		}
	}
	return out
}

// IsSettingValueUsed implements instance.Repository.
func (r *Repository) IsSettingValueUsed(ctx context.Context, key instance.SettingKey, value string) (bool, error) {
	ok, err := r.db.reader().IsInstanceSettingValueUsed(ctx, instancegen.IsInstanceSettingValueUsedParams{Key: string(key), Value: value})
	return ok, sqlite.Wrap(err, "look up instance setting value")
}
