// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// TestRoleChangesApplyAndAreAudited checks, through the composition root on SQLite, that a role
// grant and revocation change the user's permissions for the next check and record their audit
// log entries (FR-720, ADR-0037).
func TestRoleChangesApplyAndAreAudited(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "inline", "MY_MAIL": "none"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	root, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	may := func(want bool) {
		t.Helper()
		if ok, err := app.Identity.HasPermission(ctx, bob.User.ID, domainidentity.PermManageReports); err != nil || ok != want {
			t.Errorf("bob may manage reports = %v, %v; want %v", ok, err, want)
		}
	}
	may(false)
	in := identity.RoleChangeInput{UserID: bob.User.ID, RoleKey: domainidentity.RoleModerator, ByUserID: root.User.ID}
	if changed, err := app.Identity.GrantRole(ctx, in); err != nil || !changed {
		t.Fatalf("GrantRole = %v, %v", changed, err)
	}
	may(true)
	in.ByUserID = ""
	if changed, err := app.Identity.RevokeRole(ctx, in); err != nil || !changed {
		t.Fatalf("RevokeRole = %v, %v", changed, err)
	}
	may(false)

	got, err := app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetRole}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("audit log of roles = %+v, want 2 entries", got)
	}
	revoked, granted := got[0], got[1]
	if granted.Action != moderation.AuditRoleGranted || granted.ActorKind != moderation.AuditActorUser || granted.ActorUserID != root.User.ID ||
		granted.TargetLabel != "moderator @bob" || granted.SourceEventID == "" {
		t.Errorf("grant entry = %+v", granted)
	}
	if revoked.Action != moderation.AuditRoleRevoked || revoked.ActorKind != moderation.AuditActorCLI || revoked.TargetID != granted.TargetID ||
		revoked.SourceEventID != granted.SourceEventID {
		t.Errorf("revocation entry = %+v, want the CLI revoking the grant %s", revoked, granted.SourceEventID)
	}
}

// TestRoleGrantNeedsTheRolePermissions checks that a user who manages roles grants only the
// roles whose every permission they hold, besides the rank (FR-720).
func TestRoleGrantNeedsTheRolePermissions(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "inline", "MY_MAIL": "none"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	pw := "correct horse battery staple"
	if _, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: pw}); err != nil {
		t.Fatal(err)
	}
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: pw})
	if err != nil {
		t.Fatal(err)
	}
	carol, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "carol", Password: pw})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []identity.CreateRoleInput{
		{Key: "role-manager", Position: 50, Permissions: []domainidentity.Permission{domainidentity.PermManageRoles, domainidentity.PermViewAudit}},
		{Key: "settings", Position: 10, Permissions: []domainidentity.Permission{domainidentity.PermManageSettings}},
		{Key: "auditor", Position: 10, Permissions: []domainidentity.Permission{domainidentity.PermViewAudit}},
	} {
		if _, err := app.Identity.CreateRole(ctx, r); err != nil {
			t.Fatalf("CreateRole %s: %v", r.Key, err)
		}
	}
	if _, err := app.Identity.GrantRole(ctx, identity.RoleChangeInput{UserID: bob.User.ID, RoleKey: "role-manager"}); err != nil {
		t.Fatal(err)
	}

	in := identity.RoleChangeInput{UserID: carol.User.ID, RoleKey: "settings", ByUserID: bob.User.ID}
	var missing *identity.MissingPermissionsError
	if changed, err := app.Identity.GrantRole(ctx, in); changed || !errors.As(err, &missing) ||
		!slices.Equal(missing.Missing, []domainidentity.Permission{domainidentity.PermManageSettings}) {
		t.Errorf("GrantRole(settings) by bob = %v, %v; want a *MissingPermissionsError for manage_settings", changed, err)
	}
	if ok, err := app.Identity.HasPermission(ctx, carol.User.ID, domainidentity.PermManageSettings); err != nil || ok {
		t.Errorf("carol may manage settings = %v, %v; want false", ok, err)
	}
	for _, key := range []string{"auditor", "role-manager"} {
		in.RoleKey = key
		if changed, err := app.Identity.GrantRole(ctx, in); err != nil || !changed {
			t.Errorf("GrantRole(%s) by bob = %v, %v; want granted", key, changed, err)
		}
	}
}
