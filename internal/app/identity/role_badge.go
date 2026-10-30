// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"slices"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Role badges and their display on profiles (ADR-0071, FR-913). The badge of a role is a current
// value (role_badges) that UpdateRole sets and removes; the choice of a user to show it on their
// profile is another (user_role_badge_displays), off by default. Revoking a role, granting it
// anew, and removing its badge clear the choice in the same transaction, so that a user who
// gets the role back starts from the hidden badge again. ReplaceRole, for a change of plan,
// moves the choice from the old role to the new one. The badges are never federated.

// The errors of the badges, from internal/domain/identity.
var (
	ErrInvalidBadgeLabel = identity.ErrInvalidBadgeLabel
	ErrInvalidBadgeColor = identity.ErrInvalidBadgeColor
)

// RoleBadgeRepository is the persistence of role badges and of their display, part of
// RoleRepository.
type RoleBadgeRepository interface {
	// RoleBadge returns inside tx the badge of the role, and false when it has none.
	RoleBadge(ctx context.Context, tx uow.Tx, roleID string) (identity.RoleBadge, bool, error)
	// SetRoleBadge gives the role the badge b, replacing the badge it has.
	SetRoleBadge(ctx context.Context, tx uow.Tx, roleID string, b identity.RoleBadge) error
	// DeleteRoleBadge removes the badge of the role and every user's choice to show it.
	DeleteRoleBadge(ctx context.Context, tx uow.Tx, roleID string) error
	// RoleBadgeDisplayed reports inside tx whether the user chose to show the badge of the role.
	RoleBadgeDisplayed(ctx context.Context, tx uow.Tx, userID, roleID string) (bool, error)
	// SetRoleBadgeDisplayed records whether the user shows the badge of the role; showing it
	// again, or hiding a hidden one, changes nothing.
	SetRoleBadgeDisplayed(ctx context.Context, tx uow.Tx, userID, roleID string, displayed bool, at time.Time) error
	// ListBadgeChoices returns the roles that the user holds and that have a badge, with whether
	// the user shows it, highest position first.
	ListBadgeChoices(ctx context.Context, userID string) ([]identity.BadgeChoice, error)
	// ListProfileBadges returns the badges that the user shows: of the roles they hold, that have
	// a badge, and that they chose to show, highest position first, in one read.
	ListProfileBadges(ctx context.Context, userID string) ([]identity.ProfileBadge, error)
}

// BadgeInput is the badge to give a role.
type BadgeInput struct {
	Label string
	Color identity.BadgeColor
}

// ProfileBadges returns the badges that a user shows on their profile, highest position first.
func (s *Service) ProfileBadges(ctx context.Context, userID string) ([]identity.ProfileBadge, error) {
	return s.d.Repository.ListProfileBadges(ctx, userID)
}

// BadgeChoices returns the roles that a user holds and that have a badge, with whether the user
// shows it on their profile, highest position first.
func (s *Service) BadgeChoices(ctx context.Context, userID string) ([]identity.BadgeChoice, error) {
	return s.d.Repository.ListBadgeChoices(ctx, userID)
}

// SetBadgeDisplayed records whether a user shows the badge of the role with ID roleID on their
// profile, and returns the choice afterwards. It fails with ErrNotFound when the user does not
// hold the role or the role has no badge.
func (s *Service) SetBadgeDisplayed(ctx context.Context, userID, roleID string, displayed bool) (identity.BadgeChoice, error) {
	held, err := s.d.Repository.ListUserRoles(ctx, userID)
	if err != nil {
		return identity.BadgeChoice{}, err
	}
	i := slices.IndexFunc(held, func(r identity.Role) bool { return r.ID == roleID })
	if i < 0 {
		return identity.BadgeChoice{}, ErrNotFound
	}
	key := held[i].Key
	now := s.d.Clock.Now()
	var out identity.BadgeChoice
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		// The lock orders the choice after a revocation or a badge change of the same role.
		role, err := s.d.Repository.LockRole(ctx, tx, key)
		if err != nil {
			return err
		}
		if _, err := s.d.Repository.FindRoleGrantID(ctx, tx, userID, role.ID); err != nil {
			return err
		}
		b, ok, err := s.d.Repository.RoleBadge(ctx, tx, role.ID)
		switch {
		case err != nil:
			return err
		case !ok:
			return ErrNotFound
		}
		if err := s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, userID, role.ID, displayed, now); err != nil {
			return err
		}
		out = identity.BadgeChoice{ProfileBadge: identity.ProfileBadge{RoleID: role.ID, Label: b.Label, Color: b.Color}, Displayed: displayed}
		return nil
	})
	if err != nil {
		return identity.BadgeChoice{}, err
	}
	return out, nil
}

// setRoleBadge applies the badge change of UpdateRole inside its transaction: it sets badge, or
// removes the badge (and every user's choice to show it) for nil, unless badge is current.
func (s *Service) setRoleBadge(ctx context.Context, tx uow.Tx, roleID string, current, badge *identity.RoleBadge) error {
	switch {
	case identity.SameBadge(current, badge):
		return nil
	case badge == nil:
		return s.d.Repository.DeleteRoleBadge(ctx, tx, roleID)
	}
	return s.d.Repository.SetRoleBadge(ctx, tx, roleID, *badge)
}

// ReplaceRoleInput names the role of a user to revoke and the role to grant in its place.
type ReplaceRoleInput struct {
	UserID string
	// FromRoleKey is the role to revoke; a user who does not hold it only gets ToRoleKey.
	FromRoleKey string
	// ToRoleKey is the role to grant; a user who holds it already keeps it.
	ToRoleKey string
	// ByUserID is the user who changes the roles, or "" for the CLI and the other modules (a
	// change of plan). A user may change only the roles at or below their highest one.
	ByUserID string
}

// ReplaceRole revokes a role from a user and grants another in one transaction, for a change of
// plan (ADR-0071 §2): when the user shows the badge of the old role, they show the badge of the
// new one, and the choice goes away when the new role has no badge. It reports whether it
// changed anything and records the audit log entries of the revocation and the grant. It fails
// as RevokeRole and GrantRole do; the roles of the supporter plans change through
// ReplaceManagedRole instead.
func (s *Service) ReplaceRole(ctx context.Context, in ReplaceRoleInput) (bool, error) {
	if in.FromRoleKey == in.ToRoleKey {
		return s.GrantRole(ctx, RoleChangeInput{UserID: in.UserID, RoleKey: in.ToRoleKey, ByUserID: in.ByUserID})
	}
	from, err := s.prepareRoleChange(ctx, RoleChangeInput{UserID: in.UserID, RoleKey: in.FromRoleKey, ByUserID: in.ByUserID})
	if err != nil {
		return false, err
	}
	to, err := s.prepareRoleChange(ctx, RoleChangeInput{UserID: in.UserID, RoleKey: in.ToRoleKey, ByUserID: in.ByUserID})
	if err != nil {
		return false, err
	}
	to.at = from.at
	changed := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		changed = false
		// Lock the two roles in the order of their keys, so that two replacements never wait on
		// each other.
		roles := map[string]identity.Role{}
		for _, key := range sortedKeys(in.FromRoleKey, in.ToRoleKey) {
			r, err := s.d.Repository.LockRole(ctx, tx, key)
			if err != nil {
				return err
			}
			if err := s.refuseManaged(ctx, tx, r); err != nil {
				return err
			}
			roles[key] = r
		}
		oldRole, newRole := roles[in.FromRoleKey], roles[in.ToRoleKey]
		displayed, revoked, err := s.revokeReplaced(ctx, tx, from, oldRole)
		if err != nil {
			return err
		}
		_, hasBadge, err := s.d.Repository.RoleBadge(ctx, tx, newRole.ID)
		if err != nil {
			return err
		}
		keep := identity.KeepsBadgeDisplay(displayed, hasBadge)
		granted := false
		switch _, err := s.d.Repository.FindRoleGrantID(ctx, tx, in.UserID, newRole.ID); {
		case errors.Is(err, ErrNotFound):
			g := identity.RoleGrant{ID: to.eventID, UserID: in.UserID, RoleID: newRole.ID, GrantedByUserID: in.ByUserID, GrantedAt: to.at}
			if err := s.d.Repository.InsertRoleGrant(ctx, tx, g); err != nil {
				return err
			}
			if err := tx.Emit(ctx, to.audit(appmoderation.AuditRoleGranted, newRole, g.ID)); err != nil {
				return err
			}
			granted = true
		case err != nil:
			return err
		}
		// A new grant starts from the moved choice; a role held already keeps its own choice
		// unless the moved one shows the badge.
		if granted || keep {
			if err := s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, in.UserID, newRole.ID, keep, to.at); err != nil {
				return err
			}
		}
		changed = revoked || granted
		return nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

// revokeReplaced revokes the old role of ReplaceRole inside its transaction, and reports whether
// the user showed its badge and whether the user held it.
func (s *Service) revokeReplaced(ctx context.Context, tx uow.Tx, c roleChange, role identity.Role) (displayed, revoked bool, err error) {
	grantID, err := s.d.Repository.FindRoleGrantID(ctx, tx, c.in.UserID, role.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		return false, false, nil
	case err != nil:
		return false, false, err
	}
	if role.Key == identity.RoleAdmin {
		n, err := s.d.Repository.CountRoleHolders(ctx, tx, role.ID)
		if err != nil {
			return false, false, err
		}
		if n <= 1 {
			return false, false, ErrLastAdmin
		}
	}
	displayed, err = s.d.Repository.RoleBadgeDisplayed(ctx, tx, c.in.UserID, role.ID)
	if err != nil {
		return false, false, err
	}
	r := identity.RoleRevocation{GrantID: grantID, RevokedByUserID: c.in.ByUserID, RevokedAt: c.at}
	if err := s.d.Repository.InsertRoleRevocation(ctx, tx, r); err != nil {
		return false, false, err
	}
	if err := s.d.Repository.SetRoleBadgeDisplayed(ctx, tx, c.in.UserID, role.ID, false, c.at); err != nil {
		return false, false, err
	}
	return displayed, true, tx.Emit(ctx, c.audit(appmoderation.AuditRoleRevoked, role, grantID))
}

// sortedKeys returns keys in order.
func sortedKeys(keys ...string) []string {
	slices.Sort(keys)
	return keys
}
