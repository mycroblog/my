// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/identity"
)

// A user creates invites of their own while the registration is open to invites, the account
// is active, approved, and unrestricted, and everyone may or the user holds create_invites
// (ADR-0108 §1, §3, §4).
func TestMayCreateInvites(t *testing.T) {
	ok := identity.InviterFacts{Mode: identity.RegistrationApproval, HasPermission: true, Active: true, Approved: true}
	if !identity.MayCreateInvites(ok) {
		t.Errorf("a permitted member: false")
	}
	everyone := ok
	everyone.HasPermission, everyone.EveryoneInvites = false, true
	if !identity.MayCreateInvites(everyone) {
		t.Errorf("anyone while everyone invites: false")
	}
	for name, change := range map[string]func(*identity.InviterFacts){
		"closed":             func(f *identity.InviterFacts) { f.Mode = identity.RegistrationClosed },
		"without permission": func(f *identity.InviterFacts) { f.HasPermission = false },
		"inactive":           func(f *identity.InviterFacts) { f.Active = false },
		"pending":            func(f *identity.InviterFacts) { f.Approved = false },
		"restricted":         func(f *identity.InviterFacts) { f.Restricted = true },
	} {
		f := ok
		change(&f)
		if identity.MayCreateInvites(f) {
			t.Errorf("%s: true", name)
		}
	}
}

// A member's invite is one use for a week unless the caps are lower, never unlimited, and
// within the caps (ADR-0108 §2).
func TestMemberInviteCapsResolve(t *testing.T) {
	caps := identity.MemberInviteCaps{MaxActive: 5, MaxUses: 10, MaxExpiresIn: 30 * 24 * time.Hour}
	if uses, in, err := caps.Resolve(0, 0); err != nil || uses != 1 || in != 7*24*time.Hour {
		t.Errorf("defaults = %d, %v, %v; want 1, a week", uses, in, err)
	}
	if uses, in, err := caps.Resolve(10, 30*24*time.Hour); err != nil || uses != 10 || in != 30*24*time.Hour {
		t.Errorf("at the caps = %d, %v, %v", uses, in, err)
	}
	if _, _, err := caps.Resolve(11, 0); !errors.Is(err, identity.ErrInviteUsesOverCap) {
		t.Errorf("over max uses: %v", err)
	}
	if _, _, err := caps.Resolve(-1, 0); !errors.Is(err, identity.ErrInviteUsesOverCap) {
		t.Errorf("negative uses: %v", err)
	}
	if _, _, err := caps.Resolve(0, 31*24*time.Hour); !errors.Is(err, identity.ErrInviteExpiryOverCap) {
		t.Errorf("over max expiry: %v", err)
	}
	low := identity.MemberInviteCaps{MaxActive: 1, MaxUses: 1, MaxExpiresIn: time.Hour}
	if uses, in, err := low.Resolve(0, 0); err != nil || uses != 1 || in != time.Hour {
		t.Errorf("defaults under low caps = %d, %v, %v; want 1, an hour", uses, in, err)
	}
}
