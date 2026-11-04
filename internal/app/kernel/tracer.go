// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// Tracer records the spans of the distributed traces (ADR-0055 §13). A span is declared once,
// at startup, with a SpanSpec that names it, gives its kind, and lists the attribute keys it
// accepts, and is then started with Start and ended with Span.End.
//
// Attributes take their values from finite sets or from the destination host of an outbound
// request (ADR-0055 §16): never an account, status, or job ID, a user name, a URL or a path, a
// client IP, a User-Agent, an SQL text, or an error message. An attribute whose key the spec did
// not declare is a programming error: adapters drop it, and testkit.Tracer reports it so that
// tests fail.
//
// The trace context crosses the process only where ADR-0055 §15 allows: Continue takes the
// traceparent of a trusted proxy as the parent of the next span, Inject writes the current span
// into a job (queue.Job.TraceParent), and Linked makes the run of that job a new trace with a
// link to it. Nothing sends a traceparent to a remote server.
//
// The adapters live in internal/adapter/telemetry; NopTracer records nothing.
type Tracer interface {
	// Start starts a span of spec with attrs as the child of the span in ctx, or as a new trace
	// (with the parent of Continue or the link of Linked, when ctx carries one). End the span
	// exactly once.
	Start(ctx context.Context, spec SpanSpec, attrs Attrs) (context.Context, Span)
	// Inject returns the W3C traceparent of the span in ctx, or "" when ctx has no span that
	// the Tracer records.
	Inject(ctx context.Context) string
	// Continue returns ctx with traceparent as the remote parent of the next span that ctx
	// starts. Call it only for a traceparent from a trusted proxy (ADR-0055 §15); an invalid
	// traceparent leaves ctx as it is.
	Continue(ctx context.Context, traceparent string) context.Context
	// Linked returns ctx without a parent: the next span that ctx starts is the root of a new
	// trace, linked to traceparent when it is valid (a job's TraceParent, ADR-0055 §14).
	Linked(ctx context.Context, traceparent string) context.Context
	// LogIDs returns the trace and span IDs that the log lines of ctx carry (ADR-0055 §18): those
	// of the span in ctx, or else those of the remote parent of Continue; "" outside a trace.
	LogIDs(ctx context.Context) (traceID, spanID string)
}

// Span is a span in progress. It is not safe for concurrent use.
type Span interface {
	// Set sets an attribute that the spec declared.
	Set(a Attr)
	// SetName replaces the name of the span, for the spans of HTTP requests that are named
	// after their route once the router has found it.
	SetName(name string)
	// Recording reports whether the Tracer records the span, so that the caller can skip the
	// work of computing attributes.
	Recording() bool
	// End ends the span. A non-empty errorType marks it failed with that error.type, a name from
	// a finite set (ErrorType). Call it exactly once.
	End(errorType string)
}

// SpanKind is the kind of a span, as in OpenTelemetry.
type SpanKind int

// The span kinds.
const (
	SpanInternal SpanKind = iota
	SpanServer
	SpanClient
	SpanProducer
	SpanConsumer
)

// SpanSpec declares a span (ADR-0055 §13).
type SpanSpec struct {
	// Name follows the OpenTelemetry semantic conventions where they name the span, and
	// starts with "my." otherwise.
	Name string
	Kind SpanKind
	// AttrKeys are the attribute keys that the span accepts, at most MaxSpanAttrs. error.type,
	// which End sets, is always accepted.
	AttrKeys []string
}

// AttrErrorType is the attribute that Span.End sets for a failed span.
const AttrErrorType = "error.type"

// Accepts reports whether the spec declared the attribute key.
func (s SpanSpec) Accepts(key string) bool {
	return key == AttrErrorType || slices.Contains(s.AttrKeys, key)
}

// Check reports the first attribute of attrs whose key s did not declare.
func (s SpanSpec) Check(attrs Attrs) error {
	for _, a := range attrs.All() {
		if err := s.CheckAttr(a); err != nil {
			return err
		}
	}
	return nil
}

// CheckAttr reports whether s declared the key of a.
func (s SpanSpec) CheckAttr(a Attr) error {
	if !s.Accepts(a.Key) {
		return fmt.Errorf("kernel: span %s has no attribute %q (declared %v)", s.Name, a.Key, s.AttrKeys)
	}
	return nil
}

// Named returns s with another name, for the spans named after a value of a finite set
// (the route of an HTTP request, the type of a job).
func (s SpanSpec) Named(name string) SpanSpec {
	s.Name = name
	return s
}

// MaxSpanAttrs is the most attributes one span carries, besides error.type.
const MaxSpanAttrs = 6

// Attr is one span attribute: a string or an integer.
type Attr struct {
	Key   string
	str   string
	num   int64
	isInt bool
}

// StringAttr returns the attribute key with a string value.
func StringAttr(key, value string) Attr { return Attr{Key: key, str: value} }

// IntAttr returns the attribute key with an integer value.
func IntAttr(key string, value int64) Attr { return Attr{Key: key, num: value, isInt: true} }

// IsInt reports whether a has an integer value.
func (a Attr) IsInt() bool { return a.isInt }

// Str returns the string value of a, or the integer in decimal.
func (a Attr) Str() string {
	if a.isInt {
		return strconv.FormatInt(a.num, 10)
	}
	return a.str
}

// Int returns the integer value of a, or 0 for a string.
func (a Attr) Int() int64 { return a.num }

// Attrs are the attributes a span starts with. It is a value, so that starting a span does not
// allocate; build it with NewAttrs or With.
type Attrs struct {
	n int
	a [MaxSpanAttrs]Attr
}

// NewAttrs returns the attributes as. It panics beyond MaxSpanAttrs, a programming error.
func NewAttrs(as ...Attr) Attrs {
	var out Attrs
	for _, a := range as {
		out = out.With(a)
	}
	return out
}

// With returns attrs with a, replacing an attribute of the same key. It panics beyond
// MaxSpanAttrs.
func (attrs Attrs) With(a Attr) Attrs {
	for i := range attrs.n {
		if attrs.a[i].Key == a.Key {
			attrs.a[i] = a
			return attrs
		}
	}
	if attrs.n == MaxSpanAttrs {
		panic(fmt.Sprintf("kernel: more than %d span attributes", MaxSpanAttrs))
	}
	attrs.a[attrs.n] = a
	attrs.n++
	return attrs
}

// Len returns the number of attributes.
func (attrs Attrs) Len() int { return attrs.n }

// All yields the attributes in the order they were set.
func (attrs Attrs) All() func(yield func(int, Attr) bool) {
	return func(yield func(int, Attr) bool) {
		for i := range attrs.n {
			if !yield(i, attrs.a[i]) {
				return
			}
		}
	}
}

// ErrorType returns the error.type of err, a name from a finite set: canceled or timeout for an
// ended context, and _OTHER otherwise. Error messages never become attributes (ADR-0055 §16).
func ErrorType(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	return "_OTHER"
}

// TraceParent is a parsed W3C traceparent (Trace Context, §3.2) of version 00.
type TraceParent struct {
	// TraceID and SpanID are lower-case hex: 32 and 16 digits, not all zeros.
	TraceID string
	SpanID  string
	// Sampled is the sampled flag, a recommendation of the caller only (§3.2.2.5).
	Sampled bool
}

// String returns the traceparent header value of p.
func (p TraceParent) String() string {
	flags := "00"
	if p.Sampled {
		flags = "01"
	}
	return "00-" + p.TraceID + "-" + p.SpanID + "-" + flags
}

// ParseTraceParent parses a traceparent header value. It accepts version 00 and, as Trace
// Context §4.3 asks, a later version whose first fields parse like version 00. Any other value
// is invalid.
func ParseTraceParent(s string) (TraceParent, bool) {
	if len(s) < 55 || (len(s) > 55 && s[55] != '-') || s[2] != '-' || s[35] != '-' || s[52] != '-' {
		return TraceParent{}, false
	}
	version, traceID, spanID, flags := s[0:2], s[3:35], s[36:52], s[53:55]
	if !lowerHex(version) || version == "ff" || (version == "00" && len(s) != 55) {
		return TraceParent{}, false
	}
	if !lowerHex(traceID) || !lowerHex(spanID) || !lowerHex(flags) || allZeros(traceID) || allZeros(spanID) {
		return TraceParent{}, false
	}
	f, _ := strconv.ParseUint(flags, 16, 8)
	return TraceParent{TraceID: traceID, SpanID: spanID, Sampled: f&1 == 1}, true
}

func lowerHex(s string) bool {
	for _, c := range []byte(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func allZeros(s string) bool {
	for _, c := range []byte(s) {
		if c != '0' {
			return false
		}
	}
	return true
}

// NopTracer is a Tracer that records nothing, the tracer of traces_exporter = none. It keeps the
// remote parent of Continue for the log lines (ADR-0055 §18), so that the logs of a request carry
// the trace ID of the platform's trace (Cloud Run).
type NopTracer struct{}

var _ Tracer = NopTracer{}

type remoteParentKey struct{}

type nopSpan struct{}

func (nopSpan) Set(Attr)        {}
func (nopSpan) SetName(string)  {}
func (nopSpan) Recording() bool { return false }
func (nopSpan) End(string)      {}

// Start implements Tracer.
func (NopTracer) Start(ctx context.Context, _ SpanSpec, _ Attrs) (context.Context, Span) {
	return ctx, nopSpan{}
}

// Inject implements Tracer: nothing is recorded, so there is nothing to continue.
func (NopTracer) Inject(context.Context) string { return "" }

// Continue implements Tracer.
func (NopTracer) Continue(ctx context.Context, traceparent string) context.Context {
	return WithRemoteParent(ctx, traceparent)
}

// Linked implements Tracer.
func (NopTracer) Linked(ctx context.Context, _ string) context.Context {
	if _, ok := RemoteParent(ctx); !ok {
		return ctx
	}
	return context.WithValue(ctx, remoteParentKey{}, TraceParent{})
}

// LogIDs implements Tracer.
func (NopTracer) LogIDs(ctx context.Context) (string, string) {
	p, _ := RemoteParent(ctx)
	return p.TraceID, p.SpanID
}

// WithRemoteParent returns ctx carrying the parsed traceparent as its remote parent, or ctx
// when traceparent is invalid. It is the Continue of the adapters that keep no context of
// their own.
func WithRemoteParent(ctx context.Context, traceparent string) context.Context {
	p, ok := ParseTraceParent(traceparent)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, remoteParentKey{}, p)
}

// RemoteParent returns the remote parent that WithRemoteParent put in ctx.
func RemoteParent(ctx context.Context) (TraceParent, bool) {
	p, ok := ctx.Value(remoteParentKey{}).(TraceParent)
	return p, ok && p.TraceID != ""
}

// TracerOrNop returns t, or NopTracer when t is nil, for the optional Tracer of a dependency
// set.
func TracerOrNop(t Tracer) Tracer {
	if t == nil {
		return NopTracer{}
	}
	return t
}

// IsNopTracer reports whether t records nothing, so that a caller can skip the work of
// tracing.
func IsNopTracer(t Tracer) bool {
	if t == nil {
		return true
	}
	_, ok := t.(NopTracer)
	return ok
}
