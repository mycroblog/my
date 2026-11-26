// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

func TestParseTraceParent(t *testing.T) {
	for _, tc := range []struct {
		in      string
		ok      bool
		sampled bool
	}{
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", true, true},
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00", true, false},
		{"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-later", true, true},
		{"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-03", true, true},
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-later", false, false},
		{"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", false, false},
		{"00-00000000000000000000000000000000-00f067aa0ba902b7-01", false, false},
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", false, false},
		{"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01", false, false},
		{"00_4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", false, false},
		{"", false, false},
	} {
		p, ok := kernel.ParseTraceParent(tc.in)
		if ok != tc.ok || p.Sampled != tc.sampled {
			t.Errorf("ParseTraceParent(%q) = %+v, %v; want ok %v sampled %v", tc.in, p, ok, tc.ok, tc.sampled)
		}
		if ok && p.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("ParseTraceParent(%q).TraceID = %q", tc.in, p.TraceID)
		}
	}
	p, _ := kernel.ParseTraceParent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if got := p.String(); got != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Errorf("String = %q", got)
	}
}

func TestSpanSpecChecksAttributes(t *testing.T) {
	spec := kernel.SpanSpec{Name: "my.test", AttrKeys: []string{"my.outcome"}}
	if err := spec.Check(kernel.NewAttrs(kernel.StringAttr("my.outcome", "success"), kernel.StringAttr(kernel.AttrErrorType, "timeout"))); err != nil {
		t.Errorf("Check of declared attributes: %v", err)
	}
	if err := spec.Check(kernel.NewAttrs(kernel.StringAttr("url.full", "https://example.com/"))); err == nil {
		t.Error("Check of url.full = nil, want an error")
	}
	attrs := kernel.NewAttrs(kernel.IntAttr("http.response.status_code", 200)).With(kernel.IntAttr("http.response.status_code", 404))
	if attrs.Len() != 1 {
		t.Fatalf("Len = %d, want 1", attrs.Len())
	}
	for _, a := range attrs.All() {
		if !a.IsInt() || a.Int() != 404 || a.Str() != "404" {
			t.Errorf("attribute = %+v, want 404", a)
		}
	}
}

func TestErrorType(t *testing.T) {
	for err, want := range map[error]string{
		nil:              "",
		context.Canceled: "canceled",
		fmt.Errorf("x: %w", context.DeadlineExceeded): "timeout",
		errors.New("user alice"):                      "_OTHER",
	} {
		if got := kernel.ErrorType(err); got != want {
			t.Errorf("ErrorType(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestNopTracerKeepsTheRemoteParentForTheLogs(t *testing.T) {
	var tr kernel.NopTracer
	ctx := tr.Continue(t.Context(), "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if p := tr.LogIDs(ctx); p.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || p.SpanID != "00f067aa0ba902b7" || !p.Sampled {
		t.Errorf("LogIDs = %+v", p)
	}
	if p := tr.LogIDs(tr.Linked(ctx, "")); p.TraceID != "" {
		t.Errorf("LogIDs after Linked = %+v, want none", p)
	}
	if !kernel.IsNopTracer(nil) || !kernel.IsNopTracer(tr) || kernel.TracerOrNop(nil) == nil {
		t.Error("IsNopTracer / TracerOrNop")
	}
}
