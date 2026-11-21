// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// eventBoardEvents records, by event type, the IDs of the statuses that the content module's
// domain events name.
type eventBoardEvents struct {
	mu  sync.Mutex
	ids map[string][]string
}

func (r *eventBoardEvents) add(typ string, ids ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids[typ] = append(r.ids[typ], ids...)
}

func (r *eventBoardEvents) named(typ string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.ids[typ])
}

func deletedIDs(ds []appcontent.StatusDeleted) []string {
	var ids []string
	for _, d := range ds {
		ids = append(ids, d.Status.ID, d.Deletion.StatusID)
	}
	return ids
}

// recordEvents subscribes to the domain events of the statuses and of the board posts.
func (e *eventBoardEnv) recordEvents() *eventBoardEvents {
	r := &eventBoardEvents{ids: map[string][]string{}}
	record := func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		switch v := ev.(type) {
		case appcontent.StatusCreated:
			r.add(ev.EventType(), v.Status.ID)
		case *appcontent.StatusCreated:
			r.add(ev.EventType(), v.Status.ID)
		case appcontent.StatusRevised:
			r.add(ev.EventType(), v.Status.ID)
		case *appcontent.StatusRevised:
			r.add(ev.EventType(), v.Status.ID)
		case appcontent.StatusDeleted:
			r.add(ev.EventType(), v.Status.ID, v.Deletion.StatusID)
		case *appcontent.StatusDeleted:
			r.add(ev.EventType(), v.Status.ID, v.Deletion.StatusID)
		case appcontent.StatusesDeleted:
			r.add(ev.EventType(), deletedIDs(v.Deletions)...)
		case *appcontent.StatusesDeleted:
			r.add(ev.EventType(), deletedIDs(v.Deletions)...)
		case appcontent.BoardStatusPosted:
			r.add(ev.EventType(), v.Status.ID)
		case *appcontent.BoardStatusPosted:
			r.add(ev.EventType(), v.Status.ID)
		case appcontent.BoardStatusRevised:
			r.add(ev.EventType(), v.Status.ID)
		case *appcontent.BoardStatusRevised:
			r.add(ev.EventType(), v.Status.ID)
		case appcontent.BoardStatusesDeleted:
			r.add(ev.EventType(), deletedIDs(v.Deletions)...)
		case *appcontent.BoardStatusesDeleted:
			r.add(ev.EventType(), deletedIDs(v.Deletions)...)
		default:
			e.t.Errorf("an event %s of an unexpected type %T", ev.EventType(), ev)
		}
		return nil
	}
	for _, typ := range []string{
		appcontent.EventStatusCreated, appcontent.EventStatusRevised, appcontent.EventStatusDeleted, appcontent.EventStatusesDeleted,
		appcontent.EventBoardStatusPosted, appcontent.EventBoardStatusRevised, appcontent.EventBoardStatusesDeleted,
	} {
		e.app.Projectors.Register(typ, record)
	}
	return r
}

// get returns the body of a GET of the client API as user, which must answer 200.
func (e *eventBoardEnv) get(user, path string) string {
	e.t.Helper()
	res := e.send(user, http.MethodGet, path, nil, nil, nil)
	body, err := io.ReadAll(res.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		e.t.Fatalf("GET %s as %s = %d %s", path, user, res.StatusCode, body)
	}
	return string(body)
}

// TestEventBoardPostsNeverLeaveTheEvent checks ADR-0161 §17 path by path: a post of a board, its
// edit, and its deletion emit no event of an ordinary status, and the post reaches no timeline
// (home, local, federated, hashtag, profile, list), no conversation, no bookmark, no outbox, no
// delivery, no link preview card, no ActivityPub object, and no HTML page with its preview; it is
// never boosted, quoted, or replied to by an ordinary post. An ordinary post of the same author,
// with the same hashtag and link, shows that each path is there.
func TestEventBoardPostsNeverLeaveTheEvent(t *testing.T) {
	t.Parallel()
	build := fstest.MapFS{
		".vite/manifest.json":  {Data: []byte(`{"index.html": {"file": "assets/index-AAAA.js", "isEntry": true}}`)},
		"assets/index-AAAA.js": {Data: []byte("export {}")},
	}
	e := newEventBoardEnv(t, platform.AppOptions{WebBuild: webBuild(t, build)}, "alice", "gina", "sam")
	ctx := t.Context()
	events := e.recordEvents()
	for _, follower := range []string{"gina", "sam"} {
		if _, err := e.app.Graph.Follow(ctx, e.ids[follower], e.ids["alice"]); err != nil {
			t.Fatal(err)
		}
	}
	var list clientapi.List
	if res := e.send("gina", http.MethodPost, "/lists", map[string]any{"name": "Friends"}, nil, &list); res.StatusCode/100 != 2 {
		t.Fatalf("POST /lists = %d", res.StatusCode)
	}
	if res := e.send("gina", http.MethodPost, "/lists/"+list.Id+"/accounts/"+e.ids["alice"], nil, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("add alice to the list = %d", res.StatusCode)
	}
	ev := e.createEvent("readers", nil)
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	e.drain()

	ordinary := e.post("alice", map[string]any{"text": "#boardtag ordinary https://example.com/ordinary", "visibility": "public"})
	// An ordinary post enqueues its fan-out.
	if !slices.ContainsFunc(e.pendingJobs(), func(j queue.PendingJob) bool { return j.Type == federation.JobFanOut }) {
		t.Errorf("an ordinary post enqueues no %s, so this test sees nothing of it", federation.JobFanOut)
	}
	e.drain()
	// A step on the board enqueues no fan-out, no delivery, and no job of federation or of the
	// timelines that names a board post.
	seen := map[string]bool{}
	for _, j := range e.pendingJobs() {
		seen[j.ID] = true
	}
	noJobs := func(step string, ids ...string) {
		t.Helper()
		for _, j := range e.pendingJobs() {
			if seen[j.ID] {
				continue
			}
			seen[j.ID] = true
			names := slices.ContainsFunc(ids, func(id string) bool {
				return strings.Contains(string(j.Payload), id) || strings.Contains(j.IdempotencyKey, id)
			})
			if j.Type != appcontent.JobFetchLinkCard && (names || j.Type == federation.JobFanOut || j.Type == federation.JobDeliver) {
				t.Errorf("%s enqueues the job %s %s %s", step, j.Type, j.IdempotencyKey, j.Payload)
			}
		}
	}

	board := e.mustPostOnBoard("alice", ev.Id, map[string]any{"text": "#boardtag on the board https://example.com/board @gina"})
	noJobs("a board post with a link", board.Id)
	if res := e.send("alice", http.MethodPut, "/statuses/"+board.Id, map[string]any{"text": "#boardtag on the board, edited https://example.com/edited @gina"}, nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("edit the board post = %s", e.outcome(res))
	}
	noJobs("an edit of a board post with a link", board.Id)
	reply := e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": "#boardtag a reply on the board", "in_reply_to_id": board.Id})
	noJobs("a reply on the board", board.Id, reply.Id)
	deleted := e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": "#boardtag deleted from the board"})
	if got := e.outcome(e.send("gina", http.MethodDelete, "/statuses/"+deleted.Id, nil, nil, nil)); got != "204" {
		t.Fatalf("delete a board post = %s", got)
	}
	noJobs("the deletion of a board post", deleted.Id)
	boardIDs := []string{board.Id, reply.Id, deleted.Id}
	e.drain()

	// No event of an ordinary status names a board post; the events of the board posts do.
	for _, typ := range []string{appcontent.EventStatusCreated, appcontent.EventStatusRevised, appcontent.EventStatusDeleted, appcontent.EventStatusesDeleted} {
		for _, id := range events.named(typ) {
			if slices.Contains(boardIDs, id) {
				t.Errorf("%s names the board post %s", typ, id)
			}
		}
	}
	if !slices.Contains(events.named(appcontent.EventStatusCreated), ordinary.Id) {
		t.Errorf("%s does not name the ordinary post, so this test sees nothing", appcontent.EventStatusCreated)
	}
	for typ, want := range map[string][]string{
		appcontent.EventBoardStatusPosted:    boardIDs,
		appcontent.EventBoardStatusRevised:   {board.Id},
		appcontent.EventBoardStatusesDeleted: {deleted.Id},
	} {
		got := events.named(typ)
		for _, id := range want {
			if !slices.Contains(got, id) {
				t.Errorf("%s names %v, without the board post %s", typ, got, id)
			}
		}
	}

	// No timeline, conversation list, or bookmark list shows a board post, to its readers either.
	paths := []string{
		"/timelines/home", "/timelines/local", "/timelines/federated", "/timelines/tag/boardtag",
		"/accounts/" + e.ids["alice"] + "/statuses", "/accounts/" + e.ids["gina"] + "/statuses", "/conversations", "/bookmarks",
	}
	for _, user := range []string{"alice", "gina", "sam"} {
		for _, path := range paths {
			body := e.get(user, path)
			for _, id := range boardIDs {
				if strings.Contains(body, id) {
					t.Errorf("GET %s as %s shows the board post %s", path, user, id)
				}
			}
		}
	}
	for _, path := range []string{"/timelines/home", "/timelines/local", "/timelines/federated", "/timelines/tag/boardtag", "/accounts/" + e.ids["alice"] + "/statuses"} {
		if !strings.Contains(e.get("sam", path), ordinary.Id) {
			t.Errorf("GET %s as sam does not show the ordinary post, so this test sees nothing of it", path)
		}
	}
	listBody := e.get("gina", "/timelines/list/"+list.Id)
	if !strings.Contains(listBody, ordinary.Id) {
		t.Error("the list timeline does not show the ordinary post, so this test sees nothing of it")
	}
	for _, id := range boardIDs {
		if strings.Contains(listBody, id) {
			t.Errorf("the list timeline shows the board post %s", id)
		}
	}

	// No boost, bookmark, pin, quote, or ordinary reply.
	for _, user := range []string{"alice", "gina"} {
		for _, action := range []string{"boost", "bookmark", "pin"} {
			if got := e.outcome(e.send(user, http.MethodPost, "/statuses/"+board.Id+"/"+action, nil, nil, nil)); !strings.HasPrefix(got, "404") {
				t.Errorf("%s the board post as %s = %s, want 404", action, user, got)
			}
		}
		for field, name := range map[string]string{"quoted_status_id": "quote", "in_reply_to_id": "reply"} {
			missing := e.outcome(e.send(user, http.MethodPost, "/statuses", map[string]any{"text": "x", "visibility": "public", field: eventBoardMissingID}, nil, nil))
			got := e.outcome(e.send(user, http.MethodPost, "/statuses", map[string]any{"text": "x", "visibility": "public", field: board.Id}, nil, nil))
			if got != missing || !strings.HasPrefix(got, "4") {
				t.Errorf("an ordinary %s of the board post as %s = %s, want the answer for a missing status %s", name, user, got, missing)
			}
		}
	}

	// Federation: no object, no activity, nothing in the outbox.
	for _, path := range []string{"/ap/objects/" + board.Id, "/ap/objects/" + board.Id + "/activity", "/ap/objects/" + reply.Id} {
		if code, body := e.federationGet(path, "application/activity+json"); code != http.StatusNotFound {
			t.Errorf("GET %s = %d %s, want 404", path, code, body)
		}
	}
	if code, _ := e.federationGet("/ap/objects/"+ordinary.Id, "application/activity+json"); code != http.StatusOK {
		t.Errorf("GET the ordinary post's object = %d, want 200", code)
	}
	for _, name := range []string{"alice", "gina"} {
		code, body := e.federationGet("/ap/actors/"+e.ids[name]+"/outbox?page=true", "application/activity+json")
		if code != http.StatusOK {
			t.Fatalf("GET the outbox of %s = %d", name, code)
		}
		for _, id := range boardIDs {
			if strings.Contains(body, id) {
				t.Errorf("the outbox of %s lists the board post %s", name, id)
			}
		}
	}

	// No HTML page with a link preview: the page of a board post is that of a missing post.
	missingCode, _ := e.federationGet("/@alice/status/"+eventBoardMissingID, acceptBrowser)
	if code, body := e.federationGet("/@alice/status/"+board.Id, acceptBrowser); code != missingCode || code == http.StatusOK ||
		strings.Contains(body, "on the board") || strings.Contains(body, "og:description") {
		t.Errorf("the page of the board post = %d, want the page of a missing post (%d): %s", code, missingCode, body)
	}
	if code, body := e.federationGet("/@alice/status/"+ordinary.Id, acceptBrowser); code != http.StatusOK || !strings.Contains(body, "og:description") {
		t.Errorf("the page of the ordinary post = %d, want 200 with its preview", code)
	}
	// No link card.
	var st clientapi.Status
	if res := e.send("gina", http.MethodGet, "/statuses/"+board.Id, nil, nil, &st); res.StatusCode != http.StatusOK || st.Card != nil {
		t.Errorf("the board post as a reader = %d, card %+v, want 200 without a card", res.StatusCode, st.Card)
	}
}

// TestEventBoardPostsAreInTheirAuthorsExport checks FR-008 for the boards (ADR-0161 §17): the
// posts a user wrote on a board are in the outbox.json of the user's data export, and not in the
// export of another reader.
func TestEventBoardPostsAreInTheirAuthorsExport(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	ev := e.createEvent("readers", nil)
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	post := e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": "boardexportneedle"})
	e.post("gina", map[string]any{"text": "an ordinary post", "visibility": "public"})
	outbox := func(user string) string {
		t.Helper()
		var export clientapi.DataExport
		if res := e.send(user, http.MethodPost, "/exports", nil, nil, &export); res.StatusCode != http.StatusAccepted {
			t.Fatalf("POST /exports as %s = %d", user, res.StatusCode)
		}
		e.drain()
		res := e.send(user, http.MethodGet, "/exports/"+export.Id+"/archive", nil, nil, nil)
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET the archive as %s = %d %s", user, res.StatusCode, body)
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range zr.File {
			if f.Name != "outbox.json" {
				continue
			}
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(r)
			_ = r.Close()
			return string(data)
		}
		t.Fatalf("the archive of %s has no outbox.json", user)
		return ""
	}
	if got := outbox("gina"); !strings.Contains(got, "boardexportneedle") || !strings.Contains(got, "an ordinary post") {
		t.Errorf("the outbox.json of the author does not hold the board post %s:\n%s", post.Id, got)
	}
	if got := outbox("alice"); strings.Contains(got, "boardexportneedle") {
		t.Errorf("the outbox.json of another reader holds the board post %s", post.Id)
	}
}

// TestEventBoardPostsAreNeverSearched checks that the full-text search never finds a board post
// (ADR-0161 §17): the index is never given one, and one planted in it stays out, for its readers
// and its author too.
func TestEventBoardPostsAreNeverSearched(t *testing.T) {
	e := newSearchEnv(t)
	alice, gina := e.user("alice"), e.user("gina")
	e.setIndexable(alice, true)
	e.setIndexable(gina, true)
	start := time.Now().Add(3 * time.Hour)
	var ev clientapi.Event
	e.want(e.do(http.MethodPost, "/events", alice.token, map[string]any{
		"title": "Board game night", "time_zone": "Asia/Tokyo", "board_writers": "readers",
		"starts_at": start.Format(time.RFC3339), "ends_at": start.Add(2 * time.Hour).Format(time.RFC3339),
	}, &ev), http.StatusCreated, "POST /events")
	if _, err := e.app.Events.Answer(t.Context(), gina.id, ev.Id, gathering.AnswerGoing); err != nil {
		t.Fatal(err)
	}
	ordinary := e.post(alice, "needle ordinary", "public")
	var board, reply clientapi.Status
	e.want(e.do(http.MethodPost, "/events/"+ev.Id+"/board", alice.token, map[string]any{"text": "needle on the board"}, &board), http.StatusCreated, "POST on the board")
	e.want(e.do(http.MethodPost, "/events/"+ev.Id+"/board", gina.token, map[string]any{"text": "needle in a reply", "in_reply_to_id": board.Id}, &reply), http.StatusCreated, "POST a reply on the board")
	e.want(e.do(http.MethodPut, "/statuses/"+board.Id, alice.token, map[string]any{"text": "needle on the board, edited"}, nil), http.StatusOK, "edit the board post")
	e.wantFound("needle", ordinary)
	e.plant("needle", board.Id, reply.Id)
	e.wantSearch(alice.token, "needle", ordinary)
	e.wantSearch(gina.token, "needle", ordinary)
}
