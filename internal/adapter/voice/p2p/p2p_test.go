// SPDX-License-Identifier: AGPL-3.0-only

package p2p_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/voice/p2p"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

// heartbeats is the live sessions of the rooms, as their heartbeats tell them.
type heartbeats struct {
	mu   sync.Mutex
	live map[string][]appvoice.SessionRef
}

func (h *heartbeats) LiveSessions(_ context.Context, roomID string) ([]appvoice.SessionRef, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.live[roomID]), nil
}

func TestVoiceTransportContract(t *testing.T) {
	testkit.VoiceTransportContract(t, func(*testing.T) testkit.VoiceTransportHarness {
		clock := testkit.VoiceTransportClock()
		hb := &heartbeats{live: map[string][]appvoice.SessionRef{}}
		tr := p2p.New(p2p.Options{STUNURLs: []string{"stun:stun.example.org:3478"}, Sessions: hb, Clock: clock})
		return testkit.VoiceTransportHarness{
			Transport: tr, Clock: clock,
			Connect: func(_ *testing.T, room appvoice.RoomRef, s appvoice.Session) {
				hb.mu.Lock()
				defer hb.mu.Unlock()
				hb.live[room.ID] = append(hb.live[room.ID], appvoice.SessionRef(s))
			},
		}
	})
}

func TestGrant(t *testing.T) {
	t.Parallel()
	clock := testkit.VoiceTransportClock()
	tr := p2p.New(p2p.Options{STUNURLs: []string{"stun:a.example.org", "stuns:b.example.org"}, RelayOnly: true, Clock: clock})
	g, err := tr.Grant(t.Context(), appvoice.RoomRef{ID: "r"}, appvoice.Session{ID: "s"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if g.Kind != p2p.Kind || g.URL != "" || g.Token != "" || !g.RelayOnly || !g.ExpiresAt.Equal(clock.Now().Add(p2p.GrantTTL)) {
		t.Errorf("Grant = %+v", g)
	}
	if len(g.ICE) != 1 || !slices.Equal(g.ICE[0].URLs, []string{"stun:a.example.org", "stuns:b.example.org"}) {
		t.Errorf("ICE = %+v, want the STUN servers", g.ICE)
	}
	if c := tr.Capabilities(); c.ServerEnforced || c.ConnectCallback || c.Events {
		t.Errorf("Capabilities = %+v, want none", c)
	}
	none, _ := p2p.New(p2p.Options{Clock: clock}).Grant(t.Context(), appvoice.RoomRef{ID: "r"}, appvoice.Session{ID: "s"}, false)
	if len(none.ICE) != 0 {
		t.Errorf("ICE without STUN = %+v, want none", none.ICE)
	}
}
