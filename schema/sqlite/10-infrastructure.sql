-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables that adapters own
-- (docs/architecture/data-model/04-tables/10-infrastructure.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- 基盤: job queue and Transactional Outbox (data-model/04-tables/10-infrastructure.md §4.10, ADR-0004).
-- Owned by internal/adapter/queue/db. trace_parent is the W3C traceparent of the span that
-- enqueued the job, which its run links to (ADR-0063 §2); an attribute of the job, not a state.
-- ---------------------------------------------------------------------------------------------

-- Jobs written in a domain transaction, waiting for the relay to enqueue them.
CREATE TABLE outbox_events (
  id TEXT NOT NULL PRIMARY KEY,
  job_type TEXT NOT NULL,
  payload BLOB NOT NULL,
  idempotency_key TEXT,
  ordering_key TEXT,
  trace_parent TEXT,
  run_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;

-- Pending and running jobs of the database queue. A job is deleted when it finishes and moved
-- to dead_jobs when it fails for good. A claim sets locked_by and locked_until together.
CREATE TABLE jobs (
  id TEXT NOT NULL PRIMARY KEY,
  type TEXT NOT NULL,
  payload BLOB NOT NULL,
  idempotency_key TEXT,
  ordering_key TEXT,
  trace_parent TEXT,
  attempt INTEGER NOT NULL CHECK (attempt >= 0),
  run_at INTEGER NOT NULL,
  locked_by TEXT,
  locked_until INTEGER,
  last_error TEXT,
  created_at INTEGER NOT NULL,
  CHECK ((locked_by IS NULL) = (locked_until IS NULL))
) STRICT;
CREATE UNIQUE INDEX jobs_idempotency_key ON jobs (idempotency_key);
CREATE INDEX jobs_run_at ON jobs (run_at);
CREATE INDEX jobs_ordering_key_run_at ON jobs (ordering_key, run_at);

-- Jobs that failed for good, kept for the admin screen and the CLI to requeue.
CREATE TABLE dead_jobs (
  id TEXT NOT NULL PRIMARY KEY,
  type TEXT NOT NULL,
  payload BLOB NOT NULL,
  idempotency_key TEXT,
  ordering_key TEXT,
  trace_parent TEXT,
  attempt INTEGER NOT NULL,
  last_error TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  failed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX dead_jobs_failed_at ON dead_jobs (failed_at);

-- Event: a run of a job that did not succeed (ADR-0064 §1), written by the runner whatever the
-- queue adapter. outcome is retry (the job runs again) or dead (it failed for good). error is
-- at most 2,000 bytes; no payload and no handler log. A requeued dead job counts its attempts
-- again, so finished_at is part of the key. Purged job_attempt_retention after finished_at.
CREATE TABLE job_attempts (
  job_id TEXT NOT NULL,
  attempt INTEGER NOT NULL CHECK (attempt >= 1),
  job_type TEXT NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('retry', 'dead')),
  error TEXT NOT NULL CHECK (length(error) <= 2000),
  worker TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  finished_at INTEGER NOT NULL,
  PRIMARY KEY (job_id, attempt, finished_at)
) STRICT;
CREATE INDEX job_attempts_finished_at ON job_attempts (finished_at);

-- ---------------------------------------------------------------------------------------------
-- 基盤: key-value store (data-model/04-tables/10-infrastructure.md §4.10). Owned by internal/adapter/kv/db.
-- ---------------------------------------------------------------------------------------------

-- Entries of the KeyValueStore port. expires_at is NULL for entries that never expire.
CREATE TABLE kv_entries (
  namespace TEXT NOT NULL,
  key TEXT NOT NULL,
  value BLOB NOT NULL,
  expires_at INTEGER,
  PRIMARY KEY (namespace, key)
) STRICT;
CREATE INDEX kv_entries_expires_at ON kv_entries (expires_at);

-- ---------------------------------------------------------------------------------------------
-- 基盤: record of the data upgrades (data-model/04-tables/10-infrastructure.md §4.10). Owned by
-- internal/adapter/db; written by `my db upgrade` (internal/app/dbupgrade).
-- ---------------------------------------------------------------------------------------------

-- Event: a data upgrade step that completed. step_key names the step that the binary registers;
-- a step is recorded once, after its last transaction.
CREATE TABLE data_upgrade_runs (
  step_key TEXT NOT NULL PRIMARY KEY,
  completed_at INTEGER NOT NULL
) STRICT;
