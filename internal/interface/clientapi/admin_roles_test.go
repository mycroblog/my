// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeRoles keeps the roles of users with the semantics of appidentity.Service: the built-in
// roles, the rank of the actor, and the last admin. It is also the Permissions of the server, so a
// change applies to the next request.
type fakeRoles struct {
	held    map[string][]string // user ID -> role keys
	changes []string
	// anyHolderManagesRoles grants manage_roles to every holder of a role, so that the rank
	// check, not the permission, refuses a change.
	anyHolderManagesRoles bool
	// defErr is what CreateRole, UpdateRole, and DeleteRole fail with, or nil for success.
	defErr error
}

var fakeRoleIDs = map[string]string{
	identity.RoleAdmin:     "01890a5d-ac96-774b-bcce-b302099a5001",
	identity.RoleModerator: "01890a5d-ac96-774b-bcce-b302099a5002",
}

func (f *fakeRoles) Roles(context.Context) ([]appidentity.RoleWithPermissions, error) {
	var out []appidentity.RoleWithPermissions
	for _, b := range identity.BuiltinRoles() {
		out = append(out, appidentity.RoleWithPermissions{
			Role: identity.Role{ID: fakeRoleIDs[b.Key], Key: b.Key, Position: b.Position}, Permissions: b.Permissions,
		})
	}
	return out, nil
}

func (f *fakeRoles) role(key string) (appidentity.RoleWithPermissions, bool) {
	all, _ := f.Roles(context.Background())
	i := slices.IndexFunc(all, func(r appidentity.RoleWithPermissions) bool { return r.Key == key })
	if i < 0 {
		return appidentity.RoleWithPermissions{}, false
	}
	return all[i], true
}

func (f *fakeRoles) UserRoles(_ context.Context, userID string) ([]identity.Role, error) {
	var out []identity.Role
	all, _ := f.Roles(context.Background())
	for _, r := range all {
		if slices.Contains(f.held[userID], r.Key) {
			out = append(out, r.Role)
		}
	}
	return out, nil
}

func (f *fakeRoles) check(in appidentity.RoleChangeInput) error {
	r, ok := f.role(in.RoleKey)
	if !ok {
		return fmt.Errorf("role %s: %w", in.RoleKey, appidentity.ErrNotFound)
	}
	actor, _ := f.UserRoles(context.Background(), in.ByUserID)
	if in.ByUserID != "" && !identity.CanManageRole(actor, r.Role) {
		return appidentity.ErrRoleOutranksActor
	}
	return nil
}

func (f *fakeRoles) GrantRole(_ context.Context, in appidentity.RoleChangeInput) (bool, error) {
	if err := f.check(in); err != nil || slices.Contains(f.held[in.UserID], in.RoleKey) {
		return false, err
	}
	f.held[in.UserID] = append(f.held[in.UserID], in.RoleKey)
	f.changes = append(f.changes, "grant "+in.RoleKey+" to "+in.UserID+" by "+in.ByUserID)
	return true, nil
}

func (f *fakeRoles) RevokeRole(_ context.Context, in appidentity.RoleChangeInput) (bool, error) {
	if err := f.check(in); err != nil || !slices.Contains(f.held[in.UserID], in.RoleKey) {
		return false, err
	}
	if in.RoleKey == identity.RoleAdmin {
		admins := 0
		for _, keys := range f.held {
			if slices.Contains(keys, identity.RoleAdmin) {
				admins++
			}
		}
		if admins <= 1 {
			return false, appidentity.ErrLastAdmin
		}
	}
	f.held[in.UserID] = slices.DeleteFunc(f.held[in.UserID], func(k string) bool { return k == in.RoleKey })
	f.changes = append(f.changes, "revoke "+in.RoleKey+" from "+in.UserID+" by "+in.ByUserID)
	return true, nil
}

func (f *fakeRoles) Permissions(_ context.Context, userID string) ([]identity.Permission, error) {
	granted := map[identity.Permission]bool{}
	for _, k := range f.held[userID] {
		r, _ := f.role(k)
		for _, p := range r.Permissions {
			granted[p] = true
		}
	}
	out := []identity.Permission{}
	for _, p := range identity.AllPermissions() {
		if granted[p] {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRoles) HasPermission(ctx context.Context, userID string, p identity.Permission) (bool, error) {
	if f.anyHolderManagesRoles && p == identity.PermManageRoles && len(f.held[userID]) > 0 {
		return true, nil
	}
	perms, _ := f.Permissions(ctx, userID)
	return slices.Contains(perms, p), nil
}

const roleRemoteAccountID = "01890a5d-ac96-774b-bcce-b302099a8070"

// newRolesEnv is newAuthEnv with the role operations: alice is the only admin, bob holds no
// role, and a remote account exists.
func newRolesEnv(t *testing.T) (*authEnv, *fakeRoles) {
	t.Helper()
	f := &fakeRoles{held: map[string][]string{userOf(aliceAccountID): {identity.RoleAdmin}}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = f
		o.Roles = f
		accounts := o.Accounts.(fakeAccounts)
		accounts[bobAccountID] = appaccount.Actor{
			Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: adminNow},
			Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
			Lifecycle: account.InitialLifecycleState(bobAccountID),
		}
		accounts[roleRemoteAccountID] = appaccount.Actor{
			Account: account.Account{ID: roleRemoteAccountID, Domain: "remote.example", URI: "https://remote.example/users/carol", CreatedAt: adminNow},
		}
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func roleKeys(t *testing.T, res *http.Response) []string {
	t.Helper()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d %+v, want 200", res.StatusCode, decodeProblem(t, res))
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	keys := []string{}
	for _, r := range decodeJSON[clientapi.AdminRoleList](t, res).Items {
		keys = append(keys, r.Key)
	}
	return keys
}

func TestRoleOperationsNeedAPermission(t *testing.T) {
	e, f := newRolesEnv(t)
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/roles"},
		{http.MethodGet, "/admin/accounts/" + bobAccountID + "/roles"},
		{http.MethodPut, "/admin/accounts/" + bobAccountID + "/roles/moderator"},
		{http.MethodDelete, "/admin/accounts/" + aliceAccountID + "/roles/admin"},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", "")
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_roles") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_roles", op.method, op.path, res.StatusCode, p)
		}
	}
	res := e.send(t, http.MethodPut, "/admin/accounts/"+bobAccountID+"/roles/moderator", "admin-read", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("grant with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
	if len(f.changes) != 0 {
		t.Errorf("refused operations changed roles: %q", f.changes)
	}
}

func TestGrantAndRevokeRoles(t *testing.T) {
	e, f := newRolesEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/roles", "admin-read", nil)
	list := decodeJSON[clientapi.AdminRoleList](t, res)
	want := clientapi.AdminRoleList{Items: []clientapi.AdminRole{
		{Id: fakeRoleIDs["admin"], Key: "admin", Position: 1000, Permissions: []clientapi.Permission{
			"manage_reports", "manage_accounts", "manage_domains", "manage_rules", "manage_settings", "manage_roles",
			"manage_invites", "view_audit", "manage_jobs", "manage_login_recovery", "manage_custom_emojis", "manage_webhooks",
			"manage_announcements", "manage_finance", "view_finance", "manage_supporters", "create_custom_emojis", "manage_oauth_apps",
		}},
		{Id: fakeRoleIDs["moderator"], Key: "moderator", Position: 100, Permissions: []clientapi.Permission{"manage_reports", "manage_accounts", "view_audit"}},
	}}
	if res.StatusCode != http.StatusOK || !reflect.DeepEqual(list, want) {
		t.Errorf("list roles = %d %+v, want %+v", res.StatusCode, list, want)
	}
	if got := roleKeys(t, e.admin(t, http.MethodGet, "/admin/accounts/"+bobAccountID+"/roles", "admin-read", nil)); len(got) != 0 {
		t.Errorf("bob's roles = %q, want none", got)
	}

	// Granting admin to bob lets him manage roles from his next request.
	if got := roleKeys(t, e.send(t, http.MethodPut, "/admin/accounts/"+bobAccountID+"/roles/admin", "admin", "")); !reflect.DeepEqual(got, []string{"admin"}) {
		t.Errorf("grant admin = %q, want bob's roles [admin]", got)
	}
	if got := roleKeys(t, e.send(t, http.MethodPut, "/admin/accounts/"+bobAccountID+"/roles/moderator", "bob-admin", "")); !reflect.DeepEqual(got, []string{"admin", "moderator"}) {
		t.Errorf("bob grants himself moderator = %q", got)
	}
	// Granting a held role changes nothing.
	if got := roleKeys(t, e.send(t, http.MethodPut, "/admin/accounts/"+bobAccountID+"/roles/moderator", "admin", "")); len(got) != 2 {
		t.Errorf("granting again = %q", got)
	}
	if got := roleKeys(t, e.send(t, http.MethodDelete, "/admin/accounts/"+aliceAccountID+"/roles/admin", "bob-admin", "")); len(got) != 0 {
		t.Errorf("bob revokes alice's admin = %q, want none left", got)
	}
	// alice lost manage_roles with her role.
	if res := e.send(t, http.MethodGet, "/admin/roles", "admin", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("alice lists roles after losing admin = %d, want 403", res.StatusCode)
	}
	wantChanges := []string{
		"grant admin to " + userOf(bobAccountID) + " by " + userOf(aliceAccountID),
		"grant moderator to " + userOf(bobAccountID) + " by " + userOf(bobAccountID),
		"revoke admin from " + userOf(aliceAccountID) + " by " + userOf(bobAccountID),
	}
	if !reflect.DeepEqual(f.changes, wantChanges) {
		t.Errorf("changes = %q, want %q", f.changes, wantChanges)
	}
}

func TestRoleProblems(t *testing.T) {
	e, f := newRolesEnv(t)
	const unknown = "01890a5d-ac96-774b-bcce-b302099a8999"
	for _, tc := range []struct {
		name, method, path, token string
		status                    int
	}{
		{"unknown account", http.MethodPut, "/admin/accounts/" + unknown + "/roles/moderator", "admin", http.StatusNotFound},
		{"remote account", http.MethodPut, "/admin/accounts/" + roleRemoteAccountID + "/roles/moderator", "admin", http.StatusNotFound},
		{"roles of a remote account", http.MethodGet, "/admin/accounts/" + roleRemoteAccountID + "/roles", "admin", http.StatusNotFound},
		{"unknown role", http.MethodPut, "/admin/accounts/" + bobAccountID + "/roles/owner", "admin", http.StatusNotFound},
		{"revoke unknown role", http.MethodDelete, "/admin/accounts/" + bobAccountID + "/roles/owner", "admin", http.StatusNotFound},
		{"last admin", http.MethodDelete, "/admin/accounts/" + aliceAccountID + "/roles/admin", "admin", http.StatusConflict},
	} {
		res := e.send(t, tc.method, tc.path, tc.token, "")
		if p := decodeProblem(t, res); res.StatusCode != tc.status {
			t.Errorf("%s: %d %+v, want %d", tc.name, res.StatusCode, p, tc.status)
		}
	}
	// A moderator who may manage roles still cannot grant a role above its own.
	f.held[userOf(bobAccountID)] = []string{identity.RoleModerator}
	f.anyHolderManagesRoles = true
	res := e.send(t, http.MethodPut, "/admin/accounts/"+bobAccountID+"/roles/admin", "bob-admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeBlank {
		t.Errorf("a moderator grants admin = %d %+v, want 403", res.StatusCode, p)
	}
	if len(f.changes) != 0 {
		t.Errorf("changes = %q, want none", f.changes)
	}
}
