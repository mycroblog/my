// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
)

func TestValidateRoleKey(t *testing.T) {
	for key, ok := range map[string]bool{
		"helper": true, "a": true, "triage_2-b": true, strings.Repeat("a", 64): true,
		"": false, "Helper": false, "new": false, "news": true, "help me": false, "ヘルパー": false, strings.Repeat("a", 65): false,
	} {
		if err := identity.ValidateRoleKey(key); (err == nil) != ok || (err != nil && !errors.Is(err, identity.ErrInvalidRoleKey)) {
			t.Errorf("ValidateRoleKey(%q) = %v, want ok %v", key, err, ok)
		}
	}
}

func TestValidateRolePosition(t *testing.T) {
	for pos, ok := range map[int]bool{0: true, 999: true, -1: false, identity.AdminPosition: false} {
		if err := identity.ValidateRolePosition(pos); (err == nil) != ok {
			t.Errorf("ValidateRolePosition(%d) = %v, want ok %v", pos, err, ok)
		}
	}
}

func TestNormalizeRolePermissions(t *testing.T) {
	got, err := identity.NormalizeRolePermissions([]identity.Permission{identity.PermViewAudit, identity.PermManageReports})
	if err != nil || !reflect.DeepEqual(got, []identity.Permission{identity.PermManageReports, identity.PermViewAudit}) {
		t.Errorf("NormalizeRolePermissions = %v, %v; want in the order of AllPermissions", got, err)
	}
	if got, err := identity.NormalizeRolePermissions(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("NormalizeRolePermissions(nil) = %#v, %v; want an empty list", got, err)
	}
	for _, perms := range [][]identity.Permission{{"own_everything"}, {identity.PermViewAudit, identity.PermViewAudit}} {
		if _, err := identity.NormalizeRolePermissions(perms); !errors.Is(err, identity.ErrInvalidRolePermissions) {
			t.Errorf("NormalizeRolePermissions(%v) = %v, want ErrInvalidRolePermissions", perms, err)
		}
	}
}

func TestChangedAndHeldPermissions(t *testing.T) {
	before := []identity.Permission{identity.PermManageReports, identity.PermViewAudit}
	after := []identity.Permission{identity.PermManageReports, identity.PermManageJobs}
	changed := identity.ChangedPermissions(before, after)
	if want := []identity.Permission{identity.PermViewAudit, identity.PermManageJobs}; !reflect.DeepEqual(changed, want) {
		t.Errorf("ChangedPermissions = %v, want %v", changed, want)
	}
	if err := identity.CheckPermissionsHeld(before, changed); err == nil {
		t.Error("CheckPermissionsHeld = nil, want manage_jobs missing")
	} else if m := (*identity.MissingPermissionsError)(nil); !errors.As(err, &m) || !reflect.DeepEqual(m.Missing, []identity.Permission{identity.PermManageJobs}) {
		t.Errorf("CheckPermissionsHeld = %v, want manage_jobs missing", err)
	}
	if err := identity.CheckPermissionsHeld(identity.AllPermissions(), changed); err != nil {
		t.Errorf("CheckPermissionsHeld(all) = %v", err)
	}
}
