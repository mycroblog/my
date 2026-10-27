// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Granting and revoking roles (FR-720, manage_roles). A grant or a revocation writes its event
// (user_role_grants, user_role_revocations), updates the user's current roles (user_roles), and
// records its audit log entry (ADR-0037) in one transaction. Permissions are read from the
// current roles on every request (Permissions), so a change applies from the user's next
// request, and the moderator notifications go to whoever holds the permission when they are
// made.

// RoleRepository is the persistence of role grants and revocations, part of Repository.
type RoleRepository interface {
	// ListRoles returns every role with its permissions, highest position first.
	ListRoles(ctx context.Context) ([]RoleWithPermissions, error)
	// ExistingRoleIDs returns, in ID order, the roles among ids that exist.
	ExistingRoleIDs(ctx context.Context, ids []string) ([]string, error)
	// LockRole returns inside tx the role with key, so that the changes to one role run one
	// after another (SELECT ... FOR UPDATE where the database needs it).
	LockRole(ctx context.Context, tx uow.Tx, key string) (identity.Role, error)
	// FindRoleGrantID returns inside tx the grant through which the user holds the role now.
	FindRoleGrantID(ctx context.Context, tx uow.Tx, userID, roleID string) (string, error)
	// CountRoleHolders counts inside tx the users who hold the role now.
	CountRoleHolders(ctx context.Context, tx uow.Tx, roleID string) (int64, error)
	// InsertRoleRevocation records r and removes the revoked grant from the current roles.
	InsertRoleRevocation(ctx context.Context, tx uow.Tx, r identity.RoleRevocation) error
	// ListRoleHolderIDs returns the users who hold the role now, by ID.
	ListRoleHolderIDs(ctx context.Context, roleID string) ([]string, error)
	// RolePermissions returns inside tx the permissions of the role, in the order of
	// identity.AllPermissions.
	RolePermissions(ctx context.Context, tx uow.Tx, roleID string) ([]identity.Permission, error)
	// SetRolePermissions makes perms the permissions of the role.
	SetRolePermissions(ctx context.Context, tx uow.Tx, roleID string, perms []identity.Permission) error
	// UpdateRolePosition changes the position of the role.
	UpdateRolePosition(ctx context.Context, tx uow.Tx, roleID string, position int) error
	// DeleteRole deletes the role, its permissions, and its badge; nobody may hold it.
	DeleteRole(ctx context.Context, tx uow.Tx, roleID string) error
	RoleBadgeRepository
}

// RoleWithPermissions is a role with the permissions it grants, in the order of
// identity.AllPermissions, and its badge (nil for none).
type RoleWithPermissions struct {
	identity.Role
	Permissions []identity.Permission
	Badge       *identity.RoleBadge
}

// ErrRoleOutranksActor is identity.ErrRoleOutranksActor.
var ErrRoleOutranksActor = identity.ErrRoleOutranksActor

// ErrLastAdmin is identity.ErrLastAdmin.
var ErrLastAdmin = identity.ErrLastAdmin

// Roles returns every role with its permissions, highest position first.
func (s *Service) Roles(ctx context.Context) ([]RoleWithPermissions, error) {
	return s.d.Repository.ListRoles(ctx)
}

// ExistingRoleIDs returns, in ID order, the roles among ids that exist (not deleted), in one
// lookup.
func (s *Service) ExistingRoleIDs(ctx context.Context, ids []string) ([]string, error) {
	return s.d.Repository.ExistingRoleIDs(ctx, ids)
}

// UserRoles returns the roles that a user holds now, highest position first.
func (s *Service) UserRoles(ctx context.Context, userID string) ([]identity.Role, error) {
	return s.d.Repository.ListUserRoles(ctx, userID)
}

// RoleHolders returns the users who hold the role with key now, by ID. It fails with
// ErrNotFound for an unknown role.
func (s *Service) RoleHolders(ctx context.Context, key string) ([]string, error) {
	r, _, err := s.d.Repository.FindRole(ctx, key)
	if err != nil {
		return nil, err
	}
	return s.d.Repository.ListRoleHolderIDs(ctx, r.ID)
}

// RoleChangeInput names a role of a user to grant or revoke.
type RoleChangeInput struct {
	UserID  string
	RoleKey string
	// ByUserID is the user who changes the role, or "" for the CLI. A user may change only the
	// roles at or below their highest one (identity.CanManageRole), and grant only the roles
	// whose every permission they hold themselves; the CLI may change any.
	ByUserID string
}

// GrantRole grants a role to a user and reports whether it changed anything: granting a role
// that the user holds does nothing and records nothing. It fails with ErrNotFound for an
// unknown user or role, with ErrRoleOutranksActor when the role is above every role of
// in.ByUserID, with a *MissingPermissionsError when in.ByUserID lacks a permission of the role,
// and with ErrRoleManagedByPlan for the role of a supporter plan, which only its subscriptions
// grant (ADR-0072 §5).
func (s *Service) GrantRole(ctx context.Context, in RoleChangeInput) (bool, error) {
	c, err := s.prepareRoleChange(ctx, in)
	if err != nil {
		return false, err
	}
	changed := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		changed = false
		role, err := s.d.Repository.LockRole(ctx, tx, in.RoleKey)
		if err != nil {
			return err
		}
		if err := s.refuseManaged(ctx, tx, role); err != nil {
			return err
		}
		if in.ByUserID != "" {
			// The permissions are read in the transaction that locks the role, so that a change
			// to them cannot slip between the check and the grant.
			perms, err := s.d.Repository.RolePermissions(ctx, tx, role.ID)
			if err != nil {
				return err
			}
			if err := identity.CheckPermissionsHeld(c.actorPerms, perms); err != nil {
				return err
			}
		}
		switch _, err := s.d.Repository.FindRoleGrantID(ctx, tx, in.UserID, role.ID); {
		case err == nil:
			return nil
		case !errors.Is(err, ErrNotFound):
			return err
		}
		g := identity.RoleGrant{ID: c.eventID, UserID: in.UserID, RoleID: role.ID, GrantedByUserID: in.ByUserID, GrantedAt: c.at}
		if err := s.d.Repository.InsertRoleGrant(ctx, tx, g); err != nil {
			return err
		}
		// A new grant starts with the badge hidden (ADR-0071 §2).
		if err := s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, in.UserID, role.ID, false, c.at); err != nil {
			return err
		}
		changed = true
		return tx.Emit(ctx, c.audit(appmoderation.AuditRoleGranted, role, g.ID))
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

// RevokeRole revokes a role from a user and reports whether it changed anything: revoking a
// role that the user does not hold does nothing and records nothing. It fails with ErrNotFound
// for an unknown user or role, with ErrRoleOutranksActor when the role is above every role of
// in.ByUserID, with ErrLastAdmin when the user is the only holder of the admin role, and with
// ErrRoleManagedByPlan for the role of a supporter plan.
func (s *Service) RevokeRole(ctx context.Context, in RoleChangeInput) (bool, error) {
	c, err := s.prepareRoleChange(ctx, in)
	if err != nil {
		return false, err
	}
	changed := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		changed = false
		role, err := s.d.Repository.LockRole(ctx, tx, in.RoleKey)
		if err != nil {
			return err
		}
		if err := s.refuseManaged(ctx, tx, role); err != nil {
			return err
		}
		grantID, err := s.d.Repository.FindRoleGrantID(ctx, tx, in.UserID, role.ID)
		switch {
		case errors.Is(err, ErrNotFound):
			return nil
		case err != nil:
			return err
		}
		if role.Key == identity.RoleAdmin {
			n, err := s.d.Repository.CountRoleHolders(ctx, tx, role.ID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return ErrLastAdmin
			}
		}
		r := identity.RoleRevocation{GrantID: grantID, RevokedByUserID: in.ByUserID, RevokedAt: c.at}
		if err := s.d.Repository.InsertRoleRevocation(ctx, tx, r); err != nil {
			return err
		}
		// The user no longer shows the role's badge (ADR-0071 §2).
		if err := s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, in.UserID, role.ID, false, c.at); err != nil {
			return err
		}
		changed = true
		return tx.Emit(ctx, c.audit(appmoderation.AuditRoleRevoked, role, grantID))
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

// roleChange is what a grant or a revocation prepares before its transaction, so that a retry
// writes the same rows.
type roleChange struct {
	in      RoleChangeInput
	eventID string
	auditID string
	label   string
	at      time.Time
	// actorPerms are the permissions of in.ByUserID; none for the CLI.
	actorPerms []identity.Permission
}

// prepareRoleChange checks the user, the role, and the rank of the actor, reads the actor's
// permissions, and makes the IDs and the label of the audit log entry.
func (s *Service) prepareRoleChange(ctx context.Context, in RoleChangeInput) (roleChange, error) {
	if _, err := s.d.Repository.FindUser(ctx, in.UserID); err != nil {
		return roleChange{}, err
	}
	role, _, err := s.d.Repository.FindRole(ctx, in.RoleKey)
	if err != nil {
		return roleChange{}, err
	}
	actor, err := s.loadRoleActor(ctx, in.ByUserID)
	if err != nil {
		return roleChange{}, err
	}
	if in.ByUserID != "" && !identity.CanManageRole(actor.roles, role) {
		return roleChange{}, fmt.Errorf("%w: %s", ErrRoleOutranksActor, role.Key)
	}
	label := in.UserID
	la, err := s.d.Accounts.LocalAccountOfUser(ctx, in.UserID)
	switch {
	case err == nil:
		label = "@" + la.Username
	case !errors.Is(err, account.ErrNotFound):
		return roleChange{}, err
	}
	return roleChange{
		in: in, eventID: s.d.IDs.NewID(), auditID: s.d.IDs.NewID(), label: role.Key + " " + label, at: s.d.Clock.Now(),
		actorPerms: actor.perms,
	}, nil
}

// audit returns the audit log entry of the change: its target is the role, labeled with the
// role's key and the user's handle (moderator @alice), and its source is the grant (whose ID is
// also the key of its revocation).
func (c roleChange) audit(action appmoderation.AuditAction, role identity.Role, grantID string) appmoderation.AuditRecorded {
	actor, user := appmoderation.AuditActor(c.in.ByUserID)
	return appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: c.auditID, ActorKind: actor, ActorUserID: user, Action: action, TargetKind: appmoderation.AuditTargetRole,
		TargetID: role.ID, TargetLabel: c.label, SourceEventID: grantID, OccurredAt: c.at,
	}}
}
