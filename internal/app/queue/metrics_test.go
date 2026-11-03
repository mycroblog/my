// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// recordingMeter keeps the histogram values by metric and labels. (testkit.Meter does the same,
// but the application layer does not import pkg/testkit.)
type recordingMeter struct {
	kernel.NopMeter
	mu     sync.Mutex
	values map[string][]float64
}

type recordingHistogram struct {
	m *recordingMeter
	i kernel.Instrument
}

func (h recordingHistogram) Record(_ context.Context, v float64, l kernel.Labels) {
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	key := h.i.Name
	for k, val := range l.All() {
		if !h.i.Accepts(k) {
			panic("undeclared label " + k)
		}
		key += " " + k + "=" + val
	}
	h.m.values[key] = append(h.m.values[key], v)
}

func (m *recordingMeter) Histogram(i kernel.Instrument) kernel.Histogram {
	return recordingHistogram{m: m, i: i}
}

// jobsSource hands out its jobs one by one, due an hour before the clock.
type jobsSource struct {
	emptySource
	jobs []queue.Job
}

func (s *jobsSource) Claim(_ context.Context, _ string, now time.Time, _ time.Duration) (queue.Job, bool, error) {
	if len(s.jobs) == 0 {
		return queue.Job{}, false, nil
	}
	j := s.jobs[0]
	s.jobs = s.jobs[1:]
	j.Attempt, j.RunAt = 1, now.Add(-time.Hour)
	return j, true, nil
}

func TestRunnerRecordsJobMetrics(t *testing.T) {
	src := &jobsSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, jobs: []queue.Job{
		{ID: testID(1), Type: "ok"}, {ID: testID(2), Type: "flaky"}, {ID: testID(3), Type: "bad"}, {ID: testID(4), Type: "unknown.type"}, {ID: testID(5), Type: "later"},
	}}
	reg := &queue.JobHandlerRegistry{}
	reg.Register("ok", func(context.Context, queue.Job) queue.Result { return queue.Done() })
	reg.Register("flaky", func(context.Context, queue.Job) queue.Result { return queue.Retry(0, errors.New("later")) })
	reg.Register("bad", func(context.Context, queue.Job) queue.Result { return queue.Fail(errors.New("never")) })
	reg.Register("later", func(context.Context, queue.Job) queue.Result { return queue.Postpone(time.Second) })
	m := &recordingMeter{values: map[string][]float64{}}
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, Meter: m})
	if n, err := r.Drain(t.Context()); err != nil || n != 5 {
		t.Fatalf("Drain() = %d, %v", n, err)
	}
	for _, key := range []string{
		"my.job.duration my.job.type=later my.outcome=postponed",
		"my.job.duration my.job.type=ok my.outcome=success",
		"my.job.duration my.job.type=flaky my.outcome=retry",
		"my.job.duration my.job.type=bad my.outcome=dead",
		"my.job.duration my.job.type=other my.outcome=retry",
		"my.job.wait my.job.type=ok",
		"my.job.wait my.job.type=other",
	} {
		if len(m.values[key]) != 1 {
			t.Errorf("%s: %v, want one value", key, m.values[key])
		}
	}
	if got := m.values["my.job.wait my.job.type=ok"]; len(got) == 1 && got[0] != 3600 {
		t.Errorf("my.job.wait = %v, want 3600 seconds", got[0])
	}
}

// TestRunnerLogsJobRuns checks that each run writes one line with the labels of my.job.duration
// under the same names and values, and the duration and wait in seconds (ADR-0055 §5).
func TestRunnerLogsJobRuns(t *testing.T) {
	src := &jobsSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, jobs: []queue.Job{
		{ID: testID(1), Type: "ok"}, {ID: testID(2), Type: "flaky"}, {ID: testID(3), Type: "bad"}, {ID: testID(4), Type: "unknown.type"},
	}}
	reg := &queue.JobHandlerRegistry{}
	reg.Register("ok", func(context.Context, queue.Job) queue.Result { return queue.Done() })
	reg.Register("flaky", func(context.Context, queue.Job) queue.Result { return queue.Retry(0, errors.New("later")) })
	reg.Register("bad", func(context.Context, queue.Job) queue.Result { return queue.Fail(errors.New("never")) })
	var buf bytes.Buffer
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
	if n, err := r.Drain(t.Context()); err != nil || n != 4 {
		t.Fatalf("Drain() = %d, %v", n, err)
	}
	want := map[string]struct{ jobType, outcome, level, err string }{
		testID(1): {"ok", queue.OutcomeSuccess, "INFO", ""},
		testID(2): {"flaky", queue.OutcomeRetry, "WARN", "later"},
		testID(3): {"bad", queue.OutcomeDead, "ERROR", "never"},
		testID(4): {queue.JobTypeOther, queue.OutcomeRetry, "WARN", queue.ErrNoHandler.Error() + ": unknown.type"},
	}
	lines := 0
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		if m["msg"] != "job" {
			continue
		}
		lines++
		id, _ := m["job_id"].(string)
		w, ok := want[id]
		if !ok {
			t.Errorf("unexpected line %v", m)
			continue
		}
		if m[queue.LabelJobType] != w.jobType || m[queue.LabelOutcome] != w.outcome || m["level"] != w.level || m["attempt"] != 1.0 {
			t.Errorf("job %s: line %v, want my.job.type %s, my.outcome %s, level %s, attempt 1", id, m, w.jobType, w.outcome, w.level)
		}
		if _, ok := m["duration_seconds"].(float64); !ok {
			t.Errorf("job %s: duration_seconds = %v, want a number", id, m["duration_seconds"])
		}
		if m["wait_seconds"] != 3600.0 {
			t.Errorf("job %s: wait_seconds = %v, want 3600", id, m["wait_seconds"])
		}
		if got, _ := m["error"].(string); got != w.err {
			t.Errorf("job %s: error = %q, want %q", id, got, w.err)
		}
	}
	if lines != len(want) {
		t.Errorf("%d job lines, want %d:\n%s", lines, len(want), buf.String())
	}
}

// A job that found the database busy waits for it without using an attempt, however its handler
// reported the error.
func TestRunnerPostponesAJobThatFoundTheDatabaseBusy(t *testing.T) {
	busy := fmt.Errorf("read: %w", kernel.ErrDatabaseBusy)
	for name, result := range map[string]queue.Result{
		"retry":        queue.Retry(0, busy),
		"retry remote": queue.RetryRemote(0, busy),
		"fail":         queue.Fail(busy),
	} {
		t.Run(name, func(t *testing.T) {
			src := &jobsSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, jobs: []queue.Job{{ID: testID(1), Type: "busy"}}}
			reg := &queue.JobHandlerRegistry{}
			reg.Register("busy", func(context.Context, queue.Job) queue.Result { return result })
			m := &recordingMeter{values: map[string][]float64{}}
			r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, Meter: m})
			if n, err := r.Drain(t.Context()); err != nil || n != 1 {
				t.Fatalf("Drain() = %d, %v", n, err)
			}
			if key := "my.job.duration my.job.type=busy my.outcome=postponed"; len(m.values[key]) != 1 {
				t.Errorf("%s: %v, want one value", key, m.values[key])
			}
		})
	}
}
