// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The roles that another module manages (ADR-0072 §5).

// ListManagedRoles implements identity.ManagedRoleRepository.
func (r *Repository) ListManagedRoles(ctx context.Context, tx uow.Tx) ([]identity.ManagedRole, error) {
	db := r.db
	if tx != nil {
		var err error
		if db, err = r.db.in(tx); err != nil {
			return nil, err
		}
	}
	rows, err := db.reader().ListManagedRoles(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list managed roles")
	}
	out := make([]identity.ManagedRole, len(rows))
	for i, row := range rows {
		out[i] = identity.ManagedRole{RoleID: row.RoleID, Manager: identity.RoleManager(row.Manager), CreatedAt: sqlite.FromMillis(row.CreatedAt)}
	}
	return out, nil
}

// InsertManagedRole implements identity.ManagedRoleRepository.
func (r *Repository) InsertManagedRole(ctx context.Context, tx uow.Tx, m identity.ManagedRole) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertManagedRole(ctx, identitygen.InsertManagedRoleParams{
		RoleID: m.RoleID, Manager: string(m.Manager), CreatedAt: sqlite.ToMillis(m.CreatedAt),
	}), "insert managed role")
}

// DeleteManagedRole implements identity.ManagedRoleRepository.
func (r *Repository) DeleteManagedRole(ctx context.Context, tx uow.Tx, roleID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteManagedRole(ctx, roleID), "delete managed role")
}

// LockRoleByID implements identity.ManagedRoleRepository. SQLite serializes write transactions,
// so it needs no lock.
func (r *Repository) LockRoleByID(ctx context.Context, tx uow.Tx, id string) (identity.Role, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.Role{}, err
	}
	row, err := q.GetRoleByID(ctx, id)
	if err != nil {
		return identity.Role{}, sqlite.NotFound(err, appidentity.ErrNotFound, "role "+id)
	}
	return roleFrom(row), nil
}

// ListRolesTx implements identity.ManagedRoleRepository.
func (r *Repository) ListRolesTx(ctx context.Context, tx uow.Tx) ([]appidentity.RoleWithPermissions, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListRoles(ctx)
}
