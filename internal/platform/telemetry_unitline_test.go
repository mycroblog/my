// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestUnitLineWithoutExporter checks that, with metrics_exporter = none, the access log line of
// a request that uses the key-value store and the database carries the database pool's usage and
// the adapter operations of the request, which the alerts of monitoring-gcp read from the log.
func TestUnitLineWithoutExporter(t *testing.T) {
	t.Parallel()
	const path = "/test/unit-line"
	s := start(t, loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)}), platform.ServeOptions{}, func(a *platform.App) {
		if !kernel.IsNop(a.Meter) {
			t.Errorf("the Meter of metrics_exporter = none is %T, which records", a.Meter)
		}
		a.Handle("GET "+path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if err := a.KV.Set(ctx, "test", "k", []byte("v"), time.Minute); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if _, err := a.KV.Get(ctx, "test", "k"); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := a.Ready(ctx); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	})
	if resp, body := s.do(t, http.MethodGet, path, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("GET %s = %d %s", path, resp.StatusCode, body)
	}

	var line map[string]any
	testkit.Eventually(t, 10*time.Second, func() bool {
		for raw := range strings.Lines(s.logs.String()) {
			var m map[string]any
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				t.Fatalf("line %q: %v", raw, err)
			}
			if m["msg"] == "http request" && m["url.path"] == path {
				line = m
				return true
			}
		}
		return false
	})
	usage, ok := line[sysmetrics.LinePoolUsage].(float64)
	if !ok || usage < 0 || usage > 1 {
		t.Errorf("%s = %#v, want a number from 0 to 1, on the line %v", sysmetrics.LinePoolUsage, line[sysmetrics.LinePoolUsage], line)
	}
	if ops, ok := line[sysmetrics.LineAdapterOperations].(float64); !ok || ops < 2 {
		t.Errorf("%s = %#v, want at least the 2 operations on the key-value store, on the line %v", sysmetrics.LineAdapterOperations, line[sysmetrics.LineAdapterOperations], line)
	}
	if errs, ok := line[sysmetrics.LineAdapterErrors].(float64); !ok || errs != 0 {
		t.Errorf("%s = %#v, want 0, on the line %v", sysmetrics.LineAdapterErrors, line[sysmetrics.LineAdapterErrors], line)
	}
}
