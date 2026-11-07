-- SPDX-License-Identifier: AGPL-3.0-only

-- Passkeys (data-model 4.1, FR-004).

-- name: InsertPasskeyCredential :exec
INSERT INTO passkey_credentials (id, user_id, credential_id, public_key, backup_eligible, name, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListActivePasskeyCredentials :many
SELECT c.id, c.user_id, c.credential_id, c.public_key, c.backup_eligible, c.name, c.created_at
FROM passkey_credentials c
WHERE c.user_id = $1 AND NOT EXISTS (SELECT 1 FROM passkey_credential_revocations r WHERE r.passkey_id = c.id)
ORDER BY c.id;

-- name: GetPasskeyCredential :one
-- The passkey with a credential ID and whether it was revoked.
SELECT c.id, c.user_id, c.credential_id, c.public_key, c.backup_eligible, c.name, c.created_at,
  EXISTS (SELECT 1 FROM passkey_credential_revocations r WHERE r.passkey_id = c.id) AS revoked
FROM passkey_credentials c WHERE c.credential_id = $1;

-- name: InsertPasskeyCredentialRevocation :exec
INSERT INTO passkey_credential_revocations (passkey_id, revoked_at) VALUES ($1, $2)
ON CONFLICT (passkey_id) DO NOTHING;
