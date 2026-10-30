// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// listCall sends a request of the lists as viewer and returns the response, validated against
// the spec.
func (e *timelineAPI) listCall(t *testing.T, viewer timelineUser, method, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, clientapi.BasePath+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+viewer.token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return e.spec.Serve(t, e.h, req)
}

func decodeJSON[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestListsAPI drives the lists on the real database: creating and renaming one, adding only
// the accounts the owner follows, the list timeline, the limit of lists, and that another user
// gets a 404 for the list, its members, and its timeline.
func TestListsAPI(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice := e.userWithScope(t, "alice", "read write")
	bob, carol := e.user(t, "bob"), e.userWithScope(t, "carol", "read write")
	e.follow(t, alice, bob)

	res := e.listCall(t, alice, http.MethodPost, "/lists", `{"name":"friends"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /lists = %d", res.StatusCode)
	}
	list := decodeJSON[clientapi.List](t, res)
	if list.Name != "friends" {
		t.Errorf("created list = %+v", list)
	}
	if res := e.listCall(t, alice, http.MethodPatch, "/lists/"+list.Id, `{"name":"close friends"}`); res.StatusCode != http.StatusOK ||
		decodeJSON[clientapi.List](t, res).Name != "close friends" {
		t.Errorf("PATCH /lists/{id} = %d", res.StatusCode)
	}
	if res := e.listCall(t, alice, http.MethodPost, "/lists", `{"name":" "}`); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("POST /lists with a blank name = %d, want 422", res.StatusCode)
	}

	// Only the accounts that alice follows join.
	member := "/lists/" + list.Id + "/accounts/"
	res = e.listCall(t, alice, http.MethodPost, member+carol.accountID, "")
	if p := decodeJSON[clientapi.Problem](t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeNotFollowed {
		t.Errorf("adding an account alice does not follow = %d %+v, want 422 not-followed", res.StatusCode, p)
	}
	if res := e.listCall(t, alice, http.MethodPost, member+bob.accountID, ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("adding bob = %d, want 204", res.StatusCode)
	}
	res = e.listCall(t, alice, http.MethodGet, "/lists/"+list.Id+"/accounts", "")
	if page := decodeJSON[clientapi.AccountPage](t, res); len(page.Accounts) != 1 || page.Accounts[0].Id != bob.accountID {
		t.Errorf("members = %+v, want bob", page.Accounts)
	}
	res = e.listCall(t, alice, http.MethodGet, "/accounts/"+bob.accountID+"/lists", "")
	if c := decodeJSON[clientapi.ListCollection](t, res); len(c.Items) != 1 || c.Items[0].Id != list.Id {
		t.Errorf("bob's lists = %+v, want the list", c.Items)
	}

	// The timeline of the list has bob's posts and not alice's own.
	bobs := e.post(t, bob, "hello", domcontent.VisibilityPublic, "en")
	e.post(t, alice, "mine", domcontent.VisibilityPublic, "en")
	if got := itemIDs(e.page(t, &alice, "/timelines/list/"+list.Id)); !slices.Equal(got, []string{bobs}) {
		t.Errorf("list timeline = %v, want bob's post", got)
	}

	// Another user sees nothing of the list.
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/lists/" + list.Id, ""},
		{http.MethodPatch, "/lists/" + list.Id, `{"name":"mine"}`},
		{http.MethodDelete, "/lists/" + list.Id, ""},
		{http.MethodGet, "/lists/" + list.Id + "/accounts", ""},
		{http.MethodPost, member + bob.accountID, ""},
		{http.MethodDelete, member + bob.accountID, ""},
		{http.MethodGet, "/timelines/list/" + list.Id, ""},
	} {
		if res := e.listCall(t, carol, c.method, c.path, c.body); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s by another user = %d, want 404", c.method, c.path, res.StatusCode)
		}
	}
	res = e.listCall(t, carol, http.MethodGet, "/lists", "")
	if c := decodeJSON[clientapi.ListCollection](t, res); len(c.Items) != 0 {
		t.Errorf("carol's lists = %+v, want none", c.Items)
	}

	// The limit of lists.
	for i := 1; i < timeline.MaxLists; i++ {
		if res := e.listCall(t, alice, http.MethodPost, "/lists", fmt.Sprintf(`{"name":"list %d"}`, i)); res.StatusCode != http.StatusOK {
			t.Fatalf("POST /lists #%d = %d", i, res.StatusCode)
		}
	}
	res = e.listCall(t, alice, http.MethodPost, "/lists", `{"name":"one more"}`)
	if p := decodeJSON[clientapi.Problem](t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeTooManyLists {
		t.Errorf("POST /lists beyond the limit = %d %+v, want 422 too-many-lists", res.StatusCode, p)
	}

	// Unfollowing takes bob out of the list; deleting the list deletes it.
	if _, err := e.app.Graph.Unfollow(t.Context(), alice.accountID, bob.accountID); err != nil {
		t.Fatal(err)
	}
	res = e.listCall(t, alice, http.MethodGet, "/lists/"+list.Id+"/accounts", "")
	if page := decodeJSON[clientapi.AccountPage](t, res); len(page.Accounts) != 0 {
		t.Errorf("members after the unfollow = %+v, want none", page.Accounts)
	}
	if res := e.listCall(t, alice, http.MethodDelete, "/lists/"+list.Id, ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE /lists/{id} = %d, want 204", res.StatusCode)
	}
	if res := e.listCall(t, alice, http.MethodGet, "/lists/"+list.Id, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET a deleted list = %d, want 404", res.StatusCode)
	}
}

// listUpdateData is the data of a list_update event.
type listUpdateData struct {
	ListID string           `json:"list_id"`
	Status clientapi.Status `json:"status"`
}

// TestStreamingDeliversListUpdates checks that a new post of a member arrives on the stream as
// update and as list_update for each list of the viewer that has its author.
func TestStreamingDeliversListUpdates(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob, carol := e.userWithScope(t, "alice", "read write"), e.user(t, "bob"), e.user(t, "carol")
	e.follow(t, alice, bob)
	e.follow(t, alice, carol)
	var lists []string
	for _, name := range []string{"a", "b"} {
		res := e.listCall(t, alice, http.MethodPost, "/lists", `{"name":"`+name+`"}`)
		id := decodeJSON[clientapi.List](t, res).Id
		lists = append(lists, id)
		if res := e.listCall(t, alice, http.MethodPost, "/lists/"+id+"/accounts/"+bob.accountID, ""); res.StatusCode != http.StatusNoContent {
			t.Fatalf("adding bob = %d", res.StatusCode)
		}
	}

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	ctx, disconnect := context.WithCancel(t.Context())
	defer disconnect()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+clientapi.BasePath+"/streaming", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+alice.token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /streaming: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	events := readEvents(bufio.NewReader(res.Body))
	next := func() sseEvent {
		t.Helper()
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("the stream ended")
			}
			return ev
		case <-time.After(5 * time.Second):
			t.Fatal("no event within 5 seconds")
			return sseEvent{}
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.app.Streams.Len() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the stream did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// carol is in no list: her post is an update only. bob's is an update and a list_update
	// for each of the two lists.
	e.post(t, carol, "no list", domcontent.VisibilityPublic, "en")
	if ev := next(); ev.name != "update" {
		t.Fatalf("event %q, want update", ev.name)
	}
	bobs := e.post(t, bob, "listed", domcontent.VisibilityPublic, "en")
	if ev := next(); ev.name != "update" {
		t.Fatalf("event %q (%s), want update", ev.name, ev.data)
	}
	var got []string
	for range lists {
		ev := next()
		if ev.name != "list_update" {
			t.Fatalf("event %q (%s), want list_update", ev.name, ev.data)
		}
		var d listUpdateData
		if err := json.Unmarshal(ev.data, &d); err != nil {
			t.Fatal(err)
		}
		e.spec.ValidateSchema(t, "Status", mustMarshal(t, d.Status))
		if d.Status.Id != bobs {
			t.Errorf("list_update status = %s, want bob's post %s", d.Status.Id, bobs)
		}
		got = append(got, d.ListID)
	}
	slices.Sort(got)
	slices.Sort(lists)
	if !slices.Equal(got, lists) {
		t.Errorf("list_update lists = %v, want %v", got, lists)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
