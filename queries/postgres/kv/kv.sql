-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the database key-value store (internal/adapter/kv/db) for PostgreSQL.
-- An entry is live while expires_at is NULL or later than now. Counters are decimal text in value.

-- name: GetEntry :one
SELECT value FROM kv_entries
WHERE namespace = @namespace AND key = @key AND (expires_at IS NULL OR expires_at > @now::timestamptz);

-- name: SetEntry :exec
INSERT INTO kv_entries (namespace, key, value, expires_at) VALUES (@namespace, @key, @value, @expires_at)
ON CONFLICT (namespace, key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at;

-- name: DeleteEntry :exec
DELETE FROM kv_entries WHERE namespace = @namespace AND key = @key;

-- IncrEntry adds the counter in @initial (decimal text) to a live counter, keeping its expiry, or
-- starts a missing or expired one at @initial with expiry @expires_at, in one statement.
-- name: IncrEntry :one
INSERT INTO kv_entries (namespace, key, value, expires_at) VALUES (@namespace, @key, @initial, @expires_at)
ON CONFLICT (namespace, key) DO UPDATE SET
  value = CASE WHEN kv_entries.expires_at <= @now::timestamptz THEN excluded.value
    ELSE convert_to((convert_from(kv_entries.value, 'UTF8')::bigint + convert_from(excluded.value, 'UTF8')::bigint)::text, 'UTF8') END,
  expires_at = CASE WHEN kv_entries.expires_at <= @now::timestamptz THEN excluded.expires_at
    ELSE kv_entries.expires_at END
RETURNING value;

-- DeleteExpiredEntries removes at most @batch_limit expired entries, the earliest expired first.
-- The outer condition is checked again on each row it locks, so that an entry that IncrEntry
-- renewed meanwhile stays.
-- name: DeleteExpiredEntries :execrows
DELETE FROM kv_entries
WHERE expires_at <= @now::timestamptz
  AND (namespace, key) IN (
    SELECT namespace, key FROM kv_entries
    WHERE expires_at <= @now::timestamptz
    ORDER BY expires_at
    LIMIT @batch_limit::integer
  );

-- SwapEntry replaces a live entry that holds @old with @value and expiry @expires_at.
-- name: SwapEntry :execrows
UPDATE kv_entries SET value = @value, expires_at = @expires_at
WHERE namespace = @namespace AND key = @key AND value = @old
  AND (expires_at IS NULL OR expires_at > @now::timestamptz);

-- SwapAbsentEntry writes @value with expiry @expires_at at a key without a live entry.
-- name: SwapAbsentEntry :execrows
INSERT INTO kv_entries (namespace, key, value, expires_at) VALUES (@namespace, @key, @value, @expires_at)
ON CONFLICT (namespace, key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at
WHERE kv_entries.expires_at <= @now::timestamptz;

-- DeleteEntryIf removes a live entry that holds @old.
-- name: DeleteEntryIf :execrows
DELETE FROM kv_entries
WHERE namespace = @namespace AND key = @key AND value = @old
  AND (expires_at IS NULL OR expires_at > @now::timestamptz);
