// SPDX-License-Identifier: AGPL-3.0-only

package node_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

var errUnreachable = errors.New("the node cannot reach the KeyValueStore")

// unreachableKV is the KeyValueStore as a node sees it: once cut, the node reaches nothing of it,
// as a node that crashed or that the network cut off, so that it renews none of its leases.
type unreachableKV struct {
	kernel.KeyValueStore
	cut atomic.Bool
}

func (k *unreachableKV) Get(ctx context.Context, ns, key string) ([]byte, error) {
	if k.cut.Load() {
		return nil, errUnreachable
	}
	return k.KeyValueStore.Get(ctx, ns, key)
}

func (k *unreachableKV) Set(ctx context.Context, ns, key string, v []byte, ttl time.Duration) error {
	if k.cut.Load() {
		return errUnreachable
	}
	return k.KeyValueStore.Set(ctx, ns, key, v, ttl)
}

func (k *unreachableKV) Delete(ctx context.Context, ns, key string) error {
	if k.cut.Load() {
		return errUnreachable
	}
	return k.KeyValueStore.Delete(ctx, ns, key)
}

func (k *unreachableKV) IncrBy(ctx context.Context, ns, key string, n int64, ttl time.Duration) (int64, error) {
	if k.cut.Load() {
		return 0, errUnreachable
	}
	return k.KeyValueStore.IncrBy(ctx, ns, key, n, ttl)
}

func (k *unreachableKV) CompareAndSwap(ctx context.Context, ns, key string, old, v []byte, ttl time.Duration) (bool, error) {
	if k.cut.Load() {
		return false, errUnreachable
	}
	return k.KeyValueStore.CompareAndSwap(ctx, ns, key, old, v, ttl)
}

// TestLostNodeRoomsArePlacedAgain loses the node of a room with another node running (ADR-0101
// §14): the node crashes, its clients lose it, and its leases expire; the next grants of the
// room's sessions place the room on the other node, where both speakers connect with the same
// sessions and hear each other again.
func TestLostNodeRoomsArePlacedAgain(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	kv := testkit.NewKeyValueStore(clock)
	reg := sfu.NewRegistry(kv, clock)
	reach := &unreachableKV{KeyValueStore: kv}
	rs := &rooms{state: map[string]appvoice.MediaRoom{}}
	hub := stream.NewHub(stream.HubOptions{})
	first := newNodeEnvOn(t, "node-1", sfu.NewRegistry(reach, clock), reg, clock, rs, hub)
	second := newNodeEnvOn(t, "node-2", reg, reg, clock, rs, hub)
	rs.set("room-4", appvoice.MediaRoom{Open: true, MaxParticipants: 10, Sessions: []appvoice.MediaSession{
		{ID: "s-1", AccountID: "a-1", CanSpeak: true},
		{ID: "s-2", AccountID: "a-2", CanSpeak: true},
	}})
	// Both nodes reserve nothing: the room goes to the first by ID.
	if err := first.transport.Open(t.Context(), appvoice.RoomRef{ID: "room-4", MaxParticipants: 10}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	connected := func(cs ...*client) func() bool {
		return func() bool {
			for _, c := range cs {
				if c.pc.ConnectionState() != webrtc.PeerConnectionStateConnected {
					return false
				}
			}
			return true
		}
	}
	hearEachOther := func(a, b *client, on *env) {
		t.Helper()
		if !testkit.Eventually(t, 10*time.Second, connected(a, b)) {
			t.Fatalf("not connected to %s: %s, %s", on.node.ID(), a.pc.ConnectionState(), b.pc.ConnectionState())
		}
		ctx, stop := context.WithCancel(t.Context())
		defer stop()
		go a.speak(ctx, 30)
		go b.speak(ctx, 30)
		if !testkit.Eventually(t, 10*time.Second, func() bool { return a.heardFrom("a-2") > 10 && b.heardFrom("a-1") > 10 }) {
			t.Fatalf("on %s, s-1 heard %d packets of a-2 and s-2 %d of a-1; totals %+v", on.node.ID(), a.heardFrom("a-2"), b.heardFrom("a-1"), on.node.Totals())
		}
	}
	old1 := first.connect(t, "room-4", "s-1", "a-1")
	old2 := first.connect(t, "room-4", "s-2", "a-2")
	hearEachOther(old1, old2, first)
	if a, ok, err := reg.Assignment(t.Context(), "room-4"); err != nil || !ok || a.Node != "node-1" {
		t.Fatalf("Assignment = %+v, %t, %v, want node-1", a, ok, err)
	}

	// node-1 is lost: it reaches the KeyValueStore no more, its clients lose it, and its leases
	// expire.
	reach.cut.Store(true)
	_ = first.node.Close(context.Background())
	for _, c := range []*client{old1, old2} {
		if !testkit.Eventually(t, 10*time.Second, func() bool { return c.pc.ConnectionState() != webrtc.PeerConnectionStateConnected }) {
			t.Fatalf("a client of the lost node is still connected")
		}
		_ = c.pc.Close()
	}
	clock.Advance(sfu.LeaseTTL + time.Second)
	if l, ok, err := reg.Lease(t.Context(), "node-1"); err != nil || ok {
		t.Fatalf("Lease of the lost node = %+v, %t, %v, want none", l, ok, err)
	}
	// The clock passed the lease of node-2 too, which renews it on its next round.
	if !testkit.Eventually(t, 2*sfu.LeaseRenewal, func() bool { _, ok, _ := reg.Lease(t.Context(), "node-2"); return ok }) {
		t.Fatal("node-2 did not renew its lease")
	}

	for _, s := range []appvoice.Session{{ID: "s-1", AccountID: "a-1"}, {ID: "s-2", AccountID: "a-2"}} {
		g, err := second.transport.Grant(t.Context(), appvoice.RoomRef{ID: "room-4"}, s, true)
		if err != nil {
			t.Fatalf("Grant of %s after the loss: %v", s.ID, err)
		}
		if g.URL != second.url {
			t.Fatalf("Grant of %s after the loss names %s, want node-2 %s", s.ID, g.URL, second.url)
		}
	}
	if a, ok, err := reg.Assignment(t.Context(), "room-4"); err != nil || !ok || a.Node != "node-2" {
		t.Fatalf("Assignment after the grants = %+v, %t, %v, want node-2", a, ok, err)
	}
	hearEachOther(second.connect(t, "room-4", "s-1", "a-1"), second.connect(t, "room-4", "s-2", "a-2"), second)
}
