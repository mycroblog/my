// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

const webOrigin = "http://localhost:5173"

func newClientAPIHandler(t *testing.T) http.Handler {
	t.Helper()
	return newClientAPIApp(t).Handler()
}

func newClientAPIApp(t *testing.T) *platform.App {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Version: "1.2.3"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// TestShellNamesTheVersion checks that the server's version reaches the shell, from which the web client's
// error reports read it.
func TestShellNamesTheVersion(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	newClientAPIHandler(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/home", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":"1.2.3"`) {
		t.Errorf("GET /home = %d, want the version 1.2.3 in its runtime config:\n%s", rec.Code, rec.Body.String())
	}
}

// TestClientAPI checks that the composition root serves the client API under /api/v1 with the
// security headers of every response and CORS for the configured web origin.
func TestClientAPI(t *testing.T) {
	t.Parallel()
	h := newClientAPIHandler(t)
	spec := apitest.Load(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	req.Header.Set("Origin", webOrigin)
	res := spec.Serve(t, h, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/instance = %d", res.StatusCode)
	}
	var info clientapi.Instance
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Domain != "example.test" || info.Name != "example.test" || info.Version != "1.2.3" ||
		info.Limits.MaxPostCharacters != 500 || info.Registrations.Mode != clientapi.InstanceRegistrationsModeApproval || info.SendsEmail {
		t.Errorf("instance = %+v, want example.test, version 1.2.3, the default settings, no email (mail = none)", info)
	}
	for k, want := range map[string]string{
		"Access-Control-Allow-Origin":   webOrigin,
		"Access-Control-Expose-Headers": "Content-Language, X-Request-Id, WWW-Authenticate, Retry-After, RateLimit, RateLimit-Policy",
		"Vary":                          "Origin",
		"X-Content-Type-Options":        "nosniff",
		"Cross-Origin-Resource-Policy":  "same-site",
		"Strict-Transport-Security":     "max-age=63072000; includeSubDomains",
	} {
		if got := res.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got := res.Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want none", got)
	}
}

func TestClientAPICORS(t *testing.T) {
	t.Parallel()
	h := newClientAPIHandler(t)
	preflight := func(origin string) *http.Response {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/instance", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		req.Header.Set("Access-Control-Request-Headers", "authorization")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result()
	}

	res := preflight(webOrigin)
	if res.StatusCode != http.StatusNoContent || res.Header.Get("Access-Control-Allow-Origin") != webOrigin ||
		res.Header.Get("Access-Control-Allow-Headers") == "" || res.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("preflight from the web origin = %d %v, want 204 allowing it", res.StatusCode, res.Header)
	}
	res = preflight("https://evil.example")
	if res.Header.Get("Access-Control-Allow-Origin") != "" || res.Header.Get("Access-Control-Allow-Methods") != "" {
		t.Errorf("preflight from another origin = %v, want no CORS allowance", res.Header)
	}

	// Requests outside the client API get no CORS headers.
	req := httptest.NewRequest(http.MethodGet, platform.PathLiveness, nil)
	req.Header.Set("Origin", webOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("%s Access-Control-Allow-Origin = %q, want none", platform.PathLiveness, got)
	}
}

// TestClientAPIFallbackProblems checks that requests under /api/v1 that match no operation get
// localized problem documents that the web client can read across origins.
func TestClientAPIFallbackProblems(t *testing.T) {
	t.Parallel()
	h := newClientAPIHandler(t)
	tests := []struct {
		method, path string
		wantStatus   int
		wantTitle    string
	}{
		{http.MethodGet, "/api/v1/nope", http.StatusNotFound, "指定されたリソースが見つかりません。"},
		{http.MethodDelete, "/api/v1/instance", http.StatusMethodNotAllowed, ""},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		req.Header.Set("Origin", webOrigin)
		req.Header.Set("Accept-Language", "ja")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		res := rec.Result()
		body, _ := io.ReadAll(res.Body)
		p := decodeProblem(t, res, string(body))
		if res.StatusCode != tt.wantStatus || res.Header.Get("Content-Language") != "ja" || (tt.wantTitle != "" && p.Title != tt.wantTitle) {
			t.Errorf("%s %s = %d %s (Content-Language %q), want %d in ja", tt.method, tt.path, res.StatusCode, body, res.Header.Get("Content-Language"), tt.wantStatus)
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != webOrigin {
			t.Errorf("%s %s Access-Control-Allow-Origin = %q, want %q", tt.method, tt.path, got, webOrigin)
		}
	}
}

// TestClientAPIInstanceSettings checks that GET /api/v1/instance reports the instance settings
// and texts, read per request.
func TestClientAPIInstanceSettings(t *testing.T) {
	t.Parallel()
	app := newClientAPIApp(t)
	ctx := t.Context()
	for key, value := range map[instance.SettingKey]string{
		instance.SettingMaxStatusLength: "1000", instance.SettingRegistrationMode: "open",
	} {
		if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	for _, in := range []appinstance.SetTextInput{
		{Key: instance.TextName, Body: "Example"},
		{Key: instance.TextName, Body: "例"},
		{Key: instance.TextDescription, Body: "About us"},
	} {
		if _, err := app.Instance.SetText(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	spec := apitest.Load(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	req.Header.Set("Accept-Language", "ja")
	res := spec.Serve(t, app.Handler(), req)
	var got clientapi.Instance
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "例" || got.Description == nil || *got.Description != "About us" || got.ShortDescription != nil ||
		got.Limits.MaxPostCharacters != 1000 || got.Registrations.Mode != clientapi.InstanceRegistrationsModeOpen {
		t.Errorf("instance = %+v (description %v), want the stored settings and the latest name", got, got.Description)
	}
}

// TestClientAPIInstanceRules checks that GET /api/v1/instance lists the rules in effect from
// the database, without the retired ones.
func TestClientAPIInstanceRules(t *testing.T) {
	t.Parallel()
	app := newClientAPIApp(t)
	ctx := t.Context()
	kind, err := app.Instance.AddRule(ctx, appinstance.AddRuleInput{Text: "親切に", Hint: "嫌がらせ禁止"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := app.Instance.AddRule(ctx, appinstance.AddRuleInput{Text: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Instance.RetireRule(ctx, old.ID, ""); err != nil {
		t.Fatal(err)
	}
	spec := apitest.Load(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	req.Header.Set("Accept-Language", "ja-JP")
	res := spec.Serve(t, app.Handler(), req)
	var got clientapi.Instance
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 1 || got.Rules[0].Id != kind.ID || got.Rules[0].Text != "親切に" ||
		got.Rules[0].Hint == nil || *got.Rules[0].Hint != "嫌がらせ禁止" {
		t.Errorf("rules = %+v, want the rule in effect", got.Rules)
	}
}
