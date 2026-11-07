// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// LockPair implements graph.Repository. It does nothing: write transactions begin with BEGIN
// IMMEDIATE and so already run one at a time.
func (r *Repository) LockPair(context.Context, uow.Tx, string, string) error { return nil }

// InsertBlock implements graph.Repository.
func (r *Repository) InsertBlock(ctx context.Context, tx uow.Tx, b graph.Block) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertBlock(ctx, graphgen.InsertBlockParams{
		ID: b.ID, Uri: sqlite.ToNullString(b.URI), BlockerAccountID: b.BlockerAccountID, BlockedAccountID: b.BlockedAccountID,
		CreatedAt: sqlite.ToMillis(b.CreatedAt),
	}), "insert block")
}

// InsertBlockRetraction implements graph.Repository.
func (r *Repository) InsertBlockRetraction(ctx context.Context, tx uow.Tx, ret graph.BlockRetraction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertBlockRetraction(ctx, graphgen.InsertBlockRetractionParams{
		BlockID: ret.BlockID, RetractedAt: sqlite.ToMillis(ret.RetractedAt),
	})
	if sqlite.IsUniqueViolation(err, "block_retractions.block_id") {
		return fmt.Errorf("block_retractions of block %s: %w", ret.BlockID, appgraph.ErrAlreadyRecorded)
	}
	return sqlite.Wrap(err, "insert block_retractions")
}

func blockFrom(row graphgen.Block) graph.Block {
	return graph.Block{
		ID: row.ID, URI: row.Uri.String, BlockerAccountID: row.BlockerAccountID, BlockedAccountID: row.BlockedAccountID,
		CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
}

// Block implements graph.Repository.
func (r *Repository) Block(ctx context.Context, tx uow.Tx, id string) (graph.Block, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Block{}, err
	}
	row, err := q.GetBlock(ctx, id)
	if err != nil {
		return graph.Block{}, sqlite.NotFound(err, appgraph.ErrNotFound, "block "+id)
	}
	return blockFrom(row), nil
}

func blockEdgesFrom(rows []graphgen.BlockEdge) []graph.BlockEdge {
	out := make([]graph.BlockEdge, len(rows))
	for i, row := range rows {
		out[i] = graph.BlockEdge(row)
	}
	return out
}

// BlockEdge implements graph.Repository.
func (r *Repository) BlockEdge(ctx context.Context, tx uow.Tx, blockerAccountID, blockedAccountID string) (graph.BlockEdge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.BlockEdge{}, err
	}
	return findBlockEdge(ctx, q, blockerAccountID, blockedAccountID)
}

// FindBlockEdge implements graph.Repository.
func (r *Repository) FindBlockEdge(ctx context.Context, blockerAccountID, blockedAccountID string) (graph.BlockEdge, error) {
	return findBlockEdge(ctx, r.db.reader(), blockerAccountID, blockedAccountID)
}

func findBlockEdge(ctx context.Context, q *graphgen.Queries, blockerAccountID, blockedAccountID string) (graph.BlockEdge, error) {
	row, err := q.GetBlockEdge(ctx, graphgen.GetBlockEdgeParams{
		BlockerAccountID: blockerAccountID, BlockedAccountID: blockedAccountID,
	})
	if err != nil {
		return graph.BlockEdge{}, sqlite.NotFound(err, appgraph.ErrNotFound, "block edge "+blockerAccountID+" -> "+blockedAccountID)
	}
	return graph.BlockEdge(row), nil
}

// InsertBlockEdge implements graph.Repository.
func (r *Repository) InsertBlockEdge(ctx context.Context, tx uow.Tx, e graph.BlockEdge) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertBlockEdge(ctx, graphgen.InsertBlockEdgeParams(e))
	if err != nil {
		return false, sqlite.Wrap(err, "insert block edge")
	}
	return n == 1, nil
}

// DeleteBlockEdge implements graph.Repository.
func (r *Repository) DeleteBlockEdge(ctx context.Context, tx uow.Tx, blockerAccountID, blockedAccountID, blockID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteBlockEdge(ctx, graphgen.DeleteBlockEdgeParams{
		BlockerAccountID: blockerAccountID, BlockedAccountID: blockedAccountID, BlockID: blockID,
	}), "delete block edge")
}

// DeleteAllBlockEdges implements graph.Repository.
func (r *Repository) DeleteAllBlockEdges(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllBlockEdges(ctx), "delete block edges")
}

// ListBlockHistories implements graph.Repository.
func (r *Repository) ListBlockHistories(ctx context.Context, tx uow.Tx) ([]graph.BlockHistory, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListBlockHistories(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list block histories")
	}
	out := make([]graph.BlockHistory, len(rows))
	for i, row := range rows {
		h := graph.BlockHistory{Block: blockFrom(graphgen.Block{
			ID: row.ID, Uri: row.Uri, BlockerAccountID: row.BlockerAccountID, BlockedAccountID: row.BlockedAccountID,
			CreatedAt: row.CreatedAt,
		})}
		if row.RetractedAt.Valid {
			h.Retraction = &graph.BlockRetraction{BlockID: row.ID, RetractedAt: sqlite.FromMillis(row.RetractedAt.Int64)}
		}
		out[i] = h
	}
	return out, nil
}

// ListBlockEdges implements graph.Repository.
func (r *Repository) ListBlockEdges(ctx context.Context) ([]graph.BlockEdge, error) {
	rows, err := r.db.reader().ListBlockEdges(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list block edges")
	}
	return blockEdgesFrom(rows), nil
}

// ListBlocking implements graph.Repository.
func (r *Repository) ListBlocking(ctx context.Context, blockerAccountID, before string, limit int) ([]graph.BlockEdge, error) {
	rows, err := r.db.reader().ListBlockingPage(ctx, graphgen.ListBlockingPageParams{
		AccountID: blockerAccountID, Before: before, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list blocking")
	}
	return blockEdgesFrom(rows), nil
}

// UpsertMute implements graph.Repository.
func (r *Repository) UpsertMute(ctx context.Context, tx uow.Tx, m graph.Mute) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertMute(ctx, graphgen.UpsertMuteParams{
		AccountID: m.AccountID, TargetAccountID: m.TargetAccountID, HideNotifications: sqlite.BoolToInt(m.HideNotifications),
		ExpiresAt: sqlite.ToNullMillis(m.ExpiresAt), CreatedAt: sqlite.ToMillis(m.CreatedAt),
	}), "upsert mute")
}

// DeleteMute implements graph.Repository.
func (r *Repository) DeleteMute(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.DeleteMute(ctx, graphgen.DeleteMuteParams{AccountID: accountID, TargetAccountID: targetAccountID})
	if err != nil {
		return false, sqlite.Wrap(err, "delete mute")
	}
	return rows == 1, nil
}

// DeleteExpiredMutes implements graph.Repository.
func (r *Repository) DeleteExpiredMutes(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]graph.Mute, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteExpiredMutes(ctx, graphgen.DeleteExpiredMutesParams{Now: sqlite.ToMillis(now), BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "delete expired mutes")
	}
	out := make([]graph.Mute, len(rows))
	for i, row := range rows {
		out[i] = muteFrom(row)
	}
	return out, nil
}

func muteFrom(row graphgen.Mute) graph.Mute {
	return graph.Mute{
		AccountID: row.AccountID, TargetAccountID: row.TargetAccountID, HideNotifications: row.HideNotifications != 0,
		ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt), CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
}

// Mute implements graph.Repository.
func (r *Repository) Mute(ctx context.Context, tx uow.Tx, accountID, targetAccountID string, now time.Time) (graph.Mute, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Mute{}, err
	}
	return activeMute(ctx, q, accountID, targetAccountID, now)
}

// FindMute implements graph.Repository.
func (r *Repository) FindMute(ctx context.Context, accountID, targetAccountID string, now time.Time) (graph.Mute, error) {
	return activeMute(ctx, r.db.reader(), accountID, targetAccountID, now)
}

func activeMute(ctx context.Context, q *graphgen.Queries, accountID, targetAccountID string, now time.Time) (graph.Mute, error) {
	row, err := q.GetActiveMute(ctx, graphgen.GetActiveMuteParams{
		AccountID: accountID, TargetAccountID: targetAccountID, Now: sqlite.ToMillis(now),
	})
	if err != nil {
		return graph.Mute{}, sqlite.NotFound(err, appgraph.ErrNotFound, "mute "+accountID+" -> "+targetAccountID)
	}
	return muteFrom(row), nil
}

// ListMutes implements graph.Repository.
func (r *Repository) ListMutes(ctx context.Context, accountID string, now time.Time, before appgraph.MuteCursor, limit int) ([]graph.Mute, error) {
	rows, err := r.db.reader().ListActiveMutesPage(ctx, graphgen.ListActiveMutesPageParams{
		AccountID: accountID, Now: sqlite.ToMillis(now), BeforeTargetAccountID: before.TargetAccountID,
		BeforeCreatedAt: sqlite.ToMillis(before.CreatedAt), PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list mutes")
	}
	out := make([]graph.Mute, len(rows))
	for i, row := range rows {
		out[i] = muteFrom(row)
	}
	return out, nil
}

// BlockByURI implements graph.Repository.
func (r *Repository) BlockByURI(ctx context.Context, tx uow.Tx, uri string) (graph.BlockHistory, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.BlockHistory{}, err
	}
	row, err := q.GetBlockByURI(ctx, sqlite.ToNullString(uri))
	if err != nil {
		return graph.BlockHistory{}, sqlite.NotFound(err, appgraph.ErrNotFound, "block "+uri)
	}
	h := graph.BlockHistory{Block: blockFrom(graphgen.Block{
		ID: row.ID, Uri: row.Uri, BlockerAccountID: row.BlockerAccountID, BlockedAccountID: row.BlockedAccountID,
		CreatedAt: row.CreatedAt,
	})}
	if row.RetractedAt.Valid {
		h.Retraction = &graph.BlockRetraction{BlockID: row.ID, RetractedAt: sqlite.FromMillis(row.RetractedAt.Int64)}
	}
	return h, nil
}

// FindBlock implements graph.Repository.
func (r *Repository) FindBlock(ctx context.Context, id string) (graph.Block, error) {
	row, err := r.db.reader().GetBlock(ctx, id)
	if err != nil {
		return graph.Block{}, sqlite.NotFound(err, appgraph.ErrNotFound, "block "+id)
	}
	return blockFrom(row), nil
}

// ListBlockers implements graph.Repository.
func (r *Repository) ListBlockers(ctx context.Context, blockedAccountID, after string, limit int) ([]graph.BlockEdge, error) {
	rows, err := r.db.reader().ListBlockersPage(ctx, graphgen.ListBlockersPageParams{
		AccountID: blockedAccountID, After: after, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list blockers")
	}
	return blockEdgesFrom(rows), nil
}

// ListMuters implements graph.Repository.
func (r *Repository) ListMuters(ctx context.Context, targetAccountID string, now time.Time, after string, limit int) ([]graph.Mute, error) {
	rows, err := r.db.reader().ListActiveMutersPage(ctx, graphgen.ListActiveMutersPageParams{
		TargetAccountID: targetAccountID, Now: sqlite.ToMillis(now), After: after, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list muters")
	}
	out := make([]graph.Mute, len(rows))
	for i, row := range rows {
		out[i] = muteFrom(row)
	}
	return out, nil
}
