// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"sync"

	"github.com/mycroblog/my/internal/app/stream"
)

// homeGateEntries is how many home entries a homeGate remembers its decision on: the streams of
// a process handle an entry within moments of each other.
const homeGateEntries = 256

// homeAudienceBatch is a HomeEntries that tells, for an entry, which of many accounts may list it,
// as *apptimeline.Service does.
type homeAudienceBatch interface {
	HomeAudience(ctx context.Context, authorID string, accountIDs []string) (map[string]bool, error)
}

// homeGate decides, once per home entry and process, which of the accounts with an open user
// stream may have the entry in their home timelines (HomeAudience): one query for every stream
// of the process, after which only the streams of those accounts run HomeEntry. It holds no
// follow state, so nothing goes stale, and it works the same with several processes behind a
// broker.
type homeGate struct {
	mu        sync.Mutex
	open      map[string]int // open user streams by account
	decisions map[string]*homeDecision
	order     []string // the entries of decisions, oldest first
}

// homeDecision is the decision on one entry: the accounts it was asked for and those of them
// whose home timelines may list the entry.
type homeDecision struct {
	done     chan struct{}
	asked    map[string]bool
	audience map[string]bool
	err      error
}

func newHomeGate() *homeGate {
	return &homeGate{open: map[string]int{}, decisions: map[string]*homeDecision{}}
}

// register records an open user stream of accountID, until the returned function is called.
func (g *homeGate) register(accountID string) func() {
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

// mayList reports whether the home timeline of accountID may list the entry of e, so that its
// stream runs HomeEntry; false only when the decision on e, asked for accountID, says no. A
// signal without its author, an account that opened its stream after the decision, and a
// decision that failed all report true, which leaves the decision to HomeEntry.
func (g *homeGate) mayList(ctx context.Context, batch homeAudienceBatch, e stream.Event, accountID string) bool {
	if g == nil || batch == nil || e.AuthorAccountID == "" {
		return true
	}
	g.mu.Lock()
	d, ok := g.decisions[e.EntryID]
	if !ok {
		d = &homeDecision{done: make(chan struct{}), asked: make(map[string]bool, len(g.open))}
		ids := make([]string, 0, len(g.open))
		for id := range g.open {
			d.asked[id] = true
			ids = append(ids, id)
		}
		g.decisions[e.EntryID] = d
		g.order = append(g.order, e.EntryID)
		if len(g.order) > homeGateEntries {
			delete(g.decisions, g.order[0])
			g.order = g.order[1:]
		}
		g.mu.Unlock()
		d.audience, d.err = batch.HomeAudience(ctx, e.AuthorAccountID, ids)
		close(d.done)
	} else {
		g.mu.Unlock()
		select {
		case <-d.done:
		case <-ctx.Done():
			return true
		}
	}
	if d.err != nil || !d.asked[accountID] {
		return true
	}
	return d.audience[accountID]
}
