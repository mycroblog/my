// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Tracer is a kernel.Tracer that records every span, sampled, for tests of the code that
// traces. An attribute whose key the span's spec did not declare, more attributes than
// kernel.MaxSpanAttrs, or a span ended twice, is a programming error that Err reports
// (ADR-0055 §13, §16). It is safe for concurrent use.
type Tracer struct {
	mu    sync.Mutex
	next  uint64
	spans []*RecordedSpan
	errs  []error
}

var _ kernel.Tracer = (*Tracer)(nil)

// RecordedSpan is one span that a Tracer started.
type RecordedSpan struct {
	Name string
	Kind kernel.SpanKind
	// Attrs are the attributes, integers in decimal.
	Attrs map[string]string
	// TraceID and SpanID identify the span; ParentID is the span ID of its parent, local or
	// remote, and empty for a root.
	TraceID, SpanID, ParentID string
	// Links are the traceparents that the span links to (Tracer.Linked).
	Links []string
	// Ended reports whether End was called, with ErrorType.
	Ended     bool
	ErrorType string

	spec kernel.SpanSpec
}

// NewTracer returns an empty Tracer.
func NewTracer() *Tracer { return &Tracer{} }

// Spans returns a copy of every span started so far, in start order.
func (t *Tracer) Spans() []RecordedSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]RecordedSpan, len(t.spans))
	for i, s := range t.spans {
		out[i] = *s
		out[i].Links = slices.Clone(s.Links)
		out[i].Attrs = make(map[string]string, len(s.Attrs))
		for k, v := range s.Attrs {
			out[i].Attrs[k] = v
		}
	}
	return out
}

// Named returns the spans named name.
func (t *Tracer) Named(name string) []RecordedSpan {
	var out []RecordedSpan
	for _, s := range t.Spans() {
		if s.Name == name {
			out = append(out, s)
		}
	}
	return out
}

// Err returns the programming errors seen so far, joined, or nil.
func (t *Tracer) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return errors.Join(t.errs...)
}

func (t *Tracer) fail(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errs = append(t.errs, err)
}

func (t *Tracer) id(n int) string {
	t.mu.Lock()
	t.next++
	v := t.next
	t.mu.Unlock()
	return fmt.Sprintf("%0*x", n, v)
}

type fakeSpanKey struct{}

type fakeLinkKey struct{}

// fakeLink marks a context of Linked until Start consumes it.
type fakeLink struct {
	traceparent string
	active      bool
}

// Start implements kernel.Tracer.
func (t *Tracer) Start(ctx context.Context, spec kernel.SpanSpec, attrs kernel.Attrs) (context.Context, kernel.Span) {
	if len(spec.AttrKeys) > kernel.MaxSpanAttrs {
		t.fail(fmt.Errorf("span %s declares %d attributes, more than %d", spec.Name, len(spec.AttrKeys), kernel.MaxSpanAttrs))
	}
	s := &RecordedSpan{Name: spec.Name, Kind: spec.Kind, Attrs: map[string]string{}, SpanID: t.id(16), spec: spec}
	if l, ok := ctx.Value(fakeLinkKey{}).(fakeLink); ok && l.active {
		if _, valid := kernel.ParseTraceParent(l.traceparent); valid {
			s.Links = []string{l.traceparent}
		}
		ctx = context.WithValue(ctx, fakeLinkKey{}, fakeLink{})
	} else if parent, ok := ctx.Value(fakeSpanKey{}).(*RecordedSpan); ok {
		s.TraceID, s.ParentID = parent.TraceID, parent.SpanID
	} else if p, ok := kernel.RemoteParent(ctx); ok {
		s.TraceID, s.ParentID = p.TraceID, p.SpanID
	}
	if s.TraceID == "" {
		s.TraceID = t.id(32)
	}
	for _, a := range attrs.All() {
		if err := spec.CheckAttr(a); err != nil {
			t.fail(err)
		}
		s.Attrs[a.Key] = a.Str()
	}
	t.mu.Lock()
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return context.WithValue(ctx, fakeSpanKey{}, s), fakeSpan{t: t, s: s}
}

// Inject implements kernel.Tracer.
func (t *Tracer) Inject(ctx context.Context) string {
	s, ok := ctx.Value(fakeSpanKey{}).(*RecordedSpan)
	if !ok {
		return ""
	}
	return kernel.TraceParent{TraceID: s.TraceID, SpanID: s.SpanID, Sampled: true}.String()
}

// Continue implements kernel.Tracer.
func (t *Tracer) Continue(ctx context.Context, traceparent string) context.Context {
	if _, ok := kernel.ParseTraceParent(traceparent); !ok {
		return ctx
	}
	return context.WithValue(kernel.WithRemoteParent(ctx, traceparent), fakeSpanKey{}, nil)
}

// Linked implements kernel.Tracer.
func (t *Tracer) Linked(ctx context.Context, traceparent string) context.Context {
	return context.WithValue(ctx, fakeLinkKey{}, fakeLink{traceparent: traceparent, active: true})
}

// LogIDs implements kernel.Tracer.
func (t *Tracer) LogIDs(ctx context.Context) (string, string) {
	if l, ok := ctx.Value(fakeLinkKey{}).(fakeLink); ok && l.active {
		return "", ""
	}
	if s, ok := ctx.Value(fakeSpanKey{}).(*RecordedSpan); ok {
		return s.TraceID, s.SpanID
	}
	p, _ := kernel.RemoteParent(ctx)
	return p.TraceID, p.SpanID
}

type fakeSpan struct {
	t *Tracer
	s *RecordedSpan
}

func (f fakeSpan) Set(a kernel.Attr) {
	if err := f.s.spec.CheckAttr(a); err != nil {
		f.t.fail(err)
	}
	f.t.mu.Lock()
	defer f.t.mu.Unlock()
	f.s.Attrs[a.Key] = a.Str()
}

func (f fakeSpan) SetName(name string) {
	f.t.mu.Lock()
	defer f.t.mu.Unlock()
	f.s.Name = name
}

func (fakeSpan) Recording() bool { return true }

func (f fakeSpan) End(errorType string) {
	f.t.mu.Lock()
	defer f.t.mu.Unlock()
	if f.s.Ended {
		f.t.errs = append(f.t.errs, fmt.Errorf("span %s ended twice", f.s.Name))
		return
	}
	f.s.Ended, f.s.ErrorType = true, errorType
	if len(f.s.Attrs) > kernel.MaxSpanAttrs {
		f.t.errs = append(f.t.errs, fmt.Errorf("span %s has %d attributes, more than %d", f.s.Name, len(f.s.Attrs), kernel.MaxSpanAttrs))
	}
}
