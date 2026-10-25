// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
)

// TestInstanceNameAndDomain checks that the instance's display name names it wherever people
// read its name (the shell, the manifest, NodeInfo, the sign-in pages, GET /instance, and the
// emails), the same in every locale and without a restart, while handles and URLs keep the
// domain; without a name, the domain names it.
func TestInstanceNameAndDomain(t *testing.T) {
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "inline", "MY_MAIL": "none"})
	build := fstest.MapFS{
		".vite/manifest.json":  {Data: []byte(`{"index.html": {"file": "assets/index-AAAA.js", "isEntry": true}}`)},
		"assets/index-AAAA.js": {Data: []byte("export {}")},
	}
	var logs bytes.Buffer
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.NewJSONHandler(&logs, nil)), WebBuild: build})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	h := app.Handler()
	get := func(path, lang string) string {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.Host+path, nil)
		req.Header.Set("Accept", acceptBrowser)
		if lang != "" {
			req.Header.Set("Accept-Language", lang)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	check := func(en, ja string) {
		t.Helper()
		if body := get("/", ""); !strings.Contains(body, "<title>"+en+"</title>") || !strings.Contains(body, `"instanceName":"`+en+`"`) ||
			!strings.Contains(body, `"accountDomain":"`+cfg.AccountDomain+`"`) {
			t.Errorf("shell, want the name %q and the account domain:\n%s", en, body)
		}
		var manifest struct{ Name string }
		if err := json.Unmarshal([]byte(get("/manifest.webmanifest", "")), &manifest); err != nil || manifest.Name != en {
			t.Errorf("manifest name = %q (%v), want %q", manifest.Name, err, en)
		}
		var nodeInfo struct {
			Metadata struct{ NodeName string }
		}
		if err := json.Unmarshal([]byte(get("/nodeinfo/2.1", "")), &nodeInfo); err != nil || nodeInfo.Metadata.NodeName != en {
			t.Errorf("NodeInfo nodeName = %q (%v), want %q", nodeInfo.Metadata.NodeName, err, en)
		}
		if body := get("/auth/sign_in", "ja"); !strings.Contains(body, `<p class="site-name" dir="auto">`+ja+`</p>`) || !strings.Contains(body, "– "+ja+"</title>") {
			t.Errorf("sign-in page in ja, want the name %q:\n%s", ja, body)
		}
		var info struct{ Domain, Name string }
		if err := json.Unmarshal([]byte(get("/api/v1/instance", "ja")), &info); err != nil || info.Name != ja || info.Domain != cfg.Host {
			t.Errorf("GET /instance in ja = %+v (%v), want the name %q and the domain %q", info, err, ja, cfg.Host)
		}
	}
	check(cfg.Host, cfg.Host)

	admin, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "admin", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Instance.SetText(ctx, appinstance.SetTextInput{Key: instance.TextName, Body: "例のソーシャル", RevisedByUserID: admin.User.ID}); err != nil {
		t.Fatal(err)
	}
	check("例のソーシャル", "例のソーシャル")

	// The verification email of a sign-up names the instance as its sender and in its signature,
	// the domain where it says where the user signed up, and the contact address as its Reply-To
	// and in its signature (ADR-0086 §2, §3).
	for key, value := range map[instance.SettingKey]string{
		instance.SettingRegistrationMode: "open", instance.SettingContactEmail: "admin@example.com",
	} {
		if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.Identity.SignUp(ctx, identity.SignUpInput{
		Username: "bob", Email: "bob@example.com", Password: "correct horse battery staple", Locale: "ja",
	}); err != nil {
		t.Fatal(err)
	}
	var mail struct {
		FromName string `json:"from_name"`
		ReplyTo  string `json:"reply_to"`
		Subject  string
		Text     string
	}
	for line := range strings.Lines(logs.String()) {
		if strings.Contains(line, "mail not sent") {
			if err := json.Unmarshal([]byte(line), &mail); err != nil {
				t.Fatal(err)
			}
		}
	}
	if mail.Subject != "例のソーシャル のメールアドレスの確認" || !strings.HasPrefix(mail.Text, "例のソーシャル へようこそ。") ||
		!strings.Contains(mail.Text, cfg.Host+" の登録にこのメールアドレスが使われた") ||
		!strings.Contains(mail.Text, "\n-- \n例のソーシャル\nhttps://"+cfg.Host+"\n連絡先: admin@example.com\n") ||
		mail.FromName != "例のソーシャル" || mail.ReplyTo != "admin@example.com" {
		t.Errorf("verification email = %+v, want the name and the domain", mail)
	}
}
