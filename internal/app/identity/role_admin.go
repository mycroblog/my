// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"fmt"
	"slices"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Creating, changing, and deleting roles (FR-720, manage_roles). The rules are those of
// internal/domain/identity: the built-in roles keep their keys, admin is never changed, and a
// role that users hold is never deleted; a user changes only the roles at or below their highest
// one and only the permissions they hold. Each change writes the roles and their permissions
// (resources and current values) and records its audit log entry (ADR-0058) in one transaction.
// Permissions are read from the current roles on every request, so a change applies to the
// holders of the role from their next request.

// The errors of the role changes, from internal/domain/identity.
var (
	ErrInvalidRoleKey         = identity.ErrInvalidRoleKey
	ErrInvalidRolePosition    = identity.ErrInvalidRolePosition
	ErrInvalidRolePermissions = identity.ErrInvalidRolePermissions
	ErrRoleKeyTaken           = identity.ErrRoleKeyTaken
	ErrBuiltinRole            = identity.ErrBuiltinRole
	ErrRoleHeld               = identity.ErrRoleHeld
)

// MissingPermissionsError is identity.MissingPermissionsError.
type MissingPermissionsError = identity.MissingPermissionsError

// CreateRoleInput is a new role.
type CreateRoleInput struct {
	Key         string
	Position    int
	Permissions []identity.Permission
	// ByUserID is the user who creates the role, or "" for the CLI.
	ByUserID string
}

// UpdateRoleInput changes the position, the permissions, or both of the role with Key.
type UpdateRoleInput struct {
	Key string
	// Position is the new position, or nil to keep it.
	Position *int
	// Permissions are the new permissions, or nil to keep them.
	Permissions []identity.Permission
	// SetPermissions reports whether Permissions are given (an empty list removes every
	// permission).
	SetPermissions bool
	// Badge is the new badge of the role, or nil with SetBadge to remove it (ADR-0070).
	Badge *BadgeInput
	// SetBadge reports whether the badge changes: to Badge, or to none when Badge is nil.
	SetBadge bool
	// ByUserID is the user who changes the role, or "" for the CLI.
	ByUserID string
}

// roleActor is what the rules need to know about the user who changes a role.
type roleActor struct {
	userID string
	roles  []identity.Role
	perms  []identity.Permission
}

// loadRoleActor reads the roles and the permissions of userID; the CLI ("") may do anything.
func (s *Service) loadRoleActor(ctx context.Context, userID string) (roleActor, error) {
	if userID == "" {
		return roleActor{}, nil
	}
	roles, err := s.d.Repository.ListUserRoles(ctx, userID)
	if err != nil {
		return roleActor{}, err
	}
	perms, err := s.Permissions(ctx, userID)
	if err != nil {
		return roleActor{}, err
	}
	return roleActor{userID: userID, roles: roles, perms: perms}, nil
}

// check fails with ErrRoleOutranksActor when position is above every role of the actor, and with
// a *MissingPermissionsError when the actor lacks one of changed.
func (a roleActor) check(key string, position int, changed []identity.Permission) error {
	if a.userID == "" {
		return nil
	}
	if !identity.CanManageRole(a.roles, identity.Role{Key: key, Position: position}) {
		return fmt.Errorf("%w: %s", ErrRoleOutranksActor, key)
	}
	return identity.CheckPermissionsHeld(a.perms, changed)
}

// CreateRole creates a role. It fails with ErrInvalidRoleKey, ErrInvalidRolePosition, or
// ErrInvalidRolePermissions for a role that breaks a rule, with ErrRoleKeyTaken when another
// role (a built-in one included) has the key, with ErrRoleOutranksActor when the position is
// above every role of in.ByUserID, with a *MissingPermissionsError when in.ByUserID lacks one of
// the permissions, and with ErrBreaksManagedRoles for a staff role at or below the role of a
// supporter plan (ADR-0072 §5).
func (s *Service) CreateRole(ctx context.Context, in CreateRoleInput) (RoleWithPermissions, error) {
	if err := identity.ValidateRoleKey(in.Key); err != nil {
		return RoleWithPermissions{}, err
	}
	if identity.IsBuiltinRole(in.Key) {
		return RoleWithPermissions{}, fmt.Errorf("%w: %s", ErrRoleKeyTaken, in.Key)
	}
	if err := identity.ValidateRolePosition(in.Position); err != nil {
		return RoleWithPermissions{}, err
	}
	perms, err := identity.NormalizeRolePermissions(in.Permissions)
	if err != nil {
		return RoleWithPermissions{}, err
	}
	actor, err := s.loadRoleActor(ctx, in.ByUserID)
	if err != nil {
		return RoleWithPermissions{}, err
	}
	if err := actor.check(in.Key, in.Position, perms); err != nil {
		return RoleWithPermissions{}, err
	}
	now := s.d.Clock.Now()
	role := identity.Role{ID: s.d.IDs.NewID(), Key: in.Key, Position: in.Position, CreatedAt: now}
	auditID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.checkUnmanaged(ctx, tx, "", in.Key, in.Position, perms); err != nil {
			return err
		}
		stored, err := s.d.Repository.EnsureRole(ctx, tx, role, perms)
		if err != nil {
			return err
		}
		if stored.ID != role.ID {
			return fmt.Errorf("%w: %s", ErrRoleKeyTaken, in.Key)
		}
		return tx.Emit(ctx, roleAudit(auditID, appmoderation.AuditRoleCreated, role, in.ByUserID, now))
	})
	if err != nil {
		return RoleWithPermissions{}, err
	}
	return RoleWithPermissions{Role: role, Permissions: perms}, nil
}

// UpdateRole changes the position, the permissions, the badge, or several of them of a role,
// and returns the role afterwards; a change to what the role has already records nothing.
// Removing the badge also removes every user's choice to show it. It fails with ErrNotFound for
// an unknown role, with ErrBuiltinRole for a new position or permissions of admin (whose badge
// changes like any other), with ErrInvalidRolePosition, ErrInvalidRolePermissions,
// ErrInvalidBadgeLabel, or ErrInvalidBadgeColor for a change that breaks a rule, with
// ErrRoleOutranksActor when the role or its new position is above every role of in.ByUserID,
// with a *MissingPermissionsError when in.ByUserID lacks a permission that the change adds or
// removes, with ErrRoleManagedByPlan for a new position or permissions of the role of a
// supporter plan (its plan's form changes them; its badge changes here), and with
// ErrBreaksManagedRoles for a change that puts a staff role at or below one (ADR-0072 §5).
func (s *Service) UpdateRole(ctx context.Context, in UpdateRoleInput) (RoleWithPermissions, error) {
	if in.Key == identity.RoleAdmin && (in.Position != nil || in.SetPermissions || !in.SetBadge) {
		return RoleWithPermissions{}, fmt.Errorf("%w: %s", ErrBuiltinRole, in.Key)
	}
	if in.Position != nil {
		if err := identity.ValidateRolePosition(*in.Position); err != nil {
			return RoleWithPermissions{}, err
		}
	}
	var perms []identity.Permission
	if in.SetPermissions {
		var err error
		if perms, err = identity.NormalizeRolePermissions(in.Permissions); err != nil {
			return RoleWithPermissions{}, err
		}
	}
	actor, err := s.loadRoleActor(ctx, in.ByUserID)
	if err != nil {
		return RoleWithPermissions{}, err
	}
	now := s.d.Clock.Now()
	var badge *identity.RoleBadge
	if in.SetBadge && in.Badge != nil {
		label, err := identity.NormalizeRoleBadge(in.Badge.Label, in.Badge.Color)
		if err != nil {
			return RoleWithPermissions{}, err
		}
		badge = &identity.RoleBadge{Label: label, Color: in.Badge.Color, UpdatedAt: now}
	}
	auditID := s.d.IDs.NewID()
	var out RoleWithPermissions
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		role, err := s.d.Repository.LockRole(ctx, tx, in.Key)
		if err != nil {
			return err
		}
		current, err := s.d.Repository.RolePermissions(ctx, tx, role.ID)
		if err != nil {
			return err
		}
		var currentBadge *identity.RoleBadge
		if b, ok, err := s.d.Repository.RoleBadge(ctx, tx, role.ID); err != nil {
			return err
		} else if ok {
			currentBadge = &b
		}
		out = RoleWithPermissions{Role: role, Permissions: current, Badge: currentBadge}
		if err := actor.check(role.Key, role.Position, nil); err != nil {
			return err
		}
		next := role
		if in.Position != nil {
			next.Position = *in.Position
		}
		nextPerms := current
		if in.SetPermissions {
			nextPerms = perms
		}
		nextBadge := currentBadge
		if in.SetBadge && !identity.SameBadge(currentBadge, badge) {
			nextBadge = badge
		}
		changed := identity.ChangedPermissions(current, nextPerms)
		if next.Position == role.Position && len(changed) == 0 && identity.SameBadge(currentBadge, nextBadge) {
			return nil
		}
		if err := actor.check(role.Key, next.Position, changed); err != nil {
			return err
		}
		if next.Position != role.Position || len(changed) > 0 {
			if err := s.checkUnmanaged(ctx, tx, role.ID, role.Key, next.Position, nextPerms); err != nil {
				return err
			}
		}
		if next.Position != role.Position {
			if err := s.d.Repository.UpdateRolePosition(ctx, tx, role.ID, next.Position); err != nil {
				return err
			}
		}
		if len(changed) > 0 {
			if err := s.d.Repository.SetRolePermissions(ctx, tx, role.ID, nextPerms); err != nil {
				return err
			}
		}
		if err := s.setRoleBadge(ctx, tx, role.ID, currentBadge, nextBadge); err != nil {
			return err
		}
		out = RoleWithPermissions{Role: next, Permissions: slices.Clone(nextPerms), Badge: nextBadge}
		return tx.Emit(ctx, roleAudit(auditID, appmoderation.AuditRoleUpdated, role, in.ByUserID, now))
	})
	if err != nil {
		return RoleWithPermissions{}, err
	}
	return out, nil
}

// DeleteRole deletes a role that nobody holds. It fails with ErrNotFound for an unknown role,
// with ErrBuiltinRole for admin and moderator, with ErrRoleOutranksActor when the role is above
// every role of byUserID ("" for the CLI), with ErrRoleHeld while users hold the role, and with
// ErrRoleManagedByPlan for the role of a supporter plan.
func (s *Service) DeleteRole(ctx context.Context, key, byUserID string) error {
	if identity.IsBuiltinRole(key) {
		return fmt.Errorf("%w: %s", ErrBuiltinRole, key)
	}
	actor, err := s.loadRoleActor(ctx, byUserID)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	auditID := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		role, err := s.d.Repository.LockRole(ctx, tx, key)
		if err != nil {
			return err
		}
		if err := actor.check(role.Key, role.Position, nil); err != nil {
			return err
		}
		if err := s.refuseManaged(ctx, tx, role); err != nil {
			return err
		}
		n, err := s.d.Repository.CountRoleHolders(ctx, tx, role.ID)
		if err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: %s (%d)", ErrRoleHeld, key, n)
		}
		if err := s.d.Repository.DeleteRole(ctx, tx, role.ID); err != nil {
			return err
		}
		return tx.Emit(ctx, roleAudit(auditID, appmoderation.AuditRoleDeleted, role, byUserID, now))
	})
}

// roleAudit returns the audit log entry of a change to a role: its target is the role, labeled
// with its key. The change has no row of its own, so the entry has no source.
func roleAudit(id string, action appmoderation.AuditAction, role identity.Role, byUserID string, at time.Time) appmoderation.AuditRecorded {
	actor, user := appmoderation.AuditActor(byUserID)
	return appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: id, ActorKind: actor, ActorUserID: user, Action: action, TargetKind: appmoderation.AuditTargetRole,
		TargetID: role.ID, TargetLabel: role.Key, OccurredAt: at,
	}}
}
