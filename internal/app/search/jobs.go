// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// The jobs of the search module (ADR-0149 §7). A job carries IDs only, never text, and reads the
// current state when it runs, so repeating or reordering them gives the same index.
const (
	// JobSyncStatus brings the document of one status in line with the database. Its payload
	// names the status.
	JobSyncStatus = "search.sync_status"
	// JobSyncAuthor brings the documents of one author in line with the database. Its payload
	// names the account, and the part of a walk over the author's statuses.
	JobSyncAuthor = "search.sync_author"
	// JobSyncDomain enqueues JobSyncAuthor for each author of a domain and its subdomains. Its
	// payload names the domain, and the part of a walk over the accounts.
	JobSyncDomain = "search.sync_domain"
	// JobNarrow removes the documents of a visibility that search_visibilities no longer
	// includes. Its payload names the visibility.
	JobNarrow = "search.narrow"
	// JobRebuild refills the index from the database in batches. Its payload is a cursor.
	JobRebuild = "search.rebuild"
)

const (
	// authorJobSize is how many statuses one JobSyncAuthor covers: each takes a few reads and up to
	// indexCallTimeout of the backend, and the job has to end within its lease.
	authorJobSize = 200
	// authorBatch is how many statuses are read in one page of that walk.
	authorBatch = 50
)

type syncStatusPayload struct {
	StatusID string `json:"status_id"`
}

type syncAuthorPayload struct {
	queue.FanOutStep
	AccountID string `json:"account_id"`
}

type syncDomainPayload struct {
	queue.FanOutStep
	Domain string `json:"domain"`
}

type narrowPayload struct {
	Visibility Visibility `json:"visibility"`
}

// RegisterJobs registers the handlers of the jobs, with or without a backend: a type with no
// handler ends in a dead job, and worker_job_types refuses to start when a type it names does not
// exist (ADR-0149 §7). Without a backend they succeed doing nothing: only a job enqueued before the
// server moved to search = none can run. JobRebuild has no sender yet and does nothing either.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry) {
	reg.Register(JobSyncStatus, s.withBackend(s.handleSyncStatus))
	reg.Register(JobSyncAuthor, s.withBackend(s.handleSyncAuthor))
	reg.Register(JobSyncDomain, s.withBackend(s.handleSyncDomain))
	reg.Register(JobNarrow, s.withBackend(s.handleNarrow))
	reg.Register(JobRebuild, func(context.Context, queue.Job) queue.Result { return queue.Done() })
}

func (s *Service) withBackend(h func(context.Context, queue.Job) queue.Result) func(context.Context, queue.Job) queue.Result {
	return func(ctx context.Context, j queue.Job) queue.Result {
		if !s.Enabled() {
			return queue.Done()
		}
		return h(ctx, j)
	}
}

func outcome(err error) queue.Result {
	if err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

func (s *Service) handleSyncStatus(ctx context.Context, j queue.Job) queue.Result {
	var p syncStatusPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.StatusID == "" {
		return queue.Fail(fmt.Errorf("search: payload of %s: %q", JobSyncStatus, j.Payload))
	}
	return outcome(s.syncStatus(ctx, p.StatusID))
}

// handleSyncAuthor removes every document of an author that cannot have any, and otherwise walks the
// author's statuses. A sanction that lands during the walk has its own job, and each status of the
// walk is read again after it is written, so the walk leaves nothing behind that job removed.
func (s *Service) handleSyncAuthor(ctx context.Context, j queue.Job) queue.Result {
	var p syncAuthorPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.AccountID == "" {
		return queue.Fail(fmt.Errorf("search: payload of %s: %q", JobSyncAuthor, j.Payload))
	}
	indexable, author, err := s.authorState(ctx, p.AccountID)
	if err != nil {
		return queue.Retry(0, err)
	}
	if !authorQualifies(indexable, author) {
		return outcome(s.call(ctx, func(ctx context.Context) error { return s.d.Index.RemoveByAuthor(ctx, p.AccountID) }))
	}
	fan := queue.FanOut{
		Page: queue.PageByLastKey(func(ctx context.Context, after string, limit int) ([]string, error) {
			return s.d.Statuses.AuthorSearchableStatusIDs(ctx, p.AccountID, after, limit)
		}),
		Size:  authorJobSize,
		Batch: authorBatch,
		Handle: func(ctx context.Context, ids []string) error {
			for _, id := range ids {
				if err := s.syncStatus(ctx, id); err != nil {
					return err
				}
			}
			return nil
		},
		Enqueue: func(ctx context.Context, steps []queue.FanOutStep) error {
			return s.enqueueSteps(ctx, JobSyncAuthor+":"+j.ID, JobSyncAuthor, steps, func(step queue.FanOutStep) any {
				return syncAuthorPayload{FanOutStep: step, AccountID: p.AccountID}
			})
		},
	}
	return outcome(fan.Run(ctx, p.FanOutStep))
}

// handleSyncDomain enqueues JobSyncAuthor for each account of the domain and its subdomains, since a
// restriction of a domain reaches them (data-model §4.3).
func (s *Service) handleSyncDomain(ctx context.Context, j queue.Job) queue.Result {
	var p syncDomainPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.Domain == "" {
		return queue.Fail(fmt.Errorf("search: payload of %s: %q", JobSyncDomain, j.Payload))
	}
	fan := queue.FanOut{
		Page: queue.PageByLastKey(func(ctx context.Context, after string, limit int) ([]string, error) {
			return s.d.Accounts.AccountIDsOnDomain(ctx, p.Domain, after, limit)
		}),
		Handle: func(ctx context.Context, ids []string) error {
			jobs := make([]queue.NewJob, 0, len(ids))
			for _, id := range ids {
				job, err := syncAuthorJob(id)
				if err != nil {
					return err
				}
				job.IdempotencyKey = JobSyncAuthor + ":" + j.ID + ":" + id
				jobs = append(jobs, job)
			}
			return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, jobs...) })
		},
		Enqueue: func(ctx context.Context, steps []queue.FanOutStep) error {
			return s.enqueueSteps(ctx, JobSyncDomain+":"+j.ID, JobSyncDomain, steps, func(step queue.FanOutStep) any {
				return syncDomainPayload{FanOutStep: step, Domain: p.Domain}
			})
		},
	}
	return outcome(fan.Run(ctx, p.FanOutStep))
}

// handleNarrow removes the documents of a visibility, unless the setting covers it again by now. A
// status that qualified when a job read it and is written after this removal is read again after the
// write, and removed then.
func (s *Service) handleNarrow(ctx context.Context, j queue.Job) queue.Result {
	var p narrowPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || (p.Visibility != VisibilityPublic && p.Visibility != VisibilityLocal) {
		return queue.Fail(fmt.Errorf("search: payload of %s: %q", JobNarrow, j.Payload))
	}
	covered, err := s.Visibilities(ctx)
	if err != nil {
		return queue.Retry(0, err)
	}
	if slices.Contains(covered, p.Visibility) {
		return queue.Done()
	}
	return outcome(s.call(ctx, func(ctx context.Context) error { return s.d.Index.RemoveByVisibility(ctx, p.Visibility) }))
}

// enqueueSteps enqueues the jobs of the steps of a fan-out in one transaction.
func (s *Service) enqueueSteps(ctx context.Context, base, jobType string, steps []queue.FanOutStep, payload func(queue.FanOutStep) any) error {
	jobs := make([]queue.NewJob, 0, len(steps))
	for _, step := range steps {
		b, err := json.Marshal(payload(step))
		if err != nil {
			return err
		}
		jobs = append(jobs, queue.NewJob{Type: jobType, Payload: b, IdempotencyKey: queue.StepKey(base, step)})
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, jobs...) })
}

func syncAuthorJob(accountID string) (queue.NewJob, error) {
	b, err := json.Marshal(syncAuthorPayload{AccountID: accountID})
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{Type: JobSyncAuthor, Payload: b}, nil
}

// EraseAccountData implements appaccount.DataEraser: the erasure of a deleted account enqueues
// JobSyncAuthor and never calls the engine, so that an outage of an optional service does not stop
// every erasure; the job's retries, and a dead job a rebuild clears, deal with the outage
// (ADR-0149 §11). The statuses the erasure removes emit nothing that syncs them.
func (s *Service) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, _ string, _ int) (appaccount.ErasureStep, error) {
	if !s.Enabled() {
		return appaccount.ErasureStep{Done: true}, nil
	}
	job, err := syncAuthorJob(t.AccountID)
	if err != nil {
		return appaccount.ErasureStep{}, err
	}
	job.IdempotencyKey = JobSyncAuthor + ":erasure:" + t.AccountID
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, job) }); err != nil {
		return appaccount.ErasureStep{}, err
	}
	return appaccount.ErasureStep{Done: true}, nil
}
