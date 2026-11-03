// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// TestSourceURL checks that GET /instance and NodeInfo offer the source where the setting source_url says (AGPL-3.0
// section 13): the upstream repository by default, and the operator's own https URL or mailto: address when set.
// NodeInfo's software.repository takes only http(s) URLs, so a mailto: address is in metadata.sourceCode only.
func TestSourceURL(t *testing.T) {
	for _, want := range []string{"", "https://git.example.org/fork/my", "mailto:source@example.org"} {
		env := map[string]string{"MY_DATABASE_URL": newDatabase(t, true)}
		if want != "" {
			env["MY_SOURCE_URL"] = want
		} else {
			want = config.DefaultSourceURL
		}
		cfg := loadConfig(t, env)
		app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
		t.Cleanup(func() { _ = app.Close() })
		get := func(path string) []byte {
			t.Helper()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+cfg.Host+path, nil)
			rec := httptest.NewRecorder()
			app.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body)
			}
			return rec.Body.Bytes()
		}
		var instance struct {
			SourceURL string `json:"source_url"`
		}
		if err := json.Unmarshal(get("/api/v1/instance"), &instance); err != nil || instance.SourceURL != want {
			t.Errorf("GET /instance source_url = %q (%v), want %q", instance.SourceURL, err, want)
		}
		var nodeInfo struct {
			Software struct{ Repository string }
			Metadata struct{ SourceCode string }
		}
		wantRepository := want
		if strings.HasPrefix(want, "mailto:") {
			wantRepository = ""
		}
		if err := json.Unmarshal(get("/nodeinfo/2.1"), &nodeInfo); err != nil || nodeInfo.Software.Repository != wantRepository || nodeInfo.Metadata.SourceCode != want {
			t.Errorf("NodeInfo = %+v (%v), want software.repository %q and metadata.sourceCode %q", nodeInfo, err, wantRepository, want)
		}
	}
}

// TestSourceURLOfARelease checks that a release's GET /instance offers the source at the version's tag by default, while
// NodeInfo stays at the repository.
func TestSourceURLOfARelease(t *testing.T) {
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Version: "v0.1.0", Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	get := func(path string) []byte {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+cfg.Host+path, nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body)
		}
		return rec.Body.Bytes()
	}
	var instance struct {
		SourceURL string `json:"source_url"`
	}
	if err := json.Unmarshal(get("/api/v1/instance"), &instance); err != nil || instance.SourceURL != config.DefaultSourceURL+"/tree/v0.1.0" {
		t.Errorf("GET /instance source_url = %q (%v)", instance.SourceURL, err)
	}
	var nodeInfo struct {
		Software struct{ Repository string }
	}
	if err := json.Unmarshal(get("/nodeinfo/2.1"), &nodeInfo); err != nil || nodeInfo.Software.Repository != config.DefaultSourceURL {
		t.Errorf("NodeInfo software.repository = %q (%v)", nodeInfo.Software.Repository, err)
	}
}
