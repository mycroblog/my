// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// voiceQueries counts the database round trips of one GET of a room that n listeners joined,
// and of one list of the n+1 rooms open for local users, as a viewer who follows every host.
// Every query is one round trip (one call in pg_stat_statements).
func voiceQueries(t *testing.T, n int) (room, list int) {
	t.Helper()
	tracer := testkit.NewTracer()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org", "MY_VOICE_P2P_MAX_PARTICIPANTS": "20", "MY_VOICE_MAX_OPEN_ROOMS": "20",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Tracer: tracer})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &fanOutEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, app: app}
	viewer := e.user("viewer")
	open := func(name string) (testUser, string) {
		host := e.user(name)
		e.want(e.do(http.MethodPost, "/accounts/"+host.id+"/follow", viewer.token, nil, nil), http.StatusOK, "follow")
		var join clientapi.VoiceRoomJoin
		e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": name, "visibility": "local"}, &join), http.StatusCreated, "open")
		return host, join.Room.Id
	}
	_, first := open("host")
	for i := range n {
		listener := e.user(fmt.Sprintf("listener%d", i))
		e.want(e.do(http.MethodPost, "/voice/rooms/"+first+"/participation", listener.token, nil, nil), http.StatusOK, "join")
	}
	for i := range n {
		open(fmt.Sprintf("other%d", i))
	}
	e.drain()
	total := func() int {
		n := 0
		for _, c := range ops(tracer) {
			n += c
		}
		return n
	}
	before := total()
	var got clientapi.VoiceRoom
	e.want(e.do(http.MethodGet, "/voice/rooms/"+first, viewer.token, nil, &got), http.StatusOK, "get")
	room = total() - before
	if len(got.Participants) != n+1 {
		t.Fatalf("the room has %d participants, want %d", len(got.Participants), n+1)
	}
	before = total()
	var page clientapi.VoiceRoomPage
	e.want(e.do(http.MethodGet, "/voice/rooms?limit=40", viewer.token, nil, &page), http.StatusOK, "list")
	list = total() - before
	if len(page.Items) != n+1 {
		t.Fatalf("the list has %d rooms, want %d", len(page.Items), n+1)
	}
	return room, list
}

// TestVoiceQueries checks that presenting a room makes a number of round trips that does not grow
// with its participants, and listing the rooms one that does not grow with the open rooms.
func TestVoiceQueries(t *testing.T) {
	t.Parallel()
	smallRoom, smallList := voiceQueries(t, 3)
	largeRoom, largeList := voiceQueries(t, 10)
	t.Logf("room: %d queries for 4 participants, %d for 11; list: %d queries for 4 rooms, %d for 11", smallRoom, largeRoom, smallList, largeList)
	if largeRoom > smallRoom {
		t.Errorf("%d queries to present a room of 11, more than the %d for 4", largeRoom, smallRoom)
	}
	if largeList > smallList {
		t.Errorf("%d queries to list 11 rooms, more than the %d for 4", largeList, smallList)
	}
}
