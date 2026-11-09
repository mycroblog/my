// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// The follows that moderation cuts when it suspends a domain or a remote account (FR-717,
// ADR-0058). The moderation module calls these in the transaction that records what each local
// account lost (relationship_severances). The federation module cuts the follows with remote
// accounts the same way when the operator severs the federation (ADR-0006).

// Severed counts the active follows that an account lost to a severance.
type Severed struct {
	// Followers counts the other accounts' follows of the account, Following the account's
	// follows of the other accounts.
	Followers int
	Following int
}

// Lost reports whether the account lost an active follow.
func (s Severed) Lost() bool { return s.Followers+s.Following > 0 }

// Counterparts returns, inside tx, the accounts that have a pending or active follow to or from
// accountID, in ID order and each once.
func (s *Service) Counterparts(ctx context.Context, tx uow.Tx, accountID string) ([]string, error) {
	edges, err := s.d.Repository.ListEdgesOf(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(edges))
	for _, e := range edges {
		other := e.FolloweeAccountID
		if other == accountID {
			other = e.FollowerAccountID
		}
		out = append(out, other)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// SeverByModeration ends inside tx every pending or active follow between accountID and each of
// others, in both directions, with the reason moderation, and returns how many active ones
// accountID lost; the pending ones end without counting. Each follow emits FollowTerminated,
// whose Unannounced is !announce: the suspension of a remote account tells its server
// (Reject and Undo of the follows), that of a domain tells nothing. The follows stay ended when
// the suspension is lifted, and a follow that has already ended is left as it is, so severing
// again counts nothing.
func (s *Service) SeverByModeration(ctx context.Context, tx uow.Tx, accountID string, others []string, announce bool, now time.Time) (Severed, error) {
	return s.SeverDirected(ctx, tx, accountID, others, SeverBoth, announce, now)
}

// SeverDirection says which follows between two accounts a severance ends.
type SeverDirection struct {
	// Followers ends the other accounts' follows of the account, Following the account's follows
	// of the other accounts.
	Followers, Following bool
}

// SeverBoth ends the follows in both directions.
var SeverBoth = SeverDirection{Followers: true, Following: true}

// SeverDirected is SeverByModeration of the follows in the direction dir only.
func (s *Service) SeverDirected(ctx context.Context, tx uow.Tx, accountID string, others []string, dir SeverDirection, announce bool, now time.Time) (Severed, error) {
	each, err := s.SeverEach(ctx, tx, accountID, others, dir, announce, now)
	var out Severed
	for _, sv := range each {
		out.Followers += sv.Followers
		out.Following += sv.Following
	}
	return out, err
}

// SeverEach is SeverDirected that returns what accountID lost with each of others, by other:
// the follows with all of them end together, with a number of statements that does not grow
// with them, and one FollowsTerminated tells the projectors.
func (s *Service) SeverEach(ctx context.Context, tx uow.Tx, accountID string, others []string, dir SeverDirection, announce bool, now time.Time) (map[string]Severed, error) {
	out := map[string]Severed{}
	others = slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(others))), func(id string) bool { return id == accountID })
	if len(others) == 0 {
		return out, nil
	}
	repo := s.d.Repository
	if err := repo.LockPairs(ctx, tx, accountID, others); err != nil {
		return nil, err
	}
	edges, err := repo.ListEdgesOf(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	var ending []graph.Edge
	var ids []string
	for _, e := range edges {
		follower, followee := e.FollowerAccountID == accountID, e.FolloweeAccountID == accountID
		switch {
		case follower && dir.Following && slices.Contains(others, e.FolloweeAccountID),
			followee && dir.Followers && slices.Contains(others, e.FollowerAccountID):
			ending, ids = append(ending, e), append(ids, e.FollowID)
		}
	}
	if len(ending) == 0 {
		return out, nil
	}
	follows, err := repo.FollowsByIDs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	ev := FollowsTerminated{}
	ids = ids[:0]
	for _, e := range ending {
		f, ok := follows[e.FollowID]
		if !ok {
			continue // an edge without its follow ends nothing, as liveFollow finds none
		}
		ids = append(ids, f.ID)
		ev.Terminations = append(ev.Terminations, FollowTerminated{
			Follow: f, PreviousState: e.State, Unannounced: !announce,
			Termination: graph.Termination{FollowID: f.ID, Reason: graph.TerminatedModeration, TerminatedAt: now},
		})
		if e.State != graph.StateActive {
			continue
		}
		if e.FolloweeAccountID == accountID {
			sv := out[e.FollowerAccountID]
			sv.Followers++
			out[e.FollowerAccountID] = sv
		} else {
			sv := out[e.FolloweeAccountID]
			sv.Following++
			out[e.FolloweeAccountID] = sv
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	if err := repo.InsertTerminations(ctx, tx, ids, graph.TerminatedModeration, now); err != nil {
		return nil, err
	}
	return out, tx.Emit(ctx, ev)
}

// Reversed is what the other account lost: its followers are the follows that the account lost
// of it, and the other way round.
func (s Severed) Reversed() Severed { return Severed{Followers: s.Following, Following: s.Followers} }
