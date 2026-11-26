// SPDX-License-Identifier: AGPL-3.0-only

package sfu_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/pkg/testkit"
)

func newRegistry(t *testing.T, nodes ...sfu.NodeLease) (*sfu.Registry, *testkit.Clock) {
	t.Helper()
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	reg := sfu.NewRegistry(testkit.NewKeyValueStore(clock), clock)
	for _, n := range nodes {
		if err := reg.Register(t.Context(), n); err != nil {
			t.Fatal(err)
		}
	}
	return reg, clock
}

// TestPlaceNeverPassesTheCapacity places rooms from many goroutines at once: each room gets one
// node, no node reserves past its capacity, and the rooms that fit nowhere get ErrNoCapacity.
func TestPlaceNeverPassesTheCapacity(t *testing.T) {
	reg, _ := newRegistry(t, sfu.NodeLease{ID: "a", URL: "wss://a", Capacity: 25}, sfu.NodeLease{ID: "b", URL: "wss://b", Capacity: 25})
	var wg sync.WaitGroup
	var mu sync.Mutex
	placed, refused := map[string]string{}, 0
	for i := range 12 {
		room := fmt.Sprintf("room-%d", i)
		for range 3 { // three web processes place the same room
			wg.Go(func() {
				n, err := reg.Place(t.Context(), room, 10, "")
				mu.Lock()
				defer mu.Unlock()
				switch {
				case errors.Is(err, sfu.ErrNoCapacity):
					refused++
				case err != nil:
					t.Errorf("Place(%s): %v", room, err)
				case placed[room] != "" && placed[room] != n.ID:
					t.Errorf("Place(%s) = %s and %s", room, placed[room], n.ID)
				default:
					placed[room] = n.ID
				}
			})
		}
	}
	wg.Wait()
	per := map[string]int{}
	for room, node := range placed {
		a, ok, err := reg.Assignment(t.Context(), room)
		if err != nil || !ok || a.Node != node {
			t.Errorf("Assignment(%s) = %+v, %t, %v, want node %s", room, a, ok, err, node)
		}
		per[node] += 10
	}
	if per["a"] > 25 || per["b"] > 25 || len(placed) != 4 {
		t.Errorf("placed %d rooms, reserved %v; want 4 rooms, at most 25 a node", len(placed), per)
	}
}

// TestPlaceMovesAwayFromADrainingNode places a room again when its node drains or is gone, and
// keeps it on a draining node when no other node has room.
func TestPlaceMovesAwayFromADrainingNode(t *testing.T) {
	reg, _ := newRegistry(t, sfu.NodeLease{ID: "a", URL: "wss://a", Capacity: 10})
	n, err := reg.Place(t.Context(), "room", 10, "")
	if err != nil || n.ID != "a" {
		t.Fatalf("Place = %+v, %v, want node a", n, err)
	}
	if err := reg.Register(t.Context(), sfu.NodeLease{ID: "a", URL: "wss://a", Capacity: 10, Draining: true}); err != nil {
		t.Fatal(err)
	}
	if n, err := reg.Place(t.Context(), "room", 10, "a"); err != nil || n.ID != "a" {
		t.Errorf("Place with no other node = %+v, %v, want it kept on the draining node a", n, err)
	}
	if err := reg.Register(t.Context(), sfu.NodeLease{ID: "b", URL: "wss://b", Capacity: 10}); err != nil {
		t.Fatal(err)
	}
	if n, err := reg.Place(t.Context(), "room", 10, "a"); err != nil || n.ID != "b" {
		t.Errorf("Place away from a = %+v, %v, want node b", n, err)
	}
	if err := reg.Deregister(t.Context(), "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Place(t.Context(), "other", 10, ""); !errors.Is(err, sfu.ErrNoCapacity) {
		t.Errorf("Place with only a draining node = %v, want ErrNoCapacity", err)
	}
}

// TestConsumeOnce uses a ticket once, and refuses one of another node.
func TestConsumeOnce(t *testing.T) {
	reg, clock := newRegistry(t)
	tok, err := reg.Issue(t.Context(), sfu.Ticket{Room: "r", Session: "s", Account: "a", Node: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reg.Consume(t.Context(), tok, "n"); err != nil || got.Session != "s" {
		t.Errorf("Consume = %+v, %v", got, err)
	}
	if _, err := reg.Consume(t.Context(), tok, "n"); !errors.Is(err, sfu.ErrInvalidTicket) {
		t.Errorf("Consume again = %v, want ErrInvalidTicket", err)
	}
	other, _ := reg.Issue(t.Context(), sfu.Ticket{Room: "r", Session: "s", Node: "m"})
	if _, err := reg.Consume(t.Context(), other, "n"); !errors.Is(err, sfu.ErrOtherNode) {
		t.Errorf("Consume of another node's ticket = %v, want ErrOtherNode", err)
	}
	late, _ := reg.Issue(t.Context(), sfu.Ticket{Room: "r", Session: "s", Node: "n"})
	clock.Advance(sfu.TicketTTL + time.Second)
	if _, err := reg.Consume(t.Context(), late, "n"); !errors.Is(err, sfu.ErrInvalidTicket) {
		t.Errorf("Consume of an expired ticket = %v, want ErrInvalidTicket", err)
	}
}
