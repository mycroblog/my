// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/metrics"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestEMFMetrics checks that, with metrics_exporter = emf, the access log line of a request and
// the log line of a job carry their metrics as EMF (ADR-0055 §5): one line per request and per
// job, and no metric line of their own.
func TestEMFMetrics(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_METRICS_EXPORTER": "emf",
		"MY_METRICS_EMF_DIMENSIONS": "my.surface, http.response.status_code, my.job.type",
	})
	var metricLines syncBuffer
	s := startApp(t, cfg, platform.AppOptions{MetricsWriter: &metricLines}, platform.ServeOptions{}, func(a *platform.App) {
		a.Handlers.Register("test.metrics", func(context.Context, queue.Job) queue.Result { return queue.Done() })
	})
	if resp, _ := s.do(t, http.MethodGet, "/api/v1/instance", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/instance = %d", resp.StatusCode)
	}
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.metrics"}); err != nil {
		t.Fatal(err)
	}

	parse := func(b *syncBuffer) []map[string]any {
		var out []map[string]any
		for line := range strings.Lines(b.String()) {
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("line %q: %v", line, err)
			}
			out = append(out, m)
		}
		return out
	}
	var requests, jobs []map[string]any
	deadline := time.Now().Add(10 * time.Second)
	for {
		requests, jobs = nil, nil
		for _, m := range parse(s.logs) {
			switch {
			case m["msg"] == "http request" && m["url.path"] == "/api/v1/instance":
				requests = append(requests, m)
			case m["msg"] == "job" && m["my.job.type"] == "test.metrics":
				jobs = append(jobs, m)
			}
		}
		if len(requests) > 0 && len(jobs) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(requests) != 1 || len(jobs) != 1 {
		t.Fatalf("%d access log lines and %d job lines, want 1 each:\n%s", len(requests), len(jobs), s.logs.String())
	}

	directives := func(m map[string]any) map[string]string {
		aws, _ := m["_aws"].(map[string]any)
		cw, _ := aws["CloudWatchMetrics"].([]any)
		out := map[string]string{}
		for _, d := range cw {
			d := d.(map[string]any)
			dims, _ := json.Marshal(d["Dimensions"])
			for _, metric := range d["Metrics"].([]any) {
				out[metric.(map[string]any)["Name"].(string)] = string(dims)
			}
		}
		return out
	}
	req := requests[0]
	if _, ok := req["http.server.request.duration"].(float64); !ok || req["http.route"] != "/api/v1/instance" || req["my.role"] != "all" {
		t.Errorf("access log line = %v", req)
	}
	// EMF dimensions are strings.
	if req["http.response.status_code"] != "200" {
		t.Errorf("http.response.status_code = %#v, want \"200\"", req["http.response.status_code"])
	}
	if got := directives(req); got["http.server.request.duration"] != `[["my.surface","http.response.status_code"]]` {
		t.Errorf("directives of the access log line = %v", got)
	}
	job := jobs[0]
	_, hasDuration := job["my.job.duration"].(float64)
	_, hasWait := job["my.job.wait"].(float64)
	if !hasDuration || !hasWait {
		t.Errorf("job line = %v", job)
	}
	if got := directives(job); got["my.job.duration"] != `[["my.job.type"]]` || got["my.job.wait"] != `[["my.job.type"]]` {
		t.Errorf("directives of the job line = %v", got)
	}

	// The metrics of the log lines are not written again on lines of their own.
	for _, m := range parse(&metricLines) {
		for _, name := range []string{"http.server.request.duration", "my.job.duration", "my.job.wait"} {
			if _, ok := m[name]; ok {
				t.Errorf("%s is also written on a line of its own: %v", name, m)
			}
		}
	}
}

// TestServiceMetricsLabels runs a post, its jobs, and a stream request through an App with
// testkit.Meter, which reports every label that an instrument did not declare.
func TestServiceMetricsLabels(t *testing.T) {
	t.Parallel()
	m := testkit.NewMeter()
	c := newStatusesClientWith(t, nil, platform.AppOptions{Meter: m}, "alice")
	c.post("alice", map[string]any{"text": "hello", "visibility": "public"})
	if _, err := c.app.Runner.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Err(); err != nil {
		t.Fatal(err)
	}
	if got := m.Sum("my.accounts.registered", nil); got != 1 {
		t.Errorf("my.accounts.registered = %v, want 1", got)
	}
	if got := m.Sum("my.statuses.created", map[string]string{"my.visibility": "public"}); got != 1 {
		t.Errorf("my.statuses.created = %v, want 1", got)
	}
	if got := len(m.Measurements("my.timeline.delivery.lag", nil)); got != 1 {
		t.Errorf("my.timeline.delivery.lag has %d values, want 1", got)
	}
	if got := m.Measurements("http.server.request.duration", map[string]string{
		"http.route": "/api/v1/statuses", "http.request.method": "POST", "my.surface": "client_api", "http.response.status_code": "201",
	}); len(got) != 1 {
		t.Errorf("http.server.request.duration of POST /api/v1/statuses: %v", got)
	}
	if got := m.Sum("http.server.active_requests", nil); got != 0 {
		t.Errorf("http.server.active_requests = %v after the requests, want 0", got)
	}
	if got := len(m.Measurements("my.job.duration", map[string]string{"my.outcome": "success"})); got == 0 {
		t.Error("no my.job.duration after the post's jobs")
	}

	// The instance-wide gauges come from the snapshot (ADR-0055 §4).
	if err := m.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]float64{"my.instance.users": 1, "my.instance.statuses": 1, "my.queue.dead": 0} {
		if got := m.Measurements(name, nil); len(got) != 1 || got[0].Value != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	if err := m.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(c.app.Scheduler.Schedules(), func(s queue.Schedule) bool { return s.Name == metrics.JobSnapshot }) {
		t.Errorf("no schedule of %s", metrics.JobSnapshot)
	}
}

// TestEMFMetricsSnapshot writes the instance-wide gauges as EMF when the snapshot job runs.
func TestEMFMetricsSnapshot(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_METRICS_EXPORTER": "emf"})
	var lines syncBuffer
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{MetricsWriter: &lines})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, _, ok := app.Handlers.Lookup(metrics.JobSnapshot)
	if !ok {
		t.Fatalf("no handler of %s", metrics.JobSnapshot)
	}
	if res := h(t.Context(), queue.Job{Type: metrics.JobSnapshot}); !res.IsDone() {
		t.Fatalf("%s = %+v", metrics.JobSnapshot, res)
	}
	var found bool
	for line := range strings.Lines(lines.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		if _, ok := m["my.instance.users"]; ok {
			found = true
			if m["my.instance.users"] != 0.0 || m["my.queue.dead"] != 0.0 || m["_aws"] == nil {
				t.Errorf("EMF line = %v", m)
			}
		}
	}
	if !found {
		t.Errorf("no EMF line of the snapshot:\n%s", lines.String())
	}
}

// TestAccessLogMetricLabels checks that the access log carries the labels of
// http.server.request.duration under the same names and values (ADR-0055 §5), with no metrics
// exporter, as on Cloud Run where log-based metrics count the lines.
func TestAccessLogMetricLabels(t *testing.T) {
	t.Parallel()
	s := start(t, loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)}), platform.ServeOptions{}, nil)
	if resp, _ := s.do(t, http.MethodGet, "/api/v1/instance?token=secret", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/instance = %d", resp.StatusCode)
	}
	s.do(t, http.MethodGet, "/ap/nope", nil)
	s.do(t, http.MethodGet, platform.PathLiveness, nil)

	lines := map[string]map[string]any{}
	for line := range strings.Lines(s.logs.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		if path, ok := m["url.path"].(string); ok && m["msg"] == "http request" {
			lines[path] = m
		}
	}
	for path, want := range map[string]map[string]any{
		"/api/v1/instance": {
			"http.request.method": "GET", "http.route": "/api/v1/instance", "my.surface": "client_api", "http.response.status_code": 200.0,
		},
		"/ap/nope":            {"http.request.method": "GET", "http.route": "other", "my.surface": "web", "http.response.status_code": 404.0},
		platform.PathLiveness: {"http.request.method": "GET", "http.response.status_code": 200.0},
	} {
		m, ok := lines[path]
		if !ok {
			t.Errorf("no access log line of %s:\n%s", path, s.logs.String())
			continue
		}
		for k, v := range want {
			if m[k] != v {
				t.Errorf("%s: %s = %v, want %v", path, k, m[k], v)
			}
		}
		if _, ok := m["duration_seconds"].(float64); !ok {
			t.Errorf("%s: duration_seconds = %v, want a number", path, m["duration_seconds"])
		}
		if _, ok := m["request_id"].(string); !ok {
			t.Errorf("%s: no request_id", path)
		}
	}
	if m := lines[platform.PathLiveness]; m != nil && (m["http.route"] != nil || m["my.surface"] != nil) {
		t.Errorf("the health check has a route: %v", m)
	}
	if strings.Contains(s.logs.String(), "secret") {
		t.Errorf("the access log has the query string:\n%s", s.logs.String())
	}
}

// TestClientThatLeftIsNotAServerError checks that a request whose client has left writes neither
// an ERROR line nor a 5xx in the access log, and that a panic and a client that stayed still do.
func TestClientThatLeftIsNotAServerError(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": "web"})
	var logger *slog.Logger
	fail := func(w http.ResponseWriter, r *http.Request) {
		logger.ErrorContext(r.Context(), "handler failed", slog.String("url.path", r.URL.Path))
		w.WriteHeader(http.StatusInternalServerError)
	}
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		logger = a.Logger
		a.Handle("GET /left", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			fail(w, r)
		}))
		a.Handle("GET /left-panic", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			panic("boom")
		}))
		a.Handle("GET /stayed", http.HandlerFunc(fail))
	})

	for _, path := range []string{"/left", "/left-panic"} {
		ctx, cancel := context.WithCancel(t.Context())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		time.AfterFunc(50*time.Millisecond, cancel)
		if resp, err := http.DefaultTransport.RoundTrip(req); err == nil {
			_ = resp.Body.Close()
			t.Errorf("GET %s answered %d, want the client to leave first", path, resp.StatusCode)
		}
		cancel()
	}
	if resp, _ := s.do(t, http.MethodGet, "/stayed", nil); resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("GET /stayed = %d, want 500", resp.StatusCode)
	}

	type line struct {
		msg, path, severity string
		status              float64
	}
	var lines []line
	accessLines := func() int {
		lines = lines[:0]
		n := 0
		for raw := range strings.Lines(s.logs.String()) {
			var m map[string]any
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				t.Fatalf("line %q: %v", raw, err)
			}
			l := line{}
			l.msg, _ = m["msg"].(string)
			l.path, _ = m["url.path"].(string)
			l.severity, _ = m["severity"].(string)
			l.status, _ = m["http.response.status_code"].(float64)
			lines = append(lines, l)
			if l.msg == "http request" && (l.path == "/left" || l.path == "/left-panic" || l.path == "/stayed") {
				n++
			}
		}
		return n
	}
	for deadline := time.Now().Add(10 * time.Second); accessLines() < 3 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond) // the server finishes the requests of the clients that left on its own
	}
	want := map[line]bool{
		{msg: "handler failed", path: "/left", severity: "INFO"}:                  false,
		{msg: "http request", path: "/left", severity: "INFO", status: 499}:       false,
		{msg: "http request", path: "/left-panic", severity: "INFO", status: 500}: false,
		{msg: "handler failed", path: "/stayed", severity: "ERROR"}:               false,
		{msg: "http request", path: "/stayed", severity: "INFO", status: 500}:     false,
	}
	for _, l := range lines {
		if _, ok := want[l]; ok {
			want[l] = true
		}
	}
	for l, found := range want {
		if !found {
			t.Errorf("no line %+v in:\n%s", l, s.logs.String())
		}
	}
	reports := errorReportLines(t, s.logs.String())
	if len(reports) != 1 || reports[0]["my.error.kind"] != "panic" || reports[0]["severity"] != "ERROR" {
		t.Errorf("reports = %v, want the panic at ERROR", reports)
	}
}
