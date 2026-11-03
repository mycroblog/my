-- SPDX-License-Identifier: AGPL-3.0-only
-- The jobs table as in schema/postgres/10-infrastructure.sql, with 1M due jobs.
-- :mode is 'burst' (900k deliveries older than everything else: a worker that skips them walks them all)
-- or 'mixed' (the same jobs with run_at uniformly mixed).
CREATE TABLE jobs (
  id uuid NOT NULL PRIMARY KEY, type text NOT NULL, payload bytea NOT NULL, idempotency_key text, ordering_key text,
  trace_parent text, attempt integer NOT NULL, run_at timestamptz NOT NULL, locked_by text, locked_until timestamptz,
  last_error text, created_at timestamptz NOT NULL,
  CONSTRAINT jobs_attempt_check CHECK (attempt >= 0),
  CONSTRAINT jobs_lock_check CHECK ((locked_by IS NULL) = (locked_until IS NULL))
);
INSERT INTO jobs (id, type, payload, idempotency_key, ordering_key, attempt, run_at, created_at)
SELECT uuidv7(), t.type, '\x7b7d'::bytea, 'k' || t.i, t.ok, 0, t.run_at, t.run_at FROM (
  SELECT i,
    CASE WHEN i <= 900000 THEN 'federation.deliver'
         WHEN i <= 940000 THEN 'notification.email'
         WHEN i <= 980000 THEN 'federation.inbox'
         WHEN i <= 999000 THEN 'media.process'
         ELSE 'identity.email' END AS type,
    CASE WHEN i > 900000 AND i <= 980000 AND i % 2 = 0 THEN 'actor:' || (i % 5000) END AS ok,
    CASE WHEN :'mode' = 'burst' THEN
      CASE WHEN i <= 900000 THEN now() - interval '3 hour' + i * interval '8 millisecond'
           ELSE now() - interval '1 hour' + (i - 900000) * interval '30 millisecond' END
      ELSE now() - interval '3 hour' + random() * interval '3 hour' END AS run_at
  FROM generate_series(1, 1000000) AS i
) AS t ORDER BY t.run_at;
CREATE UNIQUE INDEX jobs_idempotency_key ON jobs (idempotency_key);
CREATE INDEX jobs_run_at ON jobs (run_at);
CREATE INDEX jobs_ordering_key_run_at ON jobs (ordering_key, run_at);
VACUUM ANALYZE jobs;
