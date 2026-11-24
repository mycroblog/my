// SPDX-License-Identifier: AGPL-3.0-only

package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// JobVerify is the type of the scheduled job that verifies every projection and corrects the
// ones that drifted, one projection per job.
const JobVerify = "projection.verify"

// DefaultVerifyInterval is how often JobVerify runs.
const DefaultVerifyInterval = 24 * time.Hour

// verifyMaxAttempts is how many times a JobVerify runs its projection before the run moves on
// to the next one and the job becomes dead.
const verifyMaxAttempts = 3

// enqueueTimeout bounds the enqueue of the next projection after the job's context ended.
const enqueueTimeout = 10 * time.Second

// verifyPayload is the payload of JobVerify: the run, the ID of the scheduled job that began it,
// and the projection this job verifies. The scheduled job has none and verifies the first.
type verifyPayload struct {
	Run        string `json:"run"`
	Projection string `json:"projection"`
}

// RegisterJobs registers the handler of JobVerify on reg and its schedule on sched.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.RegisterWithPolicy(JobVerify, s.handleVerify, queue.RetryPolicy{MaxAttempts: verifyMaxAttempts})
	sched.Add(queue.Schedule{Name: JobVerify, Every: DefaultVerifyInterval, Job: queue.NewJob{Type: JobVerify}})
}

// handleVerify rebuilds one projection, which writes only what drifted, and logs it when it did;
// then it queues the next projection of the run. Each projection so has the whole lease of its
// own job, and one that fails neither uses up the time of the others nor stops them: it is
// retried on its own, and the next is queued once it succeeds or runs out of attempts, so that a
// retry never runs the rest of the run again. Rebuilding a projection again changes nothing
// that is right.
func (s *Service) handleVerify(ctx context.Context, j queue.Job) queue.Result {
	pl := verifyPayload{Run: j.ID}
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("projection: verify payload %s: %w", j.Payload, err))
		}
	}
	if len(s.projections) == 0 {
		return queue.Done()
	}
	i := 0
	if pl.Projection != "" {
		i = slices.IndexFunc(s.projections, func(p Projection) bool { return p.Name == pl.Projection })
		if i < 0 {
			// A version that no longer has it took over the run; the next scheduled run verifies all.
			s.logger.WarnContext(ctx, "projection.verify names a projection this version does not have; the run ends",
				slog.String("projection", pl.Projection))
			return queue.Done()
		}
	}
	p := s.projections[i]
	r, err := s.Rebuild(ctx, p)
	if err == nil && r.Drifted() {
		s.logger.WarnContext(ctx, "projection drifted from its sources; corrected",
			slog.String("projection", p.Name), slog.Int64("deleted", r.Deleted), slog.Int64("inserted", r.Inserted))
	}
	if err != nil && j.Attempt < verifyMaxAttempts {
		return queue.Retry(0, err)
	}
	if next := i + 1; next < len(s.projections) && s.jobs != nil {
		if qerr := s.enqueueNext(ctx, pl.Run, s.projections[next].Name); qerr != nil {
			return queue.Retry(0, errors.Join(err, qerr))
		}
	}
	if err != nil {
		return queue.Fail(err)
	}
	return queue.Done()
}

// enqueueNext queues the job of the projection named name in run. It runs after the rebuild,
// which may have ended the job's context; the enqueue gets enqueueTimeout of its own.
func (s *Service) enqueueNext(ctx context.Context, run, name string) error {
	b, err := json.Marshal(verifyPayload{Run: run, Projection: name})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), enqueueTimeout)
	defer cancel()
	return s.jobs.Enqueue(ctx, queue.NewJob{Type: JobVerify, Payload: b, IdempotencyKey: JobVerify + ":" + run + ":" + name})
}
