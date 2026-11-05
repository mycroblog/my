// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// plant writes a document straight into the index, as an index that lags behind or was never told
// would hold it.
func (e *searchEnv) plant(text string, ids ...string) {
	e.t.Helper()
	for _, id := range ids {
		if err := e.index.Replace(e.t.Context(), search.Document{StatusID: id, Visibility: search.VisibilityPublic, Text: text}); err != nil {
			e.t.Fatal(err)
		}
	}
}

func (e *searchEnv) search(token, q, cursor string, limit int) clientapi.StatusPage {
	e.t.Helper()
	v := url.Values{"q": {q}}
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	var page clientapi.StatusPage
	e.want(e.do(http.MethodGet, "/statuses/search?"+v.Encode(), token, nil, &page), http.StatusOK, "GET /statuses/search")
	return page
}

func (e *searchEnv) wantSearch(token, q string, want ...string) {
	e.t.Helper()
	page := e.search(token, q, "", 0)
	if got := statusIDs(page); !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
		e.t.Errorf("search %q = %v, want %v", q, got, want)
	}
}

// TestSearchStatusesNoneOverHTTP checks the whole server with search = none.
func TestSearchStatusesNoneOverHTTP(t *testing.T) {
	e := newAccountsEnv(t)
	alice := e.user("alice")
	if typ := e.want(e.do(http.MethodGet, "/statuses/search?q=cats", alice.token, nil, nil), http.StatusNotFound, "search"); typ != clientapi.ProblemTypeSearchUnavailable {
		t.Errorf("type = %q", typ)
	}
	e.want(e.do(http.MethodGet, "/statuses/search?q=cats", "", nil, nil), http.StatusUnauthorized, "signed out")
}

// TestSearchStatusesNeverShowsWhatItMustNot checks ADR-0149 §8 end to end: whatever the index holds,
// only what the database says the viewer may see comes back, in the database's terms and with the
// backend's order.
func TestSearchStatusesNeverShowsWhatItMustNot(t *testing.T) {
	e := newSearchEnv(t)
	ctx := t.Context()
	viewer, alice, carol := e.user("viewer"), e.user("alice"), e.user("carol")
	e.setIndexable(alice, true)

	public := e.post(alice, "needle one", "public")
	if got := e.found("needle"); !slices.Equal(got, []string{public}) {
		t.Fatalf("the index holds %v, want %v", got, []string{public})
	}
	e.wantSearch(viewer.token, "needle", public)

	// Never in the index with the jobs, but planted: they stay out whoever asks, the author included.
	unlisted := e.post(alice, "planted", "unlisted")
	private := e.post(alice, "planted", "private")
	direct := e.post(alice, "planted", "direct")
	e.drain()
	e.plant("planted", unlisted, private, direct, "01890a5d-ac96-774b-bcce-b302099a8058")
	e.wantSearch(viewer.token, "planted")
	e.wantSearch(alice.token, "planted")

	// Deleted after indexing, with the job of the deletion not having run.
	deleted := e.post(alice, "gone", "public")
	e.drain()
	e.want(e.do(http.MethodDelete, "/statuses/"+deleted, alice.token, nil, nil), http.StatusNoContent, "DELETE")
	e.wantSearch(viewer.token, "gone")

	// An author who turned indexable off, while the index still has the post.
	kept := e.post(alice, "optout", "public")
	e.drain()
	e.setIndexable(alice, false)
	e.wantSearch(viewer.token, "optout")
	e.setIndexable(alice, true)
	e.wantSearch(viewer.token, "optout", kept)

	// An author who never turned it on.
	never := e.post(carol, "neverindexed", "public")
	e.drain()
	e.plant("neverindexed", never)
	e.wantSearch(viewer.token, "neverindexed")

	// A visibility the setting does not include, or no longer does.
	local := e.post(alice, "narrowed", "local")
	e.drain()
	e.plant("narrowed", local)
	e.wantSearch(viewer.token, "narrowed")
	e.setVisibilities("public,local")
	e.wantSearch(viewer.token, "narrowed", local)
	e.setVisibilities("public")
	e.plant("narrowed", local)
	e.wantSearch(viewer.token, "narrowed")

	// Blocks and mutes go by the viewer.
	e.want(e.do(http.MethodPost, "/accounts/"+alice.id+"/mute", viewer.token, map[string]any{}, nil), http.StatusOK, "mute")
	e.wantSearch(viewer.token, "needle")
	e.wantSearch(alice.token, "needle", public)
	e.want(e.do(http.MethodDelete, "/accounts/"+alice.id+"/mute", viewer.token, nil, nil), http.StatusOK, "unmute")
	e.wantSearch(viewer.token, "needle", public)
	e.want(e.do(http.MethodPost, "/accounts/"+viewer.id+"/block", alice.token, nil, nil), http.StatusOK, "block")
	e.wantSearch(viewer.token, "needle")
	e.want(e.do(http.MethodDelete, "/accounts/"+viewer.id+"/block", alice.token, nil, nil), http.StatusOK, "unblock")
	e.wantSearch(viewer.token, "needle", public)

	// A moderation action hides the author at once, with the job not having run.
	if _, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: alice.id, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategoryOther, Duration: time.Hour,
	}); err != nil {
		t.Fatal(err)
	}
	e.wantSearch(viewer.token, "needle")
	e.wantSearch(alice.token, "needle")
}

// TestSearchStatusesMutedDomain checks a remote author and the viewer's mute of a parent domain.
func TestSearchStatusesMutedDomain(t *testing.T) {
	e := newSearchEnv(t)
	viewer := e.user("viewer")
	const uri = "https://mute.remote.example/users/bob"
	bob := e.remoteActor(uri, "mute.remote.example", "bob", true)
	id := e.remoteStatus(uri, "1", bob, "mute.remote.example", "<p>federated</p>")
	if got := e.found("federated"); !slices.Equal(got, []string{id}) {
		t.Fatalf("the index holds %v", got)
	}
	e.wantSearch(viewer.token, "federated", id)
	if _, err := e.app.Graph.MuteDomain(t.Context(), viewer.id, "remote.example"); err != nil {
		t.Fatal(err)
	}
	e.wantSearch(viewer.token, "federated")
	if err := e.app.Graph.UnmuteDomain(t.Context(), viewer.id, "remote.example"); err != nil {
		t.Fatal(err)
	}
	e.wantSearch(viewer.token, "federated", id)
}

// TestSearchStatusesPaging checks that a page filtering cut short keeps the cursor, down to an empty
// page with one, and that following the cursors finds everything once, newest first.
func TestSearchStatusesPaging(t *testing.T) {
	e := newSearchEnv(t)
	viewer, alice := e.user("viewer"), e.user("alice")
	e.setIndexable(alice, true)
	var visible []string
	for range 2 {
		visible = append(visible, e.post(alice, "paged", "public"))
	}
	e.drain()
	// Two pages of limit 2 that the policy empties, between the old posts and the newest.
	var hidden []string
	for range 4 {
		hidden = append(hidden, e.post(alice, "other", "unlisted"))
	}
	newest := e.post(alice, "paged", "public")
	e.drain()
	e.plant("paged", hidden...)

	var got []string
	var lengths []int
	cursor := ""
	for range 10 {
		page := e.search(viewer.token, "paged", cursor, 2)
		got = append(got, statusIDs(page)...)
		lengths = append(lengths, len(page.Items))
		if page.NextCursor == nil {
			cursor = ""
			break
		}
		cursor = *page.NextCursor
	}
	if cursor != "" {
		t.Fatal("the cursors did not end")
	}
	want := []string{newest, visible[1], visible[0]}
	if !slices.Equal(got, want) {
		t.Errorf("found %v, want %v", got, want)
	}
	if !slices.Contains(lengths, 1) || !slices.Contains(lengths, 0) {
		t.Errorf("page lengths = %v, want a short page and an empty one", lengths)
	}

	e.want(e.do(http.MethodGet, "/statuses/search?q=paged&cursor=forged!", viewer.token, nil, nil), http.StatusBadRequest, "a forged cursor")
}
