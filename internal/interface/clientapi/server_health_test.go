// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeHealthCheck answers every check with its fields.
type fakeHealthCheck struct {
	healthy bool
	at      time.Time
}

func (f fakeHealthCheck) Check(context.Context) (bool, time.Time) { return f.healthy, f.at }

func TestGetServerHealth(t *testing.T) {
	at := time.Date(2026, 10, 25, 12, 0, 0, 123_000_000, time.UTC)
	for _, tt := range []struct {
		name       string
		health     clientapi.HealthCheck
		wantStatus int
		wantBody   string
	}{
		{"healthy", fakeHealthCheck{true, at}, http.StatusOK, `{"checked_at":"2026-10-25T12:00:00Z","status":"ok"}`},
		{"unhealthy", fakeHealthCheck{false, at}, http.StatusServiceUnavailable, `{"checked_at":"2026-10-25T12:00:00Z","status":"unavailable"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) { o.ServerHealth = tt.health })
			// Neither a token, even an invalid one, nor the rate limits are looked at.
			for _, header := range []map[string]string{nil, {"Authorization": "Bearer nope", "Accept-Language": "ja"}} {
				res := e.get(t, "/instance/health", header)
				body, _ := io.ReadAll(res.Body)
				if res.StatusCode != tt.wantStatus || string(body) != tt.wantBody+"\n" {
					t.Errorf("GET /instance/health %v = %d %s, want %d %s", header, res.StatusCode, body, tt.wantStatus, tt.wantBody)
				}
				if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
					t.Errorf("Cache-Control = %q, want no-store", cc)
				}
				if rl := res.Header.Get("RateLimit"); rl != "" {
					t.Errorf("RateLimit = %q, want none", rl)
				}
			}
		})
	}
}

func TestGetServerHealthWithoutCheck(t *testing.T) {
	e := newAuthEnv(t, nil)
	if res := e.get(t, "/instance/health", nil); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("GET /instance/health without a HealthCheck = %d, want 503", res.StatusCode)
	}
}
