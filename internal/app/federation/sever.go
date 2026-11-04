// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// SeverDirection says which follows with remote accounts SeverFederation ends (ADR-0006, `my
// admin federation sever --direction`).
type SeverDirection string

// Sever directions.
const (
	// SeverIn ends the remote accounts' follows of local ones.
	SeverIn SeverDirection = "in"
	// SeverOut ends the local accounts' follows of remote ones.
	SeverOut SeverDirection = "out"
	// SeverBothWays ends both.
	SeverBothWays SeverDirection = "both"
)

// ErrInvalidSeverDirection reports a SeverDirection that is not one of the directions.
var ErrInvalidSeverDirection = errors.New("federation: the direction is not in, out, or both")

// severBatch is the number of local accounts SeverFederation reads at a time.
const severBatch = 100

// SeverGraph is the part of the graph module's public API that severing the federation uses;
// *appgraph.Service implements it.
type SeverGraph interface {
	Counterparts(ctx context.Context, tx uow.Tx, accountID string) ([]string, error)
	SeverDirected(ctx context.Context, tx uow.Tx, accountID string, others []string, dir appgraph.SeverDirection, announce bool, now time.Time) (appgraph.Severed, error)
}

// SeverAccounts lists the local accounts and tells the local ones among the counterparts of
// their follows; *appaccount.Service implements it.
type SeverAccounts interface {
	LocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error)
	IsLocal(ctx context.Context, tx uow.Tx, accountID string) (bool, error)
}

// SeveranceDeps are the ports of Severance.
type SeveranceDeps struct {
	UnitOfWork uow.UnitOfWork
	Graph      SeverGraph
	Accounts   SeverAccounts
	// Modes reads the federation mode, which decides whether the Reject and the Undo go out;
	// nil federates both ways.
	Modes ModeSettings
	Clock kernel.Clock
}

// Severance ends the follows between the local accounts and the remote ones in one or both
// directions, for the operator who narrows the federation mode and wants the relationships gone
// too (ADR-0006, 切り替え). Changing the mode never calls it.
type Severance struct{ d SeveranceDeps }

// NewSeverance returns the Severance on d.
func NewSeverance(d SeveranceDeps) *Severance { return &Severance{d: d} }

// Severed counts the active follows that a severance ended: of local accounts by remote ones
// (Followers) and of remote accounts by local ones (Following). The pending ones end without
// counting.
type Severed struct {
	Followers, Following int
}

// Sever ends, in dir, every pending or active follow between a local account and a remote one,
// as moderation ends them (follow_terminations with the reason moderation, ADR-0058). Each ended
// follow is told to the remote side, a Reject of its Follow or an Undo of the local account's,
// only when the federation mode at the time sends it (federation.Sends); otherwise nothing is
// recorded to send. It runs one short transaction per local account and is safe to repeat: a
// follow that has ended is left as it is.
func (s *Severance) Sever(ctx context.Context, dir SeverDirection) (Severed, error) {
	var in, out bool
	switch dir {
	case SeverIn:
		in = true
	case SeverOut:
		out = true
	case SeverBothWays:
		in, out = true, true
	default:
		return Severed{}, fmt.Errorf("%w: %q", ErrInvalidSeverDirection, dir)
	}
	mode, err := currentMode(ctx, s.d.Modes)
	if err != nil {
		return Severed{}, err
	}
	rejects := federation.Sends(mode, federation.OutboundActivity{Type: federation.ActivityReject, ObjectKind: federation.OutboundFollow}, false)
	undoes := federation.Sends(mode, federation.OutboundActivity{Type: federation.ActivityUndo, ObjectKind: federation.OutboundFollow}, false)

	var total Severed
	after := ""
	for {
		locals, err := s.d.Accounts.LocalAccountIDs(ctx, after, severBatch)
		if err != nil {
			return total, err
		}
		for _, local := range locals {
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				remotes, err := s.remoteCounterparts(ctx, tx, local)
				if err != nil || len(remotes) == 0 {
					return err
				}
				now := kernel.Normalize(s.d.Clock.Now())
				if in {
					got, err := s.d.Graph.SeverDirected(ctx, tx, local, remotes, appgraph.SeverDirection{Followers: true}, rejects, now)
					if err != nil {
						return err
					}
					total.Followers += got.Followers
				}
				if out {
					got, err := s.d.Graph.SeverDirected(ctx, tx, local, remotes, appgraph.SeverDirection{Following: true}, undoes, now)
					if err != nil {
						return err
					}
					total.Following += got.Following
				}
				return nil
			})
			if err != nil {
				return total, fmt.Errorf("sever the follows of %s: %w", local, err)
			}
		}
		if len(locals) < severBatch {
			return total, nil
		}
		after = locals[len(locals)-1]
	}
}

// remoteCounterparts returns, inside tx, the remote accounts with a pending or active follow to
// or from the local account.
func (s *Severance) remoteCounterparts(ctx context.Context, tx uow.Tx, local string) ([]string, error) {
	others, err := s.d.Graph.Counterparts(ctx, tx, local)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, o := range others {
		isLocal, err := s.d.Accounts.IsLocal(ctx, tx, o)
		if err != nil {
			return nil, err
		}
		if !isLocal {
			out = append(out, o)
		}
	}
	return out, nil
}
