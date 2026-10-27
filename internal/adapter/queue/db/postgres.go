// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package db

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/queue/db/internal/postgresqueue"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// NewPostgres returns the queue kept in the PostgreSQL database d.
func NewPostgres(d *postgres.DB, opts Options) *Queue {
	opts.check()
	return &Queue{db: d, st: postgresStore{d}, opts: opts}
}

// NewPostgresOutbox returns the outbox kept in the PostgreSQL database d.
func NewPostgresOutbox(d *postgres.DB, opts Options) *Outbox {
	opts.check()
	return &Outbox{db: d, st: postgresStore{d}, opts: opts}
}

type postgresStore struct{ db *postgres.DB }

var _ store = postgresStore{}

func (s postgresStore) queries(tx uow.Tx) (*postgresqueue.Queries, error) {
	pgTx, err := s.db.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return postgresqueue.New(pgTx), nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// int32Of converts attempt counts and limits, which stay far below the int32 range.
func int32Of(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < math.MinInt32:
		return math.MinInt32
	}
	return int32(n)
}

func (s postgresStore) insertJob(ctx context.Context, tx uow.Tx, j queue.Job) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.InsertJob(ctx, postgresqueue.InsertJobParams{
		ID:             j.ID,
		Type:           j.Type,
		Payload:        j.Payload,
		IdempotencyKey: optional(j.IdempotencyKey),
		OrderingKey:    optional(j.OrderingKey),
		TraceParent:    optional(j.TraceParent),
		RunAt:          j.RunAt,
		CreatedAt:      j.CreatedAt,
	})
}

func (s postgresStore) claim(ctx context.Context, tx uow.Tx, worker string, now, until time.Time) (queue.Job, bool, error) {
	q, err := s.queries(tx)
	if err != nil {
		return queue.Job{}, false, err
	}
	// A candidate fails the second check only when a claim of its ordering key committed in
	// between, which the next pick sees; the bound keeps a claim short under contention.
	for range maxClaimPicks {
		c, err := q.PickJob(ctx, now)
		if errors.Is(err, pgx.ErrNoRows) {
			return queue.Job{}, false, nil
		}
		if err != nil {
			return queue.Job{}, false, err
		}
		if c.OrderingKey != nil {
			if err := q.LockOrderingKey(ctx, orderingLockPrefix+*c.OrderingKey); err != nil {
				return queue.Job{}, false, err
			}
		}
		r, err := q.ClaimJob(ctx, postgresqueue.ClaimJobParams{
			Worker:      &worker,
			LockedUntil: pgtype.Timestamptz{Time: until, Valid: true},
			ID:          c.ID,
			Now:         now,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return queue.Job{}, false, err
		}
		return claimedJob(r), true, nil
	}
	return queue.Job{}, false, nil
}

const (
	// maxClaimPicks bounds the candidates one claim tries.
	maxClaimPicks = 4
	// orderingLockPrefix namespaces the advisory locks of ordering keys.
	orderingLockPrefix = "queue:ordering:"
)

func claimedJob(r postgresqueue.ClaimJobRow) queue.Job {
	return queue.Job{
		ID:             r.ID,
		Type:           r.Type,
		Payload:        json.RawMessage(r.Payload),
		IdempotencyKey: deref(r.IdempotencyKey),
		OrderingKey:    deref(r.OrderingKey),
		TraceParent:    deref(r.TraceParent),
		Attempt:        int(r.Attempt),
		RunAt:          kernel.Normalize(r.RunAt),
		LastError:      deref(r.LastError),
		CreatedAt:      kernel.Normalize(r.CreatedAt),
	}
}

func (s postgresStore) deleteClaimed(ctx context.Context, tx uow.Tx, id, worker string, attempt int) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteClaimedJob(ctx, postgresqueue.DeleteClaimedJobParams{ID: id, LockedBy: &worker, Attempt: int32Of(attempt)})
}

func (s postgresStore) release(ctx context.Context, tx uow.Tx, id, worker string, attempt, newAttempt int, runAt time.Time, lastErr string) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.ReleaseJob(ctx, postgresqueue.ReleaseJobParams{
		NewAttempt: int32Of(newAttempt),
		RunAt:      runAt,
		LastError:  optional(lastErr),
		ID:         id,
		Worker:     &worker,
		Attempt:    int32Of(attempt),
	})
}

func (s postgresStore) insertDeadFromClaim(ctx context.Context, tx uow.Tx, id, worker string, attempt int, lastErr string, failedAt time.Time) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.InsertDeadJobFromClaim(ctx, postgresqueue.InsertDeadJobFromClaimParams{
		LastError: lastErr,
		FailedAt:  failedAt,
		ID:        id,
		Worker:    &worker,
		Attempt:   int32Of(attempt),
	})
}

func (s postgresStore) listDead(ctx context.Context, tx uow.Tx, jobType string, after queue.DeadJobPosition, limit int) ([]queue.DeadJob, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDeadJobs(ctx, postgresqueue.ListDeadJobsParams{JobType: jobType, AfterID: toNullString(after.ID), AfterFailedAt: after.FailedAt, BatchLimit: int32Of(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]queue.DeadJob, len(rows))
	for i, r := range rows {
		out[i] = queue.DeadJob{
			ID:             r.ID,
			Type:           r.Type,
			Payload:        json.RawMessage(r.Payload),
			IdempotencyKey: deref(r.IdempotencyKey),
			OrderingKey:    deref(r.OrderingKey),
			TraceParent:    deref(r.TraceParent),
			Attempt:        int(r.Attempt),
			LastError:      r.LastError,
			CreatedAt:      kernel.Normalize(r.CreatedAt),
			FailedAt:       kernel.Normalize(r.FailedAt),
		}
	}
	return out, nil
}

func (s postgresStore) requeueDead(ctx context.Context, tx uow.Tx, id string, runAt time.Time) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	_, err = q.RequeueDeadJob(ctx, postgresqueue.RequeueDeadJobParams{RunAt: runAt, ID: id})
	return err
}

func (s postgresStore) deleteDead(ctx context.Context, tx uow.Tx, id string) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteDeadJob(ctx, id)
}

func (s postgresStore) deleteDeadBefore(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteDeadJobsBefore(ctx, postgresqueue.DeleteDeadJobsBeforeParams{FailedBefore: before, BatchLimit: int32Of(limit)})
}

func (s postgresStore) insertOutbox(ctx context.Context, tx uow.Tx, j queue.Job) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.InsertOutboxEvent(ctx, postgresqueue.InsertOutboxEventParams{
		ID:             j.ID,
		JobType:        j.Type,
		Payload:        j.Payload,
		IdempotencyKey: optional(j.IdempotencyKey),
		OrderingKey:    optional(j.OrderingKey),
		TraceParent:    optional(j.TraceParent),
		RunAt:          j.RunAt,
		CreatedAt:      j.CreatedAt,
	})
}

func (s postgresStore) listOutbox(ctx context.Context, tx uow.Tx, limit int) ([]queue.Job, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListOutboxEvents(ctx, int32Of(limit))
	if err != nil {
		return nil, err
	}
	out := make([]queue.Job, len(rows))
	for i, r := range rows {
		out[i] = queue.Job{
			ID:             r.ID,
			Type:           r.JobType,
			Payload:        json.RawMessage(r.Payload),
			IdempotencyKey: deref(r.IdempotencyKey),
			OrderingKey:    deref(r.OrderingKey),
			TraceParent:    deref(r.TraceParent),
			RunAt:          kernel.Normalize(r.RunAt),
			CreatedAt:      kernel.Normalize(r.CreatedAt),
		}
	}
	return out, nil
}

func (s postgresStore) deleteOutbox(ctx context.Context, tx uow.Tx, id string) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.DeleteOutboxEvent(ctx, id)
}

func (s postgresStore) backlog(ctx context.Context, tx uow.Tx, now time.Time) (queue.Backlog, error) {
	q, err := s.queries(tx)
	if err != nil {
		return queue.Backlog{}, err
	}
	rows, err := q.CountReadyJobs(ctx, now)
	if err != nil {
		return queue.Backlog{}, err
	}
	b := queue.Backlog{Ready: make(map[string]int64, len(rows))}
	for _, r := range rows {
		b.Ready[r.Type] = r.Ready
		if b.OldestReadyAt.IsZero() || r.OldestRunAt.Before(b.OldestReadyAt) {
			b.OldestReadyAt = r.OldestRunAt.UTC()
		}
	}
	b.Dead, err = q.CountDeadJobs(ctx)
	return b, err
}

func (s postgresStore) listPending(ctx context.Context, tx uow.Tx, now time.Time, f queue.JobFilter, after queue.JobPosition, limit int) ([]queue.PendingJob, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListPendingJobs(ctx, postgresqueue.ListPendingJobsParams{
		JobType: f.Type, State: string(f.State), Now: now, AfterID: toNullString(after.ID), AfterRunAt: after.RunAt, BatchLimit: int32Of(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]queue.PendingJob, len(rows))
	for i, r := range rows {
		var lockedUntil time.Time
		if r.LockedUntil.Valid {
			lockedUntil = kernel.Normalize(r.LockedUntil.Time)
		}
		out[i] = pendingJob(now, queue.PendingJob{
			ID:             r.ID,
			Type:           r.Type,
			Payload:        json.RawMessage(r.Payload),
			IdempotencyKey: deref(r.IdempotencyKey),
			OrderingKey:    deref(r.OrderingKey),
			Attempt:        int(r.Attempt),
			RunAt:          kernel.Normalize(r.RunAt),
			LastError:      deref(r.LastError),
			Worker:         deref(r.LockedBy),
			LockedUntil:    lockedUntil,
			CreatedAt:      kernel.Normalize(r.CreatedAt),
		})
	}
	return out, nil
}

func (s postgresStore) summarize(ctx context.Context, tx uow.Tx, now time.Time) (queue.JobSummary, error) {
	q, err := s.queries(tx)
	if err != nil {
		return queue.JobSummary{}, err
	}
	pending, err := q.CountJobsByState(ctx, now)
	if err != nil {
		return queue.JobSummary{}, err
	}
	dead, err := q.CountDeadJobsByType(ctx)
	if err != nil {
		return queue.JobSummary{}, err
	}
	b, err := s.backlog(ctx, tx, now)
	if err != nil {
		return queue.JobSummary{}, err
	}
	counts := make([]queue.JobCount, 0, len(pending)+len(dead))
	for _, r := range pending {
		counts = append(counts, queue.JobCount{State: queue.JobState(r.State), Type: r.Type, Count: r.Jobs})
	}
	for _, r := range dead {
		counts = append(counts, queue.JobCount{State: queue.JobStateDead, Type: r.Type, Count: r.Jobs})
	}
	return queue.JobSummary{Counts: sortCounts(counts), OldestReadyAt: kernel.Normalize(b.OldestReadyAt)}, nil
}

func (s postgresStore) insertAttempt(ctx context.Context, tx uow.Tx, a queue.Attempt) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.InsertJobAttempt(ctx, postgresqueue.InsertJobAttemptParams{
		JobID: a.JobID, Attempt: int32Of(a.Attempt), JobType: a.JobType, Outcome: a.Outcome, Error: a.Error, Worker: a.Worker,
		StartedAt: a.StartedAt, FinishedAt: a.FinishedAt,
	})
}

func (s postgresStore) listAttempts(ctx context.Context, tx uow.Tx, jobID string, after queue.AttemptPosition, limit int) ([]queue.Attempt, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListJobAttempts(ctx, postgresqueue.ListJobAttemptsParams{
		JobID: jobID, AfterAttempt: int32Of(after.Attempt), AfterFinishedAt: after.FinishedAt, BatchLimit: int32Of(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]queue.Attempt, len(rows))
	for i, r := range rows {
		out[i] = queue.Attempt{
			JobID: r.JobID, JobType: r.JobType, Attempt: int(r.Attempt), Outcome: r.Outcome, Error: r.Error, Worker: r.Worker,
			StartedAt: kernel.Normalize(r.StartedAt), FinishedAt: kernel.Normalize(r.FinishedAt),
		}
	}
	return out, nil
}

func (s postgresStore) deleteAttemptsBefore(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteJobAttemptsBefore(ctx, postgresqueue.DeleteJobAttemptsBeforeParams{FinishedBefore: before, BatchLimit: int32Of(limit)})
}
