// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/translator/fake"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/app/uow"
	domaincontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// recordingTranslator is the fake provider, which records what it receives. answer, when set,
// replaces the fake's answer.
type recordingTranslator struct {
	fake *fake.Translator

	mu       sync.Mutex
	requests []translation.Request
	answer   func(ctx context.Context, req translation.Request) (translation.Result, error)
}

func newRecordingTranslator() *recordingTranslator {
	return &recordingTranslator{fake: fake.New(fake.Options{Detected: "en"})}
}

func (r *recordingTranslator) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	r.mu.Lock()
	r.requests = append(r.requests, req)
	answer := r.answer
	r.mu.Unlock()
	if answer != nil {
		return answer(ctx, req)
	}
	return r.fake.Translate(ctx, req)
}

func (r *recordingTranslator) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *recordingTranslator) setAnswer(f func(ctx context.Context, req translation.Request) (translation.Result, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.answer = f
}

// changingProviders are the operator's providers, which a test changes.
type changingProviders struct {
	mu   sync.Mutex
	list []translation.Provider
}

func (p *changingProviders) Providers(context.Context) ([]translation.Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.list), nil
}

func (p *changingProviders) set(list ...translation.Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = list
}

// countingLimiter counts what the service reserves and refuses when told to.
type countingLimiter struct {
	reserved atomic.Int64
	refuse   atomic.Bool
}

func (l *countingLimiter) Reserve(_ context.Context, _ string, n int) error {
	if l.refuse.Load() {
		return translation.ErrLimitReached
	}
	l.reserved.Add(int64(n))
	return nil
}

// translationEnv is a server with a provider "one" enabled, and a user bob who follows alice.
type translationEnv struct {
	c         *statusesClient
	db        *sql.DB
	providers *changingProviders
	limiter   *countingLimiter
	one       *recordingTranslator
}

func newTranslationEnv(t *testing.T) *translationEnv {
	t.Helper()
	path := newDatabase(t, true)
	e := &translationEnv{providers: &changingProviders{}, limiter: &countingLimiter{}, one: newRecordingTranslator()}
	e.providers.set(translation.Provider{ID: "one", Name: "Provider One", URL: "https://one.example/", Translator: e.one})
	e.c = newStatusesClientWith(t, map[string]string{"MY_DATABASE_URL": path},
		platform.AppOptions{TranslationProviders: e.providers, TranslationLimiter: e.limiter}, "alice", "bob", "carol")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e.db = db
	if _, err := e.c.app.Graph.Follow(t.Context(), e.c.ids["bob"], e.c.ids["alice"]); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	return e
}

// translate asks for a translation into ja as user and returns the response.
func (e *translationEnv) translate(user, statusID string) (*http.Response, clientapi.StatusTranslation) {
	e.c.t.Helper()
	var out clientapi.StatusTranslation
	res := e.c.do(user, http.MethodPost, "/statuses/"+statusID+"/translation", map[string]any{"target_language": "ja"}, &out)
	return res, out
}

func (e *translationEnv) wantProblem(user, statusID string, status int, typ string) {
	e.c.t.Helper()
	res, _ := e.translate(user, statusID)
	if res.StatusCode != status {
		e.c.t.Fatalf("translate %s as %s = %d, want %d", statusID, user, res.StatusCode, status)
	}
	if typ != "" {
		var p clientapi.Problem
		if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
			e.c.t.Fatal(err)
		}
		if p.Type != typ {
			e.c.t.Errorf("problem type = %q, want %q", p.Type, typ)
		}
	}
}

func (e *translationEnv) rows(table string) int {
	e.c.t.Helper()
	var n int
	// #nosec G202 -- the table names come from the test itself.
	if err := e.db.QueryRowContext(e.c.t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		e.c.t.Fatal(err)
	}
	return n
}

func TestTranslateStatusAsksAProvider(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	st := e.c.post("alice", map[string]any{
		"text":         "Hello @bob read :blob_cat: at https://example.com/secret about #cats",
		"spoiler_text": "careful @carol",
		"language":     "en",
		"visibility":   "public",
		"poll":         map[string]any{"options": []string{"Yes", "No"}, "expires_in": 3600},
	})

	res, got := e.translate("bob", st.Id)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", res.Header.Get("Cache-Control"))
	}
	if got.Provider == nil || got.Provider.Id != "one" || got.Provider.Name != "Provider One" || got.Provider.Url == nil {
		t.Errorf("provider = %+v", got.Provider)
	}
	if got.Language != "ja" || got.SourceLanguage != "en" {
		t.Errorf("languages = %q from %q", got.Language, got.SourceLanguage)
	}
	for _, want := range []string{"[ja] Hello", `href="https://example.com/secret"`, ":blob_cat:", "tags/cats"} {
		if !strings.Contains(got.Content, want) {
			t.Errorf("content = %q, without %q", got.Content, want)
		}
	}
	if !strings.HasPrefix(got.SpoilerText, "[ja] careful") || !strings.Contains(got.SpoilerText, "@carol") {
		t.Errorf("spoiler_text = %q", got.SpoilerText)
	}
	if !slices.Equal(got.PollOptions, []string{"[ja] Yes", "[ja] No"}) {
		t.Errorf("poll_options = %q", got.PollOptions)
	}

	// Only the words reach the provider.
	if n := e.one.calls(); n != 1 {
		t.Fatalf("provider calls = %d, want 1", n)
	}
	req := e.one.requests[0]
	if req.SourceLanguage != "en" || req.TargetLanguage != "ja" {
		t.Errorf("languages sent = %q to %q", req.SourceLanguage, req.TargetLanguage)
	}
	sent := strings.Join(req.Fragments, "\n")
	for _, secret := range []string{"bob", "carol", "alice", "example", "blob_cat", "cats", "http", "<", st.Id, st.Uri} {
		if strings.Contains(sent, secret) {
			t.Errorf("sent %q, with %q", sent, secret)
		}
	}
	if got, want := e.limiter.reserved.Load(), int64(translation.CountCharacters(req.Fragments)); got != want {
		t.Errorf("reserved %d characters, want %d", got, want)
	}

	// A cached translation reaches the next reader without a provider or the limit.
	before := e.limiter.reserved.Load()
	res, again := e.translate("carol", st.Id)
	if res.StatusCode != http.StatusOK || again.Content != got.Content || again.Provider == nil {
		t.Errorf("second translate = %d, %+v", res.StatusCode, again)
	}
	if n := e.one.calls(); n != 1 {
		t.Errorf("provider calls = %d after the cache hit, want 1", n)
	}
	if e.limiter.reserved.Load() != before {
		t.Errorf("a cache hit counted against the limit")
	}

	// An edit makes a new revision, which misses the cache.
	plain := e.c.post("alice", map[string]any{"text": "Hello", "visibility": "public", "language": "en"})
	if res, _ := e.translate("bob", plain.Id); res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	calls := e.one.calls()
	if res := e.c.do("alice", http.MethodPut, "/statuses/"+plain.Id, map[string]any{"text": "Hello again", "language": "en"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT = %d", res.StatusCode)
	}
	res, after := e.translate("bob", plain.Id)
	if res.StatusCode != http.StatusOK || !strings.Contains(after.Content, "[ja] Hello again") {
		t.Errorf("translate after the edit = %d, %q", res.StatusCode, after.Content)
	}
	if n := e.one.calls(); n != calls+1 {
		t.Errorf("provider calls = %d after the edit, want %d", n, calls+1)
	}
}

func TestTranslateStatusChecksTheViewerFirst(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	e.providers.set(translation.Provider{
		ID: "one", Name: "One", Translator: e.one,
		Visibilities: []appcontent.Visibility{appcontent.VisibilityPublic, appcontent.VisibilityPrivate, appcontent.VisibilityDirect},
	})
	private := e.c.post("alice", map[string]any{"text": "for followers", "visibility": "private", "language": "en"})
	direct := e.c.post("alice", map[string]any{"text": "just us @bob", "visibility": "direct", "language": "en"})

	if res, _ := e.translate("", private.Id); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous = %d, want 401", res.StatusCode)
	}
	// Bob caches the translation, and carol, who may not see it, still gets a 404.
	if res, _ := e.translate("bob", private.Id); res.StatusCode != http.StatusOK {
		t.Fatalf("follower = %d", res.StatusCode)
	}
	e.wantProblem("carol", private.Id, http.StatusNotFound, "")
	if n := e.one.calls(); n != 1 {
		t.Errorf("provider calls = %d, want 1", n)
	}
	e.wantProblem("carol", direct.Id, http.StatusNotFound, "")
	e.wantProblem("bob", "019f0000-0000-7000-8000-000000000000", http.StatusNotFound, "")

	// A third-party app may call it with read:statuses, but without read:direct it sees no direct post.
	app := e.c.appToken("bob", "read:statuses")
	if res, _ := e.translate(app, private.Id); res.StatusCode != http.StatusOK {
		t.Errorf("app with read:statuses = %d, want 200", res.StatusCode)
	}
	e.wantProblem(app, direct.Id, http.StatusNotFound, "")
	if res, _ := e.translate(e.c.appToken("bob", "write:statuses"), private.Id); res.StatusCode != http.StatusForbidden {
		t.Errorf("app without read:statuses = %d, want 403", res.StatusCode)
	}
	if res, _ := e.translate("bob", direct.Id); res.StatusCode != http.StatusOK {
		t.Errorf("first-party token on a direct post = %d, want 200", res.StatusCode)
	}
}

func TestTranslateStatusGoesByEffectiveVisibility(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	ctx := t.Context()
	unavailable := clientapi.ProblemTypeTranslationUnavailable
	for _, vis := range []string{"public", "local", "unlisted"} {
		st := e.c.post("alice", map[string]any{"text": "words of " + vis, "visibility": vis, "language": "en"})
		if res, _ := e.translate("bob", st.Id); res.StatusCode != http.StatusOK {
			t.Errorf("%s by default = %d, want 200", vis, res.StatusCode)
		}
	}
	calls := e.one.calls()
	private := e.c.post("alice", map[string]any{"text": "for followers", "visibility": "private", "language": "en"})
	direct := e.c.post("alice", map[string]any{"text": "just us @bob", "visibility": "direct", "language": "en"})
	e.wantProblem("bob", private.Id, http.StatusUnprocessableEntity, unavailable)
	e.wantProblem("bob", direct.Id, http.StatusUnprocessableEntity, unavailable)

	public := e.c.post("alice", map[string]any{"text": "open to all", "visibility": "public", "language": "en"})
	action, err := e.c.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.c.ids["alice"], Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction: %v", err)
	}
	// A limited author's public post is judged as private.
	e.wantProblem("bob", public.Id, http.StatusUnprocessableEntity, unavailable)
	if n := e.one.calls(); n != calls {
		t.Errorf("provider calls = %d, want %d: nothing goes out", n, calls)
	}
	if err := e.c.app.Moderation.LiftAccountAction(ctx, action.ID, "", ""); err != nil {
		t.Fatal(err)
	}

	// A provider allowed private gets it; a provider that has it taken away does not get public.
	e.providers.set(translation.Provider{
		ID: "one", Name: "One", Translator: e.one, Visibilities: []appcontent.Visibility{appcontent.VisibilityPrivate},
	})
	if res, _ := e.translate("bob", private.Id); res.StatusCode != http.StatusOK {
		t.Errorf("private when allowed = %d, want 200", res.StatusCode)
	}
	e.wantProblem("bob", public.Id, http.StatusUnprocessableEntity, unavailable)
}

func TestTranslateStatusAuthorsOwnLanguage(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	ctx := t.Context()
	var id string
	err := e.c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a, err := e.c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: "https://remote.example/users/dave", Domain: "remote.example", Type: "Person", PreferredUsername: "dave",
			InboxURL: "https://remote.example/users/dave/inbox", Document: []byte(`{}`),
		})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		p, err := e.c.app.Content.IngestRemoteStatus(ctx, tx, appcontent.RemoteStatusInput{
			URI: "https://remote.example/statuses/1", AuthorAccountID: a.ID, AuthorDomain: "remote.example",
			Visibility: domaincontent.VisibilityPublic, PublishedAt: now, RevisedAt: now,
			Bodies: []appcontent.RemoteBody{{Language: "en", HTML: "<p>hello</p>"}, {Language: "ja", HTML: "<p>作者のことば</p>"}},
		})
		id = p.Status.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	res, got := e.translate("bob", id)
	if res.StatusCode != http.StatusOK || got.Provider != nil || got.Content != "<p>作者のことば</p>" {
		t.Errorf("translate = %d, %+v", res.StatusCode, got)
	}
	if n := e.one.calls(); n != 0 || e.limiter.reserved.Load() != 0 {
		t.Errorf("provider calls = %d, reserved = %d, want none", n, e.limiter.reserved.Load())
	}
	// The same holds with no provider enabled.
	e.providers.set()
	if res, _ := e.translate("bob", id); res.StatusCode != http.StatusOK {
		t.Errorf("translate without providers = %d", res.StatusCode)
	}
	// A post without words has nothing to send.
	only := e.c.post("alice", map[string]any{"text": ":blob_cat: https://example.com/", "visibility": "public", "language": "en"})
	e.providers.set(translation.Provider{ID: "one", Name: "One", Translator: e.one})
	if res, got := e.translate("bob", only.Id); res.StatusCode != http.StatusOK || got.Provider != nil {
		t.Errorf("translate of a post without words = %d, %+v", res.StatusCode, got)
	}
	if n := e.one.calls(); n != 0 {
		t.Errorf("provider calls = %d, want 0", n)
	}
	// An invalid language is refused before anything else.
	if res := e.c.do("bob", http.MethodPost, "/statuses/"+id+"/translation", map[string]any{"target_language": "not a tag"}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("invalid language = %d", res.StatusCode)
	}
	if res := e.c.do("bob", http.MethodPost, "/statuses/"+id+"/translation", map[string]any{"target_language": "und"}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("und = %d", res.StatusCode)
	}
}

func TestTranslateStatusFollowsThePriorityAndTheSettings(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	two := newRecordingTranslator()
	first := translation.Provider{ID: "one", Name: "One", Translator: e.one}
	second := translation.Provider{ID: "two", Name: "Two", Translator: two}
	e.providers.set(first, second)
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})

	// A provider that cannot do it passes to the next: each kind of failure.
	for _, failure := range []error{translation.ErrUnsupportedLanguage, translation.ErrProviderLimit, translation.ErrTemporary} {
		e.one.fake.FailNext(failure)
		res, got := e.translate("bob", st.Id)
		if res.StatusCode != http.StatusOK || got.Provider == nil || got.Provider.Id != "two" {
			t.Fatalf("after %v: %d, %+v", failure, res.StatusCode, got.Provider)
		}
		if _, err := e.db.ExecContext(t.Context(), "DELETE FROM status_translations"); err != nil {
			t.Fatal(err)
		}
	}

	// Two has a translation now, and one does not. A disabled provider's translation is not returned,
	if res, got := e.translate("bob", st.Id); res.StatusCode != http.StatusOK || got.Provider.Id != "one" {
		t.Fatalf("translate = %d, %+v", res.StatusCode, got.Provider)
	}
	before := two.calls()
	e.providers.set(second)
	res, got := e.translate("bob", st.Id)
	if res.StatusCode != http.StatusOK || got.Provider.Id != "two" || two.calls() != before+1 {
		t.Errorf("after disabling one: %d, %+v, %d calls", res.StatusCode, got.Provider, two.calls()-before)
	}
	// and one that is enabled again answers from its row, in the order of priority.
	calls := e.one.calls()
	e.providers.set(first, second)
	if res, got := e.translate("bob", st.Id); res.StatusCode != http.StatusOK || got.Provider.Id != "one" || e.one.calls() != calls {
		t.Errorf("after enabling one: %d, %+v, %d calls", res.StatusCode, got.Provider, e.one.calls()-calls)
	}
	// A provider no longer allowed the visibility is the same as disabled.
	first.Visibilities = []appcontent.Visibility{}
	e.providers.set(first, second)
	if res, got := e.translate("bob", st.Id); res.StatusCode != http.StatusOK || got.Provider.Id != "two" {
		t.Errorf("after refusing one: %d, %+v", res.StatusCode, got.Provider)
	}
	// With no candidate left, the rows stay and nothing is returned.
	e.providers.set()
	e.wantProblem("bob", st.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	if n := e.rows("status_translations"); n == 0 {
		t.Errorf("rows = 0, want them kept until they expire")
	}
}

func TestTranslateStatusRefusesBadOutput(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	st := e.c.post("alice", map[string]any{
		"text":       "Hello https://example.com/a",
		"visibility": "public", "language": "en", "spoiler_text": "look",
	})
	answers := map[string][]string{
		"a marker lost":     {"こんにちは", "見て"},
		"a marker twice":    {"⟦1⟧ ⟦1⟧", "見て"},
		"a marker invented": {"⟦1⟧ ⟦2⟧", "見て"},
		"too few":           {"⟦1⟧"},
		"empty":             {"⟦1⟧", " "},
	}
	failed := clientapi.ProblemTypeTranslationFailed
	for name, answer := range answers {
		e.one.setAnswer(func(context.Context, translation.Request) (translation.Result, error) {
			return translation.Result{Translations: answer, DetectedLanguage: "en"}, nil
		})
		e.wantProblem("bob", st.Id, http.StatusServiceUnavailable, failed)
		if n := e.rows("status_translations"); n != 0 {
			t.Errorf("%s: stored %d rows", name, n)
		}
	}
	// The characters of an answer stay counted: the provider has received them.
	if e.limiter.reserved.Load() == 0 {
		t.Errorf("no characters counted for answers that the provider gave")
	}
	// A failure counts too: the port cannot say that the provider did not receive them.
	e.limiter.reserved.Store(0)
	e.one.setAnswer(func(context.Context, translation.Request) (translation.Result, error) {
		return translation.Result{}, translation.ErrTemporary
	})
	e.wantProblem("bob", st.Id, http.StatusServiceUnavailable, failed)
	if got := e.limiter.reserved.Load(); got == 0 {
		t.Errorf("reserved %d after a failure, want the characters of the call", got)
	}
	e.one.setAnswer(func(context.Context, translation.Request) (translation.Result, error) {
		return translation.Result{}, translation.ErrUnsupportedLanguage
	})
	e.wantProblem("bob", st.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnsupportedLanguage)

	// Markup in the output is text in the body, and tags do not stay in the plain text.
	e.one.setAnswer(func(context.Context, translation.Request) (translation.Result, error) {
		return translation.Result{
			Translations:     []string{`<script>alert(1)</script><img src=x onerror=y> ⟦1⟧`, `<b>見て</b><script>alert(1)</script>`},
			DetectedLanguage: "en",
		}, nil
	})
	res, got := e.translate("bob", st.Id)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	if strings.Contains(got.Content, "<script") || strings.Contains(got.Content, "<img") {
		t.Errorf("content = %q", got.Content)
	}
	if !strings.Contains(got.Content, "&lt;script&gt;") || !strings.Contains(got.Content, `href="https://example.com/a"`) {
		t.Errorf("content = %q", got.Content)
	}
	if got.SpoilerText != "見て" {
		t.Errorf("spoiler_text = %q", got.SpoilerText)
	}
}

func TestTranslateStatusOverALimit(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	e.limiter.refuse.Store(true)
	e.wantProblem("bob", st.Id, http.StatusTooManyRequests, clientapi.ProblemTypeTranslationLimit)
	if n := e.one.calls(); n != 0 {
		t.Errorf("provider calls = %d, want 0", n)
	}
}

// TestTranslateStatusConcurrently checks that two requests for one translation call the provider
// once, outside any transaction: while the provider works, the database takes other writes.
func TestTranslateStatusConcurrently(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	e.one.setAnswer(func(ctx context.Context, req translation.Request) (translation.Result, error) {
		once.Do(func() { close(started) })
		<-release
		return e.one.fake.Translate(ctx, req)
	})
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: e.c.ids["bob"]}
	results := make(chan error, 2)
	translate := func() {
		_, err := e.c.app.Translation.Translate(context.WithoutCancel(t.Context()), viewer, st.Id, "ja")
		results <- err
	}
	go translate()
	<-started
	go translate()

	// SQLite has one writer, so this write would wait on a transaction left open around the call.
	e.c.post("alice", map[string]any{"text": "a write meanwhile", "visibility": "public", "language": "en"})
	time.Sleep(300 * time.Millisecond) // the second request reaches the lock
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Errorf("Translate: %v", err)
		}
	}
	if n := e.one.calls(); n != 1 {
		t.Errorf("provider calls = %d, want 1", n)
	}
	if n := e.rows("status_translations"); n != 1 {
		t.Errorf("rows = %d, want 1", n)
	}
	// The request that lost the lock reserved nothing.
	want := int64(translation.CountCharacters(e.one.requests[0].Fragments))
	if got := e.limiter.reserved.Load(); got != want {
		t.Errorf("reserved %d characters, want %d", got, want)
	}

	// A request that waits for the lock takes it over when the holder fails.
	other := e.c.post("alice", map[string]any{"text": "other words", "visibility": "public", "language": "en"})
	calls := e.one.calls()
	started, release = make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	e.one.setAnswer(func(ctx context.Context, req translation.Request) (translation.Result, error) {
		if first.CompareAndSwap(false, true) {
			close(started)
			<-release
			return translation.Result{}, translation.ErrTemporary
		}
		return e.one.fake.Translate(ctx, req)
	})
	failed := make(chan error, 1)
	go func() {
		_, err := e.c.app.Translation.Translate(context.WithoutCancel(t.Context()), viewer, other.Id, "ja")
		failed <- err
	}()
	<-started
	waited := make(chan error, 1)
	go func() {
		_, err := e.c.app.Translation.Translate(context.WithoutCancel(t.Context()), viewer, other.Id, "ja")
		waited <- err
	}()
	time.Sleep(300 * time.Millisecond)
	close(release)
	if err := <-failed; !errors.Is(err, translation.ErrFailed) {
		t.Errorf("the holder = %v, want ErrFailed", err)
	}
	if err := <-waited; err != nil {
		t.Errorf("the request that waited = %v", err)
	}
	if n := e.one.calls(); n != calls+2 {
		t.Errorf("provider calls = %d, want %d", n, calls+2)
	}
}

// TestTranslationsGoWithThePost checks the deletion paths (ADR-0148 §10): each emits
// content.StatusDeleted, in whose transaction the rows go, the child rows too.
func TestTranslationsGoWithThePost(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	ctx := t.Context()
	children := func() int {
		return e.rows("status_translation_poll_options") + e.rows("status_translation_attachments")
	}
	withPoll := func() clientapi.Status {
		return e.c.post("alice", map[string]any{
			"text": "some words", "visibility": "public", "language": "en",
			"poll": map[string]any{"options": []string{"Yes", "No"}, "expires_in": 3600},
		})
	}

	st := withPoll()
	if res, _ := e.translate("bob", st.Id); res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	if n := e.rows("status_translations"); n != 1 || children() != 2 {
		t.Fatalf("rows = %d and %d children, want 1 and 2", n, children())
	}
	if res := e.c.do("alice", http.MethodDelete, "/statuses/"+st.Id, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE = %d", res.StatusCode)
	}
	if n := e.rows("status_translations"); n != 0 || children() != 0 {
		t.Errorf("after the author's deletion: %d rows and %d children", n, children())
	}
	e.wantProblem("bob", st.Id, http.StatusNotFound, "")

	// A deletion by moderators, on the same event.
	st = withPoll()
	if res, _ := e.translate("bob", st.Id); res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	found, err := e.c.app.Statuses.Visible(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: e.c.ids["bob"]}, st.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.c.app.Content.DeleteByModeration(ctx, tx, found, e.c.app.IDs.NewID(), time.Now().UTC())
	}); err != nil {
		t.Fatalf("delete by a moderator: %v", err)
	}
	if n := e.rows("status_translations"); n != 0 || children() != 0 {
		t.Errorf("after the moderators' deletion: %d rows and %d children", n, children())
	}

	// A post deleted while the provider works stores nothing.
	st = withPoll()
	e.one.setAnswer(func(c context.Context, req translation.Request) (translation.Result, error) {
		if _, err := e.c.app.Content.Delete(c, e.c.ids["alice"], st.Id); err != nil {
			return translation.Result{}, errors.Join(translation.ErrTemporary, err)
		}
		return e.one.fake.Translate(c, req)
	})
	e.wantProblem("bob", st.Id, http.StatusNotFound, "")
	if n := e.rows("status_translations"); n != 0 {
		t.Errorf("after the deletion during the call: %d rows", n)
	}
}

// TestSuspensionKeepsTranslations checks that a suspension hides the translations through the view
// check and keeps them, and that lifting it brings them back without a provider (ADR-0148 §10).
func TestSuspensionKeepsTranslations(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	ctx := t.Context()
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	if res, _ := e.translate("bob", st.Id); res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	action, err := e.c.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.c.ids["alice"], Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction: %v", err)
	}
	e.wantProblem("bob", st.Id, http.StatusNotFound, "")
	if n := e.rows("status_translations"); n != 1 {
		t.Errorf("rows under suspension = %d, want 1", n)
	}
	if err := e.c.app.Moderation.LiftAccountAction(ctx, action.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	if res, got := e.translate("bob", st.Id); res.StatusCode != http.StatusOK || got.Provider == nil {
		t.Errorf("translate after the lift = %d, %+v", res.StatusCode, got)
	}
	if n := e.one.calls(); n != 1 {
		t.Errorf("provider calls = %d, want 1", n)
	}
}

func TestPurgeTranslationCache(t *testing.T) {
	t.Parallel()
	e := newTranslationEnv(t)
	ctx := t.Context()
	for _, text := range []string{"old words", "new words"} {
		st := e.c.post("alice", map[string]any{
			"text": text, "visibility": "public", "language": "en",
			"poll": map[string]any{"options": []string{"Yes", "No"}, "expires_in": 3600},
		})
		if res, _ := e.translate("bob", st.Id); res.StatusCode != http.StatusOK {
			t.Fatalf("translate = %d", res.StatusCode)
		}
	}
	now := time.Now().UTC()
	expired := now.AddDate(0, 0, -translation.DefaultRetentionDays-1).UnixMilli()
	if _, err := e.db.ExecContext(ctx, "UPDATE status_translations SET created_at = ? WHERE content_html LIKE '%old words%'", expired); err != nil {
		t.Fatal(err)
	}
	lastMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	before := time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	for _, month := range []string{before, lastMonth} {
		if _, err := e.db.ExecContext(ctx, "INSERT INTO translation_account_usage (account_id, month, characters) VALUES (?, ?, 1)", e.c.ids["bob"], month); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.ExecContext(ctx, "INSERT INTO translation_server_usage (month, characters) VALUES (?, 1)", month); err != nil {
			t.Fatal(err)
		}
	}
	handler, _, ok := e.c.app.Handlers.Lookup(translation.JobPurgeCache)
	if !ok {
		t.Fatalf("no handler of %s", translation.JobPurgeCache)
	}
	if res := handler(ctx, queue.Job{Type: translation.JobPurgeCache}); res.Err() != nil {
		t.Fatalf("purge: %v", res.Err())
	}
	if n := e.rows("status_translations"); n != 1 {
		t.Errorf("translations = %d, want the new one", n)
	}
	if n := e.rows("status_translation_poll_options"); n != 2 {
		t.Errorf("poll options = %d, want those of the new one", n)
	}
	// The server's rows are last month's and this month's, of the requests above.
	if a, s := e.rows("translation_account_usage"), e.rows("translation_server_usage"); a != 1 || s != 2 {
		t.Errorf("usage = %d and %d rows, want last month's for the account and this month's too for the server", a, s)
	}
}

// TestErasureRemovesTranslationUsage checks that the erasure of an account takes its usage and the
// translations of its posts, and leaves the server's usage.
func TestErasureRemovesTranslationUsage(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, true)
	network := &relayNet{docs: map[string][]byte{}}
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	one := newRecordingTranslator()
	providers := &changingProviders{}
	providers.set(translation.Provider{ID: "one", Name: "One", Translator: one})
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline", "MY_DATABASE_URL": path}, platform.AppOptions{
		HTTPClient: network, Clock: clock, TranslationProviders: providers, TranslationLimiter: &countingLimiter{},
	}, "alice")
	ctx := t.Context()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rows := func(table string) int {
		t.Helper()
		var n int
		// #nosec G202 -- the table names come from the test itself.
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	drain := func() {
		t.Helper()
		if _, err := c.app.Runner.Drain(ctx); err != nil {
			t.Fatalf("Drain: %v", err)
		}
	}
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	note := carol.id + "/statuses/1"
	aliceIRI := "https://example.test/ap/actors/" + c.ids["alice"]
	postInbox(t, c, carol, map[string]any{
		"id": note + "/activity", "type": "Create", "actor": carol.id,
		"object": map[string]any{
			"id": note, "type": "Note", "attributedTo": carol.id, "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{aliceIRI}, "content": "<p>hi there</p>",
			"tag": []any{map[string]any{"type": "Mention", "href": aliceIRI}},
		},
	})
	drain()
	actor, err := c.app.Accounts.FindByURI(ctx, carol.id)
	if err != nil {
		t.Fatal(err)
	}
	var statusID string
	if err := db.QueryRowContext(ctx, "SELECT id FROM statuses WHERE uri = ?", note).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	if res := c.do("alice", http.MethodPost, "/statuses/"+statusID+"/translation", map[string]any{"target_language": "ja"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	for _, q := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO translation_account_usage (account_id, month, characters) VALUES (?, '2026-11', 5)", []any{actor.Account.ID}},
		{"INSERT INTO translation_account_usage (account_id, month, characters) VALUES (?, '2026-11', 7)", []any{c.ids["alice"]}},
		{"INSERT INTO translation_server_usage (month, characters) VALUES ('2026-11', 12) ON CONFLICT (month) DO UPDATE SET characters = 12", nil},
	} {
		if _, err := db.ExecContext(ctx, q.query, q.args...); err != nil {
			t.Fatal(err)
		}
	}

	postInbox(t, c, carol, map[string]any{
		"id": carol.id + "#delete", "type": "Delete", "actor": carol.id, "object": carol.id,
		"to": []string{"https://www.w3.org/ns/activitystreams#Public"},
	})
	drain()
	handler, _, ok := c.app.Handlers.Lookup(appaccount.JobEraseDeletedAccounts)
	if !ok {
		t.Fatalf("no handler of %s", appaccount.JobEraseDeletedAccounts)
	}
	clock.ahead.Store(int64(appaccount.ErasureDelay + time.Minute))
	if res := handler(ctx, queue.Job{Type: appaccount.JobEraseDeletedAccounts}); res.Err() != nil {
		t.Fatalf("erasure: %v", res.Err())
	}
	drain()
	clock.ahead.Store(0)

	var left int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM translation_account_usage WHERE account_id = ?", actor.Account.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("usage rows of the erased account = %d", left)
	}
	if n := rows("translation_account_usage"); n != 1 {
		t.Errorf("usage rows = %d, want alice's", n)
	}
	if n := rows("translation_server_usage"); n != 1 {
		t.Errorf("server usage rows = %d, want it kept", n)
	}
	if n := rows("status_translations"); n != 0 {
		t.Errorf("translations of the erased account's posts = %d", n)
	}
}
