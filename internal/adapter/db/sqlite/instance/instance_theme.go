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

// InsertThemeRevision implements instance.ThemeRepository.
func (r *Repository) InsertThemeRevision(ctx context.Context, tx uow.Tx, rev instance.ThemeRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceThemeRevision(ctx, instancegen.InsertInstanceThemeRevisionParams{
		ID: rev.ID, CustomCss: rev.CustomCSS, RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID), RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert instance theme revision")
	}
	for _, v := range rev.Tokens {
		for _, row := range v.Rows() {
			err := q.InsertInstanceThemeToken(ctx, instancegen.InsertInstanceThemeTokenParams{
				RevisionID: rev.ID, Token: row.Token, Scheme: string(row.Scheme), Value: row.Value,
			})
			if err != nil {
				return sqlite.Wrap(err, "insert instance theme token")
			}
		}
	}
	return nil
}

// ThemeHead implements instance.ThemeRepository.
func (r *Repository) ThemeHead(ctx context.Context, tx uow.Tx) (instance.ThemeHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ThemeHead{}, err
	}
	id, err := q.GetInstanceThemeHead(ctx)
	if err != nil {
		return instance.ThemeHead{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance theme")
	}
	return instance.ThemeHead{RevisionID: id}, nil
}

// UpsertThemeHead implements instance.ThemeRepository.
func (r *Repository) UpsertThemeHead(ctx context.Context, tx uow.Tx, h instance.ThemeHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertInstanceThemeHead(ctx, h.RevisionID), "upsert instance theme")
}

// DeleteThemeHead implements instance.ThemeRepository.
func (r *Repository) DeleteThemeHead(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteInstanceThemeHead(ctx), "delete instance theme")
}

// LatestThemeRevisionID implements instance.ThemeRepository.
func (r *Repository) LatestThemeRevisionID(ctx context.Context, tx uow.Tx) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetLatestInstanceThemeRevisionID(ctx)
	if err != nil {
		return "", sqlite.NotFound(err, appinstance.ErrNotFound, "instance theme revision")
	}
	return id, nil
}

// CurrentTheme implements instance.ThemeRepository.
func (r *Repository) CurrentTheme(ctx context.Context) (instance.ThemeRevision, error) {
	q := r.db.reader()
	id, err := q.GetInstanceThemeHead(ctx)
	if err != nil {
		return instance.ThemeRevision{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance theme")
	}
	return findThemeRevision(ctx, q, id)
}

// FindThemeRevision implements instance.ThemeRepository.
func (r *Repository) FindThemeRevision(ctx context.Context, id string) (instance.ThemeRevision, error) {
	return findThemeRevision(ctx, r.db.reader(), id)
}

func findThemeRevision(ctx context.Context, q *instancegen.Queries, id string) (instance.ThemeRevision, error) {
	row, err := q.GetInstanceThemeRevision(ctx, id)
	if err != nil {
		return instance.ThemeRevision{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance theme revision "+id)
	}
	tokens, err := q.ListInstanceThemeTokens(ctx, id)
	if err != nil {
		return instance.ThemeRevision{}, sqlite.Wrap(err, "list instance theme tokens")
	}
	rows := make([]instance.ThemeTokenRow, len(tokens))
	for i, t := range tokens {
		rows[i] = instance.ThemeTokenRow{Token: t.Token, Scheme: instance.ColorScheme(t.Scheme), Value: t.Value}
	}
	return instance.ThemeRevision{
		ID: row.ID, Tokens: instance.ThemeTokenValuesFromRows(rows), CustomCSS: row.CustomCss,
		RevisedByUserID: row.RevisedByUserID.String, RevisedAt: sqlite.FromMillis(row.RevisedAt),
	}, nil
}
