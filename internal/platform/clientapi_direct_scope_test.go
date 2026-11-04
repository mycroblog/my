// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// appToken registers a third-party app of user and returns the name under which c sends requests
// with a token of that app holding scope.
func (c *statusesClient) appToken(user, scope string) string {
	c.t.Helper()
	ctx := c.t.Context()
	userID := c.userIDs[user]
	const redirect = "https://app.example/callback"
	reg, err := c.app.Identity.RegisterOAuthApp(ctx, userID, identity.OAuthAppInput{
		Name: "App", RedirectURIs: []string{redirect}, Scope: scope,
	})
	if err != nil {
		c.t.Fatalf("RegisterOAuthApp(%q): %v", scope, err)
	}
	verifier := strings.Repeat("v", 43)
	a, err := c.app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
		ResponseType: "code", ClientID: reg.Client.ClientID, RedirectURI: redirect, Scope: scope,
		CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
	})
	if err != nil {
		c.t.Fatalf("ValidateAuthorization(%q): %v", scope, err)
	}
	code, err := c.app.Identity.IssueAuthorizationCode(ctx, a, userID)
	if err != nil {
		c.t.Fatal(err)
	}
	tokens, err := c.app.Identity.ExchangeAuthorizationCode(ctx, identity.CodeExchange{
		ClientCredentials: identity.ClientCredentials{ClientID: reg.Client.ClientID},
		Code:              code, RedirectURI: redirect, CodeVerifier: verifier,
	})
	if err != nil {
		c.t.Fatalf("ExchangeAuthorizationCode(%q): %v", scope, err)
	}
	name := user + " with " + scope
	c.tokens[name], c.ids[name] = tokens.AccessToken, c.ids[user]
	return name
}

func (c *statusesClient) statusPage(user, path string) clientapi.StatusPage {
	c.t.Helper()
	var page clientapi.StatusPage
	if res := c.do(user, http.MethodGet, path, nil, &page); res.StatusCode != http.StatusOK {
		c.t.Fatalf("GET %s as %s = %d", path, user, res.StatusCode)
	}
	return page
}

func statusIDs(page clientapi.StatusPage) []string {
	ids := make([]string, 0, len(page.Items))
	for _, st := range page.Items {
		ids = append(ids, st.Id)
	}
	return ids
}

// TestDirectPostsNeedReadDirect reads direct posts through the lists that build their own viewer,
// with the tokens of a third-party app.
func TestDirectPostsNeedReadDirect(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	narrow := c.appToken("alice", "read:statuses read:bookmarks")
	wide := c.appToken("alice", "read:statuses read:bookmarks read:direct")

	public := c.post("alice", map[string]any{"text": "public", "visibility": "public"})
	own := c.post("alice", map[string]any{"text": "mine @bob", "visibility": "direct"})
	received := c.post("bob", map[string]any{"text": "secret @alice", "visibility": "direct"})
	publicQuote := c.post("alice", map[string]any{"text": "public quote", "visibility": "public", "quoted_status_id": public.Id})
	directQuote := c.post("alice", map[string]any{"text": "direct quote @bob", "visibility": "direct", "quoted_status_id": public.Id})
	for _, id := range []string{public.Id, own.Id, received.Id} {
		if res := c.do("alice", http.MethodPost, "/statuses/"+id+"/bookmark", nil, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("bookmark %s = %d", id, res.StatusCode)
		}
	}

	if got, want := statusIDs(c.statusPage(narrow, "/bookmarks")), []string{public.Id}; !slices.Equal(got, want) {
		t.Errorf("bookmarks without read:direct = %v, want %v", got, want)
	}
	if got, want := statusIDs(c.statusPage(wide, "/bookmarks")), []string{received.Id, own.Id, public.Id}; !slices.Equal(got, want) {
		t.Errorf("bookmarks with read:direct = %v, want %v", got, want)
	}

	if res := c.do(narrow, http.MethodGet, "/statuses/"+own.Id+"/source", nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("source of the own direct post without read:direct = %d, want 404", res.StatusCode)
	}
	if res := c.do(narrow, http.MethodGet, "/statuses/"+public.Id+"/source", nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("source of a public post = %d, want 200", res.StatusCode)
	}
	if res := c.do(wide, http.MethodGet, "/statuses/"+own.Id+"/source", nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("source of the own direct post with read:direct = %d, want 200", res.StatusCode)
	}

	if got, want := statusIDs(c.statusPage(narrow, "/statuses/"+public.Id+"/quotes")), []string{publicQuote.Id}; !slices.Equal(got, want) {
		t.Errorf("quotes without read:direct = %v, want %v", got, want)
	}
	if got, want := statusIDs(c.statusPage(wide, "/statuses/"+public.Id+"/quotes")), []string{directQuote.Id, publicQuote.Id}; !slices.Equal(got, want) {
		t.Errorf("quotes with read:direct = %v, want %v", got, want)
	}
	if res := c.do(narrow, http.MethodGet, "/statuses/"+own.Id+"/quotes", nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("quotes of the own direct post without read:direct = %d, want 404", res.StatusCode)
	}
}

// TestActionsOnStatusesTheTokenCannotSee checks that an action on a post that the token does not
// see, or that is gone, answers 404 and changes nothing; that an un-action changes nothing on a
// post that is gone but takes effect on one that is hidden, answering the same 404; and that a
// write scope reads a post it may act on.
func TestActionsOnStatusesTheTokenCannotSee(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	ctx := t.Context()
	favourites := c.appToken("alice", "write:favourites")
	bookmarks := c.appToken("alice", "write:bookmarks")
	statuses := c.appToken("alice", "write:statuses")

	get := func(id string) clientapi.Status {
		t.Helper()
		var st clientapi.Status
		if res := c.do("alice", http.MethodGet, "/statuses/"+id, nil, &st); res.StatusCode != http.StatusOK {
			t.Fatalf("GET status = %d", res.StatusCode)
		}
		return st
	}
	isTrue := func(b *bool) bool { return b != nil && *b }
	reactions := func(id string) int {
		t.Helper()
		var page clientapi.StatusReactionPage
		if res := c.do("alice", http.MethodGet, "/statuses/"+id+"/reactions", nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET reactions = %d", res.StatusCode)
		}
		// The counts on the status come from a projection of the same transaction.
		counted := 0
		for _, r := range get(id).Reactions {
			counted += r.Count
		}
		if counted != len(page.Items) {
			t.Errorf("status %s: Status.reactions counts %d, the list of reactors has %d", id, counted, len(page.Items))
		}
		return len(page.Items)
	}
	wantNotFound := func(user, method, path string, body any) {
		t.Helper()
		if res := c.do(user, method, path, body, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s as %s = %d, want 404", method, path, user, res.StatusCode)
		}
	}

	// The actions on a direct post change nothing without read:direct, the author's own included.
	dm := c.post("bob", map[string]any{
		"text": "secret @alice", "visibility": "direct",
		"poll": map[string]any{"options": []string{"yes", "no"}, "expires_in": 3600},
	})
	own := c.post("alice", map[string]any{"text": "mine @bob", "visibility": "direct"})
	for _, id := range []string{dm.Id, own.Id} {
		wantNotFound(favourites, http.MethodPost, "/statuses/"+id+"/favourite", nil)
		wantNotFound(favourites, http.MethodPut, "/statuses/"+id+"/reaction", map[string]any{"emoji": "🎉"})
		wantNotFound(bookmarks, http.MethodPost, "/statuses/"+id+"/bookmark", nil)
		wantNotFound(statuses, http.MethodPost, "/statuses/"+id+"/boost", nil)
		st := get(id)
		if isTrue(st.Favourited) || isTrue(st.Bookmarked) || isTrue(st.Boosted) || reactions(id) != 0 {
			t.Errorf("status %s after refused actions: %+v", id, st)
		}
	}
	wantNotFound(statuses, http.MethodPost, "/statuses/"+dm.Id+"/poll/votes", map[string]any{"poll_id": dm.Poll.Id, "choices": []int{0}})
	if st := get(dm.Id); st.Poll == nil || isTrue(st.Poll.Voted) {
		t.Errorf("poll after a refused vote: %+v", st.Poll)
	}

	// An un-action on it takes effect and answers the same 404, which shows nothing of the post.
	for _, id := range []string{dm.Id, own.Id} {
		for _, path := range []string{"/favourite", "/bookmark"} {
			if res := c.do("alice", http.MethodPost, "/statuses/"+id+path, nil, nil); res.StatusCode != http.StatusOK {
				t.Fatalf("POST %s = %d", path, res.StatusCode)
			}
		}
		wantNotFound(favourites, http.MethodDelete, "/statuses/"+id+"/favourite", nil)
		wantNotFound(bookmarks, http.MethodDelete, "/statuses/"+id+"/bookmark", nil)
		if st := get(id); isTrue(st.Favourited) || isTrue(st.Bookmarked) {
			t.Errorf("status %s after un-actions without read:direct: favourited %v, bookmarked %v", id, isTrue(st.Favourited), isTrue(st.Bookmarked))
		}
		if res := c.do("alice", http.MethodPut, "/statuses/"+id+"/reaction", map[string]any{"emoji": "🎉"}, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("PUT reaction = %d", res.StatusCode)
		}
		if n := reactions(id); n != 1 {
			t.Fatalf("reactions after reacting = %d, want 1", n)
		}
		wantNotFound(favourites, http.MethodDelete, "/statuses/"+id+"/reaction", nil)
		if n := reactions(id); n != 0 {
			t.Errorf("status %s after taking back the reaction without read:direct: %d reactions", id, n)
		}
	}

	// A write scope reads the post it acts on, followers-only content included.
	if _, err := c.app.Graph.Follow(ctx, c.ids["alice"], c.ids["bob"]); err != nil {
		t.Fatal(err)
	}
	private := c.post("bob", map[string]any{"text": "for followers", "visibility": "private"})
	var acted clientapi.Status
	if res := c.do(bookmarks, http.MethodPost, "/statuses/"+private.Id+"/bookmark", nil, &acted); res.StatusCode != http.StatusOK {
		t.Fatalf("bookmark with write:bookmarks = %d", res.StatusCode)
	}
	if acted.Id != private.Id || !strings.Contains(acted.Content, "for followers") {
		t.Errorf("response of the action = %+v, want the whole post", acted)
	}
	if res := c.do(favourites, http.MethodPost, "/statuses/"+private.Id+"/favourite", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("favourite with write:favourites = %d", res.StatusCode)
	}

	// A post hidden from the user that still exists: the un-actions take effect, and the answer
	// shows nothing of it, as following again shows.
	if res := c.do("alice", http.MethodDelete, "/statuses/"+private.Id+"/favourite", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE favourite = %d", res.StatusCode)
	}
	if res := c.do("alice", http.MethodPut, "/statuses/"+private.Id+"/reaction", map[string]any{"emoji": "🎉"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT reaction = %d", res.StatusCode)
	}
	if _, err := c.app.Graph.Unfollow(ctx, c.ids["alice"], c.ids["bob"]); err != nil {
		t.Fatal(err)
	}
	wantHidden := func(method, path string) {
		t.Helper()
		missing := problemShape(t, c.do("alice", method, "/statuses/0192f0c1-0000-7000-8000-000000000000"+path, nil, nil))
		res := c.do("alice", method, "/statuses/"+private.Id+path, nil, nil)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s %s on a hidden post = %d, want 404", method, path, res.StatusCode)
		}
		if got := problemShape(t, res); got != missing {
			t.Errorf("%s %s on a hidden post = %s, want the answer of a missing post %s", method, path, got, missing)
		}
	}
	wantHidden(http.MethodDelete, "/bookmark")
	wantHidden(http.MethodDelete, "/reaction")
	if _, err := c.app.Graph.Follow(ctx, c.ids["alice"], c.ids["bob"]); err != nil {
		t.Fatal(err)
	}
	if st := get(private.Id); isTrue(st.Bookmarked) || reactions(private.Id) != 0 {
		t.Errorf("status after un-actions on a hidden post: bookmarked %v, %d reactions", isTrue(st.Bookmarked), reactions(private.Id))
	}

	// So is a favourite and a boost, on a post hidden by a block.
	public := c.post("bob", map[string]any{"text": "public", "visibility": "public"})
	for _, path := range []string{"/favourite", "/boost"} {
		if res := c.do("alice", http.MethodPost, "/statuses/"+public.Id+path, nil, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("POST %s = %d", path, res.StatusCode)
		}
	}
	if _, err := c.app.Graph.Block(ctx, c.ids["bob"], c.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	wantNotFound("alice", http.MethodDelete, "/statuses/"+public.Id+"/favourite", nil)
	wantNotFound("alice", http.MethodDelete, "/statuses/"+public.Id+"/boost", nil)
	if _, err := c.app.Graph.Unblock(ctx, c.ids["bob"], c.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	if st := get(public.Id); isTrue(st.Favourited) || isTrue(st.Boosted) {
		t.Errorf("status after un-actions on a post hidden by a block: favourited %v, boosted %v", isTrue(st.Favourited), isTrue(st.Boosted))
	}

	// A post that is gone answers 404 for every un-action and changes nothing: the bookmark
	// of the deleted post still takes its place in the list, so the page of one is empty.
	older := c.post("bob", map[string]any{"text": "older", "visibility": "public"})
	newer := c.post("bob", map[string]any{"text": "newer", "visibility": "public"})
	for _, id := range []string{older.Id, newer.Id} {
		if res := c.do("alice", http.MethodPost, "/statuses/"+id+"/bookmark", nil, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("bookmark = %d", res.StatusCode)
		}
	}
	if res := c.do("bob", http.MethodDelete, "/statuses/"+newer.Id, nil, nil); res.StatusCode/100 != 2 {
		t.Fatalf("DELETE status = %d", res.StatusCode)
	}
	for _, path := range []string{"/favourite", "/bookmark", "/boost", "/reaction", "/pin"} {
		wantNotFound("alice", http.MethodDelete, "/statuses/"+newer.Id+path, nil)
	}
	if page := c.statusPage("alice", "/bookmarks?limit=1"); len(page.Items) != 0 || page.NextCursor == nil {
		t.Errorf("bookmarks?limit=1 after un-actions on a gone post = %v, next %v; want the deleted post's bookmark kept", statusIDs(page), page.NextCursor)
	}
}

// TestBookmarksAndQuotesPages checks the limit of the two lists and that an exact-size last page
// has no next page.
func TestBookmarksAndQuotesPages(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice")
	target := c.post("alice", map[string]any{"text": "quoted", "visibility": "public"})
	var marked, quotes []string
	for range 2 {
		st := c.post("alice", map[string]any{"text": "post", "visibility": "public"})
		if res := c.do("alice", http.MethodPost, "/statuses/"+st.Id+"/bookmark", nil, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("bookmark = %d", res.StatusCode)
		}
		marked = append(marked, st.Id)
		quotes = append(quotes, c.post("alice", map[string]any{"text": "quote", "visibility": "public", "quoted_status_id": target.Id}).Id)
	}
	for _, list := range []struct {
		path string
		ids  []string
	}{{"/bookmarks", marked}, {"/statuses/" + target.Id + "/quotes", quotes}} {
		for _, limit := range []string{"0", "-1", "41", "1000000"} {
			res := c.do("alice", http.MethodGet, list.path+"?limit="+limit, nil, nil)
			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("%s?limit=%s = %d, want 400", list.path, limit, res.StatusCode)
				continue
			}
			var p clientapi.Problem
			if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
				t.Error(err)
			}
		}
		first := c.statusPage("alice", list.path+"?limit=1")
		if got := statusIDs(first); len(got) != 1 || got[0] != list.ids[1] || first.NextCursor == nil {
			t.Fatalf("%s?limit=1 = %v, next %v; want the newest and a next page", list.path, got, first.NextCursor)
		}
		last := c.statusPage("alice", list.path+"?limit=1&cursor="+*first.NextCursor)
		if got := statusIDs(last); len(got) != 1 || got[0] != list.ids[0] || last.NextCursor != nil {
			t.Errorf("last page of %s = %v, next %v; want no next page", list.path, got, last.NextCursor)
		}
		if whole := c.statusPage("alice", list.path+"?limit=2"); len(whole.Items) != 2 || whole.NextCursor != nil {
			t.Errorf("%s?limit=2 = %d items, next %v; want no next page", list.path, len(whole.Items), whole.NextCursor)
		}
	}
}
