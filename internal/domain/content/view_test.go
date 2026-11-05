// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
)

// The policy tables below are the spec of CanView (docs/architecture/visibility.md).
// Each row reads: in this context, this viewer sees a status of each visibility — Y — or not — "-".
// The columns are always, in order:
//
//	public  local  unlisted  private  direct
//
// The tests fail unless every table lists every context and every viewer exactly once, so the
// tables cover every visibility × viewer × context combination.

// viewer is a named viewer kind: who is looking and how they relate to the author and status.
type viewer struct {
	name string
	who  func(authorID string) content.Viewer
	rel  content.Relation
}

func local(id string) func(string) content.Viewer {
	return func(string) content.Viewer { return content.Viewer{Kind: content.ViewerLocal, AccountID: id} }
}

func remote(id string) func(string) content.Viewer {
	return func(string) content.Viewer { return content.Viewer{Kind: content.ViewerRemote, AccountID: id} }
}

var (
	localAuthorID  = testID(1)
	remoteAuthorID = testID(2)
	viewerID       = testID(3)
)

var (
	following   = content.Relation{Follow: content.FollowActive}
	followAndAt = content.Relation{Follow: content.FollowActive, Mentioned: true}
)

// viewers is every viewer kind. For an object fetch a remote viewer stands for the signer's
// domain: "remote follower" means the signer's domain has an active follower of the author.
var viewers = []viewer{
	{"anonymous", func(string) content.Viewer { return content.Viewer{} }, content.Relation{}},
	{"author", authorViewer, content.Relation{}},
	{"local stranger", local(testID(11)), content.Relation{}},
	{"local pending follower", local(testID(12)), content.Relation{Follow: content.FollowPending}},
	{"local follower", local(testID(13)), following},
	{"local mentioned", local(testID(14)), content.Relation{Mentioned: true}},
	{"local blocking author", local(testID(15)), withBlocking(followAndAt)},
	{"local blocked by author", local(testID(16)), withBlockedBy(followAndAt)},
	{"moderator", func(string) content.Viewer {
		return content.Viewer{Kind: content.ViewerLocal, AccountID: testID(17), Moderator: true}
	}, content.Relation{}},
	{"remote stranger", remote(testID(21)), content.Relation{}},
	{"remote follower", remote(testID(22)), following},
	{"remote mentioned", remote(testID(23)), content.Relation{Mentioned: true}},
	{"remote blocked by author", remote(testID(24)), withBlockedBy(followAndAt)},
}

// authorViewer is the author looking at their own status, local or remote.
func authorViewer(authorID string) content.Viewer {
	if authorID == remoteAuthorID {
		return content.Viewer{Kind: content.ViewerRemote, AccountID: authorID}
	}
	return content.Viewer{Kind: content.ViewerLocal, AccountID: authorID}
}

func withBlocking(r content.Relation) content.Relation  { r.Blocking = true; return r }
func withBlockedBy(r content.Relation) content.Relation { r.BlockedBy = true; return r }

type block struct {
	ctx  content.ViewContext
	rows []string // "viewer | public local unlisted private direct"
}

// localAuthorPolicy: statuses of a local author with no restrictions.
var localAuthorPolicy = []block{
	{content.ContextStatus, []string{
		// viewer                   pub loc unl prv dir
		"anonymous                | Y   -   Y   -   -",
		"author                   | Y   Y   Y   Y   Y",
		"local stranger           | Y   Y   Y   -   -",
		"local pending follower   | Y   Y   Y   -   -",
		"local follower           | Y   Y   Y   Y   -",
		"local mentioned          | Y   Y   Y   Y   Y",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   Y   Y   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextProfile, []string{
		"anonymous                | Y   -   Y   -   -",
		"author                   | Y   Y   Y   Y   Y",
		"local stranger           | Y   Y   Y   -   -",
		"local pending follower   | Y   Y   Y   -   -",
		"local follower           | Y   Y   Y   Y   -",
		"local mentioned          | Y   Y   Y   Y   Y",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   Y   Y   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextHome, []string{
		"anonymous                | -   -   -   -   -",
		"author                   | Y   Y   Y   Y   Y",
		"local stranger           | -   -   -   -   -",
		"local pending follower   | -   -   -   -   -",
		"local follower           | Y   Y   Y   Y   -",
		"local mentioned          | -   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | -   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextLocalTimeline, []string{
		"anonymous                | Y   -   -   -   -",
		"author                   | Y   Y   -   -   -",
		"local stranger           | Y   Y   -   -   -",
		"local pending follower   | Y   Y   -   -   -",
		"local follower           | Y   Y   -   -   -",
		"local mentioned          | Y   Y   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   Y   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextFederatedTimeline, []string{
		"anonymous                | Y   -   -   -   -",
		"author                   | Y   -   -   -   -",
		"local stranger           | Y   -   -   -   -",
		"local pending follower   | Y   -   -   -   -",
		"local follower           | Y   -   -   -   -",
		"local mentioned          | Y   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextSearch, []string{
		"anonymous                | -   -   -   -   -",
		"author                   | Y   Y   -   -   -",
		"local stranger           | Y   Y   -   -   -",
		"local pending follower   | Y   Y   -   -   -",
		"local follower           | Y   Y   -   -   -",
		"local mentioned          | Y   Y   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   Y   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextHashtag, []string{
		"anonymous                | Y   -   -   -   -",
		"author                   | Y   Y   -   -   -",
		"local stranger           | Y   Y   -   -   -",
		"local pending follower   | Y   Y   -   -   -",
		"local follower           | Y   Y   -   -   -",
		"local mentioned          | Y   Y   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   Y   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextObjectFetch, []string{
		"anonymous                | Y   -   Y   -   -", // unsigned GET
		"author                   | -   -   -   -   -",
		"local stranger           | -   -   -   -   -",
		"local pending follower   | -   -   -   -   -",
		"local follower           | -   -   -   -   -",
		"local mentioned          | -   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | -   -   -   -   -",
		"remote stranger          | Y   -   Y   -   -",
		"remote follower          | Y   Y   Y   Y   -",
		"remote mentioned         | Y   -   Y   Y   Y", // ADR-0017: local needs a follower
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextDelivery, []string{
		"anonymous                | -   -   -   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | -   -   -   -   -",
		"local pending follower   | -   -   -   -   -",
		"local follower           | -   -   -   -   -",
		"local mentioned          | -   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | -   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | Y   Y   Y   Y   -",
		"remote mentioned         | Y   -   Y   Y   Y", // ADR-0017: local is not sent to non-followers
		"remote blocked by author | -   -   -   -   -",
	}},
}

// remoteAuthorPolicy: statuses of a remote author with no restrictions. A remote status never has
// the local visibility, so that column is always denied.
var remoteAuthorPolicy = []block{
	{content.ContextStatus, []string{
		// viewer                   pub loc unl prv dir
		"anonymous                | Y   -   Y   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | Y   -   Y   -   -",
		"local pending follower   | Y   -   Y   -   -",
		"local follower           | Y   -   Y   Y   -",
		"local mentioned          | Y   -   Y   Y   Y",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   -   Y   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextProfile, []string{
		"anonymous                | Y   -   Y   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | Y   -   Y   -   -",
		"local pending follower   | Y   -   Y   -   -",
		"local follower           | Y   -   Y   Y   -",
		"local mentioned          | Y   -   Y   Y   Y",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   -   Y   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextHome, []string{
		"anonymous                | -   -   -   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | -   -   -   -   -",
		"local pending follower   | -   -   -   -   -",
		"local follower           | Y   -   Y   Y   -",
		"local mentioned          | -   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | -   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextLocalTimeline, []string{
		"anonymous                | -   -   -   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | -   -   -   -   -",
		"local pending follower   | -   -   -   -   -",
		"local follower           | -   -   -   -   -",
		"local mentioned          | -   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | -   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextFederatedTimeline, []string{
		"anonymous                | Y   -   -   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | Y   -   -   -   -",
		"local pending follower   | Y   -   -   -   -",
		"local follower           | Y   -   -   -   -",
		"local mentioned          | Y   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextSearch, []string{
		"anonymous                | -   -   -   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | Y   -   -   -   -",
		"local pending follower   | Y   -   -   -   -",
		"local follower           | Y   -   -   -   -",
		"local mentioned          | Y   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextHashtag, []string{
		"anonymous                | Y   -   -   -   -",
		"author                   | -   -   -   -   -",
		"local stranger           | Y   -   -   -   -",
		"local pending follower   | Y   -   -   -   -",
		"local follower           | Y   -   -   -   -",
		"local mentioned          | Y   -   -   -   -",
		"local blocking author    | -   -   -   -   -",
		"local blocked by author  | -   -   -   -   -",
		"moderator                | Y   -   -   -   -",
		"remote stranger          | -   -   -   -   -",
		"remote follower          | -   -   -   -   -",
		"remote mentioned         | -   -   -   -   -",
		"remote blocked by author | -   -   -   -   -",
	}},
	{content.ContextObjectFetch, allDenied()},
	{content.ContextDelivery, allDenied()},
}

func allDenied() []string {
	rows := make([]string, len(viewers))
	for i, v := range viewers {
		rows[i] = v.name + " | -   -   -   -   -"
	}
	return rows
}

func TestCanViewLocalAuthor(t *testing.T) {
	checkPolicy(t, localAuthorPolicy, localAuthorID, true)
}

func TestCanViewRemoteAuthor(t *testing.T) {
	checkPolicy(t, remoteAuthorPolicy, remoteAuthorID, false)
}

func checkPolicy(t *testing.T, policy []block, authorID string, authorIsLocal bool) {
	t.Helper()
	byName := map[string]viewer{}
	for _, v := range viewers {
		byName[v.name] = v
	}
	seenCtx := map[content.ViewContext]bool{}
	for _, b := range policy {
		if seenCtx[b.ctx] {
			t.Fatalf("context %v listed twice", b.ctx)
		}
		seenCtx[b.ctx] = true
		seenViewer := map[string]bool{}
		for _, r := range b.rows {
			name, sees, ok := strings.Cut(r, "|")
			name = strings.TrimSpace(name)
			v, known := byName[name]
			if !ok || !known || seenViewer[name] {
				t.Fatalf("%v: malformed row, unknown or duplicate viewer: %q", b.ctx, r)
			}
			seenViewer[name] = true
			want := parseSees(t, sees)
			for i, vis := range content.Visibilities() {
				s := content.StatusFacts{AuthorAccountID: authorID, AuthorIsLocal: authorIsLocal, Visibility: vis}
				got := content.CanView(b.ctx, v.who(authorID), s, content.AuthorRestrictions{}, v.rel)
				if got != want[i] {
					t.Errorf("%v / %s / %s: CanView = %v, want %v", b.ctx, name, vis, got, want[i])
				}
			}
		}
		if len(seenViewer) != len(viewers) {
			t.Errorf("%v: lists %d of %d viewers", b.ctx, len(seenViewer), len(viewers))
		}
	}
	if len(seenCtx) != len(content.ViewContexts()) {
		t.Errorf("policy lists %d of %d contexts", len(seenCtx), len(content.ViewContexts()))
	}
}

func parseSees(t *testing.T, s string) []bool {
	t.Helper()
	cells := strings.Fields(s)
	if len(cells) != len(content.Visibilities()) {
		t.Fatalf("row %q has %d cells, want %d", s, len(cells), len(content.Visibilities()))
	}
	out := make([]bool, len(cells))
	for i, c := range cells {
		switch c {
		case "Y":
			out[i] = true
		case "-":
		default:
			t.Fatalf("row %q: cell %q is neither Y nor -", s, c)
		}
	}
	return out
}

// combos runs f over every context × viewer × visibility, for local and remote authors.
func combos(f func(name string, ctx content.ViewContext, v content.Viewer, s content.StatusFacts, rel content.Relation, isAuthor bool)) {
	for _, authorID := range []string{localAuthorID, remoteAuthorID} {
		for _, ctx := range content.ViewContexts() {
			for _, vw := range viewers {
				for _, vis := range content.Visibilities() {
					s := content.StatusFacts{AuthorAccountID: authorID, AuthorIsLocal: authorID == localAuthorID, Visibility: vis}
					name := fmt.Sprintf("%s / %v / %s / %s", authorID, ctx, vw.name, vis)
					f(name, ctx, vw.who(authorID), s, vw.rel, vw.name == "author")
				}
			}
		}
	}
}

// Silence (visibility_ceiling_private): every post, past ones included, is judged as
// min(original, private) — exactly as if it had been posted followers-only.
func TestCanViewVisibilityCeiling(t *testing.T) {
	silenced := content.AuthorRestrictions{VisibilityCeilingPrivate: true}
	combos(func(name string, ctx content.ViewContext, v content.Viewer, s content.StatusFacts, rel content.Relation, _ bool) {
		capped := s
		capped.Visibility = content.EffectiveVisibility(s.Visibility, silenced)
		if s.Visibility == content.VisibilityLocal && !s.AuthorIsLocal {
			capped.Visibility = s.Visibility // invalid stays invalid
		}
		got := content.CanView(ctx, v, s, silenced, rel)
		want := content.CanView(ctx, v, capped, content.AuthorRestrictions{}, rel)
		if got != want {
			t.Errorf("%s: silenced CanView = %v, want %v (as %s)", name, got, want, capped.Visibility)
		}
	})
}

// public_discovery: the author's posts leave the local, federated, and hashtag timelines and the
// full-text search, and nothing else changes.
func TestCanViewPublicDiscovery(t *testing.T) {
	limited := content.AuthorRestrictions{PublicDiscovery: true}
	combos(func(name string, ctx content.ViewContext, v content.Viewer, s content.StatusFacts, rel content.Relation, _ bool) {
		want := content.CanView(ctx, v, s, content.AuthorRestrictions{}, rel)
		switch ctx {
		case content.ContextLocalTimeline, content.ContextFederatedTimeline, content.ContextHashtag, content.ContextSearch:
			want = false
		}
		if got := content.CanView(ctx, v, s, limited, rel); got != want {
			t.Errorf("%s: CanView = %v, want %v", name, got, want)
		}
	})
}

// profile_view (suspension, deletion): only the author and moderators see the posts, only on
// the status and profile pages, and still within the visibility rules.
func TestCanViewProfileView(t *testing.T) {
	hidden := content.AuthorRestrictions{ProfileView: true}
	combos(func(name string, ctx content.ViewContext, v content.Viewer, s content.StatusFacts, rel content.Relation, isAuthor bool) {
		want := false
		if (isAuthor || v.Moderator) && (ctx == content.ContextStatus || ctx == content.ContextProfile) {
			want = content.CanView(ctx, v, s, content.AuthorRestrictions{}, rel)
		}
		if got := content.CanView(ctx, v, s, hidden, rel); got != want {
			t.Errorf("%s: CanView = %v, want %v", name, got, want)
		}
	})
}

// The rules ADR-0017 and the owner confirmed, spelled out one by one.
func TestCanViewADR0017(t *testing.T) {
	localPost := content.StatusFacts{AuthorAccountID: localAuthorID, AuthorIsLocal: true, Visibility: content.VisibilityLocal}
	none := content.AuthorRestrictions{}
	anon := content.Viewer{}
	remoteActor := content.Viewer{Kind: content.ViewerRemote, AccountID: remoteAuthorID}
	cases := []struct {
		rule string
		got  bool
		want bool
	}{
		{"an anonymous visitor never sees a local post, even on its page",
			content.CanView(content.ContextStatus, anon, localPost, none, content.Relation{}), false},
		{"an anonymous visitor never sees a local post on the local timeline",
			content.CanView(content.ContextLocalTimeline, anon, localPost, none, content.Relation{}), false},
		{"an unsigned fetch of a local post is denied (404)",
			content.CanView(content.ContextObjectFetch, anon, localPost, none, content.Relation{}), false},
		{"a logged-in local account sees a local post without following",
			content.CanView(content.ContextStatus, content.Viewer{Kind: content.ViewerLocal, AccountID: viewerID}, localPost, none, content.Relation{}), true},
		{"a local post is on the local timeline",
			content.CanView(content.ContextLocalTimeline, content.Viewer{Kind: content.ViewerLocal, AccountID: viewerID}, localPost, none, content.Relation{}), true},
		{"a local post is never on the federated timeline",
			content.CanView(content.ContextFederatedTimeline, content.Viewer{Kind: content.ViewerLocal, AccountID: localAuthorID}, localPost, none, content.Relation{}), false},
		{"a local post is delivered to a remote follower",
			content.CanView(content.ContextDelivery, remoteActor, localPost, none, content.Relation{Follow: content.FollowActive}), true},
		{"a local post is not delivered to a mentioned remote actor that does not follow",
			content.CanView(content.ContextDelivery, remoteActor, localPost, none, content.Relation{Mentioned: true}), false},
		{"a local post is not delivered to a pending remote follower",
			content.CanView(content.ContextDelivery, remoteActor, localPost, none, content.Relation{Follow: content.FollowPending, Mentioned: true}), false},
		{"a signed fetch from a domain with an active follower gets a local post",
			content.CanView(content.ContextObjectFetch, remoteActor, localPost, none, content.Relation{Follow: content.FollowActive}), true},
		{"a signed fetch from a domain with only a mention gets 404 for a local post",
			content.CanView(content.ContextObjectFetch, remoteActor, localPost, none, content.Relation{Mentioned: true}), false},
		{"a block wins over following",
			content.CanView(content.ContextStatus, content.Viewer{Kind: content.ViewerLocal, AccountID: viewerID}, localPost, none, content.Relation{Follow: content.FollowActive, BlockedBy: true}), false},
		{"suspension wins over following",
			content.CanView(content.ContextStatus, content.Viewer{Kind: content.ViewerLocal, AccountID: viewerID}, localPost, content.AuthorRestrictions{ProfileView: true}, content.Relation{Follow: content.FollowActive}), false},
		{"a remote status cannot carry the local visibility",
			content.CanView(content.ContextStatus, content.Viewer{Kind: content.ViewerLocal, AccountID: viewerID},
				content.StatusFacts{AuthorAccountID: remoteAuthorID, Visibility: content.VisibilityLocal}, none, content.Relation{Follow: content.FollowActive}), false},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: CanView = %v, want %v", c.rule, c.got, c.want)
		}
	}
}

// TestCanViewDirectDenied checks that a viewer without read:direct sees no direct post, its
// author's own and a mentioned recipient's alike, and that other visibilities are unaffected
// (ADR-0090 §4).
func TestCanViewDirectDenied(t *testing.T) {
	direct := content.StatusFacts{AuthorAccountID: localAuthorID, AuthorIsLocal: true, Visibility: content.VisibilityDirect}
	public := direct
	public.Visibility = content.VisibilityPublic
	author := content.Viewer{Kind: content.ViewerLocal, AccountID: localAuthorID}
	if !content.CanView(content.ContextStatus, author, direct, content.AuthorRestrictions{}, content.Relation{}) {
		t.Fatal("the author does not see their own direct post")
	}
	author.DirectDenied = true
	for _, ctx := range content.ViewContexts() {
		if content.CanView(ctx, author, direct, content.AuthorRestrictions{}, content.Relation{Mentioned: true}) {
			t.Errorf("%v: a viewer without read:direct sees a direct post", ctx)
		}
	}
	denied := content.Viewer{Kind: content.ViewerLocal, AccountID: "01890a5d-ac96-774b-bcce-b302099a8099", DirectDenied: true}
	if content.CanView(content.ContextStatus, denied, direct, content.AuthorRestrictions{}, content.Relation{Mentioned: true}) {
		t.Error("a mentioned viewer without read:direct sees a direct post")
	}
	allowed := denied
	allowed.DirectDenied = false
	if !content.CanView(content.ContextStatus, allowed, direct, content.AuthorRestrictions{}, content.Relation{Mentioned: true}) {
		t.Error("a mentioned viewer with read:direct does not see a direct post")
	}
	if !content.CanView(content.ContextStatus, denied, public, content.AuthorRestrictions{}, content.Relation{}) {
		t.Error("DirectDenied hides a public post")
	}
}

func TestCanViewFailsClosed(t *testing.T) {
	author := content.Viewer{Kind: content.ViewerLocal, AccountID: localAuthorID}
	public := content.StatusFacts{AuthorAccountID: localAuthorID, AuthorIsLocal: true, Visibility: content.VisibilityPublic}
	if content.CanView(content.ViewContext(0), author, public, content.AuthorRestrictions{}, content.Relation{}) {
		t.Error("the zero context is allowed")
	}
	if content.CanView(content.ContextStatus, content.Viewer{Kind: content.ViewerKind(99)}, public, content.AuthorRestrictions{}, content.Relation{}) {
		t.Error("an unknown viewer kind is allowed")
	}
	bad := public
	bad.Visibility = "everyone"
	if content.CanView(content.ContextStatus, author, bad, content.AuthorRestrictions{}, content.Relation{}) {
		t.Error("an unknown visibility is allowed")
	}
	if content.CanView(content.ContextStatus, content.Viewer{Kind: content.ViewerLocal}, content.StatusFacts{AuthorIsLocal: true, Visibility: content.VisibilityDirect},
		content.AuthorRestrictions{}, content.Relation{}) {
		t.Error("an empty account ID is taken for the author")
	}
	if content.CanView(content.ContextStatus, content.Viewer{Moderator: true}, public, content.AuthorRestrictions{ProfileView: true}, content.Relation{}) {
		t.Error("the moderator flag is honored for an anonymous viewer")
	}
}
