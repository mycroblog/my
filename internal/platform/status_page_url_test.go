// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mycroblog/my/internal/platform"
)

// TestStatusPageURL checks that GET /instance has status_page_url only when the setting is set (ADR-0085 §6).
func TestStatusPageURL(t *testing.T) {
	for _, want := range []string{"", "https://status.example.org"} {
		env := map[string]string{"MY_DATABASE_URL": newDatabase(t, true)}
		if want != "" {
			env["MY_STATUS_PAGE_URL"] = want
		}
		cfg := loadConfig(t, env)
		app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
		t.Cleanup(func() { _ = app.Close() })
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+cfg.Host+"/api/v1/instance", nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		var instance map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &instance); rec.Code != http.StatusOK || err != nil {
			t.Fatalf("GET /instance = %d %s (%v)", rec.Code, rec.Body, err)
		}
		got, ok := instance["status_page_url"]
		if want == "" && ok || want != "" && got != want {
			t.Errorf("status_page_url = %v (present %t), want %q", got, ok, want)
		}
	}
}
