// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// TestGCPLogsLinkTheTrace checks that with logs_exporter = gcp the lines of a request from a
// trusted proxy carry the trace fields of Cloud Logging, with the bare trace ID, and its access
// line the request of Cloud Logging, without the query, next to the fields of stderr, which stay
// (ADR-0055 §18, §20); and that stderr writes neither.
func TestGCPLogsLinkTheTrace(t *testing.T) {
	for _, exporter := range []string{config.LogsGCP, config.LogsStderr} {
		t.Run(exporter, func(t *testing.T) {
			a, logs := newTracedApp(t, map[string]string{"MY_LOGS_EXPORTER": exporter}, func(context.Context, queue.Job) queue.Result { return queue.Done() })
			post(t, a.Handler(), "192.0.2.10:4000")
			var access map[string]any
			for line := range strings.Lines(logs.String()) {
				var m map[string]any
				if json.Unmarshal([]byte(line), &m) == nil && m["msg"] == "http request" {
					access = m
				}
			}
			if access == nil {
				t.Fatalf("no access line:\n%s", logs)
			}
			if exporter == config.LogsStderr {
				for _, k := range []string{gcpTraceKey, gcpSpanIDKey, gcpTraceSampledKey, gcpHTTPRequestKey} {
					if _, ok := access[k]; ok {
						t.Errorf("stderr line has %s: %v", k, access)
					}
				}
				return
			}
			if access[gcpTraceKey] != incomingTraceID || access[gcpSpanIDKey] != incomingSpanID || access[gcpTraceSampledKey] != true {
				t.Errorf("trace fields = %v %v %v, want %s %s true", access[gcpTraceKey], access[gcpSpanIDKey], access[gcpTraceSampledKey], incomingTraceID, incomingSpanID)
			}
			if access["trace_id"] != incomingTraceID || access["msg"] != "http request" || access["url.path"] != "/test/traced" {
				t.Errorf("line = %v, want the fields of stderr kept", access)
			}
			req, _ := access[gcpHTTPRequestKey].(map[string]any)
			if req["requestMethod"] != "POST" || req["requestUrl"] != "/test/traced" || req["status"] != float64(202) || req["responseSize"] != "0" || req["protocol"] != "HTTP/1.1" {
				t.Errorf("httpRequest = %v", req)
			}
			if l, _ := req["latency"].(string); !regexp.MustCompile(`^\d+\.\d{9}s$`).MatchString(l) {
				t.Errorf("latency = %q, want seconds with nine fractional digits", l)
			}
			for _, k := range []string{"remoteIp", "userAgent", "referer"} {
				if _, ok := req[k]; ok {
					t.Errorf("httpRequest has %s: %v", k, req)
				}
			}
		})
	}
}
