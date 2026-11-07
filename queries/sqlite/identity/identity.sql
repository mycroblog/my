-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertUser :exec
INSERT INTO users (id, created_at) VALUES (?, ?);

-- name: GetUser :one
SELECT id, created_at FROM users WHERE id = ?;

-- name: InsertPasswordCredential :exec
INSERT INTO password_credentials (id, user_id, password_hash, set_at) VALUES (?, ?, ?, ?);

-- GetCurrentPasswordCredential returns the newest password credential of a user and when it
-- was removed, if it was.
-- name: GetCurrentPasswordCredential :one
SELECT c.id, c.user_id, c.password_hash, c.set_at, r.removed_at
FROM password_credentials c
LEFT JOIN password_credential_removals r ON r.password_credential_id = c.id
WHERE c.user_id = ? ORDER BY c.id DESC LIMIT 1;

-- name: InsertPasswordCredentialRemoval :exec
INSERT INTO password_credential_removals (password_credential_id, removed_at) VALUES (?, ?)
ON CONFLICT (password_credential_id) DO NOTHING;

-- name: InsertRoleIfAbsent :exec
INSERT INTO roles (id, key, position, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT (key) DO NOTHING;

-- name: GetRoleByKey :one
SELECT id, key, position, created_at FROM roles WHERE key = ?;

-- name: InsertRolePermissionIfAbsent :exec
INSERT INTO role_permissions (role_id, permission) VALUES (?, ?)
ON CONFLICT (role_id, permission) DO NOTHING;

-- name: ListRolePermissions :many
SELECT permission FROM role_permissions WHERE role_id = ? ORDER BY permission;

-- name: InsertEmailAddress :exec
INSERT INTO user_email_addresses (id, user_id, address, address_normalized, added_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertPrimaryEmail :exec
INSERT INTO user_primary_emails (user_id, email_address_id, address_normalized) VALUES (?, ?, ?);

-- name: GetUserIDByPrimaryEmail :one
SELECT user_id FROM user_primary_emails WHERE address_normalized = ?;

-- name: InsertUserRoleGrant :exec
INSERT INTO user_role_grants (id, user_id, role_id, granted_by_user_id, granted_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertUserRole :exec
INSERT INTO user_roles (user_id, role_id, grant_id) VALUES (?, ?, ?);

-- name: ListUserRoles :many
SELECT r.id, r.key, r.position, r.created_at FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ? ORDER BY r.position DESC, r.key;

-- Moderator notifications (#338) --------------------------------------------------------------

-- ListUserIDsWithPermission returns the users whose current roles grant a permission, by ID.
-- name: ListUserIDsWithPermission :many
SELECT DISTINCT ur.user_id FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
WHERE rp.permission = ?
ORDER BY ur.user_id;
