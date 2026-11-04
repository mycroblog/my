// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/account"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
)

// TestLinkPreviews checks end to end which profile and post pages carry link preview meta
// tags (ADR-0028): public and unlisted posts of active local accounts do; every other post
// page, whatever the reason, is the same generic shell with 404 and noindex.
func TestLinkPreviews(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	build := fstest.MapFS{
		".vite/manifest.json":  {Data: []byte(`{"index.html": {"file": "assets/index-AAAA.js", "isEntry": true}}`)},
		"assets/index-AAAA.js": {Data: []byte("export {}")},
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), WebBuild: build})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	// The instance lets search engines index the accounts that allow it, which new accounts do
	// (ADR-0057), so that only unlisted and hidden pages are noindex.
	if _, err := app.Instance.SetSettings(ctx, []appinstance.SetSettingInput{
		{Key: instance.SettingSearchIndexing, Value: "allow"}, {Key: instance.SettingAIUsage, Value: "allow"},
		{Key: instance.SettingIndexableDefault, Value: "true"},
	}); err != nil {
		t.Fatal(err)
	}
	accounts := map[string]string{}
	for _, name := range []string{"alice", "bob"} {
		reg, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: name, Password: "correct horse battery staple", DisplayName: strings.ToUpper(name[:1]) + name[1:], Locale: "ja",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser(%s): %v", name, err)
		}
		accounts[name] = reg.AccountID
	}
	post := func(author string, vis domcontent.Visibility, text string) string {
		t.Helper()
		p, err := app.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: accounts[author], Visibility: vis, TextInput: appcontent.TextInput{Text: text, Language: "ja"},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return p.Status.ID
	}
	ids := map[string]string{}
	for _, vis := range domcontent.Visibilities() {
		ids[string(vis)] = post("alice", vis, "a "+string(vis)+" post <b>@bob</b>")
	}
	ids["deleted"] = post("alice", domcontent.VisibilityPublic, "deleted")
	if _, err := app.Content.Delete(ctx, accounts["alice"], ids["deleted"]); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ids["inactive author"] = post("bob", domcontent.VisibilityPublic, "bob's post")
	if _, err := app.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: accounts["bob"], Kind: account.LifecycleDeactivated, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatalf("RecordLifecycleEvent: %v", err)
	}
	ids["missing"] = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"
	h := app.Handler()
	getAs := func(path, accept string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.Host+path, nil)
		req.Header.Set("Accept", accept)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	get := func(path string) *httptest.ResponseRecorder { return getAs(path, acceptBrowser) }

	for _, vis := range []string{"public", "unlisted"} {
		rec := get("/@alice/status/" + ids[vis])
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, `<meta property="og:url" content="https://`+cfg.Host+`/@alice/status/`+ids[vis]+`">`) ||
			!strings.Contains(body, `<meta property="og:description" content="a `+vis+` post &lt;b&gt;@bob&lt;/b&gt;">`) ||
			!strings.Contains(body, `<html lang="ja" dir="ltr">`) || !strings.Contains(body, `<meta name="fediverse:creator" content="@alice@`) {
			t.Errorf("%s post = %d:\n%s", vis, rec.Code, body)
		}
		if noindex := rec.Header().Get("X-Robots-Tag") == "noindex, noarchive"; noindex != (vis == "unlisted") {
			t.Errorf("%s post: X-Robots-Tag = %q", vis, rec.Header().Get("X-Robots-Tag"))
		}
		// A remote server resolving the page's URL (FR-305) is redirected to the post's id (ADR-0018).
		want := "https://" + cfg.Host + "/ap/objects/" + ids[vis]
		if rec := getAs("/@alice/status/"+ids[vis], acceptMastodon); rec.Code != http.StatusFound || rec.Header().Get("Location") != want {
			t.Errorf("%s post, ActivityPub = %d %q, want 302 %q", vis, rec.Code, rec.Header().Get("Location"), want)
		}
	}
	if rec := get("/@alice"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<meta property="og:type" content="profile">`) {
		t.Errorf("profile page = %d:\n%s", rec.Code, rec.Body)
	}

	generic := get("/home")
	var hidden []*httptest.ResponseRecorder
	for _, path := range []string{
		"/@alice/status/" + ids["local"], "/@alice/status/" + ids["private"], "/@alice/status/" + ids["direct"], "/@alice/status/" + ids["deleted"],
		"/@alice/status/" + ids["missing"], "/@bob/status/" + ids["inactive author"], "/@bob", "/@nobody",
	} {
		rec := get(path)
		if rec.Code != http.StatusNotFound || rec.Header().Get("X-Robots-Tag") != "noindex, noarchive" || rec.Body.String() != generic.Body.String() {
			t.Errorf("GET %s = %d %v, want the generic shell with 404 and noindex:\n%s", path, rec.Code, rec.Header(), rec.Body)
		}
		hidden = append(hidden, rec)
		if strings.Contains(path, "/status/") {
			// The page is anonymous: what an anonymous visitor may not see is no redirect either (ADR-0017).
			if rec := getAs(path, acceptMastodon); rec.Code != http.StatusNotFound || rec.Header().Get("Location") != "" {
				t.Errorf("GET %s, ActivityPub = %d %q, want 404", path, rec.Code, rec.Header().Get("Location"))
			}
		}
	}
	for i, rec := range hidden[1:] {
		if a, b := rec.Header(), hidden[0].Header(); len(a) != len(b) || a.Get("Cache-Control") != b.Get("Cache-Control") ||
			a.Get("Content-Length") != b.Get("Content-Length") || a.Get("ETag") != b.Get("ETag") {
			t.Errorf("hidden page %d headers %v differ from %v", i+1, a, b)
		}
	}
}
