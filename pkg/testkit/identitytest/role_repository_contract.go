// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"cmp"
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// RoleRepositoryContract verifies the roles of a database adapter's identity repository
// (data-model §4.1, FR-720) by its methods alone, the port contract of ADR-0153 §1: EnsureRole
// inserts a role once by key and keeps the permissions of the existing one; roles are found by
// key and by ID, listed highest position first with their permissions and badge, also inside a
// transaction; grants and revocations change the current roles, the holders, and the holders of
// a permission at once; the permissions and the position of a role are replaced; a role is
// deleted with its permissions and badge; the managed roles are marked and unmarked; badges are
// set, replaced, and removed with every user's choice to show them, and listed as choices and
// profile badges highest position first; and LockRole serializes the transactions that change
// one role, so that two revocations never take its last holder (ADR-0153 §5). The role use cases
// on top of it are RoleFlows, which internal/flowtest/identity runs once on SQLite.
func RoleRepositoryContract(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *identityRepo)
	}{
		{"EnsuresRolesOnceByKey", roleRepoEnsures},
		{"ListsAndFindsRoles", roleRepoLists},
		{"GrantsAndRevokes", roleRepoGrants},
		{"ChangesAndDeletesRoles", roleRepoChanges},
		{"MarksManagedRoles", roleRepoManaged},
		{"KeepsBadgesAndTheirDisplay", roleRepoBadges},
		{"LockRoleSerializesTheChangesOfARole", roleRepoLock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newIdentityRepo(t, newHarness)) })
	}
}

func roleRepoEnsures(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	first := r.role(t, "helper", 10, identity.PermManageReports, identity.PermViewAudit)
	again := r.role(t, "helper", 20, identity.PermManageRoles)
	if again != first {
		t.Errorf("EnsureRole of a taken key = %+v, want the existing %+v", again, first)
	}
	role, perms, err := r.h.Identity.FindRole(ctx, "helper")
	if err != nil || role != first || !slices.Equal(perms, []identity.Permission{identity.PermManageReports, identity.PermViewAudit}) {
		t.Errorf("FindRole = %+v, %v, %v; want the first role with its permissions, sorted", role, perms, err)
	}
	if _, _, err := r.h.Identity.FindRole(ctx, "nobody"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRole(unknown) = %v, want ErrNotFound", err)
	}
}

func roleRepoLists(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	low := r.role(t, "low", 10)
	high := r.role(t, "high", 90, identity.PermManageAccounts)
	mid := r.role(t, "mid", 50, identity.PermViewAudit, identity.PermManageReports)
	badge := identity.RoleBadge{Label: "Mid", Color: identity.BadgeTeal, UpdatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.SetRoleBadge(ctx, tx, mid.ID, badge) })
	want := []appidentity.RoleWithPermissions{
		{Role: high, Permissions: []identity.Permission{identity.PermManageAccounts}},
		{Role: mid, Permissions: []identity.Permission{identity.PermManageReports, identity.PermViewAudit}, Badge: &badge},
		{Role: low},
	}
	check := func(name string, got []appidentity.RoleWithPermissions, err error) {
		t.Helper()
		if err != nil || len(got) != len(want) {
			t.Fatalf("%s = %+v, %v; want %d roles", name, got, err, len(want))
		}
		for i := range want {
			if got[i].Role != want[i].Role || len(got[i].Permissions) != len(want[i].Permissions) ||
				len(want[i].Permissions) > 0 && !slices.Equal(got[i].Permissions, want[i].Permissions) ||
				(got[i].Badge == nil) != (want[i].Badge == nil) || got[i].Badge != nil && !sameBadge(*got[i].Badge, *want[i].Badge) {
				t.Errorf("%s[%d] = %+v, want %+v", name, i, got[i], want[i])
			}
		}
	}
	got, err := r.h.Identity.ListRoles(ctx)
	check("ListRoles", got, err)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.h.Identity.ListRolesTx(ctx, tx)
		check("ListRolesTx", got, err)
		if got, err := r.h.Identity.LockRole(ctx, tx, "mid"); err != nil || got != mid {
			t.Errorf("LockRole = %+v, %v; want %+v", got, err, mid)
		}
		if _, err := r.h.Identity.LockRole(ctx, tx, "nobody"); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("LockRole(unknown) = %v, want ErrNotFound", err)
		}
		if got, err := r.h.Identity.LockRoleByID(ctx, tx, high.ID); err != nil || got != high {
			t.Errorf("LockRoleByID = %+v, %v; want %+v", got, err, high)
		}
		if _, err := r.h.Identity.LockRoleByID(ctx, tx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("LockRoleByID(unknown) = %v, want ErrNotFound", err)
		}
		if perms, err := r.h.Identity.RolePermissions(ctx, tx, mid.ID); err != nil || !slices.Equal(perms, []identity.Permission{identity.PermManageReports, identity.PermViewAudit}) {
			t.Errorf("RolePermissions = %v, %v", perms, err)
		}
		return nil
	})
	ids := []string{high.ID, r.ids.NewID(), low.ID}
	wantIDs := []string{high.ID, low.ID}
	slices.Sort(wantIDs)
	if got, err := r.h.Identity.ExistingRoleIDs(ctx, ids); err != nil || !slices.Equal(got, wantIDs) {
		t.Errorf("ExistingRoleIDs = %v, %v; want %v", got, err, wantIDs)
	}
}

func roleRepoGrants(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	mod := r.role(t, "mod", 50, identity.PermManageReports)
	admin := r.role(t, "boss", 90, identity.PermManageReports, identity.PermManageRoles)
	alice, bob := r.user(t), r.user(t)
	ga := r.grant(t, alice, mod.ID)
	r.grant(t, alice, admin.ID)
	gb := r.grant(t, bob, mod.ID)

	if roles, err := r.h.Identity.ListUserRoles(ctx, alice); err != nil || len(roles) != 2 || roles[0] != admin || roles[1] != mod {
		t.Errorf("ListUserRoles = %+v, %v; want boss then mod", roles, err)
	}
	carol := r.user(t)
	if of, err := r.h.Identity.ListUserRolesOf(ctx, []string{alice, bob, carol}); err != nil || len(of) != 2 ||
		!slices.Equal(of[alice], []identity.Role{admin, mod}) || !slices.Equal(of[bob], []identity.Role{mod}) || len(of[carol]) != 0 {
		t.Errorf("ListUserRolesOf = %+v, %v; want alice's boss then mod and bob's mod, as ListUserRoles", of, err)
	}
	holders := []string{alice, bob}
	slices.Sort(holders)
	if got, err := r.h.Identity.ListRoleHolderIDs(ctx, mod.ID); err != nil || !slices.Equal(got, holders) {
		t.Errorf("ListRoleHolderIDs = %v, %v; want %v", got, err, holders)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if id, err := r.h.Identity.FindRoleGrantID(ctx, tx, alice, mod.ID); err != nil || id != ga.ID {
			t.Errorf("FindRoleGrantID = %q, %v; want %q", id, err, ga.ID)
		}
		if _, err := r.h.Identity.FindRoleGrantID(ctx, tx, bob, admin.ID); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("FindRoleGrantID(not held) = %v, want ErrNotFound", err)
		}
		if n, err := r.h.Identity.CountRoleHolders(ctx, tx, mod.ID); err != nil || n != 2 {
			t.Errorf("CountRoleHolders = %d, %v; want 2", n, err)
		}
		if got, err := r.h.Identity.ListUserIDsWithPermission(ctx, tx, identity.PermManageReports); err != nil || !slices.Equal(got, holders) {
			t.Errorf("ListUserIDsWithPermission(manage_reports) = %v, %v; want %v once each", got, err, holders)
		}
		if got, err := r.h.Identity.ListUserIDsWithPermission(ctx, tx, identity.PermManageRoles); err != nil || !slices.Equal(got, []string{alice}) {
			t.Errorf("ListUserIDsWithPermission(manage_roles) = %v, %v; want alice", got, err)
		}
		return nil
	})

	// A revocation removes the grant from the current roles at once.
	r.revoke(t, gb.ID)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Identity.ListRoleHolderIDsTx(ctx, tx, mod.ID); err != nil || !slices.Equal(got, []string{alice}) {
			t.Errorf("ListRoleHolderIDsTx after the revocation = %v, %v; want alice", got, err)
		}
		if _, err := r.h.Identity.FindRoleGrantID(ctx, tx, bob, mod.ID); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("FindRoleGrantID after the revocation = %v, want ErrNotFound", err)
		}
		return nil
	})
	if roles, err := r.h.Identity.ListUserRoles(ctx, bob); err != nil || len(roles) != 0 {
		t.Errorf("ListUserRoles(bob) = %+v, %v; want none", roles, err)
	}
	// The role can be granted again.
	if g := r.grant(t, bob, mod.ID); g.ID == gb.ID {
		t.Error("a new grant reused the revoked one")
	}
}

func roleRepoChanges(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	role := r.role(t, "editor", 30, identity.PermManageRules)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.SetRolePermissions(ctx, tx, role.ID, []identity.Permission{identity.PermViewAudit, identity.PermManageReports}); err != nil {
			return err
		}
		if err := r.h.Identity.UpdateRolePosition(ctx, tx, role.ID, 40); err != nil {
			return err
		}
		perms, err := r.h.Identity.RolePermissions(ctx, tx, role.ID)
		// In the order of identity.AllPermissions.
		if err != nil || !slices.Equal(perms, []identity.Permission{identity.PermManageReports, identity.PermViewAudit}) {
			t.Errorf("RolePermissions after SetRolePermissions = %v, %v", perms, err)
		}
		return nil
	})
	got, perms, err := r.h.Identity.FindRole(ctx, "editor")
	if err != nil || got.Position != 40 || len(perms) != 2 {
		t.Errorf("FindRole after the changes = %+v, %v, %v; want position 40 and two permissions", got, perms, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.SetRoleBadge(ctx, tx, role.ID, identity.RoleBadge{Label: "Ed", Color: identity.BadgeRed, UpdatedAt: r.clock.Now()}); err != nil {
			return err
		}
		return r.h.Identity.DeleteRole(ctx, tx, role.ID)
	})
	if _, _, err := r.h.Identity.FindRole(ctx, "editor"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRole after DeleteRole = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if perms, err := r.h.Identity.RolePermissions(ctx, tx, role.ID); err != nil || len(perms) != 0 {
			t.Errorf("RolePermissions after DeleteRole = %v, %v; want none", perms, err)
		}
		if _, ok, err := r.h.Identity.RoleBadge(ctx, tx, role.ID); err != nil || ok {
			t.Errorf("RoleBadge after DeleteRole = %t, %v; want none", ok, err)
		}
		return nil
	})
	// The key is free again.
	if again := r.role(t, "editor", 30); again.ID == role.ID {
		t.Error("EnsureRole after DeleteRole returned the deleted role")
	}
}

func roleRepoManaged(t *testing.T, r *identityRepo) {
	a, b := r.role(t, "plan-a", 5), r.role(t, "plan-b", 6)
	ma := identity.ManagedRole{RoleID: a.ID, Manager: identity.ManagerSupporters, CreatedAt: r.clock.Now()}
	mb := identity.ManagedRole{RoleID: b.ID, Manager: identity.ManagerSupporters, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertManagedRole(ctx, tx, mb); err != nil {
			return err
		}
		return r.h.Identity.InsertManagedRole(ctx, tx, ma)
	})
	want := []identity.ManagedRole{ma, mb}
	slices.SortFunc(want, func(x, y identity.ManagedRole) int { return cmp.Compare(x.RoleID, y.RoleID) })
	if got, err := r.h.Identity.ListManagedRoles(t.Context(), nil); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ListManagedRoles = %+v, %v; want %+v", got, err, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.DeleteManagedRole(ctx, tx, a.ID); err != nil {
			return err
		}
		if got, err := r.h.Identity.ListManagedRoles(ctx, tx); err != nil || !reflect.DeepEqual(got, []identity.ManagedRole{mb}) {
			t.Errorf("ListManagedRoles in the transaction = %+v, %v; want plan-b", got, err)
		}
		return nil
	})
}

func roleRepoBadges(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	high, low, plain := r.role(t, "high", 80), r.role(t, "low", 20), r.role(t, "plain", 50)
	alice, bob := r.user(t), r.user(t)
	for _, role := range []string{high.ID, low.ID, plain.ID} {
		r.grant(t, alice, role)
	}
	r.grant(t, bob, low.ID)
	hb := identity.RoleBadge{Label: "High", Color: identity.BadgeBlue, UpdatedAt: r.clock.Now()}
	lb := identity.RoleBadge{Label: "Low", Color: identity.BadgeGreen, UpdatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.SetRoleBadge(ctx, tx, high.ID, identity.RoleBadge{Label: "Old", Color: identity.BadgeRed, UpdatedAt: r.clock.Now()}); err != nil {
			return err
		}
		if err := r.h.Identity.SetRoleBadge(ctx, tx, high.ID, hb); err != nil { // replaces the badge
			return err
		}
		return r.h.Identity.SetRoleBadge(ctx, tx, low.ID, lb)
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, ok, err := r.h.Identity.RoleBadge(ctx, tx, high.ID); err != nil || !ok || !sameBadge(got, hb) {
			t.Errorf("RoleBadge = %+v, %t, %v; want %+v", got, ok, err, hb)
		}
		if _, ok, err := r.h.Identity.RoleBadge(ctx, tx, plain.ID); err != nil || ok {
			t.Errorf("RoleBadge(no badge) = %t, %v; want none", ok, err)
		}
		for range 2 { // showing again changes nothing
			if err := r.h.Identity.SetRoleBadgeDisplayed(ctx, tx, alice, low.ID, true, r.clock.Now()); err != nil {
				return err
			}
		}
		if err := r.h.Identity.SetRoleBadgeDisplayed(ctx, tx, alice, high.ID, false, r.clock.Now()); err != nil {
			return err
		}
		if err := r.h.Identity.SetRoleBadgeDisplayed(ctx, tx, bob, low.ID, true, r.clock.Now()); err != nil {
			return err
		}
		if shown, err := r.h.Identity.RoleBadgeDisplayed(ctx, tx, alice, low.ID); err != nil || !shown {
			t.Errorf("RoleBadgeDisplayed(low) = %t, %v; want true", shown, err)
		}
		if shown, err := r.h.Identity.RoleBadgeDisplayed(ctx, tx, alice, high.ID); err != nil || shown {
			t.Errorf("RoleBadgeDisplayed(high) = %t, %v; want false", shown, err)
		}
		return nil
	})
	choices, err := r.h.Identity.ListBadgeChoices(ctx, alice)
	wantChoices := []identity.BadgeChoice{
		{ProfileBadge: identity.ProfileBadge{RoleID: high.ID, Label: "High", Color: identity.BadgeBlue}},
		{ProfileBadge: identity.ProfileBadge{RoleID: low.ID, Label: "Low", Color: identity.BadgeGreen}, Displayed: true},
	}
	if err != nil || !reflect.DeepEqual(choices, wantChoices) {
		t.Errorf("ListBadgeChoices = %+v, %v; want %+v", choices, err, wantChoices)
	}
	if got, err := r.h.Identity.ListProfileBadges(ctx, alice); err != nil || !reflect.DeepEqual(got, []identity.ProfileBadge{wantChoices[1].ProfileBadge}) {
		t.Errorf("ListProfileBadges = %+v, %v; want the low badge", got, err)
	}
	// Removing the badge removes every user's choice to show it.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.DeleteRoleBadge(ctx, tx, low.ID) })
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if shown, err := r.h.Identity.RoleBadgeDisplayed(ctx, tx, bob, low.ID); err != nil || shown {
			t.Errorf("RoleBadgeDisplayed after DeleteRoleBadge = %t, %v; want false", shown, err)
		}
		return nil
	})
	if got, err := r.h.Identity.ListProfileBadges(ctx, bob); err != nil || len(got) != 0 {
		t.Errorf("ListProfileBadges(bob) after DeleteRoleBadge = %+v, %v; want none", got, err)
	}
}

// roleRepoLock revokes the role from each of its two holders in concurrent transactions that
// each lock the role, count its holders, and revoke only while another holder stays: the role
// keeps one holder whatever order the transactions run in.
func roleRepoLock(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	for round := range 3 {
		role := r.role(t, "keeper"+string(rune('a'+round)), 100)
		grants := []identity.RoleGrant{r.grant(t, r.user(t), role.ID), r.grant(t, r.user(t), role.ID)}
		var wg sync.WaitGroup
		for _, g := range grants {
			wg.Go(func() {
				err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
					if _, err := r.h.Identity.LockRole(ctx, tx, role.Key); err != nil {
						return err
					}
					n, err := r.h.Identity.CountRoleHolders(ctx, tx, role.ID)
					if err != nil || n < 2 {
						return err
					}
					return r.h.Identity.InsertRoleRevocation(ctx, tx, identity.RoleRevocation{GrantID: g.ID, RevokedAt: r.clock.Now()})
				})
				if err != nil {
					t.Errorf("round %d: transaction = %v", round, err)
				}
			})
		}
		wg.Wait()
		if got, err := r.h.Identity.ListRoleHolderIDs(ctx, role.ID); err != nil || len(got) != 1 {
			t.Errorf("round %d: holders = %v, %v; want one", round, got, err)
		}
	}
}

// sameBadge reports whether two badges are equal, their times compared as instants.
func sameBadge(a, b identity.RoleBadge) bool {
	return a.Label == b.Label && a.Color == b.Color && a.UpdatedAt.Equal(b.UpdatedAt)
}
