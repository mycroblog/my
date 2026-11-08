// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

// TestDefaultLocaleAppliesWithoutRestart checks that every HTTP surface resolves the locale
// of a request without Accept-Language from the current default_locale setting.
func TestDefaultLocaleAppliesWithoutRestart(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h := app.Handler()

	paths := map[string]string{
		"platform problem":   "/ap/nope",
		"client API problem": "/api/v1/nope",
		"sign-in screen":     authpages.PathSignIn,
	}
	check := func(want string, header map[string]string) {
		t.Helper()
		for name, path := range paths {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
			for k, v := range header {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if got := w.Header().Get("Content-Language"); got != want {
				t.Errorf("%s (%s, %v): Content-Language = %q, want %q", name, path, header, got, want)
			}
		}
	}

	check("en", nil)
	if _, err := app.Instance.SetSetting(t.Context(), appinstance.SetSettingInput{Key: instance.SettingDefaultLocale, Value: "ja"}); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	check("ja", nil)
	check("en", map[string]string{"Accept-Language": "en-US"})
	check("ja", map[string]string{"Accept-Language": "it"})
}

// TestPseudoLocales checks that the pseudo-locales are selectable on the sign-in screen only
// with AppOptions.PseudoLocales (`my serve -pseudo-locales`), and that ar-XB renders right to left.
func TestPseudoLocales(t *testing.T) {
	t.Parallel()
	render := func(t *testing.T, pseudo bool, acceptLanguage string) (string, string) {
		t.Helper()
		cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
		app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), PseudoLocales: pseudo})
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
		t.Cleanup(func() { _ = app.Close() })
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, authpages.PathSignIn, nil)
		req.Header.Set("Accept-Language", acceptLanguage)
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, req)
		return w.Header().Get("Content-Language"), w.Body.String()
	}

	for _, locale := range []string{"en-XA", "ar-XB"} {
		if got, _ := render(t, false, locale); got != "en" {
			t.Errorf("without PseudoLocales, %s: Content-Language = %q, want en", locale, got)
		}
	}
	for locale, dir := range map[string]string{"en-XA": "ltr", "ar-XB": "rtl"} {
		got, body := render(t, true, locale)
		if got != locale {
			t.Errorf("%s: Content-Language = %q", locale, got)
		}
		if html := `<html lang="` + locale + `" dir="` + dir + `">`; !strings.Contains(body, html) {
			t.Errorf("%s: the page does not contain %s", locale, html)
		}
	}
}
