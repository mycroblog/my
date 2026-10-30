// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// RoleRepositoryContract verifies the role persistence of a database adapter's identity
// repository (data-model §4.1, FR-720) through the role use cases: the roles list with their
// permissions; a grant and a revocation change the user's permissions and the holders of a
// permission at once, record their audit log entry in their transaction, and do nothing when
// they would change nothing; a user changes only the roles at or below their own; the admin role
// keeps its last holder, also when two revocations race; unknown users and roles are reported;
// roles are created, changed, and deleted (except the built-in ones, and those that users hold)
// with their audit log entries, within the rank and the permissions of the actor; role badges
// are set and removed, shown only by the users who chose to, and their choice goes with the
// role, moving to the new role of a replacement (ADR-0071). It uses the same harness as
// AccountRepositoryContract.
func RoleRepositoryContract(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *roleEnv)
	}{
		{"ListsTheRoles", roleLists},
		{"GrantsAndRevokes", roleGrantsAndRevokes},
		{"RefusesRolesAboveTheActor", roleRefusesOutranking},
		{"KeepsTheLastAdmin", roleKeepsLastAdmin},
		{"KeepsTheLastAdminUnderConcurrency", roleLastAdminConcurrency},
		{"ReportsUnknownUsersAndRoles", roleReportsUnknown},
		{"CreatesUpdatesAndDeletesRoles", roleCreatesUpdatesAndDeletes},
		{"KeepsTheBuiltinRoles", roleKeepsBuiltins},
		{"RefusesInvalidRoles", roleRefusesInvalid},
		{"RefusesRoleChangesBeyondTheActor", roleRefusesChangesBeyondActor},
		{"ShowsBadgesByChoice", roleBadgesAreShownByChoice},
		{"ChangesBadges", roleBadgeChanges},
		{"ReplacesRolesAndMovesTheBadge", roleReplacesAndMovesTheBadge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &auditRecorder{}
			e := &roleEnv{audit: rec}
			e.accountEnv = newAccountEnv(t, newHarness, func(p *uow.Projectors) {
				p.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
					rec.add(ev.(appmoderation.AuditRecorded).Entry)
					return nil
				})
			})
			admin, err := e.identity.CreateAdmin(t.Context(), appidentity.CreateAdminInput{
				Username: "root", Email: "root@example.com", Password: "correct horse battery staple",
			})
			if err != nil {
				t.Fatal(err)
			}
			e.admin = admin.User.ID
			tc.run(t, e)
		})
	}
}

type auditRecorder struct {
	mu      sync.Mutex
	entries []moderation.AuditEntry
}

func (r *auditRecorder) add(e moderation.AuditEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

func (r *auditRecorder) all() []moderation.AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.entries)
}

type roleEnv struct {
	*accountEnv
	audit *auditRecorder
	// admin is the user of @root, the only admin at first.
	admin string
}

func (e *roleEnv) grant(t *testing.T, userID, key, by string) bool {
	t.Helper()
	changed, err := e.identity.GrantRole(t.Context(), appidentity.RoleChangeInput{UserID: userID, RoleKey: key, ByUserID: by})
	if err != nil {
		t.Fatalf("GrantRole(%s, %s, by %q) = %v", userID, key, by, err)
	}
	return changed
}

func (e *roleEnv) revoke(t *testing.T, userID, key, by string) bool {
	t.Helper()
	changed, err := e.identity.RevokeRole(t.Context(), appidentity.RoleChangeInput{UserID: userID, RoleKey: key, ByUserID: by})
	if err != nil {
		t.Fatalf("RevokeRole(%s, %s, by %q) = %v", userID, key, by, err)
	}
	return changed
}

func (e *roleEnv) permissions(t *testing.T, userID string) []identity.Permission {
	t.Helper()
	perms, err := e.identity.Permissions(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return perms
}

func (e *roleEnv) holders(t *testing.T, p identity.Permission) []string {
	t.Helper()
	var out []string
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		hs, err := e.identity.PermissionHolders(t.Context(), tx, p)
		out = out[:0]
		for _, h := range hs {
			out = append(out, h.UserID)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func roleLists(t *testing.T, e *roleEnv) {
	roles, err := e.identity.Roles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 {
		t.Fatalf("Roles = %+v, want admin and moderator", roles)
	}
	for i, b := range identity.BuiltinRoles() {
		if r := roles[i]; r.ID == "" || r.Key != b.Key || r.Position != b.Position || !reflect.DeepEqual(r.Permissions, b.Permissions) {
			t.Errorf("role %d = %+v, want %+v", i, r, b)
		}
	}
	holders, err := e.identity.RoleHolders(t.Context(), identity.RoleAdmin)
	if err != nil || !reflect.DeepEqual(holders, []string{e.admin}) {
		t.Errorf("RoleHolders(admin) = %q, %v; want root", holders, err)
	}
	if holders, err := e.identity.RoleHolders(t.Context(), identity.RoleModerator); err != nil || len(holders) != 0 {
		t.Errorf("RoleHolders(moderator) = %q, %v; want none", holders, err)
	}
}

func roleGrantsAndRevokes(t *testing.T, e *roleEnv) {
	bob := e.register(t, "bob").User.ID
	if got := e.permissions(t, bob); len(got) != 0 {
		t.Fatalf("bob's permissions = %v, want none", got)
	}
	if !e.grant(t, bob, identity.RoleModerator, e.admin) {
		t.Fatal("granting moderator changed nothing")
	}
	grantedAt := e.clock.Now()
	moderatorPerms := []identity.Permission{identity.PermManageReports, identity.PermManageAccounts, identity.PermViewAudit}
	if got := e.permissions(t, bob); !reflect.DeepEqual(got, moderatorPerms) {
		t.Errorf("bob's permissions = %v, want %v", got, moderatorPerms)
	}
	if got, want := e.holders(t, identity.PermManageReports), sorted(e.admin, bob); !reflect.DeepEqual(got, want) {
		t.Errorf("holders of manage_reports = %q, want %q", got, want)
	}
	roles, err := e.identity.UserRoles(t.Context(), bob)
	if err != nil || len(roles) != 1 || roles[0].Key != identity.RoleModerator {
		t.Fatalf("UserRoles(bob) = %+v, %v; want moderator", roles, err)
	}
	moderatorID := roles[0].ID
	e.clock.Advance(time.Second)
	if e.grant(t, bob, identity.RoleModerator, "") {
		t.Error("granting a held role changed something")
	}

	e.clock.Advance(time.Second)
	if !e.revoke(t, bob, identity.RoleModerator, "") {
		t.Fatal("revoking moderator changed nothing")
	}
	revokedAt := e.clock.Now()
	if got := e.permissions(t, bob); len(got) != 0 {
		t.Errorf("bob's permissions after the revocation = %v, want none", got)
	}
	if got := e.holders(t, identity.PermManageReports); !reflect.DeepEqual(got, []string{e.admin}) {
		t.Errorf("holders of manage_reports after the revocation = %q, want root", got)
	}
	if e.revoke(t, bob, identity.RoleModerator, e.admin) {
		t.Error("revoking a role not held changed something")
	}
	// A new grant after a revocation is a grant of its own.
	e.clock.Advance(time.Second)
	if !e.grant(t, bob, identity.RoleModerator, "") {
		t.Fatal("granting moderator again changed nothing")
	}

	got := e.audit.all()
	if len(got) != 3 {
		t.Fatalf("audit entries = %+v, want 3", got)
	}
	want := []moderation.AuditEntry{
		{ActorKind: moderation.AuditActorUser, ActorUserID: e.admin, Action: moderation.AuditRoleGranted, OccurredAt: grantedAt},
		{ActorKind: moderation.AuditActorCLI, Action: moderation.AuditRoleRevoked, OccurredAt: revokedAt},
		{ActorKind: moderation.AuditActorCLI, Action: moderation.AuditRoleGranted, OccurredAt: e.clock.Now()},
	}
	for i, w := range want {
		g := got[i]
		if g.ID == "" || g.ActorKind != w.ActorKind || g.ActorUserID != w.ActorUserID || g.Action != w.Action ||
			g.TargetKind != moderation.AuditTargetRole || g.TargetID != moderatorID || g.TargetLabel != "moderator @bob" ||
			g.SourceEventID == "" || !g.OccurredAt.Equal(w.OccurredAt) {
			t.Errorf("audit entry %d = %+v, want %+v on the moderator role of @bob", i, g, w)
		}
		if err := g.Validate(); err != nil {
			t.Errorf("audit entry %d: %v", i, err)
		}
	}
	if got[0].SourceEventID != got[1].SourceEventID || got[2].SourceEventID == got[0].SourceEventID {
		t.Errorf("sources = %s, %s, %s; want the revocation to name the first grant and the new grant its own",
			got[0].SourceEventID, got[1].SourceEventID, got[2].SourceEventID)
	}
}

func sorted(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

func roleRefusesOutranking(t *testing.T, e *roleEnv) {
	bob := e.register(t, "bob").User.ID
	carol := e.register(t, "carol").User.ID
	dave := e.register(t, "dave").User.ID
	e.grant(t, bob, identity.RoleModerator, "")
	for _, tc := range []struct {
		name      string
		change    func(context.Context, appidentity.RoleChangeInput) (bool, error)
		user, key string
		by        string
	}{
		{"a moderator grants admin", e.identity.GrantRole, carol, identity.RoleAdmin, bob},
		{"a moderator revokes admin", e.identity.RevokeRole, e.admin, identity.RoleAdmin, bob},
		{"a user without roles grants moderator", e.identity.GrantRole, carol, identity.RoleModerator, dave},
	} {
		if _, err := tc.change(t.Context(), appidentity.RoleChangeInput{UserID: tc.user, RoleKey: tc.key, ByUserID: tc.by}); !errors.Is(err, appidentity.ErrRoleOutranksActor) {
			t.Errorf("%s = %v, want ErrRoleOutranksActor", tc.name, err)
		}
	}
	// A moderator manages the roles at its own position.
	if !e.grant(t, carol, identity.RoleModerator, bob) || !e.revoke(t, carol, identity.RoleModerator, bob) {
		t.Error("a moderator could not grant and revoke moderator")
	}
	if got := e.permissions(t, carol); len(got) != 0 {
		t.Errorf("carol's permissions = %v, want none", got)
	}
	if n := len(e.audit.all()); n != 3 { // bob's grant, then carol's grant and revocation
		t.Errorf("audit entries = %d, want 3; refused changes record nothing", n)
	}
}

func roleKeepsLastAdmin(t *testing.T, e *roleEnv) {
	bob := e.register(t, "bob").User.ID
	if _, err := e.identity.RevokeRole(t.Context(), appidentity.RoleChangeInput{UserID: e.admin, RoleKey: identity.RoleAdmin, ByUserID: e.admin}); !errors.Is(err, appidentity.ErrLastAdmin) {
		t.Fatalf("revoking the only admin = %v, want ErrLastAdmin", err)
	}
	if got := e.permissions(t, e.admin); !reflect.DeepEqual(got, identity.AllPermissions()) {
		t.Errorf("root's permissions = %v, want all", got)
	}
	e.grant(t, bob, identity.RoleAdmin, e.admin)
	if !e.revoke(t, e.admin, identity.RoleAdmin, e.admin) {
		t.Fatal("revoking one of two admins changed nothing")
	}
	if _, err := e.identity.RevokeRole(t.Context(), appidentity.RoleChangeInput{UserID: bob, RoleKey: identity.RoleAdmin}); !errors.Is(err, appidentity.ErrLastAdmin) {
		t.Errorf("revoking the new only admin from the CLI = %v, want ErrLastAdmin", err)
	}
	if n := len(e.audit.all()); n != 2 {
		t.Errorf("audit entries = %d, want 2", n)
	}
}

func roleLastAdminConcurrency(t *testing.T, e *roleEnv) {
	bob := e.register(t, "bob").User.ID
	e.grant(t, bob, identity.RoleAdmin, "")
	// Two operators revoke the admin role of each admin at the same time: one admin must stay.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, user := range []string{e.admin, bob} {
		wg.Go(func() {
			_, errs[i] = e.identity.RevokeRole(t.Context(), appidentity.RoleChangeInput{UserID: user, RoleKey: identity.RoleAdmin})
		})
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, appidentity.ErrLastAdmin):
			t.Errorf("RevokeRole = %v, want nil or ErrLastAdmin", err)
		}
	}
	holders, err := e.identity.RoleHolders(t.Context(), identity.RoleAdmin)
	if ok != 1 || err != nil || len(holders) != 1 {
		t.Errorf("%d revocations succeeded, admins %q (%v); want 1 and one admin left", ok, holders, err)
	}
}

func roleReportsUnknown(t *testing.T, e *roleEnv) {
	bob := e.register(t, "bob").User.ID
	for _, in := range []appidentity.RoleChangeInput{
		{UserID: bob, RoleKey: "owner"},
		{UserID: e.ids.NewID(), RoleKey: identity.RoleModerator},
	} {
		if _, err := e.identity.GrantRole(t.Context(), in); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("GrantRole(%+v) = %v, want ErrNotFound", in, err)
		}
		if _, err := e.identity.RevokeRole(t.Context(), in); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("RevokeRole(%+v) = %v, want ErrNotFound", in, err)
		}
	}
	if _, err := e.identity.RoleHolders(t.Context(), "owner"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("RoleHolders(owner) = %v, want ErrNotFound", err)
	}
}

func roleCreatesUpdatesAndDeletes(t *testing.T, e *roleEnv) {
	ctx := t.Context()
	bob := e.register(t, "bob").User.ID
	created, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{
		Key: "helper", Position: 50, Permissions: []identity.Permission{identity.PermViewAudit, identity.PermManageReports}, ByUserID: e.admin,
	})
	if err != nil {
		t.Fatalf("CreateRole = %v", err)
	}
	if created.ID == "" || created.Key != "helper" || created.Position != 50 ||
		!reflect.DeepEqual(created.Permissions, []identity.Permission{identity.PermManageReports, identity.PermViewAudit}) {
		t.Errorf("CreateRole = %+v, want helper at 50 with manage_reports and view_audit", created)
	}
	roles, err := e.identity.Roles(ctx)
	if err != nil || len(roles) != 3 || roles[2].ID != created.ID || !reflect.DeepEqual(roles[2].Permissions, created.Permissions) {
		t.Fatalf("Roles = %+v, %v; want admin, moderator, and helper", roles, err)
	}
	e.grant(t, bob, "helper", e.admin)
	if got := e.permissions(t, bob); !reflect.DeepEqual(got, created.Permissions) {
		t.Errorf("bob's permissions = %v, want %v", got, created.Permissions)
	}

	e.clock.Advance(time.Second)
	pos := 150
	updated, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{
		Key: "helper", Position: &pos, Permissions: []identity.Permission{identity.PermManageJobs}, SetPermissions: true, ByUserID: e.admin,
	})
	if err != nil {
		t.Fatalf("UpdateRole = %v", err)
	}
	if updated.ID != created.ID || updated.Position != 150 || !reflect.DeepEqual(updated.Permissions, []identity.Permission{identity.PermManageJobs}) {
		t.Errorf("UpdateRole = %+v, want helper at 150 with manage_jobs", updated)
	}
	if got := e.permissions(t, bob); !reflect.DeepEqual(got, []identity.Permission{identity.PermManageJobs}) {
		t.Errorf("bob's permissions after the change = %v, want manage_jobs", got)
	}
	// Only the position, and then nothing.
	pos = 120
	if r, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: "helper", Position: &pos}); err != nil || r.Position != 120 ||
		!reflect.DeepEqual(r.Permissions, []identity.Permission{identity.PermManageJobs}) {
		t.Errorf("UpdateRole(position) = %+v, %v; want 120 with manage_jobs", r, err)
	}
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{
		Key: "helper", Position: &pos, Permissions: []identity.Permission{identity.PermManageJobs}, SetPermissions: true,
	}); err != nil {
		t.Errorf("UpdateRole(no change) = %v", err)
	}

	if err := e.identity.DeleteRole(ctx, "helper", e.admin); !errors.Is(err, appidentity.ErrRoleHeld) {
		t.Errorf("DeleteRole(held) = %v, want ErrRoleHeld", err)
	}
	e.revoke(t, bob, "helper", e.admin)
	missing := e.ids.NewID()
	if got, err := e.identity.ExistingRoleIDs(ctx, []string{missing, created.ID}); err != nil || !reflect.DeepEqual(got, []string{created.ID}) {
		t.Errorf("ExistingRoleIDs = %v, %v; want helper", got, err)
	}
	e.clock.Advance(time.Second)
	if err := e.identity.DeleteRole(ctx, "helper", e.admin); err != nil {
		t.Fatalf("DeleteRole = %v", err)
	}
	if got, err := e.identity.ExistingRoleIDs(ctx, []string{created.ID}); err != nil || len(got) != 0 {
		t.Errorf("ExistingRoleIDs after the deletion = %v, %v; want none", got, err)
	}
	if _, _, err := e.h.Identity.FindRole(ctx, "helper"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRole(helper) after the deletion = %v, want ErrNotFound", err)
	}
	if err := e.identity.DeleteRole(ctx, "helper", e.admin); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("DeleteRole(deleted) = %v, want ErrNotFound", err)
	}
	// The key is free again.
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: "helper", Position: 1}); err != nil {
		t.Errorf("CreateRole(helper again) = %v", err)
	}

	var got []moderation.AuditEntry
	for _, a := range e.audit.all() {
		if a.Action != moderation.AuditRoleGranted && a.Action != moderation.AuditRoleRevoked {
			got = append(got, a)
		}
	}
	want := []struct {
		action moderation.AuditAction
		actor  string
	}{
		{moderation.AuditRoleCreated, e.admin},
		{moderation.AuditRoleUpdated, e.admin},
		{moderation.AuditRoleUpdated, ""},
		{moderation.AuditRoleDeleted, e.admin},
		{moderation.AuditRoleCreated, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("audit entries = %+v, want %d", got, len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.Action != w.action || g.ActorUserID != w.actor || g.TargetKind != moderation.AuditTargetRole ||
			g.TargetLabel != "helper" || g.SourceEventID != "" {
			t.Errorf("audit entry %d = %+v, want %s by %q on helper", i, g, w.action, w.actor)
		}
		if i < 4 && g.TargetID != created.ID {
			t.Errorf("audit entry %d targets %s, want %s", i, g.TargetID, created.ID)
		}
		if err := g.Validate(); err != nil {
			t.Errorf("audit entry %d: %v", i, err)
		}
	}
}

func roleKeepsBuiltins(t *testing.T, e *roleEnv) {
	ctx := t.Context()
	for _, key := range []string{identity.RoleAdmin, identity.RoleModerator} {
		if err := e.identity.DeleteRole(ctx, key, e.admin); !errors.Is(err, appidentity.ErrBuiltinRole) {
			t.Errorf("DeleteRole(%s) = %v, want ErrBuiltinRole", key, err)
		}
		if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: key, Position: 1}); !errors.Is(err, appidentity.ErrRoleKeyTaken) {
			t.Errorf("CreateRole(%s) = %v, want ErrRoleKeyTaken", key, err)
		}
	}
	pos := 10
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: identity.RoleAdmin, Position: &pos}); !errors.Is(err, appidentity.ErrBuiltinRole) {
		t.Errorf("UpdateRole(admin) = %v, want ErrBuiltinRole", err)
	}
	// The moderator role takes a new position and permissions, and keeps them when the built-in
	// roles are ensured again.
	pos = 200
	perms := []identity.Permission{identity.PermManageReports, identity.PermManageInvites}
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{
		Key: identity.RoleModerator, Position: &pos, Permissions: perms, SetPermissions: true, ByUserID: e.admin,
	}); err != nil {
		t.Fatalf("UpdateRole(moderator) = %v", err)
	}
	if err := e.identity.EnsureBuiltinRoles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{
		Username: "root2", Email: "root2@example.com", Password: "correct horse battery staple",
	}); err != nil {
		t.Fatal(err)
	}
	r, got, err := e.h.Identity.FindRole(ctx, identity.RoleModerator)
	if err != nil || r.Position != 200 || !reflect.DeepEqual(got, []identity.Permission{identity.PermManageInvites, identity.PermManageReports}) {
		t.Errorf("moderator = %+v %v, %v; want position 200 with manage_invites and manage_reports", r, got, err)
	}
	r, got, err = e.h.Identity.FindRole(ctx, identity.RoleAdmin)
	if err != nil || r.Position != identity.AdminPosition || len(got) != len(identity.AllPermissions()) {
		t.Errorf("admin = %+v %v, %v; want every permission at %d", r, got, err, identity.AdminPosition)
	}
}

func roleRefusesInvalid(t *testing.T, e *roleEnv) {
	ctx := t.Context()
	for _, tc := range []struct {
		in   appidentity.CreateRoleInput
		want error
	}{
		{appidentity.CreateRoleInput{Key: "", Position: 1}, appidentity.ErrInvalidRoleKey},
		{appidentity.CreateRoleInput{Key: "Helper", Position: 1}, appidentity.ErrInvalidRoleKey},
		{appidentity.CreateRoleInput{Key: "help me", Position: 1}, appidentity.ErrInvalidRoleKey},
		{appidentity.CreateRoleInput{Key: "helper", Position: identity.AdminPosition}, appidentity.ErrInvalidRolePosition},
		{appidentity.CreateRoleInput{Key: "helper", Position: -1}, appidentity.ErrInvalidRolePosition},
		{appidentity.CreateRoleInput{Key: "helper", Permissions: []identity.Permission{"own_everything"}}, appidentity.ErrInvalidRolePermissions},
		{appidentity.CreateRoleInput{Key: "helper", Permissions: []identity.Permission{identity.PermViewAudit, identity.PermViewAudit}}, appidentity.ErrInvalidRolePermissions},
	} {
		if _, err := e.identity.CreateRole(ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("CreateRole(%+v) = %v, want %v", tc.in, err, tc.want)
		}
	}
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: "helper_2-b", Position: identity.MaxRolePosition}); err != nil {
		t.Fatalf("CreateRole(helper_2-b) = %v", err)
	}
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: "helper_2-b", Position: 1}); !errors.Is(err, appidentity.ErrRoleKeyTaken) {
		t.Errorf("CreateRole(taken) = %v, want ErrRoleKeyTaken", err)
	}
	pos := identity.AdminPosition
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: "helper_2-b", Position: &pos}); !errors.Is(err, appidentity.ErrInvalidRolePosition) {
		t.Errorf("UpdateRole(position 1000) = %v, want ErrInvalidRolePosition", err)
	}
	pos = 1
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: "owner", Position: &pos}); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("UpdateRole(unknown) = %v, want ErrNotFound", err)
	}
	if err := e.identity.DeleteRole(ctx, "owner", ""); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("DeleteRole(unknown) = %v, want ErrNotFound", err)
	}
	if n := len(e.audit.all()); n != 1 {
		t.Errorf("audit entries = %d, want 1; refused changes record nothing", n)
	}
}

func roleRefusesChangesBeyondActor(t *testing.T, e *roleEnv) {
	ctx := t.Context()
	bob := e.register(t, "bob").User.ID
	e.grant(t, bob, identity.RoleModerator, "") // position 100: manage_reports, manage_accounts, view_audit
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: "above", Position: 101}); err != nil {
		t.Fatal(err)
	}
	var missing *appidentity.MissingPermissionsError
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: "boss", Position: 101, ByUserID: bob}); !errors.Is(err, appidentity.ErrRoleOutranksActor) {
		t.Errorf("CreateRole above the actor = %v, want ErrRoleOutranksActor", err)
	}
	_, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{
		Key: "settings", Position: 10, Permissions: []identity.Permission{identity.PermManageReports, identity.PermManageSettings}, ByUserID: bob,
	})
	if !errors.As(err, &missing) || !reflect.DeepEqual(missing.Missing, []identity.Permission{identity.PermManageSettings}) {
		t.Errorf("CreateRole with a permission the actor lacks = %v, want manage_settings missing", err)
	}
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{
		Key: "triage", Position: 100, Permissions: []identity.Permission{identity.PermManageReports}, ByUserID: bob,
	}); err != nil {
		t.Fatalf("CreateRole within the actor = %v", err)
	}
	pos := 101
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: "triage", Position: &pos, ByUserID: bob}); !errors.Is(err, appidentity.ErrRoleOutranksActor) {
		t.Errorf("UpdateRole above the actor = %v, want ErrRoleOutranksActor", err)
	}
	pos = 1
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: "above", Position: &pos, ByUserID: bob}); !errors.Is(err, appidentity.ErrRoleOutranksActor) {
		t.Errorf("UpdateRole of a role above the actor = %v, want ErrRoleOutranksActor", err)
	}
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{
		Key: "triage", Permissions: []identity.Permission{identity.PermManageReports, identity.PermManageJobs}, SetPermissions: true, ByUserID: bob,
	}); !errors.As(err, &missing) || !reflect.DeepEqual(missing.Missing, []identity.Permission{identity.PermManageJobs}) {
		t.Errorf("UpdateRole adding a permission the actor lacks = %v, want manage_jobs missing", err)
	}
	if err := e.identity.DeleteRole(ctx, "above", bob); !errors.Is(err, appidentity.ErrRoleOutranksActor) {
		t.Errorf("DeleteRole above the actor = %v, want ErrRoleOutranksActor", err)
	}
	if err := e.identity.DeleteRole(ctx, "triage", bob); err != nil {
		t.Errorf("DeleteRole within the actor = %v", err)
	}
}
