// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/identity"
)

// A user creates invites of their own while the registration is open to invites, the account
// is active, approved, and unrestricted, and everyone may or the user holds create_invites
// (ADR-0106 §14, §16, §17).
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

// A user who may not create invites has one cause, the first that holds of: the registration
// is closed, the user is not let, the registration is not approved, the account is not active
// or restricted (api/spec InvitesNotAllowedReason). Every set of causes is tried, so each
// cause wins over each later one alone and together.
func TestInviterFactsRefusal(t *testing.T) {
	ok := identity.InviterFacts{Mode: identity.RegistrationApproval, HasPermission: true, Active: true, Approved: true}
	everyone := ok
	everyone.HasPermission, everyone.EveryoneInvites = false, true
	for name, f := range map[string]identity.InviterFacts{"a permitted member": ok, "anyone while everyone invites": everyone} {
		for _, mode := range []identity.RegistrationMode{identity.RegistrationOpen, identity.RegistrationApproval, identity.RegistrationInvite} {
			f.Mode = mode
			if got := f.Refusal(); got != "" || !identity.MayCreateInvites(f) {
				t.Errorf("%s in %s: Refusal = %q, MayCreateInvites = %t; want none, true", name, mode, got, identity.MayCreateInvites(f))
			}
		}
	}

	causes := []struct {
		name string
		hold func(*identity.InviterFacts)
		want identity.InviteRefusal
	}{
		{"closed", func(f *identity.InviterFacts) { f.Mode = identity.RegistrationClosed }, identity.InviteRefusalRegistrationClosed},
		{"without permission", func(f *identity.InviterFacts) { f.HasPermission = false }, identity.InviteRefusalNotPermitted},
		{"pending", func(f *identity.InviterFacts) { f.Approved = false }, identity.InviteRefusalNotApproved},
		{"inactive", func(f *identity.InviterFacts) { f.Active = false }, identity.InviteRefusalRestricted},
		{"restricted", func(f *identity.InviterFacts) { f.Restricted = true }, identity.InviteRefusalRestricted},
	}
	for set := 1; set < 1<<len(causes); set++ {
		f, want, names := ok, identity.InviteRefusal(""), []string{}
		for i := len(causes) - 1; i >= 0; i-- {
			if set&(1<<i) != 0 {
				causes[i].hold(&f)
				want, names = causes[i].want, append([]string{causes[i].name}, names...)
			}
		}
		if got := f.Refusal(); got != want {
			t.Errorf("%s: Refusal = %q, want %q", strings.Join(names, ", "), got, want)
		}
		if identity.MayCreateInvites(f) {
			t.Errorf("%s: MayCreateInvites = true", strings.Join(names, ", "))
		}
	}
	// Everyone inviting takes the second cause away, and no other.
	pending := everyone
	pending.Approved = false
	if got := pending.Refusal(); got != identity.InviteRefusalNotApproved {
		t.Errorf("pending while everyone invites: Refusal = %q, want %q", got, identity.InviteRefusalNotApproved)
	}
}

// A member's invite is one use for a week unless the caps are lower, never unlimited, and
// within the caps (ADR-0106 §15).
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
