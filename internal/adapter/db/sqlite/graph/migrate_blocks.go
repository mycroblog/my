// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// blocksJSON encodes bs as the JSON array that InsertBlocks and InsertBlockEdges read.
func blocksJSON(bs []graph.Block) (string, error) {
	type row struct {
		ID      string `json:"id"`
		URI     string `json:"uri"`
		Blocker string `json:"blocker"`
		Blocked string `json:"blocked"`
	}
	rows := make([]row, len(bs))
	for i, b := range bs {
		rows[i] = row{ID: b.ID, URI: b.URI, Blocker: b.BlockerAccountID, Blocked: b.BlockedAccountID}
	}
	out, err := json.Marshal(rows)
	return string(out), err
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
	list, err := blocksJSON(bs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertBlocks(ctx, graphgen.InsertBlocksParams{CreatedAt: sqlite.ToMillis(bs[0].CreatedAt), Blocks: list}), "insert blocks")
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
	list, err := blocksJSON(bs)
	if err != nil {
		return 0, err
	}
	n, err := q.InsertBlockEdges(ctx, list)
	return int(n), sqlite.Wrap(err, "insert block edges")
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
	type row struct {
		AccountID         string `json:"account_id"`
		HideNotifications bool   `json:"hide_notifications"`
		ExpiresAt         *int64 `json:"expires_at"`
	}
	rows := make([]row, len(ms))
	for i, m := range ms {
		rows[i] = row{AccountID: m.AccountID, HideNotifications: m.HideNotifications}
		if m.ExpiresAt != nil {
			v := sqlite.ToMillis(*m.ExpiresAt)
			rows[i].ExpiresAt = &v
		}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertMutes(ctx, graphgen.UpsertMutesParams{TargetAccountID: ms[0].TargetAccountID, CreatedAt: sqlite.ToMillis(ms[0].CreatedAt), Mutes: string(list)}), "upsert mutes")
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
	targets, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListActiveMutesTo(ctx, graphgen.ListActiveMutesToParams{AccountID: targetID, Targets: targets, Now: sqlite.ToMillis(now)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list mutes of "+targetID)
	}
	for _, row := range rows {
		m := muteFrom(row)
		out[m.AccountID] = m
	}
	return out, nil
}
