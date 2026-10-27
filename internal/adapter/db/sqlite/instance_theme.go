// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertThemeRevision implements instance.ThemeRepository.
func (r *InstanceRepository) InsertThemeRevision(ctx context.Context, tx uow.Tx, rev instance.ThemeRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceThemeRevision(ctx, sqlitegen.InsertInstanceThemeRevisionParams{
		ID: rev.ID, CustomCss: rev.CustomCSS, RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(rev.RevisedAt),
	})
	if err != nil {
		return wrap(err, "insert instance theme revision")
	}
	for _, v := range rev.Tokens {
		for _, row := range v.Rows() {
			err := q.InsertInstanceThemeToken(ctx, sqlitegen.InsertInstanceThemeTokenParams{
				ThemeRevisionID: rev.ID, Token: row.Token, Scheme: string(row.Scheme), Value: row.Value,
			})
			if err != nil {
				return wrap(err, "insert instance theme token")
			}
		}
	}
	return nil
}

// ThemeHead implements instance.ThemeRepository.
func (r *InstanceRepository) ThemeHead(ctx context.Context, tx uow.Tx) (instance.ThemeHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ThemeHead{}, err
	}
	id, err := q.GetInstanceThemeHead(ctx)
	if err != nil {
		return instance.ThemeHead{}, notFound(err, appinstance.ErrNotFound, "instance theme")
	}
	return instance.ThemeHead{RevisionID: id}, nil
}

// UpsertThemeHead implements instance.ThemeRepository.
func (r *InstanceRepository) UpsertThemeHead(ctx context.Context, tx uow.Tx, h instance.ThemeHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertInstanceThemeHead(ctx, h.RevisionID), "upsert instance theme")
}

// DeleteThemeHead implements instance.ThemeRepository.
func (r *InstanceRepository) DeleteThemeHead(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteInstanceThemeHead(ctx), "delete instance theme")
}

// LatestThemeRevisionID implements instance.ThemeRepository.
func (r *InstanceRepository) LatestThemeRevisionID(ctx context.Context, tx uow.Tx) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetLatestInstanceThemeRevisionID(ctx)
	if err != nil {
		return "", notFound(err, appinstance.ErrNotFound, "instance theme revision")
	}
	return id, nil
}

// CurrentTheme implements instance.ThemeRepository.
func (r *InstanceRepository) CurrentTheme(ctx context.Context) (instance.ThemeRevision, error) {
	q := r.db.reader()
	id, err := q.GetInstanceThemeHead(ctx)
	if err != nil {
		return instance.ThemeRevision{}, notFound(err, appinstance.ErrNotFound, "instance theme")
	}
	return findThemeRevision(ctx, q, id)
}

// FindThemeRevision implements instance.ThemeRepository.
func (r *InstanceRepository) FindThemeRevision(ctx context.Context, id string) (instance.ThemeRevision, error) {
	return findThemeRevision(ctx, r.db.reader(), id)
}

func findThemeRevision(ctx context.Context, q *sqlitegen.Queries, id string) (instance.ThemeRevision, error) {
	row, err := q.GetInstanceThemeRevision(ctx, id)
	if err != nil {
		return instance.ThemeRevision{}, notFound(err, appinstance.ErrNotFound, "instance theme revision "+id)
	}
	tokens, err := q.ListInstanceThemeTokens(ctx, id)
	if err != nil {
		return instance.ThemeRevision{}, wrap(err, "list instance theme tokens")
	}
	rows := make([]instance.ThemeTokenRow, len(tokens))
	for i, t := range tokens {
		rows[i] = instance.ThemeTokenRow{Token: t.Token, Scheme: instance.ColorScheme(t.Scheme), Value: t.Value}
	}
	return instance.ThemeRevision{
		ID: row.ID, Tokens: instance.ThemeTokenValuesFromRows(rows), CustomCSS: row.CustomCss,
		RevisedByUserID: row.RevisedByUserID.String, RevisedAt: fromMillis(row.RevisedAt),
	}, nil
}
