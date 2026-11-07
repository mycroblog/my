-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertSessionOrigin :exec
INSERT INTO session_origins (session_id, user_id, ip, user_agent, created_at)
VALUES (@session_id, @user_id, @ip, @user_agent, @created_at);

-- name: InsertRegistrationOrigin :exec
INSERT INTO registration_origins (user_id, ip, recorded_at) VALUES (@user_id, @ip, @recorded_at);

-- name: GetSessionOrigin :one
SELECT session_id, user_id, ip, user_agent, created_at FROM session_origins WHERE session_id = sqlc.arg(session_id)::uuid;

-- name: GetRegistrationOrigin :one
SELECT user_id, ip, recorded_at FROM registration_origins WHERE user_id = sqlc.arg(user_id)::uuid;

-- DeleteSessionOriginsBefore removes at most @batch_limit session origins created before
-- @created_before, except those of the users in @except_user_ids, the oldest first.
-- name: DeleteSessionOriginsBefore :execrows
DELETE FROM session_origins
WHERE session_id IN (
  SELECT o.session_id FROM session_origins o WHERE o.created_at < @created_before
    AND NOT (o.user_id = ANY(sqlc.arg(except_user_ids)::uuid[]))
  ORDER BY o.created_at, o.session_id LIMIT @batch_limit
);

-- DeleteRegistrationOriginsBefore removes at most @batch_limit registration origins recorded
-- before @recorded_before, except those of the users in sqlc.arg(except_user_ids)::uuid, the oldest first.
-- name: DeleteRegistrationOriginsBefore :execrows
DELETE FROM registration_origins
WHERE user_id IN (
  SELECT o.user_id FROM registration_origins o WHERE o.recorded_at < @recorded_before
    AND NOT (o.user_id = ANY(sqlc.arg(except_user_ids)::uuid[]))
  ORDER BY o.recorded_at, o.user_id LIMIT @batch_limit
);
