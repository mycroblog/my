// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The outcomes of a job run, the my.outcome label of my.job.duration (ADR-0055 §3).
const (
	OutcomeSuccess = "success"
	OutcomeRetry   = "retry"
	// OutcomePostponed is a run that gave its attempt back (Postpone).
	OutcomePostponed = "postponed"
	OutcomeDead      = "dead"
)

// Label keys of the job metrics.
const (
	LabelJobType = "my.job.type"
	LabelOutcome = "my.outcome"
)

// AttrJobAttempt is the span attribute of the number of a run, Job.Attempt (ADR-0063 §2).
const AttrJobAttempt = "my.job.attempt"

// JobSpan is the span of one run of a job (ADR-0063 §1, §2), named after the job's type: the
// root of a new trace, linked to the span that enqueued the job.
var JobSpan = kernel.SpanSpec{
	Name: "my.job", Kind: kernel.SpanConsumer, AttrKeys: []string{LabelJobType, AttrJobAttempt, LabelOutcome},
}

// JobTypeOther is the my.job.type of a job whose type has no handler, which is not one of the
// finite set of registered types.
const JobTypeOther = "other"

// The job metrics of ADR-0055 §3.
var (
	JobDuration = kernel.Instrument{
		Name: "my.job.duration", Unit: "s", Description: "Duration of the job runs, by the outcome of the run.",
		LabelKeys: []string{LabelJobType, LabelOutcome}, Buckets: kernel.DurationBuckets,
	}
	JobWait = kernel.Instrument{
		Name: "my.job.wait", Unit: "s", Description: "Time from when a job became due to when a worker started it.",
		LabelKeys: []string{LabelJobType}, Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
	}
)

// jobMetrics records the job metrics of a runner.
type jobMetrics struct {
	meter    kernel.Meter
	duration kernel.Histogram
	wait     kernel.Histogram
}

func newJobMetrics(m kernel.Meter) jobMetrics {
	m = kernel.MeterOrNop(m)
	return jobMetrics{meter: m, duration: m.Histogram(JobDuration), wait: m.Histogram(JobWait)}
}

func (m jobMetrics) waited(ctx context.Context, jobType string, d time.Duration) {
	m.wait.Record(ctx, max(d, 0).Seconds(), kernel.NewLabels(LabelJobType, jobType))
}

func (m jobMetrics) ran(ctx context.Context, jobType, outcome string, d time.Duration) {
	m.duration.Record(ctx, d.Seconds(), kernel.NewLabels(LabelJobType, jobType, LabelOutcome, outcome))
}
