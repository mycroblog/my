// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"database/sql"
	"regexp"
	"strings"
	"testing"
)

func projectionCmd(args []string, out, errOut *bytes.Buffer) int {
	return runProjection(args, out, errOut)
}

func TestProjectionCommands(t *testing.T) {
	path := newAdminEnv(t)
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "root", "-email", "root@example.com"); code != 0 {
		t.Fatalf("admin create: %d %q", code, stderr)
	}
	code, stdout, stderr := runCmd(t, projectionCmd, "list")
	if code != 0 || !regexp.MustCompile(`(?m)^account +account +account_handles, account_profile_heads, account_lifecycle_states, remote_account_heads, account_move_states$`).MatchString(stdout) ||
		!strings.Contains(stdout, "notification_deleted_statuses") {
		t.Fatalf("projection list = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runCmd(t, projectionCmd, "verify"); code != 0 || strings.Count(stdout, ": ok\n") != strings.Count(stdout, "\n") {
		t.Fatalf("projection verify = %d %q %q; want every projection ok", code, stdout, stderr)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(t.Context(), `UPDATE account_handles SET username = 'mallory'`); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr = runCmd(t, projectionCmd, "verify", "account_handles")
	if code != 1 || !strings.Contains(stdout, "account: differs from its sources\n  account_handles: 1 extra, 1 missing\n") ||
		!strings.Contains(stdout, `    - account_id=`) || !strings.Contains(stdout, `username="mallory"`) ||
		!strings.Contains(stdout, `    + account_id=`) || !strings.Contains(stdout, `username="root"`) ||
		!strings.Contains(stderr, "run my projection rebuild") {
		t.Fatalf("projection verify after breaking a handle = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd(t, projectionCmd, "rebuild", "account")
	if code != 0 || stdout != "account: rebuilt (1 rows deleted, 1 inserted)\n" {
		t.Fatalf("projection rebuild = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runCmd(t, projectionCmd, "verify", "account"); code != 0 || stdout != "account: ok\n" {
		t.Errorf("projection verify after rebuild = %d %q %q", code, stdout, stderr)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"verify", "no_such_projection"}, 2, `unknown projection "no_such_projection"`},
		{[]string{"verify", "a", "b"}, 2, "want at most 1 argument(s)"},
		{[]string{"list", "account"}, 2, "want at most 0 argument(s)"},
		{[]string{"repair"}, 2, "unknown subcommand"},
	} {
		code, _, stderr := runCmd(t, projectionCmd, tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("projection %v = %d %q; want %d containing %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
