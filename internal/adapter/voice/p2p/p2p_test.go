// SPDX-License-Identifier: AGPL-3.0-only

package p2p_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

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

// TestCoturnCredentials checks the credentials of coturn's use-auth-secret against a vector
// computed apart from the code: printf '1767225600:session-1' | openssl dgst -sha1 -hmac
// north-wind -binary | base64.
func TestCoturnCredentials(t *testing.T) {
	t.Parallel()
	clock := testkit.NewClock(time.Date(2025, 12, 31, 18, 0, 0, 500, time.UTC))
	c, err := p2p.NewCoturn([]string{"turn:turn.example.org:3478", "turns:turn.example.org:5349"}, "north-wind", clock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Credentials(t.Context(), appvoice.Session{ID: "session-1"}, 6*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := appvoice.ICEServer{ //nolint:gosec // G101: a test vector, not a credential
		URLs:     []string{"turn:turn.example.org:3478", "turns:turn.example.org:5349"},
		Username: "1767225600:session-1", Credential: "YEWSM93FRmapc7g+ntbByoYPzmk=",
	}
	if len(got.Servers) != 1 || !reflect.DeepEqual(got.Servers[0], want) || got.Reused || got.ExpiresAt.Unix() != 1767225600 {
		t.Errorf("Credentials = %+v, want %+v expiring at 1767225600", got, want)
	}
	if _, err := p2p.NewCoturn(nil, "north-wind", clock); err == nil {
		t.Error("NewCoturn without URLs = nil error")
	}
}

// turnSource is a TURN source that records what the transport asked.
type turnSource struct {
	fail    error
	revoked []string
	ttl     time.Duration
	expires time.Time
}

func (*turnSource) Name() string { return "coturn" }

func (s *turnSource) Credentials(_ context.Context, session appvoice.Session, ttl time.Duration) (p2p.Credentials, error) {
	s.ttl = ttl
	if s.fail != nil {
		return p2p.Credentials{}, s.fail
	}
	return p2p.Credentials{
		Servers:   []appvoice.ICEServer{{URLs: []string{"turn:turn.example.org"}, Username: session.ID, Credential: "c"}},
		ExpiresAt: s.expires,
	}, nil
}

func (s *turnSource) Revoke(_ context.Context, session appvoice.Session) error {
	s.revoked = append(s.revoked, session.ID)
	return s.fail
}

func TestGrantWithTURN(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	clock := testkit.VoiceTransportClock()
	meter := testkit.NewMeter()
	src := &turnSource{expires: clock.Now().Add(90 * time.Minute)}
	tr := p2p.New(p2p.Options{STUNURLs: []string{"stun:a.example.org"}, TURN: src, TTL: 2 * time.Hour, RelayOnly: true, Clock: clock, Meter: meter})
	room, s := appvoice.RoomRef{ID: "r"}, appvoice.Session{ID: "s1", AccountID: "a"}
	g, err := tr.Grant(ctx, room, s, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.ICE) != 2 || g.ICE[1].Username != "s1" || !g.RelayOnly || src.ttl != 2*time.Hour {
		t.Errorf("Grant = %+v (ttl %s), want the STUN and TURN servers", g, src.ttl)
	}
	if !g.ExpiresAt.Equal(src.expires) {
		t.Errorf("ExpiresAt = %s, want the expiry of the TURN credentials %s", g.ExpiresAt, src.expires)
	}
	if n := meter.Sum(p2p.TURNCredentials.Name, map[string]string{"my.voice.turn": "coturn", "my.outcome": p2p.OutcomeIssued}); n != 1 {
		t.Errorf("issued = %v, want 1", n)
	}

	if err := tr.Remove(ctx, room, s); err != nil || !slices.Equal(src.revoked, []string{"s1"}) {
		t.Errorf("Remove = %v, revoked %v; want the session revoked", err, src.revoked)
	}

	src.fail = errors.New("down")
	if _, err := tr.Grant(ctx, room, s, true); err == nil {
		t.Error("Grant with the TURN source failing = nil error")
	}
	if err := tr.Remove(ctx, room, s); err == nil {
		t.Error("Remove with the revoke failing = nil error, want one for voice.apply to retry")
	}
	if n := meter.Sum(p2p.TURNCredentials.Name, map[string]string{"my.outcome": p2p.OutcomeFailed}); n != 1 {
		t.Errorf("failed = %v, want 1", n)
	}
	if err := meter.Err(); err != nil {
		t.Error(err)
	}
}

func TestVoiceTransportContractWithCoturn(t *testing.T) {
	testkit.VoiceTransportContract(t, func(*testing.T) testkit.VoiceTransportHarness {
		clock := testkit.VoiceTransportClock()
		hb := &heartbeats{live: map[string][]appvoice.SessionRef{}}
		turn, err := p2p.NewCoturn([]string{"turn:turn.example.org"}, "secret", clock)
		if err != nil {
			t.Fatal(err)
		}
		tr := p2p.New(p2p.Options{TURN: turn, Sessions: hb, Clock: clock, Meter: testkit.NewMeter()})
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
