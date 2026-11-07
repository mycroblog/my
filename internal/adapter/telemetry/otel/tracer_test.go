// SPDX-License-Identifier: AGPL-3.0-only

package otel_test

import (
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel"
	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

func newTracer(t *testing.T, ratio float64, flush string) (*otel.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tr, err := otel.NewTracer(t.Context(), otel.TracerOptions{ServiceVersion: "1.2.3", Role: "all", SampleRatio: ratio, Flush: flush, Exporter: exp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Shutdown(t.Context()) })
	return tr, exp
}

func TestTracerContract(t *testing.T) {
	t.Run("Sampled", func(t *testing.T) {
		kerneltest.TracerContract(t, func(t *testing.T) kernel.Tracer { tr, _ := newTracer(t, 1, otel.FlushBatch); return tr })
	})
	t.Run("NotSampled", func(t *testing.T) {
		kerneltest.TracerContract(t, func(t *testing.T) kernel.Tracer { tr, _ := newTracer(t, 0, otel.FlushUnit); return tr })
	})
}

func flushed(t *testing.T, tr *otel.Tracer, exp *tracetest.InMemoryExporter) tracetest.SpanStubs {
	t.Helper()
	if err := tr.ForceFlush(t.Context()); err != nil {
		t.Fatal(err)
	}
	return exp.GetSpans()
}

func attrs(s tracetest.SpanStub) map[attribute.Key]attribute.Value {
	m := map[attribute.Key]attribute.Value{}
	for _, kv := range s.Attributes {
		m[kv.Key] = kv.Value
	}
	return m
}

// TestTracerKeepsOnlyDeclaredAttributes checks the allowlist of ADR-0055 §16: an attribute whose
// key the spec did not declare, such as a URL, a client IP, or a User-Agent, never reaches the
// receiver.
func TestTracerKeepsOnlyDeclaredAttributes(t *testing.T) {
	tr, exp := newTracer(t, 1, otel.FlushBatch)
	spec := kernel.SpanSpec{Name: "GET", Kind: kernel.SpanClient, AttrKeys: []string{"http.request.method", "server.address", "http.response.status_code"}}
	_, s := tr.Start(t.Context(), spec, kernel.NewAttrs(
		kernel.StringAttr("http.request.method", "GET"),
		kernel.StringAttr("server.address", "remote.example"),
		kernel.StringAttr("url.full", "https://remote.example/users/alice"),
		kernel.StringAttr("client.address", "192.0.2.1"),
	))
	s.Set(kernel.IntAttr("http.response.status_code", 503))
	s.Set(kernel.StringAttr("user_agent.original", "Mastodon/4.5"))
	s.End("503")

	spans := flushed(t, tr, exp)
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	got := attrs(spans[0])
	want := map[attribute.Key]attribute.Value{
		"http.request.method":       attribute.StringValue("GET"),
		"server.address":            attribute.StringValue("remote.example"),
		"http.response.status_code": attribute.Int64Value(503),
		"error.type":                attribute.StringValue("503"),
	}
	if len(got) != len(want) {
		t.Errorf("attributes = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attribute %s = %v, want %v", k, got[k].String(), v.String())
		}
	}
	if spans[0].Status.Code != codes.Error || spans[0].Status.Description != "" {
		t.Errorf("status = %+v, want an error without a message", spans[0].Status)
	}
	if spans[0].SpanKind.String() != "client" {
		t.Errorf("kind = %v, want client", spans[0].SpanKind)
	}
}

// TestTracerLinksJobsToTheirRequest checks ADR-0055 §14: the run of a job is the root of a new
// trace with a link to the span that enqueued it, recorded because that span was, even at a
// ratio of 0; and the spans inside the run are its children.
func TestTracerLinksJobsToTheirRequest(t *testing.T) {
	tr, exp := newTracer(t, 0, otel.FlushBatch)
	spec := kernel.SpanSpec{Name: "POST /api/v1/statuses", Kind: kernel.SpanServer}
	// A trusted proxy's sampled parent makes the request recorded.
	ctx, req := tr.Start(tr.Continue(t.Context(), kerneltest.ContractTraceParent), spec, kernel.Attrs{})
	tp := tr.Inject(ctx)
	req.End("")
	if tp == "" {
		t.Fatal("Inject of a recorded request = empty")
	}
	jctx, job := tr.Start(tr.Linked(ctx, tp), kernel.SpanSpec{Name: "federation.deliver", Kind: kernel.SpanConsumer}, kernel.Attrs{})
	_, q := tr.Start(jctx, kernel.SpanSpec{Name: "InsertJob", Kind: kernel.SpanClient}, kernel.Attrs{})
	q.End("")
	job.End("")
	// Without a sampled link, a job of ratio 0 is not recorded.
	_, other := tr.Start(tr.Linked(t.Context(), ""), kernel.SpanSpec{Name: "federation.deliver"}, kernel.Attrs{})
	if other.Recording() {
		t.Error("a job without a link is recorded at ratio 0")
	}
	other.End("")

	spans := flushed(t, tr, exp)
	if len(spans) != 3 {
		t.Fatalf("spans = %d, want 3", len(spans))
	}
	byName := map[string]tracetest.SpanStub{}
	for _, s := range spans {
		byName[s.Name] = s
	}
	r, j, i := byName["POST /api/v1/statuses"], byName["federation.deliver"], byName["InsertJob"]
	if r.SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || r.Parent.SpanID().String() != "00f067aa0ba902b7" {
		t.Errorf("request span = %v under %v, want the child of the trusted parent", r.SpanContext, r.Parent)
	}
	if j.Parent.IsValid() || j.SpanContext.TraceID() == r.SpanContext.TraceID() {
		t.Errorf("job span under %v in trace %v, want a new root", j.Parent, j.SpanContext.TraceID())
	}
	if len(j.Links) != 1 || j.Links[0].SpanContext.SpanID() != r.SpanContext.SpanID() {
		t.Errorf("job links = %v, want the request span", j.Links)
	}
	if i.Parent.SpanID() != j.SpanContext.SpanID() {
		t.Errorf("query span under %v, want the job span", i.Parent)
	}
}

// TestTracerExportsOverOTLP checks that the spans go over OTLP/HTTP (protobuf, gzip) to the
// receiver of OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, and that traces_flush = unit sends them when
// the local root span ends.
func TestTracerExportsOverOTLP(t *testing.T) {
	c := &otlptest.Collector{}
	srv := httptest.NewServer(c)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", srv.URL+"/v1/traces")

	tr, err := otel.NewTracer(t.Context(), otel.TracerOptions{ServiceVersion: "1.2.3", Role: "web", SampleRatio: 1, Flush: otel.FlushUnit})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Shutdown(t.Context()) }()
	ctx, root := tr.Start(t.Context(), kernel.SpanSpec{Name: "GET /api/v1/instance", Kind: kernel.SpanServer}, kernel.Attrs{})
	_, child := tr.Start(ctx, kernel.SpanSpec{Name: "GetInstance", Kind: kernel.SpanClient}, kernel.Attrs{})
	child.End("")
	root.End("") // a local root of traces_flush = unit sends before End returns

	spans := c.Spans()
	if len(spans) != 2 {
		t.Fatalf("exported spans = %+v, want the request and its query", spans)
	}
	for _, s := range spans {
		if !s.Gzip {
			t.Errorf("span %s was not sent gzip-compressed", s.Name)
		}
		for k, v := range map[string]string{"service.name": "my", "service.version": "1.2.3", "my.role": "web"} {
			if s.Resource[k] != v {
				t.Errorf("resource %s = %q, want %q", k, s.Resource[k], v)
			}
		}
	}
	if spans[0].Name != "GetInstance" || spans[0].ParentSpanID != spans[1].SpanID || spans[0].Kind != "client" {
		t.Errorf("spans = %+v, want the query under the request", spans)
	}
}

func TestNewTracerRejectsAnUnknownFlush(t *testing.T) {
	if _, err := otel.NewTracer(t.Context(), otel.TracerOptions{Flush: "never", Exporter: tracetest.NewInMemoryExporter()}); err == nil {
		t.Error("NewTracer with flush never = nil error")
	}
}

var _ sdktrace.SpanExporter = (*tracetest.InMemoryExporter)(nil)
