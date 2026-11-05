// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// configuredTranslation is a server with the real settings and limits, which has the providers
// "one" and "two" and enables neither.
type configuredTranslation struct {
	translationEnv
	two   *recordingTranslator
	meter *testkit.Meter
	ahead *atomic.Int64
}

func newConfiguredTranslation(t *testing.T) *configuredTranslation {
	t.Helper()
	path := newDatabase(t, true)
	e := &configuredTranslation{two: newRecordingTranslator(), meter: testkit.NewMeter(), ahead: &atomic.Int64{}}
	e.one = newRecordingTranslator()
	e.c = newStatusesClientWith(t, map[string]string{"MY_DATABASE_URL": path}, platform.AppOptions{
		Clock: laterClock{steadyClock: newSteadyClock(), ahead: e.ahead}, Meter: e.meter,
		TranslationRegistered: []translation.Provider{
			{ID: "one", Name: "Provider One", URL: "https://one.example/", Translator: e.one},
			{ID: "two", Name: "Provider Two", Translator: e.two},
		},
	}, "alice", "bob", "carol")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e.db = db
	return e
}

func (e *configuredTranslation) set(key instance.SettingKey, value string) {
	e.c.t.Helper()
	if _, err := e.c.app.Instance.SetSetting(e.c.t.Context(), appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
		e.c.t.Fatalf("set %s = %q: %v", key, value, err)
	}
}

func (e *configuredTranslation) instance() clientapi.InstanceTranslation {
	e.c.t.Helper()
	var out clientapi.Instance
	if res := e.c.do("bob", http.MethodGet, "/instance", nil, &out); res.StatusCode != http.StatusOK {
		e.c.t.Fatalf("GET /instance = %d", res.StatusCode)
	}
	return out.Translation
}

func (e *configuredTranslation) overview() clientapi.AdminTranslationOverview {
	e.c.t.Helper()
	var out clientapi.AdminTranslationOverview
	if res := e.c.do("bob", http.MethodGet, "/admin/translation", nil, &out); res.StatusCode != http.StatusOK {
		e.c.t.Fatalf("GET /admin/translation = %d", res.StatusCode)
	}
	return out
}

func (e *configuredTranslation) characters(provider string) float64 {
	return e.meter.Sum(translation.Characters.Name, map[string]string{"my.provider": provider})
}

// TestTranslationStartsOff checks ADR-0148 §3: with no setting, nothing is sent and clients are told
// that there is nothing to offer.
func TestTranslationStartsOff(t *testing.T) {
	t.Parallel()
	e := newConfiguredTranslation(t)
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	e.wantProblem("bob", st.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	if n := e.one.calls() + e.two.calls(); n != 0 {
		t.Errorf("provider calls = %d", n)
	}
	got := e.instance()
	if got.Browser || len(got.Providers) != 0 {
		t.Errorf("instance translation = %+v, want nothing", got)
	}
	settings, err := e.c.app.Instance.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.TranslationUserMonthlyCharacters() != 50000 || settings.TranslationServerMonthlyCharacters() != 500000 || settings.TranslationRetentionDays() != 30 {
		t.Errorf("defaults = %d, %d, %d", settings.TranslationUserMonthlyCharacters(), settings.TranslationServerMonthlyCharacters(), settings.TranslationRetentionDays())
	}
}

// TestTranslationVisibilitiesAreThoseOfPosts keeps the values that the setting accepts the same as
// the visibilities of posts, which the domain of the instance may not import.
func TestTranslationVisibilitiesAreThoseOfPosts(t *testing.T) {
	t.Parallel()
	got := translation.Provider{Visibilities: []appcontent.Visibility{
		appcontent.VisibilityPublic, appcontent.VisibilityLocal, appcontent.VisibilityUnlisted, appcontent.VisibilityPrivate, appcontent.VisibilityDirect,
	}}.Allowed()
	if len(got) != len(instance.TranslationVisibilities) {
		t.Fatalf("visibilities = %v, want %v", got, instance.TranslationVisibilities)
	}
	for i, v := range got {
		if string(v) != instance.TranslationVisibilities[i] {
			t.Errorf("visibility %d = %q, want %q", i, v, instance.TranslationVisibilities[i])
		}
	}
}

// TestTranslationSettingsDecideTheProviders checks the order, the visibilities of each provider,
// what GET /instance publishes, and that a change takes effect at once.
func TestTranslationSettingsDecideTheProviders(t *testing.T) {
	t.Parallel()
	e := newConfiguredTranslation(t)
	ctx := t.Context()
	public := e.c.post("alice", map[string]any{"text": "public words", "visibility": "public", "language": "en"})
	direct := e.c.post("alice", map[string]any{"text": "@bob direct words", "visibility": "direct", "language": "en"})

	// A provider that this server does not have does nothing.
	e.set(instance.SettingTranslationProviders, "gone,two,one")
	e.set(instance.SettingTranslationBrowser, "true")
	got := e.instance()
	if !got.Browser || len(got.Providers) != 2 || got.Providers[0].Id != "two" || got.Providers[1].Id != "one" {
		t.Fatalf("instance translation = %+v", got)
	}
	if want := []clientapi.Visibility{"public", "local", "unlisted"}; !slices.Equal(got.Providers[0].Visibilities, want) {
		t.Errorf("default visibilities = %v, want %v", got.Providers[0].Visibilities, want)
	}
	if got.Providers[1].Url == nil || *got.Providers[1].Url != "https://one.example/" {
		t.Errorf("the provider's URL = %v", got.Providers[1].Url)
	}

	// The first in the order goes first.
	if res, out := e.translate("bob", public.Id); res.StatusCode != http.StatusOK || out.Provider == nil || out.Provider.Id != "two" {
		t.Fatalf("translate = %d, %+v", res.StatusCode, out)
	}
	if e.one.calls() != 0 || e.two.calls() != 1 {
		t.Errorf("calls = %d and %d", e.one.calls(), e.two.calls())
	}

	// By default direct goes to neither.
	e.wantProblem("bob", direct.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)

	// Allowing one provider allows only that one, and takes it from the other's turn.
	e.set(instance.SettingTranslationProviderVisibilities, "one=public|direct")
	if res, out := e.translate("bob", direct.Id); res.StatusCode != http.StatusOK || out.Provider == nil || out.Provider.Id != "one" {
		t.Fatalf("translate direct = %d, %+v", res.StatusCode, out)
	}
	if e.two.calls() != 1 {
		t.Errorf("provider two received a direct post: %d calls", e.two.calls())
	}
	got = e.instance()
	if want := []clientapi.Visibility{"public", "direct"}; !slices.Equal(got.Providers[1].Visibilities, want) {
		t.Errorf("visibilities = %v, want %v", got.Providers[1].Visibilities, want)
	}

	// A provider taken off every visibility, or disabled, stops at once, and its kept translation
	// is not returned.
	e.set(instance.SettingTranslationProviderVisibilities, "one=,two=")
	e.wantProblem("bob", public.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	e.set(instance.SettingTranslationProviderVisibilities, "")
	e.set(instance.SettingTranslationProviders, "")
	e.wantProblem("bob", public.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)

	// Each change is in the audit log.
	log, err := e.c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	audited := map[string]bool{}
	for _, entry := range log {
		if entry.Action == moderation.AuditSettingChanged {
			audited[entry.TargetID] = true
		}
	}
	for _, k := range []instance.SettingKey{instance.SettingTranslationProviders, instance.SettingTranslationProviderVisibilities, instance.SettingTranslationBrowser} {
		if !audited[string(k)] {
			t.Errorf("the change of %s is not in the audit log", k)
		}
	}
}

// TestTranslationLimits runs the limits of the settings (ADR-0148 §11): over one the request is
// refused without a provider call; what does not go to a provider does not count; months are in UTC.
func TestTranslationLimits(t *testing.T) {
	t.Parallel()
	e := newConfiguredTranslation(t)
	ctx := t.Context()
	e.set(instance.SettingTranslationProviders, "one")
	post := func(text string) clientapi.Status {
		return e.c.post("alice", map[string]any{"text": text, "visibility": "public", "language": "en"})
	}
	usage := func(user string) translation.Usage {
		t.Helper()
		month := time.Now().Add(time.Duration(e.ahead.Load())).UTC().Format("2006-01")
		var u translation.Usage
		if err := e.db.QueryRowContext(ctx, "SELECT coalesce((SELECT characters FROM translation_account_usage WHERE account_id = ? AND month = ?), 0), coalesce((SELECT characters FROM translation_server_usage WHERE month = ?), 0)", e.c.ids[user], month, month).Scan(&u.Account, &u.Server); err != nil {
			t.Fatal(err)
		}
		return u
	}

	// A limit of 0 sends nothing: there is no unlimited value.
	for _, key := range []instance.SettingKey{instance.SettingTranslationUserMonthlyCharacters, instance.SettingTranslationServerMonthlyCharacters} {
		e.set(key, "0")
		e.wantProblem("bob", post("words for zero").Id, http.StatusTooManyRequests, clientapi.ProblemTypeTranslationLimit)
		e.set(key, "1000")
	}
	if n := e.one.calls(); n != 0 {
		t.Fatalf("provider calls at a limit of 0 = %d", n)
	}
	for _, v := range []string{"-1", "unlimited"} {
		if _, err := e.c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingTranslationUserMonthlyCharacters, Value: v}); err == nil {
			t.Errorf("the limit accepted %q", v)
		}
	}

	// Markers are not counted: only "hello there" goes out.
	e.set(instance.SettingTranslationUserMonthlyCharacters, "20")
	first := post("hello there @carol")
	res, _ := e.translate("bob", first.Id)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	sent := translation.CountCharacters(e.one.requests[0].Fragments)
	if sent == 0 || sent > 20 || strings.Contains(strings.Join(e.one.requests[0].Fragments, ""), "carol") {
		t.Fatalf("sent %q, %d characters", e.one.requests[0].Fragments, sent)
	}
	if got := usage("bob"); got.Account != int64(sent) || got.Server != int64(sent) {
		t.Errorf("usage = %+v, want %d", got, sent)
	}
	if got := e.characters("one"); got != float64(sent) {
		t.Errorf("metric = %v, want %d", got, sent)
	}

	// A cache hit does not count, nor does it reach the limit.
	e.set(instance.SettingTranslationUserMonthlyCharacters, "1")
	e.set(instance.SettingTranslationServerMonthlyCharacters, "1")
	if res, _ := e.translate("bob", first.Id); res.StatusCode != http.StatusOK {
		t.Errorf("a cache hit over the limit = %d", res.StatusCode)
	}
	if res, _ := e.translate("carol", first.Id); res.StatusCode != http.StatusOK {
		t.Errorf("a cache hit of another reader over the limit = %d", res.StatusCode)
	}
	// Nor does the author's own language.
	var own clientapi.StatusTranslation
	if res := e.c.do("bob", http.MethodPost, "/statuses/"+first.Id+"/translation", map[string]any{"target_language": "en"}, &own); res.StatusCode != http.StatusOK {
		t.Errorf("the author's own language over the limit = %d", res.StatusCode)
	}
	if got := usage("bob"); got.Account != int64(sent) {
		t.Errorf("usage = %+v after what does not count", got)
	}

	// Over the user's limit: another user still has room, and no provider is called.
	calls := e.one.calls()
	e.set(instance.SettingTranslationServerMonthlyCharacters, "1000")
	second := post("some more words")
	e.wantProblem("bob", second.Id, http.StatusTooManyRequests, clientapi.ProblemTypeTranslationLimit)
	if n := e.one.calls(); n != calls {
		t.Errorf("provider calls = %d over the user's limit, want %d", n, calls)
	}
	e.set(instance.SettingTranslationUserMonthlyCharacters, "1000")
	if res, _ := e.translate("carol", second.Id); res.StatusCode != http.StatusOK {
		t.Errorf("another user = %d", res.StatusCode)
	}

	// Over the server's limit.
	calls = e.one.calls()
	over := usage("carol").Server
	e.set(instance.SettingTranslationServerMonthlyCharacters, "1")
	third := post("yet more words")
	e.wantProblem("carol", third.Id, http.StatusTooManyRequests, clientapi.ProblemTypeTranslationLimit)
	if n := e.one.calls(); n != calls {
		t.Errorf("provider calls = %d over the server's limit, want %d", n, calls)
	}
	if got := usage("carol").Server; got != over {
		t.Errorf("a refusal counted: %d, want %d", got, over)
	}

	// Another month's rows do not count. The limiter's own test covers where a month starts.
	for _, table := range []string{"translation_account_usage", "translation_server_usage"} {
		// #nosec G202 -- the table names come from the test itself.
		if _, err := e.db.ExecContext(ctx, "UPDATE "+table+" SET month = '2000-01'"); err != nil {
			t.Fatal(err)
		}
	}
	e.set(instance.SettingTranslationServerMonthlyCharacters, "1000")
	if res, _ := e.translate("carol", third.Id); res.StatusCode != http.StatusOK {
		t.Errorf("translate in the next month = %d", res.StatusCode)
	}
	if got := usage("carol"); got.Server == 0 || got.Server >= over {
		t.Errorf("usage of the new month = %+v, want a fresh count below %d", got, over)
	}
}

// TestTranslationCountsEveryCall checks that what counts is what was sent (ADR-0148 §11): a provider
// that fails may have received the text, and the next one receives it again.
func TestTranslationCountsEveryCall(t *testing.T) {
	t.Parallel()
	e := newConfiguredTranslation(t)
	e.set(instance.SettingTranslationProviders, "one,two")
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	e.one.setAnswer(func(context.Context, translation.Request) (translation.Result, error) {
		return translation.Result{}, translation.ErrTemporary
	})
	res, out := e.translate("bob", st.Id)
	if res.StatusCode != http.StatusOK || out.Provider == nil || out.Provider.Id != "two" {
		t.Fatalf("translate = %d, %+v", res.StatusCode, out)
	}
	sent := translation.CountCharacters(e.one.requests[0].Fragments)
	got := e.overview()
	if got.Characters != int64(2*sent) {
		t.Errorf("characters = %d, want %d: both providers received them", got.Characters, 2*sent)
	}
	if e.characters("one") != float64(sent) || e.characters("two") != float64(sent) {
		t.Errorf("metric = %v and %v, want %d each", e.characters("one"), e.characters("two"), sent)
	}

	// With room for one call only, the failure stops the second with the limit, without a call.
	st = e.c.post("alice", map[string]any{"text": "other words", "visibility": "public", "language": "en"})
	e.set(instance.SettingTranslationUserMonthlyCharacters, "40")
	two := e.two.calls()
	e.wantProblem("bob", st.Id, http.StatusTooManyRequests, clientapi.ProblemTypeTranslationLimit)
	if e.two.calls() != two {
		t.Errorf("the second provider was called over the limit")
	}
}

// TestTranslationWaitingHoldsNoCharacters checks that a request waiting for another one's
// translation holds nothing against the limit: near it, that request gets the row and not a 429.
func TestTranslationWaitingHoldsNoCharacters(t *testing.T) {
	t.Parallel()
	e := newConfiguredTranslation(t)
	e.set(instance.SettingTranslationProviders, "one")
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	started, release := make(chan struct{}), make(chan struct{})
	e.one.setAnswer(func(ctx context.Context, req translation.Request) (translation.Result, error) {
		close(started)
		<-release
		return e.one.fake.Translate(ctx, req)
	})
	viewer := func(user string) appcontent.Viewer {
		return appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: e.c.ids[user]}
	}
	results := make(chan error, 2)
	translate := func(user string) {
		_, err := e.c.app.Translation.Translate(context.WithoutCancel(t.Context()), viewer(user), st.Id, "ja")
		results <- err
	}
	go translate("bob")
	<-started
	// The limit leaves no room beyond the call under way.
	sent := translation.CountCharacters(e.one.requests[0].Fragments)
	e.set(instance.SettingTranslationServerMonthlyCharacters, strconv.Itoa(sent))
	go translate("carol")
	time.Sleep(300 * time.Millisecond)
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Errorf("Translate: %v", err)
		}
	}
	if got := e.overview().Characters; got != int64(sent) {
		t.Errorf("characters = %d, want %d", got, sent)
	}
}

// TestAdminTranslationOverview checks the numbers of the admin screen, its permission, and that no
// API answer holds what stays in the configuration.
func TestAdminTranslationOverview(t *testing.T) {
	t.Parallel()
	e := newConfiguredTranslation(t)
	got := e.overview()
	if got.Characters != 0 || got.CacheHitRate != nil || len(got.Providers) != 2 || got.Providers[0].Id != "one" {
		t.Errorf("overview before any use = %+v", got)
	}
	e.set(instance.SettingTranslationProviders, "one")
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	for _, user := range []string{"bob", "carol", "bob", "carol"} {
		if res, _ := e.translate(user, st.Id); res.StatusCode != http.StatusOK {
			t.Fatalf("translate = %d", res.StatusCode)
		}
	}
	got = e.overview()
	if got.CacheHits != 3 || got.CacheMisses != 1 || got.CacheHitRate == nil || *got.CacheHitRate != 0.75 {
		t.Errorf("overview = %+v, want 3 hits of 4", got)
	}
	if got.Characters == 0 || got.Month != time.Now().UTC().Format("2006-01") {
		t.Errorf("overview = %+v", got)
	}
	if got.Providers[1].Id != "two" {
		t.Errorf("a provider that is not enabled is not offered: %+v", got.Providers)
	}

	res := e.c.do("bob", http.MethodGet, "/admin/translation", nil, nil)
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(body)), "key") {
		t.Errorf("the answer names a key: %s", body)
	}
}

// TestTranslationRetentionSetting checks that the purge job reads the setting.
func TestTranslationRetentionSetting(t *testing.T) {
	t.Parallel()
	e := newConfiguredTranslation(t)
	e.set(instance.SettingTranslationProviders, "one")
	ctx := t.Context()
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})
	if res, _ := e.translate("bob", st.Id); res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	handler, _, ok := e.c.app.Handlers.Lookup(translation.JobPurgeCache)
	if !ok {
		t.Fatal("no purge job")
	}
	purge := func() {
		t.Helper()
		if res := handler(ctx, queue.Job{Type: translation.JobPurgeCache}); res.Err() != nil {
			t.Fatal(res.Err())
		}
	}
	e.ahead.Store(int64(3 * 24 * time.Hour))
	purge()
	if n := e.rows("status_translations"); n != 1 {
		t.Fatalf("rows = %d after 3 of 30 days", n)
	}
	e.set(instance.SettingTranslationRetentionDays, "2")
	purge()
	if n := e.rows("status_translations"); n != 0 {
		t.Errorf("rows = %d after 3 of 2 days", n)
	}
}
