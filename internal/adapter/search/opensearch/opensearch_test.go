// SPDX-License-Identifier: AGPL-3.0-only

package opensearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/mycroblog/my/internal/app/search"
)

// recorded is one request that the stand-in engine received.
type recorded struct {
	Method, Path, Query string
	Header              http.Header
	Body                map[string]any
}

// engine is an httptest stand-in of the REST API that records the requests and answers each
// with the next of its responses (200 {} when it has none).
type engine struct {
	mu        sync.Mutex
	requests  []recorded
	responses []response
}

type response struct {
	status int
	body   string
}

func (e *engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	e.mu.Lock()
	e.requests = append(e.requests, recorded{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body})
	res := response{status: http.StatusOK, body: "{}"}
	if len(e.responses) > 0 {
		res, e.responses = e.responses[0], e.responses[1:]
	}
	e.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.status)
	_, _ = io.WriteString(w, res.body)
}

func (e *engine) last(t *testing.T) recorded {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.requests) == 0 {
		t.Fatal("the engine received no request")
	}
	return e.requests[len(e.requests)-1]
}

func newTestIndex(t *testing.T, o Options, responses ...response) (*Index, *engine) {
	t.Helper()
	e := &engine{responses: responses}
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	if o.URL == "" {
		o.URL = srv.URL + "/prefix/"
	}
	o.Index = "my-statuses"
	o.HTTPClient = srv.Client()
	o.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	x, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return x, e
}

func TestNewRefusesOptions(t *testing.T) {
	ok := Options{URL: "https://search.example.org", Index: "i", HTTPClient: &http.Client{}, Logger: slog.Default()}
	signing := &AWSSigning{Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""), Region: "us-east-1", Service: ServiceManaged}
	for _, tc := range []struct {
		name string
		edit func(o *Options)
	}{
		{"no URL", func(o *Options) { o.URL = "" }},
		{"another scheme", func(o *Options) { o.URL = "ftp://search.example.org" }},
		{"credentials in the URL", func(o *Options) { o.URL = "https://user:s3cr3t@search.example.org" }},
		{"a query", func(o *Options) { o.URL = "https://search.example.org/?a=b" }},
		{"no index", func(o *Options) { o.Index = "" }},
		{"no client", func(o *Options) { o.HTTPClient = nil }},
		{"a password without a username", func(o *Options) { o.Password = "p" }},
		{"signing and a username", func(o *Options) { o.AWS, o.Username = signing, "u" }},
		{"signing without a region", func(o *Options) { o.AWS = &AWSSigning{Credentials: signing.Credentials, Service: ServiceManaged} }},
		{"signing of another service", func(o *Options) {
			o.AWS = &AWSSigning{Credentials: signing.Credentials, Region: "us-east-1", Service: "aoss"}
		}},
	} {
		o := ok
		tc.edit(&o)
		if _, err := New(o); err == nil {
			t.Errorf("%s: New succeeded", tc.name)
		} else if strings.Contains(err.Error(), "s3cr3t") || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: the error %q holds a credential", tc.name, err)
		}
	}
	if _, err := New(ok); err != nil {
		t.Errorf("New: %v", err)
	}
}

func TestReplaceWritesThroughTheAlias(t *testing.T) {
	x, e := newTestIndex(t, Options{Username: "my", Password: "hunter2"})
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("JST", 9*3600))
	err := x.Replace(t.Context(), search.Document{
		StatusID: "0190b000-0000-7000-8000-000000000001", AuthorID: "a", Visibility: search.VisibilityLocal,
		Language: "ja", PublishedAt: at, Text: "本文 alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := e.last(t)
	if r.Method != http.MethodPut || r.Path != "/prefix/my-statuses/_doc/0190b000-0000-7000-8000-000000000001" || r.Query != "require_alias=true" {
		t.Errorf("request %s %s?%s", r.Method, r.Path, r.Query)
	}
	if u, p, ok := (&http.Request{Header: r.Header}).BasicAuth(); !ok || u != "my" || p != "hunter2" {
		t.Errorf("Basic authentication %q %q %v", u, p, ok)
	}
	want := map[string]any{
		"status_id": "0190b000-0000-7000-8000-000000000001", "author_id": "a", "visibility": "local",
		"language": "ja", "published_at": "2026-01-01T18:04:05Z", "text": "本文 alpha",
	}
	if got, _ := json.Marshal(r.Body); string(got) != mustJSON(want) {
		t.Errorf("body %s, want %s", got, mustJSON(want))
	}

	if err := x.Replace(t.Context(), search.Document{StatusID: "s", Visibility: "unlisted"}); err == nil {
		t.Error("Replace of an unlisted status succeeded")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestRemove(t *testing.T) {
	t.Run("a document that is not there", func(t *testing.T) {
		x, e := newTestIndex(t, Options{}, response{http.StatusNotFound, `{"_index":"my-statuses-v1","_id":"a/b","result":"not_found"}`})
		if err := x.Remove(t.Context(), "a/b"); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if r := e.last(t); r.Method != http.MethodDelete || r.Path != "/prefix/my-statuses/_doc/a%2Fb" {
			t.Errorf("request %s %s", r.Method, r.Path)
		}
	})
	t.Run("an alias that is not there", func(t *testing.T) {
		x, _ := newTestIndex(t, Options{}, response{http.StatusNotFound, `{"error":{"type":"index_not_found_exception","reason":"no such index [my-statuses]"},"status":404}`})
		err := x.Remove(t.Context(), "a")
		var e *Error
		if !errors.As(err, &e) || e.Type != "index_not_found_exception" {
			t.Fatalf("Remove: %v, want index_not_found_exception", err)
		}
	})
}

func TestDeleteByQuery(t *testing.T) {
	x, e := newTestIndex(t, Options{})
	for _, tc := range []struct {
		run   func(context.Context) error
		query string
	}{
		{func(ctx context.Context) error { return x.RemoveByAuthor(ctx, "author") }, `{"term":{"author_id":"author"}}`},
		{func(ctx context.Context) error { return x.RemoveByVisibility(ctx, search.VisibilityLocal) }, `{"term":{"visibility":"local"}}`},
		{x.Clear, `{"match_all":{}}`},
	} {
		if err := tc.run(t.Context()); err != nil {
			t.Fatal(err)
		}
		r := e.last(t)
		// A delete by query sees only refreshed documents, so a refresh goes first.
		if before := e.requests[len(e.requests)-2]; before.Method != http.MethodPost || before.Path != "/prefix/my-statuses/_refresh" {
			t.Errorf("the request before the delete %s %s", before.Method, before.Path)
		}
		if r.Method != http.MethodPost || r.Path != "/prefix/my-statuses/_delete_by_query" || r.Query != "conflicts=proceed" {
			t.Errorf("request %s %s?%s", r.Method, r.Path, r.Query)
		}
		if got := mustJSON(r.Body["query"]); got != tc.query {
			t.Errorf("query %s, want %s", got, tc.query)
		}
	}
}

func TestSearchRequestAndPages(t *testing.T) {
	x, e := newTestIndex(t, Options{},
		response{http.StatusOK, `{"hits":{"hits":[{"_id":"3"},{"_id":"2"},{"_id":"1"}]}}`},
		response{http.StatusOK, `{"hits":{"hits":[{"_id":"1"}]}}`},
	)
	text := `alpha OR "bravo" *:* status_id:*`
	page, err := x.Search(t.Context(), search.Query{Text: text, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(page.StatusIDs) != `["3","2"]` || page.Next == "" {
		t.Fatalf("page %+v", page)
	}
	r := e.last(t)
	if r.Method != http.MethodPost || r.Path != "/prefix/my-statuses/_search" {
		t.Errorf("request %s %s", r.Method, r.Path)
	}
	// The text goes as it is into a match query, which never reads the query language.
	wantQuery := `{"bool":{"must":[{"match":{"text":{"operator":"and","query":` + mustJSON(text) + `,"zero_terms_query":"none"}}}]}}`
	if got := mustJSON(r.Body["query"]); got != wantQuery {
		t.Errorf("query %s, want %s", got, wantQuery)
	}
	if r.Body["size"] != float64(3) || mustJSON(r.Body["sort"]) != `[{"status_id":"desc"}]` || r.Body["_source"] != false {
		t.Errorf("body %v", r.Body)
	}

	next, err := x.Search(t.Context(), search.Query{Text: "alpha", Cursor: page.Next, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(next.StatusIDs) != `["1"]` || next.Next != "" {
		t.Errorf("the next page %+v", next)
	}
	if got := mustJSON(e.last(t).Body["query"].(map[string]any)["bool"].(map[string]any)["filter"]); got != `[{"range":{"status_id":{"lt":"2"}}}]` {
		t.Errorf("the filter of the next page %s", got)
	}
}

func TestSearchWithoutWordsOrWithABadCursor(t *testing.T) {
	x, e := newTestIndex(t, Options{})
	for _, text := range []string{"", " \t　"} {
		if page, err := x.Search(t.Context(), search.Query{Text: text, Limit: 10}); err != nil || len(page.StatusIDs) != 0 {
			t.Errorf("Search(%q) = %+v, %v", text, page, err)
		}
	}
	for _, cursor := range []string{"not a cursor", "%%%", "YWZ0ZXI6MQ"} {
		if _, err := x.Search(t.Context(), search.Query{Text: "alpha", Cursor: cursor, Limit: 10}); !errors.Is(err, search.ErrInvalidCursor) {
			t.Errorf("Search with the cursor %q: %v", cursor, err)
		}
	}
	if len(e.requests) != 0 {
		t.Errorf("the engine received %d requests", len(e.requests))
	}
}

// TestErrorsHoldNothingTheEngineOrTheCredentialsSay checks that an error names the operation,
// the status, and the engine's error type, and not its reason, the query, or a credential.
func TestErrorsHoldNothingTheEngineOrTheCredentialsSay(t *testing.T) {
	const secret = "hunter2"
	for _, tc := range []struct {
		res      response
		wantType string
	}{
		{response{http.StatusBadRequest, `{"error":{"type":"search_phase_execution_exception","reason":"query [secret words] failed"},"status":400}`}, "search_phase_execution_exception"},
		{response{http.StatusUnauthorized, `{"error":{"type":"security_exception","reason":"user my with password hunter2"}}`}, "security_exception"},
		{response{http.StatusNotFound, `{"error":"alias [my-statuses] missing","status":404}`}, "error"},
		{response{http.StatusBadGateway, `<html>secret words of a proxy</html>`}, ""},
		{response{http.StatusBadRequest, `{"error":{"type":"Not A Type: secret words"}}`}, ""},
	} {
		x, _ := newTestIndex(t, Options{Username: "my", Password: secret}, tc.res)
		_, err := x.Search(t.Context(), search.Query{Text: "secret words", Limit: 1})
		var e *Error
		if !errors.As(err, &e) || e.Status != tc.res.status || e.Type != tc.wantType || e.Op != "search" {
			t.Errorf("error %#v, want status %d and type %q", err, tc.res.status, tc.wantType)
			continue
		}
		if s := err.Error(); strings.Contains(s, "secret") || strings.Contains(s, secret) {
			t.Errorf("the error %q quotes the engine, the query, or the password", s)
		}
	}

	// A transport error does not quote the URL.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	x, _ := newTestIndex(t, Options{URL: closed.URL + "/very-secret-path", Username: "my", Password: secret})
	err := x.Remove(t.Context(), "status")
	if err == nil || strings.Contains(err.Error(), "very-secret-path") || strings.Contains(err.Error(), secret) {
		t.Errorf("transport error %v", err)
	}
}

func TestAWSSigning(t *testing.T) {
	now := time.Date(2026, 11, 7, 1, 2, 3, 0, time.UTC)
	x, e := newTestIndex(t, Options{AWS: &AWSSigning{
		Credentials: credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "session-token"),
		Region:      "ap-northeast-1", Service: ServiceManaged, Now: func() time.Time { return now },
	}})
	if err := x.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	r := e.last(t)
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261107/ap-northeast-1/es/aws4_request, SignedHeaders=") ||
		!strings.Contains(auth, "x-amz-content-sha256") || !strings.Contains(auth, "Signature=") {
		t.Errorf("Authorization %q", auth)
	}
	if r.Header.Get("X-Amz-Date") != "20261107T010203Z" || r.Header.Get("X-Amz-Security-Token") != "session-token" {
		t.Errorf("headers %v", r.Header)
	}
	// The hash of the body that was sent.
	if h := r.Header.Get("X-Amz-Content-Sha256"); len(h) != 64 || h == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("X-Amz-Content-Sha256 %q", h)
	}

	failing := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{}, errors.New("no EC2 IMDS role found")
	})
	y, _ := newTestIndex(t, Options{AWS: &AWSSigning{Credentials: failing, Region: "us-east-1", Service: ServiceManaged}})
	if err := y.Clear(t.Context()); err == nil || !strings.Contains(err.Error(), "retrieve the AWS credentials") {
		t.Errorf("Clear without credentials: %v", err)
	}
}

func TestEnsure(t *testing.T) {
	t.Run("creates the index and the alias", func(t *testing.T) {
		x, e := newTestIndex(t, Options{},
			response{http.StatusNotFound, `{"error":"alias [my-statuses] missing","status":404}`},
			response{http.StatusOK, `{"acknowledged":true}`},
			response{http.StatusOK, `{"acknowledged":true}`},
		)
		if err := x.Ensure(t.Context()); err != nil {
			t.Fatal(err)
		}
		reqs := e.requests
		if len(reqs) != 3 || reqs[0].Path != "/prefix/_alias/my-statuses" || reqs[1].Method != http.MethodPut || reqs[1].Path != "/prefix/my-statuses-v1" || reqs[2].Path != "/prefix/_aliases" {
			t.Fatalf("requests %+v", reqs)
		}
		if got := mustJSON(reqs[2].Body); got != `{"actions":[{"add":{"alias":"my-statuses","index":"my-statuses-v1"}}]}` {
			t.Errorf("actions %s", got)
		}
		if mustJSON(reqs[1].Body["mappings"].(map[string]any)["dynamic"]) != `"strict"` {
			t.Errorf("mapping %v", reqs[1].Body["mappings"])
		}
	})
	t.Run("does nothing when the alias points at the index", func(t *testing.T) {
		x, e := newTestIndex(t, Options{}, response{http.StatusOK, `{"my-statuses-v1":{"aliases":{"my-statuses":{}}}}`})
		if err := x.Ensure(t.Context()); err != nil || len(e.requests) != 1 {
			t.Fatalf("Ensure: %v after %d requests", err, len(e.requests))
		}
	})
	t.Run("moves the alias from an older version", func(t *testing.T) {
		x, e := newTestIndex(t, Options{}, response{http.StatusOK, `{"my-statuses-v1":{"aliases":{"my-statuses":{}}}}`},
			response{http.StatusBadRequest, `{"error":{"type":"resource_already_exists_exception"}}`})
		x.version = 2
		if err := x.Ensure(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := mustJSON(e.last(t).Body); got != `{"actions":[{"remove":{"alias":"my-statuses","index":"my-statuses-v1"}},{"add":{"alias":"my-statuses","index":"my-statuses-v2"}}]}` {
			t.Errorf("actions %s", got)
		}
	})
	t.Run("leaves a newer version", func(t *testing.T) {
		x, e := newTestIndex(t, Options{}, response{http.StatusOK, `{"my-statuses-v9":{"aliases":{"my-statuses":{}}}}`})
		if err := x.Ensure(t.Context()); err != nil || len(e.requests) != 1 {
			t.Fatalf("Ensure: %v after %d requests", err, len(e.requests))
		}
	})
	t.Run("refuses an alias on an index it did not name", func(t *testing.T) {
		x, _ := newTestIndex(t, Options{}, response{http.StatusOK, `{"someone-else":{"aliases":{"my-statuses":{}}}}`})
		if err := x.Ensure(t.Context()); err == nil {
			t.Fatal("Ensure succeeded")
		}
	})
}
