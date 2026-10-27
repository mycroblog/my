-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for PostgreSQL (ADR-0023): the tables that adapters own
-- (docs/architecture/data-model/04-tables/10-infrastructure.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- 基盤: job queue and Transactional Outbox (data-model/04-tables/10-infrastructure.md §4.10, ADR-0004).
-- Owned by internal/adapter/queue/db. trace_parent is the W3C traceparent of the span that
-- enqueued the job, which its run links to (ADR-0063 §2); an attribute of the job, not a state.
-- ---------------------------------------------------------------------------------------------

-- Jobs written in a domain transaction, waiting for the relay to enqueue them.
CREATE TABLE outbox_events (
  id uuid NOT NULL PRIMARY KEY,
  job_type text NOT NULL,
  payload bytea NOT NULL,
  idempotency_key text,
  ordering_key text,
  trace_parent text,
  run_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL
);

-- Current value: the lease of the relay that moves outbox_events, one row per outbox. Only the
-- holder relays until lease_until, so that overlapping relays keep the events in order.
CREATE TABLE outbox_relay_leases (
  name text NOT NULL PRIMARY KEY,
  holder text NOT NULL,
  lease_until timestamptz NOT NULL
);

-- Pending and running jobs of the database queue. A job is deleted when it finishes and moved
-- to dead_jobs when it fails for good. A claim sets locked_by and locked_until together.
CREATE TABLE jobs (
  id uuid NOT NULL PRIMARY KEY,
  type text NOT NULL,
  payload bytea NOT NULL,
  idempotency_key text,
  ordering_key text,
  trace_parent text,
  attempt integer NOT NULL,
  run_at timestamptz NOT NULL,
  locked_by text,
  locked_until timestamptz,
  last_error text,
  created_at timestamptz NOT NULL,
  CONSTRAINT jobs_attempt_check CHECK (attempt >= 0),
  CONSTRAINT jobs_lock_check CHECK ((locked_by IS NULL) = (locked_until IS NULL))
);
CREATE UNIQUE INDEX jobs_idempotency_key ON jobs (idempotency_key);
CREATE INDEX jobs_run_at ON jobs (run_at);
CREATE INDEX jobs_ordering_key_run_at ON jobs (ordering_key, run_at);

-- Jobs that failed for good, kept for the admin screen and the CLI to requeue.
CREATE TABLE dead_jobs (
  id uuid NOT NULL PRIMARY KEY,
  type text NOT NULL,
  payload bytea NOT NULL,
  idempotency_key text,
  ordering_key text,
  trace_parent text,
  attempt integer NOT NULL,
  last_error text NOT NULL,
  created_at timestamptz NOT NULL,
  failed_at timestamptz NOT NULL
);
CREATE INDEX dead_jobs_failed_at ON dead_jobs (failed_at);

-- Event: a run of a job that did not succeed (ADR-0064 §1), written by the runner whatever the
-- queue adapter. outcome is retry (the job runs again) or dead (it failed for good). error is
-- at most 2,000 bytes; no payload and no handler log. A requeued dead job counts its attempts
-- again, so finished_at is part of the key. Purged job_attempt_retention after finished_at.
CREATE TABLE job_attempts (
  job_id uuid NOT NULL,
  attempt integer NOT NULL CHECK (attempt >= 1),
  job_type text NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('retry', 'dead')),
  error text NOT NULL CHECK (length(error) <= 2000),
  worker text NOT NULL,
  started_at timestamptz NOT NULL,
  finished_at timestamptz NOT NULL,
  PRIMARY KEY (job_id, attempt, finished_at)
);
CREATE INDEX job_attempts_finished_at ON job_attempts (finished_at);

-- ---------------------------------------------------------------------------------------------
-- 基盤: key-value store (data-model/04-tables/10-infrastructure.md §4.10). Owned by internal/adapter/kv/db.
-- ---------------------------------------------------------------------------------------------

-- Entries of the KeyValueStore port. expires_at is NULL for entries that never expire.
CREATE TABLE kv_entries (
  namespace text NOT NULL,
  key text NOT NULL,
  value bytea NOT NULL,
  expires_at timestamptz,
  PRIMARY KEY (namespace, key)
);
CREATE INDEX kv_entries_expires_at ON kv_entries (expires_at);

-- ---------------------------------------------------------------------------------------------
-- 基盤: record of the data upgrades (data-model/04-tables/10-infrastructure.md §4.10). Owned by
-- internal/adapter/db; written by `my db upgrade` (internal/app/dbupgrade).
-- ---------------------------------------------------------------------------------------------

-- Event: a data upgrade step that completed. step_key names the step that the binary registers;
-- a step is recorded once, after its last transaction.
CREATE TABLE data_upgrade_runs (
  step_key text COLLATE "C" NOT NULL PRIMARY KEY,
  completed_at timestamptz NOT NULL
);
