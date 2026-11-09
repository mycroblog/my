// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// expiry is the text of an expiry that the queries of many rows read, "" for none.
func expiry(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// InsertBlocks implements graph.Repository.
func (r *Repository) InsertBlocks(ctx context.Context, tx uow.Tx, bs []graph.Block) error {
	if len(bs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	p := graphgen.InsertBlocksParams{CreatedAt: bs[0].CreatedAt}
	for _, b := range bs {
		p.Ids, p.Uris = append(p.Ids, b.ID), append(p.Uris, b.URI)
		p.BlockerAccountIds, p.BlockedAccountIds = append(p.BlockerAccountIds, b.BlockerAccountID), append(p.BlockedAccountIds, b.BlockedAccountID)
	}
	return postgres.Wrap(q.InsertBlocks(ctx, p), "insert blocks")
}

// InsertBlockEdges implements graph.Repository.
func (r *Repository) InsertBlockEdges(ctx context.Context, tx uow.Tx, bs []graph.Block) (int, error) {
	if len(bs) == 0 {
		return 0, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	var p graphgen.InsertBlockEdgesParams
	for _, b := range bs {
		p.BlockerAccountIds, p.BlockedAccountIds, p.BlockIds = append(p.BlockerAccountIds, b.BlockerAccountID), append(p.BlockedAccountIds, b.BlockedAccountID), append(p.BlockIds, b.ID)
	}
	n, err := q.InsertBlockEdges(ctx, p)
	return int(n), postgres.Wrap(err, "insert block edges")
}

// UpsertMutes implements graph.Repository.
func (r *Repository) UpsertMutes(ctx context.Context, tx uow.Tx, ms []graph.Mute) error {
	if len(ms) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	p := graphgen.UpsertMutesParams{TargetAccountID: ms[0].TargetAccountID, CreatedAt: ms[0].CreatedAt}
	for _, m := range ms {
		p.AccountIds, p.HideNotifications, p.ExpiresAts = append(p.AccountIds, m.AccountID), append(p.HideNotifications, m.HideNotifications), append(p.ExpiresAts, expiry(m.ExpiresAt))
	}
	return postgres.Wrap(q.UpsertMutes(ctx, p), "upsert mutes")
}

// ActiveMutesOfTargetTx implements graph.Repository.
func (r *Repository) ActiveMutesOfTargetTx(ctx context.Context, tx uow.Tx, targetID string, accountIDs []string, now time.Time) (map[string]graph.Mute, error) {
	out := make(map[string]graph.Mute, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListActiveMutesTo(ctx, graphgen.ListActiveMutesToParams{AccountID: targetID, Targets: accountIDs, Now: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list mutes of "+targetID)
	}
	for _, row := range rows {
		m := muteFrom(row)
		out[m.AccountID] = m
	}
	return out, nil
}
