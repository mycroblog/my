// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// Block is graph.Block, a block as the federation module delivers it.
type Block = graph.Block

// ErrLocalAccount reports a local account where a remote one is required: the blocker of a
// received Block.
var ErrLocalAccount = errors.New("graph: the account is local")

// RemoteBlockInput is a Block received from another server.
type RemoteBlockInput struct {
	// URI is the id of the Block activity; "" for a Block without one.
	URI string
	// BlockerAccountID is the remote actor of the Block.
	BlockerAccountID string
	// BlockedAccountID is the local account it blocks.
	BlockedAccountID string
}

// ReceiveBlock records a remote account's block of a local account, received in a Block. In the
// same transaction it ends the follows between them in both directions with the reason blocked,
// as Block does, and it emits Blocked, whose subscribers hide the blocker's posts from the
// blocked account. A Block whose URI is already recorded, or between accounts with a block in
// force, changes nothing, so that receiving it again leaves the same rows. The blocker must be a
// remote account (ErrLocalAccount) and the blocked account a local one (ErrRemoteAccount); either
// missing fails with ErrAccountUnavailable.
func (s *Service) ReceiveBlock(ctx context.Context, in RemoteBlockInput) error {
	b := graph.Block{URI: in.URI, BlockerAccountID: in.BlockerAccountID, BlockedAccountID: in.BlockedAccountID}
	if err := b.Validate(); err != nil {
		return err
	}
	if err := s.remotePair(ctx, in.BlockerAccountID, in.BlockedAccountID); err != nil {
		return err
	}
	b.ID = s.d.IDs.NewID()
	b.CreatedAt = s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if b.URI != "" {
			if _, err := s.d.Repository.BlockByURI(ctx, tx, b.URI); !errors.Is(err, ErrNotFound) {
				return err // nil: received before
			}
		}
		return s.blockIn(ctx, tx, b)
	})
}

// RemoteUnblockInput is an Undo of a Block received from another server.
type RemoteUnblockInput struct {
	// BlockURI is the id of the Block undone; "" for a Block without one, which is found by its
	// accounts instead.
	BlockURI string
	// BlockerAccountID is the remote actor of the Undo, who must be the blocker.
	BlockerAccountID string
	// BlockedAccountID is the local account of a Block without an id; ignored otherwise.
	BlockedAccountID string
}

// RetractRemoteBlock lifts a block that a remote account made, received in an Undo of its Block,
// and emits Unblocked. The follows that the block ended stay ended. A block that is already
// lifted changes nothing. It fails with ErrNotFound when the blocker made no such block, so that
// the federation module records a tombstone of the Block's URI (ADR-0006 §9).
func (s *Service) RetractRemoteBlock(ctx context.Context, in RemoteUnblockInput) error {
	now := s.d.Clock.Now()
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if in.BlockURI == "" {
			edge, err := repo.BlockEdge(ctx, tx, in.BlockerAccountID, in.BlockedAccountID)
			if err != nil {
				return err
			}
			b, err := repo.Block(ctx, tx, edge.BlockID)
			if err != nil {
				return err
			}
			return s.retractIn(ctx, tx, b, now)
		}
		h, err := repo.BlockByURI(ctx, tx, in.BlockURI)
		switch {
		case err != nil:
			return err
		case h.Block.BlockerAccountID != in.BlockerAccountID:
			return fmt.Errorf("block %s of another account: %w", in.BlockURI, ErrNotFound)
		case h.Retraction != nil:
			return nil
		}
		return s.retractIn(ctx, tx, h.Block, now)
	})
}

// remotePair checks that blockerID is a remote account and blockedID a local one.
func (s *Service) remotePair(ctx context.Context, blockerID, blockedID string) error {
	blocker, err := s.existing(ctx, blockerID)
	if err != nil {
		return err
	}
	if blocker.Account.IsLocal() {
		return fmt.Errorf("account %s: %w", blockerID, ErrLocalAccount)
	}
	blocked, err := s.existing(ctx, blockedID)
	if err != nil {
		return err
	}
	if !blocked.Account.IsLocal() {
		return fmt.Errorf("account %s: %w", blockedID, ErrRemoteAccount)
	}
	return nil
}
