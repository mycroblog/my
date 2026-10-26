// SPDX-License-Identifier: AGPL-3.0-only

package livekit

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/livekittest"
)

const (
	testKey    = "APIkey"
	testSecret = "a-secret-of-the-api-key-for-tests"
)

func newTestTransport(t *testing.T, clock *testkit.Clock) (*Transport, *fakeServer) {
	t.Helper()
	f, srv := newFakeServer(t, testKey, testSecret, clock.Now)
	tr, err := New(Options{
		URL: "wss://livekit.example.org", APIURL: srv.URL + "/", APIKey: testKey, APISecret: testSecret,
		MaxParticipants: 50, HTTPClient: srv.Client(), Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tr, f
}

func TestVoiceTransportContract(t *testing.T) {
	testkit.VoiceTransportContract(t, func(t *testing.T) testkit.VoiceTransportHarness {
		clock := testkit.VoiceTransportClock()
		tr, f := newTestTransport(t, clock)
		return testkit.VoiceTransportHarness{
			Transport: tr, Clock: clock,
			Connect: func(_ *testing.T, room appvoice.RoomRef, s appvoice.Session) { f.connect(room.ID, s.ID) },
		}
	})
}

// TestVoiceTransportContractLiveKitServer runs the contract on livekit-server --dev
// (scripts/task.sh go:test-voice). Only a WebRTC client connects a participant, so the contract
// runs without Connect: it checks the calls of the API on rooms without participants.
func TestVoiceTransportContractLiveKitServer(t *testing.T) {
	srv := livekittest.New(t)
	testkit.VoiceTransportContract(t, func(t *testing.T) testkit.VoiceTransportHarness {
		clock := testkit.NewClock(time.Now())
		tr, err := New(Options{
			URL: srv.URL, APIURL: srv.APIURL, APIKey: livekittest.APIKey, APISecret: livekittest.APISecret,
			HTTPClient: &http.Client{Timeout: 5 * time.Second}, Clock: clock,
		})
		if err != nil {
			t.Fatal(err)
		}
		return testkit.VoiceTransportHarness{Transport: tr, Clock: clock}
	})
	t.Run("CreatesTheRoomWithItsOptions", func(t *testing.T) {
		tr, err := New(Options{
			URL: srv.URL, APIURL: srv.APIURL, APIKey: livekittest.APIKey, APISecret: livekittest.APISecret, MaxParticipants: 7,
			HTTPClient: &http.Client{Timeout: 5 * time.Second}, Clock: testkit.NewClock(time.Now()),
		})
		if err != nil {
			t.Fatal(err)
		}
		room := appvoice.RoomRef{ID: "room-options"}
		if err := tr.Open(t.Context(), room); err != nil {
			t.Fatal(err)
		}
		var res struct {
			Rooms []struct {
				Name             string `json:"name"`
				EmptyTimeout     int    `json:"empty_timeout"`
				DepartureTimeout int    `json:"departure_timeout"`
				MaxParticipants  int    `json:"max_participants"`
			} `json:"rooms"`
		}
		if err := tr.call(t.Context(), "ListRooms", "", map[string]any{"names": []string{room.ID}}, &res); err != nil {
			t.Fatal(err)
		}
		if len(res.Rooms) != 1 || res.Rooms[0].EmptyTimeout != 120 || res.Rooms[0].DepartureTimeout != 120 || res.Rooms[0].MaxParticipants != 7 {
			t.Errorf("ListRooms = %+v, want the room with its timeouts and limit", res.Rooms)
		}
		// UpdateParticipant of a session that has not connected asks for a reconnect.
		if reconnect, err := tr.SetSpeaking(t.Context(), room, appvoice.Session{ID: "absent"}, true); err != nil || !reconnect {
			t.Errorf("SetSpeaking of an absent session = %t, %v; want a reconnect", reconnect, err)
		}
	})
}

func decodeClaims(t *testing.T, token string) claims {
	t.Helper()
	c, err := verify(token, testKey, testSecret, testkit.VoiceTransportClock().Now())
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	return c
}

func TestGrant(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	tr, _ := newTestTransport(t, clock)
	room := appvoice.RoomRef{ID: "room-1"}
	s := appvoice.Session{ID: "session-1", AccountID: "account-1"}
	for _, speak := range []bool{true, false} {
		g, err := tr.Grant(t.Context(), room, s, speak)
		if err != nil {
			t.Fatal(err)
		}
		if g.Kind != Kind || g.URL != "wss://livekit.example.org" || !g.ExpiresAt.Equal(clock.Now().Add(GrantTTL)) || len(g.ICE) != 0 {
			t.Errorf("Grant(%t) = %+v", speak, g)
		}
		c := decodeClaims(t, g.Token)
		if c.Issuer != testKey || c.Subject != s.ID || c.ExpiresAt != clock.Now().Add(GrantTTL).Unix() {
			t.Errorf("Grant(%t) claims = %+v, want the session's identity for %s", speak, c, GrantTTL)
		}
		if strings.Contains(g.Token, s.AccountID) {
			t.Errorf("Grant(%t) token names the account", speak)
		}
		v := c.Video
		if v == nil || !v.RoomJoin || v.Room != room.ID || v.CanSubscribe == nil || !*v.CanSubscribe ||
			v.CanPublishData == nil || *v.CanPublishData || v.CanPublish == nil || *v.CanPublish != speak || v.RoomAdmin || v.RoomCreate {
			t.Errorf("Grant(%t) video grant = %+v", speak, v)
		}
		if want := []string(nil); !speak && !slices.Equal(v.CanPublishSources, want) {
			t.Errorf("listener's sources = %v, want none", v.CanPublishSources)
		}
		if speak && !slices.Equal(v.CanPublishSources, []string{"microphone"}) {
			t.Errorf("speaker's sources = %v, want the microphone only", v.CanPublishSources)
		}
	}
}

func TestOpenSetSpeakingMute(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	tr, f := newTestTransport(t, clock)
	room := appvoice.RoomRef{ID: "room-2"}
	s := appvoice.Session{ID: "session-2"}
	if err := tr.Open(t.Context(), room); err != nil {
		t.Fatal(err)
	}
	opts := f.rooms[room.ID].opts
	if opts["max_participants"] != float64(50) || opts["empty_timeout"] != float64(120) || opts["departure_timeout"] != float64(120) {
		t.Errorf("CreateRoom = %v", opts)
	}
	// A session that has not connected gets a new grant.
	if reconnect, err := tr.SetSpeaking(t.Context(), room, s, true); err != nil || !reconnect {
		t.Errorf("SetSpeaking before connecting = %t, %v; want a reconnect", reconnect, err)
	}
	f.connect(room.ID, s.ID)
	if reconnect, err := tr.SetSpeaking(t.Context(), room, s, true); err != nil || reconnect {
		t.Errorf("SetSpeaking = %t, %v; want no reconnect", reconnect, err)
	}
	perm := f.participant(room.ID, s.ID).permission
	if perm["can_publish"] != true || perm["can_subscribe"] != true || perm["can_publish_data"] != false ||
		!slices.Equal(anyStrings(perm["can_publish_sources"]), []string{"MICROPHONE"}) {
		t.Errorf("permission of a speaker = %v", perm)
	}
	if _, err := tr.SetSpeaking(t.Context(), room, s, false); err != nil {
		t.Fatal(err)
	}
	perm = f.participant(room.ID, s.ID).permission
	if perm["can_publish"] != false || perm["can_publish_sources"] != nil {
		t.Errorf("permission of a listener = %v", perm)
	}
	if err := tr.Mute(t.Context(), room, s); err != nil {
		t.Fatal(err)
	}
	if muted := f.participant(room.ID, s.ID).tracks[0]["muted"]; muted != true {
		t.Errorf("track muted = %v, want true", muted)
	}
	if err := tr.Mute(t.Context(), room, appvoice.Session{ID: "absent"}); err != nil {
		t.Errorf("Mute of an absent session: %v", err)
	}
}

func anyStrings(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, s := range l {
		str, _ := s.(string)
		out = append(out, str)
	}
	return out
}

func TestCallFailure(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	tr, _ := newTestTransport(t, clock)
	// A wrong secret: the API refuses the token.
	tr.o.APISecret = "another secret"
	err := tr.Open(t.Context(), appvoice.RoomRef{ID: "room"})
	var e *apiError
	if !errors.As(err, &e) || e.Status != http.StatusUnauthorized || isNotFound(err) {
		t.Errorf("Open with a wrong secret = %v, want 401", err)
	}
}

// webhook returns the header and body of a delivery of event, signed as LiveKit signs them.
func webhook(t *testing.T, key, secret string, now time.Time, event map[string]any) (http.Header, []byte) {
	t.Helper()
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	token, err := sign(claims{
		Issuer: key, NotBefore: now.Unix(), ExpiresAt: now.Add(10 * time.Minute).Unix(), SHA256: base64.StdEncoding.EncodeToString(sum[:]),
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	h.Set("Authorization", token)
	h.Set("Content-Type", "application/webhook+json")
	return h, body
}

func TestParseEvents(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	tr, _ := newTestTransport(t, clock)
	now := clock.Now()
	joined := map[string]any{
		"event": "participant_joined", "id": "EV_1", "createdAt": strconv.FormatInt(now.Unix(), 10),
		"room": map[string]any{"sid": "RM_1", "name": "room-1"}, "participant": map[string]any{"sid": "PA_1", "identity": "session-1"},
	}
	h, body := webhook(t, testKey, testSecret, now, joined)
	evs, err := tr.ParseEvents(t.Context(), h, body)
	if err != nil {
		t.Fatal(err)
	}
	want := appvoice.Event{ID: "EV_1", Kind: appvoice.MediaSessionJoined, Room: appvoice.RoomRef{ID: "room-1"}, Session: "session-1", OccurredAt: now}
	if len(evs) != 1 || evs[0] != want {
		t.Errorf("ParseEvents = %+v, want %+v", evs, want)
	}

	for name, ev := range map[string]struct {
		event map[string]any
		want  appvoice.EventKind
	}{
		"left":     {map[string]any{"event": "participant_left", "id": "EV_2", "room": map[string]any{"name": "r"}, "participant": map[string]any{"identity": "s"}}, appvoice.MediaSessionLeft},
		"aborted":  {map[string]any{"event": "participant_connection_aborted", "id": "EV_3", "room": map[string]any{"name": "r"}, "participant": map[string]any{"identity": "s"}}, appvoice.MediaSessionLeft},
		"finished": {map[string]any{"event": "room_finished", "id": "EV_4", "room": map[string]any{"name": "r"}}, appvoice.MediaRoomFinished},
		"ignored":  {map[string]any{"event": "track_published", "id": "EV_5", "room": map[string]any{"name": "r"}}, ""},
	} {
		h, body := webhook(t, testKey, testSecret, now, ev.event)
		evs, err := tr.ParseEvents(t.Context(), h, body)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if ev.want == "" {
			if len(evs) != 0 {
				t.Errorf("%s: ParseEvents = %+v, want none", name, evs)
			}
			continue
		}
		if len(evs) != 1 || evs[0].Kind != ev.want || evs[0].Room.ID != "r" {
			t.Errorf("%s: ParseEvents = %+v, want %s", name, evs, ev.want)
		}
	}
}

func TestParseEventsRejects(t *testing.T) {
	clock := testkit.VoiceTransportClock()
	tr, _ := newTestTransport(t, clock)
	now := clock.Now()
	event := map[string]any{
		"event": "participant_joined", "id": "EV_1", "room": map[string]any{"name": "room-1"}, "participant": map[string]any{"identity": "session-1"},
	}
	h, body := webhook(t, testKey, testSecret, now, event)
	tampered := []byte(strings.Replace(string(body), "session-1", "session-2", 1))
	other, _ := webhook(t, testKey, "another secret", now, event)
	otherKey, _ := webhook(t, "another key", testSecret, now, event)
	expired, _ := webhook(t, testKey, testSecret, now.Add(-time.Hour), event)
	unsigned := strings.Join(strings.Split(h.Get("Authorization"), ".")[:2], ".") + "."
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + strings.Split(h.Get("Authorization"), ".")[1] + "."
	for name, c := range map[string]struct {
		auth string
		body []byte
	}{
		"tampered body":    {h.Get("Authorization"), tampered},
		"wrong secret":     {other.Get("Authorization"), body},
		"wrong key":        {otherKey.Get("Authorization"), body},
		"expired":          {expired.Get("Authorization"), body},
		"no signature":     {unsigned, body},
		"alg none":         {none, body},
		"no authorization": {"", body},
		"garbage":          {"not a token", body},
	} {
		hh := http.Header{}
		if c.auth != "" {
			hh.Set("Authorization", c.auth)
		}
		if _, err := tr.ParseEvents(t.Context(), hh, c.body); !errors.Is(err, ErrWebhookSignature) {
			t.Errorf("%s: ParseEvents = %v, want ErrWebhookSignature", name, err)
		}
	}
	// A signed body that is not an event.
	h, body = webhook(t, testKey, testSecret, now, map[string]any{"event": "participant_joined", "id": "EV_9"})
	if _, err := tr.ParseEvents(t.Context(), h, body); !errors.Is(err, ErrWebhookBody) {
		t.Errorf("an event without a room: %v, want ErrWebhookBody", err)
	}
	// The Bearer form of the header verifies too.
	h, body = webhook(t, testKey, testSecret, now, event)
	h.Set("Authorization", "Bearer "+h.Get("Authorization"))
	if _, err := tr.ParseEvents(t.Context(), h, body); err != nil {
		t.Errorf("Bearer token: %v", err)
	}
}
