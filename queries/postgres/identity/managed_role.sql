-- SPDX-License-Identifier: AGPL-3.0-only

-- The roles that another module manages (ADR-0072 section 5).

-- name: ListManagedRoles :many
SELECT role_id, manager, created_at FROM managed_roles ORDER BY role_id;

-- name: InsertManagedRole :exec
INSERT INTO managed_roles (role_id, manager, created_at) VALUES ($1, $2, $3);

-- name: DeleteManagedRole :exec
DELETE FROM managed_roles WHERE role_id = $1;

-- GetRoleByIDForUpdate reads and locks a role by ID inside the transaction that changes it.
-- name: GetRoleByIDForUpdate :one
SELECT id, key, position, created_at FROM roles WHERE id = $1
FOR UPDATE;
