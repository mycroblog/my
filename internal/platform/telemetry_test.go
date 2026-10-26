// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
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

// TestPrometheusMetrics serves the scrape on its own listener and checks the HTTP and job
// metrics of ADR-0055 §3 in it.
func TestPrometheusMetrics(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_METRICS_EXPORTER": "prometheus"})
	mln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ran := make(chan struct{}, 1)
	s := start(t, cfg, platform.ServeOptions{MetricsListener: mln}, func(a *platform.App) {
		a.Handlers.Register("test.metrics", func(context.Context, queue.Job) queue.Result {
			ran <- struct{}{}
			return queue.Done()
		})
	})
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.metrics"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(10 * time.Second):
		t.Fatal("the job did not run")
	}

	if resp, _ := s.do(t, http.MethodGet, "/api/v1/instance", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/instance = %d", resp.StatusCode)
	}
	s.do(t, http.MethodGet, "/ap/nope", nil)
	s.do(t, http.MethodGet, platform.PathLiveness, nil)
	// The public listener never serves the metrics.
	if _, body := s.do(t, http.MethodGet, platform.PathMetrics, nil); strings.Contains(body, "http_server_request_duration") {
		t.Error("the public listener serves the metrics")
	}

	scrape := func() string {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+mln.Addr().String()+platform.PathMetrics, nil)
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	var out string
	deadline := time.Now().Add(10 * time.Second)
	for {
		out = scrape()
		// The job's result is recorded after the handler returns.
		if strings.Contains(out, `my_job_duration_seconds_count{my_job_type="test.metrics",my_outcome="success"} 1`) || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, want := range []string{
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="200",http_route="/api/v1/instance",my_surface="client_api"} 1`,
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="404",http_route="other",my_surface="web"} 1`,
		`http_server_active_requests{my_surface="client_api"} 0`,
		`my_job_duration_seconds_count{my_job_type="test.metrics",my_outcome="success"} 1`,
		`my_job_wait_seconds_count{my_job_type="test.metrics"} 1`,
		"\nmy_queue_dead 0\n",
		"\nmy_instance_users 0\n",
		`my_media_stored_bytes{my_media_origin="remote_cache"`,
		"\nmy_db_size_bytes ",
		`my_federation_instances{my_federation_instance_state="unavailable"} 0`,
		`my_metrics_snapshot_age_seconds{my_metrics_snapshot_part="queue"`,
		`target_info{`,
		// The system metrics of ADR-0056 §1.
		"\ngo_goroutine_count ",
		`go_schedule_duration_seconds_bucket{`,
		`go_memory_gc_pause_duration_seconds_bucket{`,
		"\ngo_memory_gc_cycles_total ",
		"\nprocess_uptime_seconds ",
		`db_client_connection_count{db_client_connection_pool_name="sqlite",db_client_connection_state="used"}`,
		`my_adapter_operation_duration_seconds_count{my_adapter="queue.db",my_operation="enqueue",my_outcome="success"}`,
		`my_adapter_operation_duration_seconds_count{my_adapter="kv.db",`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("scrape lacks %s\n%s", want, out)
		}
	}
	if strings.Contains(out, `http_route="/healthz"`) || strings.Contains(out, `GET /healthz`) {
		t.Errorf("the health checks are measured\n%s", out)
	}
}

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
		"http.route": "/api/v1/statuses", "http.request.method": "POST", "my.surface": "client_api", "http.response.status_code": "200",
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
