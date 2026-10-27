// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_otel

package platform_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
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
