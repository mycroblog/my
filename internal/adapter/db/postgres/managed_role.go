// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The roles that another module manages (ADR-0072 §5).

// ListManagedRoles implements identity.ManagedRoleRepository.
func (r *IdentityRepository) ListManagedRoles(ctx context.Context, tx uow.Tx) ([]identity.ManagedRole, error) {
	q := r.db.reader()
	if tx != nil {
		var err error
		if q, err = r.db.queries(tx); err != nil {
			return nil, err
		}
	}
	rows, err := q.ListManagedRoles(ctx)
	if err != nil {
		return nil, wrap(err, "list managed roles")
	}
	out := make([]identity.ManagedRole, len(rows))
	for i, row := range rows {
		out[i] = identity.ManagedRole{RoleID: row.RoleID, Manager: identity.RoleManager(row.Manager), CreatedAt: utc(row.CreatedAt)}
	}
	return out, nil
}

// InsertManagedRole implements identity.ManagedRoleRepository.
func (r *IdentityRepository) InsertManagedRole(ctx context.Context, tx uow.Tx, m identity.ManagedRole) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertManagedRole(ctx, postgresgen.InsertManagedRoleParams{
		RoleID: m.RoleID, Manager: string(m.Manager), CreatedAt: m.CreatedAt,
	}), "insert managed role")
}

// DeleteManagedRole implements identity.ManagedRoleRepository.
func (r *IdentityRepository) DeleteManagedRole(ctx context.Context, tx uow.Tx, roleID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteManagedRole(ctx, roleID), "delete managed role")
}

// LockRoleByID implements identity.ManagedRoleRepository with SELECT ... FOR UPDATE.
func (r *IdentityRepository) LockRoleByID(ctx context.Context, tx uow.Tx, id string) (identity.Role, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.Role{}, err
	}
	row, err := q.GetRoleByIDForUpdate(ctx, id)
	if err != nil {
		return identity.Role{}, notFound(err, appidentity.ErrNotFound, "role "+id)
	}
	return roleFrom(row), nil
}

// ListRolesTx implements identity.ManagedRoleRepository.
func (r *IdentityRepository) ListRolesTx(ctx context.Context, tx uow.Tx) ([]appidentity.RoleWithPermissions, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&IdentityRepository{db: db}).ListRoles(ctx)
}
