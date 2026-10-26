// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"slices"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
)

// VoiceTransportHarness is what VoiceTransportContract needs from a transport adapter: the
// transport, the clock of its grants, and Connect, which makes a session seen by the media path
// of a room as a client that connected with its grant would (a heartbeat for p2p).
type VoiceTransportHarness struct {
	Transport appvoice.Transport
	Clock     *Clock
	Connect   func(t *testing.T, room appvoice.RoomRef, s appvoice.Session)
}

// VoiceTransportContract verifies a voice transport adapter (ADR-0101 §2): Open and Close are
// idempotent; a grant names the method and expires in the future, and its ICE servers have
// URLs; SetSpeaking reports a reconnect only for a transport whose server enforces speaking;
// Mute, Unmute, and Remove accept a session twice; Sessions lists the connected sessions, and
// on a transport whose server enforces speaking, never one it removed or of a room it closed.
func VoiceTransportContract(t *testing.T, newHarness func(t *testing.T) VoiceTransportHarness) {
	t.Helper()
	t.Run("OpensAndClosesIdempotently", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		room := appvoice.RoomRef{ID: "room-open"}
		for range 2 {
			if err := h.Transport.Open(t.Context(), room); err != nil {
				t.Fatalf("Open: %v", err)
			}
		}
		for range 2 {
			if err := h.Transport.Close(t.Context(), room); err != nil {
				t.Fatalf("Close: %v", err)
			}
		}
	})
	t.Run("GrantsASession", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		room := appvoice.RoomRef{ID: "room-grant"}
		if err := h.Transport.Open(t.Context(), room); err != nil {
			t.Fatal(err)
		}
		for _, speak := range []bool{true, false} {
			g, err := h.Transport.Grant(t.Context(), room, appvoice.Session{ID: "session-1", AccountID: "account-1"}, speak)
			if err != nil {
				t.Fatalf("Grant(%t): %v", speak, err)
			}
			if g.Kind == "" || !g.ExpiresAt.After(h.Clock.Now()) {
				t.Errorf("Grant(%t) = %+v, want a kind and an expiry after now", speak, g)
			}
			for _, s := range g.ICE {
				if len(s.URLs) == 0 {
					t.Errorf("Grant(%t) ICE server %+v has no URL", speak, s)
				}
			}
		}
	})
	t.Run("ChangesASession", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		room := appvoice.RoomRef{ID: "room-change"}
		s := appvoice.Session{ID: "session-2", AccountID: "account-2"}
		if err := h.Transport.Open(t.Context(), room); err != nil {
			t.Fatal(err)
		}
		h.Connect(t, room, s)
		for _, speak := range []bool{true, false} {
			reconnect, err := h.Transport.SetSpeaking(t.Context(), room, s, speak)
			if err != nil {
				t.Fatalf("SetSpeaking(%t): %v", speak, err)
			}
			if reconnect && !h.Transport.Capabilities().ServerEnforced {
				t.Errorf("SetSpeaking(%t) asks a reconnect of a transport that does not enforce speaking", speak)
			}
		}
		for range 2 {
			if err := h.Transport.Mute(t.Context(), room, s); err != nil {
				t.Fatalf("Mute: %v", err)
			}
			if err := h.Transport.Unmute(t.Context(), room, s); err != nil {
				t.Fatalf("Unmute: %v", err)
			}
		}
	})
	t.Run("ListsAndRemovesSessions", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		room := appvoice.RoomRef{ID: "room-sessions"}
		a := appvoice.Session{ID: "session-a", AccountID: "account-a"}
		b := appvoice.Session{ID: "session-b", AccountID: "account-b"}
		if err := h.Transport.Open(t.Context(), room); err != nil {
			t.Fatal(err)
		}
		h.Connect(t, room, a)
		h.Connect(t, room, b)
		ids := func() []string {
			t.Helper()
			ss, err := h.Transport.Sessions(t.Context(), room)
			if err != nil {
				t.Fatalf("Sessions: %v", err)
			}
			var out []string
			for _, s := range ss {
				out = append(out, s.ID+"/"+s.AccountID)
			}
			slices.Sort(out)
			return out
		}
		if got, want := ids(), []string{"session-a/account-a", "session-b/account-b"}; !slices.Equal(got, want) {
			t.Fatalf("Sessions = %v, want %v", got, want)
		}
		for range 2 {
			if err := h.Transport.Remove(t.Context(), room, a); err != nil {
				t.Fatalf("Remove: %v", err)
			}
		}
		if !h.Transport.Capabilities().ServerEnforced {
			return
		}
		if got, want := ids(), []string{"session-b/account-b"}; !slices.Equal(got, want) {
			t.Errorf("Sessions after Remove = %v, want %v", got, want)
		}
		if err := h.Transport.Close(t.Context(), room); err != nil {
			t.Fatal(err)
		}
		if got := ids(); len(got) != 0 {
			t.Errorf("Sessions after Close = %v, want none", got)
		}
	})
}

// VoiceTransportClock returns a clock for the harness of VoiceTransportContract.
func VoiceTransportClock() *Clock { return NewClock(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)) }
