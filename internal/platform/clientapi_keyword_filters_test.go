// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// keywordFilter creates a keyword filter of viewer from the JSON body and returns it.
func (e *timelineAPI) keywordFilter(t *testing.T, viewer timelineUser, body string) clientapi.KeywordFilter {
	t.Helper()
	res := e.listCall(t, viewer, http.MethodPost, "/filters", body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /filters %s = %d", body, res.StatusCode)
	}
	return decodeJSON[clientapi.KeywordFilter](t, res)
}

// filteredBy returns the titles and words of the filters that st carries.
func filteredBy(st clientapi.Status) []string {
	var out []string
	for _, f := range st.Filtered {
		out = append(out, f.Filter.Title+":"+strings.Join(f.KeywordMatches, ","))
	}
	return out
}

// TestKeywordFiltersAPI drives the keyword filters on the real database: creating, reading,
// changing, and deleting them only as their owner, the limits, and what they do to the home and
// local timelines, threads, and notifications: a hide filter leaves a post out (and the pages
// keep their cursors), a warn filter marks it with its title and words, Japanese words match
// without spaces and across katakana and hiragana, the viewer's own posts never match, and an
// expired filter stops applying at once.
func TestKeywordFiltersAPI(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice := e.userWithScope(t, "alice", "read write")
	bob, carol := e.user(t, "bob"), e.userWithScope(t, "carol", "read write")
	e.follow(t, alice, bob)

	// Creating checks every field.
	for _, bad := range []string{
		`{"title":"","action":"warn","contexts":["home"],"keywords":[{"keyword":"x","whole_word":false}]}`,
		`{"title":"x","action":"warn","contexts":["home"],"keywords":[{"keyword":"   ","whole_word":false}]}`,
	} {
		if res := e.listCall(t, alice, http.MethodPost, "/filters", bad); res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("POST /filters %s = %d, want 422", bad, res.StatusCode)
		}
	}
	spoilers := e.keywordFilter(t, alice, `{"title":"Spoilers","action":"warn","contexts":["home","public","thread","notifications"],
		"keywords":[{"keyword":"Spoiler","whole_word":true},{"keyword":"ネタバレ","whole_word":true}]}`)
	ads := e.keywordFilter(t, alice, `{"title":"Ads","action":"hide","contexts":["home","public","thread","notifications"],
		"keywords":[{"keyword":"SALE","whole_word":false}]}`)
	if spoilers.Expired || spoilers.ExpiresAt != nil || len(spoilers.Keywords) != 2 || spoilers.Keywords[1].Keyword != "ネタバレ" ||
		!slices.Equal(spoilers.Contexts, []clientapi.KeywordFilterContext{"home", "notifications", "public", "thread"}) {
		t.Errorf("created %+v", spoilers)
	}
	res := e.listCall(t, alice, http.MethodGet, "/filters", "")
	if c := decodeJSON[clientapi.KeywordFilterCollection](t, res); len(c.Items) != 2 || c.Items[0].Id != spoilers.Id || c.Items[1].Id != ads.Id {
		t.Errorf("GET /filters = %+v, want both, oldest first", c.Items)
	}
	// Another user's filter does not exist for them.
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		if res := e.listCall(t, carol, method, "/filters/"+ads.Id, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s /filters/{alice's} by carol = %d, want 404", method, res.StatusCode)
		}
	}
	if res := e.listCall(t, carol, http.MethodGet, "/filters", ""); len(decodeJSON[clientapi.KeywordFilterCollection](t, res).Items) != 0 {
		t.Error("carol lists alice's filters")
	}

	// Home: the hidden post is left out, the warned ones are marked, alice's own is not.
	plain := e.post(t, bob, "hello", domcontent.VisibilityPublic, "en")
	warned := e.post(t, bob, "a big spoiler!", domcontent.VisibilityPublic, "en")
	notWhole := e.post(t, bob, "spoilers are fine", domcontent.VisibilityPublic, "en")
	hidden := e.post(t, bob, "everything on sale", domcontent.VisibilityPublic, "en")
	japanese := e.post(t, bob, "これはねたばれです", domcontent.VisibilityPublic, "ja")
	own := e.post(t, alice, "my own spoiler and sale", domcontent.VisibilityPublic, "en")
	page := e.page(t, &alice, "/timelines/home")
	if got, want := itemIDs(page), []string{own, japanese, notWhole, warned, plain}; !slices.Equal(got, want) {
		t.Fatalf("home = %v, want %v without the hidden post", got, want)
	}
	marks := map[string][]string{}
	for _, st := range page.Items {
		marks[st.Id] = filteredBy(st)
	}
	for id, want := range map[string][]string{
		warned: {"Spoilers:Spoiler"}, japanese: {"Spoilers:ネタバレ"}, notWhole: nil, plain: nil, own: nil,
	} {
		if !slices.Equal(marks[id], want) {
			t.Errorf("filtered of %s = %v, want %v", id, marks[id], want)
		}
	}
	// The page of the hidden post keeps its cursor and is empty.
	pages := e.all(t, &alice, "/timelines/home", 1, nil)
	var flat []string
	empty := 0
	for _, p := range pages {
		flat = append(flat, p...)
		if len(p) == 0 {
			empty++
		}
	}
	if !slices.Equal(flat, itemIDs(page)) || empty == 0 {
		t.Errorf("home by pages of one = %v, want %v with an empty page", pages, itemIDs(page))
	}
	// The local timeline is a public context; a visitor has no filters.
	if got := itemIDs(e.page(t, &alice, "/timelines/local")); slices.Contains(got, hidden) || !slices.Contains(got, warned) {
		t.Errorf("local = %v, want the warned post and not the hidden one", got)
	}
	if got := itemIDs(e.page(t, nil, "/timelines/local")); !slices.Contains(got, hidden) {
		t.Errorf("local for a visitor = %v, want the hidden post", got)
	}
	// A profile is not a context of either filter.
	if got := itemIDs(e.page(t, &alice, "/accounts/"+bob.accountID+"/statuses")); !slices.Contains(got, hidden) {
		t.Errorf("bob's profile = %v, want the hidden post", got)
	}

	// Threads: a hidden reply is left out, the opened post is folded even for hide.
	reply := e.reply(t, carol, warned, "sale in the replies")
	kept := e.reply(t, carol, warned, "a fine reply")
	res = e.listCall(t, alice, http.MethodGet, "/statuses/"+warned+"/context", "")
	thread := decodeJSON[clientapi.StatusContext](t, res)
	var descendants []string
	for _, st := range thread.Descendants {
		descendants = append(descendants, st.Id)
	}
	if !slices.Equal(descendants, []string{kept}) {
		t.Errorf("descendants = %v, want %s only", descendants, kept)
	}
	res = e.listCall(t, alice, http.MethodGet, "/statuses/"+reply, "")
	if st := decodeJSON[clientapi.Status](t, res); res.StatusCode != http.StatusOK || !slices.Equal(filteredBy(st), []string{"Ads:SALE"}) {
		t.Errorf("GET /statuses/{hidden reply} = %d %v, want it folded by Ads", res.StatusCode, filteredBy(st))
	}

	// Notifications: a hidden mention is neither listed nor counted unread.
	mentionHidden := e.post(t, carol, "@alice a sale for you", domcontent.VisibilityPublic, "en")
	mentionWarned := e.post(t, carol, "@alice a spoiler for you", domcontent.VisibilityPublic, "en")
	res = e.listCall(t, alice, http.MethodGet, "/notifications", "")
	notes := decodeJSON[clientapi.NotificationPage](t, res)
	var mentions []string
	for _, n := range notes.Items {
		if n.Status != nil && (n.Status.Id == mentionHidden || n.Status.Id == mentionWarned) {
			mentions = append(mentions, n.Status.Id)
			if n.Status.Id == mentionWarned && !slices.Equal(filteredBy(*n.Status), []string{"Spoilers:Spoiler"}) {
				t.Errorf("the warned mention carries %v", filteredBy(*n.Status))
			}
		}
	}
	if !slices.Equal(mentions, []string{mentionWarned}) {
		t.Errorf("notified mentions = %v, want the warned one only", mentions)
	}
	res = e.listCall(t, alice, http.MethodGet, "/notifications/marker", "")
	if m := decodeJSON[clientapi.NotificationMarker](t, res); m.UnreadCount != int64(len(notes.Items)) {
		t.Errorf("unread_count = %d, want the %d listed", m.UnreadCount, len(notes.Items))
	}

	// An expired filter stops applying at once and stays listed; a later expiry applies it again.
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	res = e.listCall(t, alice, http.MethodPatch, "/filters/"+ads.Id, fmt.Sprintf(
		`{"title":"Ads","action":"hide","contexts":["home"],"keywords":[{"keyword":"sale","whole_word":false}],"expires_at":%q}`, past))
	if f := decodeJSON[clientapi.KeywordFilter](t, res); res.StatusCode != http.StatusOK || !f.Expired || f.ExpiresAt == nil {
		t.Fatalf("PATCH /filters/{ads} = %d %+v, want it expired", res.StatusCode, f)
	}
	if got := itemIDs(e.page(t, &alice, "/timelines/home")); !slices.Contains(got, hidden) {
		t.Errorf("home after the expiry = %v, want the post back", got)
	}
	res = e.listCall(t, alice, http.MethodGet, "/filters/"+ads.Id, "")
	if f := decodeJSON[clientapi.KeywordFilter](t, res); res.StatusCode != http.StatusOK || !f.Expired {
		t.Errorf("GET /filters/{expired} = %d %+v, want it listed as expired", res.StatusCode, f)
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	res = e.listCall(t, alice, http.MethodPatch, "/filters/"+ads.Id, fmt.Sprintf(
		`{"title":"Ads","action":"hide","contexts":["home"],"keywords":[{"keyword":"sale","whole_word":false}],"expires_at":%q}`, future))
	if f := decodeJSON[clientapi.KeywordFilter](t, res); res.StatusCode != http.StatusOK || f.Expired {
		t.Fatalf("PATCH /filters/{ads} with a later expiry = %d %+v, want it active", res.StatusCode, f)
	}
	if got := itemIDs(e.page(t, &alice, "/timelines/home")); slices.Contains(got, hidden) {
		t.Errorf("home after the extension = %v, want the post hidden again", got)
	}
	if res := e.listCall(t, alice, http.MethodDelete, "/filters/"+ads.Id, ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE /filters/{ads} = %d", res.StatusCode)
	}
	if res := e.listCall(t, alice, http.MethodGet, "/filters/"+ads.Id, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET /filters/{deleted} = %d, want 404", res.StatusCode)
	}

	// At most timeline.MaxKeywordFilters filters.
	for i := range timeline.MaxKeywordFilters - 1 {
		e.keywordFilter(t, carol, fmt.Sprintf(`{"title":"f%d","action":"warn","contexts":["home"],"keywords":[{"keyword":"w%d","whole_word":false}]}`, i, i))
	}
	e.keywordFilter(t, carol, `{"title":"last","action":"warn","contexts":["home"],"keywords":[{"keyword":"last","whole_word":false}]}`)
	res = e.listCall(t, carol, http.MethodPost, "/filters", `{"title":"more","action":"warn","contexts":["home"],"keywords":[{"keyword":"more","whole_word":false}]}`)
	if p := decodeJSON[clientapi.Problem](t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeTooManyKeywordFilters {
		t.Errorf("a filter beyond the limit = %d %+v, want 422 too-many-keyword-filters", res.StatusCode, p)
	}
}

// reply posts text by author in reply to statusID and returns its ID.
func (e *timelineAPI) reply(t *testing.T, author timelineUser, statusID, text string) string {
	t.Helper()
	res := e.listCall(t, author, http.MethodPost, "/statuses", fmt.Sprintf(`{"text":%q,"in_reply_to_id":%q,"visibility":"public"}`, text, statusID))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /statuses (reply) = %d", res.StatusCode)
	}
	return decodeJSON[clientapi.Status](t, res).Id
}

// TestStreamingAppliesKeywordFilters checks that the home stream never sends a post that a hide
// filter matches, and sends a warned one with its marks.
func TestStreamingAppliesKeywordFilters(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob := e.userWithScope(t, "alice", "read write"), e.user(t, "bob")
	e.follow(t, alice, bob)
	e.keywordFilter(t, alice, `{"title":"Ads","action":"hide","contexts":["home","public"],"keywords":[{"keyword":"sale","whole_word":false}]}`)
	e.keywordFilter(t, alice, `{"title":"Cats","action":"warn","contexts":["home","public"],"keywords":[{"keyword":"猫","whole_word":true}]}`)

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	ctx, disconnect := context.WithCancel(t.Context())
	defer disconnect()
	streams := map[string]<-chan sseEvent{
		"home":  openTimelineStream(ctx, t, e, srv, alice, "/streaming"),
		"local": openTimelineStream(ctx, t, e, srv, alice, "/streaming/timelines/local"),
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.app.Streams.Len() != len(streams) {
		if time.Now().After(deadline) {
			t.Fatal("the streams did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}

	e.post(t, bob, "a sale", domcontent.VisibilityPublic, "en")
	cats := e.post(t, bob, "子猫の写真", domcontent.VisibilityPublic, "ja")
	for name, events := range streams {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("the %s stream ended", name)
			}
			var st clientapi.Status
			if err := json.Unmarshal(ev.data, &st); err != nil {
				t.Fatal(err)
			}
			e.spec.ValidateSchema(t, "Status", ev.data)
			if ev.name != "update" || st.Id != cats || !slices.Equal(filteredBy(st), []string{"Cats:猫"}) {
				t.Errorf("the %s stream sent %s %s %v, want the warned post first", name, ev.name, st.Id, filteredBy(st))
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no event on the %s stream within 5 seconds", name)
		}
	}
}
