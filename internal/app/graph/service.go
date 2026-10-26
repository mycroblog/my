// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

var (
	// ErrAccountUnavailable reports a follower or followee that does not exist or is not active.
	ErrAccountUnavailable = errors.New("graph: the account does not exist or is not active")
	// ErrRemoteAccount reports a remote account where a use case needs a local one: the account
	// that follows, blocks, or mutes, and the target of a block. Follows received from remote
	// accounts go through ReceiveFollow.
	ErrRemoteAccount = errors.New("graph: the account is not local")
	// ErrBlocked reports a follow between two accounts when either blocks the other.
	ErrBlocked = errors.New("graph: one of the accounts blocks the other")
	// ErrInteractionRestricted reports a follower whose moderation restrictions forbid
	// following (the interact restriction, data-model §5.2).
	ErrInteractionRestricted = errors.New("graph: the account may not interact")
)

// InteractionPolicy reads whether moderation forbids an account to interact (data-model §5.3).
// *moderation.AccountPolicy implements it.
type InteractionPolicy interface {
	InteractionRestricted(ctx context.Context, accountID string) (bool, error)
}

// Accounts is the part of the account module's public API that the graph module reads: an
// account's locality, lifecycle state, and current profile (whose locked flag decides whether a
// follow needs approval). *appaccount.Service implements it.
type Accounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
}

// Deps are the ports and settings a Service uses.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Accounts   Accounts
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// Host is the serving domain of the Follow and Block activity URIs (ADR-0018).
	Host string
	// Interactions forbids following to the accounts that moderation restricts; nil restricts
	// nobody.
	Interactions InteractionPolicy
	// Outbox enqueues the migration of the relationships of a moved account in the transaction
	// of the move, and Jobs its later pages (JobMigrateFollows); with a nil Outbox no move is
	// migrated, and with nil Jobs a migration runs all its pages in one job.
	Outbox queue.Outbox
	Jobs   queue.JobQueue
}

// Service runs the graph module's follow, block, and mute use cases: those of local accounts,
// and the follows with remote accounts that the federation module receives (remote.go).
type Service struct {
	d Deps
}

// NewService returns a Service on d.
func NewService(d Deps) *Service { return &Service{d: d} }

// Relationship is the follow, block, and mute relationship between an account and a target
// account, as the account sees it.
type Relationship struct {
	AccountID       string
	TargetAccountID string
	// Following is the state of the account's follow of the target; StatePending means a
	// follow request awaits the target's approval.
	Following graph.State
	// FollowedBy is the state of the target's follow of the account.
	FollowedBy graph.State
	// Preferences are those of the account's follow of the target, or the defaults when the
	// account does not follow the target.
	ShowBoosts bool
	Notify     bool
	// Blocking reports that the account blocks the target; BlockedBy that the target blocks
	// the account.
	Blocking  bool
	BlockedBy bool
	// Muting reports that the account mutes the target (a mute that has not expired), and
	// MutingNotifications that the mute also hides the target's notifications.
	Muting              bool
	MutingNotifications bool
}

// ActivelyFollowing reports whether the account's follow of the target is active; a pending
// follow request does not count.
func (r Relationship) ActivelyFollowing() bool { return r.Following == graph.StateActive }

// RequestedFollow reports whether the account's follow of the target awaits approval.
func (r Relationship) RequestedFollow() bool { return r.Following == graph.StatePending }

// Follow makes followerID follow followeeID. The follow is active at once, unless the followee's
// current profile is locked: then it is a follow request, pending until the followee accepts it
// (FR-302). A follow of a remote account is always pending until its server sends an Accept
// (AcceptRemote). Following an account that the follower already follows or has requested to
// follow changes nothing. The follower must be local and active and allowed to interact
// (ErrInteractionRestricted), the followee active (local or remote), and neither may block the
// other (ErrBlocked).
func (s *Service) Follow(ctx context.Context, followerID, followeeID string) (Relationship, error) {
	f := graph.Follow{FollowerAccountID: followerID, FolloweeAccountID: followeeID}
	if err := f.Validate(); err != nil {
		return Relationship{}, err
	}
	if _, err := s.localActive(ctx, followerID); err != nil {
		return Relationship{}, err
	}
	if s.d.Interactions != nil {
		restricted, err := s.d.Interactions.InteractionRestricted(ctx, followerID)
		if err != nil {
			return Relationship{}, err
		}
		if restricted {
			return Relationship{}, fmt.Errorf("account %s: %w", followerID, ErrInteractionRestricted)
		}
	}
	followee, err := s.active(ctx, followeeID)
	if err != nil {
		return Relationship{}, err
	}
	locked := !followee.Account.IsLocal() || (followee.Profile != nil && followee.Profile.Locked)

	f.ID = s.d.IDs.NewID()
	f.URI = graph.FollowURI(s.d.Host, f.ID)
	f.RequestedAt = s.d.Clock.Now()
	prefs := graph.DefaultPreferences(f.ID)
	acceptance := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByAuto, AcceptedAt: f.RequestedAt}

	repo := s.d.Repository
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.LockPair(ctx, tx, followerID, followeeID); err != nil {
			return err
		}
		if err := s.checkNotBlocked(ctx, tx, followerID, followeeID); err != nil {
			return err
		}
		if _, err := repo.Edge(ctx, tx, followerID, followeeID); !errors.Is(err, ErrNotFound) {
			return err // nil: already following or requested
		}
		if err := repo.InsertFollow(ctx, tx, f); err != nil {
			return err
		}
		if err := repo.UpsertPreferences(ctx, tx, prefs); err != nil {
			return err
		}
		if locked {
			return tx.Emit(ctx, FollowRequested{Follow: f, Preferences: prefs})
		}
		if err := repo.InsertAcceptance(ctx, tx, acceptance); err != nil {
			return err
		}
		return tx.Emit(ctx, Followed{Follow: f, Acceptance: acceptance, Preferences: prefs})
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, followerID, followeeID)
}

// localActive returns a local, active account.
func (s *Service) localActive(ctx context.Context, id string) (appaccount.Actor, error) {
	a, err := s.active(ctx, id)
	if err != nil {
		return appaccount.Actor{}, err
	}
	if !a.Account.IsLocal() {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, ErrRemoteAccount)
	}
	return a, nil
}

// active returns an active account, local or remote.
func (s *Service) active(ctx context.Context, id string) (appaccount.Actor, error) {
	a, err := s.d.Accounts.FindByID(ctx, id)
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, ErrAccountUnavailable)
	}
	if err != nil {
		return appaccount.Actor{}, err
	}
	if !a.Lifecycle.Active() {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, ErrAccountUnavailable)
	}
	return a, nil
}

// AcceptFollowRequest accepts the pending follow request from followerID to followeeID, made by
// the followee. Accepting a follow that is already active changes nothing; without a follow it
// fails with ErrNotFound.
func (s *Service) AcceptFollowRequest(ctx context.Context, followeeID, followerID string) (Relationship, error) {
	now := s.d.Clock.Now()
	repo := s.d.Repository
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, f, err := s.liveFollow(ctx, tx, followerID, followeeID)
		if err != nil || edge.State == graph.StateActive {
			return err
		}
		a := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByFollowee, AcceptedAt: now}
		if err := repo.InsertAcceptance(ctx, tx, a); err != nil {
			return err
		}
		prefs, err := repo.Preferences(ctx, tx, f.ID)
		if err != nil {
			return err
		}
		return tx.Emit(ctx, FollowAccepted{Follow: f, Acceptance: a, Preferences: prefs})
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, followeeID, followerID)
}

// RejectFollowRequest declines the pending follow request from followerID to followeeID, made by
// the followee. Without a pending request it fails with ErrNotFound; to end an active follow,
// use RemoveFollower.
func (s *Service) RejectFollowRequest(ctx context.Context, followeeID, followerID string) (Relationship, error) {
	now := s.d.Clock.Now()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, f, err := s.liveFollow(ctx, tx, followerID, followeeID)
		if err != nil {
			return err
		}
		if edge.State != graph.StatePending {
			return fmt.Errorf("follow request from %s to %s: %w", followerID, followeeID, ErrNotFound)
		}
		r := graph.Rejection{FollowID: f.ID, RejectedBy: graph.RejectedByFollowee, RejectedAt: now}
		if err := s.d.Repository.InsertRejection(ctx, tx, r); err != nil {
			return err
		}
		return tx.Emit(ctx, FollowRejected{Follow: f, Rejection: r})
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, followeeID, followerID)
}

// Unfollow ends followerID's follow of followeeID, or withdraws the pending follow request.
// Without a follow it changes nothing.
func (s *Service) Unfollow(ctx context.Context, followerID, followeeID string) (Relationship, error) {
	if err := s.terminate(ctx, followerID, followeeID, graph.TerminatedUnfollowed, false); err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, followerID, followeeID)
}

// RemoveFollower ends followerID's active follow of followeeID, made by the followee. Without an
// active follow it changes nothing; to decline a pending request, use RejectFollowRequest.
func (s *Service) RemoveFollower(ctx context.Context, followeeID, followerID string) (Relationship, error) {
	if err := s.terminate(ctx, followerID, followeeID, graph.TerminatedRemovedByFollowee, true); err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, followeeID, followerID)
}

func (s *Service) terminate(ctx context.Context, followerID, followeeID string, reason graph.TerminationReason, activeOnly bool) error {
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.terminateIn(ctx, tx, followerID, followeeID, reason, activeOnly, now)
	})
}

// terminateIn ends the follow from followerID to followeeID inside tx, if there is one (and it
// is active, when activeOnly).
func (s *Service) terminateIn(ctx context.Context, tx uow.Tx, followerID, followeeID string, reason graph.TerminationReason, activeOnly bool, now time.Time) error {
	_, err := s.endIn(ctx, tx, FollowTerminated{Termination: graph.Termination{Reason: reason, TerminatedAt: now}}, followerID, followeeID, activeOnly)
	return err
}

// endIn is terminateIn with the termination's reason, time, and announcement taken from ev, and
// returns the state that the follow ended in, graph.StateNone when no follow ended.
func (s *Service) endIn(ctx context.Context, tx uow.Tx, ev FollowTerminated, followerID, followeeID string, activeOnly bool) (graph.State, error) {
	edge, f, err := s.liveFollow(ctx, tx, followerID, followeeID)
	if errors.Is(err, ErrNotFound) || (err == nil && activeOnly && edge.State != graph.StateActive) {
		return graph.StateNone, nil
	}
	if err != nil {
		return graph.StateNone, err
	}
	ev.Follow, ev.PreviousState = f, edge.State
	ev.Termination.FollowID = f.ID
	if err := s.d.Repository.InsertTermination(ctx, tx, ev.Termination); err != nil {
		return graph.StateNone, err
	}
	return edge.State, tx.Emit(ctx, ev)
}

// liveFollow returns the edge from follower to followee and its follow; ErrNotFound means none.
func (s *Service) liveFollow(ctx context.Context, tx uow.Tx, followerID, followeeID string) (graph.Edge, graph.Follow, error) {
	edge, err := s.d.Repository.Edge(ctx, tx, followerID, followeeID)
	if err != nil {
		return graph.Edge{}, graph.Follow{}, err
	}
	f, err := s.d.Repository.Follow(ctx, tx, edge.FollowID)
	if err != nil {
		return graph.Edge{}, graph.Follow{}, err
	}
	return edge, f, nil
}

// PreferencesInput is the follower's settings of a follow.
type PreferencesInput struct {
	ShowBoosts bool
	Notify     bool
}

// UpdatePreferences changes the preferences of followerID's pending or active follow of
// followeeID. Without a follow it fails with ErrNotFound.
func (s *Service) UpdatePreferences(ctx context.Context, followerID, followeeID string, in PreferencesInput) (Relationship, error) {
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		edge, f, err := s.liveFollow(ctx, tx, followerID, followeeID)
		if err != nil {
			return err
		}
		p := graph.Preferences{FollowID: f.ID, ShowBoosts: in.ShowBoosts, Notify: in.Notify}
		if err := s.d.Repository.UpsertPreferences(ctx, tx, p); err != nil {
			return err
		}
		return tx.Emit(ctx, FollowPreferencesChanged{Follow: f, State: edge.State, Preferences: p})
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, followerID, followeeID)
}

// Relationship returns the follow, block, and mute relationship between accountID and targetID.
func (s *Service) Relationship(ctx context.Context, accountID, targetID string) (Relationship, error) {
	repo := s.d.Repository
	out := Relationship{AccountID: accountID, TargetAccountID: targetID, Following: graph.StateNone, FollowedBy: graph.StateNone}
	defaults := graph.DefaultPreferences("")
	out.ShowBoosts, out.Notify = defaults.ShowBoosts, defaults.Notify

	outgoing, err := repo.FindEdge(ctx, accountID, targetID)
	switch {
	case err == nil:
		out.Following = outgoing.State
		p, err := repo.FindPreferences(ctx, outgoing.FollowID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return Relationship{}, err
		}
		if err == nil {
			out.ShowBoosts, out.Notify = p.ShowBoosts, p.Notify
		}
	case !errors.Is(err, ErrNotFound):
		return Relationship{}, err
	}
	incoming, err := repo.FindEdge(ctx, targetID, accountID)
	switch {
	case err == nil:
		out.FollowedBy = incoming.State
	case !errors.Is(err, ErrNotFound):
		return Relationship{}, err
	}
	if out.Blocking, err = found(repo.FindBlockEdge(ctx, accountID, targetID)); err != nil {
		return Relationship{}, err
	}
	if out.BlockedBy, err = found(repo.FindBlockEdge(ctx, targetID, accountID)); err != nil {
		return Relationship{}, err
	}
	m, err := repo.FindMute(ctx, accountID, targetID, s.d.Clock.Now())
	switch {
	case err == nil:
		out.Muting, out.MutingNotifications = true, m.HideNotifications
	case !errors.Is(err, ErrNotFound):
		return Relationship{}, err
	}
	return out, nil
}

// found turns the result of a lookup into whether it found a row; ErrNotFound is not an error.
func found[T any](_ T, err error) (bool, error) {
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Page selects a page of a list ordered newest first.
type Page struct {
	// Before is the follow ID of the last edge of the previous page; "" starts at the newest.
	Before string
	// Limit is the page size; zero or less means DefaultPageLimit, and it is capped at MaxPageLimit.
	Limit int
}

// Page sizes of the follow lists.
const (
	DefaultPageLimit = 40
	MaxPageLimit     = 80
)

func (p Page) limit() int {
	switch {
	case p.Limit <= 0:
		return DefaultPageLimit
	case p.Limit > MaxPageLimit:
		return MaxPageLimit
	}
	return p.Limit
}

// Followers returns accountID's active followers, newest follow first.
func (s *Service) Followers(ctx context.Context, accountID string, p Page) ([]graph.Edge, error) {
	return s.d.Repository.ListFollowers(ctx, accountID, graph.StateActive, p.Before, p.limit())
}

// MaxFollowerBatch caps the batch size of ActiveFollowerIDs.
const MaxFollowerBatch = 1000

// ActiveFollowerIDs returns a batch of accountID's active followers, local and remote, newest
// follow first, for the jobs that walk all of them (the delivery fan-out, data-model §2.1). It
// starts after the follow ID before ("" for the newest) and returns at most limit accounts
// (capped at MaxFollowerBatch), with the cursor of the next batch, "" after the last one.
func (s *Service) ActiveFollowerIDs(ctx context.Context, accountID, before string, limit int) (ids []string, next string, err error) {
	limit = min(max(limit, 1), MaxFollowerBatch)
	edges, err := s.d.Repository.ListFollowers(ctx, accountID, graph.StateActive, before, limit)
	if err != nil {
		return nil, "", err
	}
	ids = make([]string, len(edges))
	for i, e := range edges {
		ids[i] = e.FollowerAccountID
	}
	if len(edges) == limit {
		next = edges[len(edges)-1].FollowID
	}
	return ids, next, nil
}

// Following returns the accounts accountID actively follows, newest follow first.
func (s *Service) Following(ctx context.Context, accountID string, p Page) ([]graph.Edge, error) {
	return s.d.Repository.ListFollowing(ctx, accountID, graph.StateActive, p.Before, p.limit())
}

// FollowsAnyone reports whether accountID actively follows an account. The follows of a remote
// account that this server knows are of local accounts, so for a remote account it tells a
// follower of the instance's users.
func (s *Service) FollowsAnyone(ctx context.Context, accountID string) (bool, error) {
	edges, err := s.d.Repository.ListFollowing(ctx, accountID, graph.StateActive, "", 1)
	return len(edges) > 0, err
}

// FollowRequests returns the pending follow requests to accountID, newest first.
func (s *Service) FollowRequests(ctx context.Context, accountID string, p Page) ([]graph.Edge, error) {
	return s.d.Repository.ListFollowers(ctx, accountID, graph.StatePending, p.Before, p.limit())
}

// Counters returns the numbers of accountID's active followers and followees.
func (s *Service) Counters(ctx context.Context, accountID string) (graph.Counters, error) {
	c, err := s.d.Repository.FindCounters(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return graph.Counters{AccountID: accountID}, nil
	}
	return c, err
}

// CandidateRelation is the relation of a viewer with a candidate of the prefix search of
// accounts (ADR-0092 §1): Hidden when either blocks the other or the viewer mutes the candidate
// now, Following when the viewer actively follows it, and the number of its active followers.
type CandidateRelation struct {
	Hidden    bool
	Following bool
	Followers int64
}

// BlockedAmong returns which of the accounts ids are in a block relation with accountID: one
// blocks the other, either way. It reads the relations for all of ids at once.
func (s *Service) BlockedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	r, err := s.d.Repository.RelationsAmong(ctx, accountID, ids, s.d.Clock.Now())
	if err != nil {
		return nil, err
	}
	for _, list := range [][]string{r.Blocked, r.BlockedBy} {
		for _, id := range list {
			out[id] = true
		}
	}
	return out, nil
}

// CandidateRelations returns the relations of viewerID with the accounts ids, by account ID;
// every account in ids has an entry. It reads each relation for all of ids at once.
func (s *Service) CandidateRelations(ctx context.Context, viewerID string, ids []string) (map[string]CandidateRelation, error) {
	out := make(map[string]CandidateRelation, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	r, err := s.d.Repository.RelationsAmong(ctx, viewerID, ids, s.d.Clock.Now())
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = CandidateRelation{}
	}
	for _, list := range [][]string{r.Blocked, r.BlockedBy, r.Muted} {
		for _, id := range list {
			c := out[id]
			c.Hidden = true
			out[id] = c
		}
	}
	for _, id := range r.Following {
		c := out[id]
		c.Following = true
		out[id] = c
	}
	for _, n := range r.Counters {
		c := out[n.AccountID]
		c.Followers = n.FollowersCount
		out[n.AccountID] = c
	}
	return out, nil
}
