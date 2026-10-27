-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for PostgreSQL (ADR-0023): the tables of the diagnostics module
-- (docs/architecture/data-model/04-tables/15-diagnostics.md). schema/README.md describes the
-- layout and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- diagnostics: the error groups (data-model §4.15, ADR-0103 §4 (2), §7). Owned by
-- internal/app/diagnostics. The reported errors arrive scrubbed (ADR-0103 §3): no column holds
-- an account, a user name, an IP address, or a request body.
-- ---------------------------------------------------------------------------------------------

-- Resource: the errors of one fingerprint (ADR-0103 §1). error_type is the last type of the
-- chain, message the first line of the scrubbed message (at most 200 runes), where_name the
-- finite name of the place, and first_version the service.version of the first occurrence.
-- unauthenticated marks a group whose first occurrence was a web report sent without a session,
-- which notifies nobody (error_group_confirmations).
CREATE TABLE error_groups (
  id uuid NOT NULL PRIMARY KEY,
  fingerprint text COLLATE "C" NOT NULL,
  source text NOT NULL CHECK (source IN ('server', 'web')),
  kind text NOT NULL CHECK (kind IN ('panic', 'error')),
  error_type text NOT NULL,
  message text NOT NULL,
  where_name text NOT NULL,
  first_version text NOT NULL,
  unauthenticated boolean NOT NULL,
  first_seen_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX error_groups_fingerprint ON error_groups (fingerprint);

-- Current value: the occurrences of a group in the hour that starts at hour_start, with the time
-- and version of the last of them. Every write adds to occurrences in one statement outside a
-- transaction; the daily diagnostics.prune_error_counts deletes the rows older than 90 days.
CREATE TABLE error_group_counts (
  group_id uuid NOT NULL,
  hour_start timestamptz NOT NULL,
  occurrences bigint NOT NULL CHECK (occurrences > 0),
  last_seen_at timestamptz NOT NULL,
  last_version text NOT NULL,
  PRIMARY KEY (group_id, hour_start)
);
CREATE INDEX error_group_counts_hour_start ON error_group_counts (hour_start);

-- Current value: the latest occurrence of a group, scrubbed. request_id and trace_id are '' for
-- none.
CREATE TABLE error_group_samples (
  group_id uuid NOT NULL PRIMARY KEY,
  error_type text NOT NULL,
  message text NOT NULL,
  stack text NOT NULL,
  request_id text NOT NULL,
  trace_id text NOT NULL,
  version text NOT NULL,
  role text NOT NULL,
  occurred_at timestamptz NOT NULL
);

-- Event: a change of a group's state. version is the version of the last occurrence for
-- resolved (a recurrence with another version regresses) and of the recurrence for regressed,
-- '' otherwise. actor_user_id is set exactly for a user actor; the server records regressed.
CREATE TABLE error_group_events (
  id uuid NOT NULL PRIMARY KEY,
  group_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('resolved', 'regressed', 'muted', 'unmuted')),
  actor_kind text NOT NULL CHECK (actor_kind IN ('user', 'cli', 'system')),
  actor_user_id uuid,
  version text NOT NULL,
  occurred_at timestamptz NOT NULL,
  CHECK ((actor_kind = 'user') = (actor_user_id IS NOT NULL)),
  CHECK ((kind = 'regressed') = (actor_kind = 'system'))
);
CREATE INDEX error_group_events_group_id_id ON error_group_events (group_id, id DESC);

-- Event: the first occurrence with a session of a group whose first occurrence had none: from
-- then on the group is one to notify, like a group that a report with a session created.
CREATE TABLE error_group_confirmations (
  group_id uuid NOT NULL PRIMARY KEY,
  confirmed_at timestamptz NOT NULL
);

-- Projection: the current state of every group, from its newest event (open without one).
-- resolved_version is the version of the resolved event while resolved, '' otherwise; since is
-- when the state began.
CREATE TABLE error_group_states (
  group_id uuid NOT NULL PRIMARY KEY,
  state text NOT NULL CHECK (state IN ('open', 'resolved', 'muted')),
  resolved_version text NOT NULL,
  since timestamptz NOT NULL,
  CHECK (state = 'resolved' OR resolved_version = '')
);
