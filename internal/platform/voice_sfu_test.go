// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// TestVoiceSFUStartsOnlyWithSFU builds the App in every role with each method of the voice chat
// (ADR-0101 §10): only voice = sfu with role = all or sfu has a media node (its sockets, its
// registration, and its WebSocket); voice = off or p2p has none in any role, and neither do web
// and worker with sfu, which only place the rooms.
func TestVoiceSFUStartsOnlyWithSFU(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		voice, role string
		node        bool
	}{
		{"off", "all", false}, {"off", "web", false}, {"off", "worker", false},
		{"p2p", "all", false}, {"p2p", "web", false}, {"p2p", "worker", false},
		{"sfu", "all", true}, {"sfu", "web", false}, {"sfu", "worker", false}, {"sfu", "sfu", true},
	} {
		t.Run(c.voice+"/"+c.role, func(t *testing.T) {
			t.Parallel()
			cfg := loadConfig(t, map[string]string{
				"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": c.voice, "MY_ROLE": c.role,
				"MY_VOICE_STUN_URLS": "stun:stun.example.org",
			})
			app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			if got := app.VoiceNode != nil; got != c.node {
				t.Errorf("VoiceNode = %v, want a node %t", app.VoiceNode, c.node)
			}
			if c.voice != "off" && !app.VoiceRooms.Enabled() {
				t.Errorf("the voice chat is off with voice = %s", c.voice)
			}
			if !c.node {
				// No WebSocket of the grants either.
				req := httptest.NewRequest(http.MethodGet, config.VoiceSFUPath+"?ticket=t", nil)
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Sec-WebSocket-Version", "13")
				req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
				rec := httptest.NewRecorder()
				app.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusNotFound {
					t.Errorf("GET %s = %d, want 404 without a media node", config.VoiceSFUPath, rec.Code)
				}
			}
		})
	}
}

// TestVoiceSFURoleNeedsSFU refuses to start role = sfu with another method of the voice chat
// (ADR-0101 §1, §10): no media node runs without voice = sfu, whatever the role.
func TestVoiceSFURoleNeedsSFU(t *testing.T) {
	t.Parallel()
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	load := func(voice string) error {
		env := map[string]string{
			"MY_HOST": "example.test", "MY_KEK": base64.StdEncoding.EncodeToString(kek), "MY_BLOB_FS_DIR": t.TempDir(),
			"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_ROLE": "sfu", "MY_VOICE": voice,
			"MY_VOICE_STUN_URLS": "stun:stun.example.org",
		}
		_, _, err := config.Load(config.Source{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }})
		return err
	}
	if err := load("sfu"); err != nil {
		t.Fatalf("role = sfu with voice = sfu: %v", err)
	}
	for _, voice := range []string{"off", "p2p"} {
		if err := load(voice); err == nil {
			t.Errorf("role = sfu with voice = %s loads", voice)
		}
	}
}

// TestVoiceSFUInOneProcess opens a room of voice = sfu in one process (role = all, SQLite): with
// no media node registered the opening answers 503; once the process's node runs, the room is
// placed on it, the grant names its WebSocket with a ticket, and the node reads the room through
// the voice module.
func TestVoiceSFUInOneProcess(t *testing.T) {
	t.Parallel()
	udp, tcp := freePort(t, "udp"), freePort(t, "tcp")
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "sfu",
		"MY_VOICE_SFU_UDP_PORT": strconv.Itoa(udp), "MY_VOICE_SFU_TCP_PORT": strconv.Itoa(tcp),
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	host := e.user("alice")
	if p := e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Talk"}, nil), http.StatusServiceUnavailable, "open without a node"); p != clientapi.ProblemTypeVoiceUnavailable {
		t.Errorf("open without a node = %s, want %s", p, clientapi.ProblemTypeVoiceUnavailable)
	}
	if err := app.VoiceNode.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = app.VoiceNode.Close(context.Background()) })
	var joined clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Talk"}, &joined), http.StatusCreated, "open")
	g := joined.Grant
	if g.Kind != clientapi.VoiceGrantKindSfu || g.Url == nil || *g.Url != cfg.VoiceSFUURL() || g.Token == nil || *g.Token == "" {
		t.Errorf("grant = %+v, want kind sfu, the node's URL %s, and a ticket", g, cfg.VoiceSFUURL())
	}
	st, err := app.VoiceRooms.MediaRoom(t.Context(), joined.Room.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Open || len(st.Sessions) != 1 || st.Sessions[0].ID != joined.Session || !st.Sessions[0].CanSpeak {
		t.Errorf("MediaRoom = %+v, want the open room with the host's session, a speaker", st)
	}
}

// freePort returns a port of network that is free now.
func freePort(t *testing.T, network string) int {
	t.Helper()
	if network == "udp" {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// TestVoiceSFURaisingTheMaximum raises a room's maximum through the client API with voice = sfu
// (ADR-0101 §14): a rise that the room's node has room for is reserved and commits; one it has no
// room for is a 503 and the maximum stays; lowering it always commits.
func TestVoiceSFURaisingTheMaximum(t *testing.T) {
	t.Parallel()
	udp, tcp := freePort(t, "udp"), freePort(t, "tcp")
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "sfu",
		"MY_VOICE_MAX_PARTICIPANTS": "50", "MY_VOICE_SFU_NODE_CAPACITY": "15",
		"MY_VOICE_SFU_UDP_PORT": strconv.Itoa(udp), "MY_VOICE_SFU_TCP_PORT": strconv.Itoa(tcp),
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if err := app.VoiceNode.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = app.VoiceNode.Close(context.Background()) })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	host := e.user("alice")
	var joined clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Talk", "max_participants": 10}, &joined), http.StatusCreated, "open with 10")
	room := "/voice/rooms/" + joined.Room.Id
	if p := e.want(e.do(http.MethodPatch, room, host.token, map[string]any{"max_participants": 20}, nil), http.StatusServiceUnavailable, "raise past the node"); p != clientapi.ProblemTypeVoiceUnavailable {
		t.Errorf("raise past the node = %s, want %s", p, clientapi.ProblemTypeVoiceUnavailable)
	}
	var got clientapi.VoiceRoom
	e.want(e.do(http.MethodGet, room, host.token, nil, &got), http.StatusOK, "get")
	if got.MaxParticipants != 10 {
		t.Errorf("max_participants after a refused rise = %d, want 10", got.MaxParticipants)
	}
	e.want(e.do(http.MethodPatch, room, host.token, map[string]any{"max_participants": 15}, &got), http.StatusOK, "raise to the node's capacity")
	if got.MaxParticipants != 15 {
		t.Errorf("max_participants = %d, want 15", got.MaxParticipants)
	}
	e.want(e.do(http.MethodPatch, room, host.token, map[string]any{"max_participants": 5}, &got), http.StatusOK, "lower")
	if got.MaxParticipants != 5 {
		t.Errorf("max_participants = %d, want 5", got.MaxParticipants)
	}
}
