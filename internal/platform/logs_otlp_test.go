// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_otel

package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

const jobLine = "test job ran"

// TestOTLPLogsOfARequestAndAJob checks that with logs_exporter = otlp the collector receives the
// lines of a request and of a job, with the attributes of their lines on stderr and the trace and
// span of their spans, by the time the App closes; and that the App logger writes nothing on
// stderr meanwhile.
func TestOTLPLogsOfARequestAndAJob(t *testing.T) {
	logsCollector, tracesCollector := &otlptest.LogCollector{}, &otlptest.Collector{}
	logsSrv, tracesSrv := httptest.NewServer(logsCollector), httptest.NewServer(tracesCollector)
	defer logsSrv.Close()
	defer tracesSrv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", logsSrv.URL+"/v1/logs")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", tracesSrv.URL+"/v1/traces")

	var a *App
	var jobID string
	a, stderr := newTracedApp(t, map[string]string{
		"MY_LOGS_EXPORTER": config.LogsOTLP, "MY_TRACES_EXPORTER": "otlp", "MY_TRACES_SAMPLE_RATIO": "1", "MY_TRACES_FLUSH": "unit",
	}, func(ctx context.Context, j queue.Job) queue.Result {
		jobID = j.ID
		a.Logger.InfoContext(ctx, jobLine, "job_id", j.ID, "attempt", j.Attempt)
		return queue.Done()
	})
	post(t, a.Handler(), "192.0.2.10:4000")
	if n, err := a.Runner.Drain(t.Context()); err != nil || n != 1 {
		t.Fatalf("Drain = %d, %v", n, err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	var requestSpan, jobSpan *otlptest.Span
	spans := tracesCollector.Spans()
	for i, s := range spans {
		switch s.Name {
		case "POST /test/traced":
			requestSpan = &spans[i]
		case "test.traced":
			jobSpan = &spans[i]
		}
	}
	if requestSpan == nil || jobSpan == nil {
		t.Fatalf("no request span or no job span: %+v", spans)
	}

	var access, job *otlptest.Log
	logs := logsCollector.Logs()
	for i, l := range logs {
		switch l.Body {
		case "http request":
			access = &logs[i]
		case jobLine:
			job = &logs[i]
		}
		if l.Resource["service.namespace"] != "example.test" {
			t.Errorf("record %q resource service.namespace = %q, want example.test", l.Body, l.Resource["service.namespace"])
		}
	}
	if access == nil || job == nil {
		t.Fatalf("the collector lacks the access line or the job's line: %+v", logs)
	}
	if access.TraceID != incomingTraceID || access.SpanID != requestSpan.SpanID {
		t.Errorf("access record trace %q span %q, want %s %s", access.TraceID, access.SpanID, incomingTraceID, requestSpan.SpanID)
	}
	if access.SeverityText != "INFO" || access.Attrs["request_id"] == "" || access.Attrs["url.path"] != "/test/traced" {
		t.Errorf("access record = %+v, want INFO with request_id and url.path", *access)
	}
	if job.TraceID != jobSpan.TraceID || job.SpanID != jobSpan.SpanID {
		t.Errorf("job record trace %q span %q, want the job span's %s %s", job.TraceID, job.SpanID, jobSpan.TraceID, jobSpan.SpanID)
	}
	if job.Attrs["job_id"] != jobID || job.Attrs["attempt"] != "1" {
		t.Errorf("job record attributes = %v, want job_id %s and attempt 1", job.Attrs, jobID)
	}
	for line := range strings.Lines(stderr.String()) {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && (m["msg"] == "http request" || m["msg"] == jobLine) {
			t.Errorf("stderr has a line that the collector received: %s", line)
		}
	}
}

// TestOTLPLogsAccessLineKeepsTheAttributesOfStderr checks that the access record carries every
// attribute of the access line on stderr, apart from the fields that the record holds itself.
func TestOTLPLogsAccessLineKeepsTheAttributesOfStderr(t *testing.T) {
	a, stderr := newTracedApp(t, nil, func(context.Context, queue.Job) queue.Result { return queue.Done() })
	post(t, a.Handler(), "192.0.2.10:4000")
	var line map[string]any
	for l := range strings.Lines(stderr.String()) {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["msg"] == "http request" {
			line = m
		}
	}
	if line == nil {
		t.Fatalf("no access line on stderr:\n%s", stderr)
	}

	c := &otlptest.LogCollector{}
	srv := httptest.NewServer(c)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", srv.URL+"/v1/logs")
	b, _ := newTracedApp(t, map[string]string{"MY_LOGS_EXPORTER": config.LogsOTLP}, func(context.Context, queue.Job) queue.Result { return queue.Done() })
	post(t, b.Handler(), "192.0.2.10:4000")
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	var access *otlptest.Log
	logs := c.Logs()
	for i := range logs {
		if logs[i].Body == "http request" {
			access = &logs[i]
		}
	}
	if access == nil {
		t.Fatalf("the collector lacks the access line: %+v", logs)
	}
	for k := range line {
		switch k {
		case "time", "level", "msg", "severity", "trace_id", "span_id":
			continue
		}
		if _, ok := access.Attrs[k]; !ok {
			t.Errorf("the access record lacks %s, which the stderr line has: %v", k, access.Attrs)
		}
	}
	if access.TraceID != incomingTraceID || access.SpanID != incomingSpanID {
		t.Errorf("access record trace %q span %q, want the proxy's %s %s", access.TraceID, access.SpanID, incomingTraceID, incomingSpanID)
	}
}

// TestOTLPLogsCollectorDown checks that with the collector unreachable the lines reach stderr
// through the fallback, with the warning of the failed export, and that the server keeps serving
// requests and jobs.
func TestOTLPLogsCollectorDown(t *testing.T) {
	down := httptest.NewServer(&otlptest.LogCollector{})
	endpoint := down.URL + "/v1/logs"
	down.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", endpoint)
	var a *App
	a, stderr := newTracedApp(t, map[string]string{"MY_LOGS_EXPORTER": config.LogsOTLP}, func(ctx context.Context, j queue.Job) queue.Result {
		a.Logger.InfoContext(ctx, jobLine, "job_id", j.ID)
		return queue.Done()
	})
	h := a.Handler()
	for range 3 {
		post(t, h, "192.0.2.10:4000")
	}
	if n, err := a.Runner.Drain(t.Context()); err != nil || n != 3 {
		t.Fatalf("Drain = %d, %v", n, err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	var access, jobs, warnings int
	for line := range strings.Lines(stderr.String()) {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		switch m["msg"] {
		case "http request":
			access++
			if m["trace_id"] != incomingTraceID || m["span_id"] != incomingSpanID || m["request_id"] == nil {
				t.Errorf("access line = %v, want the proxy's trace and a request_id", m)
			}
		case jobLine:
			jobs++
			if m["job_id"] == nil {
				t.Errorf("job line = %v, want its job_id", m)
			}
		case "otlp logs export failed":
			warnings++
			if m["level"] != "WARN" || m["error"] == nil || m["records"] == nil {
				t.Errorf("warning = %v, want WARN with error and records", m)
			}
		}
	}
	if access != 3 || jobs != 3 || warnings == 0 {
		t.Errorf("stderr has %d access lines, %d job lines, and %d warnings; want 3, 3, and some:\n%s", access, jobs, warnings, stderr)
	}
}

// TestOTLPLogsAfterCloseGoToStderr checks that the App logger still writes on stderr once the App
// has closed the exporter.
func TestOTLPLogsAfterCloseGoToStderr(t *testing.T) {
	c := &otlptest.LogCollector{}
	srv := httptest.NewServer(c)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", srv.URL+"/v1/logs")
	a, stderr := newTracedApp(t, map[string]string{"MY_LOGS_EXPORTER": config.LogsOTLP}, func(context.Context, queue.Job) queue.Result { return queue.Done() })
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a.Logger.Info("after close", "n", 1)
	if !strings.Contains(stderr.String(), `"msg":"after close"`) {
		t.Errorf("stderr lacks the line logged after Close:\n%s", stderr)
	}
	for _, l := range c.Logs() {
		if l.Body == "after close" {
			t.Error("the collector received a line logged after Close")
		}
	}
}
