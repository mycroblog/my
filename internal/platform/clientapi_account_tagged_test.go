// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"slices"
	"testing"

	domcontent "github.com/mycroblog/my/internal/domain/content"
)

// TestProfileListsThePostsWithAHashtag checks GET /accounts/{id}/statuses?tagged= on the real
// database: the account's posts with the hashtag alone, newest first, with the visibility of the
// profile, and without the account's boosts or the deleted posts.
func TestProfileListsThePostsWithAHashtag(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.follow(t, bob, alice)
	first := e.post(t, alice, "first #cats", domcontent.VisibilityPublic, "en")
	e.post(t, alice, "about #dogs", domcontent.VisibilityPublic, "en")
	followers := e.post(t, alice, "for followers #Cats", domcontent.VisibilityPrivate, "en")
	deleted := e.post(t, alice, "deleted #cats", domcontent.VisibilityPublic, "en")
	if _, err := e.app.Content.Delete(t.Context(), alice.accountID, deleted); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	e.boost(t, alice, e.post(t, carol, "carol's #cats", domcontent.VisibilityPublic, "en"))
	third := e.post(t, alice, "unlisted #CATS", domcontent.VisibilityUnlisted, "en")

	path := "/accounts/" + alice.accountID + "/statuses?tagged=cats"
	if got, want := slices.Concat(e.all(t, &bob, path, 1, nil)...), []string{third, followers, first}; !slices.Equal(got, want) {
		t.Errorf("a follower's tagged posts = %v, want %v", got, want)
	}
	if got, want := slices.Concat(e.all(t, &carol, path, 2, nil)...), []string{third, first}; !slices.Equal(got, want) {
		t.Errorf("another user's tagged posts = %v, want %v (no followers-only post)", got, want)
	}
	if got, want := slices.Concat(e.all(t, nil, "/accounts/"+alice.accountID+"/statuses?tagged=%23Cats", 5, nil)...), []string{third, first}; !slices.Equal(got, want) {
		t.Errorf("a visitor's tagged posts = %v, want %v", got, want)
	}
	if page := e.page(t, &bob, "/accounts/"+alice.accountID+"/statuses?tagged=birds"); len(page.Items) != 0 || page.NextCursor != nil {
		t.Errorf("a hashtag nobody used = %+v, want an empty page", page)
	}
}
