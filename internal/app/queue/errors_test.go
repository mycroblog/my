// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// reportSink keeps the reported events.
type reportSink struct {
	mu     sync.Mutex
	events []kernel.ErrorEvent
}

func (s *reportSink) Report(_ context.Context, ev kernel.ErrorEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *reportSink) Recover(ctx context.Context, where string) {
	kernel.RecoverPanic(ctx, s, recover(), where)
}

// attemptSource hands out one job at the given attempt.
type attemptSource struct {
	emptySource
	attempt int
	claimed bool
}

func (s *attemptSource) Claim(ctx context.Context, w string, now time.Time, lease time.Duration) (queue.Job, bool, error) {
	if s.claimed {
		return s.emptySource.Claim(ctx, w, now, lease)
	}
	s.claimed = true
	return queue.Job{ID: testID(7), Type: "test.job", Attempt: s.attempt, RunAt: now}, true, nil
}

// runOne runs one job of h at attempt with MaxAttempts 3 and returns the reports.
func runOne(t *testing.T, attempt int, h queue.Handler) []kernel.ErrorEvent {
	t.Helper()
	reg := &queue.JobHandlerRegistry{}
	reg.RegisterWithPolicy("test.job", h, queue.RetryPolicy{MaxAttempts: 3})
	sink := &reportSink{}
	r := queue.NewPullRunner(&attemptSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, attempt: attempt}, reg,
		queue.RunnerOptions{Clock: clock{}, Errors: sink})
	if _, err := r.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	return sink.events
}

func TestRunnerReportsPanicsAndDeadJobs(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name    string
		attempt int
		h       queue.Handler
		kind    kernel.ErrorKind // "" for no report
	}{
		{"success", 1, func(context.Context, queue.Job) queue.Result { return queue.Done() }, ""},
		{"retry", 1, func(context.Context, queue.Job) queue.Result { return queue.Retry(0, boom) }, ""},
		{"fail", 1, func(context.Context, queue.Job) queue.Result { return queue.Fail(boom) }, kernel.ErrorKindError},
		{"retries used up", 3, func(context.Context, queue.Job) queue.Result { return queue.Retry(0, boom) }, kernel.ErrorKindError},
		{"remote retry", 1, func(context.Context, queue.Job) queue.Result { return queue.RetryRemote(0, boom) }, ""},
		{"remote retries used up", 3, func(context.Context, queue.Job) queue.Result { return queue.RetryRemote(0, boom) }, ""},
		{"panic", 1, func(context.Context, queue.Job) queue.Result { panic("handler broke") }, kernel.ErrorKindPanic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evs := runOne(t, tc.attempt, tc.h)
			if tc.kind == "" {
				if len(evs) != 0 {
					t.Errorf("reports = %+v, want none", evs)
				}
				return
			}
			if len(evs) != 1 {
				t.Fatalf("reports = %+v, want 1", evs)
			}
			ev := evs[0]
			if ev.Kind != tc.kind || ev.Where != "test.job" || ev.JobID != testID(7) || ev.Attempt != tc.attempt || ev.Stack == "" {
				t.Errorf("report = %+v", ev)
			}
			if tc.kind == kernel.ErrorKindPanic && (ev.Message != "handler broke" || !strings.Contains(ev.Stack, "panic(")) {
				t.Errorf("panic report = %+v", ev)
			}
		})
	}
}

func TestRetryRemote(t *testing.T) {
	r := queue.RetryRemote(time.Second, errors.New("inbox answered 503"))
	if !r.IsRetry() || !r.IsRemote() || r.After() != time.Second {
		t.Errorf("RetryRemote: IsRetry %v IsRemote %v After %v", r.IsRetry(), r.IsRemote(), r.After())
	}
	if queue.Retry(0, nil).IsRemote() || queue.Fail(nil).IsRemote() {
		t.Error("Retry or Fail is remote")
	}
}
