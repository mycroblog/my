// SPDX-License-Identifier: AGPL-3.0-only

package none_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/telemetry/none"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestMeterContract(t *testing.T) {
	testkit.MeterContract(t, func(*testing.T) kernel.Meter { return none.New() })
}

// BenchmarkRecord measures what a measurement costs with metrics_exporter = none: one call on
// a method that does nothing, without an allocation (ADR-0055 §1).
func BenchmarkRecord(b *testing.B) {
	m := none.New()
	h := m.Histogram(kernel.Instrument{Name: "http.server.request.duration", Unit: "s", LabelKeys: []string{"http.route", "my.surface"}})
	c := m.Counter(kernel.Instrument{Name: "my.job.runs", Unit: "{job}", LabelKeys: []string{"my.outcome"}})
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		ctx, end := m.Begin(ctx)
		h.Record(ctx, 0.01, kernel.NewLabels("http.route", "/api/v1/statuses", "my.surface", "client_api"))
		c.Add(ctx, 1, kernel.NewLabels("my.outcome", "success"))
		end()
	}
}

func TestTracerContract(t *testing.T) {
	testkit.TracerContract(t, func(*testing.T) kernel.Tracer { return none.NewTracer() })
}

// spanSpec is a span as the job runner declares it.
var spanSpec = kernel.SpanSpec{Name: "federation.deliver", Kind: kernel.SpanConsumer, AttrKeys: []string{"my.job.type", "my.job.attempt", "my.outcome"}}

// TestTracerAllocatesNothing checks that a span costs no allocation with traces_exporter = none
// (ADR-0055 §17), so that tracing adds next to nothing to an instance that does not export.
func TestTracerAllocatesNothing(t *testing.T) {
	tr := none.NewTracer()
	ctx := t.Context()
	allocs := testing.AllocsPerRun(100, func() {
		sctx, s := tr.Start(ctx, spanSpec, kernel.NewAttrs(kernel.StringAttr("my.job.type", "federation.deliver"), kernel.IntAttr("my.job.attempt", 1)))
		s.Set(kernel.StringAttr("my.outcome", "success"))
		_ = tr.Inject(sctx)
		_, _ = tr.LogIDs(sctx)
		s.End("")
	})
	if allocs != 0 {
		t.Errorf("allocations per span = %v, want 0", allocs)
	}
}

// BenchmarkSpan measures what a span costs with traces_exporter = none.
func BenchmarkSpan(b *testing.B) {
	tr := none.NewTracer()
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		sctx, s := tr.Start(ctx, spanSpec, kernel.NewAttrs(kernel.StringAttr("my.job.type", "federation.deliver"), kernel.IntAttr("my.job.attempt", 1)))
		s.Set(kernel.StringAttr("my.outcome", "success"))
		_ = tr.Inject(sctx)
		s.End("")
	}
}
