// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// eventBoardStart is when the clock of an eventBoardEnv starts; its events start three hours later
// and end two hours after that.
var eventBoardStart = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

const (
	eventBoardStartsIn = 3 * time.Hour
	eventBoardEndsIn   = 5 * time.Hour
	// missingID names no status and no event.
	eventBoardMissingID = "0192f0c1-0000-7000-8000-000000000000"
)

// eventBoardEnv is a real App on SQLite with a clock that the test moves, users who sign in to
// the client API, and a queue whose jobs run when the test drains them.
type eventBoardEnv struct {
	*statusesClient
	clock *testkit.Clock
	keys  int
}

func newEventBoardEnv(t *testing.T, opts platform.AppOptions, users ...string) *eventBoardEnv {
	t.Helper()
	clock := testkit.NewClock(eventBoardStart)
	opts.Clock = clock
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "db"}, opts, users...)
	return &eventBoardEnv{statusesClient: c, clock: clock}
}

// at moves the clock to d after the start, and signs every user in again: a token issued before
// may have expired since.
func (e *eventBoardEnv) at(d time.Duration) {
	e.t.Helper()
	e.clock.Set(eventBoardStart.Add(d))
	for name, userID := range e.userIDs {
		e.tokens[name] = issueTokens(e.t, e.app, userID, "").AccessToken
	}
}

func (e *eventBoardEnv) drain() {
	e.t.Helper()
	if _, err := e.app.Runner.Drain(e.t.Context()); err != nil {
		e.t.Fatalf("Drain: %v", err)
	}
}

// send is statusesClient.do with the request headers header.
func (e *eventBoardEnv) send(user, method, path string, body any, header map[string]string, out any) *http.Response {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		r = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, clientapi.BasePath+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+e.tokens[user])
	}
	res := e.spec.Serve(e.t, e.h, req)
	if out != nil && res.StatusCode < 300 {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			e.t.Fatal(err)
		}
	}
	return res
}

// outcome is the status of res and, for an error, its problem type.
func (e *eventBoardEnv) outcome(res *http.Response) string {
	e.t.Helper()
	if res.StatusCode < 400 {
		return fmt.Sprint(res.StatusCode)
	}
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		e.t.Fatal(err)
	}
	return fmt.Sprintf("%d %s", res.StatusCode, p.Type)
}

// createEvent creates an event of alice's through the API, with the fields of extra; writers ""
// leaves board_writers to its default.
func (e *eventBoardEnv) createEvent(writers string, extra map[string]any) clientapi.Event {
	e.t.Helper()
	now := e.clock.Now()
	body := map[string]any{
		"title": "Board game night", "time_zone": "Asia/Tokyo",
		"starts_at": now.Add(eventBoardStartsIn).Format(time.RFC3339), "ends_at": now.Add(eventBoardEndsIn).Format(time.RFC3339),
	}
	if writers != "" {
		body["board_writers"] = writers
	}
	for k, v := range extra {
		body[k] = v
	}
	var ev clientapi.Event
	if res := e.send("alice", http.MethodPost, "/events", body, nil, &ev); res.StatusCode != http.StatusCreated {
		e.t.Fatalf("POST /events %v = %s", body, e.outcome(res))
	}
	return ev
}

func (e *eventBoardEnv) answer(name, eventID string, a gathering.Answer) {
	e.t.Helper()
	if _, err := e.app.Events.Answer(e.t.Context(), e.ids[name], eventID, a); err != nil {
		e.t.Fatalf("Answer(%s, %s) = %v", name, a, err)
	}
}

func (e *eventBoardEnv) event(user, eventID string) clientapi.Event {
	e.t.Helper()
	var ev clientapi.Event
	if res := e.send(user, http.MethodGet, "/events/"+eventID, nil, nil, &ev); res.StatusCode != http.StatusOK {
		e.t.Fatalf("GET /events/%s as %s = %s", eventID, user, e.outcome(res))
	}
	return ev
}

// postOnBoard posts body on the board of eventID as user, with an Idempotency-Key of its own.
func (e *eventBoardEnv) postOnBoard(user, eventID string, body map[string]any, out *clientapi.Status) *http.Response {
	e.t.Helper()
	e.keys++
	return e.send(user, http.MethodPost, "/events/"+eventID+"/board", body, map[string]string{"Idempotency-Key": fmt.Sprintf("board-%d", e.keys)}, out)
}

func (e *eventBoardEnv) mustPostOnBoard(user, eventID string, body map[string]any) clientapi.Status {
	e.t.Helper()
	var st clientapi.Status
	if res := e.postOnBoard(user, eventID, body, &st); res.StatusCode != http.StatusCreated {
		e.t.Fatalf("POST /events/%s/board %v as %s = %s", eventID, body, user, e.outcome(res))
	}
	return st
}

// board lists the board of eventID as user, the query appended to the path.
func (e *eventBoardEnv) board(user, eventID, query string) (string, clientapi.EventBoardPage) {
	e.t.Helper()
	var page clientapi.EventBoardPage
	res := e.send(user, http.MethodGet, "/events/"+eventID+"/board"+query, nil, nil, &page)
	return e.outcome(res), page
}

func (e *eventBoardEnv) boardIDs(user, eventID string) []string {
	e.t.Helper()
	got, page := e.board(user, eventID, "")
	if got != "200" {
		e.t.Fatalf("GET /events/%s/board as %s = %s", eventID, user, got)
	}
	return eventBoardStatusIDs(page.Items)
}

func eventBoardStatusIDs(items []clientapi.Status) []string {
	ids := []string{}
	for _, s := range items {
		ids = append(ids, s.Id)
	}
	return ids
}

// missing is how the API answers user for a status that does not exist.
func (e *eventBoardEnv) missing(user, method, suffix string, body any) string {
	e.t.Helper()
	return problemShape(e.t, e.send(user, method, "/statuses/"+eventBoardMissingID+suffix, body, nil, nil))
}

// federationGet sends a GET of path to the whole handler, as a client that accepts accept.
func (e *eventBoardEnv) federationGet(path, accept string) (int, string) {
	e.t.Helper()
	req := httptest.NewRequestWithContext(e.t.Context(), http.MethodGet, "https://"+e.app.Config.Host+path, nil)
	req.Header.Set("Accept", accept)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// pendingJobs lists the jobs that wait in the queue, whenever they are due.
func (e *eventBoardEnv) pendingJobs() []queue.PendingJob {
	e.t.Helper()
	jobs, err := e.app.JobInspector.ListJobs(e.t.Context(), e.clock.Now().Add(365*24*time.Hour), queue.JobFilter{}, queue.JobPosition{}, 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	return jobs
}

func (e *eventBoardEnv) upload(user string) string {
	e.t.Helper()
	m, err := e.app.Media.Upload(e.t.Context(), appcontent.UploadInput{OwnerAccountID: e.ids[user], Content: bytes.NewReader(testkit.FixturePNG(64, 48, false))})
	if err != nil {
		e.t.Fatalf("Upload: %v", err)
	}
	e.app.Media.Process(e.t.Context(), m.File.ID, "")
	return m.File.ID
}

// eventBoardUsers are the users of the tests of the boards: the organizer, a co-organizer who
// does not answer, attendees who answer going, maybe, and not going, an invitee who has not
// answered, and a stranger who sees the public events but never answers.
var eventBoardUsers = []string{"alice", "cora", "gina", "mona", "dan", "ivy", "sam"}

// cast gives each user of eventBoardUsers their part in the event.
func (e *eventBoardEnv) cast(eventID string) {
	e.t.Helper()
	ctx := e.t.Context()
	if _, err := e.app.Events.AssignCoorganizer(ctx, e.ids["alice"], eventID, e.ids["cora"]); err != nil {
		e.t.Fatalf("AssignCoorganizer = %v", err)
	}
	e.answer("gina", eventID, gathering.AnswerGoing)
	e.answer("mona", eventID, gathering.AnswerMaybe)
	e.answer("dan", eventID, gathering.AnswerDeclined)
	if _, err := e.app.Events.Invite(ctx, e.ids["alice"], eventID, e.ids["ivy"]); err != nil {
		e.t.Fatalf("Invite = %v", err)
	}
}

// TestEventBoardWritersThroughTheAPI checks, for each choice of board_writers, who reads and who
// writes on the board through the API (ADR-0161 §17): the organizer, the co-organizers, and those
// who answer going or maybe read it; who declined, an invitee who has not answered, and a stranger
// who sees the event do not; and board_writers decides who among the readers posts.
func TestEventBoardWritersThroughTheAPI(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, eventBoardUsers...)
	readers := []string{"alice", "cora", "gina", "mona"}
	for _, tc := range []struct {
		writers string // "" for the default
		want    clientapi.EventBoardWriters
		writes  []string
	}{
		{"organizer", clientapi.EventBoardWritersOrganizer, []string{"alice"}},
		{"", clientapi.EventBoardWritersOrganizers, []string{"alice", "cora"}},
		{"organizers", clientapi.EventBoardWritersOrganizers, []string{"alice", "cora"}},
		{"readers", clientapi.EventBoardWritersReaders, readers},
	} {
		ev := e.createEvent(tc.writers, nil)
		if ev.BoardWriters != tc.want {
			t.Errorf("writers %q: created with board_writers %q, want %q", tc.writers, ev.BoardWriters, tc.want)
		}
		e.cast(ev.Id)
		for _, user := range eventBoardUsers {
			reads, writes := slices.Contains(readers, user), slices.Contains(tc.writes, user)
			got := e.event(user, ev.Id)
			if got.BoardWriters != tc.want || got.Viewer.Board.Readable != reads || got.Viewer.Board.Writable != writes {
				t.Errorf("writers %q, %s: board_writers %q, viewer.board %+v, want %q readable %v writable %v",
					tc.writers, user, got.BoardWriters, got.Viewer.Board, tc.want, reads, writes)
			}
			wantList, wantPost := "200", "201"
			switch {
			case !reads:
				wantList = "403 " + clientapi.ProblemTypeEventBoardNotReader
				wantPost = wantList
			case !writes:
				wantPost = "403 " + clientapi.ProblemTypeEventBoardNotWriter
			}
			if got, _ := e.board(user, ev.Id, ""); got != wantList {
				t.Errorf("writers %q: GET the board as %s = %s, want %s", tc.writers, user, got, wantList)
			}
			var st clientapi.Status
			if got := e.outcome(e.postOnBoard(user, ev.Id, map[string]any{"text": "hello from " + user}, &st)); got != wantPost {
				t.Errorf("writers %q: POST on the board as %s = %s, want %s", tc.writers, user, got, wantPost)
			}
		}
		// Every reader lists the posts of the writers, newest first.
		want := slices.Clone(tc.writes)
		slices.Reverse(want)
		for _, user := range readers {
			got, page := e.board(user, ev.Id, "")
			var texts []string
			for _, s := range page.Items {
				texts = append(texts, strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(s.Content, "<p>"), "</p>"), "hello from "))
				if s.EventBoard == nil || s.EventBoard.EventId != ev.Id {
					t.Errorf("writers %q: a post on the board has event_board %+v, want %s", tc.writers, s.EventBoard, ev.Id)
				}
			}
			if got != "200" || !slices.Equal(texts, want) {
				t.Errorf("writers %q: the board as %s = %s %v, want the posts of %v", tc.writers, user, got, texts, want)
			}
		}
	}

	// Only the organizer changes who writes.
	ev := e.createEvent("", nil)
	e.cast(ev.Id)
	if got := e.outcome(e.send("cora", http.MethodPatch, "/events/"+ev.Id, map[string]any{"board_writers": "readers"}, nil, nil)); got != "403 "+clientapi.ProblemTypeEventOrganizerOnly {
		t.Errorf("a co-organizer changes board_writers = %s, want 403 %s", got, clientapi.ProblemTypeEventOrganizerOnly)
	}
	var changed clientapi.Event
	if res := e.send("alice", http.MethodPatch, "/events/"+ev.Id, map[string]any{"board_writers": "readers"}, nil, &changed); res.StatusCode != http.StatusOK ||
		changed.BoardWriters != clientapi.EventBoardWritersReaders {
		t.Errorf("the organizer changes board_writers = %d %q, want 200 readers", res.StatusCode, changed.BoardWriters)
	}
	if got := e.event("gina", ev.Id).Viewer.Board; !got.Readable || !got.Writable {
		t.Errorf("going after board_writers became readers: viewer.board %+v, want readable and writable", got)
	}
	if got := e.outcome(e.send("alice", http.MethodPatch, "/events/"+ev.Id, map[string]any{"board_writers": "everyone"}, nil, nil)); !strings.HasPrefix(got, "4") {
		t.Errorf("an unknown board_writers = %s, want a 4xx", got)
	}

	// A board of an event that the user does not see, or that does not exist, is a 404.
	hidden := e.createEvent("readers", map[string]any{"visibility": "invited"})
	for _, path := range []string{"/events/" + hidden.Id + "/board", "/events/" + eventBoardMissingID + "/board"} {
		if got := e.outcome(e.send("sam", http.MethodGet, path, nil, nil, nil)); !strings.HasPrefix(got, "404") {
			t.Errorf("GET %s as a user who does not see the event = %s, want 404", path, got)
		}
		if got := e.outcome(e.postOnBoard("sam", strings.TrimSuffix(strings.TrimPrefix(path, "/events/"), "/board"), map[string]any{"text": "hi"}, nil)); !strings.HasPrefix(got, "404") {
			t.Errorf("POST %s as a user who does not see the event = %s, want 404", path, got)
		}
	}
}

// TestEventBoardPostsThroughTheAPI checks that a writer posts on a board what an ordinary post
// carries (media, a content warning, a poll, mentions, replies on the same board), that the readers
// read, favourite, react to, and vote in the posts by their IDs, that its author edits and deletes
// a post while the board admits them, and that nobody else reaches a post by its ID (ADR-0161 §17).
func TestEventBoardPostsThroughTheAPI(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, eventBoardUsers...)
	ev := e.createEvent("readers", nil)
	e.cast(ev.Id)
	other := e.createEvent("readers", nil)
	e.answer("gina", other.Id, gathering.AnswerGoing)

	// Media, a content warning, and the sensitivity of the media.
	media := e.upload("gina")
	withMedia := e.mustPostOnBoard("gina", ev.Id, map[string]any{
		"text": "the map to the park", "spoiler_text": "directions", "sensitive": true, "language": "en",
		"media": []map[string]any{{"id": media, "description": "a map"}},
	})
	if len(withMedia.MediaAttachments) != 1 || withMedia.SpoilerText != "directions" || !withMedia.Sensitive || withMedia.Language != "en" ||
		withMedia.EventBoard == nil || withMedia.EventBoard.EventId != ev.Id {
		t.Errorf("a post with media = %+v", withMedia)
	}
	// The same Idempotency-Key answers with the same post.
	var first, again clientapi.Status
	key := map[string]string{"Idempotency-Key": "the-same-key"}
	body := map[string]any{"text": "posted once"}
	if res := e.send("gina", http.MethodPost, "/events/"+ev.Id+"/board", body, key, &first); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST with a key = %d", res.StatusCode)
	}
	if res := e.send("gina", http.MethodPost, "/events/"+ev.Id+"/board", body, key, &again); res.StatusCode >= 300 || again.Id != first.Id {
		t.Errorf("POST again with the key = %d %s, want the post %s", res.StatusCode, again.Id, first.Id)
	}
	// A poll.
	poll := e.mustPostOnBoard("mona", ev.Id, map[string]any{
		"text": "what do we bring?", "poll": map[string]any{"options": []string{"snacks", "drinks"}, "expires_in": 3600},
	})
	if poll.Poll == nil || len(poll.Poll.Options) != 2 {
		t.Errorf("a post with a poll = %+v", poll)
	}
	// Mentions of a reader and of a user who does not read the board.
	mention := e.mustPostOnBoard("alice", ev.Id, map[string]any{"text": "@mona @sam see you there"})
	if !slices.ContainsFunc(mention.Mentions, func(m clientapi.StatusMention) bool { return m.Id == e.ids["mona"] }) {
		t.Errorf("a post mentioning a reader has the mentions %+v", mention.Mentions)
	}
	// A reply on the same board.
	reply := e.mustPostOnBoard("cora", ev.Id, map[string]any{"text": "thanks", "in_reply_to_id": withMedia.Id})
	if reply.InReplyToId == nil || *reply.InReplyToId != withMedia.Id || reply.EventBoard == nil || reply.EventBoard.EventId != ev.Id {
		t.Errorf("a reply on the board = %+v", reply)
	}
	// No reply to an ordinary status, or to a post of another board.
	ordinary := e.post("gina", map[string]any{"text": "an ordinary post", "visibility": "public"})
	elsewhere := e.mustPostOnBoard("gina", other.Id, map[string]any{"text": "on another board"})
	for name, target := range map[string]string{"an ordinary status": ordinary.Id, "a post of another board": elsewhere.Id, "a missing status": eventBoardMissingID} {
		if got := e.outcome(e.postOnBoard("gina", ev.Id, map[string]any{"text": "a reply", "in_reply_to_id": target}, nil)); !strings.HasPrefix(got, "422") {
			t.Errorf("a board reply to %s = %s, want 422", name, got)
		}
	}

	// The list, newest first, a page at a time.
	want := []string{reply.Id, mention.Id, poll.Id, first.Id, withMedia.Id}
	if got := e.boardIDs("alice", ev.Id); !slices.Equal(got, want) {
		t.Errorf("the board = %v, want %v", got, want)
	}
	var paged []string
	query := "?limit=2"
	for range len(want) {
		got, page := e.board("mona", ev.Id, query)
		if got != "200" || len(page.Items) > 2 {
			t.Fatalf("GET the board%s = %s with %d posts", query, got, len(page.Items))
		}
		paged = append(paged, eventBoardStatusIDs(page.Items)...)
		if page.NextCursor == nil {
			break
		}
		query = "?limit=2&cursor=" + *page.NextCursor
	}
	if !slices.Equal(paged, want) {
		t.Errorf("the board page by page = %v, want %v", paged, want)
	}

	// A reader gets a post by its ID, with what it may do with it; nobody else does.
	for user, replies := range map[string]clientapi.InteractionPermission{"gina": clientapi.InteractionPermissionAllowed, "mona": clientapi.InteractionPermissionAllowed} {
		var st clientapi.Status
		if res := e.send(user, http.MethodGet, "/statuses/"+withMedia.Id, nil, nil, &st); res.StatusCode != http.StatusOK {
			t.Errorf("GET a board post as the reader %s = %d", user, res.StatusCode)
			continue
		}
		if st.EventBoard == nil || st.EventBoard.EventId != ev.Id || st.Permissions == nil || st.Permissions.Announce != clientapi.InteractionPermissionDenied ||
			st.Permissions.Quote != clientapi.InteractionPermissionDenied || st.Permissions.Reply != replies {
			t.Errorf("a board post as %s: event_board %+v, permissions %+v, want boost and quote denied and reply %s", user, st.EventBoard, st.Permissions, replies)
		}
	}
	for _, user := range []string{"", "dan", "ivy", "sam"} {
		for _, suffix := range []string{"", "/context", "/history", "/reactions"} {
			if got, want := problemShape(t, e.send(user, http.MethodGet, "/statuses/"+withMedia.Id+suffix, nil, nil, nil)), e.missing(user, http.MethodGet, suffix, nil); got != want {
				t.Errorf("GET a board post%s as %q = %s, want the answer to a missing status %s", suffix, user, got, want)
			}
		}
	}
	// What a user who does not read the board would do with a post answers as a missing status.
	for _, user := range []string{"dan", "sam"} {
		for _, act := range []struct {
			method, suffix string
			body           any
		}{
			{http.MethodPost, "/favourite", nil},
			{http.MethodPut, "/reaction", map[string]any{"emoji": "🎉"}},
			{http.MethodPost, "/poll/votes", map[string]any{"poll_id": poll.Poll.Id, "choices": []int{0}}},
		} {
			target := withMedia.Id
			if act.suffix == "/poll/votes" {
				target = poll.Id
			}
			if got, want := problemShape(t, e.send(user, act.method, "/statuses/"+target+act.suffix, act.body, nil, nil)), e.missing(user, act.method, act.suffix, act.body); got != want {
				t.Errorf("%s %s of a board post as %s = %s, want the answer to a missing status %s", act.method, act.suffix, user, got, want)
			}
		}
	}

	// Readers favourite, react to, and vote in the posts.
	var st clientapi.Status
	if res := e.send("mona", http.MethodPost, "/statuses/"+withMedia.Id+"/favourite", nil, nil, &st); res.StatusCode != http.StatusOK || st.FavouritesCount != 1 {
		t.Errorf("favourite a board post = %d %+v", res.StatusCode, st)
	}
	if res := e.send("cora", http.MethodPut, "/statuses/"+withMedia.Id+"/reaction", map[string]any{"emoji": "🎉"}, nil, &st); res.StatusCode != http.StatusOK || len(st.Reactions) != 1 {
		t.Errorf("react to a board post = %d %+v", res.StatusCode, st.Reactions)
	}
	if res := e.send("gina", http.MethodPost, "/statuses/"+poll.Id+"/poll/votes", map[string]any{"poll_id": poll.Poll.Id, "choices": []int{1}}, nil, &st); res.StatusCode != http.StatusOK {
		t.Errorf("vote in the poll of a board post = %s", e.outcome(res))
	}
	// A board post is never boosted, bookmarked, or pinned: a 404, its author included.
	for _, user := range []string{"gina", "mona"} {
		for _, action := range []string{"boost", "bookmark", "pin"} {
			if got := e.outcome(e.send(user, http.MethodPost, "/statuses/"+withMedia.Id+"/"+action, nil, nil, nil)); !strings.HasPrefix(got, "404") {
				t.Errorf("%s a board post as %s = %s, want 404", action, user, got)
			}
		}
	}

	// The author edits a post; nobody else does.
	var edited clientapi.Status
	if res := e.send("gina", http.MethodPut, "/statuses/"+withMedia.Id, map[string]any{"text": "the map, corrected"}, nil, &edited); res.StatusCode != http.StatusOK ||
		edited.EditedAt == nil || !strings.Contains(edited.Content, "corrected") || edited.EventBoard == nil {
		t.Errorf("the author edits a board post = %s %+v", e.outcome(res), edited)
	}
	if got := e.outcome(e.send("mona", http.MethodPut, "/statuses/"+withMedia.Id, map[string]any{"text": "mine"}, nil, nil)); !strings.HasPrefix(got, "403") {
		t.Errorf("a reader edits another's board post = %s, want 403", got)
	}
	if got := e.outcome(e.send("sam", http.MethodPut, "/statuses/"+withMedia.Id, map[string]any{"text": "mine"}, nil, nil)); !strings.HasPrefix(got, "404") {
		t.Errorf("a user who does not read the board edits a board post = %s, want 404", got)
	}

	// The author deletes a post: it leaves the board.
	if got := e.outcome(e.send("mona", http.MethodDelete, "/statuses/"+first.Id, nil, nil, nil)); !strings.HasPrefix(got, "403") {
		t.Errorf("a reader deletes another's board post = %s, want 403", got)
	}
	if got := e.outcome(e.send("gina", http.MethodDelete, "/statuses/"+first.Id, nil, nil, nil)); got != "204" {
		t.Errorf("the author deletes a board post = %s, want 204", got)
	}
	if slices.Contains(e.boardIDs("alice", ev.Id), first.Id) {
		t.Error("a deleted post is still on the board")
	}

	// Once board_writers leaves the author out, the author no longer edits nor replies, and still
	// deletes the post.
	if res := e.send("alice", http.MethodPatch, "/events/"+ev.Id, map[string]any{"board_writers": "organizers"}, nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH board_writers = %d", res.StatusCode)
	}
	if got := e.outcome(e.send("gina", http.MethodPut, "/statuses/"+withMedia.Id, map[string]any{"text": "again"}, nil, nil)); got != "403 "+clientapi.ProblemTypeEventBoardNotWriter {
		t.Errorf("an edit once the author no longer writes = %s, want 403 %s", got, clientapi.ProblemTypeEventBoardNotWriter)
	}
	if got := e.outcome(e.postOnBoard("gina", ev.Id, map[string]any{"text": "again"}, nil)); got != "403 "+clientapi.ProblemTypeEventBoardNotWriter {
		t.Errorf("a post once the user no longer writes = %s, want 403 %s", got, clientapi.ProblemTypeEventBoardNotWriter)
	}
	if res := e.send("gina", http.MethodGet, "/statuses/"+withMedia.Id, nil, nil, &st); res.StatusCode != http.StatusOK || st.Permissions == nil ||
		st.Permissions.Reply != clientapi.InteractionPermissionDenied {
		t.Errorf("a board post to a reader who no longer writes = %d, permissions %+v, want reply denied", res.StatusCode, st.Permissions)
	}
	if got := e.outcome(e.send("gina", http.MethodDelete, "/statuses/"+withMedia.Id, nil, nil, nil)); got != "204" {
		t.Errorf("the author deletes a post once no longer a writer = %s, want 204", got)
	}

	// A mention reaches nobody who does not read the board.
	e.drain()
	var notes clientapi.NotificationPage
	if res := e.send("sam", http.MethodGet, "/notifications", nil, nil, &notes); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /notifications = %d", res.StatusCode)
	}
	for _, n := range notes.Items {
		if n.Status != nil && n.Status.Id == mention.Id {
			t.Errorf("a user who does not read the board is notified of a board post: %+v", n)
		}
	}
}

// TestEventBoardReadersFollowTheirAnswers checks that the readers of a board follow the answers
// and the co-organizers as they change (ADR-0161 §17): one who changes to not going stops reading
// it, and the posts written before stay; answering maybe again reads it again; a co-organizer
// removed stops reading it.
func TestEventBoardReadersFollowTheirAnswers(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, eventBoardUsers...)
	ev := e.createEvent("readers", nil)
	e.cast(ev.Id)
	post := e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": "I bring the cards"})
	missing := e.missing("gina", http.MethodGet, "", nil)

	e.answer("gina", ev.Id, gathering.AnswerDeclined)
	if got := e.event("gina", ev.Id).Viewer.Board; got.Readable || got.Writable {
		t.Errorf("after not going: viewer.board %+v, want neither readable nor writable", got)
	}
	if got, _ := e.board("gina", ev.Id, ""); got != "403 "+clientapi.ProblemTypeEventBoardNotReader {
		t.Errorf("after not going: GET the board = %s, want 403 %s", got, clientapi.ProblemTypeEventBoardNotReader)
	}
	if got := e.outcome(e.postOnBoard("gina", ev.Id, map[string]any{"text": "still here?"}, nil)); got != "403 "+clientapi.ProblemTypeEventBoardNotReader {
		t.Errorf("after not going: POST on the board = %s, want 403 %s", got, clientapi.ProblemTypeEventBoardNotReader)
	}
	if got := problemShape(t, e.send("gina", http.MethodGet, "/statuses/"+post.Id, nil, nil, nil)); got != missing {
		t.Errorf("after not going: GET the own board post = %s, want the answer to a missing status %s", got, missing)
	}
	if got := e.outcome(e.send("gina", http.MethodPut, "/statuses/"+post.Id, map[string]any{"text": "edited"}, nil, nil)); !strings.HasPrefix(got, "4") {
		t.Errorf("after not going: edit the own board post = %s, want a 4xx", got)
	}
	// The post stays for the readers.
	if got := e.boardIDs("alice", ev.Id); !slices.Equal(got, []string{post.Id}) {
		t.Errorf("after its author stopped going, the board = %v, want %v", got, []string{post.Id})
	}

	e.answer("gina", ev.Id, gathering.AnswerMaybe)
	if got := e.event("gina", ev.Id).Viewer.Board; !got.Readable || !got.Writable {
		t.Errorf("after maybe again: viewer.board %+v, want readable and writable", got)
	}
	if got := e.boardIDs("gina", ev.Id); !slices.Equal(got, []string{post.Id}) {
		t.Errorf("after maybe again, the board = %v, want %v", got, []string{post.Id})
	}

	if _, err := e.app.Events.RemoveCoorganizer(t.Context(), e.ids["alice"], ev.Id, e.ids["cora"]); err != nil {
		t.Fatalf("RemoveCoorganizer = %v", err)
	}
	if got, _ := e.board("cora", ev.Id, ""); got != "403 "+clientapi.ProblemTypeEventBoardNotReader {
		t.Errorf("a removed co-organizer who never answered: GET the board = %s, want 403 %s", got, clientapi.ProblemTypeEventBoardNotReader)
	}
}

// TestEventBoardTerm checks the term of a board (ADR-0161 §17): it is read-only at once when the
// event is cancelled and from 7 days after its end, it is still read then, and an author still
// deletes a post.
func TestEventBoardTerm(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	ctx := t.Context()
	cancelled := e.createEvent("readers", nil)
	ended := e.createEvent("readers", nil)
	posts := map[string][]string{}
	for _, id := range []string{cancelled.Id, ended.Id} {
		e.answer("gina", id, gathering.AnswerGoing)
		posts[id] = append(posts[id], e.mustPostOnBoard("gina", id, map[string]any{"text": "going!"}).Id)
	}

	// Cancelled: read-only at once.
	if _, err := e.app.Events.Cancel(ctx, e.ids["alice"], cancelled.Id, ""); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	closed := "409 " + clientapi.ProblemTypeEventBoardClosed
	for _, user := range []string{"alice", "gina"} {
		if got := e.outcome(e.postOnBoard(user, cancelled.Id, map[string]any{"text": "still on?"}, nil)); got != closed {
			t.Errorf("POST on the board of a cancelled event as %s = %s, want %s", user, got, closed)
		}
		if got := e.event(user, cancelled.Id).Viewer.Board; !got.Readable || got.Writable {
			t.Errorf("a cancelled event as %s: viewer.board %+v, want readable, not writable", user, got)
		}
		if got := e.boardIDs(user, cancelled.Id); !slices.Equal(got, posts[cancelled.Id]) {
			t.Errorf("the board of a cancelled event as %s = %v, want %v", user, got, posts[cancelled.Id])
		}
	}
	if got := e.outcome(e.send("gina", http.MethodPut, "/statuses/"+posts[cancelled.Id][0], map[string]any{"text": "edited"}, nil, nil)); got != closed {
		t.Errorf("an edit on the board of a cancelled event = %s, want %s", got, closed)
	}
	// A post that its author deletes once the board is read-only.
	gone := e.mustPostOnBoard("gina", ended.Id, map[string]any{"text": "to delete later"})

	// Ended: writable until 7 days after the end, read-only after.
	e.at(eventBoardEndsIn + 7*24*time.Hour - time.Minute)
	if got := e.event("gina", ended.Id).Viewer.Board; !got.Writable {
		t.Errorf("a minute before 7 days after the end: viewer.board %+v, want writable", got)
	}
	late := e.mustPostOnBoard("gina", ended.Id, map[string]any{"text": "thanks everyone"})
	posts[ended.Id] = append(posts[ended.Id], gone.Id, late.Id)
	if res := e.send("gina", http.MethodPut, "/statuses/"+late.Id, map[string]any{"text": "thanks, everyone"}, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("an edit a minute before 7 days after the end = %s, want 200", e.outcome(res))
	}
	e.at(eventBoardEndsIn + 7*24*time.Hour + time.Minute)
	if got := e.event("gina", ended.Id).Viewer.Board; !got.Readable || got.Writable {
		t.Errorf("a minute after 7 days after the end: viewer.board %+v, want readable, not writable", got)
	}
	if got := e.outcome(e.postOnBoard("alice", ended.Id, map[string]any{"text": "one more"}, nil)); got != closed {
		t.Errorf("POST a minute after 7 days after the end = %s, want %s", got, closed)
	}
	if got := e.outcome(e.send("gina", http.MethodPut, "/statuses/"+late.Id, map[string]any{"text": "edited"}, nil, nil)); got != closed {
		t.Errorf("an edit a minute after 7 days after the end = %s, want %s", got, closed)
	}
	if got := e.outcome(e.send("gina", http.MethodDelete, "/statuses/"+gone.Id, nil, nil, nil)); got != "204" {
		t.Errorf("the author deletes a post of a read-only board = %s, want 204", got)
	}
	posts[ended.Id] = slices.DeleteFunc(posts[ended.Id], func(id string) bool { return id == gone.Id })
	if got, want := e.boardIDs("gina", ended.Id), []string{late.Id, posts[ended.Id][0]}; !slices.Equal(got, want) {
		t.Errorf("a read-only board = %v, want %v", got, want)
	}

}

// eventBoardRecords are events whose boards go with their records: one cancelled at the start of
// the clock, one that ends, one that nobody answered nor was invited to, and one that still polls
// its date; and the posts on each, and an ordinary post.
type eventBoardRecords struct {
	cancelled, ended, quiet, polling string
	posts                            map[string][]string
	ordinary                         string
}

func (e *eventBoardEnv) records() eventBoardRecords {
	e.t.Helper()
	ctx := e.t.Context()
	r := eventBoardRecords{
		cancelled: e.createEvent("readers", nil).Id, ended: e.createEvent("readers", nil).Id, quiet: e.createEvent("readers", nil).Id,
		posts: map[string][]string{},
	}
	polling, err := e.app.Events.Create(ctx, e.ids["alice"], appgathering.CreateInput{
		Settings: gathering.Settings{Title: "When do we meet?", TimeZone: "Asia/Tokyo", BoardWriters: gathering.BoardWritersReaders},
		Candidates: []gathering.Schedule{
			{StartsAt: eventBoardStart.Add(48 * time.Hour), EndsAt: eventBoardStart.Add(50 * time.Hour)},
			{StartsAt: eventBoardStart.Add(72 * time.Hour), EndsAt: eventBoardStart.Add(74 * time.Hour)},
		},
	})
	if err != nil {
		e.t.Fatalf("Create a date poll = %v", err)
	}
	r.polling = polling.ID
	for _, id := range []string{r.cancelled, r.ended} {
		e.answer("gina", id, gathering.AnswerGoing)
		r.posts[id] = []string{e.mustPostOnBoard("gina", id, map[string]any{"text": "going!"}).Id}
	}
	r.posts[r.quiet] = []string{e.mustPostOnBoard("alice", r.quiet, map[string]any{"text": "anyone?"}).Id}
	r.posts[r.polling] = []string{e.mustPostOnBoard("alice", r.polling, map[string]any{"text": "vote for a date"}).Id}
	r.ordinary = e.post("alice", map[string]any{"text": "an ordinary post", "visibility": "public"}).Id
	if _, err := e.app.Events.Cancel(ctx, e.ids["alice"], r.cancelled, ""); err != nil {
		e.t.Fatalf("Cancel = %v", err)
	}
	return r
}

// TestEventBoardsToPurge checks which boards are due to go with their events' records (ADR-0161
// §14, §17): none within event_records_retention_days of the end or the cancellation; after it,
// those of the ended and the cancelled events, that of an event nobody answered too, and an
// unknown event; never that of an event that still polls its date.
func TestEventBoardsToPurge(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	r := e.records()
	all := []string{r.cancelled, r.ended, r.quiet, r.polling, eventBoardMissingID}
	e.at(eventBoardEndsIn + 89*24*time.Hour)
	if due, err := e.app.Events.BoardsToPurge(t.Context(), all); err != nil || !slices.Equal(due, []string{eventBoardMissingID}) {
		t.Errorf("BoardsToPurge within the retention = %v, %v, want the unknown event alone", due, err)
	}
	e.at(eventBoardEndsIn + 91*24*time.Hour)
	want := []string{r.cancelled, r.ended, r.quiet, eventBoardMissingID}
	if due, err := e.app.Events.BoardsToPurge(t.Context(), all); err != nil || !slices.Equal(due, want) {
		t.Errorf("BoardsToPurge after the retention = %v, %v, want %v", due, err, want)
	}
	// Answering deletes nothing: the posts go with the purge alone.
	for _, id := range []string{r.cancelled, r.ended, r.quiet, r.polling} {
		if got := e.boardIDs("alice", id); !slices.Equal(got, r.posts[id]) {
			t.Errorf("after BoardsToPurge, the board of %s = %v, want %v", id, got, r.posts[id])
		}
	}
	if res := e.send("alice", http.MethodGet, "/statuses/"+r.ordinary, nil, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("the ordinary post = %d, want 200", res.StatusCode)
	}
}
