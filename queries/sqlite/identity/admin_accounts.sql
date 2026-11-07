-- SPDX-License-Identifier: AGPL-3.0-only

-- The account search of the moderators (FR-714, #344). Each query reads the tables of one module
-- and returns IDs newest first in keyset pages; internal/app/identity.AccountAdmin combines them.

-- ListUserIDsByEmailDomain returns the users whose primary address ends with a suffix ('@' and
-- the domain, lowercase) newest first, before the user before (empty for the first page).
-- name: ListUserIDsByEmailDomain :many
SELECT user_id FROM user_primary_emails
WHERE substr(address_normalized, -length(CAST(sqlc.arg(suffix) AS TEXT))) = CAST(sqlc.arg(suffix) AS TEXT)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR user_id < sqlc.arg(before))
ORDER BY user_id DESC
LIMIT sqlc.arg(lim);

-- ListUserIDsByIP returns the users who registered or signed in from an IP address newest
-- first, before the user before (empty for the first page).
-- name: ListUserIDsByIP :many
SELECT u.user_id FROM (
  SELECT o.user_id FROM session_origins o WHERE o.ip = CAST(sqlc.arg(ip) AS TEXT)
  UNION
  SELECT r.user_id FROM registration_origins r WHERE r.ip = CAST(sqlc.arg(ip) AS TEXT)
) AS u
WHERE CAST(sqlc.arg(before) AS TEXT) = '' OR u.user_id < sqlc.arg(before)
ORDER BY u.user_id DESC
LIMIT sqlc.arg(lim);

-- ListUserIDsInRegistrationState returns the users whose registration is in a state newest
-- first, before the user before (empty for the first page).
-- name: ListUserIDsInRegistrationState :many
SELECT user_id FROM user_registration_states
WHERE state = sqlc.arg(state) AND (CAST(sqlc.arg(before) AS TEXT) = '' OR user_id < sqlc.arg(before))
ORDER BY user_id DESC
LIMIT sqlc.arg(lim);

-- ListSessionOriginsOfUser returns the newest origins of a user's sessions.
-- name: ListSessionOriginsOfUser :many
SELECT session_id, user_id, ip, user_agent, created_at FROM session_origins
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, session_id DESC
LIMIT sqlc.arg(lim);
