// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// foreignRoom is a well-formed UUIDv7 that names no room of the instance: a room of another
// instance on the same media server.
const foreignRoom = "01936f5e-1b2c-7d3e-8f40-123456789abc"

// mediaServer is an httptest stand-in for the API of a media server shared by several
// instances. It records every call as "<operation> <room>" and answers each with an empty
// success, so that what the instance asks of it is all a test reads.
type mediaServer struct {
	// room reads the operation and the room of a request.
	room func(r *http.Request, body map[string]any) (op, room string)
	// answer is the body of the answer to an operation.
	answer func(op string) any

	mu    sync.Mutex
	calls []string
}

func newMediaServer(t *testing.T, ms *mediaServer) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		op, room := ms.room(r, body)
		ms.mu.Lock()
		ms.calls = append(ms.calls, op+" "+room)
		ms.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ms.answer(op))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// liveKitServer answers the Twirp API of LiveKit's RoomService.
func liveKitServer() *mediaServer {
	return &mediaServer{
		room: func(r *http.Request, body map[string]any) (string, string) {
			op := strings.TrimPrefix(r.URL.Path, "/twirp/livekit.RoomService/")
			room, _ := body["room"].(string)
			if op == "CreateRoom" {
				room, _ = body["name"].(string)
			}
			return op, room
		},
		answer: func(op string) any {
			switch op {
			case "ListParticipants":
				return map[string]any{"participants": []any{}}
			case "ListRooms":
				return map[string]any{"rooms": []any{map[string]any{"name": foreignRoom, "sid": "RM_foreign"}}}
			}
			return map[string]any{}
		},
	}
}

// soraServer answers the HTTP API of a Sora server, whose operation is the header x-sora-target.
func soraServer() *mediaServer {
	return &mediaServer{
		room: func(r *http.Request, body map[string]any) (string, string) {
			ch, _ := body["channel_id"].(string)
			return r.Header.Get("x-sora-target"), ch
		},
		answer: func(op string) any {
			if strings.Contains(op, "ListChannelConnections") {
				return []any{}
			}
			return map[string]any{}
		},
	}
}

// Calls returns the calls so far.
func (ms *mediaServer) Calls() []string {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return slices.Clone(ms.calls)
}

// newLiveKitApp starts the server with voice = livekit on a LiveKit API at apiURL.
func newLiveKitApp(t *testing.T, apiURL string) *platform.App {
	t.Helper()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "livekit",
		"MY_VOICE_LIVEKIT_URL": "wss://livekit.example.org", "MY_VOICE_LIVEKIT_API_URL": apiURL,
		"MY_VOICE_LIVEKIT_API_KEY": liveKitKey, "MY_VOICE_LIVEKIT_API_SECRET": liveKitSecret,
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

const liveKitKey, liveKitSecret = "APIkey", "a-secret-of-the-livekit-api-key"

// liveKitJoined is a participant_joined event of LiveKit for a session in room.
func liveKitJoined(id, room, session string) []byte {
	b, _ := json.Marshal(map[string]any{
		"event": "participant_joined", "id": id, "createdAt": "1793000000",
		"room":        map[string]any{"name": room, "sid": "RM_" + id},
		"participant": map[string]any{"identity": session, "sid": "PA_" + id},
	})
	return b
}

// drainMediaEvents runs the queued jobs and checks that no voice.apply_media_event is left to
// run again: the events ended as done, not as a retry.
func drainMediaEvents(t *testing.T, app *platform.App) {
	t.Helper()
	if _, err := app.Runner.Drain(t.Context()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	// The summary counts the pending jobs of every state and the dead ones: a retry or a
	// failure of the event leaves its job there.
	sum, err := app.JobInspector.SummarizeJobs(t.Context(), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("SummarizeJobs: %v", err)
	}
	for _, c := range sum.Counts {
		if c.Type == appvoice.JobApplyMediaEvent {
			t.Errorf("a %s did not end as done: %+v", c.Type, c)
		}
	}
}

// TestVoiceLiveKitIgnoresTheEventsOfOtherRooms checks #3453 on LiveKit: an event of the media
// server for a room the instance does not have, whether its name is a UUIDv7 or not one of the
// instance's form, ends as done and asks nothing of the media server, so no participant of
// another instance's room is removed.
func TestVoiceLiveKitIgnoresTheEventsOfOtherRooms(t *testing.T) {
	t.Parallel()
	for name, room := range map[string]string{"a UUIDv7": foreignRoom, "a name of another form": "other-room"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lk := liveKitServer()
			app := newLiveKitApp(t, newMediaServer(t, lk).URL)
			body := liveKitJoined("EV_1", room, "session-of-another-instance")
			if code := postWebhook(t, app.Handler(), liveKitWebhookToken(liveKitKey, liveKitSecret, body), body); code != http.StatusNoContent {
				t.Fatalf("signed delivery = %d, want 204", code)
			}
			drainMediaEvents(t, app)
			if calls := lk.Calls(); len(calls) != 0 {
				t.Errorf("the event of a room the instance does not have called the media server: %v", calls)
			}
		})
	}
}

// TestVoiceSoraIgnoresTheEventsOfOtherRooms checks #3453 on a self-hosted Sora: an event of a
// channel that is no room of the instance ends as done and asks nothing of the media server.
func TestVoiceSoraIgnoresTheEventsOfOtherRooms(t *testing.T) {
	t.Parallel()
	for name, room := range map[string]string{"a UUIDv7": foreignRoom, "a name of another form": "other-room"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sora := soraServer()
			cfg := loadConfig(t, map[string]string{
				"MY_DATABASE_URL": newDatabase(t, true), "MY_VOICE": "sora", "MY_VOICE_SORA_MODE": "self_hosted",
				"MY_VOICE_SORA_SIGNALING_URLS": "wss://sora.example.org/signaling", "MY_VOICE_SORA_API_URL": newMediaServer(t, sora).URL,
				"MY_VOICE_SORA_WEBHOOK_USERNAME": "sora", "MY_VOICE_SORA_WEBHOOK_PASSWORD": "hunter2",
				"MY_VOICE_SORA_JOIN_SECRET": "a-join-secret-of-at-least-32-bytes!",
			})
			app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			event, _ := json.Marshal(map[string]any{
				"type": "connection.created", "id": "EV_1", "channel_id": room, "client_id": "session-of-another-instance",
				"connection_id": "CONN1", "role": "sendrecv", "timestamp": "2026-11-24T09:00:00.000000Z",
			})
			if code, body := postSora(t, app.Handler(), "/voice/webhooks/sora/events", "sora", "hunter2", event); code != http.StatusOK {
				t.Fatalf("events webhook = %d %s, want 200", code, body)
			}
			drainMediaEvents(t, app)
			if calls := sora.Calls(); len(calls) != 0 {
				t.Errorf("the event of a room the instance does not have called the media server: %v", calls)
			}
		})
	}
}

// TestVoiceLiveKitRemovesAStrangerFromItsOwnRoom checks that the instance still enforces its
// own rooms (ADR-0101 §13): a session that joined a room of the instance without being live in
// it is removed from the media server.
func TestVoiceLiveKitRemovesAStrangerFromItsOwnRoom(t *testing.T) {
	t.Parallel()
	lk := liveKitServer()
	app := newLiveKitApp(t, newMediaServer(t, lk).URL)
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk"}, &opened), http.StatusCreated, "open")
	drainMediaEvents(t, app)

	body := liveKitJoined("EV_1", opened.Room.Id, "a-stranger")
	if code := postWebhook(t, app.Handler(), liveKitWebhookToken(liveKitKey, liveKitSecret, body), body); code != http.StatusNoContent {
		t.Fatalf("signed delivery = %d, want 204", code)
	}
	drainMediaEvents(t, app)
	if !slices.Contains(lk.Calls(), "RemoveParticipant "+opened.Room.Id) {
		t.Errorf("calls = %v, want the stranger removed from the room %s", lk.Calls(), opened.Room.Id)
	}
}

// TestVoiceSweepTouchesOnlyItsOwnRooms checks #3453 for the periodic jobs: the sweep of the
// voice rooms, on a LiveKit server that also holds a room of another instance, lists, closes,
// and removes in the instance's own open rooms only.
func TestVoiceSweepTouchesOnlyItsOwnRooms(t *testing.T) {
	t.Parallel()
	lk := liveKitServer()
	app := newLiveKitApp(t, newMediaServer(t, lk).URL)
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk"}, &opened), http.StatusCreated, "open")
	drainMediaEvents(t, app)
	before := len(lk.Calls())

	if err := app.VoiceRooms.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := app.Runner.Drain(t.Context()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	for _, c := range lk.Calls()[before:] {
		if !strings.HasSuffix(c, " "+opened.Room.Id) {
			t.Errorf("the sweep called %q, which is not about the instance's room %s", c, opened.Room.Id)
		}
	}
}
