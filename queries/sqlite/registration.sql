-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertEmailVerificationIfAbsent :exec
INSERT INTO user_email_verifications (email_address_id, verified_at) VALUES (?, ?)
ON CONFLICT (email_address_id) DO NOTHING;

-- name: GetPrimaryEmail :one
SELECT e.id, e.user_id, e.address, e.address_normalized, e.added_at,
  EXISTS (SELECT 1 FROM user_email_verifications v WHERE v.email_address_id = e.id) AS verified
FROM user_primary_emails p
JOIN user_email_addresses e ON e.id = p.email_address_id
WHERE p.user_id = ?;

-- name: InsertRegistrationApplication :exec
INSERT INTO registration_applications (id, user_id, reason, invite_id, submitted_at) VALUES (?, ?, ?, ?, ?);

-- name: GetRegistrationApplicationByUser :one
SELECT id, user_id, reason, invite_id, submitted_at FROM registration_applications WHERE user_id = ?;

-- name: InsertRegistrationDecision :exec
INSERT INTO registration_decisions (id, application_id, decision, decided_by, decided_by_user_id, note, decided_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpsertRegistrationState :exec
INSERT INTO user_registration_states (user_id, state, since) VALUES (?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET state = excluded.state, since = excluded.since;

-- TransitionPendingRegistrationState moves a pending registration to a decided state and
-- affects no row when the registration is not pending.
-- name: TransitionPendingRegistrationState :execrows
UPDATE user_registration_states SET state = ?, since = ? WHERE user_id = ? AND state = 'pending';

-- name: GetRegistrationState :one
SELECT user_id, state, since FROM user_registration_states WHERE user_id = ?;

-- name: ListRegistrationsInState :many
SELECT s.user_id, s.since, a.id AS application_id, a.reason, a.invite_id, a.submitted_at,
  COALESCE(e.address, '') AS email,
  EXISTS (SELECT 1 FROM user_email_verifications v WHERE v.email_address_id = p.email_address_id) AS email_verified
FROM user_registration_states s
JOIN registration_applications a ON a.user_id = s.user_id
LEFT JOIN user_primary_emails p ON p.user_id = s.user_id
LEFT JOIN user_email_addresses e ON e.id = p.email_address_id
WHERE s.state = sqlc.arg(state)
  AND (CAST(sqlc.arg(after_user_id) AS TEXT) = '' OR s.since > sqlc.arg(after_since)
    OR (s.since = sqlc.arg(after_since) AND s.user_id > sqlc.arg(after_user_id)))
ORDER BY s.since, s.user_id
LIMIT sqlc.arg(page_limit);

-- name: InsertInvite :exec
INSERT INTO invites (id, code, created_by_user_id, max_uses, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetInviteByCode :one
SELECT id, code, created_by_user_id, max_uses, expires_at, created_at FROM invites WHERE code = ?;

-- name: GetInviteByID :one
SELECT id, code, created_by_user_id, max_uses, expires_at, created_at FROM invites WHERE id = ?;

-- name: GetInviteUsage :one
SELECT
  (SELECT COUNT(*) FROM invite_redemptions r WHERE r.invite_id = sqlc.arg(invite_id)) AS uses,
  EXISTS (SELECT 1 FROM invite_revocations v WHERE v.invite_id = sqlc.arg(invite_id)) AS revoked;

-- name: InsertInviteRedemption :exec
INSERT INTO invite_redemptions (invite_id, seq, user_id, redeemed_at) VALUES (?, ?, ?, ?);

-- name: InsertInviteRevocationIfAbsent :exec
INSERT INTO invite_revocations (invite_id, revoked_by_user_id, revoked_at) VALUES (?, ?, ?)
ON CONFLICT (invite_id) DO NOTHING;

-- name: ListInvites :many
SELECT i.id, i.code, i.created_by_user_id, i.max_uses, i.expires_at, i.created_at,
  (SELECT COUNT(*) FROM invite_redemptions r WHERE r.invite_id = i.id) AS uses,
  EXISTS (SELECT 1 FROM invite_revocations v WHERE v.invite_id = i.id) AS revoked
FROM invites i
WHERE CAST(sqlc.arg(before) AS TEXT) = '' OR i.id < sqlc.arg(before)
ORDER BY i.id DESC
LIMIT sqlc.arg(page_limit);

-- name: InsertUserToken :exec
INSERT INTO user_tokens (id, user_id, purpose, email_address_id, unsubscribe_category, token_hash, expires_at, issued_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetUserTokenByHash :one
SELECT id, user_id, purpose, email_address_id, unsubscribe_category, token_hash, expires_at, issued_at FROM user_tokens WHERE token_hash = ?;

-- UserTokenRedeemed reports whether a user token was used.
-- name: UserTokenRedeemed :one
SELECT EXISTS (SELECT 1 FROM user_token_redemptions WHERE token_id = ?);

-- name: InsertUserTokenRedemption :exec
INSERT INTO user_token_redemptions (token_id, redeemed_at) VALUES (?, ?);

-- CountRegistrationsInState counts the users whose registration is in a state (#338).
-- name: CountRegistrationsInState :one
SELECT COUNT(*) FROM user_registration_states WHERE state = ?;
