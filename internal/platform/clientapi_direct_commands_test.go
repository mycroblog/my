// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestAuthorCommandsOnDirectPostsNeedReadDirect checks with third-party tokens that write:statuses
// sends a direct post and sees it in the answer, and that it does nothing else to one: editing it,
// setting who may quote it, and deleting it answer 404 and change nothing, the author's own post
// included. Another account's direct post answers 404 too rather than the 403 that shows it exists.
func TestAuthorCommandsOnDirectPostsNeedReadDirect(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	narrow := c.appToken("alice", "write:statuses")
	wide := c.appToken("alice", "write:statuses read:direct")

	var sent clientapi.Status
	if res := c.do(narrow, http.MethodPost, "/statuses", map[string]any{"text": "sent @bob", "visibility": "direct"}, &sent); res.StatusCode != http.StatusCreated ||
		sent.Visibility != clientapi.VisibilityDirect {
		t.Fatalf("POST a direct post = %d %+v, want 201 and the post", res.StatusCode, sent)
	}
	received := c.post("bob", map[string]any{"text": "secret @alice", "visibility": "direct"})
	public := c.post("alice", map[string]any{"text": "public", "visibility": "public"})

	commands := []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "", map[string]any{"text": "edited"}},
		{http.MethodPut, "/quote_policy", map[string]any{"quote_policy": "nobody"}},
		{http.MethodDelete, "", nil},
		{http.MethodDelete, "/quotes/" + public.Id + "/approval", nil},
	}
	for _, id := range []string{sent.Id, received.Id} {
		for _, cmd := range commands {
			if res := c.do(narrow, cmd.method, "/statuses/"+id+cmd.path, cmd.body, nil); res.StatusCode != http.StatusNotFound {
				t.Errorf("%s %s%s without read:direct = %d, want 404", cmd.method, id, cmd.path, res.StatusCode)
			}
		}
	}
	var st clientapi.Status
	if res := c.do("alice", http.MethodGet, "/statuses/"+sent.Id, nil, &st); res.StatusCode != http.StatusOK || st.Content != sent.Content || st.EditedAt != nil {
		t.Errorf("the post after the refused commands = %d %+v", res.StatusCode, st)
	}

	// With read:direct the author's commands work, and another account's post is a 403.
	if res := c.do(wide, http.MethodPut, "/statuses/"+received.Id, map[string]any{"text": "edited"}, nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("PUT another account's direct post with read:direct = %d, want 403", res.StatusCode)
	}
	if res := c.do(wide, http.MethodPut, "/statuses/"+sent.Id, map[string]any{"text": "edited"}, nil); res.StatusCode != http.StatusOK {
		t.Errorf("PUT the own direct post with read:direct = %d, want 200", res.StatusCode)
	}
	if res := c.do(wide, http.MethodPut, "/statuses/"+sent.Id+"/quote_policy", map[string]any{"quote_policy": "nobody"}, nil); res.StatusCode != http.StatusOK {
		t.Errorf("PUT the quote policy of the own direct post with read:direct = %d, want 200", res.StatusCode)
	}
	if res := c.do(wide, http.MethodDelete, "/statuses/"+sent.Id, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE the own direct post with read:direct = %d, want 204", res.StatusCode)
	}

	// Without read:direct the same commands still work on a post that is not direct.
	if res := c.do(narrow, http.MethodPut, "/statuses/"+public.Id, map[string]any{"text": "edited"}, nil); res.StatusCode != http.StatusOK {
		t.Errorf("PUT the own public post = %d, want 200", res.StatusCode)
	}
	if res := c.do(narrow, http.MethodPut, "/statuses/"+public.Id+"/quote_policy", map[string]any{"quote_policy": "nobody"}, nil); res.StatusCode != http.StatusOK {
		t.Errorf("PUT the quote policy of the own public post = %d, want 200", res.StatusCode)
	}
	if res := c.do(narrow, http.MethodDelete, "/statuses/"+public.Id, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE the own public post = %d, want 204", res.StatusCode)
	}
}
