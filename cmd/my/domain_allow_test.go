// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
)

func TestAdminDomainAllowAndFederationScope(t *testing.T) {
	newAdminEnv(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"domain", "allow", "add", "-comment", "partner", "Friend.example"}, "allowed friend.example and its subdomains\n"},
		{[]string{"domain", "allow", "add", "friend.example"}, "friend.example is already allowed\n"},
		{[]string{"domain", "allow", "add", "spare.example"}, "allowed spare.example and its subdomains\n"},
		{[]string{"domain", "allow", "remove", "spare.example"}, "removed the allow of spare.example\n"},
		{[]string{"domain", "allow", "remove", "spare.example"}, "spare.example has no allow\n"},
		{[]string{"federation", "scope"}, "federation_scope is open\n"},
	} {
		if code, stdout, stderr := runCmd(t, adminCmd, tc.args...); code != 0 || stdout != tc.want {
			t.Errorf("%v = %d %q %q; want %q", tc.args, code, stdout, stderr, tc.want)
		}
	}
	code, stdout, stderr := runCmd(t, adminCmd, "domain", "allow", "list")
	if code != 0 || !strings.Contains(stdout, "friend.example") || !strings.Contains(stdout, "partner") || strings.Contains(stdout, "spare.example") {
		t.Errorf("domain allow list = %d %q %q; want friend.example with its comment", code, stdout, stderr)
	}
	// Without -apply, the switch only shows what it would end.
	code, stdout, stderr = runCmd(t, adminCmd, "federation", "scope", "allowlist")
	if code != 0 || !strings.Contains(stdout, "ends 0 follows of 0 local accounts with 0 servers") || !strings.Contains(stdout, "nothing changed") {
		t.Errorf("federation scope allowlist = %d %q %q; want the impact and nothing changed", code, stdout, stderr)
	}
	if _, stdout, _ = runCmd(t, adminCmd, "federation", "scope"); stdout != "federation_scope is open\n" {
		t.Errorf("after the preview: %q, want still open", stdout)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"federation", "scope", "-apply", "allowlist"}, "federation_scope is now allowlist\n"},
		{[]string{"federation", "scope", "allowlist"}, "federation_scope is already allowlist\n"},
		{[]string{"federation", "scope", "open"}, "federation_scope is now open\n"},
	} {
		if code, stdout, stderr := runCmd(t, adminCmd, tc.args...); code != 0 || !strings.HasSuffix(stdout, tc.want) {
			t.Errorf("%v = %d %q %q; want %q", tc.args, code, stdout, stderr, tc.want)
		}
	}
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"domain", "allow", "add"}, 2, "want 1 argument(s)"},
		{[]string{"domain", "allow", "add", "self.example.com/x"}, 1, "my admin domain allow add"},
		{[]string{"federation", "scope", "closed"}, 2, "must be open or allowlist"},
		{[]string{"federation", "scope", "open", "allowlist"}, 2, "want at most 1 argument(s)"},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("%v = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
