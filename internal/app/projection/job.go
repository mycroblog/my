// SPDX-License-Identifier: AGPL-3.0-only

package projection

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// JobVerify is the type of the scheduled job that verifies every projection and corrects the
// ones that drifted.
const JobVerify = "projection.verify"

// DefaultVerifyInterval is how often JobVerify runs.
const DefaultVerifyInterval = 24 * time.Hour

// RegisterJobs registers the handler of JobVerify on reg and its schedule on sched.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobVerify, s.handleVerify)
	sched.Add(queue.Schedule{Name: JobVerify, Every: DefaultVerifyInterval, Job: queue.NewJob{Type: JobVerify}})
}

// handleVerify rebuilds every projection, which writes only what drifted, and logs the
// projections that did. A projection that fails does not stop the others; the job is retried
// when any failed, and rebuilding a projection again changes nothing that is right.
func (s *Service) handleVerify(ctx context.Context, _ queue.Job) queue.Result {
	var errs []error
	for _, p := range s.projections {
		r, err := s.Rebuild(ctx, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if r.Drifted() {
			s.logger.WarnContext(ctx, "projection drifted from its sources; corrected",
				slog.String("projection", p.Name), slog.Int64("deleted", r.Deleted), slog.Int64("inserted", r.Inserted))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}
