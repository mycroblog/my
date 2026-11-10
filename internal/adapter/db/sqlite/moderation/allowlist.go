// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertDomainAllow implements moderation.AllowlistRepository.
func (r *Repository) InsertDomainAllow(ctx context.Context, tx uow.Tx, a moderation.DomainAllow) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertDomainAllow(ctx, moderationgen.InsertDomainAllowParams{
		Domain: a.Domain, Comment: sqlite.ToNullString(a.Comment), CreatedByUserID: sqlite.ToNullString(a.CreatedByUserID), CreatedAt: sqlite.ToMillis(a.CreatedAt),
	})
	if err != nil {
		return false, sqlite.Wrap(err, "insert domain allow")
	}
	return n > 0, nil
}

// DeleteDomainAllow implements moderation.AllowlistRepository.
func (r *Repository) DeleteDomainAllow(ctx context.Context, tx uow.Tx, domain string) (moderation.DomainAllow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.DomainAllow{}, err
	}
	row, err := q.DeleteDomainAllow(ctx, domain)
	if err != nil {
		return moderation.DomainAllow{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "domain allow "+domain)
	}
	return domainAllowFrom(row), nil
}

// FindDomainAllow implements moderation.AllowlistRepository.
func (r *Repository) FindDomainAllow(ctx context.Context, tx uow.Tx, domain string) (moderation.DomainAllow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.DomainAllow{}, err
	}
	row, err := q.GetDomainAllow(ctx, domain)
	if err != nil {
		return moderation.DomainAllow{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "domain allow "+domain)
	}
	return domainAllowFrom(row), nil
}

// ListDomainAllows implements moderation.AllowlistRepository.
func (r *Repository) ListDomainAllows(ctx context.Context) ([]moderation.DomainAllow, error) {
	rows, err := r.db.reader().ListDomainAllows(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain allows")
	}
	out := make([]moderation.DomainAllow, len(rows))
	for i, row := range rows {
		out[i] = domainAllowFrom(row)
	}
	return out, nil
}

// CountDomainAllows implements moderation.AllowlistRepository.
func (r *Repository) CountDomainAllows(ctx context.Context, tx uow.Tx) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountDomainAllows(ctx)
	if err != nil {
		return 0, sqlite.Wrap(err, "count domain allows")
	}
	return int(n), nil
}

// AllowedDomains implements moderation.AllowlistRepository.
func (r *Repository) AllowedDomains(ctx context.Context, domains []string) ([]string, error) {
	if len(domains) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(domains)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListAllowedDomains(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list allowed domains")
	}
	return out, nil
}

// AllowedDomainsTx implements moderation.AllowlistRepository.
func (r *Repository) AllowedDomainsTx(ctx context.Context, tx uow.Tx, domains []string) ([]string, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).AllowedDomains(ctx, domains)
}

// InsertAllowlistCutoff implements moderation.AllowlistRepository.
func (r *Repository) InsertAllowlistCutoff(ctx context.Context, tx uow.Tx, c moderation.AllowlistCutoff) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAllowlistCutoff(ctx, moderationgen.InsertAllowlistCutoffParams{
		ID: c.ID, Cause: string(c.Cause), Domain: sqlite.ToNullString(c.Domain), IssuedByUserID: sqlite.ToNullString(c.IssuedByUserID), OccurredAt: sqlite.ToMillis(c.OccurredAt),
	})
	return sqlite.Wrap(err, "insert allowlist cutoff")
}

// FindAllowlistCutoff implements moderation.AllowlistRepository.
func (r *Repository) FindAllowlistCutoff(ctx context.Context, id string) (moderation.AllowlistCutoff, error) {
	row, err := r.db.reader().GetAllowlistCutoff(ctx, id)
	if err != nil {
		return moderation.AllowlistCutoff{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "allowlist cutoff "+id)
	}
	return moderation.AllowlistCutoff{
		ID: row.ID, Cause: moderation.CutoffCause(row.Cause), Domain: row.Domain.String,
		IssuedByUserID: row.IssuedByUserID.String, OccurredAt: sqlite.FromMillis(row.OccurredAt),
	}, nil
}

func domainAllowFrom(row moderationgen.DomainAllow) moderation.DomainAllow {
	return moderation.DomainAllow{
		Domain: row.Domain, Comment: row.Comment.String, CreatedByUserID: row.CreatedByUserID.String,
		CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
}
