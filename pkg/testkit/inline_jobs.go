// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// InlineJobs is a queue.JobQueue that runs every job it is given at once, once, through the
// handler registered in Registry, and keeps the results. With queue.AfterCommit it lets a
// contract run the jobs that a use case enqueues, such as its emails, right after the commit.
// It is safe for concurrent use.
type InlineJobs struct {
	Registry *queue.JobHandlerRegistry

	mu      sync.Mutex
	results []InlineResult
}

// InlineResult is a job that InlineJobs ran and the handler's result.
type InlineResult struct {
	Job    queue.Job
	Result queue.Result
}

var _ queue.JobQueue = (*InlineJobs)(nil)

// Enqueue implements queue.JobQueue: it runs the jobs in order and fails for a job type
// without a handler.
func (q *InlineJobs) Enqueue(ctx context.Context, jobs ...queue.NewJob) error {
	for i, nj := range jobs {
		h, _, ok := q.Registry.Lookup(nj.Type)
		if !ok {
			return fmt.Errorf("testkit: no handler of %s", nj.Type)
		}
		j := queue.Job{
			ID: fmt.Sprintf("inline-%s-%d", nj.IdempotencyKey, i), Type: nj.Type, Payload: nj.Payload,
			IdempotencyKey: nj.IdempotencyKey, Attempt: 1,
		}
		r := h(ctx, j)
		q.mu.Lock()
		q.results = append(q.results, InlineResult{Job: j, Result: r})
		q.mu.Unlock()
	}
	return nil
}

// Results returns the jobs run so far with their results, in order.
func (q *InlineJobs) Results() []InlineResult {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]InlineResult(nil), q.results...)
}

// inlineOutbox returns an Outbox on projectors that runs the jobs of a committed transaction at
// once through a new registry, which it returns for the handlers.
func inlineOutbox(projectors *uow.Projectors) (queue.Outbox, *InlineJobs) {
	jobs := &InlineJobs{Registry: &queue.JobHandlerRegistry{}}
	return queue.AfterCommit(projectors, jobs, nil), jobs
}
