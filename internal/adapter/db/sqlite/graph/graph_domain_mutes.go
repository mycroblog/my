// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// InsertDomainMute implements graph.Repository.
func (r *Repository) InsertDomainMute(ctx context.Context, tx uow.Tx, m graph.DomainMute) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertDomainMute(ctx, graphgen.InsertDomainMuteParams{AccountID: m.AccountID, Domain: m.Domain, CreatedAt: sqlite.ToMillis(m.CreatedAt)})
	if err != nil {
		return false, sqlite.Wrap(err, "insert domain mute")
	}
	return rows == 1, nil
}

// DeleteDomainMute implements graph.Repository.
func (r *Repository) DeleteDomainMute(ctx context.Context, tx uow.Tx, accountID, domain string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.DeleteDomainMute(ctx, graphgen.DeleteDomainMuteParams{AccountID: accountID, Domain: domain})
	if err != nil {
		return false, sqlite.Wrap(err, "delete domain mute")
	}
	return rows == 1, nil
}

// CountDomainMutes implements graph.Repository.
func (r *Repository) CountDomainMutes(ctx context.Context, tx uow.Tx, accountID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountDomainMutes(ctx, accountID)
	return int(n), sqlite.Wrap(err, "count domain mutes")
}

// ListDomainMutes implements graph.Repository.
func (r *Repository) ListDomainMutes(ctx context.Context, accountID, after string, limit int) ([]graph.DomainMute, error) {
	rows, err := r.db.reader().ListDomainMutesPage(ctx, graphgen.ListDomainMutesPageParams{
		AccountID: accountID, BeforeDomain: after, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain mutes")
	}
	return domainMutesFrom(rows), nil
}

// ListAllDomainMutes implements graph.Repository.
func (r *Repository) ListAllDomainMutes(ctx context.Context, tx uow.Tx) ([]graph.DomainMute, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAllDomainMutes(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list all domain mutes")
	}
	return domainMutesFrom(rows), nil
}

func domainMutesFrom(rows []graphgen.AccountDomainMute) []graph.DomainMute {
	out := make([]graph.DomainMute, len(rows))
	for i, row := range rows {
		out[i] = graph.DomainMute{AccountID: row.AccountID, Domain: row.Domain, CreatedAt: sqlite.FromMillis(row.CreatedAt)}
	}
	return out
}

// MutesAnyDomain implements graph.Repository.
func (r *Repository) MutesAnyDomain(ctx context.Context, accountID string, domains []string) (bool, error) {
	if len(domains) == 0 {
		return false, nil
	}
	list, err := sqlite.JSONList(domains)
	if err != nil {
		return false, err
	}
	muted, err := r.db.reader().MutesAnyDomain(ctx, graphgen.MutesAnyDomainParams{AccountID: accountID, Domains: list})
	return muted, sqlite.Wrap(err, "look up domain mutes")
}
