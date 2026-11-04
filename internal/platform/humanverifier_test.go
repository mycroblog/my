// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform"
)

// recordingHTTP answers every request with a passing siteverify answer and records it.
type recordingHTTP struct {
	mu   sync.Mutex
	reqs []*kernel.HTTPRequest
}

func (h *recordingHTTP) Do(_ context.Context, r *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	h.mu.Lock()
	h.reqs = append(h.reqs, r)
	h.mu.Unlock()
	body := `{"success":true,"hostname":"example.test","action":"sign_up"}`
	return &kernel.HTTPResponse{StatusCode: http.StatusOK, Header: http.Header{}, URL: r.URL, Body: io.NopCloser(strings.NewReader(body))}, nil
}

// TestHumanVerifier checks that captcha = none builds no verifier, and that a configured
// service gets a verifier and widget that send through the App's HTTPClient (ADR-0106).
func TestHumanVerifier(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		client := &recordingHTTP{}
		app, err := platform.NewApp(t.Context(), loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, false)}),
			platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: client})
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
		t.Cleanup(func() { _ = app.Close() })
		if app.HumanVerifier != nil || app.HumanCheckWidget != nil {
			t.Errorf("captcha = none built %v, %+v; want nil", app.HumanVerifier, app.HumanCheckWidget)
		}
		if len(client.reqs) != 0 {
			t.Errorf("NewApp sent %d requests", len(client.reqs))
		}
	})
	t.Run("turnstile", func(t *testing.T) {
		client := &recordingHTTP{}
		cfg := loadConfig(t, map[string]string{
			"MY_DATABASE_URL": newDatabase(t, false),
			"MY_CAPTCHA":      "turnstile", "MY_CAPTCHA_SITE_KEY": "site-key", "MY_CAPTCHA_SECRET_KEY": "secret-key",
		})
		app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: client})
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
		t.Cleanup(func() { _ = app.Close() })
		if app.HumanVerifier == nil || app.HumanCheckWidget == nil {
			t.Fatal("captcha = turnstile built no verifier or widget")
		}
		if w := app.HumanCheckWidget; w.Service != kernel.HumanCheckTurnstile || w.SiteKey != "site-key" || w.ResponseField != "cf-turnstile-response" {
			t.Errorf("widget = %+v", w)
		}
		if len(client.reqs) != 0 {
			t.Errorf("NewApp sent %d requests; the key is not tried at start", len(client.reqs))
		}
		if err := app.HumanVerifier.Verify(t.Context(), "token", kernel.HumanCheckActionSignUp); err != nil {
			t.Fatalf("Verify() = %v", err)
		}
		if len(client.reqs) != 1 || client.reqs[0].Purpose != kernel.HTTPPurposeHumanCheck {
			t.Errorf("requests = %+v, want one with HTTPPurposeHumanCheck", client.reqs)
		}
	})
}
