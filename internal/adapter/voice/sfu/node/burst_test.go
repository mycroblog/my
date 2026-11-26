// SPDX-License-Identifier: AGPL-3.0-only

package node_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node/nodetest"
	"github.com/mycroblog/my/internal/app/stream"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

// dial connects session of room to the node of its grant with a simulated browser.
func (e *env) dial(t *testing.T, room, session, account string) (*nodetest.Client, error) {
	g, err := e.transport.Grant(t.Context(), appvoice.RoomRef{ID: room}, appvoice.Session{ID: session, AccountID: account}, true)
	if err != nil {
		return nil, fmt.Errorf("grant of %s: %w", session, err)
	}
	return nodetest.Dial(t, g.URL, g.Token, session)
}

// goroutinesSettle fails the test unless the goroutines of the process come back to at most
// baseline: those of the connections that left end.
func goroutinesSettle(t *testing.T, baseline int) {
	t.Helper()
	if !testkit.Eventually(t, 15*time.Second, func() bool { return runtime.NumGoroutine() <= baseline }) {
		var b strings.Builder
		_ = pprof.Lookup("goroutine").WriteTo(&b, 1)
		t.Fatalf("%d goroutines once the clients left, want at most the %d before they joined:\n%s", runtime.NumGoroutine(), baseline, b.String())
	}
}

// TestBurstOfJoins opens more rooms at once than the node has room for and joins every room to
// its maximum at the same moment (ADR-0101 §14): the reservations take the node to its capacity
// and never past it, every client connects and hears its room's speaker, and once they leave the
// node holds nothing and no goroutine of theirs is left.
func TestBurstOfJoins(t *testing.T) {
	const placed, maximum = 4, 10
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	reg := sfu.NewRegistry(testkit.NewKeyValueStore(clock), clock)
	rs := &rooms{state: map[string]appvoice.MediaRoom{}}
	e := newNodeEnvWith(t, "node-1", reg, reg, clock, rs, stream.NewHub(stream.HubOptions{}), placed*maximum)
	session := func(room, i int) appvoice.MediaSession {
		return appvoice.MediaSession{ID: fmt.Sprintf("s-%d-%d", room, i), AccountID: fmt.Sprintf("a-%d-%d", room, i), CanSpeak: i == 0}
	}
	for r := range placed + 1 {
		st := appvoice.MediaRoom{Open: true, MaxParticipants: maximum}
		for i := range maximum {
			st.Sessions = append(st.Sessions, session(r, i))
		}
		rs.set(fmt.Sprintf("room-%d", r), st)
	}

	// One room more than the capacity opens at the same moment: one of them finds no room.
	opened := make([]error, placed+1)
	var wg sync.WaitGroup
	for r := range placed + 1 {
		wg.Go(func() {
			opened[r] = e.transport.Open(t.Context(), appvoice.RoomRef{ID: fmt.Sprintf("room-%d", r), MaxParticipants: maximum})
		})
	}
	wg.Wait()
	var open []int
	full := -1
	for r, err := range opened {
		switch {
		case err == nil:
			open = append(open, r)
		case errors.Is(err, sfu.ErrNoCapacity):
			full = r
		default:
			t.Fatalf("Open room-%d: %v", r, err)
		}
	}
	if len(open) != placed || full < 0 {
		t.Fatalf("opens = %v, want %d placed and one refused with ErrNoCapacity", opened, placed)
	}
	reserved := 0
	for _, r := range open {
		a, ok, err := reg.Assignment(t.Context(), fmt.Sprintf("room-%d", r))
		if err != nil || !ok || a.Node != "node-1" {
			t.Fatalf("Assignment of room-%d = %+v, %t, %v, want node-1", r, a, ok, err)
		}
		reserved += a.Max
	}
	if reserved != placed*maximum {
		t.Errorf("reserved %d, want the node's capacity %d", reserved, placed*maximum)
	}
	if _, ok, err := reg.Assignment(t.Context(), fmt.Sprintf("room-%d", full)); err != nil || ok {
		t.Errorf("the refused room-%d is assigned: %t, %v", full, ok, err)
	}
	if _, err := e.dial(t, fmt.Sprintf("room-%d", full), "s-late", "a-late"); !errors.Is(err, sfu.ErrNoCapacity) {
		t.Errorf("a grant of the refused room = %v, want ErrNoCapacity", err)
	}

	// Every session of the placed rooms joins at the same moment.
	baseline := runtime.NumGoroutine()
	clients := make([][]*nodetest.Client, placed+1)
	for _, r := range open {
		clients[r] = make([]*nodetest.Client, maximum)
	}
	start := make(chan struct{})
	var joined sync.WaitGroup
	for _, r := range open {
		for i := range maximum {
			joined.Go(func() {
				<-start
				s := session(r, i)
				c, err := e.dial(t, fmt.Sprintf("room-%d", r), s.ID, s.AccountID)
				if err != nil {
					t.Errorf("join of %s: %v", s.ID, err)
					return
				}
				clients[r][i] = c
			})
		}
	}
	close(start)
	joined.Wait()
	if t.Failed() {
		t.FailNow()
	}
	var all []*nodetest.Client
	for _, r := range open {
		all = append(all, clients[r]...)
	}
	nodetest.WaitConnected(t, all...)
	if got := e.node.Sessions(); got != placed*maximum {
		t.Errorf("Sessions = %d, want %d", got, placed*maximum)
	}
	if got := e.node.Rooms(); got != placed {
		t.Errorf("Rooms = %d, want %d", got, placed)
	}

	ctx, stop := context.WithCancel(t.Context())
	var speaking sync.WaitGroup
	for _, r := range open {
		speaking.Go(func() { clients[r][0].Speak(ctx, 30) })
	}
	for _, r := range open {
		speaker := session(r, 0).AccountID
		for i, c := range clients[r][1:] {
			if !testkit.Eventually(t, 10*time.Second, func() bool { return c.HeardFrom(speaker) > 10 }) {
				t.Errorf("listener %d of room-%d heard %d packets of its speaker", i+1, r, c.HeardFrom(speaker))
			}
		}
	}
	stop()
	speaking.Wait()
	if tot := e.node.Totals(); tot.QueueFull != 0 {
		t.Errorf("totals = %+v, want no packet dropped for a full queue", tot)
	}

	for _, c := range all {
		c.Close()
	}
	if !testkit.Eventually(t, 15*time.Second, func() bool { return e.node.Sessions() == 0 && e.node.Rooms() == 0 }) {
		t.Fatalf("once the clients left: Sessions = %d, Rooms = %d, want 0", e.node.Sessions(), e.node.Rooms())
	}
	goroutinesSettle(t, baseline)
}
