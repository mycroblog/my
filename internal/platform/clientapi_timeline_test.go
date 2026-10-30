// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// timelineAPI drives the timelines of the client API on the real database.
type timelineAPI struct {
	app  *platform.App
	h    http.Handler
	spec *apitest.Spec
}

func newTimelineAPI(t *testing.T) *timelineAPI {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &timelineAPI{app: app, h: app.Handler(), spec: apitest.Load(t)}
}

// timelineUser is a user who can sign in, with an access token of the read scope.
type timelineUser struct {
	userID, accountID, token string
}

func (e *timelineAPI) user(t *testing.T, name string) timelineUser {
	t.Helper()
	return e.userWithScope(t, name, "read")
}

// userWithScope is user with an access token of scope.
func (e *timelineAPI) userWithScope(t *testing.T, name, scope string) timelineUser {
	t.Helper()
	ctx := t.Context()
	reg, err := e.app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: name, Email: name + "@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin(%s): %v", name, err)
	}
	verifier := strings.Repeat("v", 43)
	redirect := webOrigin + platform.WebClientCallbackPath
	a, err := e.app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
		ResponseType: "code", ClientID: identity.DefaultWebClientID, RedirectURI: redirect, Scope: scope,
		CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
	})
	if err != nil {
		t.Fatalf("ValidateAuthorization: %v", err)
	}
	code, err := e.app.Identity.IssueAuthorizationCode(ctx, a, reg.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := e.app.Identity.ExchangeAuthorizationCode(ctx, identity.CodeExchange{
		ClientCredentials: identity.ClientCredentials{ClientID: identity.DefaultWebClientID},
		Code:              code, RedirectURI: redirect, CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	return timelineUser{userID: reg.User.ID, accountID: reg.AccountID, token: tokens.AccessToken}
}

func (e *timelineAPI) post(t *testing.T, author timelineUser, text string, v domcontent.Visibility, language string) string {
	t.Helper()
	p, err := e.app.Content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author.accountID, TextInput: appcontent.TextInput{Text: text, Language: language}, Visibility: v,
	})
	if err != nil {
		t.Fatalf("Create(%q): %v", text, err)
	}
	return p.Status.ID
}

func (e *timelineAPI) boost(t *testing.T, booster timelineUser, statusID string) string {
	t.Helper()
	b, err := e.app.Interactions.Boost(t.Context(), booster.accountID, statusID)
	if err != nil {
		t.Fatalf("Boost: %v", err)
	}
	return b.ID
}

func (e *timelineAPI) follow(t *testing.T, follower, followee timelineUser) {
	t.Helper()
	if _, err := e.app.Graph.Follow(t.Context(), follower.accountID, followee.accountID); err != nil {
		t.Fatalf("Follow: %v", err)
	}
}

// page gets one page of path as viewer (nil for a visitor).
func (e *timelineAPI) page(t *testing.T, viewer *timelineUser, path string) clientapi.StatusPage {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+path, nil)
	if viewer != nil {
		req.Header.Set("Authorization", "Bearer "+viewer.token)
	}
	res := e.spec.Serve(t, e.h, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", path, res.StatusCode)
	}
	var p clientapi.StatusPage
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func itemIDs(p clientapi.StatusPage) []string {
	var out []string
	for _, s := range p.Items {
		out = append(out, s.Id)
	}
	return out
}

// all follows next_cursor from the first page of path with limit, calling between after each
// page, and returns the IDs of every page.
func (e *timelineAPI) all(t *testing.T, viewer *timelineUser, path string, limit int, between func()) [][]string {
	t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var pages [][]string
	cursor := ""
	for range 20 {
		q := url.Values{"limit": {strconv.Itoa(limit)}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		p := e.page(t, viewer, path+sep+q.Encode())
		pages = append(pages, itemIDs(p))
		if between != nil {
			between()
		}
		if p.NextCursor == nil {
			return pages
		}
		cursor = *p.NextCursor
	}
	t.Fatalf("%s: more than 20 pages", path)
	return nil
}

func reversed(s []string) []string {
	out := slices.Clone(s)
	slices.Reverse(out)
	return out
}

// TestTimelinePagesHaveNoDuplicatesOrGaps pages through the timelines with keyset cursors:
// every entry appears exactly once, in order, the last page has no next_cursor, and posts that
// arrive while a client pages do not shift the pages it has yet to read.
func TestTimelinePagesHaveNoDuplicatesOrGaps(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.follow(t, bob, alice)
	var home, posts []string
	for i := range 7 {
		id := e.post(t, alice, "post "+strconv.Itoa(i), domcontent.VisibilityPublic, "en")
		home, posts = append(home, id), append(posts, id)
		if i == 2 || i == 5 {
			home = append(home, e.boost(t, bob, id))
		}
	}
	// 9 entries in pages of 3: exactly three pages, the last without next_cursor.
	arrived := 0
	pages := e.all(t, &bob, "/timelines/home", 3, func() {
		arrived++
		e.post(t, alice, "late "+strconv.Itoa(arrived), domcontent.VisibilityPublic, "en")
	})
	if len(pages) != 3 {
		t.Errorf("home pages = %v, want 3 pages of 3", pages)
	}
	if got := slices.Concat(pages...); !slices.Equal(got, reversed(home)) {
		t.Errorf("home = %v, want %v", got, reversed(home))
	}

	// Pages of a size that does not divide the timeline, for the local and profile timelines.
	want := reversed(posts)
	for _, path := range []string{"/timelines/local", "/accounts/" + alice.accountID + "/statuses?exclude_replies=true"} {
		pages := e.all(t, nil, path, 2, nil)
		got := slices.Concat(pages...)
		// The late posts are newer than the first page, so they come before the posts.
		if len(got) < len(want) || !slices.Equal(got[len(got)-len(want):], want) {
			t.Errorf("%s = %v, want it to end with %v", path, got, want)
		}
		if len(got) != len(slices.Compact(slices.Sorted(slices.Values(got)))) {
			t.Errorf("%s has duplicates: %v", path, got)
		}
	}
}

// TestTimelinesKeepLocalPostsFromVisitors checks ADR-0017 on the public timelines: a visitor who
// is not logged in never sees a local post, and a logged-in local user does.
func TestTimelinesKeepLocalPostsFromVisitors(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	pub := e.post(t, alice, "public #cats", domcontent.VisibilityPublic, "en")
	loc := e.post(t, alice, "local #cats", domcontent.VisibilityLocal, "en")
	e.post(t, alice, "unlisted #cats", domcontent.VisibilityUnlisted, "en")
	e.post(t, alice, "private #cats", domcontent.VisibilityPrivate, "en")
	for _, path := range []string{"/timelines/local", "/timelines/tag/cats"} {
		if got := itemIDs(e.page(t, nil, path)); !slices.Equal(got, []string{pub}) {
			t.Errorf("visitor %s = %v, want only the public post %s", path, got, pub)
		}
		if got := itemIDs(e.page(t, &bob, path)); !slices.Equal(got, []string{loc, pub}) {
			t.Errorf("bob %s = %v, want the local and public posts", path, got)
		}
	}
	page := e.page(t, nil, "/accounts/"+alice.accountID+"/statuses")
	for _, s := range page.Items {
		if s.Visibility == clientapi.VisibilityLocal || s.Visibility == clientapi.VisibilityPrivate {
			t.Errorf("visitor profile shows %s post %s", s.Visibility, s.Id)
		}
	}
}

// TestTimelinesRenderBoostsAndViewerState checks that a boost carries the boosted post, and that
// timelines apply the viewer's languages and show what the viewer did with each post.
func TestTimelinesRenderBoostsAndViewerState(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.follow(t, bob, carol)
	ja := e.post(t, alice, "こんにちは #挨拶", domcontent.VisibilityPublic, "ja")
	en := e.post(t, alice, "hello", domcontent.VisibilityPublic, "en")
	boostJa, boostEn := e.boost(t, carol, ja), e.boost(t, carol, en)
	if _, err := e.app.Interactions.Favourite(t.Context(), bob.accountID, ja); err != nil {
		t.Fatalf("Favourite: %v", err)
	}

	page := e.page(t, &bob, "/timelines/home")
	if got := itemIDs(page); !slices.Equal(got, []string{boostEn, boostJa}) {
		t.Fatalf("bob's home = %v, want carol's boosts", got)
	}
	b := page.Items[1]
	if b.Account.Username != "carol" || b.Boost == nil || b.Boost.Id != ja || b.Boost.Account.Username != "alice" ||
		!strings.Contains(b.Boost.Content, "こんにちは") || b.Boost.BoostsCount != 1 || b.Boost.FavouritesCount != 1 {
		t.Errorf("boost = %+v, want carol's boost of alice's post", b)
	}
	if inner := b.Boost; inner.Favourited == nil || !*inner.Favourited || inner.Boosted == nil || *inner.Boosted ||
		len(inner.Tags) != 1 || inner.Tags[0].Name != "挨拶" {
		t.Errorf("boosted post = %+v, want bob's favourite and the hashtag", inner)
	}

	// The viewer's timeline languages filter the home timeline.
	if _, err := e.app.Identity.SetTimelineLanguages(t.Context(), bob.userID, []string{"ja"}); err != nil {
		t.Fatalf("SetTimelineLanguages: %v", err)
	}
	if got := itemIDs(e.page(t, &bob, "/timelines/home")); !slices.Equal(got, []string{boostJa}) {
		t.Errorf("bob's home in ja = %v, want only the boost of the ja post", got)
	}
	// A profile shows every language.
	if got := itemIDs(e.page(t, &bob, "/accounts/"+alice.accountID+"/statuses")); !slices.Equal(got, []string{en, ja}) {
		t.Errorf("alice's profile for bob = %v, want both posts", got)
	}
	if got := itemIDs(e.page(t, nil, "/accounts/"+carol.accountID+"/statuses?exclude_boosts=true")); len(got) != 0 {
		t.Errorf("carol's profile without boosts = %v, want none", got)
	}
}
