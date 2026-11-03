// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	postID   = "01890a5d-ac96-774b-bcce-b302099a8100"
	parentID = "01890a5d-ac96-774b-bcce-b302099a8101"
	boostID  = "01890a5d-ac96-774b-bcce-b302099a8102"
	goneID   = "01890a5d-ac96-774b-bcce-b302099a8199"
	tagID    = "01890a5d-ac96-774b-bcce-b302099a8104"
)

// timelineCall records a call of fakeTimelines.
type timelineCall struct {
	timeline string
	viewer   appcontent.Viewer
	arg      string
	filter   apptimeline.ProfileFilter
	query    apptimeline.Query
}

// fakeTimelines returns page from every timeline and records the calls.
type fakeTimelines struct {
	page  apptimeline.Page
	calls []timelineCall
}

func (f *fakeTimelines) record(c timelineCall) (apptimeline.Page, error) {
	f.calls = append(f.calls, c)
	return f.page, nil
}

func (f *fakeTimelines) Home(_ context.Context, v appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error) {
	return f.record(timelineCall{timeline: "home", viewer: v, query: q})
}

func (f *fakeTimelines) Local(_ context.Context, v appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error) {
	return f.record(timelineCall{timeline: "local", viewer: v, query: q})
}

func (f *fakeTimelines) Public(_ context.Context, v appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error) {
	return f.record(timelineCall{timeline: "public", viewer: v, query: q})
}

func (f *fakeTimelines) Hashtag(_ context.Context, v appcontent.Viewer, tagID string, q apptimeline.Query) (apptimeline.Page, error) {
	return f.record(timelineCall{timeline: "hashtag", viewer: v, arg: tagID, query: q})
}

func (f *fakeTimelines) Profile(_ context.Context, v appcontent.Viewer, author string, pf apptimeline.ProfileFilter, q apptimeline.Query) (apptimeline.Page, error) {
	return f.record(timelineCall{timeline: "profile", viewer: v, arg: author, filter: pf, query: q})
}

// fakeViews holds the statuses, boosts, tags, and pins that the handlers read.
type fakeViews struct {
	statuses map[string]appcontent.StatusView
	boosts   map[string]content.Boost
	tags     map[string]content.Tag
	pinned   []string
	viewers  []appcontent.Viewer
	requests [][]string // the ids of each call of Views
}

func (f *fakeViews) Views(_ context.Context, viewer appcontent.Viewer, ids []string) ([]appcontent.StatusView, error) {
	f.viewers = append(f.viewers, viewer)
	f.requests = append(f.requests, ids)
	var out []appcontent.StatusView
	for _, id := range ids {
		if v, ok := f.statuses[id]; ok {
			if viewer.Kind == appcontent.ViewerAnonymous {
				v.Relation = nil
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeViews) Boosts(_ context.Context, ids []string) ([]content.Boost, error) {
	var out []content.Boost
	for _, id := range ids {
		if b, ok := f.boosts[id]; ok {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeViews) Tag(_ context.Context, name string) (content.Tag, error) {
	t, ok := f.tags[appcontent.NormalizeTag(name)]
	if !ok {
		return content.Tag{}, appcontent.ErrNotFound
	}
	return t, nil
}

func (f *fakeViews) Pinned(ctx context.Context, viewer appcontent.Viewer, _ string) ([]appcontent.StatusView, error) {
	return f.Views(ctx, viewer, f.pinned)
}

var (
	aliceActor = appaccount.Actor{
		Account: account.Account{ID: aliceAccountID, URI: "https://social.example/ap/actors/" + aliceAccountID, CreatedAt: createdAt},
		Local:   &account.LocalAccount{AccountID: aliceAccountID, UserID: userOf(aliceAccountID), Username: "alice"},
	}
	bobActor = appaccount.Actor{
		Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: createdAt},
		Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
		Lifecycle: account.InitialLifecycleState(bobAccountID),
	}
)

var publishedAt = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

// timelineEnv is an authEnv whose timelines list a boost by bob of alice's reply, and alice's
// reply itself.
type timelineEnv struct {
	*authEnv
	timelines *fakeTimelines
	views     *fakeViews
	outboxes  *fakeOutboxes
	languages []string
}

// fakeOutboxes records the outbox fetches that profiles start, and starts them when start is
// set.
type fakeOutboxes struct {
	start bool
	err   error
	calls [][2]string // viewer, account
}

func (f *fakeOutboxes) FetchOutbox(_ context.Context, viewerAccountID, accountID string) (bool, error) {
	f.calls = append(f.calls, [2]string{viewerAccountID, accountID})
	return f.start && f.err == nil, f.err
}

func newTimelineEnv(t *testing.T) *timelineEnv {
	t.Helper()
	te := &timelineEnv{
		timelines: &fakeTimelines{page: apptimeline.Page{
			Entries: []apptimeline.Entry{
				{ID: boostID, Kind: timeline.KindBoost, StatusID: postID, AuthorAccountID: bobAccountID, OriginalAuthorAccountID: aliceAccountID},
				{ID: postID, Kind: timeline.KindPost, StatusID: postID, AuthorAccountID: aliceAccountID, OriginalAuthorAccountID: aliceAccountID},
				// A status deleted after the page was read is left out.
				{ID: goneID, Kind: timeline.KindPost, StatusID: goneID, AuthorAccountID: aliceAccountID, OriginalAuthorAccountID: aliceAccountID},
			},
			Next: goneID,
		}},
		views: &fakeViews{
			statuses: map[string]appcontent.StatusView{postID: {
				Status: content.Status{
					ID: postID, URI: "https://social.example/ap/statuses/" + postID,
					AuthorAccountID: aliceAccountID, IsLocal: true, Visibility: content.VisibilityPublic,
					InReplyToURI: "https://social.example/ap/statuses/" + parentID, PublishedAt: publishedAt,
				},
				Revision: content.Revision{
					Seq: 2, SpoilerText: "cw", Sensitive: true, Language: "ja", RevisedAt: publishedAt.Add(time.Hour),
					Bodies: []content.Body{{Language: "ja", ContentHTML: "<p>ja</p>"}},
				},
				Author:    aliceActor,
				InReplyTo: &content.Status{ID: parentID, AuthorAccountID: bobAccountID},
				Mentions:  []appaccount.Actor{bobActor},
				Counters:  content.Counters{StatusID: postID, RepliesCount: 1, BoostsCount: 2, FavouritesCount: 3},
				Tags:      []content.Tag{{ID: tagID, Name: "Cats", NameNormalized: "cats"}},
				Relation:  &appcontent.ViewerRelation{Favourited: true, Bookmarked: true, Author: true},
			}},
			boosts: map[string]content.Boost{boostID: {
				ID: boostID, URI: "https://social.example/ap/boosts/" + boostID, BoosterAccountID: bobAccountID, StatusID: postID,
				Visibility: content.VisibilityPublic, IsLocal: true, BoostedAt: publishedAt.Add(2 * time.Hour),
			}},
			tags: map[string]content.Tag{"cats": {ID: tagID, Name: "Cats", NameNormalized: "cats"}},
		},
		outboxes:  &fakeOutboxes{},
		languages: []string{"ja"},
	}
	te.authEnv = newAuthEnv(t, func(o *clientapi.Options) {
		o.Timelines, o.TimelineReader, o.Outboxes = te.timelines, te.views, te.outboxes
		// These tests read the federated timeline anonymously, which its default does not allow.
		o.Instance = federatedTimelineForEveryone()
		o.TimelineLanguages = func(_ context.Context, userID string) ([]string, error) {
			if userID != userOf(aliceAccountID) {
				t.Errorf("TimelineLanguages(%q), want alice's user", userID)
			}
			return te.languages, nil
		}
		o.Accounts.(fakeAccounts)[bobAccountID] = bobActor
		o.Accounts.(fakeAccounts)[remoteActor.Account.ID] = remoteActor
	})
	return te
}

// remoteActor is a remote account, whose profile may start fetching its recent posts.
var remoteActor = appaccount.Actor{
	Account: account.Account{
		ID: "01890a5d-ac96-774b-bcce-b302099a8110", Domain: "remote.example", URI: "https://remote.example/users/carol", CreatedAt: createdAt,
	},
	Lifecycle: account.InitialLifecycleState("01890a5d-ac96-774b-bcce-b302099a8110"),
}

func decodePage(t *testing.T, res *http.Response) clientapi.StatusPage {
	t.Helper()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var p clientapi.StatusPage
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHomeTimelineRendersPostsAndBoosts(t *testing.T) {
	e := newTimelineEnv(t)
	res := e.get(t, "/timelines/home?cursor="+postID+"&limit=3", map[string]string{"Authorization": "Bearer read"})
	page := decodePage(t, res)

	want := []timelineCall{{
		timeline: "home", viewer: appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: aliceAccountID},
		query: apptimeline.Query{Before: postID, Limit: 3, Languages: []string{"ja"}},
	}}
	if !slices.EqualFunc(e.timelines.calls, want, equalCall) {
		t.Errorf("calls = %+v, want %+v", e.timelines.calls, want)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if page.NextCursor == nil || *page.NextCursor != goneID {
		t.Errorf("next_cursor = %v, want the last entry read", page.NextCursor)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %+v, want the boost and the post", page.Items)
	}
	boost, post := page.Items[0], page.Items[1]

	if post.Id != postID || post.Account.Username != "alice" || post.Content != "<p>ja</p>" || post.SpoilerText != "cw" ||
		!post.Sensitive || post.Visibility != clientapi.VisibilityPublic || post.Language != "ja" ||
		deref(post.Url) != "https://social.example/@alice/status/"+postID || !post.CreatedAt.Equal(publishedAt) ||
		post.EditedAt == nil || !post.EditedAt.Equal(publishedAt.Add(time.Hour)) ||
		post.RepliesCount != 1 || post.BoostsCount != 2 || post.FavouritesCount != 3 || post.Boost != nil {
		t.Errorf("post = %+v", post)
	}
	if deref(post.InReplyToId) != parentID || deref(post.InReplyToAccountId) != bobAccountID {
		t.Errorf("reply = %v %v, want the parent and its author", post.InReplyToId, post.InReplyToAccountId)
	}
	if !deref(post.Favourited) || deref(post.Boosted) || !deref(post.Bookmarked) || post.Pinned == nil || *post.Pinned {
		t.Errorf("viewer state = %v %v %v %v, want favourited and bookmarked", post.Favourited, post.Boosted, post.Bookmarked, post.Pinned)
	}
	if got := post.Mentions; len(got) != 1 || got[0].Id != bobAccountID || got[0].Acct != "bob" || got[0].Url != "https://social.example/@bob" {
		t.Errorf("mentions = %+v, want bob only", got)
	}
	if got := post.Tags; len(got) != 1 || got[0].Name != "Cats" || got[0].Url != "https://social.example/tags/cats" {
		t.Errorf("tags = %+v", got)
	}
	// A boost is the booster's, at the time of the boost, with the boosted post inside.
	if boost.Id != boostID || boost.Account.Username != "bob" || !boost.CreatedAt.Equal(publishedAt.Add(2*time.Hour)) ||
		boost.Content != "" || boost.Language != "ja" || len(boost.MediaAttachments)+len(boost.Mentions)+len(boost.Tags) != 0 || boost.Url != nil {
		t.Errorf("boost = %+v", boost)
	}
	if boost.Boost == nil || boost.Boost.Id != postID || boost.Boost.Account.Username != "alice" || boost.Boost.Content != "<p>ja</p>" {
		t.Errorf("boosted status = %+v, want alice's post", boost.Boost)
	}
}

// TestLinksUseTheOrigin checks that the links to the instance's own pages follow Options.Origin
// (plain HTTP on the local Docker setup) while the ActivityPub ids stay under https://Host.
func TestLinksUseTheOrigin(t *testing.T) {
	te := newTimelineEnv(t)
	timelines, views := te.timelines, te.views
	te.authEnv = newAuthEnv(t, func(o *clientapi.Options) {
		o.Origin = "http://social.example"
		o.Timelines, o.TimelineReader = timelines, views
		o.Accounts.(fakeAccounts)[bobAccountID] = bobActor
	})
	page := decodePage(t, te.get(t, "/timelines/home", map[string]string{"Authorization": "Bearer read"}))
	if len(page.Items) != 2 {
		t.Fatalf("items = %+v, want the boost and the post", page.Items)
	}
	post := page.Items[1]
	if deref(post.Url) != "http://social.example/@alice/status/"+postID || post.Uri != "https://social.example/ap/statuses/"+postID {
		t.Errorf("post url, uri = %q, %q", deref(post.Url), post.Uri)
	}
	if post.Account.Url != "http://social.example/@alice" || post.Account.Uri != aliceActor.Account.URI {
		t.Errorf("account url, uri = %q, %q", post.Account.Url, post.Account.Uri)
	}
	if got := post.Mentions; len(got) != 1 || got[0].Url != "http://social.example/@bob" {
		t.Errorf("mentions = %+v", got)
	}
	if got := post.Tags; len(got) != 1 || got[0].Url != "http://social.example/tags/cats" {
		t.Errorf("tags = %+v", got)
	}
}

func equalCall(a, b timelineCall) bool {
	return a.timeline == b.timeline && a.viewer == b.viewer && a.arg == b.arg && a.filter == b.filter &&
		a.query.Before == b.query.Before && a.query.Limit == b.query.Limit && slices.Equal(a.query.Languages, b.query.Languages)
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func TestLocalTimelineForVisitors(t *testing.T) {
	e := newTimelineEnv(t)
	res := e.get(t, "/timelines/local", nil)
	page := decodePage(t, res)
	want := []timelineCall{{timeline: "local", viewer: appcontent.Viewer{Kind: appcontent.ViewerAnonymous}}}
	if !slices.EqualFunc(e.timelines.calls, want, equalCall) {
		t.Errorf("calls = %+v, want an anonymous viewer without languages", e.timelines.calls)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want public, max-age=60", cc)
	}
	if !slices.Contains(res.Header.Values("Vary"), "Authorization") {
		t.Errorf("Vary = %q, want Authorization", res.Header.Values("Vary"))
	}
	if !slices.Equal(e.views.viewers, []appcontent.Viewer{{Kind: appcontent.ViewerAnonymous}}) {
		t.Errorf("views read for %+v, want an anonymous viewer", e.views.viewers)
	}
	for _, it := range page.Items {
		if it.Favourited != nil || it.Boosted != nil || it.Bookmarked != nil || it.Pinned != nil {
			t.Errorf("item %s has viewer state for a visitor", it.Id)
		}
	}

	// A logged-in viewer gets their own, unshared page.
	res = e.get(t, "/timelines/local", map[string]string{"Authorization": "Bearer read"})
	decodePage(t, res)
	if c := e.timelines.calls[1]; c.viewer.Kind != appcontent.ViewerLocal || c.viewer.AccountID != aliceAccountID || !slices.Equal(c.query.Languages, []string{"ja"}) {
		t.Errorf("logged-in call = %+v", c)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("logged-in Cache-Control = %q, want no-store", cc)
	}
}

func TestPublicTimeline(t *testing.T) {
	e := newTimelineEnv(t)
	res := e.get(t, "/timelines/federated?limit=2", nil)
	page := decodePage(t, res)
	want := []timelineCall{{timeline: "public", viewer: appcontent.Viewer{Kind: appcontent.ViewerAnonymous}, query: apptimeline.Query{Limit: 2}}}
	if !slices.EqualFunc(e.timelines.calls, want, equalCall) || len(page.Items) != 2 {
		t.Errorf("calls = %+v with %d items, want an anonymous page", e.timelines.calls, len(page.Items))
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want public, max-age=60", cc)
	}
	res = e.get(t, "/timelines/federated", map[string]string{"Authorization": "Bearer read"})
	decodePage(t, res)
	if c := e.timelines.calls[1]; c.viewer.AccountID != aliceAccountID || !slices.Equal(c.query.Languages, []string{"ja"}) {
		t.Errorf("logged-in call = %+v, want alice in her timeline languages", c)
	}
	if res := e.get(t, "/timelines/federated?limit=41", nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("limit=41 = %d, want 400", res.StatusCode)
	}
}

// timelineAccessEnv is a timeline environment whose instance settings are settings.
// federatedTimelineForEveryone is an instance whose federated timeline anonymous visitors may read.
func federatedTimelineForEveryone() *fakeInstance {
	return &fakeInstance{settings: []instance.CurrentSetting{{Key: instance.SettingFederatedTimelineAccess, RevisionID: testkit.NamedID("revision-1"), Value: "everyone"}}}
}

func timelineAccessEnv(t *testing.T, settings ...instance.CurrentSetting) *timelineEnv {
	te := &timelineEnv{timelines: &fakeTimelines{}, views: &fakeViews{}}
	te.authEnv = newAuthEnv(t, func(o *clientapi.Options) {
		o.Timelines, o.TimelineReader = te.timelines, te.views
		o.Instance = &fakeInstance{settings: settings}
		o.TimelineLanguages = func(context.Context, string) ([]string, error) { return []string{"ja"}, nil }
	})
	return te
}

func TestPublicTimelineCanBeTurnedOff(t *testing.T) {
	te := timelineAccessEnv(t, instance.CurrentSetting{Key: instance.SettingFederatedTimelineAccess, RevisionID: testkit.NamedID("revision-1"), Value: "off"})
	for _, auth := range []map[string]string{{"Accept-Language": "ja"}, {"Accept-Language": "ja", "Authorization": "Bearer read"}} {
		res := te.get(t, "/timelines/federated", auth)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("federated timeline turned off = %d, want 404", res.StatusCode)
		}
		if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeBlank || p.Title != "指定されたリソースが見つかりません。" {
			t.Errorf("problem = %+v", p)
		}
	}
	if len(te.timelines.calls) != 0 {
		t.Errorf("calls = %+v, want none", te.timelines.calls)
	}
	// The local timeline has its own setting.
	decodePage(t, te.get(t, "/timelines/local", nil))
}

func TestLocalTimelineCanBeTurnedOff(t *testing.T) {
	te := timelineAccessEnv(t, instance.CurrentSetting{Key: instance.SettingLocalTimelineAccess, RevisionID: testkit.NamedID("revision-1"), Value: "off"})
	for _, auth := range []map[string]string{nil, {"Authorization": "Bearer read"}} {
		if res := te.get(t, "/timelines/local", auth); res.StatusCode != http.StatusNotFound {
			t.Errorf("local timeline turned off = %d, want 404", res.StatusCode)
		}
	}
	if len(te.timelines.calls) != 0 {
		t.Errorf("calls = %+v, want none", te.timelines.calls)
	}
	// The federated timeline has its own setting.
	decodePage(t, te.get(t, "/timelines/federated", map[string]string{"Authorization": "Bearer read"}))
}

// TestTimelineDefaults checks what an instance with no stored settings serves: only the federated
// timeline needs a sign-in, and the local timeline stays open to anonymous visitors.
func TestTimelineDefaults(t *testing.T) {
	te := timelineAccessEnv(t)
	if res := te.get(t, "/timelines/federated", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous federated timeline = %d, want 401", res.StatusCode)
	}
	decodePage(t, te.get(t, "/timelines/federated", map[string]string{"Authorization": "Bearer read"}))
	decodePage(t, te.get(t, "/timelines/local", nil))
}

// TestTimelinesForLoggedInUsers checks the timelines whose access is logged_in: an anonymous
// request is a 401 that no shared cache keeps, and a logged-in user gets the same page as ever.
func TestTimelinesForLoggedInUsers(t *testing.T) {
	for _, tc := range []struct {
		path, timeline string
		key            instance.SettingKey
	}{
		{"/timelines/local", "local", instance.SettingLocalTimelineAccess},
		{"/timelines/federated", "public", instance.SettingFederatedTimelineAccess},
	} {
		t.Run(tc.timeline, func(t *testing.T) {
			te := timelineAccessEnv(t, instance.CurrentSetting{Key: tc.key, RevisionID: testkit.NamedID("revision-1"), Value: "logged_in"})
			res := te.get(t, tc.path, map[string]string{"Accept-Language": "ja"})
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("anonymous request = %d, want 401", res.StatusCode)
			}
			if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("anonymous Cache-Control = %q, want no-store", cc)
			}
			if !slices.Contains(res.Header.Values("Vary"), "Authorization") {
				t.Errorf("Vary = %q, want Authorization", res.Header.Values("Vary"))
			}
			if res.Header.Get("WWW-Authenticate") == "" {
				t.Error("WWW-Authenticate is missing")
			}
			if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeUnauthenticated {
				t.Errorf("problem = %+v, want unauthenticated", p)
			}
			if len(te.timelines.calls) != 0 {
				t.Errorf("calls = %+v, want none", te.timelines.calls)
			}

			res = te.get(t, tc.path, map[string]string{"Authorization": "Bearer read"})
			decodePage(t, res)
			want := []timelineCall{{
				timeline: tc.timeline,
				viewer:   appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: aliceAccountID},
				query:    apptimeline.Query{Languages: []string{"ja"}},
			}}
			if !slices.EqualFunc(te.timelines.calls, want, equalCall) {
				t.Errorf("logged-in calls = %+v, want %+v", te.timelines.calls, want)
			}
			if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("logged-in Cache-Control = %q, want no-store", cc)
			}
		})
	}
}

// TestDirectPostsNeedReadDirect checks that read:statuses alone does not show direct posts: the
// timeline is read for a viewer that the visibility policy then denies them (ADR-0090 §4).
func TestDirectPostsNeedReadDirect(t *testing.T) {
	for token, denied := range map[string]bool{"read-statuses": true, "read-direct": false, "read": false} {
		te := newTimelineEnv(t)
		decodePage(t, te.get(t, "/timelines/home", map[string]string{"Authorization": "Bearer " + token}))
		if len(te.timelines.calls) != 1 {
			t.Fatalf("%s: calls = %+v, want one", token, te.timelines.calls)
		}
		if got := te.timelines.calls[0].viewer.DirectDenied; got != denied {
			t.Errorf("%s: DirectDenied = %v, want %v", token, got, denied)
		}
	}
}

func TestHashtagTimeline(t *testing.T) {
	e := newTimelineEnv(t)
	page := decodePage(t, e.get(t, "/timelines/tag/nobody-uses-this", nil))
	if len(page.Items) != 0 || page.NextCursor != nil || len(e.timelines.calls) != 0 {
		t.Errorf("unknown hashtag = %+v after %+v, want an empty page without reading a timeline", page, e.timelines.calls)
	}
	page = decodePage(t, e.get(t, "/timelines/tag/CATS", nil))
	if len(e.timelines.calls) != 1 || e.timelines.calls[0].arg != tagID || len(page.Items) != 2 {
		t.Errorf("hashtag CATS = %d items after %+v, want the timeline of #cats", len(page.Items), e.timelines.calls)
	}
}

func TestListAccountStatuses(t *testing.T) {
	e := newTimelineEnv(t)
	auth := map[string]string{"Authorization": "Bearer read"}
	decodePage(t, e.get(t, "/accounts/"+aliceAccountID+"/statuses?exclude_replies=true&exclude_boosts=true&limit=5", auth))
	want := []timelineCall{{
		timeline: "profile", viewer: appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: aliceAccountID}, arg: aliceAccountID,
		filter: apptimeline.ProfileFilter{ExcludeReplies: true, ExcludeBoosts: true}, query: apptimeline.Query{Limit: 5},
	}}
	if !slices.EqualFunc(e.timelines.calls, want, equalCall) {
		t.Errorf("calls = %+v, want %+v (a profile shows every language)", e.timelines.calls, want)
	}

	e.views.pinned = []string{postID}
	page := decodePage(t, e.get(t, "/accounts/"+aliceAccountID+"/statuses?pinned=true", nil))
	if len(page.Items) != 1 || page.Items[0].Id != postID || page.NextCursor != nil || len(e.timelines.calls) != 1 {
		t.Errorf("pinned = %+v, want the pinned post in one page", page)
	}
	page = decodePage(t, e.get(t, "/accounts/"+aliceAccountID+"/statuses?pinned=true&cursor="+postID, nil))
	if len(page.Items) != 0 || page.NextCursor != nil {
		t.Errorf("pinned after the first page = %+v, want none", page)
	}

	res := e.get(t, "/accounts/01890a5d-ac96-774b-bcce-b302099a80ff/statuses", map[string]string{"Accept-Language": "ja"})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown account = %d, want 404", res.StatusCode)
	}
	if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeBlank || p.Title != "指定されたリソースが見つかりません。" ||
		res.Header.Get("Content-Language") != "ja" || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("unknown account problem = %+v %v", p, res.Header)
	}
}

// The first page of a profile that a logged-in viewer opens starts fetching the account's recent
// posts, and the page tells when it did; later pages, pinned posts, local accounts, and anonymous
// viewers start nothing, and a failure to start does not fail the page (ADR-0044).
func TestListAccountStatusesStartsFetchingRecentPosts(t *testing.T) {
	e := newTimelineEnv(t)
	auth := map[string]string{"Authorization": "Bearer read"}
	remote := remoteActor.Account.ID
	decode := func(res *http.Response) clientapi.AccountStatusPage {
		t.Helper()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var p clientapi.AccountStatusPage
		if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := decode(e.get(t, "/accounts/"+remote+"/statuses", auth)); p.RemoteFetchStarted != nil || len(p.Items) != 2 {
		t.Errorf("page without a started fetch = %+v, want no remote_fetch_started", p)
	}
	e.outboxes.start = true
	if p := decode(e.get(t, "/accounts/"+remote+"/statuses?exclude_replies=true", auth)); p.RemoteFetchStarted == nil || !*p.RemoteFetchStarted {
		t.Errorf("page that started a fetch = %+v, want remote_fetch_started", p)
	}
	want := [][2]string{{aliceAccountID, remote}, {aliceAccountID, remote}}
	for _, path := range []string{"/statuses?cursor=" + postID, "/statuses?pinned=true"} {
		if p := decode(e.get(t, "/accounts/"+remote+path, auth)); p.RemoteFetchStarted != nil {
			t.Errorf("%s = %+v, want no fetch", path, p)
		}
	}
	if p := decode(e.get(t, "/accounts/"+remote+"/statuses", nil)); p.RemoteFetchStarted != nil {
		t.Errorf("anonymous page = %+v, want no fetch", p)
	}
	// A local account has no outbox to fetch.
	if p := decode(e.get(t, "/accounts/"+bobAccountID+"/statuses", auth)); p.RemoteFetchStarted != nil {
		t.Errorf("local profile = %+v, want no fetch", p)
	}
	if !slices.Equal(e.outboxes.calls, want) {
		t.Errorf("fetches = %v, want %v: the first pages of the logged-in viewer only", e.outboxes.calls, want)
	}
	e.outboxes.err = errors.New("queue down")
	if p := decode(e.get(t, "/accounts/"+remote+"/statuses", auth)); p.RemoteFetchStarted != nil || len(p.Items) != 2 {
		t.Errorf("page while the fetch fails = %+v, want the stored posts", p)
	}
}

func TestTimelinesRejectLimitsOutOfRange(t *testing.T) {
	e := newTimelineEnv(t)
	for _, path := range []string{"/timelines/local?limit=0", "/timelines/tag/cats?limit=41", "/accounts/" + aliceAccountID + "/statuses?limit=41"} {
		res := e.get(t, path, nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", path, res.StatusCode)
		}
	}
	if len(e.timelines.calls) != 0 {
		t.Errorf("calls = %+v, want none", e.timelines.calls)
	}
}
