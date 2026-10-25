// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// countingCheck is a readiness check that counts its runs and fails while err is set.
type countingCheck struct {
	runs atomic.Int64
	mu   sync.Mutex
	err  error
	// gate, when set, holds every run until it is closed.
	gate chan struct{}
}

func (c *countingCheck) check(context.Context) error {
	c.runs.Add(1)
	c.mu.Lock()
	gate, err := c.gate, c.err
	c.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return err
}

func (c *countingCheck) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

// waitRuns waits until c ran n times.
func waitRuns(t *testing.T, c *countingCheck, n int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for c.runs.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("the check ran %d times, want %d", c.runs.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHealthCacheChecksOncePerTTL(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	clock := testkit.NewClock(start)
	c := &countingCheck{}
	check := platform.NewHealthCache(c.check, clock)

	for range 100 {
		if healthy, at := check(t.Context()); !healthy || !at.Equal(start) {
			t.Fatalf("Check = %v %v, want healthy at %v", healthy, at, start)
		}
	}
	clock.Advance(platform.HealthCacheTTL - time.Second)
	c.fail(errors.New("database down"))
	if healthy, at := check(t.Context()); !healthy || !at.Equal(start) {
		t.Errorf("Check within the TTL = %v %v, want the cached result", healthy, at)
	}
	if n := c.runs.Load(); n != 1 {
		t.Fatalf("the check ran %d times within the TTL, want 1", n)
	}

	// Past the TTL, the next call checks again, and a failure is cached as long.
	at := clock.Advance(time.Second)
	if healthy, got := check(t.Context()); healthy || !got.Equal(at) {
		t.Errorf("Check past the TTL = %v %v, want unhealthy at %v", healthy, got, at)
	}
	c.fail(nil)
	clock.Advance(platform.HealthCacheTTL - time.Millisecond)
	if healthy, _ := check(t.Context()); healthy {
		t.Error("Check within the TTL of a failure = healthy, want the cached failure")
	}
	if n := c.runs.Load(); n != 2 {
		t.Errorf("the check ran %d times, want 2", n)
	}
}

func TestHealthCacheRunsOneCheckForConcurrentCalls(t *testing.T) {
	t.Parallel()
	clock := testkit.NewClock(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	c := &countingCheck{gate: make(chan struct{})}
	check := platform.NewHealthCache(c.check, clock)

	const calls = 20
	var wg sync.WaitGroup
	results := make([]bool, calls)
	for i := range calls {
		wg.Go(func() { results[i], _ = check(t.Context()) })
	}
	waitRuns(t, c, 1)
	// Let the other calls join the running check. A call that comes after it ends finds the
	// fresh result, so it never runs a second check either.
	time.Sleep(50 * time.Millisecond)
	close(c.gate)
	wg.Wait()
	if n := c.runs.Load(); n != 1 {
		t.Errorf("the check ran %d times for %d concurrent calls, want 1", n, calls)
	}
	if slices.Contains(results, false) {
		t.Errorf("results = %v, want every call healthy", results)
	}
}

func TestInstanceHealth(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		applied    bool
		wantStatus int
		want       string
	}{
		{"ready", true, http.StatusOK, "ok"},
		// Without the schema the database is not ready, and the body still holds the fixed words.
		{"without the schema", false, http.StatusServiceUnavailable, "unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, tt.applied)})
			s := start(t, cfg, platform.ServeOptions{}, nil)

			before := time.Now().Add(-time.Second)
			resp, body := s.do(t, http.MethodGet, platform.PathInstanceHealth, map[string]string{"Accept-Language": "ja"})
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("GET %s = %d %q, want %d", platform.PathInstanceHealth, resp.StatusCode, body, tt.wantStatus)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", cc)
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("body %q: %v", body, err)
			}
			if len(got) != 2 || got["status"] != tt.want {
				t.Errorf("body = %q, want only status %q and checked_at", body, tt.want)
			}
			at, err := time.Parse(time.RFC3339, got["checked_at"])
			if err != nil || at.Before(before.Truncate(time.Second)) || at.After(time.Now()) {
				t.Errorf("checked_at = %q (%v), want the RFC 3339 time of the check", got["checked_at"], err)
			}
			if !tt.applied && !strings.Contains(s.logs.String(), "schema not applied") {
				t.Errorf("the failure is not logged:\n%s", s.logs.String())
			}
			// Uptime monitors call it every minute: the access log writes it at debug level.
			logged := false
			for line := range strings.Lines(s.logs.String()) {
				if strings.Contains(line, `"url.path":"`+platform.PathInstanceHealth+`"`) {
					logged = true
					if !strings.Contains(line, `"level":"DEBUG"`) {
						t.Errorf("access log line %q, want debug level", line)
					}
				}
			}
			if !logged {
				t.Errorf("the request is not in the access log:\n%s", s.logs.String())
			}
		})
	}
}

func TestInstanceHealthChecksTheDatabaseOncePerTTL(t *testing.T) {
	t.Parallel()
	c := &countingCheck{}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) { platform.SetReadyCheck(a, c.check) })

	for range 5 {
		if resp, body := s.do(t, http.MethodGet, platform.PathInstanceHealth, nil); resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d %q, want 200", platform.PathInstanceHealth, resp.StatusCode, body)
		}
	}
	// The cached answer outlives the database: it is the result of the last check.
	c.fail(errors.New("database down"))
	if resp, _ := s.do(t, http.MethodGet, platform.PathInstanceHealth, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s within the TTL = %d, want the cached 200", platform.PathInstanceHealth, resp.StatusCode)
	}
	if n := c.runs.Load(); n != 1 {
		t.Errorf("the database was checked %d times, want 1", n)
	}
	// Neither a token nor the rate limits are looked at, both of which may read the database.
	if resp, _ := s.do(t, http.MethodGet, platform.PathInstanceHealth, map[string]string{"Authorization": "Bearer nope"}); resp.StatusCode != http.StatusOK || resp.Header.Get("RateLimit") != "" {
		t.Errorf("GET %s with a token = %d RateLimit %q, want 200 without a rate limit", platform.PathInstanceHealth, resp.StatusCode, resp.Header.Get("RateLimit"))
	}
}

func TestReadinessRunsOneCheckForConcurrentRequests(t *testing.T) {
	t.Parallel()
	c := &countingCheck{gate: make(chan struct{})}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) { platform.SetReadyCheck(a, c.check) })

	const requests = 10
	var wg sync.WaitGroup
	statuses := make([]int, requests)
	bodies := make([]string, requests)
	for i := range requests {
		wg.Go(func() {
			resp, body := s.do(t, http.MethodGet, platform.PathReadiness, nil)
			statuses[i], bodies[i] = resp.StatusCode, body
		})
	}
	waitRuns(t, c, 1)
	time.Sleep(200 * time.Millisecond) // let the other requests join the running check
	close(c.gate)
	wg.Wait()
	if n := c.runs.Load(); n != 1 {
		t.Errorf("the check ran %d times for %d concurrent requests, want 1", n, requests)
	}
	for i := range requests {
		if statuses[i] != http.StatusOK || bodies[i] != `{"status":"ok"}`+"\n" {
			t.Errorf("readiness = %d %q, want 200 ok", statuses[i], bodies[i])
		}
	}

	// Each request after the check ends checks again: readiness keeps no result.
	if resp, _ := s.do(t, http.MethodGet, platform.PathReadiness, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("readiness = %d, want 200", resp.StatusCode)
	}
	if n := c.runs.Load(); n != 2 {
		t.Errorf("the check ran %d times, want 2", n)
	}
}
