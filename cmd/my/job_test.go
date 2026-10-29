// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

func jobCmd(args []string, out, errOut *bytes.Buffer) int {
	return runJob(args, out, errOut)
}

func TestJobCommands(t *testing.T) {
	path := newAdminEnv(t)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	// Two dead jobs, the second failed later; 1767225600000 is 2026-01-01T00:00:00Z.
	if _, err := raw.ExecContext(t.Context(), `INSERT INTO dead_jobs
		(id, type, payload, idempotency_key, ordering_key, attempt, last_error, created_at, failed_at) VALUES
		('job-a', 'delivery.send', X'7B7D', NULL, NULL, 8, 'remote said
500', 1767225600000, 1767225600000),
		('job-b', 'media.fetch', X'7B7D', 'k-b', NULL, 1, 'bug', 1767225600000, 1767225660000)`); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCmd(t, jobCmd, "list")
	if code != 0 || !strings.HasPrefix(stdout, "ID ") ||
		strings.Index(stdout, "job-b") > strings.Index(stdout, "job-a") ||
		!strings.Contains(stdout, "2026-01-01T00:01:00Z") || !strings.Contains(stdout, "remote said 500") {
		t.Fatalf("job list = %d %q %q; want job-b before job-a, the time, and the error on one line", code, stdout, stderr)
	}
	if code, stdout, stderr := runCmd(t, jobCmd, "list", "-limit", "1"); code != 0 || strings.Contains(stdout, "job-a") || !strings.Contains(stdout, "job-b") {
		t.Fatalf("job list -limit 1 = %d %q %q", code, stdout, stderr)
	}

	code, stdout, stderr = runCmd(t, jobCmd, "retry", "job-a", "job-x")
	if code != 1 || stdout != "requeued dead job job-a\n" || !strings.Contains(stderr, `no dead job has the ID "job-x"`) {
		t.Fatalf("job retry = %d %q %q", code, stdout, stderr)
	}
	var runs int
	if err := raw.QueryRowContext(t.Context(), `SELECT count(*) FROM jobs WHERE id = 'job-a' AND type = 'delivery.send'`).Scan(&runs); err != nil || runs != 1 {
		t.Fatalf("jobs holding job-a = %d, %v; want 1", runs, err)
	}
	if code, stdout, stderr := runCmd(t, jobCmd, "delete", "job-b"); code != 0 || stdout != "deleted dead job job-b\n" {
		t.Fatalf("job delete = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, _ := runCmd(t, jobCmd, "list"); code != 0 || strings.Contains(stdout, "job-") {
		t.Fatalf("job list after retry and delete = %d %q; want no dead jobs", code, stdout)
	}
	if code, _, stderr := runCmd(t, jobCmd, "delete", "job-b"); code != 1 || !strings.Contains(stderr, `no dead job has the ID "job-b"`) {
		t.Errorf("job delete of a deleted job = %d %q", code, stderr)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"retry"}, "want the IDs of one or more dead jobs"},
		{[]string{"list", "job-a"}, "want 0 arguments"},
		{[]string{"list", "-limit", "0"}, "-limit must be positive"},
		{[]string{"rerun"}, "unknown subcommand"},
	} {
		if code, _, stderr := runCmd(t, jobCmd, tc.args...); code != 2 || !strings.Contains(stderr, tc.want) {
			t.Errorf("job %v = %d %q; want 2 containing %q", tc.args, code, stderr, tc.want)
		}
	}

	t.Setenv(config.EnvPrefix+"QUEUE", "inline")
	if code, _, stderr := runCmd(t, jobCmd, "list"); code != 1 || !strings.Contains(stderr, "only queue = db and cloudtasks do") {
		t.Errorf("job list with the inline queue = %d %q", code, stderr)
	}
}
