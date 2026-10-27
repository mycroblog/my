// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"slices"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Granting and revoking roles (FR-720).

// ListRoles implements identity.RoleRepository.
func (r *IdentityRepository) ListRoles(ctx context.Context) ([]appidentity.RoleWithPermissions, error) {
	q := r.db.reader()
	roles, err := q.ListRoles(ctx)
	if err != nil {
		return nil, wrap(err, "list roles")
	}
	perms, err := q.ListAllRolePermissions(ctx)
	if err != nil {
		return nil, wrap(err, "list role permissions")
	}
	badges, err := roleBadges(ctx, q)
	if err != nil {
		return nil, err
	}
	granted := map[string]map[identity.Permission]bool{}
	for _, p := range perms {
		if granted[p.RoleID] == nil {
			granted[p.RoleID] = map[identity.Permission]bool{}
		}
		granted[p.RoleID][identity.Permission(p.Permission)] = true
	}
	out := make([]appidentity.RoleWithPermissions, len(roles))
	for i, row := range roles {
		out[i] = appidentity.RoleWithPermissions{Role: roleFrom(row), Permissions: []identity.Permission{}, Badge: badges[row.ID]}
		for _, p := range identity.AllPermissions() {
			if granted[row.ID][p] {
				out[i].Permissions = append(out[i].Permissions, p)
			}
		}
	}
	return out, nil
}

// LockRole implements identity.RoleRepository with SELECT ... FOR UPDATE.
func (r *IdentityRepository) LockRole(ctx context.Context, tx uow.Tx, key string) (identity.Role, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.Role{}, err
	}
	row, err := q.GetRoleByKeyForUpdate(ctx, key)
	if err != nil {
		return identity.Role{}, notFound(err, appidentity.ErrNotFound, "role "+key)
	}
	return roleFrom(row), nil
}

// FindRoleGrantID implements identity.RoleRepository.
func (r *IdentityRepository) FindRoleGrantID(ctx context.Context, tx uow.Tx, userID, roleID string) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetUserRoleGrantID(ctx, postgresgen.GetUserRoleGrantIDParams{UserID: userID, RoleID: roleID})
	if err != nil {
		return "", notFound(err, appidentity.ErrNotFound, "role "+roleID+" of user "+userID)
	}
	return id, nil
}

// CountRoleHolders implements identity.RoleRepository.
func (r *IdentityRepository) CountRoleHolders(ctx context.Context, tx uow.Tx, roleID string) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountRoleHolders(ctx, roleID)
	return n, wrap(err, "count role holders")
}

// InsertRoleRevocation implements identity.RoleRepository.
func (r *IdentityRepository) InsertRoleRevocation(ctx context.Context, tx uow.Tx, rev identity.RoleRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserRoleRevocation(ctx, postgresgen.InsertUserRoleRevocationParams{
		GrantID: rev.GrantID, RevokedByUserID: toNullString(rev.RevokedByUserID), RevokedAt: rev.RevokedAt,
	})
	if err != nil {
		return wrap(err, "insert role revocation")
	}
	return wrap(q.DeleteUserRoleByGrant(ctx, rev.GrantID), "delete user role")
}

// ListRoleHolderIDs implements identity.RoleRepository.
func (r *IdentityRepository) ListRoleHolderIDs(ctx context.Context, roleID string) ([]string, error) {
	ids, err := r.db.reader().ListRoleHolderIDs(ctx, roleID)
	return ids, wrap(err, "list role holders")
}

// Creating, changing, and deleting roles (FR-720).

// RolePermissions implements identity.RoleRepository.
func (r *IdentityRepository) RolePermissions(ctx context.Context, tx uow.Tx, roleID string) ([]identity.Permission, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRolePermissions(ctx, roleID)
	if err != nil {
		return nil, wrap(err, "list role permissions")
	}
	out := []identity.Permission{}
	for _, p := range identity.AllPermissions() {
		if slices.Contains(rows, string(p)) {
			out = append(out, p)
		}
	}
	return out, nil
}

// SetRolePermissions implements identity.RoleRepository.
func (r *IdentityRepository) SetRolePermissions(ctx context.Context, tx uow.Tx, roleID string, perms []identity.Permission) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteRolePermissions(ctx, roleID); err != nil {
		return wrap(err, "delete role permissions")
	}
	for _, p := range perms {
		err := q.InsertRolePermissionIfAbsent(ctx, postgresgen.InsertRolePermissionIfAbsentParams{RoleID: roleID, Permission: string(p)})
		if err != nil {
			return wrap(err, "insert role permission")
		}
	}
	return nil
}

// UpdateRolePosition implements identity.RoleRepository.
func (r *IdentityRepository) UpdateRolePosition(ctx context.Context, tx uow.Tx, roleID string, position int) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	pos, err := toInt32(position, "role position")
	if err != nil {
		return err
	}
	return wrap(q.UpdateRolePosition(ctx, postgresgen.UpdateRolePositionParams{ID: roleID, Position: pos}), "update role position")
}

// DeleteRole implements identity.RoleRepository.
func (r *IdentityRepository) DeleteRole(ctx context.Context, tx uow.Tx, roleID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteRolePermissions(ctx, roleID); err != nil {
		return wrap(err, "delete role permissions")
	}
	if err := r.DeleteRoleBadge(ctx, tx, roleID); err != nil {
		return err
	}
	return wrap(q.DeleteRole(ctx, roleID), "delete role")
}

// ExistingRoleIDs implements identity.RoleRepository.
func (r *IdentityRepository) ExistingRoleIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := r.db.reader().ListExistingRoleIDs(ctx, ids)
	if err != nil {
		return nil, wrap(err, "list existing roles")
	}
	return out, nil
}
