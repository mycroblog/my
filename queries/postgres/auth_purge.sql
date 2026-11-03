-- SPDX-License-Identifier: AGPL-3.0-only

-- ListExpiredTokenFamilies returns at most @batch_limit families whose every token, access and
-- refresh, expired before @expired_before: rotating or reusing a refresh token of such a family
-- can no longer revoke anything, so its rows may go.
-- name: ListExpiredTokenFamilies :many
SELECT DISTINCT t.family_id FROM oauth_access_tokens t
WHERE NOT EXISTS (
  SELECT 1 FROM oauth_access_tokens n
  WHERE n.family_id = t.family_id AND (n.expires_at >= @expired_before OR n.refresh_expires_at >= @expired_before)
)
ORDER BY t.family_id
LIMIT @batch_limit;

-- name: DeleteCodesOfTokenFamilies :exec
DELETE FROM oauth_authorization_codes WHERE code_hash IN (
  SELECT r.code_hash FROM oauth_authorization_code_redemptions r WHERE r.token_id = ANY(sqlc.arg(family_ids)::uuid[]));

-- name: DeleteCodeRedemptionsOfTokenFamilies :exec
DELETE FROM oauth_authorization_code_redemptions WHERE token_id = ANY(sqlc.arg(family_ids)::uuid[]);

-- name: DeleteRevocationsOfTokenFamilies :exec
DELETE FROM oauth_token_revocations WHERE token_id IN (
  SELECT t.id FROM oauth_access_tokens t WHERE t.family_id = ANY(sqlc.arg(family_ids)::uuid[]));

-- name: DeleteLastUsedOfTokenFamilies :exec
DELETE FROM oauth_token_family_last_used WHERE family_id = ANY(sqlc.arg(family_ids)::uuid[]);

-- name: DeleteTokensOfTokenFamilies :exec
DELETE FROM oauth_access_tokens WHERE family_id = ANY(sqlc.arg(family_ids)::uuid[]);

-- DeleteExpiredUnredeemedCodes removes at most @batch_limit codes that expired before
-- @expired_before without being exchanged, the oldest first. An exchanged code goes with its family.
-- name: DeleteExpiredUnredeemedCodes :execrows
DELETE FROM oauth_authorization_codes WHERE code_hash IN (
  SELECT c.code_hash FROM oauth_authorization_codes c
  WHERE c.expires_at < @expired_before
    AND NOT EXISTS (SELECT 1 FROM oauth_authorization_code_redemptions r WHERE r.code_hash = c.code_hash)
  ORDER BY c.expires_at, c.code_hash LIMIT @batch_limit
);

-- ListExpiredSessions returns at most @batch_limit sessions that expired before @expired_before,
-- the oldest first, except those that an authorization code still names: a token family started
-- from the code still lists and revokes its browser session.
-- name: ListExpiredSessions :many
SELECT s.id FROM sessions s
WHERE s.expires_at < @expired_before
  AND NOT EXISTS (SELECT 1 FROM oauth_authorization_codes c WHERE c.session_id = s.id)
ORDER BY s.expires_at, s.id LIMIT @batch_limit;

-- name: DeleteRevocationsOfSessions :exec
DELETE FROM session_revocations WHERE session_id = ANY(sqlc.arg(session_ids)::uuid[]);

-- name: DeleteActivityOfSessions :exec
DELETE FROM session_activity WHERE session_id = ANY(sqlc.arg(session_ids)::uuid[]);

-- name: DeleteSessions :exec
DELETE FROM sessions WHERE id = ANY(sqlc.arg(session_ids)::uuid[]);

-- ListExpiredUserTokens returns at most @batch_limit one-time tokens that expired before
-- @expired_before, the oldest first.
-- name: ListExpiredUserTokens :many
SELECT t.id FROM user_tokens t WHERE t.expires_at < @expired_before ORDER BY t.expires_at, t.id LIMIT @batch_limit;

-- name: DeleteRedemptionsOfUserTokens :exec
DELETE FROM user_token_redemptions WHERE token_id = ANY(sqlc.arg(token_ids)::uuid[]);

-- name: DeleteUserTokens :exec
DELETE FROM user_tokens WHERE id = ANY(sqlc.arg(token_ids)::uuid[]);
