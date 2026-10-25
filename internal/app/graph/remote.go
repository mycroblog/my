// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// The use cases of follows with remote accounts, called by the federation module's inbox
// handlers (federation.md §3.4). Each runs in one short transaction and emits the same events as
// the local use cases, so that the projectors (and the federation module's outbound projectors,
// which send Accept, Reject, and Undo) see every change. Every one of them is safe to repeat and
// to run in any order with the others (NFR-REL-1): a Follow is recorded once per URI, and the
// acceptance, rejection, or termination of a follow that already has one changes nothing. A
// Follow received again under another URI is known by that URI too (follow_resends).

// ReceiveFollowInput is a Follow activity received from a remote account.
type ReceiveFollowInput struct {
	// URI is the IRI of the Follow activity, the key that makes receiving it again a no-op.
	URI string
	// FollowerID is the remote account that sent the Follow; FolloweeID is the local account
	// it follows.
	FollowerID string
	FolloweeID string
}

// ReceiveFollow records a Follow that remote account in.FollowerID sent to local account
// in.FolloweeID. The follow is active at once unless the followee is locked, when it is a
// follow request pending until the followee accepts it (FR-302). When either account blocks the
// other, the follow is recorded and rejected by the system at once, so that the follower's
// server gets a Reject. Receiving a Follow whose URI is already recorded, or one between a pair
// that already has a pending follow, changes nothing. A Follow under a new URI for a follow that
// is already active means that the follower's server does not count the follow as established
// (Mastodon sends its Follow again until it gets an Accept): it is recorded as a Resend of the
// follow, which keeps its row, and FollowResent sends the Accept again. The follower must be an
// active remote account and the followee an active local one (ErrAccountUnavailable,
// ErrRemoteAccount).
func (s *Service) ReceiveFollow(ctx context.Context, in ReceiveFollowInput) error {
	f := graph.Follow{URI: in.URI, FollowerAccountID: in.FollowerID, FolloweeAccountID: in.FolloweeID}
	if err := f.Validate(); err != nil {
		return err
	}
	if f.URI == "" {
		return errors.New("graph: a received follow needs the URI of its activity")
	}
	follower, err := s.active(ctx, in.FollowerID)
	if err != nil {
		return err
	}
	if follower.Account.IsLocal() {
		return fmt.Errorf("account %s is local: %w", in.FollowerID, ErrAccountUnavailable)
	}
	followee, err := s.localActive(ctx, in.FolloweeID)
	if err != nil {
		return err
	}
	locked := followee.Profile != nil && followee.Profile.Locked

	f.ID = s.d.IDs.NewID()
	f.RequestedAt = s.d.Clock.Now()
	prefs := graph.DefaultPreferences(f.ID)
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.followByAnyURI(ctx, tx, f.URI); !errors.Is(err, ErrNotFound) {
			return err // nil: this Follow is recorded already
		}
		if err := repo.LockPair(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID); err != nil {
			return err
		}
		blocked := s.checkNotBlocked(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID)
		if blocked != nil && !errors.Is(blocked, ErrBlocked) {
			return blocked
		}
		if blocked == nil {
			edge, err := repo.Edge(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID)
			switch {
			case errors.Is(err, ErrNotFound):
			case err != nil || edge.State != graph.StateActive:
				return err // nil: already requested
			default:
				return s.resend(ctx, tx, edge.FollowID, f)
			}
		}
		if err := repo.InsertFollow(ctx, tx, f); err != nil {
			return err
		}
		if err := repo.UpsertPreferences(ctx, tx, prefs); err != nil {
			return err
		}
		switch {
		case blocked != nil:
			r := graph.Rejection{FollowID: f.ID, RejectedBy: graph.RejectedBySystem, RejectedAt: f.RequestedAt}
			if err := repo.InsertRejection(ctx, tx, r); err != nil {
				return err
			}
			// The follow was never live: the edge it would have had is not there to delete.
			return tx.Emit(ctx, FollowRejected{Follow: f, Rejection: r})
		case locked:
			return tx.Emit(ctx, FollowRequested{Follow: f, Preferences: prefs})
		}
		a := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByAuto, AcceptedAt: f.RequestedAt}
		if err := repo.InsertAcceptance(ctx, tx, a); err != nil {
			return err
		}
		return tx.Emit(ctx, Followed{Follow: f, Acceptance: a, Preferences: prefs})
	})
}

// resend records f, a Follow received again, as a Resend of the active follow followID.
func (s *Service) resend(ctx context.Context, tx uow.Tx, followID string, f graph.Follow) error {
	existing, err := s.d.Repository.Follow(ctx, tx, followID)
	if err != nil {
		return err
	}
	r := graph.Resend{URI: f.URI, FollowID: followID, ReceivedAt: f.RequestedAt}
	inserted, err := s.d.Repository.InsertResend(ctx, tx, r)
	if err != nil || !inserted {
		return err
	}
	return tx.Emit(ctx, FollowResent{Follow: existing, Resend: r})
}

// followByAnyURI returns the follow whose Follow, or a Follow received again for it, has the
// IRI uri.
func (s *Service) followByAnyURI(ctx context.Context, tx uow.Tx, uri string) (graph.Follow, error) {
	f, err := s.d.Repository.FollowByURI(ctx, tx, uri)
	if errors.Is(err, ErrNotFound) {
		return s.d.Repository.FollowByResendURI(ctx, tx, uri)
	}
	return f, err
}

// RemoteResponse is an Accept or Reject that a remote followee sent about a follow of it by a
// local account.
type RemoteResponse struct {
	// FollowURI is the IRI of the Follow it answers.
	FollowURI string
	// FolloweeID is the remote account that sent the response.
	FolloweeID string
	// FollowerID is the local account that the response names as the follower, "" when it does
	// not say. It is a hint: the follow is found by FollowURI first, and by the pending or active
	// follow of FollowerID of FolloweeID only when no follow of FolloweeID has that URI.
	FollowerID string
	// ActivityURI is the IRI of the Accept or Reject.
	ActivityURI string
}

// AcceptRemote records that remote followee in.FolloweeID accepted the follow that in names:
// a pending follow becomes active. A follow that is already active, or has ended, stays so. Without a pending or
// active follow that in names, it fails with ErrNotFound and changes nothing.
func (s *Service) AcceptRemote(ctx context.Context, in RemoteResponse) error {
	now := s.d.Clock.Now()
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, f, err := s.remoteFollow(ctx, tx, in.FollowURI, in.FollowerID, in.FolloweeID, false)
		if errors.Is(err, errFollowEnded) {
			return nil
		}
		if err != nil || edge.State == graph.StateActive {
			return err
		}
		a := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByRemote, ActivityURI: in.ActivityURI, AcceptedAt: now}
		if err := repo.InsertAcceptance(ctx, tx, a); err != nil {
			return err
		}
		prefs, err := repo.Preferences(ctx, tx, f.ID)
		if err != nil {
			return err
		}
		return tx.Emit(ctx, FollowAccepted{Follow: f, Acceptance: a, Preferences: prefs})
	})
}

// RejectRemote records that remote followee in.FolloweeID declined the follow that in names: a
// pending follow is rejected, and an active one ends as removed by the followee (an Undo of its
// Accept means the same). A follow that has ended stays so. Without a pending or active follow that in names, it fails with
// ErrNotFound and changes nothing.
func (s *Service) RejectRemote(ctx context.Context, in RemoteResponse) error {
	now := s.d.Clock.Now()
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, f, err := s.remoteFollow(ctx, tx, in.FollowURI, in.FollowerID, in.FolloweeID, false)
		if errors.Is(err, errFollowEnded) {
			return nil
		}
		if err != nil {
			return err
		}
		if edge.State == graph.StatePending {
			r := graph.Rejection{FollowID: f.ID, RejectedBy: graph.RejectedByRemote, ActivityURI: in.ActivityURI, RejectedAt: now}
			if err := repo.InsertRejection(ctx, tx, r); err != nil {
				return err
			}
			return tx.Emit(ctx, FollowRejected{Follow: f, Rejection: r})
		}
		t := graph.Termination{FollowID: f.ID, Reason: graph.TerminatedRemovedByFollowee, TerminatedAt: now}
		if err := repo.InsertTermination(ctx, tx, t); err != nil {
			return err
		}
		return tx.Emit(ctx, FollowTerminated{Follow: f, Termination: t, PreviousState: edge.State})
	})
}

// RemoteUndo is an Undo of a Follow that a remote follower sent.
type RemoteUndo struct {
	// FollowURI is the IRI of the Follow it takes back.
	FollowURI string
	// FollowerID is the remote account that sent the Undo.
	FollowerID string
	// FolloweeID is the local account that the undone Follow names, "" when the Undo does not
	// say: a hint used only when no follow of FollowerID has FollowURI, as in RemoteResponse.
	FolloweeID string
}

// TerminateRemote ends the follow that remote follower in.FollowerID takes back with an Undo,
// pending or active, as unfollowed. A follow with that URI that has already ended stays so. Without
// a follow that in names, it fails with ErrNotFound and changes nothing: the federation module
// then records a tombstone, so that the Follow arriving later is dropped (federation.md §3.3).
func (s *Service) TerminateRemote(ctx context.Context, in RemoteUndo) error {
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, f, err := s.remoteFollow(ctx, tx, in.FollowURI, in.FollowerID, in.FolloweeID, true)
		if errors.Is(err, errFollowEnded) {
			return nil
		}
		if err != nil {
			return err
		}
		t := graph.Termination{FollowID: f.ID, Reason: graph.TerminatedUnfollowed, TerminatedAt: now}
		if err := s.d.Repository.InsertTermination(ctx, tx, t); err != nil {
			return err
		}
		return tx.Emit(ctx, FollowTerminated{Follow: f, Termination: t, PreviousState: edge.State})
	})
}

// errFollowEnded reports that the follow an activity names exists but is no longer live.
var errFollowEnded = errors.New("graph: the follow has ended")

// remoteFollow returns the live follow that an activity of a remote account names, with its
// edge, after locking its pair. The remote account is the sender: the follower of the follow when
// byFollower, its followee otherwise. The follow is the one with uri, as its Follow or one
// received again, whose sender side is the sender; when there is none, it is the live follow between the sender and the hint (the other
// side, "" for none). A follow with uri that is no longer live fails with errFollowEnded, and no
// follow at all with ErrNotFound.
func (s *Service) remoteFollow(ctx context.Context, tx uow.Tx, uri, followerID, followeeID string, byFollower bool) (graph.Edge, graph.Follow, error) {
	repo := s.d.Repository
	sender := followeeID
	if byFollower {
		sender = followerID
	}
	f, err := s.followByAnyURI(ctx, tx, uri)
	switch {
	case err == nil && (byFollower && f.FollowerAccountID == sender || !byFollower && f.FolloweeAccountID == sender):
		if err := repo.LockPair(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID); err != nil {
			return graph.Edge{}, graph.Follow{}, err
		}
		edge, err := repo.Edge(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID)
		if errors.Is(err, ErrNotFound) || (err == nil && edge.FollowID != f.ID) {
			return graph.Edge{}, graph.Follow{}, fmt.Errorf("follow %s: %w", uri, errFollowEnded)
		}
		return edge, f, err
	case err != nil && !errors.Is(err, ErrNotFound):
		return graph.Edge{}, graph.Follow{}, err
	}
	if followerID == "" || followeeID == "" {
		return graph.Edge{}, graph.Follow{}, fmt.Errorf("follow %s: %w", uri, ErrNotFound)
	}
	if err := repo.LockPair(ctx, tx, followerID, followeeID); err != nil {
		return graph.Edge{}, graph.Follow{}, err
	}
	return s.liveFollow(ctx, tx, followerID, followeeID)
}

// FindFollow returns a follow by ID, whatever became of it, for the federation module's
// fan-out of the activities about it.
func (s *Service) FindFollow(ctx context.Context, id string) (graph.Follow, error) {
	return s.d.Repository.FindFollow(ctx, id)
}

// The graph domain's types and values that other modules' application layers use through this
// package (ADR-0002: they import no other module's domain).
type (
	// FollowRecord is a follow; see graph.Follow.
	FollowRecord = graph.Follow
)

// Who accepted, rejected, or ended a follow, for the subscribers of the follow events.
const (
	AcceptedByRemote            = graph.AcceptedByRemote
	RejectedByRemote            = graph.RejectedByRemote
	TerminatedUnfollowed        = graph.TerminatedUnfollowed
	TerminatedRemovedByFollowee = graph.TerminatedRemovedByFollowee
	TerminatedBlocked           = graph.TerminatedBlocked
	TerminatedModeration        = graph.TerminatedModeration
	TerminatedMoved             = graph.TerminatedMoved
)

// ErrSelfFollow reports a follow of an account by itself; see graph.ErrSelfFollow.
var ErrSelfFollow = graph.ErrSelfFollow
