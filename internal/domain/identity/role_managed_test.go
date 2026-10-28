// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"testing"
)

// testMemberPermission is a member permission (ADR-0070 §1).
const testMemberPermission = PermViewFinance

// The ids of the roles of managedRoleFixtures; newRoleID and otherRoleID are not among them.
const (
	adminRoleID  = "01890a5d-ac96-7000-8000-000000000001"
	modRoleID    = "01890a5d-ac96-7000-8000-000000000002"
	helperRoleID = "01890a5d-ac96-7000-8000-000000000003"
	planRoleID   = "01890a5d-ac96-7000-8000-000000000004"
	fanRoleID    = "01890a5d-ac96-7000-8000-000000000005"
	badgeRoleID  = "01890a5d-ac96-7000-8000-000000000006"
	newRoleID    = "01890a5d-ac96-7000-8000-000000000007"
	otherRoleID  = "01890a5d-ac96-7000-8000-000000000008"
)

func managedRoleFixtures() []RoleFacts {
	return []RoleFacts{
		{Role: Role{ID: adminRoleID, Key: RoleAdmin, Position: AdminPosition}, Permissions: AllPermissions()},
		{Role: Role{ID: modRoleID, Key: RoleModerator, Position: 100}, Permissions: []Permission{PermManageReports}},
		{Role: Role{ID: helperRoleID, Key: "helper", Position: 50}, Permissions: []Permission{PermManageCustomEmojis}},
		{Role: Role{ID: planRoleID, Key: "plan", Position: 10}, Permissions: []Permission{testMemberPermission}, Managed: true},
		{Role: Role{ID: fanRoleID, Key: "fan", Position: 20}, Permissions: []Permission{testMemberPermission}},
		{Role: Role{ID: badgeRoleID, Key: "badge", Position: 70}},
	}
}

// A plan links only a role that nobody holds, that is not built in nor linked already, whose
// permissions are member permissions, and that is below every staff role (ADR-0072 §5).
func TestCheckManageable(t *testing.T) {
	roles := managedRoleFixtures()
	byID := map[string]RoleFacts{}
	for _, r := range roles {
		byID[r.Role.ID] = r
	}
	for _, tc := range []struct {
		name    string
		role    RoleFacts
		holders int64
		want    ManageRule
	}{
		{"member role below staff", byID[fanRoleID], 0, ""},
		{"no permissions below staff", RoleFacts{Role: Role{ID: newRoleID, Key: "new", Position: 49}}, 0, ""},
		{"every member permission below staff", RoleFacts{Role: Role{ID: newRoleID, Key: "new", Position: 49},
			Permissions: []Permission{PermViewFinance, PermCreateCustomEmojis, PermStartVoiceRooms, PermCreateInvites}}, 0, ""},
		{"admin", byID[adminRoleID], 0, RuleBuiltin},
		{"moderator", byID[modRoleID], 0, RuleBuiltin},
		{"managed already", byID[planRoleID], 0, RuleManaged},
		{"held", byID[fanRoleID], 1, RuleHeld},
		{"staff permission", byID[helperRoleID], 0, RulePermissions},
		{"server permission below staff", RoleFacts{Role: Role{ID: otherRoleID, Key: "x", Position: 1}, Permissions: []Permission{PermManageSupporters}}, 0, RulePermissions},
		{"at the lowest staff role", RoleFacts{Role: Role{ID: otherRoleID, Key: "x", Position: 50}}, 0, RulePosition},
		{"above a staff role", byID[badgeRoleID], 0, RulePosition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckManageable(tc.role, tc.holders, roles)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("CheckManageable = %v, want nil", err)
				}
				return
			}
			var ue *UnmanageableRoleError
			if !errors.As(err, &ue) || ue.Rule != tc.want || !errors.Is(err, ErrRoleNotManageable) {
				t.Fatalf("CheckManageable = %v, want rule %s", err, tc.want)
			}
		})
	}
}

// The plan form keeps a managed role to member permissions below every staff role.
func TestCheckManagedShape(t *testing.T) {
	roles := managedRoleFixtures()
	if err := CheckManagedShape(planRoleID, 49, []Permission{testMemberPermission}, roles); err != nil {
		t.Errorf("below the lowest staff role: %v", err)
	}
	if err := CheckManagedShape(planRoleID, 49, []Permission{PermStartVoiceRooms}, roles); err != nil {
		t.Errorf("start_voice_rooms (ADR-0100 §2): %v", err)
	}
	if err := CheckManagedShape(planRoleID, 50, nil, roles); !errors.Is(err, ErrRoleNotManageable) {
		t.Errorf("at the lowest staff role: %v", err)
	}
	if err := CheckManagedShape(planRoleID, 5, []Permission{PermViewAudit}, roles); !errors.Is(err, ErrRoleNotManageable) {
		t.Errorf("a moderation permission: %v", err)
	}
}

// A change to a role that is not managed never puts a staff role at or below a managed one.
func TestCheckUnmanagedChange(t *testing.T) {
	roles := managedRoleFixtures()
	for _, tc := range []struct {
		name     string
		id, key  string
		position int
		perms    []Permission
		ok       bool
	}{
		{"staff role above the managed ones", helperRoleID, "helper", 11, []Permission{PermManageCustomEmojis}, true},
		{"staff role moved onto a managed one", helperRoleID, "helper", 10, []Permission{PermManageCustomEmojis}, false},
		{"moderator moved below a managed one", modRoleID, RoleModerator, 5, nil, false},
		{"staff permission added below a managed one", fanRoleID, "fan", 5, []Permission{PermManageReports}, false},
		{"member permission below a managed one", fanRoleID, "fan", 5, []Permission{testMemberPermission}, true},
		{"new staff role below a managed one", "", "new", 3, []Permission{PermManageJobs}, false},
		{"new role without permissions", "", "new", 3, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckUnmanagedChange(tc.id, tc.key, tc.position, tc.perms, roles)
			if tc.ok != (err == nil) || (err != nil && !errors.Is(err, ErrBreaksManagedRoles)) {
				t.Fatalf("CheckUnmanagedChange = %v, want ok %v", err, tc.ok)
			}
		})
	}
	// Without a managed role, anything goes.
	if err := CheckUnmanagedChange(modRoleID, RoleModerator, 0, nil, roles[:3]); err != nil {
		t.Errorf("without managed roles: %v", err)
	}
}
