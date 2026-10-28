// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"time"
)

// InviteCreatorKind is how an invite was created (ADR-0108 §3, invites.creator_kind).
type InviteCreatorKind string

// Invite creator kinds.
const (
	// InviteByStaff: a holder of manage_invites, or the CLI. No cap limits these invites, and
	// a sign-up does not check their creator.
	InviteByStaff InviteCreatorKind = "staff"
	// InviteByMember: a member, through their own invites (ADR-0108 §1), under the caps of
	// MemberInviteCaps; every sign-up checks that the creator may still create invites (§3).
	InviteByMember InviteCreatorKind = "member"
)

// InviterFacts are what decides whether a user may create invites of their own (ADR-0108 §1),
// and whether the invites a member created still work (§3).
type InviterFacts struct {
	// Mode is the registration mode: no invite is made or used while it is closed (§4).
	Mode RegistrationMode
	// EveryoneInvites reports that the instance setting invite_creators is everyone.
	EveryoneInvites bool
	// HasPermission reports that the user's roles grant create_invites.
	HasPermission bool
	// Active reports that the user's local account exists and is not deactivated, scheduled
	// for deletion, or deleted.
	Active bool
	// Approved reports that the user's registration is approved (not pending or rejected).
	Approved bool
	// Restricted reports that moderation suspended or limited the account, or disabled its
	// login.
	Restricted bool
}

// MayCreateInvites reports whether a user may create invites of their own, and whether an
// invite a member created lets people sign up (ADR-0108 §1, §3): the registration is not
// closed, the account is active, approved, and neither suspended nor limited, and the instance
// lets everyone create invites or the user holds create_invites.
func MayCreateInvites(f InviterFacts) bool {
	return f.Mode != RegistrationClosed && f.Active && f.Approved && !f.Restricted && (f.EveryoneInvites || f.HasPermission)
}

// The defaults of the invites members create (ADR-0108 §2).
const (
	DefaultMemberInviteUses      = 1
	DefaultMemberInviteExpiresIn = 7 * 24 * time.Hour
)

// MemberInviteCaps bound the invites of one member (ADR-0108 §2): the instance settings
// member_invite_max_active, member_invite_max_uses, and member_invite_max_expires_in.
type MemberInviteCaps struct {
	// MaxActive is how many usable invites a member may hold at once; 0 lets nobody create
	// one.
	MaxActive int
	// MaxUses bounds the sign-ups of one invite.
	MaxUses int
	// MaxExpiresIn bounds how long one invite works.
	MaxExpiresIn time.Duration
}

// DefaultUses returns the sign-ups of an invite that does not name them: one, or the cap when
// it is lower.
func (c MemberInviteCaps) DefaultUses() int { return min(DefaultMemberInviteUses, c.MaxUses) }

// DefaultExpiresIn returns how long an invite that does not name it works: a week, or the cap
// when it is shorter.
func (c MemberInviteCaps) DefaultExpiresIn() time.Duration {
	return min(DefaultMemberInviteExpiresIn, c.MaxExpiresIn)
}

// Errors of the member invite caps.
var (
	// ErrInviteUsesOverCap reports sign-ups outside 1 to MaxUses.
	ErrInviteUsesOverCap = errors.New("identity: the uses of the invite are outside the cap")
	// ErrInviteExpiryOverCap reports a lifetime outside 1 second to MaxExpiresIn.
	ErrInviteExpiryOverCap = errors.New("identity: the expiry of the invite is outside the cap")
)

// Resolve returns the uses and the lifetime of a member's new invite: those asked for, or the
// defaults for zero values. Unlimited uses and no expiry cannot be asked for (ADR-0108 §2). It
// fails with ErrInviteUsesOverCap or ErrInviteExpiryOverCap for a value outside its cap.
func (c MemberInviteCaps) Resolve(uses int, expiresIn time.Duration) (int, time.Duration, error) {
	if uses == 0 {
		uses = c.DefaultUses()
	}
	if expiresIn == 0 {
		expiresIn = c.DefaultExpiresIn()
	}
	if uses < 1 || uses > c.MaxUses {
		return 0, 0, ErrInviteUsesOverCap
	}
	if expiresIn < time.Second || expiresIn > c.MaxExpiresIn {
		return 0, 0, ErrInviteExpiryOverCap
	}
	return uses, expiresIn, nil
}
