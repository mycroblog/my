// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Repository implements identity.Repository on SQLite.
type Repository struct {
	db *moduleDB
}

var _ appidentity.Repository = (*Repository)(nil)

// NewRepository returns the identity module's repository on d.
func NewRepository(d *sqlite.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertUser implements identity.Repository.
func (r *Repository) InsertUser(ctx context.Context, tx uow.Tx, u identity.User) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertUser(ctx, identitygen.InsertUserParams{ID: u.ID, CreatedAt: sqlite.ToMillis(u.CreatedAt)}), "insert user")
}

// InsertPasswordCredential implements identity.Repository.
func (r *Repository) InsertPasswordCredential(ctx context.Context, tx uow.Tx, c identity.PasswordCredential) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasswordCredential(ctx, identitygen.InsertPasswordCredentialParams{
		ID: c.ID, UserID: c.UserID, PasswordHash: c.Hash, SetAt: sqlite.ToMillis(c.SetAt),
	})
	return sqlite.Wrap(err, "insert password credential")
}

// EnsureRole implements identity.Repository.
func (r *Repository) EnsureRole(ctx context.Context, tx uow.Tx, role identity.Role, perms []identity.Permission) (identity.Role, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.Role{}, err
	}
	err = q.InsertRoleIfAbsent(ctx, identitygen.InsertRoleIfAbsentParams{
		ID: role.ID, Key: role.Key, Position: int64(role.Position), CreatedAt: sqlite.ToMillis(role.CreatedAt),
	})
	if err != nil {
		return identity.Role{}, sqlite.Wrap(err, "insert role")
	}
	row, err := q.GetRoleByKey(ctx, role.Key)
	if err != nil {
		return identity.Role{}, sqlite.Wrap(err, "get role")
	}
	if row.ID != role.ID {
		return roleFrom(row), nil
	}
	for _, p := range perms {
		err := q.InsertRolePermissionIfAbsent(ctx, identitygen.InsertRolePermissionIfAbsentParams{RoleID: row.ID, Permission: string(p)})
		if err != nil {
			return identity.Role{}, sqlite.Wrap(err, "insert role permission")
		}
	}
	return roleFrom(row), nil
}

func roleFrom(row identitygen.Role) identity.Role {
	return identity.Role{ID: row.ID, Key: row.Key, Position: int(row.Position), CreatedAt: sqlite.FromMillis(row.CreatedAt)}
}

// FindUser implements identity.Repository.
func (r *Repository) FindUser(ctx context.Context, id string) (identity.User, error) {
	row, err := r.db.reader().GetUser(ctx, id)
	if err != nil {
		return identity.User{}, sqlite.NotFound(err, appidentity.ErrNotFound, "user "+id)
	}
	return identity.User{ID: row.ID, CreatedAt: sqlite.FromMillis(row.CreatedAt)}, nil
}

// FindCurrentPasswordCredential implements identity.Repository.
func (r *Repository) FindCurrentPasswordCredential(ctx context.Context, userID string) (identity.PasswordCredential, error) {
	c, removedAt, err := r.newestPasswordCredential(ctx, userID)
	if err == nil && removedAt != nil {
		return identity.PasswordCredential{}, fmt.Errorf("password of %s: removed: %w", userID, appidentity.ErrNotFound)
	}
	return c, err
}

// LastPasswordChange implements identity.Repository.
func (r *Repository) LastPasswordChange(ctx context.Context, userID string) (time.Time, error) {
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
func (r *Repository) newestPasswordCredential(ctx context.Context, userID string) (identity.PasswordCredential, *time.Time, error) {
	row, err := r.db.reader().GetCurrentPasswordCredential(ctx, userID)
	if err != nil {
		return identity.PasswordCredential{}, nil, sqlite.NotFound(err, appidentity.ErrNotFound, "password of "+userID)
	}
	return identity.PasswordCredential{ID: row.ID, UserID: row.UserID, Hash: row.PasswordHash, SetAt: sqlite.FromMillis(row.SetAt)}, sqlite.FromNullMillis(row.RemovedAt), nil
}

// InsertPasswordCredentialRemoval implements identity.Repository.
func (r *Repository) InsertPasswordCredentialRemoval(ctx context.Context, tx uow.Tx, rem identity.PasswordCredentialRemoval) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasswordCredentialRemoval(ctx, identitygen.InsertPasswordCredentialRemovalParams{
		PasswordCredentialID: rem.PasswordCredentialID, RemovedAt: sqlite.ToMillis(rem.RemovedAt),
	})
	return sqlite.Wrap(err, "insert password credential removal")
}

// LockSignInMethods implements identity.Repository. SQLite runs one write transaction at a time,
// so there is nothing to lock.
func (r *Repository) LockSignInMethods(context.Context, uow.Tx, string) error { return nil }

// FindRole implements identity.Repository.
func (r *Repository) FindRole(ctx context.Context, key string) (identity.Role, []identity.Permission, error) {
	q := r.db.reader()
	row, err := q.GetRoleByKey(ctx, key)
	if err != nil {
		return identity.Role{}, nil, sqlite.NotFound(err, appidentity.ErrNotFound, "role "+key)
	}
	perms, err := q.ListRolePermissions(ctx, row.ID)
	if err != nil {
		return identity.Role{}, nil, sqlite.Wrap(err, "list role permissions")
	}
	out := make([]identity.Permission, len(perms))
	for i, p := range perms {
		out[i] = identity.Permission(p)
	}
	return roleFrom(row), out, nil
}

// InsertEmailAddress implements identity.Repository.
func (r *Repository) InsertEmailAddress(ctx context.Context, tx uow.Tx, e identity.EmailAddress) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertEmailAddress(ctx, identitygen.InsertEmailAddressParams{
		ID: e.ID, UserID: e.UserID, Address: e.Address, AddressNormalized: e.AddressNormalized, AddedAt: sqlite.ToMillis(e.AddedAt),
	})
	return sqlite.Wrap(err, "insert email address")
}

// InsertPrimaryEmail implements identity.Repository.
func (r *Repository) InsertPrimaryEmail(ctx context.Context, tx uow.Tx, e identity.EmailAddress) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPrimaryEmail(ctx, identitygen.InsertPrimaryEmailParams{
		UserID: e.UserID, EmailAddressID: e.ID, AddressNormalized: e.AddressNormalized,
	})
	if sqlite.IsUniqueViolation(err, "user_primary_emails.address_normalized") {
		return fmt.Errorf("insert primary email of user %s: %w", e.UserID, appidentity.ErrEmailTaken)
	}
	return sqlite.Wrap(err, "insert primary email")
}

// InsertRoleGrant implements identity.Repository.
func (r *Repository) InsertRoleGrant(ctx context.Context, tx uow.Tx, g identity.RoleGrant) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserRoleGrant(ctx, identitygen.InsertUserRoleGrantParams{
		ID: g.ID, UserID: g.UserID, RoleID: g.RoleID, GrantedByUserID: sqlite.ToNullString(g.GrantedByUserID), GrantedAt: sqlite.ToMillis(g.GrantedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert role grant")
	}
	return sqlite.Wrap(q.InsertUserRole(ctx, identitygen.InsertUserRoleParams{UserID: g.UserID, RoleID: g.RoleID, GrantID: g.ID}), "insert user role")
}

// FindUserIDByEmail implements identity.Repository.
func (r *Repository) FindUserIDByEmail(ctx context.Context, normalized string) (string, error) {
	id, err := r.db.reader().GetUserIDByPrimaryEmail(ctx, normalized)
	if err != nil {
		return "", sqlite.NotFound(err, appidentity.ErrNotFound, "user by email")
	}
	return id, nil
}

// ListUserRoles implements identity.Repository.
func (r *Repository) ListUserRoles(ctx context.Context, userID string) ([]identity.Role, error) {
	rows, err := r.db.reader().ListUserRoles(ctx, userID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list user roles")
	}
	out := make([]identity.Role, len(rows))
	for i, row := range rows {
		out[i] = roleFrom(row)
	}
	return out, nil
}
