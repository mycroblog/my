// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

func fixed(v float64) func() float64 { return func() float64 { return v } }

func TestBackoffDoublesUpToMaxDelay(t *testing.T) {
	p := queue.RetryPolicy{BaseDelay: time.Minute, MaxDelay: 10 * time.Minute, Jitter: -1}
	for attempt, want := range map[int]time.Duration{
		0: time.Minute, 1: time.Minute, 2: 2 * time.Minute, 3: 4 * time.Minute, 4: 8 * time.Minute, 5: 10 * time.Minute, 500: 10 * time.Minute,
	} {
		if got := p.Backoff(attempt, fixed(0.9)); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestBackoffFollowsDelays(t *testing.T) {
	p := queue.RetryPolicy{Delays: []time.Duration{time.Minute, 5 * time.Minute, time.Hour}, BaseDelay: time.Second, Jitter: -1}
	for attempt, want := range map[int]time.Duration{
		0: time.Minute, 1: time.Minute, 2: 5 * time.Minute, 3: time.Hour, 4: time.Hour, 500: time.Hour,
	} {
		if got := p.Backoff(attempt, fixed(0.9)); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
	// Jitter applies to the scheduled delay too.
	if got := (queue.RetryPolicy{Delays: []time.Duration{100 * time.Second}, Jitter: 0.5}).Backoff(1, fixed(0.5)); got != 75*time.Second {
		t.Errorf("Backoff with jitter = %v, want 75s", got)
	}
	// Or keeps a policy's own delays and fills in missing ones.
	if got := (queue.RetryPolicy{}).Or(p); len(got.Delays) != 3 {
		t.Errorf("Or(delays) = %+v, want the fallback's delays", got)
	}
}

func TestBackoffJitter(t *testing.T) {
	p := queue.RetryPolicy{BaseDelay: 100 * time.Second, MaxDelay: time.Hour, Jitter: 0.5}
	for r, want := range map[float64]time.Duration{0: 100 * time.Second, 0.5: 75 * time.Second, 0.999: 50050 * time.Millisecond} {
		if got := p.Backoff(1, fixed(r)); got != want {
			t.Errorf("Backoff with random %v = %v, want %v", r, got, want)
		}
	}
	// The zero Jitter takes the default; Jitter above 1 counts as 1.
	if got := (queue.RetryPolicy{BaseDelay: 100 * time.Second}).Backoff(1, fixed(0.5)); got != 90*time.Second {
		t.Errorf("default jitter: Backoff = %v, want 90s", got)
	}
	if got := (queue.RetryPolicy{BaseDelay: 100 * time.Second, Jitter: 7}).Backoff(1, fixed(0.5)); got != 50*time.Second {
		t.Errorf("jitter 7: Backoff = %v, want 50s", got)
	}
}

func TestRetryPolicyOr(t *testing.T) {
	got := queue.RetryPolicy{MaxAttempts: 2}.Or(queue.RetryPolicy{MaxAttempts: 9, BaseDelay: time.Second, MaxDelay: time.Hour, Jitter: 0.1})
	want := queue.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: time.Hour, Jitter: 0.1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Or = %+v, want %+v", got, want)
	}
}

func TestNextVisible(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		runAt time.Time
		limit time.Duration
		want  time.Time
	}{
		{now.Add(time.Hour), 0, now.Add(time.Hour)},
		{now.Add(time.Hour), 15 * time.Minute, now.Add(15 * time.Minute)},
		{now.Add(10 * time.Minute), 15 * time.Minute, now.Add(10 * time.Minute)},
		{now.Add(-time.Minute), 15 * time.Minute, now.Add(-time.Minute)},
	} {
		if got := queue.NextVisible(now, tc.runAt, tc.limit); !got.Equal(tc.want) {
			t.Errorf("NextVisible(now, %v, %v) = %v, want %v", tc.runAt, tc.limit, got, tc.want)
		}
	}
}

func TestBuild(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.FixedZone("JST", 9*3600))
	j, err := queue.NewJob{Type: "t"}.Build("id", now)
	if err != nil {
		t.Fatal(err)
	}
	wantNow := now.UTC().Truncate(time.Millisecond)
	if string(j.Payload) != "null" || !j.RunAt.Equal(wantNow) || j.RunAt.Location() != time.UTC || j.CreatedAt != j.RunAt || j.ID != "id" {
		t.Errorf("Build = %+v, want null payload and RunAt = CreatedAt = %v", j, wantNow)
	}
	for _, bad := range []queue.NewJob{{}, {Type: "t", Payload: json.RawMessage("{")}} {
		if _, err := bad.Build("id", now); err == nil {
			t.Errorf("Build(%+v) succeeded, want an error", bad)
		}
	}
}

func TestResults(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		r                   queue.Result
		done, retry, failed bool
	}{
		{queue.Done(), true, false, false},
		{queue.Retry(time.Second, boom), false, true, false},
		{queue.Fail(boom), false, false, true},
		{queue.Result{}, false, true, false},
	} {
		if tc.r.IsDone() != tc.done || tc.r.IsRetry() != tc.retry || tc.r.IsFail() != tc.failed {
			t.Errorf("%+v: IsDone/IsRetry/IsFail = %v/%v/%v", tc.r, tc.r.IsDone(), tc.r.IsRetry(), tc.r.IsFail())
		}
	}
	if r := queue.Retry(time.Second, boom); r.After() != time.Second || !errors.Is(r.Err(), boom) {
		t.Errorf("Retry: After = %v, Err = %v", r.After(), r.Err())
	}
}

func TestRegistry(t *testing.T) {
	var r queue.JobHandlerRegistry
	if _, _, ok := r.Lookup("a"); ok {
		t.Fatal("empty registry found a handler")
	}
	policy := queue.RetryPolicy{MaxAttempts: 2}
	r.RegisterWithPolicy("a", func(context.Context, queue.Job) queue.Result { return queue.Done() }, policy)
	if h, p, ok := r.Lookup("a"); !ok || h == nil || !reflect.DeepEqual(p, policy) {
		t.Fatalf("Lookup = %v, %+v, %v", h != nil, p, ok)
	}
	defer func() {
		if recover() == nil {
			t.Error("registering a type twice did not panic")
		}
	}()
	r.Register("a", func(context.Context, queue.Job) queue.Result { return queue.Done() })
}

type emptySource struct{ claims chan struct{} }

func (s emptySource) Claim(context.Context, string, time.Time, time.Duration) (queue.Job, bool, error) {
	select {
	case s.claims <- struct{}{}:
	default:
	}
	return queue.Job{}, false, nil
}
func (emptySource) Complete(context.Context, string, queue.Job) error { return nil }
func (emptySource) Release(context.Context, string, queue.Job, int, time.Time, string) error {
	return nil
}
func (emptySource) Bury(context.Context, string, queue.Job, string, time.Time) error { return nil }
func (emptySource) ClaimJob(context.Context, string, string, time.Time, time.Duration) (queue.ClaimResult, error) {
	return queue.ClaimResult{}, nil
}

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func TestRunStopsWhenContextEnds(t *testing.T) {
	src := emptySource{claims: make(chan struct{}, 1)}
	r := queue.NewPullRunner(src, nil, queue.RunnerOptions{Clock: clock{}, PollInterval: time.Hour})
	if r.Worker() == "" {
		t.Error("default worker name is empty")
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error)
	go func() { done <- r.Run(ctx) }()
	<-src.claims // Run polled once and now waits for the next poll
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v, want nil after the context ends", err)
	}
}

// oneJobSource hands out one job and records whether it was completed.
type oneJobSource struct {
	emptySource
	claimed   bool
	completed chan struct{}
}

func (s *oneJobSource) Claim(ctx context.Context, w string, now time.Time, lease time.Duration) (queue.Job, bool, error) {
	if s.claimed {
		return s.emptySource.Claim(ctx, w, now, lease)
	}
	s.claimed = true
	return queue.Job{ID: testID(1), Type: "slow", Attempt: 1, RunAt: now}, true, nil
}

func (s *oneJobSource) Complete(context.Context, string, queue.Job) error {
	close(s.completed)
	return nil
}

func TestRunUntilFinishesTheJobInProgress(t *testing.T) {
	src := &oneJobSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, completed: make(chan struct{})}
	started, release := make(chan struct{}), make(chan struct{})
	reg := &queue.JobHandlerRegistry{}
	reg.Register("slow", func(ctx context.Context, _ queue.Job) queue.Result {
		close(started)
		<-release
		if ctx.Err() != nil {
			return queue.Retry(0, ctx.Err())
		}
		return queue.Done()
	})
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, PollInterval: time.Hour})
	stop := make(chan struct{})
	done := make(chan error)
	go func() { done <- r.RunUntil(t.Context(), stop) }()
	<-started
	close(stop)
	select {
	case <-done:
		t.Fatal("RunUntil returned while a job was in progress")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Errorf("RunUntil = %v, want nil", err)
	}
	select {
	case <-src.completed:
	default:
		t.Error("the job in progress was not completed")
	}
	select {
	case <-src.claims:
		t.Error("RunUntil claimed again after stop was closed")
	default:
	}
}
