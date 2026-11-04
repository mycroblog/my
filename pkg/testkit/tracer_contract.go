// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

// ContractTraceParent is a valid, sampled traceparent from outside the process, as a trusted
// proxy sends it.
const ContractTraceParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

// TracerContract verifies a kernel.Tracer adapter (ADR-0055): it starts and ends spans of every
// kind from many goroutines, with attributes, renames, and failures, and survives an attribute
// that the spec did not declare, all without a panic or a data race; Inject returns a valid
// traceparent of the current span exactly when the span is recorded, and nothing outside a
// span; Continue takes a valid traceparent as the remote parent of the next span and of the log
// IDs, and ignores an invalid one; Linked starts a new trace, whose children stay in it.
// Which spans the adapter exports, and how, is checked by its own tests.
func TracerContract(t *testing.T, newTracer func(t *testing.T) kernel.Tracer) {
	t.Helper()
	spec := kernel.SpanSpec{Name: "my.contract", Kind: kernel.SpanInternal, AttrKeys: []string{"my.outcome", "my.job.attempt"}}
	t.Run("StartsAndEndsSpans", func(t *testing.T) {
		tr := newTracer(t)
		kinds := []kernel.SpanKind{kernel.SpanInternal, kernel.SpanServer, kernel.SpanClient, kernel.SpanProducer, kernel.SpanConsumer}
		var wg sync.WaitGroup
		for n := range 8 {
			wg.Go(func() {
				for i := range 20 {
					s := spec
					s.Kind = kinds[(n+i)%len(kinds)]
					ctx, parent := tr.Start(t.Context(), s, kernel.NewAttrs(kernel.StringAttr("my.outcome", "success")))
					_, child := tr.Start(ctx, s.Named("my.contract.child"), kernel.Attrs{})
					child.Set(kernel.IntAttr("my.job.attempt", int64(i)))
					child.End("")
					parent.SetName("my.contract.renamed")
					parent.End("timeout")
				}
			})
		}
		wg.Wait()
	})
	t.Run("SurvivesUndeclaredAttributes", func(t *testing.T) {
		tr := newTracer(t)
		_, s := tr.Start(t.Context(), spec, kernel.NewAttrs(kernel.StringAttr("url.full", "https://example.com/users/alice")))
		s.Set(kernel.StringAttr("client.address", "192.0.2.1"))
		s.End("")
	})
	t.Run("InjectsOnlyRecordedSpans", func(t *testing.T) {
		tr := newTracer(t)
		if got := tr.Inject(t.Context()); got != "" {
			t.Errorf("Inject outside a span = %q, want empty", got)
		}
		ctx, s := tr.Start(t.Context(), spec, kernel.Attrs{})
		defer s.End("")
		got := tr.Inject(ctx)
		if !s.Recording() {
			if got != "" {
				t.Errorf("Inject of a span that is not recorded = %q, want empty", got)
			}
			return
		}
		p, ok := kernel.ParseTraceParent(got)
		if !ok {
			t.Fatalf("Inject = %q, not a traceparent", got)
		}
		if traceID, spanID := tr.LogIDs(ctx); traceID != p.TraceID || spanID != p.SpanID {
			t.Errorf("LogIDs = %s %s, want those of Inject %q", traceID, spanID, got)
		}
	})
	t.Run("ContinuesAValidTraceParent", func(t *testing.T) {
		tr := newTracer(t)
		want, _ := kernel.ParseTraceParent(ContractTraceParent)
		ctx := tr.Continue(t.Context(), ContractTraceParent)
		if traceID, spanID := tr.LogIDs(ctx); traceID != want.TraceID || spanID != want.SpanID {
			t.Errorf("LogIDs after Continue = %q %q, want %q %q", traceID, spanID, want.TraceID, want.SpanID)
		}
		ctx, s := tr.Start(ctx, spec, kernel.Attrs{})
		defer s.End("")
		if traceID, _ := tr.LogIDs(ctx); s.Recording() && traceID != want.TraceID {
			t.Errorf("trace ID of the child of a remote parent = %q, want %q", traceID, want.TraceID)
		}
	})
	t.Run("IgnoresInvalidTraceParents", func(t *testing.T) {
		tr := newTracer(t)
		for _, v := range []string{
			"", "garbage", "00-00000000000000000000000000000000-00f067aa0ba902b7-01",
			"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", "ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra",
		} {
			ctx := tr.Continue(t.Context(), v)
			if traceID, spanID := tr.LogIDs(ctx); traceID != "" || spanID != "" {
				t.Errorf("LogIDs after Continue(%q) = %q %q, want none", v, traceID, spanID)
			}
		}
	})
	t.Run("LinkedStartsANewTrace", func(t *testing.T) {
		tr := newTracer(t)
		parent, _ := kernel.ParseTraceParent(ContractTraceParent)
		ctx := tr.Continue(t.Context(), ContractTraceParent)
		ctx, outer := tr.Start(ctx, spec, kernel.Attrs{})
		defer outer.End("")
		ctx, s := tr.Start(tr.Linked(ctx, ContractTraceParent), spec.Named("my.contract.job"), kernel.Attrs{})
		defer s.End("")
		traceID, _ := tr.LogIDs(ctx)
		if s.Recording() && (traceID == "" || traceID == parent.TraceID) {
			t.Errorf("trace ID of a linked span = %q, want a new trace", traceID)
		}
		cctx, child := tr.Start(ctx, spec.Named("my.contract.query"), kernel.Attrs{})
		defer child.End("")
		if childTrace, _ := tr.LogIDs(cctx); child.Recording() && childTrace != traceID {
			t.Errorf("trace ID of the child of a linked span = %q, want %q", childTrace, traceID)
		}
	})
	t.Run("EndsAfterTheContextEnded", func(t *testing.T) {
		tr := newTracer(t)
		ctx, cancel := context.WithCancel(t.Context())
		ctx, s := tr.Start(ctx, spec, kernel.Attrs{})
		cancel()
		_, child := tr.Start(ctx, spec, kernel.Attrs{})
		child.End(kernel.ErrorType(ctx.Err()))
		s.End("")
	})
}
