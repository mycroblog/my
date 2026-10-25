// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
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
		{[]string{"closure", "close", "extra"}, 2, "want 0 argument(s)"},
		{[]string{"closure", "end"}, 2, `unknown subcommand "end"`},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

// adminCmdWithStdin returns adminCmd with stdin as the standard input.
func adminCmdWithStdin(stdin string) func([]string, *bytes.Buffer, *bytes.Buffer) int {
	return func(args []string, out, errOut *bytes.Buffer) int {
		return runAdmin(args, strings.NewReader(stdin), out, errOut)
	}
}

func TestAdminClosureClose(t *testing.T) {
	newAdminEnv(t)
	// No announcement: refused before any question.
	if code, _, stderr := runCmd(t, adminCmdWithStdin("y\nsocial.example\n"), "closure", "close"); code != 1 || !strings.Contains(stderr, "nothing was changed") ||
		strings.Contains(stderr, "for good?") {
		t.Errorf("close without an announcement = %d %q", code, stderr)
	}
	// The closing date has not come.
	later := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if code, _, stderr := runCmd(t, adminCmd, "closure", "announce", "-closes-at", later); code != 0 {
		t.Fatalf("announce = %d %q", code, stderr)
	}
	if code, _, stderr := runCmd(t, adminCmd, "closure", "close", "-confirm-domain", "social.example"); code != 1 || !strings.Contains(stderr, "closing date has not come") {
		t.Errorf("close before the closing date = %d %q", code, stderr)
	}
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if code, _, stderr := runCmd(t, adminCmd, "closure", "announce", "-closes-at", past); code != 0 {
		t.Fatalf("announce = %d %q", code, stderr)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "closure", "check"); code != 0 || !strings.Contains(stdout, "estimated Delete deliveries: 0\n") ||
		!strings.Contains(stdout, "live supporter subscriptions: 0\n") || !strings.Contains(stdout, "pending migrations: 0\n") {
		t.Errorf("check = %d %q", code, stdout)
	}
	// Without both confirmations, nothing closes.
	for _, tc := range []struct{ name, stdin, want string }{
		{"no answer", "", "not confirmed"},
		{"no", "n\nsocial.example\n", "not confirmed"},
		{"another domain", "y\nother.example\n", "the domain does not match"},
		{"no domain", "y\n", "the domain does not match"},
	} {
		code, stdout, stderr := runCmd(t, adminCmdWithStdin(tc.stdin), "closure", "close")
		if code != 1 || !strings.Contains(stderr, tc.want) || !strings.Contains(stdout, "state: announced\n") || !strings.Contains(stdout, "retention holds: 0\n") {
			t.Errorf("close (%s) = %d %q %q", tc.name, code, stdout, stderr)
		}
	}
	if code, _, stderr := runCmd(t, adminCmd, "closure", "close", "-confirm-domain", "other.example"); code != 1 || !strings.Contains(stderr, "does not match") {
		t.Errorf("close with another -confirm-domain = %d %q", code, stderr)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "closure", "status"); code != 0 || !strings.Contains(stdout, "state: announced\n") {
		t.Fatalf("a refused closure closed: %q", stdout)
	}
	if code, stdout, stderr := runCmd(t, adminCmdWithStdin("y\nSocial.Example\n"), "closure", "close"); code != 0 ||
		strings.Count(stdout, "state: ") != 2 || !strings.Contains(stdout, "state: closed\n") {
		t.Errorf("close = %d %q %q", code, stdout, stderr)
	}
	if code, _, stderr := runCmd(t, adminCmd, "closure", "close", "-confirm-domain", "social.example"); code != 1 || !strings.Contains(stderr, "nothing was changed") {
		t.Errorf("close twice = %d %q", code, stderr)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "audit", "list", "-target-kind", "instance"); code != 0 || strings.Count(stdout, "service_closure_closed") != 1 {
		t.Errorf("audit list of the closure = %d\n%s", code, stdout)
	}
}
