-- SPDX-License-Identifier: AGPL-3.0-only

-- The OAuth clients that users register for themselves (ADR-0090 sections 1 and 2): their
-- revisions, secret rotations, and the limit on how many a user has.

-- name: InsertOAuthClient :exec
INSERT INTO oauth_clients (id, client_id, client_secret_hash, name, website, scope, registration, owner_user_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- ListOAuthClientsOfOwner returns the clients a user registered that are not deleted, oldest first.
-- name: ListOAuthClientsOfOwner :many
SELECT c.id, c.client_id, c.client_secret_hash, c.name, c.website, c.scope, c.registration, c.owner_user_id, c.created_at
FROM oauth_clients c
WHERE c.owner_user_id = sqlc.arg(owner_user_id)
  AND NOT EXISTS (SELECT 1 FROM oauth_client_deletions x WHERE x.oauth_client_id = c.id)
ORDER BY c.id;

-- CountOAuthClientsOfOwner counts the clients a user registered that are not deleted.
-- name: CountOAuthClientsOfOwner :one
SELECT COUNT(*) FROM oauth_clients c
WHERE c.owner_user_id = sqlc.arg(owner_user_id)
  AND NOT EXISTS (SELECT 1 FROM oauth_client_deletions x WHERE x.oauth_client_id = c.id);

-- name: InsertOAuthClientRevision :exec
INSERT INTO oauth_client_revisions (id, oauth_client_id, name, website, scope, revised_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetLatestOAuthClientRevision :one
SELECT id, oauth_client_id, name, website, scope, revised_at FROM oauth_client_revisions
WHERE oauth_client_id = ? ORDER BY id DESC LIMIT 1;

-- name: InsertOAuthClientSecretRotation :exec
INSERT INTO oauth_client_secret_rotations (id, oauth_client_id, secret_hash, rotated_at) VALUES (?, ?, ?, ?);

-- name: GetLatestOAuthClientSecretRotation :one
SELECT id, oauth_client_id, secret_hash, rotated_at FROM oauth_client_secret_rotations
WHERE oauth_client_id = ? ORDER BY id DESC LIMIT 1;

-- RevokeTokensOfOwnedClients revokes every token of the clients a user registered whose access
-- or refresh token has not expired at revoked_at and that has no revocation yet.
-- name: RevokeTokensOfOwnedClients :exec
INSERT INTO oauth_token_revocations (token_id, reason, revoked_at)
SELECT t.id, sqlc.arg(reason), sqlc.arg(revoked_at) FROM oauth_access_tokens t
JOIN oauth_clients c ON c.id = t.oauth_client_id
WHERE c.owner_user_id = sqlc.arg(owner_user_id)
  AND (t.expires_at > sqlc.arg(revoked_at) OR t.refresh_expires_at > sqlc.arg(revoked_at))
ON CONFLICT (token_id) DO NOTHING;
