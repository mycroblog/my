-- SPDX-License-Identifier: AGPL-3.0-only

-- The roles that another module manages (ADR-0072 section 5).

-- name: ListManagedRoles :many
SELECT role_id, manager, created_at FROM managed_roles ORDER BY role_id;

-- name: InsertManagedRole :exec
INSERT INTO managed_roles (role_id, manager, created_at) VALUES (?, ?, ?);

-- name: DeleteManagedRole :exec
DELETE FROM managed_roles WHERE role_id = ?;

-- name: GetRoleByID :one
SELECT id, key, position, created_at FROM roles WHERE id = ?;
