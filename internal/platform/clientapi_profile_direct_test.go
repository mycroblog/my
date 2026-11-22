// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"

	domcontent "github.com/mycroblog/my/internal/domain/content"
)

// TestProfileListsNoDirectPost checks GET /accounts/{id}/statuses on the real database: a direct
// post belongs to its conversation, so no profile lists it, neither to its author nor to the
// person it is addressed to, while both still read it on its own page.
func TestProfileListsNoDirectPost(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.follow(t, bob, alice)
	public := e.post(t, alice, "hello everyone", domcontent.VisibilityPublic, "en")
	direct := e.post(t, alice, "@bob just for you", domcontent.VisibilityDirect, "en")
	followers := e.post(t, alice, "for my followers", domcontent.VisibilityPrivate, "en")

	path := "/accounts/" + alice.accountID + "/statuses"
	if got, want := slices.Concat(e.all(t, &alice, path, 1, nil)...), []string{followers, public}; !slices.Equal(got, want) {
		t.Errorf("the author's own profile = %v, want %v (no direct post)", got, want)
	}
	if got, want := slices.Concat(e.all(t, &bob, path, 1, nil)...), []string{followers, public}; !slices.Equal(got, want) {
		t.Errorf("the profile to the person addressed = %v, want %v (no direct post)", got, want)
	}
	for name, u := range map[string]timelineUser{"the author": alice, "the person addressed": bob} {
		if res := e.listCall(t, u, http.MethodGet, "/statuses/"+direct, ""); res.StatusCode != http.StatusOK {
			t.Errorf("%s reads the direct post on its own page: status %d, want 200", name, res.StatusCode)
		}
	}
}
