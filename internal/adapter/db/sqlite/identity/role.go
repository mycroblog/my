// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"slices"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Granting and revoking roles (FR-720).

// ListRoles implements identity.RoleRepository.
func (r *Repository) ListRoles(ctx context.Context) ([]appidentity.RoleWithPermissions, error) {
	q := r.db.reader()
	roles, err := q.ListRoles(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list roles")
	}
	perms, err := q.ListAllRolePermissions(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list role permissions")
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

// LockRole implements identity.RoleRepository. SQLite serializes write transactions, so it
// needs no lock.
func (r *Repository) LockRole(ctx context.Context, tx uow.Tx, key string) (identity.Role, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.Role{}, err
	}
	row, err := q.GetRoleByKey(ctx, key)
	if err != nil {
		return identity.Role{}, sqlite.NotFound(err, appidentity.ErrNotFound, "role "+key)
	}
	return roleFrom(row), nil
}

// FindRoleGrantID implements identity.RoleRepository.
func (r *Repository) FindRoleGrantID(ctx context.Context, tx uow.Tx, userID, roleID string) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	id, err := q.GetUserRoleGrantID(ctx, identitygen.GetUserRoleGrantIDParams{UserID: userID, RoleID: roleID})
	if err != nil {
		return "", sqlite.NotFound(err, appidentity.ErrNotFound, "role "+roleID+" of user "+userID)
	}
	return id, nil
}

// CountRoleHolders implements identity.RoleRepository.
func (r *Repository) CountRoleHolders(ctx context.Context, tx uow.Tx, roleID string) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountRoleHolders(ctx, roleID)
	return n, sqlite.Wrap(err, "count role holders")
}

// InsertRoleRevocation implements identity.RoleRepository.
func (r *Repository) InsertRoleRevocation(ctx context.Context, tx uow.Tx, rev identity.RoleRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserRoleRevocation(ctx, identitygen.InsertUserRoleRevocationParams{
		GrantID: rev.GrantID, RevokedByUserID: sqlite.ToNullString(rev.RevokedByUserID), RevokedAt: sqlite.ToMillis(rev.RevokedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert role revocation")
	}
	return sqlite.Wrap(q.DeleteUserRoleByGrant(ctx, rev.GrantID), "delete user role")
}

// ListRoleHolderIDs implements identity.RoleRepository.
func (r *Repository) ListRoleHolderIDs(ctx context.Context, roleID string) ([]string, error) {
	ids, err := r.db.reader().ListRoleHolderIDs(ctx, roleID)
	return ids, sqlite.Wrap(err, "list role holders")
}

// ListRoleHolderIDsTx implements identity.RoleRepository.
func (r *Repository) ListRoleHolderIDsTx(ctx context.Context, tx uow.Tx, roleID string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListRoleHolderIDs(ctx, roleID)
	return ids, sqlite.Wrap(err, "list role holders")
}

// Creating, changing, and deleting roles (FR-720).

// RolePermissions implements identity.RoleRepository.
func (r *Repository) RolePermissions(ctx context.Context, tx uow.Tx, roleID string) ([]identity.Permission, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRolePermissions(ctx, roleID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list role permissions")
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
func (r *Repository) SetRolePermissions(ctx context.Context, tx uow.Tx, roleID string, perms []identity.Permission) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteRolePermissions(ctx, roleID); err != nil {
		return sqlite.Wrap(err, "delete role permissions")
	}
	for _, p := range perms {
		err := q.InsertRolePermissionIfAbsent(ctx, identitygen.InsertRolePermissionIfAbsentParams{RoleID: roleID, Permission: string(p)})
		if err != nil {
			return sqlite.Wrap(err, "insert role permission")
		}
	}
	return nil
}

// UpdateRolePosition implements identity.RoleRepository.
func (r *Repository) UpdateRolePosition(ctx context.Context, tx uow.Tx, roleID string, position int) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpdateRolePosition(ctx, identitygen.UpdateRolePositionParams{ID: roleID, Position: int64(position)}), "update role position")
}

// DeleteRole implements identity.RoleRepository.
func (r *Repository) DeleteRole(ctx context.Context, tx uow.Tx, roleID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteRolePermissions(ctx, roleID); err != nil {
		return sqlite.Wrap(err, "delete role permissions")
	}
	if err := r.DeleteRoleBadge(ctx, tx, roleID); err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteRole(ctx, roleID), "delete role")
}

// ExistingRoleIDs implements identity.RoleRepository.
func (r *Repository) ExistingRoleIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListExistingRoleIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list existing roles")
	}
	return out, nil
}
