// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/content"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// remoteServer serves a PNG at /a.png and an SVG declared as a PNG at /b.png, counting the
// requests; /error.png fails with 503.
func remoteServer(t *testing.T) (*httptest.Server, *atomic.Int32, []byte) {
	t.Helper()
	png := testkit.FixturePNG(4, 3, false)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/a.png":
			_, _ = w.Write(png)
		case "/b.png":
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
		case "/error.png":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, png
}

// recordRemote records a remote media file of the instance actor at remoteURL and returns the
// path and query of its proxy URL.
func recordRemote(t *testing.T, app *platform.App, remoteURL string) string {
	t.Helper()
	owner, err := app.Accounts.EnsureInstanceActor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var m domain.RemoteMedia
	err = app.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		m, err = app.Media.RecordRemoteMedia(t.Context(), tx, content.RemoteMediaInput{
			OwnerAccountID: owner.ID, RemoteURL: remoteURL, DeclaredMediaType: domain.MediaTypeImage,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(app.MediaProxy.URL(m.MediaID, content.ProxyFull))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "example.test" {
		t.Errorf("proxy URL = %s, want one on the instance's origin", u)
	}
	return u.RequestURI()
}

// The media proxy relays a recorded remote file with the detected type and the cache TTL of
// the instance settings, and refuses unsigned or tampered URLs and SVG content.
func TestMediaProxy(t *testing.T) {
	t.Parallel()
	remote, _, png := remoteServer(t)
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_PROFILE": "development", "MY_HTTP_ALLOW_PRIVATE_NETWORKS": "true",
	})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	path := recordRemote(t, s.app, remote.URL+"/a.png")

	resp, body := s.do(t, http.MethodGet, path, nil)
	if resp.StatusCode != http.StatusOK || body != string(png) {
		t.Fatalf("GET %s = %d, %d bytes", path, resp.StatusCode, len(body))
	}
	// The small variant is a still WebP made from the file (ADR-0005 §10).
	small := strings.Replace(path, "/full?", "/small?", 1)
	id := strings.TrimPrefix(strings.Split(path, "/full?")[0], content.ProxyPathPrefix)
	if u, err := url.Parse(s.app.MediaProxy.URL(id, content.ProxySmall)); err == nil {
		small = u.RequestURI()
	}
	if resp, body := s.do(t, http.MethodGet, small, nil); resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/webp" ||
		!strings.HasPrefix(body, "RIFF") || resp.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("GET %s = %d %s %q", small, resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"))
	}
	for k, want := range map[string]string{
		"Content-Type":                 "image/png",
		"X-Content-Type-Options":       "nosniff",
		"Content-Security-Policy":      "default-src 'none'; sandbox",
		"Cross-Origin-Resource-Policy": "cross-origin",
		"Cache-Control":                "public, max-age=86400",
	} {
		if got := resp.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	if _, err := s.app.Instance.SetSetting(t.Context(), instance.SetSettingInput{Key: domaininstance.SettingRemoteMediaCacheTTLSeconds, Value: "3600"}); err != nil {
		t.Fatal(err)
	}
	if resp, _ := s.do(t, http.MethodGet, path, nil); resp.Header.Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q after setting remote_media_cache_ttl_seconds", resp.Header.Get("Cache-Control"))
	}

	unsigned, _, _ := strings.Cut(path, "?")
	last := "A"
	if strings.HasSuffix(path, "A") {
		last = "B"
	}
	tampered := path[:len(path)-1] + last
	for _, p := range []string{unsigned, tampered, recordRemote(t, s.app, remote.URL+"/b.png"), recordRemote(t, s.app, remote.URL+"/missing.png")} {
		if resp, _ := s.do(t, http.MethodGet, p, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, resp.StatusCode)
		}
	}

	// A remote 5xx is a 502 problem.
	resp, body = s.do(t, http.MethodGet, recordRemote(t, s.app, remote.URL+"/error.png"), nil)
	if resp.StatusCode != http.StatusBadGateway || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("GET of a failing remote file = %d Cache-Control %q, want 502 no-store", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	if p := decodeProblem(t, resp, body); p.Status != http.StatusBadGateway {
		t.Errorf("problem = %+v, want status 502", p)
	}
}

// Without http_allow_private_networks, the proxy's fetch of an internal address is refused by
// the SSRF guard: the client gets 404 and the internal server sees no request.
func TestMediaProxySSRFGuard(t *testing.T) {
	t.Parallel()
	remote, hits, _ := remoteServer(t)
	s := start(t, loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)}), platform.ServeOptions{}, nil)
	internal := strings.Replace(remote.URL, "http://", "https://", 1) + "/a.png"
	for _, target := range []string{internal, remote.URL + "/a.png", "https://localhost/a.png", "https://169.254.169.254/latest/meta-data"} {
		path := recordRemote(t, s.app, target)
		if resp, _ := s.do(t, http.MethodGet, path, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET proxy of %s = %d, want 404", target, resp.StatusCode)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the internal server got %d requests", n)
	}
}

// Under remote_media_policy all, the proxy keeps a copy in blob storage and serves the second
// request from it; under none it relays every request.
func TestMediaProxyCache(t *testing.T) {
	t.Parallel()
	remote, hits, png := remoteServer(t)
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_PROFILE": "development", "MY_HTTP_ALLOW_PRIVATE_NETWORKS": "true",
	})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	path := recordRemote(t, s.app, remote.URL+"/a.png")
	get := func() {
		t.Helper()
		resp, body := s.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusOK || body != string(png) || resp.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("GET %s = %d %q, %d bytes", path, resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
		}
	}

	get()
	get()
	if n := hits.Load(); n != 2 {
		t.Errorf("none: the remote server got %d requests for 2, want 2", n)
	}
	if _, err := s.app.Instance.SetSetting(t.Context(), instance.SetSettingInput{Key: domaininstance.SettingRemoteMediaPolicy, Value: "all"}); err != nil {
		t.Fatal(err)
	}
	get()
	get()
	if n := hits.Load(); n != 3 {
		t.Errorf("all: the remote server got %d requests in total, want 3", n)
	}
}

// The media proxy fetches nothing from a suspended domain, nor, under federation_scope allowlist,
// from a domain without an allow, and still fetches from an allowed one (ADR-0159 §2).
func TestMediaProxyRefusesSuspendedAndUnallowedDomains(t *testing.T) {
	t.Parallel()
	png := testkit.FixturePNG(4, 3, false)
	web := &pageWeb{hits: map[string]int{}, pages: map[string]webPage{}}
	for _, host := range []string{"friend.example", "stranger.example", "spam.example"} {
		web.pages["https://"+host+"/a.png"] = webPage{http.StatusOK, "image/png", png}
	}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := startApp(t, cfg, platform.AppOptions{HTTPClient: web}, platform.ServeOptions{}, nil)
	ctx := t.Context()
	get := func(t *testing.T, host string) int {
		t.Helper()
		resp, _ := s.do(t, http.MethodGet, recordRemote(t, s.app, "https://"+host+"/a.png"), nil)
		return resp.StatusCode
	}

	if _, err := s.app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "spam.example", Kind: moderation.DomainSuspend}); err != nil {
		t.Fatal(err)
	}
	if got := get(t, "spam.example"); got != http.StatusNotFound || web.count("https://spam.example/a.png") != 0 {
		t.Errorf("GET proxy of a suspended domain = %d after %d fetches, want 404 and none", got, web.count("https://spam.example/a.png"))
	}
	if got := get(t, "stranger.example"); got != http.StatusOK {
		t.Errorf("GET proxy under open = %d, want 200", got)
	}

	if _, _, err := s.app.Moderation.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "friend.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.app.Instance.SetSetting(ctx, instance.SetSettingInput{Key: domaininstance.SettingFederationScope, Value: string(domaininstance.FederationScopeAllowlist)}); err != nil {
		t.Fatal(err)
	}
	before := web.count("https://stranger.example/a.png")
	if got := get(t, "stranger.example"); got != http.StatusNotFound || web.count("https://stranger.example/a.png") != before {
		t.Errorf("GET proxy of a domain without an allow = %d, fetched %d times, want 404 and no fetch", got, web.count("https://stranger.example/a.png")-before)
	}
	if got := get(t, "friend.example"); got != http.StatusOK {
		t.Errorf("GET proxy of an allowed domain = %d, want 200", got)
	}
}
