// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	translatorplugin "github.com/mycroblog/my/internal/adapter/translator/plugin"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

const pluginTranslatorName = "Our translator"

// newPluginTranslation is a server with a plugin of the translator kind, which the settings do not enable yet, and a user
// bob who follows alice. address is where the plugin is, and extra are other providers that the server has.
func newPluginTranslation(t *testing.T, address string, extra ...translation.Provider) *configuredTranslation {
	t.Helper()
	path := newDatabase(t, true)
	e := &configuredTranslation{}
	e.c = newStatusesClientWith(t, map[string]string{
		"MY_DATABASE_URL": path, "MY_PLUGIN_TRANSLATOR_ADDRESS": address, "MY_PLUGIN_TRANSLATOR_NAME": pluginTranslatorName,
	}, platform.AppOptions{TranslationRegistered: extra}, "alice", "bob", "carol")
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

func (e *configuredTranslation) storedTranslations() int {
	e.c.t.Helper()
	var n int
	if err := e.db.QueryRowContext(e.c.t.Context(), `SELECT COUNT(*) FROM status_translations`).Scan(&n); err != nil {
		e.c.t.Fatal(err)
	}
	return n
}

// The plugin is one server-side provider: the settings enable it and say what it may receive, its name is what clients see,
// the limits count it, and the server checks its answers as any other's (ADR-0148 §5, §8, §11, §14).
func TestTranslationByAPlugin(t *testing.T) {
	t.Parallel()
	service := newRecordingTranslator()
	h, _ := fakeplugin.JSON(fakeplugin.Options{Translator: service})
	address := fakeplugin.ServeUnix(t, h)
	e := newPluginTranslation(t, address)
	ctx := t.Context()

	public := e.c.post("alice", map[string]any{"text": "public words @carol https://example.com/page #tag", "visibility": "public", "language": "en"})
	private := e.c.post("alice", map[string]any{"text": "private words", "visibility": "private", "language": "en"})
	direct := e.c.post("alice", map[string]any{"text": "@bob direct words", "visibility": "direct", "language": "en"})

	// The server has the provider and sends nothing until the settings enable it.
	over, err := e.c.app.Translation.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []translation.ProviderInfo{{ID: translatorplugin.ProviderID, Name: pluginTranslatorName}}; !slices.Equal(over.Registered, want) {
		t.Errorf("registered = %+v", over.Registered)
	}
	e.wantProblem("bob", public.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	if n := service.calls(); n != 0 {
		t.Fatalf("%d calls before the settings enable it", n)
	}

	e.set(instance.SettingTranslationProviders, translatorplugin.ProviderID)

	// What clients see is the name, and the default visibilities, never the destination.
	got := e.instance()
	if len(got.Providers) != 1 || got.Providers[0].Id != translatorplugin.ProviderID || got.Providers[0].Name != pluginTranslatorName || got.Providers[0].Url != nil {
		t.Fatalf("instance translation = %+v", got)
	}
	if want := []clientapi.Visibility{"public", "local", "unlisted"}; !slices.Equal(got.Providers[0].Visibilities, want) {
		t.Errorf("visibilities = %v, want %v", got.Providers[0].Visibilities, want)
	}
	socket := strings.TrimPrefix(address, "unix://")
	for name, v := range map[string]any{"instance": got, "overview": over} {
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), socket) {
			t.Errorf("the %s holds the destination: %s", name, body)
		}
	}
	for _, key := range []instance.SettingKey{"plugin_translator_address", "plugin_translator_name", "plugin_translator_token"} {
		if _, err := instance.Lookup(key); err == nil {
			t.Errorf("%s is an instance setting", key)
		}
	}

	// A translation carries the name, and the plugin receives the words and nothing that names anyone or anything.
	res, out := e.translate("bob", public.Id)
	if res.StatusCode != http.StatusOK || out.Provider == nil || out.Provider.Id != translatorplugin.ProviderID || out.Provider.Name != pluginTranslatorName {
		t.Fatalf("translate = %d, %+v", res.StatusCode, out)
	}
	sent := strings.Join(service.lastRequest().Fragments, "\n")
	for _, name := range []string{"carol", "example.com", "alice", "tag"} {
		if strings.Contains(sent, name) {
			t.Errorf("the plugin received %q: %q", name, sent)
		}
	}
	if !strings.Contains(sent, "public words") || !strings.Contains(sent, "⟦1⟧") {
		t.Errorf("the plugin received %q", sent)
	}

	// private and direct go only where the operator allows them, and allowing one does not allow the other.
	calls := service.calls()
	e.wantProblem("bob", private.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	e.wantProblem("bob", direct.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	if n := service.calls(); n != calls {
		t.Errorf("%d calls for private and direct by default", n-calls)
	}
	e.set(instance.SettingTranslationProviderVisibilities, translatorplugin.ProviderID+"=private")
	if res, _ := e.translate("bob", private.Id); res.StatusCode != http.StatusOK {
		t.Errorf("translate private when allowed = %d", res.StatusCode)
	}
	e.wantProblem("bob", direct.Id, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	e.set(instance.SettingTranslationProviderVisibilities, "")

	// Markup in an answer does not survive. The markers still decide whether the answer is one.
	withScript := e.c.post("alice", map[string]any{"text": "words with @carol", "visibility": "public", "language": "en"})
	service.setAnswer(func(_ context.Context, req translation.Request) (translation.Result, error) {
		out := make([]string, len(req.Fragments))
		for i, f := range req.Fragments {
			out[i] = `<script>alert(1)</script><img src=x onerror=alert(2)><a href="https://evil.example/">` + f + `</a>`
		}
		return translation.Result{Translations: out, DetectedLanguage: "<b>en</b>"}, nil
	})
	res, out = e.translate("bob", withScript.Id)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("translate with markup = %d", res.StatusCode)
	}
	for _, tag := range []string{"<script", "<img", "evil.example\">", "<a href=\"https://evil"} {
		if strings.Contains(out.Content, tag) {
			t.Errorf("the translation holds %q: %s", tag, out.Content)
		}
	}
	if out.SourceLanguage != "en" {
		t.Errorf("source language = %q", out.SourceLanguage)
	}

	// An answer that loses a marker fails and stores nothing.
	lost := e.c.post("alice", map[string]any{"text": "words of @carol again", "visibility": "public", "language": "en"})
	stored := e.storedTranslations()
	service.setAnswer(func(_ context.Context, req translation.Request) (translation.Result, error) {
		return translation.Result{Translations: slices.Repeat([]string{"no markers here"}, len(req.Fragments))}, nil
	})
	e.wantProblem("bob", lost.Id, http.StatusServiceUnavailable, clientapi.ProblemTypeTranslationFailed)
	if n := e.storedTranslations(); n != stored {
		t.Errorf("stored %d translations of an answer that failed", n-stored)
	}

	// The monthly limits stop the call.
	calls = service.calls()
	e.set(instance.SettingTranslationUserMonthlyCharacters, "0")
	e.wantProblem("bob", lost.Id, http.StatusTooManyRequests, clientapi.ProblemTypeTranslationLimit)
	if n := service.calls(); n != calls {
		t.Errorf("%d calls over the limit", n-calls)
	}
}

// A plugin that is down at the start stops nothing: the translation fails, the post stays, and the next provider answers
// (ADR-0146 §15, ADR-0148 §8).
func TestTranslationWithAPluginThatIsDown(t *testing.T) {
	t.Parallel()
	next := newRecordingTranslator()
	e := newPluginTranslation(t, "unix:///nowhere/translator.sock", translation.Provider{ID: "next", Name: "Next", Translator: next})
	st := e.c.post("alice", map[string]any{"text": "some words", "visibility": "public", "language": "en"})

	e.set(instance.SettingTranslationProviders, translatorplugin.ProviderID)
	e.wantProblem("bob", st.Id, http.StatusServiceUnavailable, clientapi.ProblemTypeTranslationFailed)

	e.set(instance.SettingTranslationProviders, translatorplugin.ProviderID+",next")
	res, out := e.translate("bob", st.Id)
	if res.StatusCode != http.StatusOK || out.Provider == nil || out.Provider.Id != "next" {
		t.Errorf("translate = %d, %+v", res.StatusCode, out)
	}
}
