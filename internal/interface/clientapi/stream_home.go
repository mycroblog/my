// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"sync"

	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
)

// homeGateEntries is how many entries an audienceGate remembers its decision on: the streams of
// a process handle a signal within moments of each other.
const homeGateEntries = 256

// homeAudienceBatch is a HomeEntries that tells, for an entry, which of many accounts may list it,
// as *apptimeline.Service does.
type homeAudienceBatch interface {
	HomeAudience(ctx context.Context, entryID, authorID string, accountIDs []string) (map[string]bool, error)
}

var _ homeAudienceBatch = (*apptimeline.Service)(nil)

// DeletedAudience tells which of many accounts may have seen a deleted status, so that their
// streams send its deletion. *apptimeline.Service implements it.
type DeletedAudience interface {
	DeletedAudience(ctx context.Context, d apptimeline.DeletedStatus, accountIDs []string) (map[string]bool, error)
}

var _ DeletedAudience = (*apptimeline.Service)(nil)

// homeGate decides, once per home entry and process, which of the accounts with an open user
// stream may have the entry in their home timelines (HomeAudience): one query for every stream
// of the process, after which only the streams of those accounts run HomeEntry. It holds no
// follow state, so nothing goes stale, and it works the same with several processes behind a
// broker.
type homeGate struct{ *audienceGate }

func newHomeGate() *homeGate { return &homeGate{newAudienceGate()} }

// mayList reports whether the home timeline of accountID may list the entry of e, so that its
// stream runs HomeEntry; false only when the decision on e, asked for accountID, says no. A
// signal without its author, an account that opened its stream after the decision, and a
// decision that failed all report true, which leaves the decision to HomeEntry.
func (g *homeGate) mayList(ctx context.Context, batch homeAudienceBatch, e stream.Event, accountID string) bool {
	if g == nil || batch == nil || e.AuthorAccountID == "" {
		return true
	}
	ok, err := g.decide(ctx, e.EntryID, accountID, func(ctx context.Context, ids []string) (map[string]bool, error) {
		return batch.HomeAudience(ctx, e.EntryID, e.AuthorAccountID, ids)
	})
	return ok || err != nil
}

// audienceGate decides, once per signal and process, which of the accounts with an open stream
// it counts are in the audience of the signal: one query for every stream of the process.
type audienceGate struct {
	mu        sync.Mutex
	open      map[string]int // open streams by account
	decisions map[string]*audienceDecision
	order     []string // the keys of decisions, oldest first
}

// errNotAsked reports an account whose stream opened after the decision on a signal: the
// decision does not say whether it is in the audience.
var errNotAsked = errors.New("clientapi: the stream opened after the decision")

// audienceDecision is the decision on one signal: the accounts it was asked for and those of them
// in its audience.
type audienceDecision struct {
	done     chan struct{}
	asked    map[string]bool
	audience map[string]bool
	err      error
}

func newAudienceGate() *audienceGate {
	return &audienceGate{open: map[string]int{}, decisions: map[string]*audienceDecision{}}
}

// register records an open stream of accountID, until the returned function is called.
func (g *audienceGate) register(accountID string) func() {
	g.mu.Lock()
	g.open[accountID]++
	g.mu.Unlock()
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.open[accountID]--; g.open[accountID] <= 0 {
			delete(g.open, accountID)
		}
	}
}

// decide reports whether accountID is in the audience of the signal key, which audience decides
// for the accounts with an open stream when the first of their streams asks. An account that
// opened its stream after the decision is not in it, and a ctx that ends while another stream
// decides is an error; so is a decision that failed.
func (g *audienceGate) decide(ctx context.Context, key, accountID string, audience func(context.Context, []string) (map[string]bool, error)) (bool, error) {
	g.mu.Lock()
	d, ok := g.decisions[key]
	if !ok {
		d = &audienceDecision{done: make(chan struct{}), asked: make(map[string]bool, len(g.open))}
		ids := make([]string, 0, len(g.open))
		for id := range g.open {
			d.asked[id] = true
			ids = append(ids, id)
		}
		g.decisions[key] = d
		g.order = append(g.order, key)
		if len(g.order) > homeGateEntries {
			delete(g.decisions, g.order[0])
			g.order = g.order[1:]
		}
		g.mu.Unlock()
		d.audience, d.err = audience(ctx, ids)
		close(d.done)
	} else {
		g.mu.Unlock()
		select {
		case <-d.done:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	if d.err != nil {
		return false, d.err
	}
	if !d.asked[accountID] {
		return false, errNotAsked
	}
	return d.audience[accountID], nil
}
