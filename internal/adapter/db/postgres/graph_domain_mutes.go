// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// InsertDomainMute implements graph.Repository.
func (r *GraphRepository) InsertDomainMute(ctx context.Context, tx uow.Tx, m graph.DomainMute) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertDomainMute(ctx, postgresgen.InsertDomainMuteParams{AccountID: m.AccountID, Domain: m.Domain, CreatedAt: m.CreatedAt})
	if err != nil {
		return false, wrap(err, "insert domain mute")
	}
	return rows == 1, nil
}

// DeleteDomainMute implements graph.Repository.
func (r *GraphRepository) DeleteDomainMute(ctx context.Context, tx uow.Tx, accountID, domain string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.DeleteDomainMute(ctx, postgresgen.DeleteDomainMuteParams{AccountID: accountID, Domain: domain})
	if err != nil {
		return false, wrap(err, "delete domain mute")
	}
	return rows == 1, nil
}

// CountDomainMutes implements graph.Repository.
func (r *GraphRepository) CountDomainMutes(ctx context.Context, tx uow.Tx, accountID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountDomainMutes(ctx, accountID)
	return int(n), wrap(err, "count domain mutes")
}

// ListDomainMutes implements graph.Repository.
func (r *GraphRepository) ListDomainMutes(ctx context.Context, accountID, after string, limit int) ([]graph.DomainMute, error) {
	n, err := toInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainMutesPage(ctx, postgresgen.ListDomainMutesPageParams{
		AccountID: accountID, BeforeDomain: after, PageLimit: n,
	})
	if err != nil {
		return nil, wrap(err, "list domain mutes")
	}
	return domainMutesFrom(rows), nil
}

// ListAllDomainMutes implements graph.Repository.
func (r *GraphRepository) ListAllDomainMutes(ctx context.Context, tx uow.Tx) ([]graph.DomainMute, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAllDomainMutes(ctx)
	if err != nil {
		return nil, wrap(err, "list all domain mutes")
	}
	return domainMutesFrom(rows), nil
}

func domainMutesFrom(rows []postgresgen.AccountDomainMute) []graph.DomainMute {
	out := make([]graph.DomainMute, len(rows))
	for i, row := range rows {
		out[i] = graph.DomainMute{AccountID: row.AccountID, Domain: row.Domain, CreatedAt: utc(row.CreatedAt)}
	}
	return out
}

// MutesAnyDomain implements graph.Repository.
func (r *GraphRepository) MutesAnyDomain(ctx context.Context, accountID string, domains []string) (bool, error) {
	if len(domains) == 0 {
		return false, nil
	}
	muted, err := r.db.reader().MutesAnyDomain(ctx, postgresgen.MutesAnyDomainParams{AccountID: accountID, Domains: domains})
	return muted, wrap(err, "look up domain mutes")
}
