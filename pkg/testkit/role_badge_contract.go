// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"reflect"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The role badges of RoleRepositoryContract (ADR-0071).

// createBadgeRole creates a role at position with a badge (none for an empty label).
func (e *roleEnv) createBadgeRole(t *testing.T, key string, position int, label string, color identity.BadgeColor) identity.Role {
	t.Helper()
	r, err := e.identity.CreateRole(t.Context(), appidentity.CreateRoleInput{Key: key, Position: position})
	if err != nil {
		t.Fatalf("CreateRole(%s) = %v", key, err)
	}
	if label != "" {
		e.setBadge(t, key, &appidentity.BadgeInput{Label: label, Color: color})
	}
	return r.Role
}

// setBadge gives the role the badge b, or removes its badge for nil, from the CLI.
func (e *roleEnv) setBadge(t *testing.T, key string, b *appidentity.BadgeInput) appidentity.RoleWithPermissions {
	t.Helper()
	r, err := e.identity.UpdateRole(t.Context(), appidentity.UpdateRoleInput{Key: key, Badge: b, SetBadge: true})
	if err != nil {
		t.Fatalf("UpdateRole(%s, badge %+v) = %v", key, b, err)
	}
	return r
}

func (e *roleEnv) display(t *testing.T, userID, roleID string, displayed bool) {
	t.Helper()
	if _, err := e.identity.SetBadgeDisplayed(t.Context(), userID, roleID, displayed); err != nil {
		t.Fatalf("SetBadgeDisplayed(%s, %s, %v) = %v", userID, roleID, displayed, err)
	}
}

func (e *roleEnv) profileBadges(t *testing.T, userID string) []identity.ProfileBadge {
	t.Helper()
	got, err := e.identity.ProfileBadges(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (e *roleEnv) badgeChoices(t *testing.T, userID string) []identity.BadgeChoice {
	t.Helper()
	got, err := e.identity.BadgeChoices(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func roleBadgesAreShownByChoice(t *testing.T, e *roleEnv) {
	ctx := t.Context()
	bob := e.register(t, "bob").User.ID
	supporter := e.createBadgeRole(t, "supporter", 10, "Supporter", identity.BadgeGreen)
	staff := e.createBadgeRole(t, "staff", 20, "", "")
	roles, err := e.identity.Roles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		switch r.Key {
		case "supporter":
			if r.Badge == nil || r.Badge.Label != "Supporter" || r.Badge.Color != identity.BadgeGreen {
				t.Errorf("Roles: supporter's badge = %+v, want Supporter in green", r.Badge)
			}
		default:
			if r.Badge != nil {
				t.Errorf("Roles: %s's badge = %+v, want none", r.Key, r.Badge)
			}
		}
	}
	// Holding nothing, there is nothing to show or choose.
	if _, err := e.identity.SetBadgeDisplayed(ctx, bob, supporter.ID, true); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("SetBadgeDisplayed(not held) = %v, want ErrNotFound", err)
	}
	e.grant(t, bob, "supporter", "")
	e.grant(t, bob, "staff", "")
	if _, err := e.identity.SetBadgeDisplayed(ctx, bob, staff.ID, true); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("SetBadgeDisplayed(no badge) = %v, want ErrNotFound", err)
	}
	// Off by default.
	shown := identity.ProfileBadge{RoleID: supporter.ID, Label: "Supporter", Color: identity.BadgeGreen}
	if got := e.badgeChoices(t, bob); !reflect.DeepEqual(got, []identity.BadgeChoice{{ProfileBadge: shown}}) {
		t.Errorf("BadgeChoices = %+v, want supporter, hidden", got)
	}
	if got := e.profileBadges(t, bob); len(got) != 0 {
		t.Errorf("ProfileBadges before the choice = %+v, want none", got)
	}
	e.display(t, bob, supporter.ID, true)
	e.display(t, bob, supporter.ID, true) // twice changes nothing
	if got := e.profileBadges(t, bob); !reflect.DeepEqual(got, []identity.ProfileBadge{shown}) {
		t.Errorf("ProfileBadges = %+v, want supporter", got)
	}
	if got := e.badgeChoices(t, bob); !reflect.DeepEqual(got, []identity.BadgeChoice{{ProfileBadge: shown, Displayed: true}}) {
		t.Errorf("BadgeChoices = %+v, want supporter, shown", got)
	}
	// Another user's choice is their own.
	carol := e.register(t, "carol").User.ID
	e.grant(t, carol, "supporter", "")
	if got := e.profileBadges(t, carol); len(got) != 0 {
		t.Errorf("carol's ProfileBadges = %+v, want none", got)
	}
	// The staff role gets a badge above supporter's; bob shows both, highest first.
	e.setBadge(t, "staff", &appidentity.BadgeInput{Label: " Staff ", Color: identity.BadgePurple})
	e.display(t, bob, staff.ID, true)
	staffBadge := identity.ProfileBadge{RoleID: staff.ID, Label: "Staff", Color: identity.BadgePurple}
	if got := e.profileBadges(t, bob); !reflect.DeepEqual(got, []identity.ProfileBadge{staffBadge, shown}) {
		t.Errorf("ProfileBadges = %+v, want staff and supporter", got)
	}
	e.display(t, bob, staff.ID, false)
	if got := e.profileBadges(t, bob); !reflect.DeepEqual(got, []identity.ProfileBadge{shown}) {
		t.Errorf("ProfileBadges after hiding staff = %+v, want supporter", got)
	}
	// A changed badge keeps the choice.
	e.setBadge(t, "supporter", &appidentity.BadgeInput{Label: "Fan", Color: identity.BadgePink})
	if got := e.profileBadges(t, bob); len(got) != 1 || got[0].Label != "Fan" || got[0].Color != identity.BadgePink {
		t.Errorf("ProfileBadges after the change = %+v, want Fan in pink", got)
	}

	// The revocation clears the choice, and a new grant starts hidden.
	e.revoke(t, bob, "supporter", "")
	if got := e.profileBadges(t, bob); len(got) != 0 {
		t.Errorf("ProfileBadges after the revocation = %+v, want none", got)
	}
	e.grant(t, bob, "supporter", "")
	if got := e.badgeChoices(t, bob); len(got) != 2 || got[1].RoleID != supporter.ID || got[1].Displayed {
		t.Errorf("BadgeChoices after a new grant = %+v, want supporter hidden", got)
	}
	// Removing the badge clears every choice to show it; a new badge starts hidden.
	e.display(t, bob, supporter.ID, true)
	removed := e.setBadge(t, "supporter", nil)
	if removed.Badge != nil {
		t.Errorf("UpdateRole(remove the badge) = %+v, want no badge", removed)
	}
	if got := e.badgeChoices(t, bob); len(got) != 1 || got[0].RoleID != staff.ID {
		t.Errorf("BadgeChoices after removing the badge = %+v, want staff only", got)
	}
	e.setBadge(t, "supporter", &appidentity.BadgeInput{Label: "Supporter", Color: identity.BadgeGreen})
	if got := e.profileBadges(t, bob); len(got) != 0 {
		t.Errorf("ProfileBadges after a new badge = %+v, want none", got)
	}

	// Deleting the role deletes its badge.
	e.revoke(t, bob, "supporter", "")
	e.revoke(t, carol, "supporter", "")
	if err := e.identity.DeleteRole(ctx, "supporter", ""); err != nil {
		t.Fatal(err)
	}
	again := e.createBadgeRole(t, "supporter", 10, "", "")
	if roles, err := e.identity.Roles(ctx); err != nil {
		t.Fatal(err)
	} else {
		for _, r := range roles {
			if r.ID == again.ID && r.Badge != nil {
				t.Errorf("a new role with the deleted one's key has the badge %+v", r.Badge)
			}
		}
	}
}

func roleBadgeChanges(t *testing.T, e *roleEnv) {
	ctx := t.Context()
	e.createBadgeRole(t, "helper", 10, "", "")
	before := len(e.audit.all())
	for _, tc := range []struct {
		in   appidentity.BadgeInput
		want error
	}{
		{appidentity.BadgeInput{Label: "", Color: identity.BadgeRed}, appidentity.ErrInvalidBadgeLabel},
		{appidentity.BadgeInput{Label: "two\nlines", Color: identity.BadgeRed}, appidentity.ErrInvalidBadgeLabel},
		{appidentity.BadgeInput{Label: "Helper", Color: "gold"}, appidentity.ErrInvalidBadgeColor},
	} {
		if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: "helper", Badge: &tc.in, SetBadge: true}); !errors.Is(err, tc.want) {
			t.Errorf("UpdateRole(badge %+v) = %v, want %v", tc.in, err, tc.want)
		}
	}
	// A badge on admin is allowed, but nothing else of admin changes.
	e.clock.Advance(time.Second)
	r, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{
		Key: identity.RoleAdmin, Badge: &appidentity.BadgeInput{Label: "Admin", Color: identity.BadgeRed}, SetBadge: true, ByUserID: e.admin,
	})
	if err != nil || r.Badge == nil || r.Badge.Label != "Admin" || r.Position != identity.AdminPosition {
		t.Errorf("UpdateRole(admin's badge) = %+v, %v; want the badge Admin", r, err)
	}
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: identity.RoleAdmin, ByUserID: e.admin}); !errors.Is(err, appidentity.ErrBuiltinRole) {
		t.Errorf("UpdateRole(admin, nothing) = %v, want ErrBuiltinRole", err)
	}
	// The same badge again records nothing.
	e.setBadge(t, identity.RoleAdmin, &appidentity.BadgeInput{Label: "Admin", Color: identity.BadgeRed})
	got := e.audit.all()[before:]
	if len(got) != 1 || got[0].Action != moderation.AuditRoleUpdated || got[0].TargetLabel != identity.RoleAdmin {
		t.Errorf("audit entries = %+v, want one role_updated on admin", got)
	}
	// A user below admin cannot change its badge.
	bob := e.register(t, "bob").User.ID
	e.grant(t, bob, identity.RoleModerator, "")
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: identity.RoleAdmin, SetBadge: true, ByUserID: bob}); !errors.Is(err, appidentity.ErrRoleOutranksActor) {
		t.Errorf("UpdateRole(admin's badge by a moderator) = %v, want ErrRoleOutranksActor", err)
	}
}

func roleReplacesAndMovesTheBadge(t *testing.T, e *roleEnv) {
	ctx := t.Context()
	bob := e.register(t, "bob").User.ID
	small := e.createBadgeRole(t, "small", 10, "Small", identity.BadgeBlue)
	medium := e.createBadgeRole(t, "medium", 20, "Medium", identity.BadgeTeal)
	e.createBadgeRole(t, "large", 30, "", "")
	held := func() []string {
		t.Helper()
		roles, err := e.identity.UserRoles(ctx, bob)
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		for _, r := range roles {
			keys = append(keys, r.Key)
		}
		return keys
	}
	replace := func(from, to string) bool {
		t.Helper()
		changed, err := e.identity.ReplaceRole(ctx, appidentity.ReplaceRoleInput{UserID: bob, FromRoleKey: from, ToRoleKey: to})
		if err != nil {
			t.Fatalf("ReplaceRole(%s, %s) = %v", from, to, err)
		}
		return changed
	}

	e.grant(t, bob, "small", "")
	e.display(t, bob, small.ID, true)
	before := len(e.audit.all())
	if !replace("small", "medium") {
		t.Error("ReplaceRole(small, medium) changed nothing")
	}
	if got := held(); !reflect.DeepEqual(got, []string{"medium"}) {
		t.Errorf("roles after small → medium = %v, want medium", got)
	}
	want := []identity.ProfileBadge{{RoleID: medium.ID, Label: "Medium", Color: identity.BadgeTeal}}
	if got := e.profileBadges(t, bob); !reflect.DeepEqual(got, want) {
		t.Errorf("ProfileBadges after small → medium = %+v, want medium's: the choice moves", got)
	}
	audit := e.audit.all()[before:]
	if len(audit) != 2 || audit[0].Action != moderation.AuditRoleRevoked || audit[0].TargetID != small.ID ||
		audit[1].Action != moderation.AuditRoleGranted || audit[1].TargetID != medium.ID {
		t.Errorf("audit entries of the replacement = %+v, want the revocation of small and the grant of medium", audit)
	}

	// The new role has no badge: the choice goes away, and does not come back with a badge.
	replace("medium", "large")
	if got := held(); !reflect.DeepEqual(got, []string{"large"}) {
		t.Errorf("roles after medium → large = %v, want large", got)
	}
	if got := e.badgeChoices(t, bob); len(got) != 0 {
		t.Errorf("BadgeChoices after medium → large = %+v, want none", got)
	}
	e.setBadge(t, "large", &appidentity.BadgeInput{Label: "Large", Color: identity.BadgeOrange})
	if got := e.profileBadges(t, bob); len(got) != 0 {
		t.Errorf("ProfileBadges after large gets a badge = %+v, want none", got)
	}

	// A hidden badge stays hidden.
	replace("large", "small")
	if got := e.badgeChoices(t, bob); len(got) != 1 || got[0].RoleID != small.ID || got[0].Displayed {
		t.Errorf("BadgeChoices after large → small = %+v, want small hidden", got)
	}
	// Without the old role, the new one is only granted; replacing with a held role changes nothing.
	if !replace("medium", "large") {
		t.Error("ReplaceRole(not held, large) changed nothing")
	}
	if got := held(); !reflect.DeepEqual(got, []string{"large", "small"}) {
		t.Errorf("roles = %v, want large and small", got)
	}
	if replace("medium", "small") {
		t.Error("ReplaceRole(not held, held) changed something")
	}
	if _, err := e.identity.ReplaceRole(ctx, appidentity.ReplaceRoleInput{UserID: bob, FromRoleKey: "small", ToRoleKey: "huge"}); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("ReplaceRole(unknown) = %v, want ErrNotFound", err)
	}
	if got := held(); !reflect.DeepEqual(got, []string{"large", "small"}) {
		t.Errorf("roles after a failed replacement = %v, want large and small", got)
	}
}
