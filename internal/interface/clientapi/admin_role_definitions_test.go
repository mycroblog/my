// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// The role definitions of fakeRoles: the use cases' answers are set per test (defErr), and the
// fake records what it was asked.

func (f *fakeRoles) CreateRole(_ context.Context, in appidentity.CreateRoleInput) (appidentity.RoleWithPermissions, error) {
	if f.defErr != nil {
		return appidentity.RoleWithPermissions{}, f.defErr
	}
	f.changes = append(f.changes, fmt.Sprintf("create %s at %d with %v by %s", in.Key, in.Position, in.Permissions, in.ByUserID))
	return appidentity.RoleWithPermissions{
		Role: identity.Role{ID: "01890a5d-ac96-774b-bcce-b302099a5003", Key: in.Key, Position: in.Position}, Permissions: in.Permissions,
	}, nil
}

func (f *fakeRoles) UpdateRole(_ context.Context, in appidentity.UpdateRoleInput) (appidentity.RoleWithPermissions, error) {
	if f.defErr != nil {
		return appidentity.RoleWithPermissions{}, f.defErr
	}
	r, ok := f.role(in.Key)
	if !ok {
		return appidentity.RoleWithPermissions{}, fmt.Errorf("role %s: %w", in.Key, appidentity.ErrNotFound)
	}
	change := "update " + in.Key
	if in.Position != nil {
		r.Position = *in.Position
		change += fmt.Sprintf(" to %d", *in.Position)
	}
	if in.SetPermissions {
		r.Permissions = in.Permissions
		change += fmt.Sprintf(" with %v", in.Permissions)
	}
	if in.SetBadge {
		r.Badge = nil
		change += " without a badge"
		if b := in.Badge; b != nil {
			r.Badge = &identity.RoleBadge{Label: b.Label, Color: b.Color}
			change = strings.TrimSuffix(change, " without a badge") + fmt.Sprintf(" with the badge %s in %s", b.Label, b.Color)
		}
	}
	f.changes = append(f.changes, change+" by "+in.ByUserID)
	return r, nil
}

func (f *fakeRoles) DeleteRole(_ context.Context, key, byUserID string) error {
	if f.defErr != nil {
		return f.defErr
	}
	f.changes = append(f.changes, "delete "+key+" by "+byUserID)
	return nil
}

func TestRoleDefinitionsNeedAPermission(t *testing.T) {
	e, f := newRolesEnv(t)
	for _, op := range []struct{ method, path, body string }{
		{http.MethodPost, "/admin/roles", `{"key":"helper","position":1,"permissions":[]}`},
		{http.MethodPatch, "/admin/roles/moderator", `{"position":1}`},
		{http.MethodDelete, "/admin/roles/helper", ""},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_roles", op.method, op.path, res.StatusCode, p)
		}
		res = e.send(t, op.method, op.path, "admin-read", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
			t.Errorf("%s %s with admin:read = %d %+v, want 403 insufficient-scope", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.changes) != 0 {
		t.Errorf("refused operations changed roles: %q", f.changes)
	}
}

func TestCreateUpdateAndDeleteRoles(t *testing.T) {
	e, f := newRolesEnv(t)
	alice := userOf(aliceAccountID)

	res := e.send(t, http.MethodPost, "/admin/roles", "admin", `{"key":"helper","position":50,"permissions":["view_audit","manage_reports"]}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d %+v", res.StatusCode, decodeProblem(t, res))
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	want := clientapi.AdminRole{
		Id: "01890a5d-ac96-774b-bcce-b302099a5003", Key: "helper", Position: 50, Permissions: []clientapi.Permission{"view_audit", "manage_reports"},
	}
	if got := decodeJSON[clientapi.AdminRole](t, res); !reflect.DeepEqual(got, want) {
		t.Errorf("created = %+v, want %+v", got, want)
	}

	res = e.send(t, http.MethodPatch, "/admin/roles/moderator", "admin", `{"permissions":["manage_reports"]}`)
	if got := decodeJSON[clientapi.AdminRole](t, res); res.StatusCode != http.StatusOK || got.Position != 100 ||
		!reflect.DeepEqual(got.Permissions, []clientapi.Permission{"manage_reports"}) {
		t.Errorf("update moderator's permissions = %d %+v", res.StatusCode, got)
	}
	res = e.send(t, http.MethodPatch, "/admin/roles/moderator", "admin", `{"position":120,"permissions":[]}`)
	if got := decodeJSON[clientapi.AdminRole](t, res); res.StatusCode != http.StatusOK || got.Position != 120 || len(got.Permissions) != 0 {
		t.Errorf("update moderator = %d %+v, want 120 without permissions", res.StatusCode, got)
	}
	res = e.send(t, http.MethodPatch, "/admin/roles/moderator", "admin", `{"badge":{"label":"Staff","color":"purple"}}`)
	if got := decodeJSON[clientapi.AdminRole](t, res); res.StatusCode != http.StatusOK ||
		got.Badge == nil || *got.Badge != (clientapi.RoleBadge{Label: "Staff", Color: clientapi.BadgeColorPurple}) {
		t.Errorf("set moderator's badge = %d %+v, want Staff in purple", res.StatusCode, got)
	}
	res = e.send(t, http.MethodPatch, "/admin/roles/moderator", "admin", `{"remove_badge":true}`)
	if got := decodeJSON[clientapi.AdminRole](t, res); res.StatusCode != http.StatusOK || got.Badge != nil {
		t.Errorf("remove moderator's badge = %d %+v, want no badge", res.StatusCode, got)
	}
	res = e.send(t, http.MethodPatch, "/admin/roles/moderator", "admin", `{"remove_badge":true,"badge":{"label":"Staff","color":"red"}}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "remove_badge") {
		t.Errorf("badge with remove_badge = %d %+v, want 422 naming remove_badge", res.StatusCode, p)
	}
	if res := e.send(t, http.MethodDelete, "/admin/roles/helper", "admin", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("delete = %d, want 204", res.StatusCode)
	}
	wantChanges := []string{
		"create helper at 50 with [view_audit manage_reports] by " + alice,
		"update moderator with [manage_reports] by " + alice,
		"update moderator to 120 with [] by " + alice,
		"update moderator with the badge Staff in purple by " + alice,
		"update moderator without a badge by " + alice,
		"delete helper by " + alice,
	}
	if !reflect.DeepEqual(f.changes, wantChanges) {
		t.Errorf("changes = %q, want %q", f.changes, wantChanges)
	}
}

func TestRoleDefinitionProblems(t *testing.T) {
	e, f := newRolesEnv(t)
	type roleProblem struct {
		clientapi.Problem
		Reason             string                 `json:"reason"`
		MissingPermissions []clientapi.Permission `json:"missing_permissions"`
	}
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		reason  string
		missing []clientapi.Permission
	}{
		{"unknown role", fmt.Errorf("x: %w", appidentity.ErrNotFound), http.StatusNotFound, "", nil},
		{"above the viewer", fmt.Errorf("x: %w", appidentity.ErrRoleOutranksActor), http.StatusForbidden, "", nil},
		{"key", appidentity.ErrInvalidRoleKey, http.StatusUnprocessableEntity, "key", nil},
		{"key taken", fmt.Errorf("x: %w", appidentity.ErrRoleKeyTaken), http.StatusUnprocessableEntity, "key_taken", nil},
		{"position", appidentity.ErrInvalidRolePosition, http.StatusUnprocessableEntity, "position", nil},
		{"permissions", fmt.Errorf("x: %w", appidentity.ErrInvalidRolePermissions), http.StatusUnprocessableEntity, "permissions", nil},
		{"built-in", fmt.Errorf("x: %w", appidentity.ErrBuiltinRole), http.StatusUnprocessableEntity, "builtin", nil},
		{"held", fmt.Errorf("x: %w", appidentity.ErrRoleHeld), http.StatusUnprocessableEntity, "held", nil},
		{"badge label", appidentity.ErrInvalidBadgeLabel, http.StatusUnprocessableEntity, "badge_label", nil},
		{"badge color", fmt.Errorf("x: %w", appidentity.ErrInvalidBadgeColor), http.StatusUnprocessableEntity, "badge_color", nil},
		{
			"missing permission", &appidentity.MissingPermissionsError{Missing: []identity.Permission{identity.PermManageJobs}},
			http.StatusUnprocessableEntity, "missing_permission", []clientapi.Permission{"manage_jobs"},
		},
	} {
		f.defErr = tc.err
		for _, op := range []struct{ method, path, body string }{
			{http.MethodPost, "/admin/roles", `{"key":"helper","position":1,"permissions":[]}`},
			{http.MethodPatch, "/admin/roles/helper", `{"position":1}`},
			{http.MethodDelete, "/admin/roles/helper", ""},
		} {
			res := e.send(t, op.method, op.path, "admin", op.body)
			var p roleProblem
			if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.status || p.Reason != tc.reason || !reflect.DeepEqual(p.MissingPermissions, tc.missing) {
				t.Errorf("%s: %s %s = %d %+v, want %d %q %v", tc.name, op.method, op.path, res.StatusCode, p, tc.status, tc.reason, tc.missing)
			}
			if tc.reason != "" && (p.Type != clientapi.ProblemTypeInvalidRole || p.Detail == nil || *p.Detail == "") {
				t.Errorf("%s: %s %s = %+v, want invalid-role with a detail", tc.name, op.method, op.path, p)
			}
		}
	}
	f.defErr = nil
	if res := e.send(t, http.MethodPost, "/admin/roles", "admin", `{"key":`); res.StatusCode < 400 || res.StatusCode >= 500 {
		t.Errorf("create with a malformed body = %d, want a 4xx", res.StatusCode)
	} else if p := decodeProblem(t, res); strings.Contains(p.Type, "invalid-role") {
		t.Errorf("a malformed request = %+v, want no invalid-role", p)
	}
	if len(f.changes) != 0 {
		t.Errorf("changes = %q, want none", f.changes)
	}
}
