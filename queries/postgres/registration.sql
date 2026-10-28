-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertEmailVerificationIfAbsent :exec
INSERT INTO user_email_verifications (email_address_id, verified_at) VALUES ($1, $2)
ON CONFLICT (email_address_id) DO NOTHING;

-- name: GetPrimaryEmail :one
SELECT e.id, e.user_id, e.address, e.address_normalized, e.added_at,
  EXISTS (SELECT 1 FROM user_email_verifications v WHERE v.email_address_id = e.id) AS verified
FROM user_primary_emails p
JOIN user_email_addresses e ON e.id = p.email_address_id
WHERE p.user_id = $1;

-- name: InsertRegistrationApplication :exec
INSERT INTO registration_applications (id, user_id, reason, invite_id, submitted_at) VALUES ($1, $2, $3, $4, $5);

-- name: GetRegistrationApplicationByUser :one
SELECT id, user_id, reason, invite_id, submitted_at FROM registration_applications WHERE user_id = $1;

-- name: InsertRegistrationDecision :exec
INSERT INTO registration_decisions (id, application_id, decision, decided_by, decided_by_user_id, note, decided_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: UpsertRegistrationState :exec
INSERT INTO user_registration_states (user_id, state, since) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET state = excluded.state, since = excluded.since;

-- TransitionPendingRegistrationState moves a pending registration to a decided state and
-- affects no row when the registration is not pending.
-- name: TransitionPendingRegistrationState :execrows
UPDATE user_registration_states SET state = $1, since = $2 WHERE user_id = $3 AND state = 'pending';

-- name: GetRegistrationState :one
SELECT user_id, state, since FROM user_registration_states WHERE user_id = $1;

-- name: ListRegistrationsInState :many
SELECT s.user_id, s.since, a.id AS application_id, a.reason, a.invite_id, a.submitted_at,
  COALESCE(e.address, '') AS email,
  EXISTS (SELECT 1 FROM user_email_verifications v WHERE v.email_address_id = p.email_address_id) AS email_verified
FROM user_registration_states s
JOIN registration_applications a ON a.user_id = s.user_id
LEFT JOIN user_primary_emails p ON p.user_id = s.user_id
LEFT JOIN user_email_addresses e ON e.id = p.email_address_id
WHERE s.state = sqlc.arg(state)
  AND (sqlc.narg(after_user_id)::uuid IS NULL OR s.since > sqlc.arg(after_since)
    OR (s.since = sqlc.arg(after_since) AND s.user_id > sqlc.narg(after_user_id)::uuid))
ORDER BY s.since, s.user_id
LIMIT sqlc.arg(page_limit);

-- name: InsertInvite :exec
INSERT INTO invites (id, code, created_by_user_id, creator_kind, max_uses, expires_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetInviteByCode :one
SELECT id, code, created_by_user_id, creator_kind, max_uses, expires_at, created_at FROM invites WHERE code = $1;

-- name: GetInviteByID :one
SELECT id, code, created_by_user_id, creator_kind, max_uses, expires_at, created_at FROM invites WHERE id = $1;

-- name: GetInviteUsage :one
SELECT
  (SELECT COUNT(*) FROM invite_redemptions r WHERE r.invite_id = sqlc.arg(invite_id)) AS uses,
  EXISTS (SELECT 1 FROM invite_revocations v WHERE v.invite_id = sqlc.arg(invite_id)) AS revoked;

-- name: InsertInviteRedemption :exec
INSERT INTO invite_redemptions (invite_id, seq, user_id, redeemed_at) VALUES ($1, $2, $3, $4);

-- name: InsertInviteRevocationIfAbsent :exec
INSERT INTO invite_revocations (invite_id, revoked_by_user_id, revoked_at) VALUES ($1, $2, $3)
ON CONFLICT (invite_id) DO NOTHING;

-- name: ListInvites :many
SELECT i.id, i.code, i.created_by_user_id, i.creator_kind, i.max_uses, i.expires_at, i.created_at,
  (SELECT COUNT(*) FROM invite_redemptions r WHERE r.invite_id = i.id) AS uses,
  EXISTS (SELECT 1 FROM invite_revocations v WHERE v.invite_id = i.id) AS revoked
FROM invites i
WHERE sqlc.narg(before)::uuid IS NULL OR i.id < sqlc.narg(before)::uuid
ORDER BY i.id DESC
LIMIT sqlc.arg(page_limit);

-- ListMemberInvitesByCreator lists the invites a member created as a member (ADR-0108),
-- newest first.
-- name: ListMemberInvitesByCreator :many
SELECT i.id, i.code, i.created_by_user_id, i.creator_kind, i.max_uses, i.expires_at, i.created_at,
  (SELECT COUNT(*) FROM invite_redemptions r WHERE r.invite_id = i.id) AS uses,
  EXISTS (SELECT 1 FROM invite_revocations v WHERE v.invite_id = i.id) AS revoked
FROM invites i
WHERE i.created_by_user_id = sqlc.arg(creator) AND i.creator_kind = 'member'
  AND (sqlc.narg(before)::uuid IS NULL OR i.id < sqlc.narg(before)::uuid)
ORDER BY i.id DESC
LIMIT sqlc.arg(page_limit);

-- CountUsableMemberInvites counts the invites a member created as a member that are not
-- revoked, expired, or used up at now (ADR-0108).
-- name: CountUsableMemberInvites :one
SELECT COUNT(*) FROM invites i
WHERE i.created_by_user_id = sqlc.arg(creator) AND i.creator_kind = 'member'
  AND (i.expires_at IS NULL OR i.expires_at > sqlc.arg(now))
  AND NOT EXISTS (SELECT 1 FROM invite_revocations v WHERE v.invite_id = i.id)
  AND (i.max_uses IS NULL OR (SELECT COUNT(*) FROM invite_redemptions r WHERE r.invite_id = i.id) < i.max_uses);

-- name: InsertUserToken :exec
INSERT INTO user_tokens (id, user_id, purpose, email_address_id, unsubscribe_category, token_hash, expires_at, issued_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetUserTokenByHash :one
SELECT id, user_id, purpose, email_address_id, unsubscribe_category, token_hash, expires_at, issued_at FROM user_tokens WHERE token_hash = $1;

-- UserTokenRedeemed reports whether a user token was used.
-- name: UserTokenRedeemed :one
SELECT EXISTS (SELECT 1 FROM user_token_redemptions WHERE token_id = $1);

-- name: InsertUserTokenRedemption :exec
INSERT INTO user_token_redemptions (token_id, redeemed_at) VALUES ($1, $2);

-- CountRegistrationsInState counts the users whose registration is in a state (#338).
-- name: CountRegistrationsInState :one
SELECT COUNT(*) FROM user_registration_states WHERE state = $1;
