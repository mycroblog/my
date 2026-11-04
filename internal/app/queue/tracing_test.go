// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// recordingTracer keeps the spans that the runner starts. (testkit.Tracer does the same, but
// the application layer does not import pkg/testkit.)
type recordingTracer struct {
	kernel.NopTracer
	mu    sync.Mutex
	spans []*recordedSpan
}

type recordedSpan struct {
	name      string
	link      string
	attrs     map[string]string
	errorType string
	ended     bool
}

type linkKey struct{}

func (t *recordingTracer) Linked(ctx context.Context, traceparent string) context.Context {
	return context.WithValue(ctx, linkKey{}, traceparent)
}

func (t *recordingTracer) Start(ctx context.Context, spec kernel.SpanSpec, attrs kernel.Attrs) (context.Context, kernel.Span) {
	if err := spec.Check(attrs); err != nil {
		panic(err)
	}
	link, _ := ctx.Value(linkKey{}).(string)
	s := &recordedSpan{name: spec.Name, link: link, attrs: map[string]string{}}
	for _, a := range attrs.All() {
		s.attrs[a.Key] = a.Str()
	}
	t.mu.Lock()
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return ctx, recordingSpan{s: s, spec: spec}
}

type recordingSpan struct {
	s    *recordedSpan
	spec kernel.SpanSpec
}

func (r recordingSpan) Set(a kernel.Attr) {
	if err := r.spec.CheckAttr(a); err != nil {
		panic(err)
	}
	r.s.attrs[a.Key] = a.Str()
}
func (r recordingSpan) SetName(name string)  { r.s.name = name }
func (recordingSpan) Recording() bool        { return true }
func (r recordingSpan) End(errorType string) { r.s.ended, r.s.errorType = true, errorType }

// TestRunnerTracesJobRuns checks that each run is a span named after the job's type, linked to
// the job's TraceParent, with its attempt and outcome (ADR-0055 §14).
func TestRunnerTracesJobRuns(t *testing.T) {
	const tp = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	src := &jobsSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, jobs: []queue.Job{
		{ID: testID(1), Type: "ok", TraceParent: tp}, {ID: testID(2), Type: "flaky"}, {ID: testID(3), Type: "unknown.type"},
	}}
	reg := &queue.JobHandlerRegistry{}
	reg.Register("ok", func(context.Context, queue.Job) queue.Result { return queue.Done() })
	reg.Register("flaky", func(context.Context, queue.Job) queue.Result {
		return queue.Retry(0, context.DeadlineExceeded)
	})
	tr := &recordingTracer{}
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, Tracer: tr})
	if n, err := r.Drain(t.Context()); err != nil || n != 3 {
		t.Fatalf("Drain() = %d, %v", n, err)
	}
	want := []recordedSpan{
		{name: "ok", link: tp, errorType: "", attrs: map[string]string{"my.job.type": "ok", "my.job.attempt": "1", "my.outcome": "success"}},
		{name: "flaky", errorType: "timeout", attrs: map[string]string{"my.job.type": "flaky", "my.job.attempt": "1", "my.outcome": "retry"}},
		{name: queue.JobTypeOther, errorType: "_OTHER", attrs: map[string]string{"my.job.type": "other", "my.job.attempt": "1", "my.outcome": "retry"}},
	}
	if len(tr.spans) != len(want) {
		t.Fatalf("spans = %d, want %d", len(tr.spans), len(want))
	}
	for i, w := range want {
		got := tr.spans[i]
		if got.name != w.name || got.link != w.link || got.errorType != w.errorType || !got.ended || len(got.attrs) != len(w.attrs) {
			t.Errorf("span %d = %+v, want %+v", i, *got, w)
			continue
		}
		for k, v := range w.attrs {
			if got.attrs[k] != v {
				t.Errorf("span %d attribute %s = %q, want %q", i, k, got.attrs[k], v)
			}
		}
	}
}

func TestTraceJobsFillsOnlyEmptyTraceParents(t *testing.T) {
	jobs := []queue.Job{{Type: "a"}, {Type: "b", TraceParent: "kept"}}
	queue.TraceJobs(t.Context(), injecting{}, jobs)
	if jobs[0].TraceParent != "injected" || jobs[1].TraceParent != "kept" {
		t.Errorf("jobs = %+v", jobs)
	}
	queue.TraceJobs(t.Context(), nil, jobs) // a nil tracer is fine
}

type injecting struct{ kernel.NopTracer }

func (injecting) Inject(context.Context) string { return "injected" }
