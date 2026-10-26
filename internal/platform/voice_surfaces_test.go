// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"html"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestVoiceSurfacesThroughAPI drives the places that tell of the voice chat rooms of voice = p2p
// on SQLite through the client API and the pages (ADR-0100 §8, §9, §13): the opening of a room
// notifies the local followers of the host who see it, but not one who mutes the host's
// notifications nor one who does not see the room; the start post is the host's post of the
// room's visibility with the link of the room; an invitation notifies the invited user; the
// profile of the host shows the room to the viewers who see it; the summary of the room and its
// page show the name, the host, and the description only with voice_public_summary, and only
// that it ended and its name after its end; and the history lists the rooms the user hosted.
func TestVoiceSurfacesThroughAPI(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	member := func(name string) testUser {
		t.Helper()
		r, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple"})
		if err != nil {
			t.Fatal(err)
		}
		return testUser{id: r.AccountID, token: issueTokens(t, app, r.User.ID, "").AccessToken}
	}
	bob, carol, dave, erin := member("bob"), member("carol"), member("dave"), member("erin")
	for _, u := range []testUser{bob, carol, dave} {
		e.want(e.do(http.MethodPost, "/accounts/"+alice.id+"/follow", u.token, nil, nil), http.StatusOK, "follow alice")
	}
	e.want(e.do(http.MethodPost, "/accounts/"+alice.id+"/mute", carol.token, map[string]any{"notifications": true}, nil), http.StatusOK, "mute alice")
	e.want(e.do(http.MethodPost, "/accounts/"+dave.id+"/follow", alice.token, nil, nil), http.StatusOK, "follow dave")
	drain := func() {
		t.Helper()
		if _, err := app.Runner.Drain(t.Context()); err != nil {
			t.Fatalf("Drain: %v", err)
		}
	}
	notified := func(u testUser, typ clientapi.NotificationType) []clientapi.Notification {
		t.Helper()
		var page clientapi.NotificationPage
		e.want(e.do(http.MethodGet, "/notifications", u.token, nil, &page), http.StatusOK, "notifications")
		var out []clientapi.Notification
		for _, n := range page.Items {
			if n.Type == typ {
				out = append(out, n)
			}
		}
		return out
	}

	// A room of the mutuals notifies only dave, whom alice follows back, and has no start post.
	var mutuals clientapi.VoiceRoomJoin
	if p := e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Close", "visibility": "mutuals", "post": map[string]any{}}, nil),
		http.StatusUnprocessableEntity, "open for the mutuals with a start post"); p != clientapi.ProblemTypeBlank {
		t.Errorf("open for the mutuals with a start post = %s", p)
	}
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Close", "visibility": "mutuals"}, &mutuals), http.StatusCreated, "open for the mutuals")
	drain()
	if len(notified(bob, clientapi.NotificationTypeVoiceRoomStarted)) != 0 || len(notified(dave, clientapi.NotificationTypeVoiceRoomStarted)) != 1 {
		t.Error("a room of the mutuals notified others than the mutual follower")
	}
	e.want(e.do(http.MethodDelete, "/voice/rooms/"+mutuals.Room.Id, alice.token, nil, nil), http.StatusNoContent, "end")

	// A room of the followers with a start post.
	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{
		"name": "Go talk", "description": "About Go", "visibility": "followers", "post": map[string]any{"text": "Come and talk"},
	}, &opened), http.StatusCreated, "open for the followers")
	room := opened.Room.Id
	drain()
	link := "https://" + cfg.Host + "/voice/" + room
	var statuses clientapi.AccountStatusPage
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id+"/statuses", alice.token, nil, &statuses), http.StatusOK, "statuses of alice")
	posts := statuses.Items
	if len(posts) != 1 || posts[0].Visibility != clientapi.VisibilityPrivate || !strings.Contains(posts[0].Content, "Come and talk") ||
		!strings.Contains(html.UnescapeString(posts[0].Content), link) {
		t.Fatalf("the start post = %+v, want a private post with the text and %s", posts, link)
	}
	got := notified(bob, clientapi.NotificationTypeVoiceRoomStarted)
	if len(got) != 1 || got[0].VoiceRoomId == nil || *got[0].VoiceRoomId != room || got[0].VoiceRoom == nil || got[0].VoiceRoom.Name != "Go talk" ||
		got[0].Account == nil || got[0].Account.Id != alice.id {
		t.Errorf("voice_room_started of bob = %+v", got)
	}
	if len(notified(carol, clientapi.NotificationTypeVoiceRoomStarted)) != 0 {
		t.Error("voice_room_started reached carol, who mutes the notifications of alice")
	}
	if len(notified(erin, clientapi.NotificationTypeVoiceRoomStarted)) != 0 {
		t.Error("voice_room_started reached erin, who does not follow alice")
	}

	// An invitation notifies erin, who then sees the room and the entry on the profile of alice.
	e.want(e.do(http.MethodPost, "/voice/rooms/"+room+"/invitations", alice.token, map[string]any{"account_id": erin.id}, nil), http.StatusCreated, "invite erin")
	if got := notified(erin, clientapi.NotificationTypeVoiceRoomInvited); len(got) != 1 || got[0].VoiceRoom == nil || got[0].VoiceRoom.Id != room {
		t.Errorf("voice_room_invited of erin = %+v", got)
	}
	var profile clientapi.Account
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, bob.token, nil, &profile), http.StatusOK, "profile of alice by bob")
	if profile.VoiceRoom == nil || profile.VoiceRoom.Id != room || profile.VoiceRoom.Role != clientapi.VoiceRoomEntryRoleHost {
		t.Errorf("the room on the profile of alice = %+v", profile.VoiceRoom)
	}
	profile = clientapi.Account{}
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, "", nil, &profile), http.StatusOK, "profile of alice signed out")
	if profile.VoiceRoom != nil {
		t.Errorf("the profile of alice shows the room to a visitor who is not signed in: %+v", profile.VoiceRoom)
	}

	// The summary and the page of the room for the visitors who do not see it.
	pageOf := func(id string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/voice/"+id, nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	page := func() string {
		t.Helper()
		code, body := pageOf(room)
		if code != http.StatusOK {
			t.Errorf("the page of an existing room = %d, want 200", code)
		}
		return body
	}
	if code, _ := pageOf("01a176ce-0000-7000-8000-000000000000"); code != http.StatusNotFound {
		t.Errorf("the page of an unknown room = %d, want 404", code)
	}
	var sum clientapi.VoiceRoomSummary
	e.want(e.do(http.MethodGet, "/voice/rooms/"+room+"/summary", "", nil, &sum), http.StatusOK, "summary of the followers' room")
	if sum.State != clientapi.VoiceRoomSummaryStateOpen || sum.Name != nil || sum.Host != nil {
		t.Errorf("summary of a room of the followers = %+v, want the state only", sum)
	}
	if body := page(); strings.Contains(body, "og:title") {
		t.Errorf("the page of a room that shows no summary carries a preview:\n%s", body)
	}
	e.want(e.do(http.MethodPatch, "/admin/settings", alice.token, map[string]any{"values": map[string]string{"voice_public_summary": "true"}}, nil), http.StatusOK, "show the summaries")
	sum = clientapi.VoiceRoomSummary{}
	e.want(e.do(http.MethodGet, "/voice/rooms/"+room+"/summary", "", nil, &sum), http.StatusOK, "summary with voice_public_summary")
	if sum.Name != nil || strings.Contains(page(), "Go talk") {
		t.Errorf("a room of the followers shows its summary: %+v", sum)
	}
	e.want(e.do(http.MethodPatch, "/voice/rooms/"+room, alice.token, map[string]any{"visibility": "local"}, nil), http.StatusOK, "open the room to every local user")
	sum = clientapi.VoiceRoomSummary{}
	e.want(e.do(http.MethodGet, "/voice/rooms/"+room+"/summary", "", nil, &sum), http.StatusOK, "summary of a local room")
	if sum.Name == nil || *sum.Name != "Go talk" || sum.Description == nil || *sum.Description != "About Go" || sum.Host == nil || sum.Host.Id != alice.id {
		t.Errorf("summary of a local room = %+v, want the name, the description, and the host", sum)
	}
	if body := page(); !strings.Contains(body, `<meta property="og:title" content="Go talk — a voice chat hosted by alice, open now">`) {
		t.Errorf("the page of a local room has no preview of its summary:\n%s", body)
	}
	e.want(e.do(http.MethodDelete, "/voice/rooms/"+room, alice.token, nil, nil), http.StatusNoContent, "end")
	sum = clientapi.VoiceRoomSummary{}
	e.want(e.do(http.MethodGet, "/voice/rooms/"+room+"/summary", "", nil, &sum), http.StatusOK, "summary of an ended room")
	if sum.State != clientapi.VoiceRoomSummaryStateEnded || sum.Name == nil || *sum.Name != "Go talk" || sum.Description != nil || sum.Host != nil {
		t.Errorf("summary of an ended room = %+v, want only the name", sum)
	}
	if body := page(); !strings.Contains(body, `content="Go talk — this voice chat has ended"`) {
		t.Errorf("the page of an ended room does not say it ended:\n%s", body)
	}
	if got := notified(bob, clientapi.NotificationTypeVoiceRoomStarted); len(got) != 1 || got[0].VoiceRoom != nil {
		t.Errorf("voice_room_started of an ended room = %+v, want no room", got)
	}

	// The history of the rooms alice hosted, newest first.
	var history clientapi.VoiceRoomHistoryPage
	e.want(e.do(http.MethodGet, "/voice/history", alice.token, nil, &history), http.StatusOK, "history")
	ids := []string{}
	for _, h := range history.Items {
		ids = append(ids, h.Id)
		if h.EndReason == nil || *h.EndReason != "host" || h.EndedAt == nil {
			t.Errorf("history entry = %+v, want ended by its host", h)
		}
	}
	if !slices.Equal(ids, []string{room, mutuals.Room.Id}) {
		t.Errorf("history = %v, want %v", ids, []string{room, mutuals.Room.Id})
	}
	e.want(e.do(http.MethodGet, "/voice/history", bob.token, nil, &history), http.StatusOK, "history of bob")
	if len(history.Items) != 0 {
		t.Errorf("history of bob = %+v, want none", history.Items)
	}
}
