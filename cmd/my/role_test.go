// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func adminRole(args []string, out, errOut *bytes.Buffer) int { return runAdminRole(args, out, errOut) }

func adminAudit(args []string, out, errOut *bytes.Buffer) int {
	return runAdminAudit(args, out, errOut)
}

func TestAdminRoleCommands(t *testing.T) {
	newAdminEnv(t)
	for _, name := range []string{"root", "bob"} {
		if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", name, "-email", name+"@example.com"); code != 0 {
			t.Fatalf("admin create %s: %d %q", name, code, stderr)
		}
	}
	code, stdout, stderr := runCmd(t, adminRole, "list")
	if code != 0 || !regexp.MustCompile(`admin +1000 +manage_reports,\S+ +manage_rules,\S+ +view_finance,create_custom_emojis,start_voice_rooms,create_invites +(@root,@bob|@bob,@root)`).MatchString(stdout) ||
		!regexp.MustCompile(`moderator +100 +manage_reports,manage_accounts,view_audit +- +- +-`).MatchString(stdout) {
		t.Errorf("role list = %d %q\n%s", code, stderr, stdout)
	}
	if code, stdout, stderr = runCmd(t, adminRole, "grant", "@bob", "moderator"); code != 0 || !regexp.MustCompile(`^granted the role moderator to @bob \(user \S+\)\n$`).MatchString(stdout) {
		t.Errorf("role grant = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, _ = runCmd(t, adminRole, "grant", "bob", "moderator"); code != 0 || stdout != "@bob already holds the role moderator; nothing was changed\n" {
		t.Errorf("granting again = %d %q", code, stdout)
	}
	if code, stdout, _ = runCmd(t, adminRole, "list", "-user", "bob"); code != 0 || !strings.Contains(stdout, "admin ") || !strings.Contains(stdout, "moderator ") {
		t.Errorf("role list -user bob = %d\n%s", code, stdout)
	}
	if code, stdout, stderr = runCmd(t, adminRole, "revoke", "root", "admin"); code != 0 || !strings.HasPrefix(stdout, "revoked the role admin from @root") {
		t.Errorf("role revoke = %d %q %q", code, stdout, stderr)
	}
	if code, _, stderr = runCmd(t, adminRole, "revoke", "bob", "admin"); code != 1 || !strings.Contains(stderr, "@bob is the only admin") {
		t.Errorf("revoking the last admin = %d %q", code, stderr)
	}
	if code, _, stderr = runCmd(t, adminRole, "grant", "bob", "owner"); code != 1 || !strings.Contains(stderr, `no role has the key "owner"`) {
		t.Errorf("granting an unknown role = %d %q", code, stderr)
	}
	if code, _, stderr = runCmd(t, adminRole, "grant", "nobody", "moderator"); code != 1 || !strings.Contains(stderr, `no local account has the username "nobody"`) {
		t.Errorf("granting to an unknown user = %d %q", code, stderr)
	}
	if code, stdout, _ = runCmd(t, adminAudit, "list", "-target-kind", "role"); code != 0 ||
		!regexp.MustCompile(`cli +role_revoked +role admin @root`).MatchString(stdout) || !regexp.MustCompile(`cli +role_granted +role moderator @bob`).MatchString(stdout) {
		t.Errorf("audit list of roles = %d\n%s", code, stdout)
	}
	for _, args := range [][]string{{}, {"burn"}, {"grant", "bob"}, {"list", "extra"}} {
		if code, _, _ := runCmd(t, adminRole, args...); code != 2 {
			t.Errorf("admin role %q = %d, want 2", args, code)
		}
	}
}
