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

// TestLostNodeRoomsArePlacedAgain loses the node of a room (ADR-0101 §14): the node stops
// renewing, its leases expire, and the next grant of the room's session places the room on the
// other node, where the speaker connects with the same session and a listener hears them.
func TestLostNodeRoomsArePlacedAgain(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	kv := testkit.NewKeyValueStore(clock)
	reg := sfu.NewRegistry(kv, clock)
	reach := &unreachableKV{KeyValueStore: kv}
	rs := &rooms{state: map[string]appvoice.MediaRoom{}}
	hub := stream.NewHub(stream.HubOptions{})
	first := newNodeEnvOn(t, "node-1", sfu.NewRegistry(reach, clock), reg, clock, rs, hub)
	rs.set("room-4", appvoice.MediaRoom{Open: true, MaxParticipants: 10, Sessions: []appvoice.MediaSession{
		{ID: "s-1", AccountID: "a-1", CanSpeak: true},
		{ID: "s-2", AccountID: "a-2"},
	}})
	if err := first.transport.Open(t.Context(), appvoice.RoomRef{ID: "room-4", MaxParticipants: 10}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	old := first.connect(t, "room-4", "s-1", "a-1")
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
	if !testkit.Eventually(t, 10*time.Second, connected(old)) {
		t.Fatalf("the speaker did not connect to node-1: %s", old.pc.ConnectionState())
	}
	if a, ok, err := reg.Assignment(t.Context(), "room-4"); err != nil || !ok || a.Node != "node-1" {
		t.Fatalf("Assignment = %+v, %t, %v, want node-1", a, ok, err)
	}

	// node-1 is lost: it renews nothing, and its leases expire.
	reach.cut.Store(true)
	clock.Advance(sfu.LeaseTTL + time.Second)
	if l, ok, err := reg.Lease(t.Context(), "node-1"); err != nil || ok {
		t.Fatalf("Lease of the lost node = %+v, %t, %v, want none", l, ok, err)
	}
	second := newNodeEnvOn(t, "node-2", reg, reg, clock, rs, hub)

	g, err := second.transport.Grant(t.Context(), appvoice.RoomRef{ID: "room-4"}, appvoice.Session{ID: "s-1", AccountID: "a-1"}, true)
	if err != nil {
		t.Fatalf("Grant after the loss: %v", err)
	}
	if g.URL != second.url {
		t.Fatalf("Grant after the loss names %s, want node-2 %s", g.URL, second.url)
	}
	if a, ok, err := reg.Assignment(t.Context(), "room-4"); err != nil || !ok || a.Node != "node-2" {
		t.Fatalf("Assignment after the grant = %+v, %t, %v, want node-2", a, ok, err)
	}
	_ = old.pc.Close()
	speaker := second.connect(t, "room-4", "s-1", "a-1")
	listener := second.connect(t, "room-4", "s-2", "a-2")
	if !testkit.Eventually(t, 10*time.Second, connected(speaker, listener)) {
		t.Fatalf("not connected to node-2: speaker %s, listener %s", speaker.pc.ConnectionState(), listener.pc.ConnectionState())
	}
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go speaker.speak(ctx, 30)
	if !testkit.Eventually(t, 10*time.Second, func() bool { return listener.heardFrom("a-1") > 10 }) {
		t.Fatalf("the listener heard %d packets of the speaker on node-2; totals %+v", listener.heardFrom("a-1"), second.node.Totals())
	}
}
