-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertSession :exec
INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertSessionRevocationIfAbsent :exec
INSERT INTO session_revocations (session_id, reason, revoked_at) VALUES ($1, $2, $3)
ON CONFLICT (session_id) DO NOTHING;

-- name: UpsertSessionActivity :exec
INSERT INTO session_activity (session_id, last_seen_at) VALUES ($1, $2)
ON CONFLICT (session_id) DO UPDATE SET last_seen_at = excluded.last_seen_at;

-- name: GetUnrevokedSessionByTokenHash :one
SELECT s.id, s.user_id, s.token_hash, s.created_at, s.expires_at, a.last_seen_at
FROM sessions s
LEFT JOIN session_activity a ON a.session_id = s.id
WHERE s.token_hash = $1
  AND NOT EXISTS (SELECT 1 FROM session_revocations r WHERE r.session_id = s.id);

-- RevokeSessionsOfUser revokes every session of a user that has not expired at revoked_at and
-- has no revocation yet.
-- name: RevokeSessionsOfUser :exec
INSERT INTO session_revocations (session_id, reason, revoked_at)
SELECT s.id, sqlc.arg(reason)::text, sqlc.arg(revoked_at)::timestamptz FROM sessions s
WHERE s.user_id = sqlc.arg(user_id) AND s.expires_at > sqlc.arg(revoked_at)::timestamptz
ON CONFLICT (session_id) DO NOTHING;
