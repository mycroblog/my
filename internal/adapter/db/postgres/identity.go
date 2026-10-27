// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// IdentityRepository implements identity.Repository on PostgreSQL.
type IdentityRepository struct {
	db *DB
}

var _ appidentity.Repository = (*IdentityRepository)(nil)

// IdentityRepository returns the identity module's repository on d.
func (d *DB) IdentityRepository() *IdentityRepository { return &IdentityRepository{db: d} }

// InsertUser implements identity.Repository.
func (r *IdentityRepository) InsertUser(ctx context.Context, tx uow.Tx, u identity.User) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertUser(ctx, postgresgen.InsertUserParams{ID: u.ID, CreatedAt: u.CreatedAt}), "insert user")
}

// InsertPasswordCredential implements identity.Repository.
func (r *IdentityRepository) InsertPasswordCredential(ctx context.Context, tx uow.Tx, c identity.PasswordCredential) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasswordCredential(ctx, postgresgen.InsertPasswordCredentialParams{
		ID: c.ID, UserID: c.UserID, PasswordHash: c.Hash, SetAt: c.SetAt,
	})
	return wrap(err, "insert password credential")
}

// EnsureRole implements identity.Repository.
func (r *IdentityRepository) EnsureRole(ctx context.Context, tx uow.Tx, role identity.Role, perms []identity.Permission) (identity.Role, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.Role{}, err
	}
	pos, err := toInt32(role.Position, "role position")
	if err != nil {
		return identity.Role{}, err
	}
	err = q.InsertRoleIfAbsent(ctx, postgresgen.InsertRoleIfAbsentParams{
		ID: role.ID, Key: role.Key, Position: pos, CreatedAt: role.CreatedAt,
	})
	if err != nil {
		return identity.Role{}, wrap(err, "insert role")
	}
	row, err := q.GetRoleByKey(ctx, role.Key)
	if err != nil {
		return identity.Role{}, wrap(err, "get role")
	}
	if row.ID != role.ID {
		return roleFrom(row), nil
	}
	for _, p := range perms {
		err := q.InsertRolePermissionIfAbsent(ctx, postgresgen.InsertRolePermissionIfAbsentParams{RoleID: row.ID, Permission: string(p)})
		if err != nil {
			return identity.Role{}, wrap(err, "insert role permission")
		}
	}
	return roleFrom(row), nil
}

func roleFrom(row postgresgen.Role) identity.Role {
	return identity.Role{ID: row.ID, Key: row.Key, Position: int(row.Position), CreatedAt: utc(row.CreatedAt)}
}

// FindUser implements identity.Repository.
func (r *IdentityRepository) FindUser(ctx context.Context, id string) (identity.User, error) {
	row, err := r.db.reader().GetUser(ctx, id)
	if err != nil {
		return identity.User{}, notFound(err, appidentity.ErrNotFound, "user "+id)
	}
	return identity.User{ID: row.ID, CreatedAt: utc(row.CreatedAt)}, nil
}

// FindCurrentPasswordCredential implements identity.Repository.
func (r *IdentityRepository) FindCurrentPasswordCredential(ctx context.Context, userID string) (identity.PasswordCredential, error) {
	c, removedAt, err := r.newestPasswordCredential(ctx, userID)
	if err == nil && removedAt != nil {
		return identity.PasswordCredential{}, fmt.Errorf("password of %s: removed: %w", userID, appidentity.ErrNotFound)
	}
	return c, err
}

// LastPasswordChange implements identity.Repository.
func (r *IdentityRepository) LastPasswordChange(ctx context.Context, userID string) (time.Time, error) {
	c, removedAt, err := r.newestPasswordCredential(ctx, userID)
	switch {
	case err != nil:
		return time.Time{}, err
	case removedAt != nil:
		return *removedAt, nil
	}
	return c.SetAt, nil
}

// newestPasswordCredential returns the newest password credential of a user and when it was
// removed, nil when it was not.
func (r *IdentityRepository) newestPasswordCredential(ctx context.Context, userID string) (identity.PasswordCredential, *time.Time, error) {
	row, err := r.db.reader().GetCurrentPasswordCredential(ctx, userID)
	if err != nil {
		return identity.PasswordCredential{}, nil, notFound(err, appidentity.ErrNotFound, "password of "+userID)
	}
	return identity.PasswordCredential{ID: row.ID, UserID: row.UserID, Hash: row.PasswordHash, SetAt: utc(row.SetAt)}, utcPtr(row.RemovedAt), nil
}

// InsertPasswordCredentialRemoval implements identity.Repository.
func (r *IdentityRepository) InsertPasswordCredentialRemoval(ctx context.Context, tx uow.Tx, rem identity.PasswordCredentialRemoval) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasswordCredentialRemoval(ctx, postgresgen.InsertPasswordCredentialRemovalParams{
		PasswordCredentialID: rem.PasswordCredentialID, RemovedAt: rem.RemovedAt,
	})
	return wrap(err, "insert password credential removal")
}

// LockSignInMethods implements identity.Repository with a transaction-scoped advisory lock on
// the user, so that removals of the password and of passkeys serialize under READ COMMITTED.
func (r *IdentityRepository) LockSignInMethods(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.LockUserSignInMethods(ctx, "identity:sign_in_methods:"+userID), "lock the sign-in methods")
}

// FindRole implements identity.Repository.
func (r *IdentityRepository) FindRole(ctx context.Context, key string) (identity.Role, []identity.Permission, error) {
	q := r.db.reader()
	row, err := q.GetRoleByKey(ctx, key)
	if err != nil {
		return identity.Role{}, nil, notFound(err, appidentity.ErrNotFound, "role "+key)
	}
	perms, err := q.ListRolePermissions(ctx, row.ID)
	if err != nil {
		return identity.Role{}, nil, wrap(err, "list role permissions")
	}
	out := make([]identity.Permission, len(perms))
	for i, p := range perms {
		out[i] = identity.Permission(p)
	}
	return roleFrom(row), out, nil
}

// InsertEmailAddress implements identity.Repository.
func (r *IdentityRepository) InsertEmailAddress(ctx context.Context, tx uow.Tx, e identity.EmailAddress) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertEmailAddress(ctx, postgresgen.InsertEmailAddressParams{
		ID: e.ID, UserID: e.UserID, Address: e.Address, AddressNormalized: e.AddressNormalized, AddedAt: e.AddedAt,
	})
	return wrap(err, "insert email address")
}

// InsertPrimaryEmail implements identity.Repository.
func (r *IdentityRepository) InsertPrimaryEmail(ctx context.Context, tx uow.Tx, e identity.EmailAddress) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPrimaryEmail(ctx, postgresgen.InsertPrimaryEmailParams{
		UserID: e.UserID, EmailAddressID: e.ID, AddressNormalized: e.AddressNormalized,
	})
	if isUniqueViolation(err, "user_primary_emails_address_normalized") {
		return fmt.Errorf("insert primary email of user %s: %w", e.UserID, appidentity.ErrEmailTaken)
	}
	return wrap(err, "insert primary email")
}

// InsertRoleGrant implements identity.Repository.
func (r *IdentityRepository) InsertRoleGrant(ctx context.Context, tx uow.Tx, g identity.RoleGrant) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserRoleGrant(ctx, postgresgen.InsertUserRoleGrantParams{
		ID: g.ID, UserID: g.UserID, RoleID: g.RoleID, GrantedByUserID: toNullString(g.GrantedByUserID), GrantedAt: g.GrantedAt,
	})
	if err != nil {
		return wrap(err, "insert role grant")
	}
	return wrap(q.InsertUserRole(ctx, postgresgen.InsertUserRoleParams{UserID: g.UserID, RoleID: g.RoleID, GrantID: g.ID}), "insert user role")
}

// FindUserIDByEmail implements identity.Repository.
func (r *IdentityRepository) FindUserIDByEmail(ctx context.Context, normalized string) (string, error) {
	id, err := r.db.reader().GetUserIDByPrimaryEmail(ctx, normalized)
	if err != nil {
		return "", notFound(err, appidentity.ErrNotFound, "user by email")
	}
	return id, nil
}

// ListUserRoles implements identity.Repository.
func (r *IdentityRepository) ListUserRoles(ctx context.Context, userID string) ([]identity.Role, error) {
	rows, err := r.db.reader().ListUserRoles(ctx, userID)
	if err != nil {
		return nil, wrap(err, "list user roles")
	}
	out := make([]identity.Role, len(rows))
	for i, row := range rows {
		out[i] = roleFrom(row)
	}
	return out, nil
}
