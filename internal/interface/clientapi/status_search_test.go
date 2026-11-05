// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeStatusSearch answers every search with page or err, and records the queries.
type fakeStatusSearch struct {
	page    search.Page
	err     error
	queries []search.Query
	viewers []appcontent.Viewer
}

func (f *fakeStatusSearch) Visibilities(context.Context) ([]search.Visibility, error) {
	return []search.Visibility{search.VisibilityPublic}, nil
}

func (f *fakeStatusSearch) Search(_ context.Context, viewer appcontent.Viewer, q search.Query) (search.Page, error) {
	f.queries = append(f.queries, q)
	f.viewers = append(f.viewers, viewer)
	return f.page, f.err
}

func newStatusSearchEnv(t *testing.T, f *fakeStatusSearch, tweak func(*clientapi.Options)) *authEnv {
	t.Helper()
	return newAuthEnv(t, func(o *clientapi.Options) {
		o.FullTextSearch = f
		if tweak != nil {
			tweak(o)
		}
	})
}

func searchPath(params ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(params); i += 2 {
		v.Set(params[i], params[i+1])
	}
	return "/statuses/search?" + v.Encode()
}

// TestSearchStatusesPage checks the 200: the parameters reach the search, the viewer's IDs come back
// as statuses in order, the cursor of the backend is passed on even with an empty page, and the
// response is not to be stored.
func TestSearchStatusesPage(t *testing.T) {
	te := newTimelineEnv(t)
	f := &fakeStatusSearch{page: search.Page{StatusIDs: []string{postID}, Next: "opaque next"}}
	e := newStatusSearchEnv(t, f, func(o *clientapi.Options) { o.TimelineReader = te.views })

	res := e.send(t, http.MethodGet, searchPath("q", "cat photos", "limit", "5", "cursor", "opaque prev"), "read-statuses", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("search = %d: %+v", res.StatusCode, decodeProblem(t, res))
	}
	if got := res.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	page := decode[clientapi.StatusPage](t, res)
	if len(page.Items) != 1 || page.Items[0].Id != postID {
		t.Errorf("items = %+v, want %s", page.Items, postID)
	}
	if page.NextCursor == nil || *page.NextCursor != "opaque next" {
		t.Errorf("next_cursor = %v", page.NextCursor)
	}
	if want := (search.Query{Text: "cat photos", Cursor: "opaque prev", Limit: 5}); len(f.queries) != 1 || f.queries[0] != want {
		t.Errorf("queries = %+v, want %+v", f.queries, want)
	}
	if v := f.viewers[0]; v.Kind != appcontent.ViewerLocal || v.AccountID != aliceAccountID {
		t.Errorf("viewer = %+v", v)
	}

	// A page that filtering emptied still says that more follow.
	f.page = search.Page{Next: "more"}
	res = e.send(t, http.MethodGet, searchPath("q", "cats"), "read-statuses", "")
	page = decode[clientapi.StatusPage](t, res)
	if res.StatusCode != http.StatusOK || page.Items == nil || len(page.Items) != 0 || page.NextCursor == nil || *page.NextCursor != "more" {
		t.Errorf("an empty page = %d %+v", res.StatusCode, page)
	}
	if last := f.queries[len(f.queries)-1]; last.Limit != 20 || last.Cursor != "" {
		t.Errorf("the defaults = %+v", last)
	}

	// The last page has no cursor.
	f.page = search.Page{}
	if page = decode[clientapi.StatusPage](t, e.send(t, http.MethodGet, searchPath("q", "cats"), "read-statuses", "")); page.NextCursor != nil {
		t.Errorf("the last page has next_cursor %q", *page.NextCursor)
	}
}

// TestSearchStatusesAccess checks who may search: a user's token with read:statuses, that of a
// third-party app too, and no one signed out (ADR-0149 §9).
func TestSearchStatusesAccess(t *testing.T) {
	f := &fakeStatusSearch{}
	e := newStatusSearchEnv(t, f, nil)
	for _, tc := range []struct {
		token string
		want  int
	}{
		{"", http.StatusUnauthorized},
		{"read-accounts", http.StatusForbidden},
		{"write", http.StatusForbidden},
		{"read-statuses", http.StatusOK},
		{"read", http.StatusOK},
		{"third-party", http.StatusOK},
	} {
		if res := e.send(t, http.MethodGet, searchPath("q", "cats"), tc.token, ""); res.StatusCode != tc.want {
			t.Errorf("token %q = %d, want %d", tc.token, res.StatusCode, tc.want)
		}
	}
	if len(f.queries) != 3 {
		t.Errorf("%d searches ran, want 3", len(f.queries))
	}
}

// TestSearchStatusesInvalidParameters checks the 400s, which query nothing.
func TestSearchStatusesInvalidParameters(t *testing.T) {
	f := &fakeStatusSearch{}
	e := newStatusSearchEnv(t, f, nil)
	for _, tc := range []struct {
		path string
		name string
	}{
		{searchPath("q", ""), "q"},
		{searchPath("q", strings.Repeat("語", 201)), "q"},
		{searchPath("q", "cats", "limit", "0"), "limit"},
		{searchPath("q", "cats", "limit", "41"), "limit"},
		{searchPath("q", "cats", "cursor", ""), "cursor"},
		{searchPath("q", "cats", "cursor", strings.Repeat("c", 257)), "cursor"},
	} {
		res := e.send(t, http.MethodGet, tc.path, "read-statuses", "")
		p := decodeProblem(t, res)
		if res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, tc.name) {
			t.Errorf("%.60s = %d %+v, want a 400 on %s", tc.path, res.StatusCode, p, tc.name)
		}
	}
	if len(f.queries) != 0 {
		t.Errorf("queries = %d, want none", len(f.queries))
	}
	// 200 characters are the most q has, counted as characters and not bytes.
	if res := e.send(t, http.MethodGet, searchPath("q", strings.Repeat("語", 200)), "read-statuses", ""); res.StatusCode != http.StatusOK {
		t.Errorf("200 characters = %d, want 200", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, "/statuses/search", "read-statuses", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("no q = %d, want 400", res.StatusCode)
	}

	f.err = search.ErrInvalidCursor
	res := e.send(t, http.MethodGet, searchPath("q", "cats", "cursor", "forged"), "read-statuses", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, "cursor") {
		t.Errorf("a cursor the backend refuses = %d %+v, want a 400 on cursor", res.StatusCode, p)
	}
}

// TestSearchStatusesNone checks that search = none answers with its own problem, after the checks of
// the token (ADR-0149 §2, §9).
func TestSearchStatusesNone(t *testing.T) {
	e := newAuthEnv(t, nil)
	res := e.send(t, http.MethodGet, searchPath("q", "cats"), "read-statuses", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound || p.Type != clientapi.ProblemTypeSearchUnavailable || p.Title == "" {
		t.Errorf("search = none = %d %+v", res.StatusCode, p)
	}
	if res := e.send(t, http.MethodGet, searchPath("q", "cats"), "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("signed out = %d, want 401", res.StatusCode)
	}
}

// TestSearchStatusesBackendFailure checks the 503 and what reaches the log, and that the query is no
// part of it.
func TestSearchStatusesBackendFailure(t *testing.T) {
	var logs bytes.Buffer
	f := &fakeStatusSearch{err: errors.Join(search.ErrBackendFailed, errors.New("timed out"))}
	e := newStatusSearchEnv(t, f, func(o *clientapi.Options) { o.Logger = slog.New(slog.NewTextHandler(&logs, nil)) })
	res := e.send(t, http.MethodGet, searchPath("q", "secretword"), "read-statuses", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusServiceUnavailable || p.Type != clientapi.ProblemTypeSearchFailed || p.Title == "" {
		t.Errorf("backend failure = %d %+v", res.StatusCode, p)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control of the problem = %q", got)
	}
	if !strings.Contains(logs.String(), "full-text search failed") {
		t.Errorf("logs = %q, want the failure", logs.String())
	}
	if strings.Contains(logs.String(), "secretword") {
		t.Errorf("logs hold the query: %q", logs.String())
	}

	// Any other failure is ours, not the backend's.
	f.err = errors.New("database")
	if res := e.send(t, http.MethodGet, searchPath("q", "cats"), "read-statuses", ""); res.StatusCode != http.StatusInternalServerError {
		t.Errorf("another failure = %d, want 500", res.StatusCode)
	}
}

// TestSearchStatusesRateLimit checks StatusSearchLimit: per account, and refusing before the search runs.
func TestSearchStatusesRateLimit(t *testing.T) {
	f := &fakeStatusSearch{}
	e := newStatusSearchEnv(t, f, nil)
	for i := range clientapi.StatusSearchLimit.Max {
		if res := e.send(t, http.MethodGet, searchPath("q", "cats"), "read-statuses", ""); res.StatusCode != http.StatusOK {
			t.Fatalf("search %d = %d, want 200", i, res.StatusCode)
		}
	}
	res := e.send(t, http.MethodGet, searchPath("q", "cats"), "read-statuses", "")
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Errorf("past the limit = %d Retry-After %q, want 429", res.StatusCode, res.Header.Get("Retry-After"))
	}
	if int64(len(f.queries)) != clientapi.StatusSearchLimit.Max {
		t.Errorf("%d searches ran, want %d", len(f.queries), clientapi.StatusSearchLimit.Max)
	}
	// An invalid request spends nothing, and the limit is per account.
	if res := e.send(t, http.MethodGet, searchPath("q", ""), "read-statuses", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("an invalid request past the limit = %d, want 400", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, searchPath("q", "cats"), "bob", ""); res.StatusCode != http.StatusOK {
		t.Errorf("another account = %d, want 200", res.StatusCode)
	}
	e.clock.Advance(2 * clientapi.StatusSearchLimit.Window)
	if res := e.send(t, http.MethodGet, searchPath("q", "cats"), "read-statuses", ""); res.StatusCode != http.StatusOK {
		t.Errorf("after the window = %d, want 200", res.StatusCode)
	}
}
