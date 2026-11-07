// SPDX-License-Identifier: AGPL-3.0-only

package testkit_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

func TestTracerContract(t *testing.T) {
	kerneltest.TracerContract(t, func(*testing.T) kernel.Tracer { return testkit.NewTracer() })
}

func TestTracerReportsUndeclaredAttributes(t *testing.T) {
	tr := testkit.NewTracer()
	spec := kernel.SpanSpec{Name: "my.test", AttrKeys: []string{"my.outcome"}}
	_, s := tr.Start(t.Context(), spec, kernel.NewAttrs(kernel.StringAttr("url.path", "/users/alice")))
	s.Set(kernel.StringAttr("my.outcome", "success"))
	s.End("")
	if tr.Err() == nil {
		t.Fatal("Err = nil, want the undeclared attribute url.path")
	}
}

func TestTracerLinksAndParents(t *testing.T) {
	tr := testkit.NewTracer()
	spec := kernel.SpanSpec{Name: "my.test"}
	ctx := tr.Continue(t.Context(), kerneltest.ContractTraceParent)
	ctx, req := tr.Start(ctx, spec.Named("POST /api/v1/statuses"), kernel.Attrs{})
	tp := tr.Inject(ctx)
	req.End("")
	jctx, job := tr.Start(tr.Linked(ctx, tp), spec.Named("federation.deliver"), kernel.Attrs{})
	_, q := tr.Start(jctx, spec.Named("InsertJob"), kernel.Attrs{})
	q.End("")
	job.End("retry")

	spans := tr.Spans()
	if len(spans) != 3 {
		t.Fatalf("spans = %d, want 3", len(spans))
	}
	if spans[0].ParentID != "00f067aa0ba902b7" || spans[0].TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("request span = %+v, want the child of the remote parent", spans[0])
	}
	if spans[1].ParentID != "" || spans[1].TraceID == spans[0].TraceID || len(spans[1].Links) != 1 || spans[1].Links[0] != tp {
		t.Errorf("job span = %+v, want a new root linked to %s", spans[1], tp)
	}
	if spans[2].TraceID != spans[1].TraceID || spans[2].ParentID != spans[1].SpanID {
		t.Errorf("query span = %+v, want the child of the job span", spans[2])
	}
	if spans[1].ErrorType != "retry" || !spans[2].Ended {
		t.Errorf("spans = %+v, want ended", spans)
	}
	if err := tr.Err(); err != nil {
		t.Error(err)
	}
}
