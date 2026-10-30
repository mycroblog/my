// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// Block makes blockerID block blockedID (FR-303). In the same transaction it ends the follows
// between them in both directions, pending or active, with the reason blocked, and while the
// block is in force neither account can follow the other (Follow fails with ErrBlocked).
// Blocking an account that the blocker already blocks changes nothing. The blocker must be
// local and active; the blocked account must exist and may be remote, in which case the
// federation module delivers a Block to its server.
func (s *Service) Block(ctx context.Context, blockerID, blockedID string) (Relationship, error) {
	b := graph.Block{BlockerAccountID: blockerID, BlockedAccountID: blockedID}
	if err := b.Validate(); err != nil {
		return Relationship{}, err
	}
	if _, err := s.localActive(ctx, blockerID); err != nil {
		return Relationship{}, err
	}
	if _, err := s.existing(ctx, blockedID); err != nil {
		return Relationship{}, err
	}
	b.ID = s.d.IDs.NewID()
	b.URI = graph.BlockURI(s.d.Host, b.ID)
	b.CreatedAt = s.d.Clock.Now()

	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.blockIn(ctx, tx, b) })
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, blockerID, blockedID)
}

// blockIn records b in tx unless its pair already has a block in force, ending the follows
// between the two accounts in both directions.
func (s *Service) blockIn(ctx context.Context, tx uow.Tx, b graph.Block) error {
	repo := s.d.Repository
	if err := repo.LockPair(ctx, tx, b.BlockerAccountID, b.BlockedAccountID); err != nil {
		return err
	}
	if _, err := repo.BlockEdge(ctx, tx, b.BlockerAccountID, b.BlockedAccountID); !errors.Is(err, ErrNotFound) {
		return err // nil: already blocked
	}
	if err := repo.InsertBlock(ctx, tx, b); err != nil {
		return err
	}
	if err := s.terminateIn(ctx, tx, b.BlockerAccountID, b.BlockedAccountID, graph.TerminatedBlocked, false, b.CreatedAt); err != nil {
		return err
	}
	if err := s.terminateIn(ctx, tx, b.BlockedAccountID, b.BlockerAccountID, graph.TerminatedBlocked, false, b.CreatedAt); err != nil {
		return err
	}
	return tx.Emit(ctx, Blocked{Block: b})
}

// Unblock lifts blockerID's block of blockedID. The follows that the block ended stay ended.
// Without a block it changes nothing.
func (s *Service) Unblock(ctx context.Context, blockerID, blockedID string) (Relationship, error) {
	now := s.d.Clock.Now()
	repo := s.d.Repository
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, err := repo.BlockEdge(ctx, tx, blockerID, blockedID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		b, err := repo.Block(ctx, tx, edge.BlockID)
		if err != nil {
			return err
		}
		return s.retractIn(ctx, tx, b, now)
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, blockerID, blockedID)
}

// retractIn lifts block b in tx.
func (s *Service) retractIn(ctx context.Context, tx uow.Tx, b graph.Block, now time.Time) error {
	r := graph.BlockRetraction{BlockID: b.ID, RetractedAt: now}
	if err := s.d.Repository.InsertBlockRetraction(ctx, tx, r); err != nil {
		return err
	}
	return tx.Emit(ctx, Unblocked{Block: b, Retraction: r})
}

// FindBlock returns a block, retracted or not, for the delivery of its Block and Undo Block. It
// fails with ErrNotFound when there is none.
func (s *Service) FindBlock(ctx context.Context, id string) (graph.Block, error) {
	return s.d.Repository.FindBlock(ctx, id)
}

// Blocks returns the accounts accountID blocks, newest block first.
func (s *Service) Blocks(ctx context.Context, accountID string, p Page) ([]graph.BlockEdge, error) {
	return s.d.Repository.ListBlocking(ctx, accountID, p.Before, p.limit())
}

// IsBlocking reports whether blockerID blocks blockedID, for readers that hide an account
// from the accounts it blocks.
func (s *Service) IsBlocking(ctx context.Context, blockerID, blockedID string) (bool, error) {
	return found(s.d.Repository.FindBlockEdge(ctx, blockerID, blockedID))
}

// checkNotBlocked fails with ErrBlocked when either account blocks the other.
func (s *Service) checkNotBlocked(ctx context.Context, tx uow.Tx, a, b string) error {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		_, err := s.d.Repository.BlockEdge(ctx, tx, pair[0], pair[1])
		if err == nil {
			return fmt.Errorf("%s blocks %s: %w", pair[0], pair[1], ErrBlocked)
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// existing returns an account that exists, local or remote, in any lifecycle state.
func (s *Service) existing(ctx context.Context, id string) (appaccount.Actor, error) {
	a, err := s.d.Accounts.FindByID(ctx, id)
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, ErrAccountUnavailable)
	}
	return a, err
}

// MuteInput is the account's settings of a mute.
type MuteInput struct {
	// HideNotifications also hides the target's notifications.
	HideNotifications bool
	// Duration is how long the mute lasts; zero mutes until unmuted. A non-zero duration must be
	// at least one second (graph.ErrMuteExpiry).
	Duration time.Duration
}

// Mute makes accountID mute targetID (FR-303), replacing the settings and expiry of an existing
// mute of the target. A mute with a duration stops applying when it expires, without any
// write: Relationship and Mutes leave it out from then on. It emits Muted. The account must be
// local and active; the target must exist and may be remote.
func (s *Service) Mute(ctx context.Context, accountID, targetID string, in MuteInput) (Relationship, error) {
	m := graph.Mute{
		AccountID: accountID, TargetAccountID: targetID, HideNotifications: in.HideNotifications,
		CreatedAt: s.d.Clock.Now(),
	}
	if in.Duration != 0 {
		if in.Duration < time.Second {
			return Relationship{}, graph.ErrMuteExpiry
		}
		expires := m.CreatedAt.Add(in.Duration)
		m.ExpiresAt = &expires
	}
	if err := m.Validate(); err != nil {
		return Relationship{}, err
	}
	if _, err := s.localActive(ctx, accountID); err != nil {
		return Relationship{}, err
	}
	if _, err := s.existing(ctx, targetID); err != nil {
		return Relationship{}, err
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.UpsertMute(ctx, tx, m); err != nil {
			return err
		}
		return tx.Emit(ctx, Muted{Mute: m})
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, accountID, targetID)
}

// Unmute removes accountID's mute of targetID and emits Unmuted. Without a mute it changes
// nothing and emits nothing.
func (s *Service) Unmute(ctx context.Context, accountID, targetID string) (Relationship, error) {
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		deleted, err := s.d.Repository.DeleteMute(ctx, tx, accountID, targetID)
		if err != nil || !deleted {
			return err
		}
		return tx.Emit(ctx, Unmuted{AccountID: accountID, TargetAccountID: targetID})
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, accountID, targetID)
}

// MutePage selects a page of a list of mutes ordered newest first.
type MutePage struct {
	// Before is the position of the last mute of the previous page (MuteCursorOf); the zero
	// cursor starts at the newest.
	Before MuteCursor
	// Limit is the page size, as in Page.
	Limit int
}

// Mutes returns accountID's mutes that have not expired, newest first.
func (s *Service) Mutes(ctx context.Context, accountID string, p MutePage) ([]graph.Mute, error) {
	return s.d.Repository.ListMutes(ctx, accountID, s.d.Clock.Now(), p.Before, Page{Limit: p.Limit}.limit())
}

// Follows reports, inside tx, whether followerID follows followeeID with an accepted follow. The
// notification module's projectors call it in the transaction of the event they project.
func (s *Service) Follows(ctx context.Context, tx uow.Tx, followerID, followeeID string) (bool, error) {
	e, err := s.d.Repository.Edge(ctx, tx, followerID, followeeID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return e.State == graph.StateActive, nil
}

// FollowRequested reports, inside tx, whether followerID's follow of followeeID awaits approval.
// The notification module reads it to keep a subscription asked for with a follow of a remote
// account until the follow is accepted.
func (s *Service) FollowRequested(ctx context.Context, tx uow.Tx, followerID, followeeID string) (bool, error) {
	e, err := s.d.Repository.Edge(ctx, tx, followerID, followeeID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return e.State == graph.StatePending, nil
}

// BlockedEitherWay reports, inside tx, whether either account blocks the other.
func (s *Service) BlockedEitherWay(ctx context.Context, tx uow.Tx, a, b string) (bool, error) {
	err := s.checkNotBlocked(ctx, tx, a, b)
	if errors.Is(err, ErrBlocked) {
		return true, nil
	}
	return false, err
}

// HidesNotifications reports, inside tx, whether recipientID is not to be notified of what
// sourceID does: either account blocks the other, or recipientID mutes sourceID with
// HideNotifications and the mute has not expired. The notification module's projectors call it
// in the transaction of the event they project, so that it sees that transaction's writes.
func (s *Service) HidesNotifications(ctx context.Context, tx uow.Tx, recipientID, sourceID string) (bool, error) {
	err := s.checkNotBlocked(ctx, tx, recipientID, sourceID)
	if errors.Is(err, ErrBlocked) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	m, err := s.d.Repository.Mute(ctx, tx, recipientID, sourceID, s.d.Clock.Now())
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.HideNotifications, nil
}
