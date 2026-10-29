-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the database job queue (internal/adapter/queue/db) for SQLite.
-- Timestamps are Unix milliseconds (see docs/architecture/data-model/README.md, section 1.2).

-- name: InsertJob :exec
INSERT INTO jobs (id, type, payload, idempotency_key, ordering_key, trace_parent, attempt, run_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
ON CONFLICT (idempotency_key) DO NOTHING;

-- ClaimJob leases the due, unleased job with the earliest run_at that its ordering key does
-- not hold back: no other job of the key may be leased or enqueued before it, by (created_at,
-- id). BEGIN IMMEDIATE serializes claims, so two workers never claim the same job or two jobs
-- of one key.
-- name: ClaimJob :one
UPDATE jobs
SET locked_by = @worker, locked_until = @locked_until, attempt = attempt + 1
WHERE id = (
  SELECT j.id FROM jobs AS j
  WHERE j.run_at <= @now AND (j.locked_until IS NULL OR j.locked_until <= @now)
    AND NOT EXISTS (
      SELECT 1 FROM jobs AS k
      WHERE k.ordering_key = j.ordering_key AND k.id <> j.id
        AND (k.locked_until > @now OR k.created_at < j.created_at OR (k.created_at = j.created_at AND k.id < j.id))
    )
  ORDER BY j.run_at, j.id
  LIMIT 1
)
RETURNING id, type, payload, idempotency_key, ordering_key, trace_parent, attempt, run_at, last_error, created_at;

-- ClaimJobByID leases the job @id under the conditions of ClaimJob, for a claim by ID (a
-- wake-up signal, ADR-0120 section 2). The claim reads the job's lease with GetJobLease before
-- and, when this leases nothing, after it, to tell why.
-- name: ClaimJobByID :one
UPDATE jobs
SET locked_by = @worker, locked_until = @locked_until, attempt = attempt + 1
WHERE id = (
  SELECT j.id FROM jobs AS j
  WHERE j.id = @id AND j.run_at <= @now AND (j.locked_until IS NULL OR j.locked_until <= @now)
    AND NOT EXISTS (
      SELECT 1 FROM jobs AS k
      WHERE k.ordering_key = j.ordering_key AND k.id <> j.id
        AND (k.locked_until > @now OR k.created_at < j.created_at OR (k.created_at = j.created_at AND k.id < j.id))
    )
)
RETURNING id, type, payload, idempotency_key, ordering_key, trace_parent, attempt, run_at, last_error, created_at;

-- name: GetJobLease :one
SELECT ordering_key, run_at, locked_until FROM jobs WHERE id = ?;

-- ListDueJobs is the condition of ClaimJob bounded by @batch_limit, for the sweep of lost
-- wake-up signals: the jobs due at @due_before that a claim at @now could take.
-- name: ListDueJobs :many
SELECT j.id FROM jobs AS j
WHERE j.run_at <= @due_before AND (j.locked_until IS NULL OR j.locked_until <= @now)
  AND NOT EXISTS (
    SELECT 1 FROM jobs AS k
    WHERE k.ordering_key = j.ordering_key AND k.id <> j.id
      AND (k.locked_until > @now OR k.created_at < j.created_at OR (k.created_at = j.created_at AND k.id < j.id))
  )
ORDER BY j.run_at, j.id
LIMIT @batch_limit;

-- name: NextJobOfKey :one
SELECT id, run_at FROM jobs WHERE ordering_key = ? ORDER BY created_at, id LIMIT 1;

-- name: DeleteClaimedJob :execrows
DELETE FROM jobs WHERE id = ? AND locked_by = ? AND attempt = ?;

-- name: ReleaseJob :execrows
UPDATE jobs
SET locked_by = NULL, locked_until = NULL,
  attempt = @new_attempt, run_at = @run_at, last_error = @last_error
WHERE id = @id AND locked_by = @worker AND attempt = @attempt;

-- name: InsertDeadJobFromClaim :execrows
INSERT INTO dead_jobs (id, type, payload, idempotency_key, ordering_key, trace_parent, attempt, last_error, created_at, failed_at)
SELECT j.id, j.type, j.payload, j.idempotency_key, j.ordering_key, j.trace_parent, j.attempt, @last_error, j.created_at, @failed_at
FROM jobs AS j
WHERE j.id = @id AND j.locked_by = @worker AND j.attempt = @attempt;

-- ListDeadJobs returns the dead jobs of the type @job_type (every type when empty), most
-- recently failed first, after the position (@after_failed_at, @after_id); an empty @after_id
-- starts at the first.
-- name: ListDeadJobs :many
SELECT id, type, payload, idempotency_key, ordering_key, trace_parent, attempt, last_error, created_at, failed_at
FROM dead_jobs
WHERE (CAST(sqlc.arg(job_type) AS TEXT) = '' OR type = CAST(sqlc.arg(job_type) AS TEXT))
  AND (CAST(sqlc.arg(after_id) AS TEXT) = ''
    OR failed_at < CAST(sqlc.arg(after_failed_at) AS INTEGER)
    OR (failed_at = CAST(sqlc.arg(after_failed_at) AS INTEGER) AND id < CAST(sqlc.arg(after_id) AS TEXT)))
ORDER BY failed_at DESC, id DESC
LIMIT sqlc.arg(batch_limit);

-- name: RequeueDeadJob :execrows
INSERT INTO jobs (id, type, payload, idempotency_key, ordering_key, trace_parent, attempt, run_at, created_at)
SELECT d.id, d.type, d.payload, d.idempotency_key, d.ordering_key, d.trace_parent, 0, @run_at, d.created_at
FROM dead_jobs AS d
WHERE d.id = @id
ON CONFLICT (idempotency_key) DO NOTHING;

-- name: DeleteDeadJob :execrows
DELETE FROM dead_jobs WHERE id = ?;

-- DeleteDeadJobsBefore removes at most @batch_limit dead jobs that failed before @failed_before,
-- the oldest first.
-- name: DeleteDeadJobsBefore :execrows
DELETE FROM dead_jobs
WHERE id IN (
  SELECT d.id FROM dead_jobs AS d WHERE d.failed_at < CAST(sqlc.arg(failed_before) AS INTEGER)
  ORDER BY d.failed_at, d.id LIMIT sqlc.arg(batch_limit)
);

-- CountReadyJobs counts the due, unleased jobs of each type, with the earliest run_at among
-- them, for the queue gauges of the metrics snapshot (ADR-0055 section 4). It counts the jobs
-- that their ordering key holds back too.
-- name: CountReadyJobs :many
SELECT type, COUNT(*) AS ready, CAST(MIN(run_at) AS INTEGER) AS oldest_run_at
FROM jobs
WHERE run_at <= @now AND (locked_until IS NULL OR locked_until <= @now)
GROUP BY type
ORDER BY type;

-- name: CountDeadJobs :one
SELECT COUNT(*) FROM dead_jobs;

-- ListPendingJobs returns the jobs of the type @job_type (every type when empty) in the state
-- @state at @now (every state when empty; the states of ADR-0064 section 3: running while the
-- lease holds, otherwise queued before the first run and retrying after it), the earliest run_at
-- first, after the position (@after_run_at, @after_id); an empty @after_id starts at the first.
-- name: ListPendingJobs :many
SELECT id, type, payload, idempotency_key, ordering_key, attempt, run_at, locked_by, locked_until, last_error, created_at
FROM jobs
WHERE (CAST(sqlc.arg(job_type) AS TEXT) = '' OR type = CAST(sqlc.arg(job_type) AS TEXT))
  AND (CAST(sqlc.arg(state) AS TEXT) = ''
    OR (CAST(sqlc.arg(state) AS TEXT) = 'running' AND locked_until > CAST(sqlc.arg(now) AS INTEGER))
    OR (CAST(sqlc.arg(state) AS TEXT) = 'queued' AND (locked_until IS NULL OR locked_until <= CAST(sqlc.arg(now) AS INTEGER)) AND attempt = 0)
    OR (CAST(sqlc.arg(state) AS TEXT) = 'retrying' AND (locked_until IS NULL OR locked_until <= CAST(sqlc.arg(now) AS INTEGER)) AND attempt > 0))
  AND (CAST(sqlc.arg(after_id) AS TEXT) = ''
    OR run_at > CAST(sqlc.arg(after_run_at) AS INTEGER)
    OR (run_at = CAST(sqlc.arg(after_run_at) AS INTEGER) AND id > CAST(sqlc.arg(after_id) AS TEXT)))
ORDER BY run_at, id
LIMIT sqlc.arg(batch_limit);

-- CountJobsByState counts the jobs by type and by their state at @now, as ListPendingJobs
-- derives it.
-- name: CountJobsByState :many
SELECT type,
  CAST(CASE WHEN locked_until > CAST(sqlc.arg(now) AS INTEGER) THEN 'running' WHEN attempt = 0 THEN 'queued' ELSE 'retrying' END AS TEXT) AS state,
  COUNT(*) AS jobs
FROM jobs
GROUP BY type, state
ORDER BY type, state;

-- name: CountDeadJobsByType :many
SELECT type, COUNT(*) AS jobs FROM dead_jobs GROUP BY type ORDER BY type;

-- name: InsertJobAttempt :exec
INSERT INTO job_attempts (job_id, attempt, job_type, outcome, error, worker, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (job_id, attempt, finished_at) DO NOTHING;

-- ListJobAttempts returns the records of the job @job_id, the most recently finished first,
-- after the position (@after_finished_at, @after_attempt); an @after_attempt of 0 starts at the
-- first.
-- name: ListJobAttempts :many
SELECT job_id, attempt, job_type, outcome, error, worker, started_at, finished_at
FROM job_attempts
WHERE job_id = sqlc.arg(job_id)
  AND (CAST(sqlc.arg(after_attempt) AS INTEGER) = 0
    OR finished_at < CAST(sqlc.arg(after_finished_at) AS INTEGER)
    OR (finished_at = CAST(sqlc.arg(after_finished_at) AS INTEGER) AND attempt < CAST(sqlc.arg(after_attempt) AS INTEGER)))
ORDER BY finished_at DESC, attempt DESC
LIMIT sqlc.arg(batch_limit);

-- DeleteJobAttemptsBefore removes at most @batch_limit records that finished before
-- @finished_before, the oldest first.
-- name: DeleteJobAttemptsBefore :execrows
DELETE FROM job_attempts
WHERE rowid IN (
  SELECT a.rowid FROM job_attempts AS a WHERE a.finished_at < CAST(sqlc.arg(finished_before) AS INTEGER)
  ORDER BY a.finished_at LIMIT sqlc.arg(batch_limit)
);
