// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
)

// TestE2EFakesSearch starts the test build with search = fake and profile = development: the
// server has a backend, and its start warns that this is a test build (ADR-0149 §3).
func TestE2EFakesSearch(t *testing.T) {
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_PROFILE": "development", "MY_SEARCH": "fake",
	})
	var logs bytes.Buffer
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !app.Search.Enabled() {
		t.Error("Search.Enabled() = false with search = fake")
	}
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "this is a test build") {
		t.Errorf("the start logged no warning of the test build:\n%s", logs.String())
	}
}
