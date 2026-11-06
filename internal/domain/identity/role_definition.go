// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Creating, changing, and deleting roles (FR-720, manage_roles). The built-in roles keep their
// keys and are never deleted; admin keeps every permission and the position AdminPosition, so
// that it stays the way back to every operation. The other roles, moderator included, take a
// position from 0 to MaxRolePosition, below admin. A user changes only the roles at or below
// their highest one (CanManageRole), and adds or removes only the permissions they hold
// themselves; the command line may do anything.

// AdminPosition is the position of the built-in admin role, above every other role.
const AdminPosition = 1000

// MaxRolePosition is the highest position of a role other than admin.
const MaxRolePosition = AdminPosition - 1

// MaxRoleKeyLength is the length limit of a role's key.
const MaxRoleKeyLength = 64

// ReservedRoleKey is the one key a role cannot have: the web client's page that creates a role is
// /admin/roles/new, beside the page of each role at /admin/roles/{key}.
const ReservedRoleKey = "new"

// ErrInvalidRoleKey reports a key that is not 1 to MaxRoleKeyLength lowercase ASCII letters,
// digits, underscores, and hyphens, or that is ReservedRoleKey.
var ErrInvalidRoleKey = errors.New(`identity: a role key is 1 to 64 lowercase letters, digits, underscores, or hyphens, other than "new"`)

// ErrInvalidRolePosition reports a position outside 0 to MaxRolePosition.
var ErrInvalidRolePosition = errors.New("identity: a role's position is from 0 to 999")

// ErrInvalidRolePermissions reports an unknown permission, or one given twice.
var ErrInvalidRolePermissions = errors.New("identity: a role's permissions are known permissions, each once")

// ErrRoleKeyTaken reports a new role whose key another role has.
var ErrRoleKeyTaken = errors.New("identity: another role has the key")

// ErrBuiltinRole reports a change that the built-in roles do not allow: deleting admin or
// moderator, or changing admin.
var ErrBuiltinRole = errors.New("identity: the built-in role does not allow the change")

// ErrRoleHeld reports the deletion of a role that users hold; revoke it from them first.
var ErrRoleHeld = errors.New("identity: users hold the role")

// MissingPermissionsError reports a change to permissions that the user who makes it does not
// hold.
type MissingPermissionsError struct {
	Missing []Permission
}

func (e *MissingPermissionsError) Error() string {
	names := make([]string, len(e.Missing))
	for i, p := range e.Missing {
		names[i] = string(p)
	}
	return "identity: the change needs the permissions " + strings.Join(names, ", ")
}

// IsBuiltinRole reports whether key names a built-in role.
func IsBuiltinRole(key string) bool {
	return key == RoleAdmin || key == RoleModerator
}

// ValidateRoleKey checks the key of a new role.
func ValidateRoleKey(key string) error {
	if key == "" || len(key) > MaxRoleKeyLength || key == ReservedRoleKey {
		return ErrInvalidRoleKey
	}
	for _, c := range key {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return ErrInvalidRoleKey
		}
	}
	return nil
}

// ValidateRolePosition checks the position of a role other than admin.
func ValidateRolePosition(position int) error {
	if position < 0 || position > MaxRolePosition {
		return ErrInvalidRolePosition
	}
	return nil
}

// NormalizeRolePermissions checks perms and returns them in the order of AllPermissions.
func NormalizeRolePermissions(perms []Permission) ([]Permission, error) {
	all := AllPermissions()
	seen := make(map[Permission]bool, len(perms))
	for _, p := range perms {
		if !slices.Contains(all, p) || seen[p] {
			return nil, fmt.Errorf("%w: %q", ErrInvalidRolePermissions, p)
		}
		seen[p] = true
	}
	out := make([]Permission, 0, len(perms))
	for _, p := range all {
		if seen[p] {
			out = append(out, p)
		}
	}
	return out, nil
}

// ChangedPermissions returns the permissions that are in exactly one of before and after, in the
// order of AllPermissions.
func ChangedPermissions(before, after []Permission) []Permission {
	var out []Permission
	for _, p := range AllPermissions() {
		if slices.Contains(before, p) != slices.Contains(after, p) {
			out = append(out, p)
		}
	}
	return out
}

// CheckPermissionsHeld returns a *MissingPermissionsError when held lacks one of perms.
func CheckPermissionsHeld(held, perms []Permission) error {
	var missing []Permission
	for _, p := range perms {
		if !slices.Contains(held, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return &MissingPermissionsError{Missing: missing}
	}
	return nil
}
