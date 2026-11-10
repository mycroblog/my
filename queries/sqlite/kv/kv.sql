-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the database key-value store (internal/adapter/kv/db) for SQLite.
-- Timestamps are Unix milliseconds (see docs/architecture/data-model/README.md, section 1.2); an entry
-- is live while expires_at is NULL or later than now. Counters are decimal text in value.

-- name: GetEntry :one
SELECT value FROM kv_entries
WHERE namespace = @namespace AND key = @key AND (expires_at IS NULL OR expires_at > @now);

-- name: ListEntries :many
SELECT key, value FROM kv_entries
WHERE namespace = @namespace AND key IN (SELECT value FROM json_each(CAST(@keys AS TEXT))) AND (expires_at IS NULL OR expires_at > @now);

-- name: SetEntry :exec
INSERT INTO kv_entries (namespace, key, value, expires_at) VALUES (@namespace, @key, @value, @expires_at)
ON CONFLICT (namespace, key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at;

-- name: DeleteEntry :exec
DELETE FROM kv_entries WHERE namespace = @namespace AND key = @key;

-- DeleteExpiredEntry removes an expired entry, so that IncrEntry in the same transaction starts it
-- again. (The sqlite engine of sqlc cannot bind parameters in an upsert's DO UPDATE clause.)
-- name: DeleteExpiredEntry :exec
DELETE FROM kv_entries WHERE namespace = @namespace AND key = @key AND expires_at <= @now;

-- IncrEntry adds the counter in @initial (decimal text) to an existing counter, keeping its
-- expiry, or starts a missing one at @initial with expiry @expires_at.
-- name: IncrEntry :one
INSERT INTO kv_entries (namespace, key, value, expires_at) VALUES (@namespace, @key, @initial, @expires_at)
ON CONFLICT (namespace, key) DO UPDATE SET
  value = CAST(CAST(CAST(CAST(kv_entries.value AS TEXT) AS INTEGER) + CAST(CAST(excluded.value AS TEXT) AS INTEGER) AS TEXT) AS BLOB)
RETURNING value;

-- DeleteExpiredEntries removes at most @batch_limit expired entries, the earliest expired first.
-- name: DeleteExpiredEntries :execrows
DELETE FROM kv_entries
WHERE rowid IN (
  SELECT e.rowid FROM kv_entries AS e WHERE e.expires_at <= @now ORDER BY e.expires_at LIMIT @batch_limit
);
