// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// accountsEnv drives the account and relationship endpoints of the client API on the real
// database, validating every response against the spec.
type accountsEnv struct {
	t    *testing.T
	app  *platform.App
	h    http.Handler
	spec *apitest.Spec
}

type testUser struct {
	id    string
	token string
}

func newAccountsEnv(t *testing.T) *accountsEnv {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
}

func (e *accountsEnv) user(name string) testUser {
	e.t.Helper()
	r, err := e.app.Identity.CreateAdmin(e.t.Context(), identity.CreateAdminInput{
		Username: name, Email: name + "@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		e.t.Fatalf("CreateAdmin(%s): %v", name, err)
	}
	return testUser{id: r.AccountID, token: issueTokens(e.t, e.app, r.User.ID, "").AccessToken}
}

// do sends a request as token ("" for anonymous) and decodes a 2xx body into out.
func (e *accountsEnv) do(method, path, token string, body any, out any) *http.Response {
	e.t.Helper()
	var b []byte
	if body != nil {
		var err error
		if b, err = json.Marshal(body); err != nil {
			e.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(b))
	req.RemoteAddr = "192.0.2.1:1234"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := e.spec.Serve(e.t, e.h, req)
	if out != nil && res.StatusCode/100 == 2 {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			e.t.Fatal(err)
		}
	}
	return res
}

// want fails the test unless res has status, and returns the problem type of an error.
func (e *accountsEnv) want(res *http.Response, status int, what string) string {
	e.t.Helper()
	if res.StatusCode != status {
		e.t.Fatalf("%s = %d, want %d", what, res.StatusCode, status)
	}
	if status < 400 {
		return ""
	}
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		e.t.Fatal(err)
	}
	return p.Type
}

func (e *accountsEnv) relate(action string, from testUser, to testUser, body any) clientapi.Relationship {
	e.t.Helper()
	var r clientapi.Relationship
	e.want(e.do(http.MethodPost, "/accounts/"+to.id+"/"+action, from.token, body, &r), http.StatusOK, action)
	if r.Id != to.id {
		e.t.Fatalf("%s: relationship of %s, want %s", action, r.Id, to.id)
	}
	return r
}

func (e *accountsEnv) relationships(of testUser, ids ...string) []clientapi.Relationship {
	e.t.Helper()
	q := url.Values{"id": ids}
	var rs []clientapi.Relationship
	e.want(e.do(http.MethodGet, "/accounts/relationships?"+q.Encode(), of.token, nil, &rs), http.StatusOK, "relationships")
	return rs
}

func (e *accountsEnv) page(path, token string) clientapi.AccountPage {
	e.t.Helper()
	var p clientapi.AccountPage
	e.want(e.do(http.MethodGet, path, token, nil, &p), http.StatusOK, path)
	return p
}

func ids(accounts []clientapi.Account) []string {
	out := make([]string, len(accounts))
	for i, a := range accounts {
		out[i] = a.Id
	}
	return out
}

// TestClientAPIAccounts covers getting accounts, follows of open and locked accounts with
// their requests, the follow lists and their paging, and the relationships.
func TestClientAPIAccounts(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice, bob, carol := e.user("alice"), e.user("bob"), e.user("carol")
	if _, err := e.app.Accounts.ReviseProfile(t.Context(), bob.id, account.ProfileInput{
		DisplayName: "Bob", NoteHTML: "<p>locked</p>", Locked: true,
		Fields: []domainaccount.ProfileField{{Name: "Site", ValueHTML: `<a href="https://bob.example">bob.example</a>`}},
	}); err != nil {
		t.Fatal(err)
	}

	// Anonymous visitors get public, cacheable profiles with their counts.
	var got clientapi.Account
	res := e.do(http.MethodGet, "/accounts/"+bob.id, "", nil, &got)
	e.want(res, http.StatusOK, "anonymous get")
	if got.Username != "bob" || !got.Locked || got.DisplayName != "Bob" || len(got.Fields) != 1 || got.Fields[0].Name != "Site" ||
		got.FollowersCount == nil || *got.FollowersCount != 0 || got.Url != "https://example.test/@bob" {
		t.Errorf("bob = %+v", got)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("anonymous Cache-Control = %q", cc)
	}
	res = e.do(http.MethodGet, "/accounts/"+bob.id, alice.token, nil, &got)
	if e.want(res, http.StatusOK, "get as alice"); res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("viewer Cache-Control = %q", res.Header.Get("Cache-Control"))
	}
	for _, acct := range []string{"bob", "@bob", "BOB@example.test", "@bob@example.test"} {
		e.want(e.do(http.MethodGet, "/accounts/lookup?acct="+url.QueryEscape(acct), "", nil, &got), http.StatusOK, "lookup "+acct)
		if got.Id != bob.id {
			t.Errorf("lookup %s = %s, want bob", acct, got.Id)
		}
	}
	if got.Domain != "example.test" || got.Uri == "" {
		t.Errorf("bob's domain and uri = %q, %q", got.Domain, got.Uri)
	}
	var byURI clientapi.Account
	e.want(e.do(http.MethodGet, "/accounts/lookup?acct="+url.QueryEscape(got.Uri), "", nil, &byURI), http.StatusOK, "lookup by uri")
	if byURI.Id != bob.id {
		t.Errorf("lookup %s = %s, want bob", got.Uri, byURI.Id)
	}
	for _, acct := range []string{"nobody", "bob@remote.example", "bob@a@b", "@"} {
		e.want(e.do(http.MethodGet, "/accounts/lookup?acct="+url.QueryEscape(acct), "", nil, nil), http.StatusNotFound, "lookup "+acct)
	}
	// Resolving needs a token, and what leads to no remote actor is a 404: this instance's URLs
	// are never fetched, and the SSRF guard refuses loopback addresses.
	if typ := e.want(e.do(http.MethodGet, "/accounts/lookup?resolve=true&acct=bob%40remote.example", "", nil, nil), http.StatusUnauthorized, "anonymous resolve"); typ != clientapi.ProblemTypeUnauthenticated {
		t.Errorf("anonymous resolve type = %s", typ)
	}
	for _, acct := range []string{"bob@127.0.0.1", "https://example.test/users/nobody", "nobody@example.test"} {
		e.want(e.do(http.MethodGet, "/accounts/lookup?resolve=true&acct="+url.QueryEscape(acct), alice.token, nil, nil), http.StatusNotFound, "resolve "+acct)
	}
	e.want(e.do(http.MethodGet, "/accounts/01890a5d-ac96-774b-bcce-b302099a8057", "", nil, nil), http.StatusNotFound, "unknown account")
	e.want(e.do(http.MethodPost, "/accounts/01890a5d-ac96-774b-bcce-b302099a8057/follow", alice.token, nil, nil), http.StatusNotFound, "follow unknown")
	if typ := e.want(e.do(http.MethodPost, "/accounts/"+alice.id+"/follow", alice.token, nil, nil), http.StatusUnprocessableEntity, "self follow"); typ != clientapi.ProblemTypeBlank {
		t.Errorf("self follow type = %s", typ)
	}

	// Following a locked account is a request until the account accepts it.
	if r := e.relate("follow", alice, bob, nil); set(r.Following) || !set(r.Requested) {
		t.Errorf("follow locked = %+v, want requested", r)
	}
	if r := e.relate("follow", alice, bob, nil); set(r.Following) || !set(r.Requested) {
		t.Errorf("follow locked again = %+v, want still requested", r)
	}
	if p := e.page("/follow_requests", bob.token); !slices.Equal(ids(p.Accounts), []string{alice.id}) || p.NextCursor != nil {
		t.Errorf("bob's follow requests = %v", ids(p.Accounts))
	}
	if rs := e.relationships(bob, alice.id); len(rs) != 1 || !set(rs[0].RequestedBy) || set(rs[0].FollowedBy) {
		t.Errorf("bob's relationship with alice = %+v", rs)
	}
	var r clientapi.Relationship
	e.want(e.do(http.MethodPost, "/follow_requests/"+alice.id+"/authorize", bob.token, nil, &r), http.StatusOK, "authorize")
	if !set(r.FollowedBy) || set(r.RequestedBy) || r.Id != alice.id {
		t.Errorf("authorize = %+v, want followed by alice", r)
	}
	if rs := e.relationships(alice, bob.id, carol.id, bob.id); len(rs) != 2 || !set(rs[0].Following) || rs[0].Id != bob.id || set(rs[1].Following) {
		t.Errorf("alice's relationships = %+v", rs)
	}

	// Rejecting declines the request; without one it is a 404.
	if r := e.relate("follow", carol, bob, nil); !set(r.Requested) {
		t.Errorf("carol follows bob = %+v", r)
	}
	e.want(e.do(http.MethodPost, "/follow_requests/"+carol.id+"/reject", bob.token, nil, &r), http.StatusOK, "reject")
	if set(r.RequestedBy) || set(r.FollowedBy) {
		t.Errorf("reject = %+v", r)
	}
	e.want(e.do(http.MethodPost, "/follow_requests/"+carol.id+"/reject", bob.token, nil, nil), http.StatusNotFound, "reject again")

	// Open accounts are followed at once, with the preferences of the body.
	if r := e.relate("follow", alice, carol, map[string]bool{"notify": true}); !set(r.Following) || !set(r.Notify) || !set(r.ShowBoosts) {
		t.Errorf("follow open = %+v, want following with notify", r)
	}
	if r := e.relate("follow", bob, carol, nil); !set(r.Following) || set(r.Notify) {
		t.Errorf("bob follows carol = %+v", r)
	}
	e.want(e.do(http.MethodGet, "/accounts/"+carol.id, "", nil, &got), http.StatusOK, "carol")
	if *got.FollowersCount != 2 || *got.FollowingCount != 0 {
		t.Errorf("carol's counts = %d / %d, want 2 / 0", *got.FollowersCount, *got.FollowingCount)
	}

	// Keyset paging: newest follow first, next_cursor only while more follow.
	first := e.page("/accounts/"+carol.id+"/followers?limit=1", "")
	if !slices.Equal(ids(first.Accounts), []string{bob.id}) || first.NextCursor == nil {
		t.Fatalf("first page = %v, %v", ids(first.Accounts), first.NextCursor)
	}
	second := e.page("/accounts/"+carol.id+"/followers?limit=1&cursor="+url.QueryEscape(*first.NextCursor), "")
	if !slices.Equal(ids(second.Accounts), []string{alice.id}) || second.NextCursor != nil {
		t.Errorf("second page = %v, %v", ids(second.Accounts), second.NextCursor)
	}
	if p := e.page("/accounts/"+alice.id+"/following", alice.token); !slices.Equal(ids(p.Accounts), []string{carol.id, bob.id}) {
		t.Errorf("alice follows %v", ids(p.Accounts))
	}
	e.want(e.do(http.MethodGet, "/accounts/"+carol.id+"/followers?cursor=%21%21", "", nil, nil), http.StatusBadRequest, "invalid cursor")
	e.want(e.do(http.MethodGet, "/accounts/"+carol.id+"/followers?limit=41", "", nil, nil), http.StatusBadRequest, "limit too large")

	// Removing a follower and unfollowing end the follows.
	e.want(e.do(http.MethodPost, "/accounts/"+alice.id+"/remove_from_followers", bob.token, nil, &r), http.StatusOK, "remove follower")
	if set(r.FollowedBy) {
		t.Errorf("remove follower = %+v", r)
	}
	if r := e.relate("unfollow", alice, carol, nil); set(r.Following) {
		t.Errorf("unfollow = %+v", r)
	}
	if p := e.page("/accounts/"+alice.id+"/following", ""); len(p.Accounts) != 0 {
		t.Errorf("alice still follows %v", ids(p.Accounts))
	}

	// Deactivated accounts are not found and leave the lists.
	if _, err := e.app.Accounts.RecordLifecycleEvent(t.Context(), account.LifecycleInput{
		AccountID: bob.id, Kind: domainaccount.LifecycleDeactivated, InitiatedBy: domainaccount.InitiatedBySelf,
	}); err != nil {
		t.Fatal(err)
	}
	e.want(e.do(http.MethodGet, "/accounts/"+bob.id, "", nil, nil), http.StatusNotFound, "deactivated account")
	if p := e.page("/accounts/"+carol.id+"/followers", ""); len(p.Accounts) != 0 {
		t.Errorf("carol's followers = %v, want the deactivated bob left out", ids(p.Accounts))
	}
}

// TestClientAPIBlocksAndMutes checks that an account that blocks the viewer is not found
// anywhere, as if it did not exist, and covers the block and mute lists.
func TestClientAPIBlocksAndMutes(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice, carol, dave := e.user("alice"), e.user("carol"), e.user("dave")
	e.relate("follow", alice, carol, nil)
	e.relate("follow", carol, alice, nil)
	e.relate("follow", alice, dave, nil)
	e.relate("follow", carol, dave, nil)

	// Carol blocks alice: the follows between them end in both directions.
	if r := e.relate("block", carol, alice, nil); !set(r.Blocking) || set(r.Following) || set(r.FollowedBy) {
		t.Errorf("block = %+v", r)
	}
	if r := e.relate("block", carol, alice, nil); !set(r.Blocking) {
		t.Errorf("block again = %+v", r)
	}
	// Alice cannot find carol anymore: every endpoint answers as for an unknown account, except the
	// ones for alice's own edges (TestBlockedUserManagesTheirOwnEdges).
	e.want(e.do(http.MethodGet, "/accounts/"+carol.id, alice.token, nil, nil), http.StatusNotFound, "get blocker")
	e.want(e.do(http.MethodGet, "/accounts/lookup?acct=carol", alice.token, nil, nil), http.StatusNotFound, "lookup blocker")
	e.want(e.do(http.MethodGet, "/accounts/"+carol.id+"/followers", alice.token, nil, nil), http.StatusNotFound, "blocker's followers")
	for _, action := range []string{"follow", "unfollow", "mute"} {
		e.want(e.do(http.MethodPost, "/accounts/"+carol.id+"/"+action, alice.token, nil, nil), http.StatusNotFound, action+" blocker")
	}
	if rs := e.relationships(alice, carol.id, dave.id); len(rs) != 1 || rs[0].Id != dave.id {
		t.Errorf("alice's relationships = %+v, want dave's only", rs)
	}
	if p := e.page("/accounts/"+dave.id+"/followers", alice.token); !slices.Equal(ids(p.Accounts), []string{alice.id}) {
		t.Errorf("dave's followers as alice = %v, want carol left out", ids(p.Accounts))
	}
	if p := e.page("/accounts/"+dave.id+"/followers", ""); !slices.Equal(ids(p.Accounts), []string{carol.id, alice.id}) {
		t.Errorf("dave's followers = %v", ids(p.Accounts))
	}
	// Carol still sees alice, and cannot follow her while blocking her.
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, carol.token, nil, nil), http.StatusOK, "blocker gets blocked")
	if typ := e.want(e.do(http.MethodPost, "/accounts/"+alice.id+"/follow", carol.token, nil, nil), http.StatusConflict, "follow blocked"); typ != clientapi.ProblemTypeBlocking {
		t.Errorf("follow blocked type = %s", typ)
	}
	if p := e.page("/blocks", carol.token); !slices.Equal(ids(p.Accounts), []string{alice.id}) {
		t.Errorf("carol's blocks = %v", ids(p.Accounts))
	}
	if r := e.relate("unblock", carol, alice, nil); set(r.Blocking) || set(r.FollowedBy) {
		t.Errorf("unblock = %+v, want no block and the follow still ended", r)
	}
	e.want(e.do(http.MethodGet, "/accounts/"+carol.id, alice.token, nil, nil), http.StatusOK, "get after unblock")
	if p := e.page("/blocks", carol.token); len(p.Accounts) != 0 {
		t.Errorf("carol's blocks after unblock = %v", ids(p.Accounts))
	}

	// Mutes, with and without notifications and an expiry.
	if r := e.relate("mute", alice, carol, map[string]any{"notifications": false, "duration": 3600}); !set(r.Muting) || set(r.MutingNotifications) {
		t.Errorf("mute = %+v", r)
	}
	if r := e.relate("mute", alice, dave, nil); !set(r.Muting) || !set(r.MutingNotifications) {
		t.Errorf("mute with defaults = %+v", r)
	}
	e.want(e.do(http.MethodPost, "/accounts/"+dave.id+"/mute", alice.token, map[string]int{"duration": -1}, nil), http.StatusBadRequest, "negative duration")
	var mutes clientapi.MutePage
	e.want(e.do(http.MethodGet, "/mutes?limit=1", alice.token, nil, &mutes), http.StatusOK, "mutes")
	if len(mutes.Mutes) != 1 || mutes.Mutes[0].Account.Id != dave.id || !mutes.Mutes[0].Notifications || mutes.Mutes[0].ExpiresAt != nil || mutes.NextCursor == nil {
		t.Fatalf("first mutes page = %+v", mutes)
	}
	cursor := *mutes.NextCursor
	mutes = clientapi.MutePage{}
	e.want(e.do(http.MethodGet, "/mutes?limit=1&cursor="+url.QueryEscape(cursor), alice.token, nil, &mutes), http.StatusOK, "mutes page 2")
	if len(mutes.Mutes) != 1 || mutes.Mutes[0].Account.Id != carol.id || mutes.Mutes[0].Notifications || mutes.Mutes[0].ExpiresAt == nil || mutes.NextCursor != nil {
		t.Errorf("second mutes page = %+v", mutes)
	}
	if r := e.relate("unmute", alice, carol, nil); set(r.Muting) {
		t.Errorf("unmute = %+v", r)
	}
	mutes = clientapi.MutePage{}
	e.want(e.do(http.MethodGet, "/mutes", alice.token, nil, &mutes), http.StatusOK, "mutes after unmute")
	if len(mutes.Mutes) != 1 || mutes.Mutes[0].Account.Id != dave.id {
		t.Errorf("mutes after unmute = %+v", mutes)
	}
}

func TestClientAPIDomainMutes(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice, bob := e.user("alice"), e.user("bob")
	mute := func(domain string) clientapi.DomainMute {
		t.Helper()
		var m clientapi.DomainMute
		e.want(e.do(http.MethodPost, "/domain_mutes", alice.token, map[string]string{"domain": domain}, &m), http.StatusOK, "mute "+domain)
		return m
	}
	// The domain is normalized: lower case, IDNA A-labels, no trailing dot.
	if m := mute("Remote.Example."); m.Domain != "remote.example" || m.CreatedAt.IsZero() {
		t.Errorf("mute = %+v", m)
	}
	if m := mute("例え.テスト"); m.Domain != "xn--r8jz45g.xn--zckzah" {
		t.Errorf("mute IDN = %+v", m)
	}
	mute("remote.example") // again: nothing changes
	mute("a.example")
	for _, bad := range []string{"bad domain", "remote.example/path", "."} {
		e.want(e.do(http.MethodPost, "/domain_mutes", alice.token, map[string]string{"domain": bad}, nil), http.StatusUnprocessableEntity, "mute "+bad)
	}

	var page clientapi.DomainMutePage
	e.want(e.do(http.MethodGet, "/domain_mutes?limit=2", alice.token, nil, &page), http.StatusOK, "domain mutes")
	if len(page.DomainMutes) != 2 || page.DomainMutes[0].Domain != "a.example" || page.DomainMutes[1].Domain != "remote.example" || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	cursor := *page.NextCursor
	page = clientapi.DomainMutePage{}
	e.want(e.do(http.MethodGet, "/domain_mutes?limit=2&cursor="+url.QueryEscape(cursor), alice.token, nil, &page), http.StatusOK, "domain mutes page 2")
	if len(page.DomainMutes) != 1 || page.DomainMutes[0].Domain != "xn--r8jz45g.xn--zckzah" || page.NextCursor != nil {
		t.Errorf("second page = %+v", page)
	}
	page = clientapi.DomainMutePage{}
	e.want(e.do(http.MethodGet, "/domain_mutes", bob.token, nil, &page), http.StatusOK, "bob's domain mutes")
	if len(page.DomainMutes) != 0 {
		t.Errorf("bob's domain mutes = %+v", page)
	}

	// Unmuting takes the domain in any spelling, and again changes nothing.
	for range 2 {
		e.want(e.do(http.MethodDelete, "/domain_mutes/REMOTE.example", alice.token, nil, nil), http.StatusNoContent, "unmute")
	}
	page = clientapi.DomainMutePage{}
	e.want(e.do(http.MethodGet, "/domain_mutes", alice.token, nil, &page), http.StatusOK, "domain mutes after unmute")
	if len(page.DomainMutes) != 2 || page.DomainMutes[1].Domain != "xn--r8jz45g.xn--zckzah" {
		t.Errorf("domain mutes after unmute = %+v", page)
	}
	e.want(e.do(http.MethodGet, "/domain_mutes", "", nil, nil), http.StatusUnauthorized, "anonymous")
}
