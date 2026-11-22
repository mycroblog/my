// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// favourite makes user favourite each status of ids, in order.
func (c *statusesClient) favourite(user string, ids ...string) {
	c.t.Helper()
	for _, id := range ids {
		if res := c.do(user, http.MethodPost, "/statuses/"+id+"/favourite", nil, nil); res.StatusCode != http.StatusOK {
			c.t.Fatalf("favourite %s as %s = %d", id, user, res.StatusCode)
		}
	}
}

// member registers name as a user without a role and signs them in.
func (c *statusesClient) member(name string) {
	c.t.Helper()
	r, err := c.app.Identity.RegisterLocalUser(c.t.Context(), identity.RegisterInput{
		Username: name, Password: "correct horse battery staple", DisplayName: name,
	})
	if err != nil {
		c.t.Fatalf("RegisterLocalUser(%s): %v", name, err)
	}
	c.tokens[name] = issueTokens(c.t, c.app, r.User.ID, "").AccessToken
	c.ids[name], c.userIDs[name] = r.AccountID, r.User.ID
}

// outcome is the status of res and, for an error, what its problem says, without the request ID
// that differs per request.
func (c *statusesClient) outcome(res *http.Response) string {
	c.t.Helper()
	if res.StatusCode < 400 {
		return http.StatusText(res.StatusCode)
	}
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		c.t.Fatal(err)
	}
	deref := func(s *string) string {
		if s == nil {
			return "<nil>"
		}
		return *s
	}
	return fmt.Sprintf("%d type=%s title=%q detail=%q field=%q cache=%q",
		res.StatusCode, p.Type, p.Title, deref(p.Detail), deref(p.Field), res.Header.Get("Cache-Control"))
}

// TestFavouritesThroughTheAPI lists the reader's favourites (FR-107): newest favourite first
// whatever the age of the post, a page at a time with the parameters and problems of the
// bookmarks; unfavouriting takes a post out, and so does losing sight of it (deleted, hidden by a
// block, followers-only once unfollowed, its author suspended) while its favourite keeps its place
// in the pages; no keyword filter applies.
func TestFavouritesThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	ctx := t.Context()

	// Favourited out of the order of the posts: the list follows the favourites.
	first := c.post("bob", map[string]any{"text": "first", "visibility": "public"}).Id
	second := c.post("bob", map[string]any{"text": "second", "visibility": "public"}).Id
	third := c.post("carol", map[string]any{"text": "third", "visibility": "public"}).Id
	c.favourite("alice", second, third, first)
	want := []string{first, third, second}
	if got := statusIDs(c.statusPage("alice", "/favourites")); !slices.Equal(got, want) {
		t.Errorf("favourites = %v, want newest favourite first %v", got, want)
	}
	if got := statusIDs(c.statusPage("bob", "/favourites")); len(got) != 0 {
		t.Errorf("bob's favourites = %v, want none: alice's are hers", got)
	}

	// A page at a time; the exact-size last page has no next page.
	var paged []string
	path := "/favourites?limit=1"
	for range len(want) + 1 {
		page := c.statusPage("alice", path)
		if len(page.Items) != 1 {
			t.Fatalf("GET %s = %v, want one post", path, statusIDs(page))
		}
		paged = append(paged, statusIDs(page)...)
		if page.NextCursor == nil {
			break
		}
		path = "/favourites?limit=1&cursor=" + *page.NextCursor
	}
	if !slices.Equal(paged, want) {
		t.Errorf("favourites page by page = %v, want %v", paged, want)
	}
	if whole := c.statusPage("alice", "/favourites?limit=3"); len(whole.Items) != 3 || whole.NextCursor != nil {
		t.Errorf("favourites?limit=3 = %v, next %v; want all three and no next page", statusIDs(whole), whole.NextCursor)
	}

	// The parameters answer as those of the bookmarks.
	for _, query := range []string{"?limit=0", "?limit=-1", "?limit=41", "?limit=1000000", "?limit=x", "?cursor=not-an-id"} {
		got := c.outcome(c.do("alice", http.MethodGet, "/favourites"+query, nil, nil))
		if want := c.outcome(c.do("alice", http.MethodGet, "/bookmarks"+query, nil, nil)); got != want {
			t.Errorf("favourites%s = %s, want the answer of the bookmarks %s", query, got, want)
		}
	}
	if res := c.do("alice", http.MethodGet, "/favourites?limit=41", nil, nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("favourites?limit=41 = %d, want 400", res.StatusCode)
	}

	// Unfavouriting takes the post out.
	if res := c.do("alice", http.MethodDelete, "/statuses/"+third+"/favourite", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("unfavourite = %d", res.StatusCode)
	}
	want = []string{first, second}
	if got := statusIDs(c.statusPage("alice", "/favourites")); !slices.Equal(got, want) {
		t.Errorf("favourites after unfavouriting = %v, want %v", got, want)
	}

	// No keyword filter applies: neither hidden nor marked.
	for _, filter := range []map[string]any{
		{"title": "Ads", "action": "hide", "contexts": []string{"home", "public", "thread", "notifications"}, "keywords": []map[string]any{{"keyword": "SALE", "whole_word": false}}},
		{"title": "Spoilers", "action": "warn", "contexts": []string{"home", "public", "thread", "notifications"}, "keywords": []map[string]any{{"keyword": "spoiler", "whole_word": false}}},
	} {
		if res := c.do("alice", http.MethodPost, "/filters", filter, nil); res.StatusCode != http.StatusCreated {
			t.Fatalf("POST /filters %v = %d", filter, res.StatusCode)
		}
	}
	sale := c.post("bob", map[string]any{"text": "everything on sale", "visibility": "public"}).Id
	spoiler := c.post("bob", map[string]any{"text": "a big spoiler", "visibility": "public"}).Id
	c.favourite("alice", sale, spoiler)
	want = append([]string{spoiler, sale}, want...)
	page := c.statusPage("alice", "/favourites")
	if got := statusIDs(page); !slices.Equal(got, want) {
		t.Errorf("favourites with keyword filters = %v, want %v", got, want)
	}
	for _, st := range page.Items {
		if len(st.Filtered) != 0 {
			t.Errorf("favourite %s carries the filters %v, want none", st.Id, filteredBy(st))
		}
	}

	// Posts the reader can no longer see are left out.
	c.member("dave")
	if _, err := c.app.Graph.Follow(ctx, c.ids["alice"], c.ids["bob"]); err != nil {
		t.Fatal(err)
	}
	deleted := c.post("carol", map[string]any{"text": "deleted later", "visibility": "public"}).Id
	blocked := c.post("carol", map[string]any{"text": "hidden by a block", "visibility": "public"}).Id
	private := c.post("bob", map[string]any{"text": "for followers", "visibility": "private"}).Id
	suspended := c.post("dave", map[string]any{"text": "suspended later", "visibility": "public"}).Id
	c.favourite("alice", deleted, private, suspended, blocked)
	if got, all := statusIDs(c.statusPage("alice", "/favourites")), append([]string{blocked, suspended, private, deleted}, want...); !slices.Equal(got, all) {
		t.Fatalf("favourites while every post is in sight = %v, want %v", got, all)
	}
	if res := c.do("carol", http.MethodDelete, "/statuses/"+deleted, nil, nil); res.StatusCode/100 != 2 {
		t.Fatalf("DELETE status = %d", res.StatusCode)
	}
	if _, err := c.app.Graph.Block(ctx, c.ids["carol"], c.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	if _, err := c.app.Graph.Unfollow(ctx, c.ids["alice"], c.ids["bob"]); err != nil {
		t.Fatal(err)
	}
	if res := c.do("alice", http.MethodPost, "/admin/accounts/"+c.ids["dave"]+"/actions", map[string]any{"kind": "suspend"}, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("suspend dave = %d", res.StatusCode)
	}
	if got := statusIDs(c.statusPage("alice", "/favourites")); !slices.Equal(got, want) {
		t.Errorf("favourites once the posts are out of sight = %v, want %v", got, want)
	}
	// The newest favourite is of a post out of sight: its page is empty and still goes on.
	if page := c.statusPage("alice", "/favourites?limit=1"); len(page.Items) != 0 || page.NextCursor == nil {
		t.Errorf("favourites?limit=1 = %v, next %v; want an empty page with a next page", statusIDs(page), page.NextCursor)
	}
	var visible []string
	path = "/favourites?limit=2"
	for range 10 {
		page := c.statusPage("alice", path)
		visible = append(visible, statusIDs(page)...)
		if page.NextCursor == nil {
			break
		}
		path = "/favourites?limit=2&cursor=" + *page.NextCursor
	}
	if !slices.Equal(visible, want) {
		t.Errorf("favourites page by page once the posts are out of sight = %v, want %v", visible, want)
	}
}

// TestFavouritesNeedTheirScope reads the favourites with the tokens of a third-party app:
// read:favourites, which read grants, lets a token list them, a token without it is refused as
// one without read:bookmarks is refused the bookmarks, and direct posts need read:direct.
func TestFavouritesNeedTheirScope(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	narrow := c.appToken("alice", "read:statuses read:favourites")
	wide := c.appToken("alice", "read:statuses read:favourites read:direct")
	read := c.appToken("alice", "read")
	other := c.appToken("alice", "read:statuses read:bookmarks")
	write := c.appToken("alice", "write")

	public := c.post("alice", map[string]any{"text": "public", "visibility": "public"}).Id
	own := c.post("alice", map[string]any{"text": "mine @bob", "visibility": "direct"}).Id
	received := c.post("bob", map[string]any{"text": "secret @alice", "visibility": "direct"}).Id
	c.favourite("alice", public, own, received)

	if got, want := statusIDs(c.statusPage(narrow, "/favourites")), []string{public}; !slices.Equal(got, want) {
		t.Errorf("favourites without read:direct = %v, want %v", got, want)
	}
	all := []string{received, own, public}
	if got := statusIDs(c.statusPage(wide, "/favourites")); !slices.Equal(got, all) {
		t.Errorf("favourites with read:direct = %v, want %v", got, all)
	}
	if got := statusIDs(c.statusPage(read, "/favourites")); !slices.Equal(got, all) {
		t.Errorf("favourites with read = %v, want %v", got, all)
	}

	// Without read:favourites, as without read:bookmarks for the bookmarks.
	for _, token := range []string{other, write} {
		res := c.do(token, http.MethodGet, "/favourites", nil, nil)
		challenge := res.Header.Get("WWW-Authenticate")
		got := c.outcome(res)
		bookmarks := c.do(narrow, http.MethodGet, "/bookmarks", nil, nil)
		wantChallenge := strings.Replace(bookmarks.Header.Get("WWW-Authenticate"), "read:bookmarks", "read:favourites", 1)
		want := strings.ReplaceAll(c.outcome(bookmarks), "read:bookmarks", "read:favourites")
		if res.StatusCode != http.StatusForbidden || got != want {
			t.Errorf("favourites as %q = %s, want the refusal of the bookmarks %s", token, got, want)
		}
		if challenge != wantChallenge || !strings.Contains(challenge, `scope="read:favourites"`) {
			t.Errorf("WWW-Authenticate of the favourites as %q = %q, want %q", token, challenge, wantChallenge)
		}
		if !strings.Contains(got, clientapi.ProblemTypeInsufficientScope) {
			t.Errorf("favourites as %q = %s, want %s", token, got, clientapi.ProblemTypeInsufficientScope)
		}
	}
}

// TestFavouritesLeaveOutBoardPosts favourites a post of an event's board, which its readers may
// favourite: it stays out of the list of favourites, as it stays out of every list but its board.
func TestFavouritesLeaveOutBoardPosts(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	ev := e.createEvent("readers", nil)
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	ordinary := e.post("alice", map[string]any{"text": "an ordinary post", "visibility": "public"}).Id
	board := e.mustPostOnBoard("alice", ev.Id, map[string]any{"text": "on the board"}).Id
	e.favourite("gina", ordinary, board)
	e.favourite("alice", board, ordinary)
	for _, user := range []string{"alice", "gina"} {
		if got, want := statusIDs(e.statusPage(user, "/favourites")), []string{ordinary}; !slices.Equal(got, want) {
			t.Errorf("favourites of %s = %v, want %v without the board post", user, got, want)
		}
	}
}
