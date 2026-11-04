// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_otel

package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
	"github.com/mycroblog/my/internal/app/queue"
)

// TestTracesFromHTTPToJobs checks ADR-0055 with traces_exporter = otlp: a request from a trusted
// proxy continues its traceparent, its queries are its children, the job it enqueued runs in a
// new trace linked to it, every span reaches the OTLP receiver, no attribute is outside the
// allowlist of §4, and the log lines carry the trace and span IDs.
func TestTracesFromHTTPToJobs(t *testing.T) {
	c := &otlptest.Collector{}
	srv := httptest.NewServer(c)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", srv.URL+"/v1/traces")
	a, logs := newTracedApp(t, map[string]string{
		"MY_TRACES_EXPORTER": "otlp", "MY_TRACES_SAMPLE_RATIO": "1", "MY_TRACES_FLUSH": "unit",
	}, func(context.Context, queue.Job) queue.Result { return queue.Done() })
	h := a.Handler()
	post(t, h, "192.0.2.10:4000")   // a trusted proxy
	post(t, h, "203.0.113.50:4000") // anyone else
	if n, err := a.Runner.Drain(t.Context()); err != nil || n != 2 {
		t.Fatalf("Drain = %d, %v", n, err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	spans := c.Spans()
	var trusted, other *otlptest.Span
	var jobs []otlptest.Span
	for i, s := range spans {
		switch {
		case s.Name == "POST /test/traced" && s.TraceID == incomingTraceID:
			trusted = &spans[i]
		case s.Name == "POST /test/traced":
			other = &spans[i]
		case s.Name == "test.traced":
			jobs = append(jobs, s)
		}
		for k, v := range s.Attrs {
			if !allowedSpanAttributes[k] {
				t.Errorf("span %s has the attribute %s = %q outside the allowlist", s.Name, k, v)
			}
			if strings.Contains(v, "secret") || strings.Contains(v, "192.0.2") || strings.Contains(v, "203.0.113") {
				t.Errorf("span %s attribute %s = %q", s.Name, k, v)
			}
		}
	}
	if trusted == nil || len(jobs) != 2 {
		t.Fatalf("no request span in the trusted proxy's trace, or %d job spans, want 2: %+v", len(jobs), spans)
	}
	if trusted.ParentSpanID != incomingSpanID || trusted.Kind != "server" || trusted.Attrs["http.route"] != "/test/traced" {
		t.Errorf("the trusted proxy's request span = %+v, want a server span under %s", *trusted, incomingSpanID)
	}
	if other == nil || other.ParentSpanID != "" {
		t.Errorf("an untrusted client's request span = %+v, want the root of a new trace", other)
	}

	// The job enqueued by the trusted request: a new trace, linked to the request's span.
	var job *otlptest.Span
	for i, j := range jobs {
		if slices.Equal(j.Links, []string{trusted.SpanID}) {
			job = &jobs[i]
		}
	}
	if job == nil {
		t.Fatalf("no job span links to the request span %s: %+v", trusted.SpanID, jobs)
	}
	if job.TraceID == incomingTraceID || job.ParentSpanID != "" || job.Kind != "consumer" || job.Attrs["my.outcome"] != "success" {
		t.Errorf("job span = %+v, want a consumer root of a new trace", *job)
	}

	// The enqueue's query is in the request's trace, and the job's result in the job's.
	inTrace := func(traceID, name string) bool {
		return slices.ContainsFunc(spans, func(s otlptest.Span) bool { return s.TraceID == traceID && s.Name == name })
	}
	if !inTrace(trusted.TraceID, "InsertJob") || !inTrace(trusted.TraceID, "my.db.transaction") {
		t.Error("the request's trace lacks its enqueue's transaction and query")
	}
	if !inTrace(job.TraceID, "DeleteClaimedJob") {
		t.Error("the job's trace lacks the transaction that completes it")
	}

	// The access log line names the request's span.
	found := false
	for line := range strings.Lines(logs.String()) {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil || m["msg"] != "http request" {
			continue
		}
		if m["trace_id"] == incomingTraceID && m["span_id"] == trusted.SpanID {
			found = true
		}
	}
	if !found {
		t.Errorf("no access log line carries the request's trace_id and span_id:\n%s", logs)
	}
}
