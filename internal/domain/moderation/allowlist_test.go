// SPDX-License-Identifier: AGPL-3.0-only

package moderation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/moderation"
)

// TestAllowed lets a domain federate when it or a parent domain is allowed, and the local domain
// always.
func TestAllowed(t *testing.T) {
	allowed := []string{"friend.example", "partner.org"}
	for domain, want := range map[string]bool{
		"friend.example": true, "a.b.friend.example": true, "partner.org": true, "": true,
		"notfriend.example": false, "example": false, "stranger.example": false, "org": false,
	} {
		if got := moderation.Allowed(domain, allowed); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", domain, got, want)
		}
	}
	if moderation.Allowed("friend.example", nil) {
		t.Error("Allowed with an empty allowlist = true, want false")
	}
}

// TestDomainAllowValidate refuses a domain that is not canonical and a comment that is too long.
func TestDomainAllowValidate(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	ok := moderation.DomainAllow{Domain: "friend.example", Comment: strings.Repeat("あ", moderation.MaxDomainAllowComment), CreatedAt: now}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate(%s) = %v", ok.Domain, err)
	}
	for _, a := range []moderation.DomainAllow{
		{Domain: "", CreatedAt: now},
		{Domain: "Friend.example", CreatedAt: now},
		{Domain: "friend.example:443", CreatedAt: now},
		{Domain: "friend.example", Comment: strings.Repeat("x", moderation.MaxDomainAllowComment+1), CreatedAt: now},
		{Domain: "friend.example", Comment: "a\x00b", CreatedAt: now},
	} {
		if err := a.Validate(); !errors.Is(err, moderation.ErrInvalidDomainAllow) {
			t.Errorf("Validate(%q, %d characters) = %v, want ErrInvalidDomainAllow", a.Domain, len(a.Comment), err)
		}
	}
}

// TestAllowlistCutoffValidate names a domain exactly for the removal of an allow.
func TestAllowlistCutoffValidate(t *testing.T) {
	for _, tc := range []struct {
		c  moderation.AllowlistCutoff
		ok bool
	}{
		{moderation.AllowlistCutoff{ID: "1", Cause: moderation.CutoffScopeSwitched}, true},
		{moderation.AllowlistCutoff{ID: "1", Cause: moderation.CutoffAllowRemoved, Domain: "friend.example"}, true},
		{moderation.AllowlistCutoff{ID: "1", Cause: moderation.CutoffScopeSwitched, Domain: "friend.example"}, false},
		{moderation.AllowlistCutoff{ID: "1", Cause: moderation.CutoffAllowRemoved}, false},
		{moderation.AllowlistCutoff{ID: "1", Cause: "other"}, false},
		{moderation.AllowlistCutoff{Cause: moderation.CutoffScopeSwitched}, false},
	} {
		if err := tc.c.Validate(); (err == nil) != tc.ok {
			t.Errorf("Validate(%+v) = %v, want ok %v", tc.c, err, tc.ok)
		}
	}
}

// TestSeveranceOfACutoff takes a cutoff as the one action of a severance.
func TestSeveranceOfACutoff(t *testing.T) {
	sv := moderation.Severance{ID: "1", AllowlistCutoffID: "2", AccountID: "3", FollowersLost: 1}
	if err := sv.Validate(); err != nil {
		t.Errorf("Validate(cutoff) = %v", err)
	}
	sv.DomainActionID = "4"
	if err := sv.Validate(); !errors.Is(err, moderation.ErrInvalidSeverance) {
		t.Errorf("Validate(cutoff and domain action) = %v, want ErrInvalidSeverance", err)
	}
}
