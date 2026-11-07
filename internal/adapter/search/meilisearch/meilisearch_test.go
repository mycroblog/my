// SPDX-License-Identifier: AGPL-3.0-only

package meilisearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/meilisearch"
	"github.com/mycroblog/my/internal/app/search"
)

const testKey = "secret-api-key-0123456789"

func newClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func statusID(n int) string { return fmt.Sprintf("0190b000-0000-7000-8000-%012d", n) }

// request is one request the stand-in received.
type request struct {
	Method, Path, Query, Auth string
	Body                      map[string]any
	List                      []map[string]any
}

// stub stands in for Meilisearch: it records the requests and answers each with what its fields
// say. Every task it enqueues takes polls GETs of processing before it ends with taskStatus.
type stub struct {
	t  *testing.T
	mu sync.Mutex
	// reqs are the requests received, in order.
	reqs []request
	// indexMissing makes GET /indexes/{uid} answer index_not_found.
	indexMissing bool
	// polls is how many GETs of a task answer processing first.
	polls int
	// taskStatus and taskCode are what every task ends with ("" means succeeded).
	taskStatus, taskCode string
	// settingsStatus is what the settings task ends with ("" means succeeded).
	settingsStatus string
	// fail answers every call but GET /tasks with this status and an error naming the key.
	fail int
	// hits are the hits of a search, and matchNothing leaves out their _matchesPosition.
	hits         []string
	matchNothing bool
	// searchCode answers a search with this error code and 404.
	searchCode string

	// marker is the settings hash of the marker document, empty when there is none.
	marker string

	next  int64
	tasks map[int64]*stubTask
}

type stubTask struct {
	kind  string
	polls int
}

func newStub(t *testing.T) (*stub, *meilisearch.Index) {
	s := &stub{t: t, tasks: map[int64]*stubTask{}}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	x, err := meilisearch.New(meilisearch.Options{URL: srv.URL + "/", APIKey: testKey, Index: "idx", HTTPClient: newClient()})
	if err != nil {
		t.Fatal(err)
	}
	return s, x
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	req := request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization")}
	if len(body) > 0 && body[0] == '[' {
		_ = json.Unmarshal(body, &req.List)
	} else if len(body) > 0 {
		_ = json.Unmarshal(body, &req.Body)
	}
	s.reqs = append(s.reqs, req)
	w.Header().Set("Content-Type", "application/json")
	answer := func(status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	meiliError := func(status int, code string) {
		answer(status, map[string]string{"message": "the key " + testKey + " and the query said this", "code": code, "type": "invalid_request", "link": "https://example.org"})
	}
	if id, ok := strings.CutPrefix(r.URL.Path, "/tasks/"); ok {
		var uid int64
		_, _ = fmt.Sscan(id, &uid)
		tk := s.tasks[uid]
		if tk == nil {
			meiliError(404, "task_not_found")
			return
		}
		if tk.polls < s.polls {
			tk.polls++
			answer(200, map[string]any{"uid": uid, "status": "processing"})
			return
		}
		status, code := s.taskStatus, s.taskCode
		if tk.kind == "settings" {
			status, code = s.settingsStatus, "invalid_settings"
		}
		if status == "" {
			answer(200, map[string]any{"uid": uid, "status": "succeeded", "error": nil})
			return
		}
		answer(200, map[string]any{"uid": uid, "status": status, "error": map[string]string{"message": "the document said " + testKey, "code": code}})
		return
	}
	if s.fail != 0 {
		meiliError(s.fail, "invalid_api_key")
		return
	}
	enqueue := func(kind string) {
		uid := s.next
		s.next++
		s.tasks[uid] = &stubTask{kind: kind}
		answer(202, map[string]any{"taskUid": uid, "status": "enqueued"})
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/indexes/idx/documents/my_settings":
		if s.marker == "" {
			meiliError(404, "document_not_found")
			return
		}
		answer(200, map[string]any{"settings_hash": s.marker})
	case r.Method == "POST" && r.URL.Path == "/indexes/idx/documents" && len(req.List) == 1 && req.List[0]["status_id"] == "my_settings":
		s.marker, _ = req.List[0]["settings_hash"].(string)
		enqueue("marker")
	case r.Method == "DELETE" && r.URL.Path == "/indexes/idx":
		s.indexMissing, s.marker = true, ""
		enqueue("delete")
	case r.Method == "GET" && r.URL.Path == "/indexes/idx":
		if s.indexMissing {
			meiliError(404, "index_not_found")
			return
		}
		answer(200, map[string]any{"uid": "idx", "primaryKey": "status_id"})
	case r.Method == "POST" && r.URL.Path == "/indexes":
		s.indexMissing = false
		enqueue("create")
	case r.Method == "PATCH" && r.URL.Path == "/indexes/idx/settings":
		enqueue("settings")
	case r.URL.Path == "/indexes/idx/search":
		if s.searchCode != "" {
			meiliError(404, s.searchCode)
			return
		}
		hits := []map[string]any{}
		for _, h := range s.hits {
			hit := map[string]any{"status_id": h}
			if !s.matchNothing {
				hit["_matchesPosition"] = map[string]any{"text": []any{map[string]int{"start": 0, "length": 1}}}
			} else {
				hit["_matchesPosition"] = map[string]any{}
			}
			hits = append(hits, hit)
		}
		answer(200, map[string]any{"hits": hits})
	default:
		enqueue("write")
	}
}

// writes returns the requests other than the polls of tasks and the setup of the index.
func (s *stub) writes() []request {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []request
	for _, r := range s.reqs {
		if strings.HasPrefix(r.Path, "/tasks/") || r.Path == "/indexes" || r.Path == "/indexes/idx" || r.Path == "/indexes/idx/settings" ||
			r.Path == "/indexes/idx/documents/my_settings" || len(r.List) == 1 && r.List[0]["status_id"] == "my_settings" {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (s *stub) count(method, path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.reqs {
		if r.Method == method && r.Path == path {
			n++
		}
	}
	return n
}

func doc(n int, lang, text string) search.Document {
	return search.Document{
		StatusID: statusID(n), AuthorID: "0190a000-0000-7000-8000-00000000000a", Visibility: search.VisibilityPublic,
		Language: lang, PublishedAt: time.Unix(1700000000, 0), Text: text,
	}
}

func TestReplaceCreatesTheIndexAndWaitsForTheTask(t *testing.T) {
	s, x := newStub(t)
	s.indexMissing, s.polls = true, 2
	if err := x.Replace(t.Context(), doc(1, "ja-JP", "ｶﾀｶﾅ ＡＢＣ")); err != nil {
		t.Fatal(err)
	}
	if err := x.Replace(t.Context(), doc(2, "en", "alpha")); err != nil {
		t.Fatal(err)
	}
	if n := s.count("POST", "/indexes"); n != 1 {
		t.Errorf("the index was created %d times, want 1", n)
	}
	if n := s.count("PATCH", "/indexes/idx/settings"); n != 1 {
		t.Errorf("the settings were sent %d times, want once in a process", n)
	}
	w := s.writes()
	if len(w) != 2 {
		t.Fatalf("writes = %+v", w)
	}
	r := w[0]
	if r.Method != "POST" || r.Path != "/indexes/idx/documents" || r.Query != "primaryKey=status_id" || r.Auth != "Bearer "+testKey {
		t.Errorf("the request = %s %s?%s (%s)", r.Method, r.Path, r.Query, r.Auth)
	}
	got := r.List[0]
	if got["status_id"] != statusID(1) || got["visibility"] != "public" || got["text_ja"] != "カタカナ ABC" || got["text"] != nil {
		t.Errorf("the Japanese document = %v, want its text, folded, in text_ja", got)
	}
	if got := w[1].List[0]; got["text"] != "alpha" || got["text_ja"] != nil {
		t.Errorf("the English document = %v, want its text in text", got)
	}
	// Each write polled its task until it succeeded, and the first the settings too.
	if n := s.count("GET", "/tasks/2"); n != 3 {
		t.Errorf("the task of the first write was polled %d times, want 3", n)
	}
}

// sentSettings returns the JSON of the last settings sent.
func (s *stub) sentSettings() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var settings map[string]any
	for _, r := range s.reqs {
		if r.Path == "/indexes/idx/settings" {
			settings = r.Body
		}
	}
	b, _ := json.Marshal(settings)
	return string(b)
}

func newStubWith(t *testing.T, s *stub, settings string) *meilisearch.Index {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	o := meilisearch.Options{URL: srv.URL, APIKey: testKey, Index: "idx", HTTPClient: newClient()}
	if settings != "" {
		o.Settings = []byte(settings)
	}
	x, err := meilisearch.New(o)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestSettings(t *testing.T) {
	s, x := newStub(t)
	if err := x.Remove(t.Context(), statusID(1)); err != nil {
		t.Fatal(err)
	}
	b := s.sentSettings()
	for _, want := range []string{
		`"filterableAttributes":["status_id","author_id","visibility"]`,
		`"sortableAttributes":["status_id"]`,
		`"rankingRules":["sort",`,
		`"typoTolerance":{"enabled":false}`,
		`{"attributePatterns":["text_ja"],"locales":["jpn"]}`,
		`"stopWords":null`,
		`"displayedAttributes":["*"]`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("the settings %s lack %s", b, want)
		}
	}
	if s.count("POST", "/indexes") != 0 {
		t.Error("an index that exists was created again")
	}
}

// The settings file overrides the defaults, the server's own attributes override nothing in it,
// and unchanged settings are not sent again by the next process.
func TestSettingsFile(t *testing.T) {
	s := &stub{t: t, tasks: map[int64]*stubTask{}}
	file := `{"rankingRules": ["sort", "words"], "typoTolerance": {"enabled": true}, "stopWords": ["the"],
		"localizedAttributes": [{"attributePatterns": ["text_ja"], "locales": ["jpn"]}]}`
	x := newStubWith(t, s, file)
	if err := x.Remove(t.Context(), statusID(1)); err != nil {
		t.Fatal(err)
	}
	b := s.sentSettings()
	for _, want := range []string{
		`"rankingRules":["sort","words"]`, `"typoTolerance":{"enabled":true}`, `"stopWords":["the"]`,
		`"localizedAttributes":[{"attributePatterns":["text_ja"],"locales":["jpn"]}]`, `"synonyms":null`,
		`"filterableAttributes":["status_id","author_id","visibility"]`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("the settings %s lack %s", b, want)
		}
	}
	if s.marker == "" {
		t.Fatal("the hash of the settings was not recorded")
	}

	// A new process with the same file sends nothing; one with another file sends it.
	if err := newStubWith(t, s, file).Remove(t.Context(), statusID(1)); err != nil {
		t.Fatal(err)
	}
	if n := s.count("PATCH", "/indexes/idx/settings"); n != 1 {
		t.Errorf("unchanged settings were sent %d times, want once", n)
	}
	if err := newStubWith(t, s, `{"stopWords": ["a"]}`).Remove(t.Context(), statusID(1)); err != nil {
		t.Fatal(err)
	}
	if n := s.count("PATCH", "/indexes/idx/settings"); n != 2 {
		t.Errorf("changed settings were sent %d times in all, want twice", n)
	}
	if b := s.sentSettings(); !strings.Contains(b, `"typoTolerance":{"enabled":false}`) || !strings.Contains(b, `"rankingRules":["sort","words","typo"`) {
		t.Errorf("the settings %s do not go back to the defaults for what the file no longer sets", b)
	}
}

func TestClearDeletesTheIndexAndAppliesTheSettingsAgain(t *testing.T) {
	s, x := newStub(t)
	if err := x.Replace(t.Context(), doc(1, "", "alpha")); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.polls = 2
	s.mu.Unlock()
	if err := x.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	if s.count("DELETE", "/indexes/idx") != 1 || s.count("POST", "/indexes") != 1 || s.count("PATCH", "/indexes/idx/settings") != 2 {
		t.Errorf("Clear sent %+v", s.reqs)
	}
	if s.marker == "" {
		t.Error("Clear returned before the settings of the new index were applied")
	}
	if err := x.Clear(t.Context()); err != nil {
		t.Errorf("Clear again: %v", err)
	}
}

func TestSettingsFileChecks(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := meilisearch.CheckSettingsFile(write("ok.json", `{"stopWords": ["the"], "rankingRules": ["sort", "words"]}`)); err != nil {
		t.Errorf("CheckSettingsFile(ok) = %v", err)
	}
	for name, content := range map[string]string{
		"array.json":    `["secret-value"]`,
		"broken.json":   `{"stopWords": [secret-value`,
		"two.json":      `{} {}`,
		"null.json":     `null`,
		"owned.json":    `{"filterableAttributes": ["secret-value"]}`,
		"unsorted.json": `{"rankingRules": ["words", "sort"]}`,
		"twice.json":    `{"rankingRules": ["sort", "words", "sort"]}`,
	} {
		err := meilisearch.CheckSettingsFile(write(name, content))
		if err == nil {
			t.Errorf("CheckSettingsFile(%s) = nil", name)
		} else if strings.Contains(err.Error(), "secret-value") {
			t.Errorf("CheckSettingsFile(%s) = %v, which quotes the file", name, err)
		}
		if _, err := meilisearch.New(meilisearch.Options{URL: "http://h", HTTPClient: newClient(), Settings: []byte(content)}); err == nil {
			t.Errorf("New with %s = nil", name)
		}
	}
	if err := meilisearch.CheckSettingsFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("CheckSettingsFile(missing) = nil")
	}
}

func TestAFailedSettingsTaskFailsTheWriteAndIsSentAgain(t *testing.T) {
	s, x := newStub(t)
	s.settingsStatus = "failed"
	err := x.Replace(t.Context(), doc(1, "", "alpha"))
	if e, ok := errors.AsType[*meilisearch.Error](err); !ok || e.Code != "invalid_settings" {
		t.Fatalf("Replace = %v, want the error of the settings task", err)
	}
	s.mu.Lock()
	s.settingsStatus = ""
	s.mu.Unlock()
	if err := x.Replace(t.Context(), doc(1, "", "alpha")); err != nil {
		t.Fatal(err)
	}
	if n := s.count("PATCH", "/indexes/idx/settings"); n != 2 {
		t.Errorf("the settings were sent %d times, want again after the failure", n)
	}
}

func TestAFailedTaskIsAnErrorWithItsCodeOnly(t *testing.T) {
	for _, status := range []string{"failed", "canceled"} {
		s, x := newStub(t)
		s.taskStatus, s.taskCode = status, "invalid_document_fields"
		err := x.Replace(t.Context(), doc(1, "", "alpha"))
		e, ok := errors.AsType[*meilisearch.Error](err)
		if !ok || e.Code != "invalid_document_fields" || !strings.Contains(err.Error(), status) {
			t.Errorf("Replace with a task %s = %v, want an *Error with its code", status, err)
		}
		if err != nil && (strings.Contains(err.Error(), testKey) || strings.Contains(err.Error(), "said")) {
			t.Errorf("the error %q quotes the key or Meilisearch's message", err)
		}
	}
}

func TestATaskThatDoesNotFinishInTimeIsAnError(t *testing.T) {
	s, x := newStub(t)
	s.polls = 1 << 30
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	err := x.Remove(ctx, statusID(1))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Remove = %v, want the deadline", err)
	}
}

func TestRemovalsOfAMissingIndexSucceed(t *testing.T) {
	s, x := newStub(t)
	s.taskStatus, s.taskCode = "failed", "index_not_found"
	if err := x.Remove(t.Context(), statusID(1)); err != nil {
		t.Errorf("Remove = %v", err)
	}
	if err := x.RemoveByAuthor(t.Context(), "0190a000-0000-7000-8000-00000000000a"); err != nil {
		t.Errorf("RemoveByAuthor = %v", err)
	}
	if err := x.Clear(t.Context()); err != nil {
		t.Errorf("Clear = %v", err)
	}
	if err := x.Replace(t.Context(), doc(1, "", "alpha")); err == nil {
		t.Error("Replace into a missing index = nil, want an error")
	}
}

func TestRemovalRequests(t *testing.T) {
	s, x := newStub(t)
	if err := x.Remove(t.Context(), statusID(1)); err != nil {
		t.Fatal(err)
	}
	if err := x.RemoveByAuthor(t.Context(), "0190a000-0000-7000-8000-00000000000a"); err != nil {
		t.Fatal(err)
	}
	if err := x.RemoveByVisibility(t.Context(), search.VisibilityLocal); err != nil {
		t.Fatal(err)
	}
	if err := x.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	w := s.writes()
	want := []string{
		"DELETE /indexes/idx/documents/" + statusID(1) + " <nil>",
		`POST /indexes/idx/documents/delete author_id = "0190a000-0000-7000-8000-00000000000a"`,
		`POST /indexes/idx/documents/delete visibility = "local"`,
	}
	for i, r := range w {
		got := fmt.Sprintf("%s %s %v", r.Method, r.Path, r.Body["filter"])
		if i >= len(want) || got != want[i] {
			t.Errorf("write %d = %s", i, got)
		}
	}
}

func TestWritesRefuseWhatIsNotAnID(t *testing.T) {
	_, x := newStub(t)
	bad := doc(1, "", "alpha")
	bad.AuthorID = `a" OR visibility = "public`
	if err := x.Replace(t.Context(), bad); err == nil {
		t.Error("Replace with a quote in the author ID = nil")
	}
	if err := x.RemoveByAuthor(t.Context(), `x" OR author_id != "y`); err == nil {
		t.Error("RemoveByAuthor with a quote = nil")
	}
	if err := x.Remove(t.Context(), "a/b"); err == nil {
		t.Error("Remove with a slash = nil")
	}
	unlisted := doc(1, "", "alpha")
	unlisted.Visibility = "unlisted"
	if err := x.Replace(t.Context(), unlisted); err == nil {
		t.Error("Replace of an unlisted status = nil")
	}
}

func TestErrorsNeverQuoteTheKeyOrTheMessage(t *testing.T) {
	s, x := newStub(t)
	s.fail = http.StatusUnauthorized
	for name, err := range map[string]error{
		"Replace": x.Replace(t.Context(), doc(1, "", "alpha")),
		"Search": func() error {
			_, err := x.Search(t.Context(), search.Query{Text: "private words", Limit: 2})
			return err
		}(),
	} {
		e, ok := errors.AsType[*meilisearch.Error](err)
		if !ok || e.Status != 401 || e.Code != "invalid_api_key" {
			t.Errorf("%s = %v, want an *Error of 401 invalid_api_key", name, err)
			continue
		}
		for _, secret := range []string{testKey, "said", "private", "127.0.0.1"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%s: the error %q holds %q", name, err, secret)
			}
		}
	}
}

func TestTransportErrorsDoNotQuoteTheURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	x, err := meilisearch.New(meilisearch.Options{URL: srv.URL + "/base", APIKey: testKey, HTTPClient: newClient()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.Search(t.Context(), search.Query{Text: "alpha", Limit: 1})
	if err == nil || strings.Contains(err.Error(), srv.URL+"/base") || strings.Contains(err.Error(), testKey) || !strings.Contains(err.Error(), "GET /indexes/{uid}:") {
		t.Errorf("Search = %v, want an error with the call and without the URL", err)
	}
}

func TestSearchRequest(t *testing.T) {
	s, x := newStub(t)
	s.hits = []string{statusID(5), statusID(4), statusID(3)}
	page, err := x.Search(t.Context(), search.Query{Text: `  bravo -alpha "phrase  --x -- ｶﾅ`, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.StatusIDs) != 2 || page.StatusIDs[1] != statusID(4) || page.Next == "" {
		t.Fatalf("the page = %+v, want two IDs and a cursor", page)
	}
	s.hits = []string{statusID(3)}
	last, err := x.Search(t.Context(), search.Query{Text: "bravo", Cursor: page.Next, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(last.StatusIDs) != 1 || last.Next != "" {
		t.Errorf("the last page = %+v", last)
	}
	var bodies []map[string]any
	for _, r := range s.reqs {
		if r.Path == "/indexes/idx/search" {
			bodies = append(bodies, r.Body)
		}
	}
	first := bodies[0]
	if first["q"] != "bravo alpha phrase x カナ" {
		t.Errorf("q = %q, want the words without Meilisearch's operators", first["q"])
	}
	if first["matchingStrategy"] != "all" || first["limit"] != float64(3) || first["filter"] != nil {
		t.Errorf("the first request = %v", first)
	}
	if fmt.Sprint(first["sort"]) != "[status_id:desc]" {
		t.Errorf("sort = %v", first["sort"])
	}
	if want := `status_id < "` + statusID(4) + `"`; bodies[1]["filter"] != want {
		t.Errorf("the filter of the next page = %v, want %s", bodies[1]["filter"], want)
	}
}

func TestSearchFindsNothingWithoutAWord(t *testing.T) {
	s, x := newStub(t)
	s.hits = []string{statusID(1)}
	for _, q := range []string{"", "  ", "*", "&&", "-", `"`, "+ : ?"} {
		page, err := x.Search(t.Context(), search.Query{Text: q, Limit: 2})
		if err != nil || len(page.StatusIDs) != 0 {
			t.Errorf("Search(%q) = %+v, %v, want nothing", q, page, err)
		}
	}
	if n := s.count("POST", "/indexes/idx/search"); n != 0 {
		t.Errorf("%d searches reached the engine, want none", n)
	}
	// A query that Meilisearch reads as no word returns hits that match nothing.
	s.matchNothing = true
	page, err := x.Search(t.Context(), search.Query{Text: "©", Limit: 2})
	if err != nil || len(page.StatusIDs) != 0 || page.Next != "" {
		t.Errorf("Search of a placeholder = %+v, %v, want nothing", page, err)
	}
}

func TestSearchOfAMissingIndexFindsNothing(t *testing.T) {
	s, x := newStub(t)
	s.searchCode = "index_not_found"
	page, err := x.Search(t.Context(), search.Query{Text: "alpha", Limit: 2})
	if err != nil || len(page.StatusIDs) != 0 {
		t.Errorf("Search = %+v, %v", page, err)
	}
	s.searchCode = ""
	if _, err := x.Search(t.Context(), search.Query{Text: "alpha", Limit: 2}); err != nil {
		t.Fatal(err)
	}
	if n := s.count("PATCH", "/indexes/idx/settings"); n != 2 {
		t.Errorf("the settings were sent %d times, want again after the index went missing", n)
	}
}

func TestSearchRefusesCursorsItDidNotIssue(t *testing.T) {
	_, x := newStub(t)
	for _, c := range []string{"not a cursor", "%%%", "\x00", "YWJj", "bTE6YSJi"} {
		if _, err := x.Search(t.Context(), search.Query{Text: "alpha", Cursor: c, Limit: 2}); !errors.Is(err, search.ErrInvalidCursor) {
			t.Errorf("Search with the cursor %q = %v, want ErrInvalidCursor", c, err)
		}
	}
}

func TestOptions(t *testing.T) {
	for _, u := range []string{"", "ftp://h", "http://", "http://user:pw@h", "http://h?x=1", "http://h#f", "://"} {
		if err := meilisearch.CheckURL(u); err == nil {
			t.Errorf("CheckURL(%q) = nil", u)
		} else if strings.Contains(err.Error(), "pw") {
			t.Errorf("CheckURL(%q) = %v, which quotes the password", u, err)
		}
	}
	for _, u := range []string{"http://localhost:7700", "https://search.example.org/meili/"} {
		if err := meilisearch.CheckURL(u); err != nil {
			t.Errorf("CheckURL(%q) = %v", u, err)
		}
	}
	for _, uid := range []string{"", "a b", "a/b", "é", strings.Repeat("a", 401)} {
		if err := meilisearch.CheckIndexUID(uid); err == nil {
			t.Errorf("CheckIndexUID(%q) = nil", uid)
		}
	}
	if err := meilisearch.CheckIndexUID("my_statuses-2"); err != nil {
		t.Error(err)
	}
	if _, err := meilisearch.New(meilisearch.Options{URL: "http://h"}); err == nil {
		t.Error("New without an HTTP client = nil")
	}
}

func TestAServerWithoutAKeyGetsNoAuthorization(t *testing.T) {
	s := &stub{t: t, tasks: map[int64]*stubTask{}}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	x, err := meilisearch.New(meilisearch.Options{URL: srv.URL, HTTPClient: newClient()})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = x.Search(t.Context(), search.Query{Text: "alpha", Limit: 1})
	for _, r := range s.reqs {
		if r.Auth != "" {
			t.Errorf("%s %s sent Authorization", r.Method, r.Path)
		}
	}
}
