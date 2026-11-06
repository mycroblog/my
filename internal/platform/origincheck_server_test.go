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

const (
	originSecret = "0123456789abcdef0123456789abcdef"
	originHeader = "X-Via-My-Cdn"
)

// The whole server behind the check: everything but the exempt requests is refused without the
// secret, the web client and the media too, and passes as usual with it.
func TestServeRefusesWhatDidNotComeThroughTheCDN(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_ORIGIN_SECRET": originSecret, "MY_ORIGIN_SECRET_HEADER": originHeader,
	})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	waitServing(t, s)

	for _, r := range []struct{ method, target string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/about"},
		{http.MethodGet, "/@alice"},
		{http.MethodGet, "/assets/app.js"},
		{http.MethodGet, "/manifest.webmanifest"},
		{http.MethodGet, "/media/a/b.png"},
		{http.MethodGet, "/media/proxy/x/original?sig=y"},
		{http.MethodGet, "/api/v1/instance"},
		{http.MethodGet, "/.well-known/webfinger?resource=acct:alice@example.test"},
		{http.MethodGet, "/_internal/jobs"},
		{http.MethodPost, "/inbox"},
		{http.MethodPost, "/oauth/token"},
		{http.MethodOptions, "/api/v1/instance"},
	} {
		for name, header := range map[string]map[string]string{"missing": nil, "wrong": {originHeader: "wrong"}} {
			resp, body := s.do(t, r.method, r.target, header)
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s with %s value = %d, want 403", r.method, r.target, name, resp.StatusCode)
				continue
			}
			decodeProblem(t, resp, body)
			if strings.Contains(strings.ToLower(body), strings.ToLower(originHeader)) {
				t.Errorf("%s %s: the problem names the header: %s", r.method, r.target, body)
			}
		}
		resp, _ := s.do(t, r.method, r.target, map[string]string{originHeader: originSecret})
		if resp.StatusCode == http.StatusForbidden && r.target != "/_internal/jobs" {
			t.Errorf("%s %s with the secret = 403", r.method, r.target)
		}
	}
	for _, target := range []string{platform.PathLiveness, platform.PathReadiness, platform.PathInstanceHealth} {
		if resp, body := s.do(t, http.MethodGet, target, nil); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s without the secret = %d %s, want 200", target, resp.StatusCode, body)
		}
	}
	if resp, _ := s.do(t, http.MethodPost, "/_internal/jobs", nil); resp.StatusCode == http.StatusForbidden {
		t.Error("POST /_internal/jobs without the secret = 403")
	}
	if logs := s.logs.String(); strings.Contains(logs, originSecret) || strings.Contains(logs, originHeader) {
		t.Errorf("the log holds the header:\n%s", logs)
	}
	if !strings.Contains(s.logs.String(), "the origin check is on") {
		t.Errorf("the start does not say that the check is on:\n%s", s.logs.String())
	}
}

// The check answers while the domains are not verified either: a probe does not learn that state.
func TestOriginCheckComesBeforeTheDomainsGate(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, false), "MY_ORIGIN_SECRET": originSecret, "MY_ORIGIN_SECRET_HEADER": originHeader,
	})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	waitServing(t, s)
	if resp, _ := s.do(t, http.MethodGet, "/", nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("GET / without the secret = %d, want 403", resp.StatusCode)
	}
	if resp, _ := s.do(t, http.MethodGet, "/", map[string]string{originHeader: originSecret}); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("GET / with the secret = %d, want the 503 of the gate", resp.StatusCode)
	}
}

func TestMaintenanceModeChecksTheOrigin(t *testing.T) {
	env := map[string]string{"MY_HOST": "example.test", "MY_ORIGIN_SECRET": originSecret, "MY_ORIGIN_SECRET_HEADER": originHeader}
	cfg, _, err := config.LoadMaintenance(config.Source{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }})
	if err != nil {
		t.Fatal(err)
	}
	app, err := platform.NewMaintenanceApp(cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()
	serve := func(method, target string, header map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(""))
		for k, v := range header {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	for _, target := range []string{"/", "/api/v1/instance", "/media/a/b.png", "/_internal/jobs"} {
		rec := serve(http.MethodGet, target, map[string]string{originHeader: "wrong"})
		if rec.Code != http.StatusForbidden || rec.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("GET %s with a wrong value = %d %v", target, rec.Code, rec.Header())
		}
		var p struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.Status != http.StatusForbidden {
			t.Errorf("GET %s: problem %s: %v", target, rec.Body, err)
		}
		if rec := serve(http.MethodGet, target, nil); rec.Code != http.StatusForbidden {
			t.Errorf("GET %s without a value = %d", target, rec.Code)
		}
		if rec := serve(http.MethodGet, target, map[string]string{originHeader: originSecret}); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s with the secret = %d, want the maintenance's 503", target, rec.Code)
		}
	}
	for _, target := range []string{platform.PathLiveness, platform.PathReadiness} {
		if rec := serve(http.MethodGet, target, nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", target, rec.Code)
		}
	}
	if rec := serve(http.MethodGet, platform.PathInstanceHealth, nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET %s = %d, want the maintenance's 503", platform.PathInstanceHealth, rec.Code)
	}
	// The queue's calls keep getting the 503 that makes them retry.
	if rec := serve(http.MethodPost, "/_internal/jobs", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("POST /_internal/jobs = %d, want the maintenance's 503", rec.Code)
	}
	if rec := serve(http.MethodPost, "/events", nil); rec.Code != http.StatusForbidden {
		t.Errorf("POST /events = %d, want 403", rec.Code)
	}
}
