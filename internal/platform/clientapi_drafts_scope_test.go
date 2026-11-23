// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestDirectDraftsNeedReadDirect checks that a token without read:direct does not see the user's
// direct drafts (ADR-0090 §4): they are left out of the list, and reading, saving over, scheduling,
// and deleting one answer 404 and change nothing; write:statuses alone still saves a direct draft,
// seeing it in the answer, and publishes one.
func TestDirectDraftsNeedReadDirect(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "bob")
	narrow := e.appToken("alice", "read:statuses write:statuses")
	wide := e.appToken("alice", "read:statuses read:direct")

	save := func(token, visibility string) clientapi.Draft {
		t.Helper()
		var d clientapi.Draft
		body := map[string]any{"text": visibility + " @bob", "visibility": visibility}
		if res := e.send(token, http.MethodPost, "/drafts", body, nil, &d); res.StatusCode != http.StatusCreated {
			t.Fatalf("POST /drafts %s = %s, want 201", visibility, e.outcome(res))
		}
		return d
	}
	direct := save(narrow, "direct")
	if direct.Visibility != clientapi.VisibilityDirect {
		t.Errorf("the direct draft just saved = %+v, want it shown", direct)
	}
	public := save(narrow, "public")
	toPublish := save(narrow, "direct")

	list := func(token string) []string {
		t.Helper()
		var page clientapi.DraftPage
		if res := e.send(token, http.MethodGet, "/drafts", nil, nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /drafts = %s", e.outcome(res))
		}
		ids := []string{}
		for _, d := range page.Items {
			ids = append(ids, d.Id)
		}
		return ids
	}
	if got, want := list(narrow), []string{public.Id}; !slices.Equal(got, want) {
		t.Errorf("drafts without read:direct = %v, want %v", got, want)
	}
	if got, want := list(wide), []string{toPublish.Id, public.Id, direct.Id}; !slices.Equal(got, want) {
		t.Errorf("drafts with read:direct = %v, want %v", got, want)
	}

	path := "/drafts/" + direct.Id
	var current clientapi.Draft
	res := e.send(wide, http.MethodGet, path, nil, nil, &current)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET the direct draft with read:direct = %s", e.outcome(res))
	}
	tag := map[string]string{"If-Match": res.Header.Get("ETag")}
	for _, cmd := range []struct {
		method, path string
		body         any
		header       map[string]string
	}{
		{http.MethodGet, path, nil, nil},
		{http.MethodPut, path, map[string]any{"text": "public now", "visibility": "public"}, tag},
		{http.MethodPut, path + "/schedule", map[string]any{"publish_at": e.clock.Now().Add(24 * time.Hour).Format(time.RFC3339)}, nil},
		{http.MethodDelete, path + "/schedule", nil, nil},
		{http.MethodDelete, path, nil, nil},
	} {
		if got := e.outcome(e.send(narrow, cmd.method, cmd.path, cmd.body, cmd.header, nil)); got[:3] != "404" {
			t.Errorf("%s %s of a direct draft without read:direct = %s, want 404", cmd.method, cmd.path, got)
		}
	}
	var after clientapi.Draft
	if res := e.send(wide, http.MethodGet, path, nil, nil, &after); res.StatusCode != http.StatusOK || after.Text != current.Text || after.ScheduledAt != nil {
		t.Errorf("the direct draft after the refused commands = %s %+v", e.outcome(res), after)
	}

	var st clientapi.Status
	if res := e.send(narrow, http.MethodPost, "/drafts/"+toPublish.Id+"/publish", nil, nil, &st); res.StatusCode != http.StatusCreated ||
		st.Visibility != clientapi.VisibilityDirect {
		t.Errorf("publish a direct draft without read:direct = %s %+v, want 201 and the post", e.outcome(res), st)
	}
}
