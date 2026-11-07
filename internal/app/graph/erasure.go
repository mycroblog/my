// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// ErasureRepository is the part of the graph module's persistence that the erasure of deleted
// accounts uses (data-model §8). The database adapters' graph repositories implement it and
// pass federationtest.AccountErasureContract.
type ErasureRepository interface {
	// ListEdgesToErase returns up to limit live follows into or out of the account.
	ListEdgesToErase(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]graph.Edge, error)
	// EraseFollowsOf deletes up to limit follows into or out of the account, with their
	// acceptances, rejections, terminations, resends, preferences, and edges, and returns how
	// many follows it deleted.
	EraseFollowsOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error)
	// EraseBlocksOf deletes up to limit blocks by or of the account, with their retractions and
	// edges, and returns how many blocks it deleted.
	EraseBlocksOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error)
	// EraseMutesOf deletes up to limit mutes by the account and up to limit mutes of it, and
	// returns the more of the two counts.
	EraseMutesOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error)
	// EraseCountersOf deletes the follow counters of the account.
	EraseCountersOf(ctx context.Context, tx uow.Tx, accountID string) error
}

// Eraser erases a deleted local account's follows, blocks, and mutes (data-model §8). The live
// follows end first with the reason account_deleted, which counts them out of the other
// accounts' counters and tells no server (the Delete of the actor did); then the follows, blocks,
// and mutes go with their events and projections.
type Eraser struct {
	uow     uow.UnitOfWork
	repo    Repository
	erasure ErasureRepository
	clock   kernel.Clock
}

// NewEraser returns the graph module's eraser.
func NewEraser(u uow.UnitOfWork, repo Repository, r ErasureRepository, clock kernel.Clock) *Eraser {
	return &Eraser{uow: u, repo: repo, erasure: r, clock: clock}
}

// EraseAccountData implements account.DataEraser.
func (e *Eraser) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, _ string, limit int) (appaccount.ErasureStep, error) {
	now := kernel.Normalize(e.clock.Now())
	var done bool
	err := e.uow.Tx(ctx, func(tx uow.Tx) error {
		edges, err := e.erasure.ListEdgesToErase(ctx, tx, t.AccountID, limit)
		if err != nil {
			return err
		}
		for _, edge := range edges {
			f, err := e.repo.Follow(ctx, tx, edge.FollowID)
			if err != nil {
				return err
			}
			ev := FollowTerminated{
				Follow:        f,
				Termination:   graph.Termination{FollowID: edge.FollowID, Reason: graph.TerminatedAccountDeleted, TerminatedAt: now},
				PreviousState: edge.State,
				Unannounced:   true,
			}
			if err := e.repo.InsertTermination(ctx, tx, ev.Termination); err != nil {
				return err
			}
			if err := tx.Emit(ctx, ev); err != nil {
				return err
			}
		}
		follows, err := e.erasure.EraseFollowsOf(ctx, tx, t.AccountID, limit)
		if err != nil {
			return err
		}
		blocks, err := e.erasure.EraseBlocksOf(ctx, tx, t.AccountID, limit)
		if err != nil {
			return err
		}
		mutes, err := e.erasure.EraseMutesOf(ctx, tx, t.AccountID, limit)
		if err != nil {
			return err
		}
		done = len(edges) < limit && follows < int64(limit) && blocks < int64(limit) && mutes < int64(limit)
		if !done {
			return nil
		}
		return e.erasure.EraseCountersOf(ctx, tx, t.AccountID)
	})
	return appaccount.ErasureStep{Done: done}, err
}
