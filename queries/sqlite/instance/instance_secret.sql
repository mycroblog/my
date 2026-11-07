-- SPDX-License-Identifier: AGPL-3.0-only

-- InsertInstanceSecret inserts a secret unless one of the same name exists.
-- name: InsertInstanceSecret :execrows
INSERT INTO instance_secrets (id, name, sealed_secret, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT (name) DO NOTHING;

-- GetInstanceSecret returns the secret of a name.
-- name: GetInstanceSecret :one
SELECT id, name, sealed_secret, created_at FROM instance_secrets WHERE name = ?;
