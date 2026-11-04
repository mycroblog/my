// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
)

// TestCrawlerStatements checks ADR-0057 end to end: an instance without settings denies search
// engines and AI everywhere, the settings change /robots.txt and the headers of pages and
// media, new accounts take indexable_default, and responses that are not pages are noindex.
func TestCrawlerStatements(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	register := func(username string) string {
		t.Helper()
		reg, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{
			Username: username, Password: "correct horse battery staple", DisplayName: username,
		})
		if err != nil {
			t.Fatal(err)
		}
		return reg.AccountID
	}
	set := func(values map[instance.SettingKey]string) {
		t.Helper()
		var ins []appinstance.SetSettingInput
		for k, v := range values {
			ins = append(ins, appinstance.SetSettingInput{Key: k, Value: v})
		}
		if _, err := app.Instance.SetSettings(t.Context(), ins); err != nil {
			t.Fatal(err)
		}
	}
	h := app.Handler()
	get := func(path, accept string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	aliceID := register("alice")

	// Without settings, every page denies everything, and robots.txt names the AI crawlers.
	robots := get("/robots.txt", "")
	if robots.Code != http.StatusOK || !strings.Contains(robots.Body.String(), "User-agent: GPTBot\n") ||
		!strings.Contains(robots.Body.String(), "Content-Usage: search=n, train-ai=n, ai-use=n\n") ||
		robots.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("GET /robots.txt = %d\n%s", robots.Code, robots.Body)
	}
	for _, path := range []string{"/", "/@alice", "/home"} {
		if got := get(path, acceptBrowser).Header().Get("X-Robots-Tag"); got != "noindex, noarchive, noai, noimageai" {
			t.Errorf("GET %s: X-Robots-Tag = %q", path, got)
		}
	}
	if rec := get("/.well-known/tdmrep.json", ""); rec.Code != http.StatusOK || rec.Header().Get("X-Robots-Tag") != "noindex" {
		t.Errorf("GET tdmrep.json = %d %v", rec.Code, rec.Header())
	}
	if rec := get("/.well-known/security.txt", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET security.txt without a contact = %d, want 404", rec.Code)
	}
	media := get("/media/a/b.png", "")
	if media.Header().Get("X-Robots-Tag") != "noindex, noarchive, noai, noimageai" || media.Header().Get("Content-Usage") != "train-ai=n, ai-use=n" {
		t.Errorf("GET /media/: headers %v", media.Header())
	}

	// Responses that are not pages are noindex: the client API, ActivityPub, and NodeInfo.
	for _, tc := range []struct{ path, accept string }{
		{"/api/v1/instance", ""}, {"/ap/actors/" + aliceID, acceptMastodon}, {"/.well-known/nodeinfo", ""}, {"/nodeinfo/2.1", ""},
	} {
		if rec := get(tc.path, tc.accept); rec.Header().Get("X-Robots-Tag") != "noindex" {
			t.Errorf("GET %s = %d, X-Robots-Tag %q", tc.path, rec.Code, rec.Header().Get("X-Robots-Tag"))
		}
	}

	// Allowing search and AI: alice registered while indexable_default was false, so her pages
	// stay out of search engines; bob, registered after it became true, is indexed.
	set(map[instance.SettingKey]string{
		instance.SettingSearchIndexing: "allow", instance.SettingAIUsage: "allow", instance.SettingIndexableDefault: "true",
		instance.SettingSecurityContact: "mailto:security@social.example",
	})
	register("bob")
	// The supporter list is never for search engines (ADR-0070 §13): its page, and its API with
	// its problems.
	for path, want := range map[string]string{
		"/": "", "/@alice": "noindex, noarchive", "/@bob": "", "/home": "noindex, noarchive",
		"/about/supporters": "noindex, noarchive", "/api/v1/instance/supporters": "noindex, noarchive",
	} {
		if got := get(path, acceptBrowser).Header().Get("X-Robots-Tag"); got != want {
			t.Errorf("GET %s: X-Robots-Tag = %q, want %q", path, got, want)
		}
	}
	if body := get("/robots.txt", "").Body.String(); strings.Contains(body, "User-agent: GPTBot\n") ||
		!strings.Contains(body, "Content-Usage: search=y, train-ai=y, ai-use=y\n") {
		t.Errorf("GET /robots.txt with everything allowed:\n%s", body)
	}
	if rec := get("/.well-known/tdmrep.json", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET tdmrep.json with AI allowed = %d, want 404", rec.Code)
	}
	if rec := get("/.well-known/security.txt", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Contact: mailto:security@social.example\n") {
		t.Errorf("GET security.txt = %d\n%s", rec.Code, rec.Body)
	}
	if media := get("/media/a/b.png", ""); media.Header().Get("X-Robots-Tag") != "" || media.Header().Get("Content-Usage") != "" {
		t.Errorf("GET /media/ with everything allowed: headers %v", media.Header())
	}
}
