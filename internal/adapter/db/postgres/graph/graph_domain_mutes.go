// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// InsertDomainMute implements graph.Repository.
func (r *Repository) InsertDomainMute(ctx context.Context, tx uow.Tx, m graph.DomainMute) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertDomainMute(ctx, graphgen.InsertDomainMuteParams{AccountID: m.AccountID, Domain: m.Domain, CreatedAt: m.CreatedAt})
	if err != nil {
		return false, postgres.Wrap(err, "insert domain mute")
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
		return false, postgres.Wrap(err, "delete domain mute")
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
	return int(n), postgres.Wrap(err, "count domain mutes")
}

// ListDomainMutes implements graph.Repository.
func (r *Repository) ListDomainMutes(ctx context.Context, accountID, after string, limit int) ([]graph.DomainMute, error) {
	n, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainMutesPage(ctx, graphgen.ListDomainMutesPageParams{
		AccountID: accountID, BeforeDomain: after, PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list domain mutes")
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
		return nil, postgres.Wrap(err, "list all domain mutes")
	}
	return domainMutesFrom(rows), nil
}

func domainMutesFrom(rows []graphgen.AccountDomainMute) []graph.DomainMute {
	out := make([]graph.DomainMute, len(rows))
	for i, row := range rows {
		out[i] = graph.DomainMute{AccountID: row.AccountID, Domain: row.Domain, CreatedAt: postgres.UTC(row.CreatedAt)}
	}
	return out
}

// MutesAnyDomain implements graph.Repository.
func (r *Repository) MutesAnyDomain(ctx context.Context, accountID string, domains []string) (bool, error) {
	if len(domains) == 0 {
		return false, nil
	}
	muted, err := r.db.reader().MutesAnyDomain(ctx, graphgen.MutesAnyDomainParams{AccountID: accountID, Domains: domains})
	return muted, postgres.Wrap(err, "look up domain mutes")
}
