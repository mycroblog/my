// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

// TestInstanceDocuments publishes the terms of service written in Markdown on the real database:
// GET /instance/documents/terms renders and sanitizes them, the registration page asks to agree
// to them, and GET /instance links to the web client's page of them; the privacy policy, which
// has no text, is a 404 and unlinked.
func TestInstanceDocuments(t *testing.T) {
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		_, err := a.Instance.SetText(t.Context(), appinstance.SetTextInput{
			Key: instance.TextTerms, Body: "# 第1条\n\n**親切に**。<script>alert(1)</script>\n\n[ヘルプ](javascript:alert(1))",
		})
		if err != nil {
			t.Fatalf("SetText: %v", err)
		}
	})
	c := &authClient{t: t, s: s, cookies: map[string]string{}}
	ja := map[string]string{"Accept-Language": "ja"}

	resp, body := c.do(http.MethodGet, "/api/v1/instance/documents/terms", nil, ja)
	var doc struct {
		Kind      string `json:"kind"`
		HTML      string `json:"html"`
		RevisedAt string `json:"revised_at"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET terms = %d %v\n%s", resp.StatusCode, err, body)
	}
	if doc.Kind != "terms" || doc.RevisedAt == "" || resp.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Errorf("terms = %+v, Cache-Control %q", doc, resp.Header.Get("Cache-Control"))
	}
	for _, want := range []string{`<h2>第1条</h2>`, `<strong>親切に</strong>`} {
		if !strings.Contains(doc.HTML, want) {
			t.Errorf("terms HTML lacks %s:\n%s", want, doc.HTML)
		}
	}
	for _, unwanted := range []string{"<script", "javascript:"} {
		if strings.Contains(doc.HTML, unwanted) {
			t.Errorf("terms HTML has %s:\n%s", unwanted, doc.HTML)
		}
	}
	if resp, _ := c.do(http.MethodGet, "/api/v1/instance/documents/privacy_policy", nil, ja); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET privacy_policy without a policy = %d, want 404", resp.StatusCode)
	}

	// The old server-rendered pages are gone: the paths belong to the SPA and the shell, or are
	// reserved by the authentication pages.
	if resp, _ := c.do(http.MethodGet, "/auth/terms", nil, ja); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /auth/terms = %d, want 404", resp.StatusCode)
	}

	if resp, _ := c.do(http.MethodGet, "/about/terms", nil, ja); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /about/terms = %d, want the SPA shell", resp.StatusCode)
	}

	if _, body := c.do(http.MethodGet, authpages.PathSignUp, nil, ja); authText(authData(t, body), "documents.0.path") != "/about/terms" ||
		authAt(authData(t, body), "documents.1") != nil {
		t.Errorf("sign-up page lacks the agreement to the terms:\n%s", body)
	}

	resp, body = c.do(http.MethodGet, "/api/v1/instance", nil, ja)
	var got struct {
		TermsURL   string `json:"terms_url"`
		PrivacyURL string `json:"privacy_policy_url"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /instance = %d %v\n%s", resp.StatusCode, err, body)
	}
	if got.TermsURL != cfg.ServerOrigin()+"/about/terms" || got.PrivacyURL != "" {
		t.Errorf("instance links = %+v, want the terms only", got)
	}
}
