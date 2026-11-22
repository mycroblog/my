// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestDraftsAndInteractionRequestsAreNotCached checks that the answers about the user's drafts and
// interaction requests, which are the user's alone, tell every cache not to keep them.
func TestDraftsAndInteractionRequestsAreNotCached(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice")
	var d clientapi.Draft
	res := e.send("alice", http.MethodPost, "/drafts", map[string]any{"text": "later", "visibility": "public"}, nil, &d)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /drafts = %s", e.outcome(res))
	}
	answers := map[string]*http.Response{"POST /drafts": res}
	for _, path := range []string{"/drafts", "/drafts/" + d.Id, "/interaction_requests"} {
		answers["GET "+path] = e.send("alice", http.MethodGet, path, nil, nil, nil)
	}
	for what, res := range answers {
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			t.Errorf("%s = %s", what, e.outcome(res))
			continue
		}
		if got := res.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", what, got)
		}
	}
}
