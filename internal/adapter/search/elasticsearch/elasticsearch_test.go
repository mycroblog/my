// SPDX-License-Identifier: AGPL-3.0-only

package elasticsearch

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/search"
)

const (
	testAPIKey   = "dGVzdC1rZXktaWQ6dGVzdC1rZXktc2VjcmV0" //nolint:gosec // G101: a made-up key of the test
	testUser     = "my-user"
	testPassword = "a-password-nobody-may-see"
)

// request is a request that the stand-in received.
type request struct {
	Method, Path, Query, Auth string
	Body                      map[string]any
}

// standIn answers each request with the response of reply, or 200 {} when reply is nil.
type standIn struct {
	mu       sync.Mutex
	requests []request
	reply    func(r request) (int, string)
}

func (s *standIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	req := request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization")}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &req.Body)
	}
	s.mu.Lock()
	s.requests = append(s.requests, req)
	reply := s.reply
	s.mu.Unlock()
	status, body := http.StatusOK, "{}"
	if reply != nil {
		status, body = reply(req)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func newStandIn(t *testing.T, o Options) (*Index, *standIn) {
	t.Helper()
	s := &standIn{}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	o.URL = srv.URL + "/proxy/"
	o.HTTPClient = srv.Client()
	if o.Index == "" {
		o.Index = "my-statuses"
	}
	x, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return x, s
}

func TestNewRefusesOptions(t *testing.T) {
	client := &http.Client{}
	for _, o := range []Options{
		{Index: "x", HTTPClient: client},
		{URL: "https://es.example", Index: "x"},
		{URL: "https://es.example", Index: "x", HTTPClient: client, APIKey: "k", Username: "u", Password: "p"},
		{URL: "https://es.example", Index: "x", HTTPClient: client, Username: "u"},
		{URL: "https://es.example", Index: "x", HTTPClient: client, Password: "p"},
		{URL: "https://u:" + testPassword + "@es.example", Index: "x", HTTPClient: client},
		{URL: "ftp://es.example", Index: "x", HTTPClient: client},
		{URL: "https://es.example", Index: "Upper", HTTPClient: client},
		{URL: "https://es.example", Index: "_x", HTTPClient: client},
	} {
		_, err := New(o)
		if err == nil {
			t.Errorf("New(%+v) succeeded, want an error", o)
		} else if strings.Contains(err.Error(), testPassword) {
			t.Errorf("New: %v quotes the password", err)
		}
	}
}

func TestCheckIndexName(t *testing.T) {
	for _, name := range []string{"my-statuses", "a", "my_index.v2", "0x"} {
		if err := CheckIndexName(name); err != nil {
			t.Errorf("CheckIndexName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "-a", "_a", "+a", ".a", "A", "a b", "a*", "a/b", "a,b", "a#", strings.Repeat("a", 201)} {
		if CheckIndexName(name) == nil {
			t.Errorf("CheckIndexName(%q) accepted it", name)
		}
	}
}

func TestCredentials(t *testing.T) {
	x, s := newStandIn(t, Options{APIKey: testAPIKey})
	if err := x.Remove(t.Context(), "s1"); err != nil {
		t.Fatal(err)
	}
	if got := s.requests[0].Auth; got != "ApiKey "+testAPIKey {
		t.Errorf("Authorization = %q, want the API key", got)
	}

	x, s = newStandIn(t, Options{Username: testUser, Password: testPassword})
	if err := x.Remove(t.Context(), "s1"); err != nil {
		t.Fatal(err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(testUser+":"+testPassword))
	if got := s.requests[0].Auth; got != want {
		t.Errorf("Authorization = %q, want the basic authentication", got)
	}

	x, s = newStandIn(t, Options{})
	if err := x.Remove(t.Context(), "s1"); err != nil {
		t.Fatal(err)
	}
	if got := s.requests[0].Auth; got != "" {
		t.Errorf("Authorization = %q without credentials, want none", got)
	}
}

func TestErrorsHoldNoSecret(t *testing.T) {
	const reason = "failed to parse [text] near alpha secret words of the post"
	for _, o := range []Options{{APIKey: testAPIKey}, {Username: testUser, Password: testPassword}} {
		x, s := newStandIn(t, o)
		s.reply = func(request) (int, string) {
			return http.StatusBadRequest, `{"error":{"type":"mapper_parsing_exception","reason":"` + reason + `"},"status":400}`
		}
		errs := []error{
			x.Replace(t.Context(), search.Document{StatusID: "s1", AuthorID: "a", Visibility: search.VisibilityPublic, Text: "secret words"}),
			x.RemoveByAuthor(t.Context(), "a"),
			x.Clear(t.Context()),
		}
		_, err := x.Search(t.Context(), search.Query{Text: "secret words", Limit: 2})
		errs = append(errs, err)
		for _, err := range errs {
			if err == nil {
				t.Fatal("a call succeeded on an error response")
			}
			msg := err.Error()
			for _, secret := range []string{testAPIKey, testPassword, testUser, "secret words", reason} {
				if strings.Contains(msg, secret) {
					t.Errorf("the error %q holds %q", msg, secret)
				}
			}
			if !strings.Contains(msg, "mapper_parsing_exception") {
				t.Errorf("the error %q lacks the type of the engine's error", msg)
			}
		}
	}
}

func TestTransportErrorHoldsNoSecret(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	x, err := New(Options{URL: srv.URL + "/a-path", Index: "x", Username: testUser, Password: testPassword, HTTPClient: &http.Client{}})
	if err != nil {
		t.Fatal(err)
	}
	err = x.Remove(t.Context(), "s1")
	if err == nil {
		t.Fatal("Remove on a closed server succeeded")
	}
	if strings.Contains(err.Error(), testPassword) || strings.Contains(err.Error(), "http://") {
		t.Errorf("the error %q holds the password or the URL", err)
	}
}

func TestErrorType(t *testing.T) {
	for body, want := range map[string]string{
		`{"error":{"type":"index_not_found_exception","reason":"x"}}`: "index_not_found_exception",
		`{"error":"a plain string"}`:                                  "",
		`{"error":{"type":"Has Spaces <b>"}}`:                         "",
		`not json`:                                                    "",
		`{"_index":"x","result":"not_found"}`:                         "",
	} {
		if got := errorType([]byte(body)); got != want {
			t.Errorf("errorType(%s) = %q, want %q", body, got, want)
		}
	}
}

func TestReplace(t *testing.T) {
	x, s := newStandIn(t, Options{})
	doc := search.Document{StatusID: "0190b000-0000-7000-8000-000000000001", AuthorID: "a1", Visibility: search.VisibilityLocal, Language: "ja", Text: "本文"}
	if err := x.Replace(t.Context(), doc); err != nil {
		t.Fatal(err)
	}
	r := s.requests[0]
	if r.Method != http.MethodPut || r.Path != "/proxy/my-statuses/_doc/"+doc.StatusID || r.Query != "require_alias=true" {
		t.Errorf("Replace sent %s %s?%s", r.Method, r.Path, r.Query)
	}
	want := map[string]any{"status_id": doc.StatusID, "author_id": "a1", "visibility": "local", "text": "本文"}
	if b, _ := json.Marshal(r.Body); string(b) != mustJSON(want) {
		t.Errorf("Replace sent %s, want %s", b, mustJSON(want))
	}
	if err := x.Replace(t.Context(), search.Document{StatusID: "s", Visibility: "unlisted"}); err == nil {
		t.Error("Replace of an unlisted document succeeded")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestRemove(t *testing.T) {
	x, s := newStandIn(t, Options{})
	s.reply = func(request) (int, string) {
		return http.StatusNotFound, `{"_index":"my-statuses-v1","result":"not_found"}`
	}
	if err := x.Remove(t.Context(), "s/1"); err != nil {
		t.Errorf("Remove of a missing document: %v", err)
	}
	if got := s.requests[0].Path; got != "/proxy/my-statuses/_doc/s%2F1" && got != "/proxy/my-statuses/_doc/s/1" {
		t.Errorf("Remove sent the path %s", got)
	}
	s.reply = func(request) (int, string) {
		return http.StatusNotFound, `{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`
	}
	if err := x.Remove(t.Context(), "s1"); err == nil {
		t.Error("Remove on a missing index succeeded")
	}
}

func TestRemoveByRefreshesFirst(t *testing.T) {
	x, s := newStandIn(t, Options{})
	if err := x.RemoveByVisibility(t.Context(), search.VisibilityLocal); err != nil {
		t.Fatal(err)
	}
	if len(s.requests) != 2 || s.requests[0].Path != "/proxy/my-statuses/_refresh" || s.requests[1].Path != "/proxy/my-statuses/_delete_by_query" {
		t.Fatalf("RemoveByVisibility sent %+v, want a refresh and a delete by query", s.requests)
	}
	if got, want := mustJSON(s.requests[1].Body), `{"query":{"term":{"visibility":"local"}}}`; got != want {
		t.Errorf("the delete by query sent %s, want %s", got, want)
	}
	if !strings.Contains(s.requests[1].Query, "conflicts=proceed") {
		t.Errorf("the delete by query has the parameters %s, want conflicts=proceed", s.requests[1].Query)
	}
}

func TestSearchRequest(t *testing.T) {
	x, s := newStandIn(t, Options{})
	s.reply = func(request) (int, string) {
		return http.StatusOK, `{"hits":{"hits":[{"_id":"s3"},{"_id":"s2"},{"_id":"s1"}]}}`
	}
	page, err := x.Search(t.Context(), search.Query{Text: ` alpha  "bravo*  OR　全文`, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(page.StatusIDs) != `["s3","s2"]` || page.Next == "" {
		t.Errorf("Search = %+v, want two IDs and a cursor", page)
	}
	body := s.requests[0].Body
	want := `{"_source":false,"query":{"bool":{"filter":[` +
		`{"match_phrase":{"text":{"query":"alpha"}}},{"match_phrase":{"text":{"query":"\"bravo*"}}},` +
		`{"match_phrase":{"text":{"query":"OR"}}},{"match_phrase":{"text":{"query":"全文"}}}]}},` +
		`"size":3,"sort":[{"status_id":"desc"}],"track_total_hits":false}`
	if got := mustJSON(body); got != want {
		t.Errorf("Search sent\n%s\nwant\n%s", got, want)
	}

	if _, err := x.Search(t.Context(), search.Query{Text: "alpha", Cursor: page.Next, Limit: 2}); err != nil {
		t.Fatal(err)
	}
	if got := mustJSON(s.requests[1].Body["search_after"]); got != `["s2"]` {
		t.Errorf("the next page searched after %s, want [\"s2\"]", got)
	}

	s.reply = func(request) (int, string) { return http.StatusOK, `{"hits":{"hits":[{"_id":"s1"}]}}` }
	if page, err := x.Search(t.Context(), search.Query{Text: "alpha", Limit: 2}); err != nil || page.Next != "" {
		t.Errorf("the last page = %+v, %v, want no cursor", page, err)
	}

	n := len(s.requests)
	for _, q := range []search.Query{{Text: " \t", Limit: 2}, {Text: "alpha", Limit: 0}} {
		if page, err := x.Search(t.Context(), q); err != nil || len(page.StatusIDs) != 0 {
			t.Errorf("Search(%+v) = %+v, %v, want nothing", q, page, err)
		}
	}
	if len(s.requests) != n {
		t.Error("a search with no word called the engine")
	}
}

func TestSearchCursor(t *testing.T) {
	x, _ := newStandIn(t, Options{})
	for _, cursor := range []string{"not a cursor", "%%%", "\x00", base64.RawURLEncoding.EncodeToString([]byte("before:s1")),
		base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix)), base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + "\x01"))} {
		if _, err := x.Search(t.Context(), search.Query{Text: "alpha", Cursor: cursor, Limit: 2}); !errors.Is(err, search.ErrInvalidCursor) {
			t.Errorf("Search with the cursor %q: %v, want ErrInvalidCursor", cursor, err)
		}
	}
	if id, err := decodeCursor(encodeCursor("0190b000-0000-7000-8000-000000000001")); err != nil || id != "0190b000-0000-7000-8000-000000000001" {
		t.Errorf("a cursor of the adapter decodes to %q, %v", id, err)
	}
}

func TestSearchFailure(t *testing.T) {
	x, s := newStandIn(t, Options{})
	s.reply = func(request) (int, string) { return http.StatusOK, `not json` }
	if _, err := x.Search(t.Context(), search.Query{Text: "alpha", Limit: 2}); err == nil || strings.Contains(err.Error(), "not json") {
		t.Errorf("Search on a broken response: %v", err)
	}
}
