// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertThemeSet implements instance.ThemeRepository.
func (r *Repository) InsertThemeSet(ctx context.Context, tx uow.Tx, set instance.ThemeSet) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceThemeSet(ctx, instancegen.InsertInstanceThemeSetParams{
		ID: set.ID, Document: set.Document(), SavedByUserID: postgres.ToNullString(set.SavedByUserID), SavedAt: set.SavedAt,
	})
	return postgres.Wrap(err, "insert instance theme set")
}

// InsertThemePublication implements instance.ThemeRepository.
func (r *Repository) InsertThemePublication(ctx context.Context, tx uow.Tx, p instance.ThemePublication) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceThemePublication(ctx, instancegen.InsertInstanceThemePublicationParams{
		ID: p.ID, SetID: p.SetID, PublishedByUserID: postgres.ToNullString(p.PublishedByUserID), PublishedAt: p.PublishedAt,
	})
	return postgres.Wrap(err, "insert instance theme publication")
}

// FindThemeSet implements instance.ThemeRepository.
func (r *Repository) FindThemeSet(ctx context.Context, tx uow.Tx, id string) (instance.ThemeSet, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ThemeSet{}, err
	}
	return findThemeSet(ctx, q, id)
}

// FindThemePublication implements instance.ThemeRepository.
func (r *Repository) FindThemePublication(ctx context.Context, tx uow.Tx, id string) (instance.ThemePublication, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ThemePublication{}, err
	}
	row, err := q.GetInstanceThemePublication(ctx, id)
	if err != nil {
		return instance.ThemePublication{}, postgres.NotFound(err, appinstance.ErrNotFound, "instance theme publication "+id)
	}
	return themePublication(row), nil
}

// PreviousThemePublication implements instance.ThemeRepository.
func (r *Repository) PreviousThemePublication(ctx context.Context, tx uow.Tx, p instance.ThemePublication) (instance.ThemePublication, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ThemePublication{}, err
	}
	row, err := q.GetPreviousInstanceThemePublication(ctx, instancegen.GetPreviousInstanceThemePublicationParams{BeforeID: p.ID, SetID: p.SetID})
	if err != nil {
		return instance.ThemePublication{}, postgres.NotFound(err, appinstance.ErrNotFound, "earlier instance theme publication")
	}
	return themePublication(row), nil
}

// ThemeHead implements instance.ThemeRepository.
func (r *Repository) ThemeHead(ctx context.Context, tx uow.Tx) (instance.ThemeHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ThemeHead{}, err
	}
	id, err := q.GetInstanceThemeHead(ctx)
	if err != nil {
		return instance.ThemeHead{}, postgres.NotFound(err, appinstance.ErrNotFound, "instance theme")
	}
	return instance.ThemeHead{RevisionID: id}, nil
}

// UpsertThemeHead implements instance.ThemeRepository.
func (r *Repository) UpsertThemeHead(ctx context.Context, tx uow.Tx, h instance.ThemeHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertInstanceThemeHead(ctx, h.RevisionID), "upsert instance theme")
}

// DeleteThemeHead implements instance.ThemeRepository.
func (r *Repository) DeleteThemeHead(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteInstanceThemeHead(ctx), "delete instance theme")
}

// LatestThemeRevisionID implements instance.ThemeRepository.
func (r *Repository) LatestThemeRevisionID(ctx context.Context, tx uow.Tx) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetLatestInstanceThemePublicationID(ctx)
	if err != nil {
		return "", postgres.NotFound(err, appinstance.ErrNotFound, "instance theme publication")
	}
	return id, nil
}

// ThemeDraftHead implements instance.ThemeRepository.
func (r *Repository) ThemeDraftHead(ctx context.Context, tx uow.Tx) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetInstanceThemeDraft(ctx)
	if err != nil {
		return "", postgres.NotFound(err, appinstance.ErrNotFound, "instance theme draft")
	}
	return id, nil
}

// UpsertThemeDraftHead implements instance.ThemeRepository.
func (r *Repository) UpsertThemeDraftHead(ctx context.Context, tx uow.Tx, setID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertInstanceThemeDraft(ctx, setID), "upsert instance theme draft")
}

// DeleteThemeDraftHead implements instance.ThemeRepository.
func (r *Repository) DeleteThemeDraftHead(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteInstanceThemeDraft(ctx), "delete instance theme draft")
}

// LatestThemeSetID implements instance.ThemeRepository.
func (r *Repository) LatestThemeSetID(ctx context.Context, tx uow.Tx) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetLatestInstanceThemeSetID(ctx)
	if err != nil {
		return "", postgres.NotFound(err, appinstance.ErrNotFound, "instance theme set")
	}
	return id, nil
}

// CurrentTheme implements instance.ThemeRepository.
func (r *Repository) CurrentTheme(ctx context.Context) (instance.ThemeRevision, error) {
	q := r.db.reader()
	id, err := q.GetInstanceThemeHead(ctx)
	if err != nil {
		return instance.ThemeRevision{}, postgres.NotFound(err, appinstance.ErrNotFound, "instance theme")
	}
	return findThemeRevision(ctx, q, id)
}

// FindThemeRevision implements instance.ThemeRepository.
func (r *Repository) FindThemeRevision(ctx context.Context, id string) (instance.ThemeRevision, error) {
	return findThemeRevision(ctx, r.db.reader(), id)
}

// ThemeDraft implements instance.ThemeRepository.
func (r *Repository) ThemeDraft(ctx context.Context) (instance.ThemeSet, error) {
	q := r.db.reader()
	id, err := q.GetInstanceThemeDraft(ctx)
	if err != nil {
		return instance.ThemeSet{}, postgres.NotFound(err, appinstance.ErrNotFound, "instance theme draft")
	}
	return findThemeSet(ctx, q, id)
}

// LegacyThemeRevisions implements instance.ThemeRepository.
func (r *Repository) LegacyThemeRevisions(ctx context.Context, tx uow.Tx, after string, limit int) ([]instance.ThemeSet, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	revs, err := q.ListLegacyInstanceThemeRevisions(ctx, instancegen.ListLegacyInstanceThemeRevisionsParams{ID: legacyAfter(after), Limit: postgres.BatchLimit(limit)})
	if err != nil {
		return nil, postgres.Wrap(err, "list legacy instance theme revisions")
	}
	out := make([]instance.ThemeSet, len(revs))
	for i, rev := range revs {
		tokens, err := q.ListLegacyInstanceThemeTokens(ctx, rev.ID)
		if err != nil {
			return nil, postgres.Wrap(err, "list legacy instance theme tokens")
		}
		rows := make([]instance.ThemeTokenRow, len(tokens))
		for j, t := range tokens {
			rows[j] = instance.ThemeTokenRow{Token: t.Token, Scheme: instance.ColorScheme(t.Scheme), Value: t.Value}
		}
		out[i] = instance.ThemeSet{
			ID: rev.ID, Tokens: instance.ThemeTokenValuesFromRows(rows),
			SavedByUserID: postgres.FromNullString(rev.RevisedByUserID), SavedAt: postgres.UTC(rev.RevisedAt),
		}
	}
	return out, nil
}

// legacyAfter is the revision ID that the first page of LegacyThemeRevisions follows: the nil UUID,
// which every UUID follows.
func legacyAfter(after string) string {
	if after == "" {
		return "00000000-0000-0000-0000-000000000000"
	}
	return after
}

func findThemeRevision(ctx context.Context, q *instancegen.Queries, id string) (instance.ThemeRevision, error) {
	row, err := q.GetInstanceThemePublication(ctx, id)
	if err != nil {
		return instance.ThemeRevision{}, postgres.NotFound(err, appinstance.ErrNotFound, "instance theme publication "+id)
	}
	set, err := findThemeSet(ctx, q, row.SetID)
	if err != nil {
		return instance.ThemeRevision{}, err
	}
	return instance.ThemeRevision{ThemePublication: themePublication(row), Set: set}, nil
}

func findThemeSet(ctx context.Context, q *instancegen.Queries, id string) (instance.ThemeSet, error) {
	row, err := q.GetInstanceThemeSet(ctx, id)
	if err != nil {
		return instance.ThemeSet{}, postgres.NotFound(err, appinstance.ErrNotFound, "instance theme set "+id)
	}
	tokens, stale, err := instance.ReadThemeSet(row.Document)
	if err != nil {
		return instance.ThemeSet{}, err
	}
	return instance.ThemeSet{
		ID: row.ID, Tokens: tokens, Stale: stale,
		SavedByUserID: postgres.FromNullString(row.SavedByUserID), SavedAt: postgres.UTC(row.SavedAt),
	}, nil
}

func themePublication(row instancegen.InstanceThemePublication) instance.ThemePublication {
	return instance.ThemePublication{
		ID: row.ID, SetID: row.SetID, PublishedByUserID: postgres.FromNullString(row.PublishedByUserID), PublishedAt: postgres.UTC(row.PublishedAt),
	}
}
