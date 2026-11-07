// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
)

// TestSingleUserFromTheCLI checks that `my instance set single_user true` needs exactly one local
// account, and that `my admin create` creates no other account on a one-person server (ADR-0152).
func TestSingleUserFromTheCLI(t *testing.T) {
	newAdminEnv(t)
	if code, _, stderr := runInstanceCmd(t, "", "set", "single_user", "true"); code == 0 || !strings.Contains(stderr, "exactly one local account; it has 0") {
		t.Errorf("single_user without accounts = %d %q, want the refusal with the count", code, stderr)
	}
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "root", "-email", "root@example.com"); code != 0 {
		t.Fatalf("admin create = %d %q", code, stderr)
	}
	if code, _, stderr := runInstanceCmd(t, "", "set", "single_user", "true"); code != 0 {
		t.Fatalf("single_user with one account = %d %q", code, stderr)
	}
	code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "other", "-email", "other@example.com")
	if code == 0 || !strings.Contains(stderr, "one-person server") {
		t.Errorf("admin create on a one-person server = %d %q, want the refusal", code, stderr)
	}
	if code, _, stderr := runInstanceCmd(t, "", "set", "single_user", "false"); code != 0 {
		t.Errorf("turning single_user off = %d %q", code, stderr)
	}
}
