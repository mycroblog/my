-- SPDX-License-Identifier: AGPL-3.0-only

-- Granting and revoking roles (FR-720).

-- ListRoles returns every role, highest position first.
-- name: ListRoles :many
SELECT id, key, position, created_at FROM roles ORDER BY position DESC, key;

-- ListAllRolePermissions returns the permissions of every role.
-- name: ListAllRolePermissions :many
SELECT role_id, permission FROM role_permissions ORDER BY role_id, permission;

-- GetUserRoleGrantID returns the grant through which a user holds a role now.
-- name: GetUserRoleGrantID :one
SELECT grant_id FROM user_roles WHERE user_id = ? AND role_id = ?;

-- CountRoleHolders counts the users who hold a role now.
-- name: CountRoleHolders :one
SELECT COUNT(*) FROM user_roles WHERE role_id = ?;

-- ListRoleHolderIDs returns the users who hold a role now, by ID.
-- name: ListRoleHolderIDs :many
SELECT user_id FROM user_roles WHERE role_id = ? ORDER BY user_id;

-- name: InsertUserRoleRevocation :exec
INSERT INTO user_role_revocations (grant_id, revoked_by_user_id, revoked_at) VALUES (?, ?, ?);

-- name: DeleteUserRoleByGrant :exec
DELETE FROM user_roles WHERE grant_id = ?;

-- Creating, changing, and deleting roles (FR-720).

-- name: UpdateRolePosition :exec
UPDATE roles SET position = ? WHERE id = ?;

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = ?;

-- name: DeleteRole :exec
DELETE FROM roles WHERE id = ?;

-- ListExistingRoleIDs returns, in ID order, the roles among ids that exist (the audit log). The ids are a JSON array.
-- name: ListExistingRoleIDs :many
SELECT id FROM roles WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;

-- Role badges (ADR-0070).

-- ListRoleBadges returns the badge of every role that has one.
-- name: ListRoleBadges :many
SELECT role_id, label, color, updated_at FROM role_badges ORDER BY role_id;

-- GetRoleBadge returns the badge of a role.
-- name: GetRoleBadge :one
SELECT role_id, label, color, updated_at FROM role_badges WHERE role_id = ?;

-- name: UpsertRoleBadge :exec
INSERT INTO role_badges (role_id, label, color, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (role_id) DO UPDATE SET label = excluded.label, color = excluded.color, updated_at = excluded.updated_at;

-- name: DeleteRoleBadge :exec
DELETE FROM role_badges WHERE role_id = ?;

-- DeleteRoleBadgeDisplays removes the choice of every user to show the badge of a role.
-- name: DeleteRoleBadgeDisplays :exec
DELETE FROM user_role_badge_displays WHERE role_id = ?;

-- name: InsertUserRoleBadgeDisplay :exec
INSERT INTO user_role_badge_displays (user_id, role_id, created_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, role_id) DO NOTHING;

-- name: DeleteUserRoleBadgeDisplay :exec
DELETE FROM user_role_badge_displays WHERE user_id = ? AND role_id = ?;

-- CountUserRoleBadgeDisplays counts the rows that show the badge of a role on a user's profile (0 or 1).
-- name: CountUserRoleBadgeDisplays :one
SELECT COUNT(*) FROM user_role_badge_displays WHERE user_id = ? AND role_id = ?;

-- ListUserRoleBadges returns the roles that a user holds and that have a badge, with whether the
-- user shows it, highest position first.
-- name: ListUserRoleBadges :many
SELECT r.id, b.label, b.color,
  (SELECT COUNT(*) FROM user_role_badge_displays d WHERE d.user_id = ur.user_id AND d.role_id = ur.role_id) AS displays
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
JOIN role_badges b ON b.role_id = ur.role_id
WHERE ur.user_id = ? ORDER BY r.position DESC, r.key;

-- ListDisplayedRoleBadges returns the badges that a user shows on their profile: of the roles they
-- hold now, that have a badge, and that they chose to show, highest position first.
-- name: ListDisplayedRoleBadges :many
SELECT r.id, b.label, b.color FROM user_role_badge_displays d
JOIN user_roles ur ON ur.user_id = d.user_id AND ur.role_id = d.role_id
JOIN roles r ON r.id = d.role_id
JOIN role_badges b ON b.role_id = d.role_id
WHERE d.user_id = ? ORDER BY r.position DESC, r.key;
