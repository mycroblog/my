// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The roles that another module manages (ADR-0072 §5, managed_roles): the role of a supporter
// plan. The supporters module marks a role managed when it creates the plan and grants, revokes,
// and changes it only through the operations below, which take its transaction so that a
// subscription's change of state and the change of the roles commit together. The grants and
// revocations act as the system (the audit log's actor system); a change of the role from the
// plan form acts as the user who used it. The operations of FR-720 refuse a managed role with
// ErrRoleManagedByPlan, and refuse a change of another role that would put a staff role at or
// below a managed one with ErrBreaksManagedRoles.

// ManagedRoleRepository is the persistence of the managed roles, part of Repository.
type ManagedRoleRepository interface {
	// ListManagedRoles returns every managed role, by role ID; inside tx when tx is not nil.
	ListManagedRoles(ctx context.Context, tx uow.Tx) ([]identity.ManagedRole, error)
	// InsertManagedRole marks a role managed; the role must not be managed yet.
	InsertManagedRole(ctx context.Context, tx uow.Tx, m identity.ManagedRole) error
	// DeleteManagedRole makes a role an ordinary one again.
	DeleteManagedRole(ctx context.Context, tx uow.Tx, roleID string) error
	// LockRoleByID returns inside tx the role with id, locked like LockRole.
	LockRoleByID(ctx context.Context, tx uow.Tx, id string) (identity.Role, error)
	// ListRolesTx returns every role with its permissions as ListRoles does, read in tx.
	ListRolesTx(ctx context.Context, tx uow.Tx) ([]RoleWithPermissions, error)
}

// The errors of the managed roles, from internal/domain/identity.
var (
	ErrRoleManagedByPlan  = identity.ErrRoleManagedByPlan
	ErrBreaksManagedRoles = identity.ErrBreaksManagedRoles
	ErrRoleNotManageable  = identity.ErrRoleNotManageable
)

// UnmanageableRoleError is identity.UnmanageableRoleError.
type UnmanageableRoleError = identity.UnmanageableRoleError

// Permission is identity.Permission, and RoleManager identity.RoleManager, for the modules that
// manage roles.
type (
	Permission  = identity.Permission
	RoleManager = identity.RoleManager
)

// ManagerSupporters is identity.ManagerSupporters.
const ManagerSupporters = identity.ManagerSupporters

// IsMemberPermission reports whether p is a member permission (ADR-0070 §1), which the role of
// a supporter plan may have.
func IsMemberPermission(p Permission) bool { return p.Category() == identity.CategoryMember }

// ManagedRoleView is a role with its permissions and whether a module manages it.
type ManagedRoleView struct {
	RoleWithPermissions
	Managed bool
}

// rolesTx reads every role in tx with whether it is managed.
func (s *Service) rolesTx(ctx context.Context, tx uow.Tx) ([]ManagedRoleView, error) {
	roles, err := s.d.Repository.ListRolesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	managed, err := s.d.Repository.ListManagedRoles(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]ManagedRoleView, len(roles))
	for i, r := range roles {
		out[i] = ManagedRoleView{
			RoleWithPermissions: r,
			Managed:             slices.ContainsFunc(managed, func(m identity.ManagedRole) bool { return m.RoleID == r.ID }),
		}
	}
	return out, nil
}

func roleFacts(roles []ManagedRoleView) []identity.RoleFacts {
	out := make([]identity.RoleFacts, len(roles))
	for i, r := range roles {
		out[i] = identity.RoleFacts{Role: r.Role, Permissions: r.Permissions, Managed: r.Managed}
	}
	return out
}

// isManaged reports inside tx whether a module manages the role.
func (s *Service) isManaged(ctx context.Context, tx uow.Tx, roleID string) (bool, error) {
	managed, err := s.d.Repository.ListManagedRoles(ctx, tx)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(managed, func(m identity.ManagedRole) bool { return m.RoleID == roleID }), nil
}

// ManagedRoleIDs returns the roles that a module manages, by ID.
func (s *Service) ManagedRoleIDs(ctx context.Context) ([]string, error) {
	managed, err := s.d.Repository.ListManagedRoles(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(managed))
	for i, m := range managed {
		out[i] = m.RoleID
	}
	return out, nil
}

// UserRoleIDs returns the roles that a user holds now, by ID.
func (s *Service) UserRoleIDs(ctx context.Context, userID string) ([]string, error) {
	roles, err := s.d.Repository.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = r.ID
	}
	slices.Sort(out)
	return out, nil
}

// RolesTx returns every role in tx, highest position first, with whether a module manages it.
func (s *Service) RolesTx(ctx context.Context, tx uow.Tx) ([]ManagedRoleView, error) {
	return s.rolesTx(ctx, tx)
}

// ManageRole marks the role with key managed by manager inside tx and returns it (ADR-0072
// §5): it fails with ErrNotFound for an unknown role, and with an *UnmanageableRoleError for a
// built-in role, a managed one, one that users hold, one with a permission that is not a member
// permission, or one at or above the lowest staff role.
func (s *Service) ManageRole(ctx context.Context, tx uow.Tx, key string, manager identity.RoleManager, at time.Time) (RoleWithPermissions, error) {
	role, err := s.d.Repository.LockRole(ctx, tx, key)
	if err != nil {
		return RoleWithPermissions{}, err
	}
	roles, err := s.rolesTx(ctx, tx)
	if err != nil {
		return RoleWithPermissions{}, err
	}
	i := slices.IndexFunc(roles, func(r ManagedRoleView) bool { return r.ID == role.ID })
	if i < 0 {
		return RoleWithPermissions{}, fmt.Errorf("%w: role %s", ErrNotFound, key)
	}
	holders, err := s.d.Repository.CountRoleHolders(ctx, tx, role.ID)
	if err != nil {
		return RoleWithPermissions{}, err
	}
	facts := roleFacts(roles)
	if err := identity.CheckManageable(facts[i], holders, facts); err != nil {
		return RoleWithPermissions{}, err
	}
	if err := s.d.Repository.InsertManagedRole(ctx, tx, identity.ManagedRole{RoleID: role.ID, Manager: manager, CreatedAt: at}); err != nil {
		return RoleWithPermissions{}, err
	}
	return roles[i].RoleWithPermissions, nil
}

// UnmanageRole makes a managed role an ordinary one again inside tx, once nobody holds it
// (ADR-0072 §5): administrators may then change and delete it. It reports whether it did; a role
// that users still hold stays managed.
func (s *Service) UnmanageRole(ctx context.Context, tx uow.Tx, roleID string) (bool, error) {
	if _, err := s.d.Repository.LockRoleByID(ctx, tx, roleID); err != nil {
		return false, err
	}
	managed, err := s.isManaged(ctx, tx, roleID)
	if err != nil || !managed {
		return false, err
	}
	n, err := s.d.Repository.CountRoleHolders(ctx, tx, roleID)
	if err != nil || n > 0 {
		return false, err
	}
	return true, s.d.Repository.DeleteManagedRole(ctx, tx, roleID)
}

// ManagedRoleUpdate changes the position, the permissions, or both of a managed role from the
// plan form.
type ManagedRoleUpdate struct {
	RoleID string
	// Position is the new position, or nil to keep it.
	Position *int
	// Permissions are the new permissions when SetPermissions is true.
	Permissions    []identity.Permission
	SetPermissions bool
	// ByUserID is the user who used the plan form, or "" for the CLI.
	ByUserID string
	At       time.Time
}

// UpdateManagedRole changes a managed role inside tx and returns it with whether it changed
// anything; it records role_updated as ByUserID. It fails with ErrNotFound for an unknown role,
// with ErrRoleNotManageable when the role is not managed, with ErrInvalidRolePosition or
// ErrInvalidRolePermissions for a change that breaks a rule of roles, and with an
// *UnmanageableRoleError for a permission that is not a member permission or a position at or
// above the lowest staff role.
func (s *Service) UpdateManagedRole(ctx context.Context, tx uow.Tx, in ManagedRoleUpdate) (RoleWithPermissions, bool, error) {
	role, err := s.d.Repository.LockRoleByID(ctx, tx, in.RoleID)
	if err != nil {
		return RoleWithPermissions{}, false, err
	}
	roles, err := s.rolesTx(ctx, tx)
	if err != nil {
		return RoleWithPermissions{}, false, err
	}
	i := slices.IndexFunc(roles, func(r ManagedRoleView) bool { return r.ID == role.ID })
	if i < 0 {
		return RoleWithPermissions{}, false, fmt.Errorf("%w: role %s", ErrNotFound, in.RoleID)
	}
	cur := roles[i]
	if !cur.Managed {
		return RoleWithPermissions{}, false, fmt.Errorf("%w: role %s is not managed", ErrRoleNotManageable, role.Key)
	}
	next := cur.RoleWithPermissions
	if in.Position != nil {
		if err := identity.ValidateRolePosition(*in.Position); err != nil {
			return RoleWithPermissions{}, false, err
		}
		next.Position = *in.Position
	}
	if in.SetPermissions {
		perms, err := identity.NormalizeRolePermissions(in.Permissions)
		if err != nil {
			return RoleWithPermissions{}, false, err
		}
		next.Permissions = perms
	}
	changed := identity.ChangedPermissions(cur.Permissions, next.Permissions)
	if next.Position == cur.Position && len(changed) == 0 {
		return cur.RoleWithPermissions, false, nil
	}
	if err := identity.CheckManagedShape(role.ID, next.Position, next.Permissions, roleFacts(roles)); err != nil {
		return RoleWithPermissions{}, false, err
	}
	if next.Position != cur.Position {
		if err := s.d.Repository.UpdateRolePosition(ctx, tx, role.ID, next.Position); err != nil {
			return RoleWithPermissions{}, false, err
		}
	}
	if len(changed) > 0 {
		if err := s.d.Repository.SetRolePermissions(ctx, tx, role.ID, next.Permissions); err != nil {
			return RoleWithPermissions{}, false, err
		}
	}
	if err := tx.Emit(ctx, roleAudit(s.d.IDs.NewID(), appmoderation.AuditRoleUpdated, role, in.ByUserID, in.At)); err != nil {
		return RoleWithPermissions{}, false, err
	}
	return next, true, nil
}

// GrantManagedRole grants a managed role to a user inside tx as the system and reports whether
// it did: a user who holds the role already is left as they are. It fails with ErrNotFound for
// an unknown role, and with ErrRoleNotManageable for a role that is not managed.
func (s *Service) GrantManagedRole(ctx context.Context, tx uow.Tx, userID, roleID string, at time.Time) (bool, error) {
	role, err := s.lockManaged(ctx, tx, roleID)
	if err != nil {
		return false, err
	}
	switch _, err := s.d.Repository.FindRoleGrantID(ctx, tx, userID, role.ID); {
	case err == nil:
		return false, nil
	case !errors.Is(err, ErrNotFound):
		return false, err
	}
	g := identity.RoleGrant{ID: s.d.IDs.NewID(), UserID: userID, RoleID: role.ID, GrantedAt: at}
	if err := s.d.Repository.InsertRoleGrant(ctx, tx, g); err != nil {
		return false, err
	}
	// A new grant starts with the badge hidden (ADR-0070 §11).
	if err := s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, userID, role.ID, false, at); err != nil {
		return false, err
	}
	return true, s.emitSystemRoleAudit(ctx, tx, appmoderation.AuditRoleGranted, role, userID, g.ID, at)
}

// RevokeManagedRole revokes a managed role from a user inside tx as the system and reports
// whether it did: a user who does not hold the role is left as they are. It fails like
// GrantManagedRole.
func (s *Service) RevokeManagedRole(ctx context.Context, tx uow.Tx, userID, roleID string, at time.Time) (bool, error) {
	role, err := s.lockManaged(ctx, tx, roleID)
	if err != nil {
		return false, err
	}
	grantID, err := s.d.Repository.FindRoleGrantID(ctx, tx, userID, role.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	}
	if err := s.d.Repository.InsertRoleRevocation(ctx, tx, identity.RoleRevocation{GrantID: grantID, RevokedAt: at}); err != nil {
		return false, err
	}
	// The choice to show the badge goes with the role (ADR-0070 §11).
	if err := s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, userID, role.ID, false, at); err != nil {
		return false, err
	}
	return true, s.emitSystemRoleAudit(ctx, tx, appmoderation.AuditRoleRevoked, role, userID, grantID, at)
}

// ReplaceManagedRole revokes one managed role from a user and grants another inside tx, for a
// change of plan (ADR-0072 §8): as ReplaceRole does (ADR-0070 §11), a user who shows the badge of
// the old role shows the badge of the new one, and the choice goes away when the new role has no
// badge. It fails like GrantManagedRole.
func (s *Service) ReplaceManagedRole(ctx context.Context, tx uow.Tx, userID, fromRoleID, toRoleID string, at time.Time) error {
	displayed, err := s.d.Repository.RoleBadgeDisplayed(ctx, tx, userID, fromRoleID)
	if err != nil {
		return err
	}
	if _, err := s.RevokeManagedRole(ctx, tx, userID, fromRoleID, at); err != nil {
		return err
	}
	if _, err := s.GrantManagedRole(ctx, tx, userID, toRoleID, at); err != nil {
		return err
	}
	_, hasBadge, err := s.d.Repository.RoleBadge(ctx, tx, toRoleID)
	if err != nil {
		return err
	}
	if !identity.KeepsBadgeDisplay(displayed, hasBadge) {
		return nil
	}
	return s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, userID, toRoleID, true, at)
}

// lockManaged locks a role by ID inside tx and checks that it is managed.
func (s *Service) lockManaged(ctx context.Context, tx uow.Tx, roleID string) (identity.Role, error) {
	role, err := s.d.Repository.LockRoleByID(ctx, tx, roleID)
	if err != nil {
		return identity.Role{}, err
	}
	managed, err := s.isManaged(ctx, tx, roleID)
	if err != nil {
		return identity.Role{}, err
	}
	if !managed {
		return identity.Role{}, fmt.Errorf("%w: role %s is not managed", ErrRoleNotManageable, role.Key)
	}
	return role, nil
}

// emitSystemRoleAudit records a grant or a revocation of a managed role by the system, labeled
// like the ones by hand (role key and the user's handle).
func (s *Service) emitSystemRoleAudit(ctx context.Context, tx uow.Tx, action appmoderation.AuditAction, role identity.Role, userID, grantID string, at time.Time) error {
	label := userID
	la, err := s.d.Accounts.LocalAccountOfUserTx(ctx, tx, userID)
	switch {
	case err == nil:
		label = "@" + la.Username
	case !errors.Is(err, account.ErrNotFound):
		return err
	}
	return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: appmoderation.AuditActorSystem, Action: action, TargetKind: appmoderation.AuditTargetRole,
		TargetID: role.ID, TargetLabel: role.Key + " " + label, SourceEventID: grantID, OccurredAt: at,
	}})
}

// checkUnmanaged checks inside tx a change of a role that is not managed (roleID "" for a new
// role) against the managed roles: it fails with ErrRoleManagedByPlan for a managed role and with
// ErrBreaksManagedRoles for a staff role at or below a managed one.
func (s *Service) checkUnmanaged(ctx context.Context, tx uow.Tx, roleID, key string, position int, perms []identity.Permission) error {
	roles, err := s.rolesTx(ctx, tx)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(roles, func(r ManagedRoleView) bool { return r.ID == roleID && roleID != "" && r.Managed }) {
		return fmt.Errorf("%w: %s", ErrRoleManagedByPlan, key)
	}
	return identity.CheckUnmanagedChange(roleID, key, position, perms, roleFacts(roles))
}

// refuseManaged fails with ErrRoleManagedByPlan inside tx when a module manages the role.
func (s *Service) refuseManaged(ctx context.Context, tx uow.Tx, role identity.Role) error {
	managed, err := s.isManaged(ctx, tx, role.ID)
	if err != nil {
		return err
	}
	if managed {
		return fmt.Errorf("%w: %s", ErrRoleManagedByPlan, role.Key)
	}
	return nil
}
