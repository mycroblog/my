// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"time"
)

// Granting and revoking roles (FR-720). A user who manages roles may grant and revoke only the
// roles at or below the highest position of their own, and grant only the roles whose every
// permission they hold (CheckPermissionsHeld); the command line may grant and revoke any role.
// The admin role always keeps at least one holder.

// RoleRevocation ends a role grant (data-model §4.1, user_role_revocations).
type RoleRevocation struct {
	GrantID string
	// RevokedByUserID is the user who revoked the role, or "" for a revocation from the CLI.
	RevokedByUserID string
	RevokedAt       time.Time
}

// ErrRoleOutranksActor reports a role above every role of the user who grants or revokes it.
var ErrRoleOutranksActor = errors.New("identity: the role outranks the roles of the user who changes it")

// ErrLastAdmin reports the revocation of the admin role from its only holder.
var ErrLastAdmin = errors.New("identity: the last holder of the admin role cannot lose it")

// ErrTargetOutranksActor reports a moderation or recovery operation on an account whose user does
// not rank below the user who runs it.
var ErrTargetOutranksActor = errors.New("identity: the target's roles do not rank below those of the user who acts on it")

// RanksAbove reports whether a user who holds actorRoles ranks above a user who holds targetRoles:
// the highest position of the former is above that of the latter. A user without roles ranks below
// every user with one, and equal ranks do not rank above each other.
func RanksAbove(actorRoles, targetRoles []Role) bool {
	if len(actorRoles) == 0 {
		return false
	}
	actor := actorRoles[0].Position
	for _, r := range actorRoles {
		actor = max(actor, r.Position)
	}
	for _, r := range targetRoles {
		if r.Position >= actor {
			return false
		}
	}
	return true
}

// CanManageRole reports whether a user who holds actorRoles may grant or revoke role: its
// position is at most the highest position among actorRoles. A user without roles manages none.
func CanManageRole(actorRoles []Role, role Role) bool {
	for _, r := range actorRoles {
		if r.Position >= role.Position {
			return true
		}
	}
	return false
}
