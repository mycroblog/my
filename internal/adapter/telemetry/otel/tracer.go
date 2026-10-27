// SPDX-License-Identifier: AGPL-3.0-only

package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The values of traces_flush (ADR-0063 §5).
const (
	// FlushBatch leaves the export to the SDK's batch span processor, which sends periodically.
	FlushBatch = "batch"
	// FlushUnit also sends the recorded spans at the end of every HTTP request and job (the
	// local root spans) before it returns, for the platforms that freeze or throttle an idle
	// instance (Cloud Run with request-based billing, Lambda).
	FlushUnit = "unit"
)

// flushTimeout bounds the export of FlushUnit at the end of a unit of work.
const flushTimeout = 5 * time.Second

// TracerOptions configure NewTracer.
type TracerOptions struct {
	// ServiceName is the service.name of the resource; the default is "my".
	ServiceName    string
	ServiceVersion string
	// Role is the my.role of the resource: web, worker, or all.
	Role string
	// SampleRatio is the share of the root spans without a sampled parent or link that are
	// recorded, from 0 to 1 (traces_sample_ratio).
	SampleRatio float64
	// Flush is FlushBatch (the default) or FlushUnit.
	Flush string
	// Exporter replaces the OTLP/HTTP exporter. Tests only, to read the spans in memory.
	Exporter sdktrace.SpanExporter
}

// Tracer is a kernel.Tracer on an OpenTelemetry TracerProvider (ADR-0063 §1, §5): it exports the
// spans over OTLP/HTTP (protobuf, gzip) to the receiver that the standard
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT and OTEL_EXPORTER_OTLP_* environment variables name
// (default http://localhost:4318/v1/traces), and samples the root spans by trace ID ratio unless
// their parent (a trusted proxy's traceparent) or their link (the request that enqueued a job)
// is sampled, which they follow.
type Tracer struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	unit     bool
}

var _ kernel.Tracer = (*Tracer)(nil)

// NewTracer returns the Tracer that exports through OTLP/HTTP, or opts.Exporter. Shut it down
// to flush and stop the exporter.
func NewTracer(ctx context.Context, opts TracerOptions) (*Tracer, error) {
	res, err := NewResource(ctx, opts.ServiceName, opts.ServiceVersion, opts.Role)
	if err != nil {
		return nil, fmt.Errorf("traces resource: %w", err)
	}
	exp := opts.Exporter
	if exp == nil {
		if exp, err = otlptracehttp.New(ctx, otlptracehttp.WithCompression(otlptracehttp.GzipCompression)); err != nil {
			return nil, fmt.Errorf("otlp trace exporter: %w", err)
		}
	}
	switch opts.Flush {
	case "", FlushBatch, FlushUnit:
	default:
		return nil, fmt.Errorf("traces flush %q is not batch or unit", opts.Flush)
	}
	ratio := min(max(opts.SampleRatio, 0), 1)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sdktrace.ParentBased(linkSampler{ratio: sdktrace.TraceIDRatioBased(ratio)})),
	)
	return &Tracer{
		provider: provider,
		tracer:   provider.Tracer("github.com/mycroblog/my"),
		unit:     opts.Flush == FlushUnit,
	}, nil
}

// Shutdown flushes the pending spans and stops the exporter.
func (t *Tracer) Shutdown(ctx context.Context) error { return t.provider.Shutdown(ctx) }

// ForceFlush exports the pending spans.
func (t *Tracer) ForceFlush(ctx context.Context) error { return t.provider.ForceFlush(ctx) }

// linkSampler records a root span that links to a sampled span, so that the jobs of a recorded
// request are recorded too (ADR-0063 §2), and samples the others by ratio.
type linkSampler struct{ ratio sdktrace.Sampler }

func (s linkSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	for _, l := range p.Links {
		if l.SpanContext.IsSampled() {
			return sdktrace.SamplingResult{
				Decision:   sdktrace.RecordAndSample,
				Tracestate: trace.SpanContextFromContext(p.ParentContext).TraceState(),
			}
		}
	}
	return s.ratio.ShouldSample(p)
}

func (s linkSampler) Description() string { return "LinkSampled{" + s.ratio.Description() + "}" }

// linked marks a context of Linked: the next span it starts is a new root with the link sc.
// Start consumes it, so that the children of that span are children again.
type linked struct {
	sc     trace.SpanContext
	active bool
}

type linkedKey struct{}

// Start implements kernel.Tracer.
func (t *Tracer) Start(ctx context.Context, spec kernel.SpanSpec, attrs kernel.Attrs) (context.Context, kernel.Span) {
	kvs := make([]attribute.KeyValue, 0, attrs.Len())
	for _, a := range attrs.All() {
		if spec.Accepts(a.Key) {
			kvs = append(kvs, keyValue(a))
		}
	}
	opts := []trace.SpanStartOption{trace.WithSpanKind(spanKind(spec.Kind)), trace.WithAttributes(kvs...)}
	parent := trace.SpanContextFromContext(ctx)
	root := !parent.IsValid() || parent.IsRemote()
	if l, ok := ctx.Value(linkedKey{}).(linked); ok && l.active {
		root = true
		opts = append(opts, trace.WithNewRoot())
		if l.sc.IsValid() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: l.sc}))
		}
		ctx = context.WithValue(ctx, linkedKey{}, linked{})
	}
	ctx, s := t.tracer.Start(ctx, spec.Name, opts...)
	return ctx, &span{s: s, t: t, spec: spec, flush: t.unit && root, ctx: ctx}
}

// Inject implements kernel.Tracer.
func (t *Tracer) Inject(ctx context.Context) string {
	s := trace.SpanFromContext(ctx)
	if !s.IsRecording() {
		return ""
	}
	return traceParent(s.SpanContext())
}

// Continue implements kernel.Tracer: the traceparent becomes the remote parent of the next span;
// a tracestate is never taken (ADR-0063 §3).
func (t *Tracer) Continue(ctx context.Context, traceparent string) context.Context {
	sc, ok := spanContext(traceparent)
	if !ok {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

// Linked implements kernel.Tracer.
func (t *Tracer) Linked(ctx context.Context, traceparent string) context.Context {
	sc, _ := spanContext(traceparent)
	return context.WithValue(ctx, linkedKey{}, linked{sc: sc, active: true})
}

// LogIDs implements kernel.Tracer.
func (t *Tracer) LogIDs(ctx context.Context) (string, string) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return "", ""
	}
	return sc.TraceID().String(), sc.SpanID().String()
}

// spanContext parses a traceparent into a remote span context.
func spanContext(traceparent string) (trace.SpanContext, bool) {
	p, ok := kernel.ParseTraceParent(traceparent)
	if !ok {
		return trace.SpanContext{}, false
	}
	traceID, err := trace.TraceIDFromHex(p.TraceID)
	if err != nil {
		return trace.SpanContext{}, false
	}
	spanID, err := trace.SpanIDFromHex(p.SpanID)
	if err != nil {
		return trace.SpanContext{}, false
	}
	var flags trace.TraceFlags
	if p.Sampled {
		flags = trace.FlagsSampled
	}
	return trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: flags, Remote: true}), true
}

// traceParent formats sc as a traceparent.
func traceParent(sc trace.SpanContext) string {
	return kernel.TraceParent{TraceID: sc.TraceID().String(), SpanID: sc.SpanID().String(), Sampled: sc.IsSampled()}.String()
}

func spanKind(k kernel.SpanKind) trace.SpanKind {
	switch k {
	case kernel.SpanServer:
		return trace.SpanKindServer
	case kernel.SpanClient:
		return trace.SpanKindClient
	case kernel.SpanProducer:
		return trace.SpanKindProducer
	case kernel.SpanConsumer:
		return trace.SpanKindConsumer
	}
	return trace.SpanKindInternal
}

func keyValue(a kernel.Attr) attribute.KeyValue {
	if a.IsInt() {
		return attribute.Int64(a.Key, a.Int())
	}
	return attribute.String(a.Key, a.Str())
}

// span is a kernel.Span on an OpenTelemetry span.
type span struct {
	s    trace.Span
	t    *Tracer
	spec kernel.SpanSpec
	// flush exports the spans at End: the span is a local root of FlushUnit.
	flush bool
	ctx   context.Context
}

func (s *span) Set(a kernel.Attr) {
	if s.spec.Accepts(a.Key) {
		s.s.SetAttributes(keyValue(a))
	}
}

func (s *span) SetName(name string) { s.s.SetName(name) }

func (s *span) Recording() bool { return s.s.IsRecording() }

func (s *span) End(errorType string) {
	recording := s.s.IsRecording()
	if errorType != "" && recording {
		s.s.SetAttributes(attribute.String(kernel.AttrErrorType, errorType))
		s.s.SetStatus(codes.Error, "")
	}
	s.s.End()
	if s.flush && recording {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), flushTimeout)
		defer cancel()
		_ = s.t.provider.ForceFlush(ctx) // a span that the receiver did not take is lost
	}
}
