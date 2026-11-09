// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"maps"
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
	lost, err := s.SeverAmong(ctx, tx, map[string][]string{accountID: others}, dir, announce, now)
	return lost[accountID], err
}

// CounterpartsOf is Counterparts of each of ids, by account, with one query.
func (s *Service) CounterpartsOf(ctx context.Context, tx uow.Tx, ids []string) (map[string][]string, error) {
	edges, err := s.d.Repository.ListEdgesOfAccounts(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(ids))
	for _, id := range ids {
		out[id] = []string{}
	}
	for _, e := range edges {
		if _, ok := out[e.FollowerAccountID]; ok {
			out[e.FollowerAccountID] = append(out[e.FollowerAccountID], e.FolloweeAccountID)
		}
		if _, ok := out[e.FolloweeAccountID]; ok {
			out[e.FolloweeAccountID] = append(out[e.FolloweeAccountID], e.FollowerAccountID)
		}
	}
	for id, others := range out {
		slices.Sort(others)
		out[id] = slices.Compact(others)
	}
	return out, nil
}

// SeverAmong is SeverDirected of each account of others with its others, by account, in dir as
// seen from the account: the follows of all the pairs end together, with a number of statements
// that does not grow with them, and one FollowsTerminated tells the projectors. A follow between
// two accounts that are both keys of others ends once and counts for both.
func (s *Service) SeverAmong(ctx context.Context, tx uow.Tx, others map[string][]string, dir SeverDirection, announce bool, now time.Time) (map[string]Severed, error) {
	return s.endAmong(ctx, tx, others, dir, graph.TerminatedModeration, announce, now)
}

// endAmong is SeverAmong with the reason of the ends.
func (s *Service) endAmong(ctx context.Context, tx uow.Tx, others map[string][]string, dir SeverDirection, reason graph.TerminationReason, announce bool, now time.Time) (map[string]Severed, error) {
	out := map[string]Severed{}
	var as, bs []string
	for _, account := range slices.Sorted(maps.Keys(others)) {
		for _, other := range slices.Compact(slices.Sorted(slices.Values(others[account]))) {
			if other != account {
				as, bs = append(as, account), append(bs, other)
			}
		}
	}
	if len(as) == 0 {
		return out, nil
	}
	repo := s.d.Repository
	if err := repo.LockPairs(ctx, tx, as, bs); err != nil {
		return nil, err
	}
	edges, err := repo.ListEdgesBetween(ctx, tx, as, bs)
	if err != nil {
		return nil, err
	}
	named := make(map[[2]string]bool, len(as))
	for i := range as {
		named[[2]string{as[i], bs[i]}] = true
	}
	// An edge ends when one of its accounts names the other in a direction of dir: as a follower
	// (Following) or as a followee (Followers).
	var ending []graph.Edge
	var ids []string
	for _, e := range edges {
		if (dir.Following && named[[2]string{e.FollowerAccountID, e.FolloweeAccountID}]) ||
			(dir.Followers && named[[2]string{e.FolloweeAccountID, e.FollowerAccountID}]) {
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
			Termination: graph.Termination{FollowID: f.ID, Reason: reason, TerminatedAt: now},
		})
		if e.State != graph.StateActive {
			continue
		}
		if _, ok := others[e.FollowerAccountID]; ok && dir.Following && named[[2]string{e.FollowerAccountID, e.FolloweeAccountID}] {
			sv := out[e.FollowerAccountID]
			sv.Following++
			out[e.FollowerAccountID] = sv
		}
		if _, ok := others[e.FolloweeAccountID]; ok && dir.Followers && named[[2]string{e.FolloweeAccountID, e.FollowerAccountID}] {
			sv := out[e.FolloweeAccountID]
			sv.Followers++
			out[e.FolloweeAccountID] = sv
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	if err := repo.InsertTerminations(ctx, tx, ids, reason, now); err != nil {
		return nil, err
	}
	return out, tx.Emit(ctx, ev)
}
