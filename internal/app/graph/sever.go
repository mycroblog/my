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
	ev := FollowTerminated{
		Termination: graph.Termination{Reason: graph.TerminatedModeration, TerminatedAt: now},
		Unannounced: !announce,
	}
	var out Severed
	for _, other := range slices.Compact(slices.Sorted(slices.Values(others))) {
		if other == accountID {
			continue
		}
		if err := s.d.Repository.LockPair(ctx, tx, accountID, other); err != nil {
			return Severed{}, err
		}
		var followed, following graph.State
		var err error
		if dir.Followers {
			if followed, err = s.endIn(ctx, tx, ev, other, accountID, false); err != nil {
				return Severed{}, err
			}
		}
		if dir.Following {
			if following, err = s.endIn(ctx, tx, ev, accountID, other, false); err != nil {
				return Severed{}, err
			}
		}
		if followed == graph.StateActive {
			out.Followers++
		}
		if following == graph.StateActive {
			out.Following++
		}
	}
	return out, nil
}
