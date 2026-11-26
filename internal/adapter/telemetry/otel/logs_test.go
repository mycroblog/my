// SPDX-License-Identifier: AGPL-3.0-only

package otel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel"
	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
	"github.com/mycroblog/my/internal/app/kernel"
)

const (
	logTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	logSpanID  = "00f067aa0ba902b7"
)

// logExporter keeps the records it is given, or fails every export with err.
type logExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
	exports int
	err     error
}

func (e *logExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exports++
	if e.err != nil {
		return e.err
	}
	for _, r := range records {
		e.records = append(e.records, r.Clone())
	}
	return nil
}

func (e *logExporter) Shutdown(context.Context) error   { return nil }
func (e *logExporter) ForceFlush(context.Context) error { return nil }

func (e *logExporter) got() ([]sdklog.Record, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...), e.exports
}

// fallbackRecord is a record that the fallback handler took, its attributes flattened with the
// handler's groups.
type fallbackRecord struct {
	ctx   context.Context
	time  time.Time
	level slog.Level
	msg   string
	attrs []slog.Attr
}

// fallbackSink records what a fallback handler and the handlers derived from it take.
type fallbackSink struct {
	mu      sync.Mutex
	records []fallbackRecord
}

func (s *fallbackSink) got() []fallbackRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fallbackRecord(nil), s.records...)
}

type fallbackHandler struct {
	sink   *fallbackSink
	level  slog.Level
	prefix string
	attrs  []slog.Attr
}

func newFallback(level slog.Level) (*fallbackHandler, *fallbackSink) {
	s := &fallbackSink{}
	return &fallbackHandler{sink: s, level: level}, s
}

func (h *fallbackHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *fallbackHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := append([]slog.Attr(nil), h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, slog.Attr{Key: h.prefix + a.Key, Value: a.Value})
		return true
	})
	h.sink.mu.Lock()
	defer h.sink.mu.Unlock()
	h.sink.records = append(h.sink.records, fallbackRecord{ctx: ctx, time: r.Time, level: r.Level, msg: r.Message, attrs: attrs})
	return nil
}

func (h *fallbackHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, a := range attrs {
		c.attrs = append(c.attrs, slog.Attr{Key: h.prefix + a.Key, Value: a.Value})
	}
	return &c
}

func (h *fallbackHandler) WithGroup(name string) slog.Handler {
	c := *h
	c.prefix = h.prefix + name + "."
	return &c
}

func newLogs(t *testing.T, opts otel.LogsOptions) *otel.Logs {
	t.Helper()
	l, err := otel.NewLogs(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Shutdown(context.Background()) })
	return l
}

func flushLogs(t *testing.T, l *otel.Logs) {
	t.Helper()
	if err := l.ForceFlush(t.Context()); err != nil {
		t.Fatalf("ForceFlush = %v", err)
	}
}

// recordAttrs returns the attributes of r in their order.
func recordAttrs(r sdklog.Record) []attribute.KeyValue {
	var kvs []attribute.KeyValue
	r.WalkAttributes(func(kv attribute.KeyValue) bool { kvs = append(kvs, kv); return true })
	return kvs
}

func TestNewLogsRequiresAFallback(t *testing.T) {
	if _, err := otel.NewLogs(t.Context(), otel.LogsOptions{Exporter: &logExporter{}}); err == nil {
		t.Error("NewLogs without a Fallback succeeded")
	}
}

// TestLogsRecordOfALine checks that a line becomes a record with its time, its severity, its
// message as the body, and its attributes in their order, the groups flattened as dotted keys and
// each value of its type.
func TestLogsRecordOfALine(t *testing.T) {
	exp := &logExporter{}
	fallback, sink := newFallback(slog.LevelDebug)
	l := newLogs(t, otel.LogsOptions{Fallback: fallback, Exporter: exp})

	at := time.Date(2026, 11, 26, 12, 34, 56, 789000000, time.UTC)
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	h := l.Handler().WithAttrs([]slog.Attr{slog.Int("a", 1)}).WithGroup("g").WithAttrs([]slog.Attr{slog.String("b", "x")})
	r := slog.NewRecord(at, slog.LevelInfo, "hello", 0)
	r.AddAttrs(
		slog.Bool("c", true),
		slog.Group("h", slog.Float64("x", 2.5), slog.Group("i", slog.Int("y", 3))),
		slog.Duration("d", 1500*time.Millisecond),
		slog.Time("t", stamp),
		slog.Any("err", errors.New("boom")),
	)
	if err := h.Handle(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	flushLogs(t, l)

	records, _ := exp.got()
	if len(records) != 1 {
		t.Fatalf("exported %d records, want 1", len(records))
	}
	rec := records[0]
	if !rec.Timestamp().Equal(at) {
		t.Errorf("timestamp = %v, want %v", rec.Timestamp(), at)
	}
	if rec.Severity() != log.SeverityInfo || rec.SeverityText() != "INFO" {
		t.Errorf("severity = %d %q, want 9 INFO", rec.Severity(), rec.SeverityText())
	}
	if b := rec.Body(); b.Type() != attribute.STRING || b.AsString() != "hello" {
		t.Errorf("body = %v, want the string hello", b.String())
	}
	want := []attribute.KeyValue{
		attribute.Int64("a", 1),
		attribute.String("g.b", "x"),
		attribute.Bool("g.c", true),
		attribute.Float64("g.h.x", 2.5),
		attribute.Int64("g.h.i.y", 3),
		attribute.Int64("g.d", int64(1500*time.Millisecond)),
		attribute.String("g.t", stamp.Format(time.RFC3339Nano)),
		attribute.String("g.err", "boom"),
	}
	got := recordAttrs(rec)
	if len(got) != len(want) {
		t.Fatalf("attributes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i].Key != want[i].Key || got[i].Value.Type() != want[i].Value.Type() || got[i].Value.String() != want[i].Value.String() {
			t.Errorf("attribute %d = %s %s %q, want %s %s %q", i, got[i].Key, got[i].Value.Type(), got[i].Value.String(),
				want[i].Key, want[i].Value.Type(), want[i].Value.String())
		}
	}
	if s := sink.got(); len(s) != 0 {
		t.Errorf("the fallback took %d records of a successful export", len(s))
	}
}

// TestLogsSeverities checks the severity number and text of each slog level.
func TestLogsSeverities(t *testing.T) {
	exp := &logExporter{}
	fallback, _ := newFallback(slog.LevelDebug)
	l := newLogs(t, otel.LogsOptions{Fallback: fallback, Exporter: exp})
	logger := slog.New(l.Handler())
	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError, slog.LevelError + 4}
	for _, level := range levels {
		logger.Log(t.Context(), level, level.String())
	}
	flushLogs(t, l)
	records, _ := exp.got()
	want := []struct {
		number log.Severity
		text   string
	}{{5, "DEBUG"}, {9, "INFO"}, {13, "WARN"}, {17, "ERROR"}, {21, "FATAL"}}
	if len(records) != len(want) {
		t.Fatalf("exported %d records, want %d", len(records), len(want))
	}
	for i, w := range want {
		if records[i].Severity() != w.number || records[i].SeverityText() != w.text {
			t.Errorf("%s: severity = %d %q, want %d %q", levels[i], records[i].Severity(), records[i].SeverityText(), w.number, w.text)
		}
	}
}

// TestLogsLevelsOfTheFallback checks that the fallback decides which levels are logged.
func TestLogsLevelsOfTheFallback(t *testing.T) {
	exp := &logExporter{}
	fallback, _ := newFallback(slog.LevelInfo)
	l := newLogs(t, otel.LogsOptions{Fallback: fallback, Exporter: exp})
	h := l.Handler()
	if h.Enabled(t.Context(), slog.LevelDebug) {
		t.Error("DEBUG is enabled, below the fallback's INFO")
	}
	if !h.Enabled(t.Context(), slog.LevelInfo) || !h.WithGroup("g").Enabled(t.Context(), slog.LevelWarn) {
		t.Error("INFO or WARN is not enabled")
	}
	slog.New(h).Debug("dropped")
	slog.New(h).Info("kept")
	flushLogs(t, l)
	if records, _ := exp.got(); len(records) != 1 || records[0].Body().AsString() != "kept" {
		t.Errorf("exported %d records, want the INFO one", len(records))
	}
}

// TestLogsTraceContext checks that a record carries the trace and span of TraceContext, or else
// those of the line's top-level trace_id and span_id attributes, then not sampled.
func TestLogsTraceContext(t *testing.T) {
	type ctxKey struct{}
	traceContext := func(ctx context.Context) kernel.TraceParent {
		if ctx.Value(ctxKey{}) == nil {
			return kernel.TraceParent{}
		}
		return kernel.TraceParent{TraceID: logTraceID, SpanID: logSpanID, Sampled: true}
	}
	exp := &logExporter{}
	fallback, _ := newFallback(slog.LevelDebug)
	l := newLogs(t, otel.LogsOptions{Fallback: fallback, Exporter: exp, TraceContext: traceContext})
	logger := slog.New(l.Handler())

	logger.InfoContext(context.WithValue(t.Context(), ctxKey{}, true), "in a span")
	logger.InfoContext(t.Context(), "from attributes", "trace_id", "0af7651916cd43dd8448eb211c80319c", "span_id", "b7ad6b7169203331")
	logger.InfoContext(t.Context(), "outside a trace")
	logger.InfoContext(t.Context(), "nested", slog.Group("g", "trace_id", "0af7651916cd43dd8448eb211c80319c", "span_id", "b7ad6b7169203331"))
	flushLogs(t, l)

	records, _ := exp.got()
	if len(records) != 4 {
		t.Fatalf("exported %d records, want 4", len(records))
	}
	for i, w := range []struct {
		traceID, spanID string
		sampled         bool
	}{
		{logTraceID, logSpanID, true},
		{"0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", false},
		{"", "", false},
		{"", "", false},
	} {
		r := records[i]
		traceID, spanID := "", ""
		if r.TraceID().IsValid() {
			traceID = r.TraceID().String()
		}
		if r.SpanID().IsValid() {
			spanID = r.SpanID().String()
		}
		if traceID != w.traceID || spanID != w.spanID || r.TraceFlags().IsSampled() != w.sampled {
			t.Errorf("%s: trace %q span %q sampled %v, want %q %q %v", r.Body().AsString(), traceID, spanID, r.TraceFlags().IsSampled(),
				w.traceID, w.spanID, w.sampled)
		}
	}
}

// TestLogsFailedExportGoesToTheFallback checks that the records of an export that fails reach the
// fallback with their time, level, message, and attributes, followed by one WARN line that says
// how many, and that the failure is not reported to the caller.
func TestLogsFailedExportGoesToTheFallback(t *testing.T) {
	exp := &logExporter{err: errors.New("collector down")}
	fallback, sink := newFallback(slog.LevelDebug)
	l := newLogs(t, otel.LogsOptions{Fallback: fallback, Exporter: exp})
	type ctxKey struct{}
	ctx := context.WithValue(t.Context(), ctxKey{}, "request")

	at := time.Date(2026, 11, 26, 1, 2, 3, 4, time.UTC)
	h := l.Handler().WithAttrs([]slog.Attr{slog.String("request_id", "r1")}).WithGroup("g")
	r := slog.NewRecord(at, slog.LevelWarn, "first", 0)
	r.AddAttrs(slog.Int("n", 7), slog.Group("h", slog.Bool("ok", true)))
	if err := h.Handle(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := l.Handler().Handle(ctx, slog.NewRecord(at.Add(time.Second), slog.LevelError+4, "second", 0)); err != nil {
		t.Fatal(err)
	}
	flushLogs(t, l)

	if _, exports := exp.got(); exports == 0 {
		t.Fatal("the exporter was never called")
	}
	got := sink.got()
	if len(got) != 3 {
		t.Fatalf("the fallback took %d records, want the 2 lines and the warning: %+v", len(got), got)
	}
	first, second, warning := got[0], got[1], got[2]
	if !first.time.Equal(at) || first.level != slog.LevelWarn || first.msg != "first" {
		t.Errorf("first = %v %s %q, want %v WARN first", first.time, first.level, first.msg, at)
	}
	wantAttrs := map[string]string{"request_id": "r1", "g.n": "7", "g.h.ok": "true"}
	gotAttrs := map[string]string{}
	for _, a := range first.attrs {
		gotAttrs[a.Key] = a.Value.String()
	}
	for k, v := range wantAttrs {
		if gotAttrs[k] != v {
			t.Errorf("first attribute %s = %q, want %q (attributes %v)", k, gotAttrs[k], v, gotAttrs)
		}
	}
	if len(gotAttrs) != len(wantAttrs) {
		t.Errorf("first attributes = %v, want %v", gotAttrs, wantAttrs)
	}
	if !second.time.Equal(at.Add(time.Second)) || second.level != slog.LevelError+4 || second.msg != "second" {
		t.Errorf("second = %v %s %q, want ERROR+4 second", second.time, second.level, second.msg)
	}
	for _, rec := range got {
		if rec.ctx.Value(ctxKey{}) != nil {
			t.Errorf("%q reached the fallback with the line's context, want context.Background()", rec.msg)
		}
	}
	if warning.level != slog.LevelWarn || warning.msg != "otlp logs export failed" {
		t.Fatalf("warning = %s %q, want WARN otlp logs export failed", warning.level, warning.msg)
	}
	warnAttrs := map[string]string{}
	for _, a := range warning.attrs {
		warnAttrs[a.Key] = a.Value.String()
	}
	if !strings.Contains(warnAttrs["error"], "collector down") || warnAttrs["records"] != "2" {
		t.Errorf("warning attributes = %v, want the error and records=2", warnAttrs)
	}
}

// TestLogsAfterShutdownGoToTheFallback checks that the lines logged after Shutdown are written by
// the fallback, with the attributes and groups of the handler.
func TestLogsAfterShutdownGoToTheFallback(t *testing.T) {
	exp := &logExporter{}
	var buf bytes.Buffer
	l := newLogs(t, otel.LogsOptions{Fallback: slog.NewJSONHandler(&buf, nil), Exporter: exp})
	logger := slog.New(l.Handler()).With("a", 1).WithGroup("g")
	if err := l.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger.Info("late", "b", "x")
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("fallback output %q: %v", buf.String(), err)
	}
	g, _ := m["g"].(map[string]any)
	if m["msg"] != "late" || m["level"] != "INFO" || m["a"] != float64(1) || g["b"] != "x" {
		t.Errorf("fallback line = %v, want late with a=1 and g.b=x", m)
	}
	if records, _ := exp.got(); len(records) != 0 {
		t.Errorf("exported %d records after Shutdown", len(records))
	}
}

// TestLogsExportOverOTLP checks that without an Exporter the records go over OTLP/HTTP, gzip, to
// the endpoint of OTEL_EXPORTER_OTLP_LOGS_ENDPOINT, with the resource of the service.
func TestLogsExportOverOTLP(t *testing.T) {
	c := &otlptest.LogCollector{}
	srv := httptest.NewServer(c)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", srv.URL+"/v1/logs")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	fallback, sink := newFallback(slog.LevelDebug)
	l := newLogs(t, otel.LogsOptions{
		ServiceVersion: "1.2.3", Role: "worker", Namespace: "example.test", Fallback: fallback,
		TraceContext: func(context.Context) kernel.TraceParent {
			return kernel.TraceParent{TraceID: logTraceID, SpanID: logSpanID, Sampled: true}
		},
	})
	slog.New(l.Handler()).Info("over the wire", "request_id", "r1", "n", 3)
	flushLogs(t, l)

	logs := c.Logs()
	if len(logs) != 1 {
		t.Fatalf("the collector got %d records, want 1 (fallback %+v)", len(logs), sink.got())
	}
	got := logs[0]
	if got.Body != "over the wire" || got.SeverityNumber != 9 || got.SeverityText != "INFO" || !got.Gzip {
		t.Errorf("record = %+v, want the INFO line, gzip", got)
	}
	if got.TraceID != logTraceID || got.SpanID != logSpanID {
		t.Errorf("trace %q span %q, want %s %s", got.TraceID, got.SpanID, logTraceID, logSpanID)
	}
	if got.Attrs["request_id"] != "r1" || got.Attrs["n"] != "3" {
		t.Errorf("attributes = %v", got.Attrs)
	}
	for k, v := range map[string]string{
		"service.name": "my", "service.version": "1.2.3", "my.role": "worker", "service.namespace": "example.test",
	} {
		if got.Resource[k] != v {
			t.Errorf("resource %s = %q, want %q", k, got.Resource[k], v)
		}
	}
}
