// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
	"time"
)

func TestAdminClosureCommands(t *testing.T) {
	newAdminEnv(t)
	if code, stdout, stderr := runCmd(t, adminCmd, "closure", "status"); code != 0 || stdout != "state: none\n" {
		t.Errorf("closure status before an announcement = %d %q %q", code, stdout, stderr)
	}

	// A notice of 10 days is announced, with a warning.
	soon := time.Now().UTC().Add(10 * 24 * time.Hour).Truncate(time.Second)
	code, stdout, stderr := runCmd(t, adminCmd, "closure", "announce", "-closes-at", soon.Format(time.RFC3339), "-message", "Thank you.")
	if code != 0 || !strings.Contains(stdout, "state: announced\n") || !strings.Contains(stdout, "closes at: "+soon.Format(time.RFC3339)+"\n") ||
		!strings.Contains(stdout, "export until: "+soon.Add(30*24*time.Hour).Format(time.RFC3339)+"\n") ||
		!strings.Contains(stdout, "message: Thank you.\n") || !strings.Contains(stderr, "warning: the announcement gives the users less than 30 days") {
		t.Errorf("closure announce (short) = %d %q %q", code, stdout, stderr)
	}
	// A change of the dates with a notice of 60 days has no warning.
	later := time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.DateOnly)
	code, stdout, stderr = runCmd(t, adminCmd, "closure", "announce", "-closes-at", later, "-export-until", later)
	if code != 0 || !strings.Contains(stdout, "closes at: "+later+"T00:00:00Z\n") || !strings.Contains(stdout, "export until: "+later+"T00:00:00Z\n") || stderr != "" {
		t.Errorf("closure announce (change of the dates) = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "closure", "status"); code != 0 || !strings.Contains(stdout, "closes at: "+later) {
		t.Errorf("closure status = %d %q", code, stdout)
	}
	if code, stdout, stderr := runCmd(t, adminCmd, "closure", "withdraw"); code != 0 || stdout != "state: none\n" {
		t.Errorf("closure withdraw = %d %q %q", code, stdout, stderr)
	}
	if code, _, stderr := runCmd(t, adminCmd, "closure", "withdraw"); code != 1 || !strings.Contains(stderr, "nothing was changed") {
		t.Errorf("closure withdraw without an announcement = %d %q", code, stderr)
	}
	code, stdout, _ = runCmd(t, adminCmd, "audit", "list", "-target-kind", "instance")
	if code != 0 || strings.Count(stdout, "service_closure_announced") != 2 || strings.Count(stdout, "service_closure_withdrawn") != 1 ||
		!strings.Contains(stdout, " cli ") {
		t.Errorf("audit list of the closure = %d\n%s", code, stdout)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"closure", "announce"}, 2, "-closes-at must be"},
		{[]string{"closure", "announce", "-closes-at", "tomorrow"}, 2, "-closes-at must be"},
		{[]string{"closure", "announce", "-closes-at", later, "-export-until", "never"}, 2, "-export-until must be"},
		{[]string{"closure", "announce", "-closes-at", later, "-export-until", "2000-01-01"}, 1, "export deadline is before the closing date"},
		{[]string{"closure", "status", "extra"}, 2, "want 0 argument(s)"},
		{[]string{"closure", "close"}, 2, `unknown subcommand "close"`},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
