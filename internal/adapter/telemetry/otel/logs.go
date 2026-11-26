// SPDX-License-Identifier: AGPL-3.0-only

package otel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/mycroblog/my/internal/app/kernel"
)

// LogsOptions configure NewLogs.
type LogsOptions struct {
	// ServiceName is the service.name of the resource; the default is "my".
	ServiceName    string
	ServiceVersion string
	// Role is the my.role of the resource: web, worker, or all.
	Role string
	// Namespace is the service.namespace of the resource, the instance's host; "" leaves it to
	// OTEL_RESOURCE_ATTRIBUTES.
	Namespace string
	// Fallback is the handler of the JSON lines on standard error. It decides which levels are
	// logged (Enabled), and it writes the records that the exporter fails to send and the
	// records logged after Shutdown. It is required.
	Fallback slog.Handler
	// TraceContext returns the trace and span of a line's context (kernel.Tracer.LogIDs), which
	// the record carries. nil, or a context without a trace, takes them from the record's
	// trace_id and span_id attributes, if any.
	TraceContext func(context.Context) kernel.TraceParent
	// Exporter replaces the OTLP/HTTP exporter. Tests only.
	Exporter sdklog.Exporter
	// BatchOptions are added to the options of the batch processor. Tests only, such as a short
	// sdklog.WithExportInterval.
	BatchOptions []sdklog.BatchProcessorOption
}

// Logs sends the process's log lines as OpenTelemetry log records (logs_exporter = otlp,
// ADR-0055 §20) over OTLP/HTTP (protobuf, gzip), to the receiver that the standard
// OTEL_EXPORTER_OTLP_LOGS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT and OTEL_EXPORTER_OTLP_*
// environment variables name (default http://localhost:4318/v1/logs), through the SDK's batch
// processor. The records of an export that fails are written to the fallback handler instead,
// so that a receiver that is down loses no line; the exporter does not retry, since the line is
// not lost. Shut it down to send the pending records.
type Logs struct {
	provider *sdklog.LoggerProvider
	logger   log.Logger
	fallback slog.Handler
	trace    func(context.Context) kernel.TraceParent
	closed   atomic.Bool
}

// NewLogs returns the Logs that export through OTLP/HTTP, or opts.Exporter.
func NewLogs(ctx context.Context, opts LogsOptions) (*Logs, error) {
	if opts.Fallback == nil {
		return nil, errors.New("otlp logs: no fallback handler")
	}
	res, err := NewResource(ctx, opts.ServiceName, opts.ServiceVersion, opts.Role, opts.Namespace)
	if err != nil {
		return nil, fmt.Errorf("logs resource: %w", err)
	}
	exp := opts.Exporter
	if exp == nil {
		exp, err = otlploghttp.New(ctx,
			otlploghttp.WithCompression(otlploghttp.GzipCompression),
			// A failed export goes to the fallback at once instead of filling the queue.
			otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: false}),
		)
		if err != nil {
			return nil, fmt.Errorf("otlp log exporter: %w", err)
		}
	}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(fallbackExporter{Exporter: exp, fallback: opts.Fallback}, opts.BatchOptions...)),
	)
	return &Logs{
		provider: provider,
		logger:   provider.Logger("github.com/mycroblog/my"),
		fallback: opts.Fallback,
		trace:    opts.TraceContext,
	}, nil
}

// Handler returns the slog.Handler that emits each record as a log record.
func (l *Logs) Handler() slog.Handler { return &logHandler{logs: l, fallback: l.fallback} }

// ForceFlush exports the pending records.
func (l *Logs) ForceFlush(ctx context.Context) error { return l.provider.ForceFlush(ctx) }

// Shutdown exports the pending records and stops the exporter. The records logged after it are
// written to the fallback handler.
func (l *Logs) Shutdown(ctx context.Context) error {
	l.closed.Store(true)
	return l.provider.Shutdown(ctx)
}

// logHandler is the slog.Handler of Logs. A group of WithGroup or of an attribute prefixes the
// keys of its attributes, "group.key", since the records keep flat attributes.
type logHandler struct {
	logs *Logs
	// attrs are the attributes of WithAttrs, prefix the groups of WithGroup, "a.b.".
	attrs  []attribute.KeyValue
	prefix string
	// fallback is the fallback handler with the same WithAttrs and WithGroup, for after Shutdown.
	fallback slog.Handler
}

func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.fallback.Enabled(ctx, level)
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	c := *h
	c.attrs = appendAttrs(append([]attribute.KeyValue(nil), h.attrs...), h.prefix, attrs)
	c.fallback = h.fallback.WithAttrs(attrs)
	return &c
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.prefix = h.prefix + name + "."
	c.fallback = h.fallback.WithGroup(name)
	return &c
}

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.logs.closed.Load() {
		// The context's request ID is already an attribute of r.
		return h.fallback.Handle(context.Background(), r)
	}
	var rec log.Record
	rec.SetTimestamp(r.Time)
	severity, text := severityOf(r.Level)
	rec.SetSeverity(severity)
	rec.SetSeverityText(text)
	rec.SetBody(attribute.StringValue(r.Message))
	attrs := make([]attribute.KeyValue, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	var traceID, spanID string
	r.Attrs(func(a slog.Attr) bool {
		if h.prefix == "" && a.Value.Kind() == slog.KindString {
			switch a.Key {
			case "trace_id":
				traceID = a.Value.String()
			case "span_id":
				spanID = a.Value.String()
			}
		}
		attrs = appendAttr(attrs, h.prefix, a)
		return true
	})
	rec.AddAttributes(attrs...)
	var p kernel.TraceParent
	if h.logs.trace != nil {
		p = h.logs.trace(ctx)
	}
	if p.TraceID == "" {
		p = kernel.TraceParent{TraceID: traceID, SpanID: spanID}
	}
	// The record's trace comes from the context of Emit; the line's own context is done with.
	emitCtx := context.Background()
	if sc, ok := logSpanContext(p); ok {
		emitCtx = trace.ContextWithSpanContext(emitCtx, sc)
	}
	h.logs.logger.Emit(emitCtx, rec)
	return nil
}

// severityOf returns the OpenTelemetry severity of a slog level and its text: DEBUG, INFO, WARN,
// ERROR, and FATAL above ERROR.
func severityOf(level slog.Level) (log.Severity, string) {
	switch {
	case level > slog.LevelError:
		return log.SeverityFatal, "FATAL"
	case level > slog.LevelWarn:
		return log.SeverityError, "ERROR"
	case level > slog.LevelInfo:
		return log.SeverityWarn, "WARN"
	case level > slog.LevelDebug:
		return log.SeverityInfo, "INFO"
	default:
		return log.SeverityDebug, "DEBUG"
	}
}

// levelOf returns the slog level of an OpenTelemetry severity, the inverse of severityOf.
func levelOf(s log.Severity) slog.Level {
	switch {
	case s >= log.SeverityFatal:
		return slog.LevelError + 4
	case s >= log.SeverityError:
		return slog.LevelError
	case s >= log.SeverityWarn:
		return slog.LevelWarn
	case s >= log.SeverityInfo:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}

// logSpanContext returns the span context of the hexadecimal IDs of p.
func logSpanContext(p kernel.TraceParent) (trace.SpanContext, bool) {
	tid, err := trace.TraceIDFromHex(p.TraceID)
	if err != nil {
		return trace.SpanContext{}, false
	}
	sid, _ := trace.SpanIDFromHex(p.SpanID) // a trace without a span still links the record
	var flags trace.TraceFlags
	if p.Sampled {
		flags = trace.FlagsSampled
	}
	return trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, TraceFlags: flags}), true
}

func appendAttrs(dst []attribute.KeyValue, prefix string, attrs []slog.Attr) []attribute.KeyValue {
	for _, a := range attrs {
		dst = appendAttr(dst, prefix, a)
	}
	return dst
}

// appendAttr appends a as attributes, a group as one attribute per member under "group.", and
// leaves out an empty attribute, as the slog handlers do.
func appendAttr(dst []attribute.KeyValue, prefix string, a slog.Attr) []attribute.KeyValue {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return dst
	}
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		if len(group) == 0 {
			return dst
		}
		if a.Key != "" {
			prefix += a.Key + "."
		}
		return appendAttrs(dst, prefix, group)
	}
	return append(dst, attribute.KeyValue{Key: attribute.Key(prefix + a.Key), Value: logValue(a.Value)})
}

// logValue returns the attribute value of a resolved slog value, as the JSON handler writes it:
// a duration in nanoseconds, a time in RFC 3339, an error that is not a json.Marshaler as its
// text, and any other value as its JSON.
func logValue(v slog.Value) attribute.Value {
	switch v.Kind() {
	case slog.KindString:
		return attribute.StringValue(v.String())
	case slog.KindInt64:
		return attribute.Int64Value(v.Int64())
	case slog.KindUint64:
		if u := v.Uint64(); u <= math.MaxInt64 {
			return attribute.Int64Value(int64(u))
		}
		return attribute.StringValue(v.String())
	case slog.KindFloat64:
		return attribute.Float64Value(v.Float64())
	case slog.KindBool:
		return attribute.BoolValue(v.Bool())
	case slog.KindDuration:
		return attribute.Int64Value(int64(v.Duration()))
	case slog.KindTime:
		return attribute.StringValue(v.Time().Format(time.RFC3339Nano))
	}
	switch x := v.Any().(type) {
	case json.Marshaler:
	case error:
		return attribute.StringValue(x.Error())
	}
	b, err := json.Marshal(v.Any())
	if err != nil {
		return attribute.StringValue(fmt.Sprintf("%+v", v.Any()))
	}
	return attribute.StringValue(string(b))
}

// fallbackExporter writes the records of a failed export to the fallback handler, as JSON lines
// with the same message, level, time, and attributes, followed by one WARN line of the failure.
// It reports no error to the SDK, whose handler would write a line of another shape. A line
// that the fallback cannot write is dropped: nothing is logged about it.
type fallbackExporter struct {
	sdklog.Exporter
	fallback slog.Handler
}

// exportFailedMsg is the msg of the line of a failed export.
const exportFailedMsg = "otlp logs export failed"

func (e fallbackExporter) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.Exporter.Export(ctx, records)
	if err == nil || len(records) == 0 {
		return nil
	}
	// The records carry their request ID as an attribute: the context adds nothing.
	bg := context.Background()
	for i := range records {
		r := fallbackRecord(&records[i])
		if e.fallback.Enabled(bg, r.Level) {
			_ = e.fallback.Handle(bg, r)
		}
	}
	if e.fallback.Enabled(bg, slog.LevelWarn) {
		r := slog.NewRecord(time.Now(), slog.LevelWarn, exportFailedMsg, 0)
		r.AddAttrs(slog.String("error", err.Error()), slog.Int("records", len(records)))
		_ = e.fallback.Handle(bg, r)
	}
	return nil
}

// fallbackRecord returns the slog record of an exported record.
func fallbackRecord(r *sdklog.Record) slog.Record {
	out := slog.NewRecord(r.Timestamp(), levelOf(r.Severity()), r.Body().String(), 0)
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		out.AddAttrs(slogAttr(kv))
		return true
	})
	return out
}

// slogAttr returns the slog attribute of an attribute of a record.
func slogAttr(kv attribute.KeyValue) slog.Attr {
	key := string(kv.Key)
	switch kv.Value.Type() {
	case attribute.STRING:
		return slog.String(key, kv.Value.AsString())
	case attribute.INT64:
		return slog.Int64(key, kv.Value.AsInt64())
	case attribute.FLOAT64:
		return slog.Float64(key, kv.Value.AsFloat64())
	case attribute.BOOL:
		return slog.Bool(key, kv.Value.AsBool())
	}
	return slog.String(key, kv.Value.String())
}
