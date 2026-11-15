// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestE2EFakesTranslator starts the test build with translator_fake = true and profile =
// development, and enables the provider fake as the E2E demo does: it puts the target language
// before each text, fails as a provider that is down on "[translation-fails]", and the server's
// own rules refuse a private post and a request over the monthly limit.
func TestE2EFakesTranslator(t *testing.T) {
	c := newStatusesClientWith(t, map[string]string{"MY_PROFILE": "development", "MY_TRANSLATOR_FAKE": "true"}, platform.AppOptions{}, "alice", "bob")
	set := func(key instance.SettingKey, value string) {
		t.Helper()
		if _, err := c.app.Instance.SetSetting(t.Context(), appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatalf("SetSetting(%s): %v", key, err)
		}
	}
	set(instance.SettingTranslationProviders, "fake")
	translate := func(statusID string) (*http.Response, clientapi.StatusTranslation, clientapi.Problem) {
		t.Helper()
		var out clientapi.StatusTranslation
		res := c.do("bob", http.MethodPost, "/statuses/"+statusID+"/translation", map[string]any{"target_language": "ja"}, &out)
		var p clientapi.Problem
		if res.StatusCode != http.StatusOK {
			_ = json.NewDecoder(res.Body).Decode(&p)
		}
		return res, out, p
	}

	st := c.post("alice", map[string]any{
		"text": "Hello", "spoiler_text": "careful", "language": "en", "visibility": "public",
		"poll": map[string]any{"options": []string{"Yes", "No"}, "expires_in": 3600},
	})
	res, got, _ := translate(st.Id)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("translate = %d", res.StatusCode)
	}
	if got.Provider == nil || got.Provider.Id != "fake" {
		t.Errorf("provider = %+v, want fake", got.Provider)
	}
	if !strings.Contains(got.Content, "[ja] Hello") || got.SpoilerText != "[ja] careful" || !slices.Equal(got.PollOptions, []string{"[ja] Yes", "[ja] No"}) {
		t.Errorf("translation = %q, %q, %q", got.Content, got.SpoilerText, got.PollOptions)
	}

	failing := c.post("alice", map[string]any{"text": "Hello [translation-fails]", "language": "en", "visibility": "public"})
	if res, _, p := translate(failing.Id); res.StatusCode != http.StatusServiceUnavailable || p.Type != clientapi.ProblemTypeTranslationFailed {
		t.Errorf("translate a failing text = %d %q, want 503 %s", res.StatusCode, p.Type, clientapi.ProblemTypeTranslationFailed)
	}

	if _, err := c.app.Graph.Follow(t.Context(), c.ids["bob"], c.ids["alice"]); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	private := c.post("alice", map[string]any{"text": "Hello", "language": "en", "visibility": "private"})
	if res, _, p := translate(private.Id); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeTranslationUnavailable {
		t.Errorf("translate a private post = %d %q, want 422 %s", res.StatusCode, p.Type, clientapi.ProblemTypeTranslationUnavailable)
	}

	set(instance.SettingTranslationUserMonthlyCharacters, "40")
	long := c.post("alice", map[string]any{"text": strings.Repeat("words ", 10), "language": "en", "visibility": "public"})
	if res, _, p := translate(long.Id); res.StatusCode != http.StatusTooManyRequests || p.Type != clientapi.ProblemTypeTranslationLimit {
		t.Errorf("translate over the limit = %d %q, want 429 %s", res.StatusCode, p.Type, clientapi.ProblemTypeTranslationLimit)
	}
}
