// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/queue/db/internal/sqlitequeue"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// NewSQLite returns the queue kept in the SQLite database d.
func NewSQLite(d *sqlite.DB, opts Options) *Queue {
	opts.check()
	return &Queue{db: d, st: sqliteStore{d}, opts: opts}
}

// NewSQLiteOutbox returns the outbox kept in the SQLite database d.
func NewSQLiteOutbox(d *sqlite.DB, opts Options) *Outbox {
	return newOutbox(d, sqliteStore{d}, opts)
}

// sqliteStore stores timestamps as Unix milliseconds (data-model/01-conventions.md §1.2).
type sqliteStore struct{ db *sqlite.DB }

var _ store = sqliteStore{}

func (s sqliteStore) queries(tx uow.Tx) (*sqlitequeue.Queries, error) {
	db, err := s.db.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return sqlitequeue.New(db), nil
}

func millis(t time.Time) int64 { return t.UnixMilli() }

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func (s sqliteStore) insertJob(ctx context.Context, tx uow.Tx, j queue.Job) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.InsertJob(ctx, sqlitequeue.InsertJobParams{
		ID:             j.ID,
		Type:           j.Type,
		Payload:        j.Payload,
		IdempotencyKey: nullString(j.IdempotencyKey),
		OrderingKey:    nullString(j.OrderingKey),
		TraceParent:    nullString(j.TraceParent),
		RunAt:          millis(j.RunAt),
		CreatedAt:      millis(j.CreatedAt),
	})
}

func (s sqliteStore) claim(ctx context.Context, tx uow.Tx, worker string, now, until time.Time) (queue.Job, bool, error) {
	q, err := s.queries(tx)
	if err != nil {
		return queue.Job{}, false, err
	}
	r, err := q.ClaimJob(ctx, sqlitequeue.ClaimJobParams{
		Worker:      nullString(worker),
		LockedUntil: sql.NullInt64{Int64: millis(until), Valid: true},
		Now:         millis(now),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return queue.Job{}, false, nil
	}
	if err != nil {
		return queue.Job{}, false, err
	}
	return queue.Job{
		ID:             r.ID,
		Type:           r.Type,
		Payload:        json.RawMessage(r.Payload),
		IdempotencyKey: r.IdempotencyKey.String,
		TraceParent:    r.TraceParent.String,
		OrderingKey:    r.OrderingKey.String,
		Attempt:        int(r.Attempt),
		RunAt:          fromMillis(r.RunAt),
		LastError:      r.LastError.String,
		CreatedAt:      fromMillis(r.CreatedAt),
	}, true, nil
}

func (s sqliteStore) deleteClaimed(ctx context.Context, tx uow.Tx, id, worker string, attempt int) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteClaimedJob(ctx, sqlitequeue.DeleteClaimedJobParams{ID: id, LockedBy: nullString(worker), Attempt: int64(attempt)})
}

func (s sqliteStore) release(ctx context.Context, tx uow.Tx, id, worker string, attempt, newAttempt int, runAt time.Time, lastErr string) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.ReleaseJob(ctx, sqlitequeue.ReleaseJobParams{
		NewAttempt: int64(newAttempt),
		RunAt:      millis(runAt),
		LastError:  nullString(lastErr),
		ID:         id,
		Worker:     nullString(worker),
		Attempt:    int64(attempt),
	})
}

func (s sqliteStore) insertDeadFromClaim(ctx context.Context, tx uow.Tx, id, worker string, attempt int, lastErr string, failedAt time.Time) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.InsertDeadJobFromClaim(ctx, sqlitequeue.InsertDeadJobFromClaimParams{
		LastError: lastErr,
		FailedAt:  millis(failedAt),
		ID:        id,
		Worker:    nullString(worker),
		Attempt:   int64(attempt),
	})
}

func (s sqliteStore) listDead(ctx context.Context, tx uow.Tx, jobType string, after queue.DeadJobPosition, limit int) ([]queue.DeadJob, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	var afterFailedAt int64
	if after.ID != "" {
		afterFailedAt = millis(after.FailedAt)
	}
	rows, err := q.ListDeadJobs(ctx, sqlitequeue.ListDeadJobsParams{JobType: jobType, AfterID: after.ID, AfterFailedAt: afterFailedAt, BatchLimit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]queue.DeadJob, len(rows))
	for i, r := range rows {
		out[i] = queue.DeadJob{
			ID:             r.ID,
			Type:           r.Type,
			Payload:        json.RawMessage(r.Payload),
			IdempotencyKey: r.IdempotencyKey.String,
			OrderingKey:    r.OrderingKey.String,
			TraceParent:    r.TraceParent.String,
			Attempt:        int(r.Attempt),
			LastError:      r.LastError,
			CreatedAt:      fromMillis(r.CreatedAt),
			FailedAt:       fromMillis(r.FailedAt),
		}
	}
	return out, nil
}

func (s sqliteStore) requeueDead(ctx context.Context, tx uow.Tx, id string, runAt time.Time) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	_, err = q.RequeueDeadJob(ctx, sqlitequeue.RequeueDeadJobParams{RunAt: millis(runAt), ID: id})
	return err
}

func (s sqliteStore) deleteDead(ctx context.Context, tx uow.Tx, id string) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteDeadJob(ctx, id)
}

func (s sqliteStore) deleteDeadBefore(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteDeadJobsBefore(ctx, sqlitequeue.DeleteDeadJobsBeforeParams{FailedBefore: millis(before), BatchLimit: int64(limit)})
}

func (s sqliteStore) insertOutbox(ctx context.Context, tx uow.Tx, j queue.Job) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.InsertOutboxEvent(ctx, sqlitequeue.InsertOutboxEventParams{
		ID:             j.ID,
		JobType:        j.Type,
		Payload:        j.Payload,
		IdempotencyKey: nullString(j.IdempotencyKey),
		OrderingKey:    nullString(j.OrderingKey),
		TraceParent:    nullString(j.TraceParent),
		RunAt:          millis(j.RunAt),
		CreatedAt:      millis(j.CreatedAt),
	})
}

func (s sqliteStore) listOutbox(ctx context.Context, tx uow.Tx, limit int) ([]queue.Job, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListOutboxEvents(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]queue.Job, len(rows))
	for i, r := range rows {
		out[i] = queue.Job{
			ID:             r.ID,
			Type:           r.JobType,
			Payload:        json.RawMessage(r.Payload),
			IdempotencyKey: r.IdempotencyKey.String,
			TraceParent:    r.TraceParent.String,
			OrderingKey:    r.OrderingKey.String,
			RunAt:          fromMillis(r.RunAt),
			CreatedAt:      fromMillis(r.CreatedAt),
		}
	}
	return out, nil
}

func (s sqliteStore) deleteOutbox(ctx context.Context, tx uow.Tx, id string) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.DeleteOutboxEvent(ctx, id)
}

func (s sqliteStore) acquireRelayLease(ctx context.Context, tx uow.Tx, name, holder string, now, until time.Time) (bool, error) {
	q, err := s.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.AcquireOutboxRelayLease(ctx, sqlitequeue.AcquireOutboxRelayLeaseParams{
		Name: name, Holder: holder, LeaseUntil: millis(until), Now: millis(now),
	})
	return n == 1, err
}

func (s sqliteStore) endRelayLease(ctx context.Context, tx uow.Tx, name, holder string, now time.Time) (bool, error) {
	q, err := s.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.EndOutboxRelayLease(ctx, sqlitequeue.EndOutboxRelayLeaseParams{Name: name, Holder: holder, Now: millis(now)})
	return n == 1, err
}

func (s sqliteStore) backlog(ctx context.Context, tx uow.Tx, now time.Time) (queue.Backlog, error) {
	q, err := s.queries(tx)
	if err != nil {
		return queue.Backlog{}, err
	}
	rows, err := q.CountReadyJobs(ctx, millis(now))
	if err != nil {
		return queue.Backlog{}, err
	}
	b := queue.Backlog{Ready: make(map[string]int64, len(rows))}
	for _, r := range rows {
		b.Ready[r.Type] = r.Ready
		if at := fromMillis(r.OldestRunAt); b.OldestReadyAt.IsZero() || at.Before(b.OldestReadyAt) {
			b.OldestReadyAt = at
		}
	}
	b.Dead, err = q.CountDeadJobs(ctx)
	return b, err
}

func (s sqliteStore) listPending(ctx context.Context, tx uow.Tx, now time.Time, f queue.JobFilter, after queue.JobPosition, limit int) ([]queue.PendingJob, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	var afterRunAt int64
	if after.ID != "" {
		afterRunAt = millis(after.RunAt)
	}
	rows, err := q.ListPendingJobs(ctx, sqlitequeue.ListPendingJobsParams{
		JobType: f.Type, State: string(f.State), Now: millis(now), AfterID: after.ID, AfterRunAt: afterRunAt, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]queue.PendingJob, len(rows))
	for i, r := range rows {
		var lockedUntil time.Time
		if r.LockedUntil.Valid {
			lockedUntil = fromMillis(r.LockedUntil.Int64)
		}
		out[i] = pendingJob(now, queue.PendingJob{
			ID:             r.ID,
			Type:           r.Type,
			Payload:        json.RawMessage(r.Payload),
			IdempotencyKey: r.IdempotencyKey.String,
			OrderingKey:    r.OrderingKey.String,
			Attempt:        int(r.Attempt),
			RunAt:          fromMillis(r.RunAt),
			LastError:      r.LastError.String,
			Worker:         r.LockedBy.String,
			LockedUntil:    lockedUntil,
			CreatedAt:      fromMillis(r.CreatedAt),
		})
	}
	return out, nil
}

func (s sqliteStore) summarize(ctx context.Context, tx uow.Tx, now time.Time) (queue.JobSummary, error) {
	q, err := s.queries(tx)
	if err != nil {
		return queue.JobSummary{}, err
	}
	pending, err := q.CountJobsByState(ctx, millis(now))
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
	return queue.JobSummary{Counts: sortCounts(counts), OldestReadyAt: b.OldestReadyAt}, nil
}

func (s sqliteStore) insertAttempt(ctx context.Context, tx uow.Tx, a queue.Attempt) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.InsertJobAttempt(ctx, sqlitequeue.InsertJobAttemptParams{
		JobID: a.JobID, Attempt: int64(a.Attempt), JobType: a.JobType, Outcome: a.Outcome, Error: a.Error, Worker: a.Worker,
		StartedAt: millis(a.StartedAt), FinishedAt: millis(a.FinishedAt),
	})
}

func (s sqliteStore) listAttempts(ctx context.Context, tx uow.Tx, jobID string, after queue.AttemptPosition, limit int) ([]queue.Attempt, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	var afterFinishedAt int64
	if after.Attempt != 0 {
		afterFinishedAt = millis(after.FinishedAt)
	}
	rows, err := q.ListJobAttempts(ctx, sqlitequeue.ListJobAttemptsParams{
		JobID: jobID, AfterAttempt: int64(after.Attempt), AfterFinishedAt: afterFinishedAt, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]queue.Attempt, len(rows))
	for i, r := range rows {
		out[i] = queue.Attempt{
			JobID: r.JobID, JobType: r.JobType, Attempt: int(r.Attempt), Outcome: r.Outcome, Error: r.Error, Worker: r.Worker,
			StartedAt: fromMillis(r.StartedAt), FinishedAt: fromMillis(r.FinishedAt),
		}
	}
	return out, nil
}

func (s sqliteStore) deleteAttemptsBefore(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteJobAttemptsBefore(ctx, sqlitequeue.DeleteJobAttemptsBeforeParams{FinishedBefore: millis(before), BatchLimit: int64(limit)})
}
