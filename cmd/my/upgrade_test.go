// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

func upgradeCmd(args []string, out, errOut *bytes.Buffer) int { return runUpgrade(args, out, errOut) }

// The step lines of my upgrade.
const (
	stepConfig  = "==> my config check\n"
	stepCheck   = "==> my db upgrade -check\n"
	stepApply   = "==> my schema apply\n"
	stepDryRun  = "==> my schema apply -dry-run\n"
	stepUpgrade = "==> my db upgrade\n"
)

// stepsInOrder reports whether the step lines of stdout are steps, in that order.
func stepsInOrder(stdout string, steps ...string) bool {
	var got []string
	for line := range strings.Lines(stdout) {
		if strings.HasPrefix(line, "==> ") {
			got = append(got, line)
		}
	}
	return strings.Join(got, "") == strings.Join(steps, "")
}

// newUpgradeEnv sets up the environment of newAdminEnv with a new SQLite database, without any
// table, at the path it returns. The development profile keeps the production checks of my config
// check, which the test host and key fail, out of the way.
func newUpgradeEnv(t *testing.T) string {
	t.Helper()
	newAdminEnv(t)
	path := filepath.Join(t.TempDir(), "new.db")
	t.Setenv(config.EnvPrefix+"DATABASE_URL", path)
	t.Setenv(config.EnvPrefix+"PROFILE", "development")
	return path
}

func TestUpgrade(t *testing.T) {
	path := newUpgradeEnv(t)

	code, stdout, stderr := runCmd(t, upgradeCmd, "-dry-run")
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, stepDryRun) ||
		!strings.Contains(stdout, "configuration OK\n") ||
		!strings.Contains(stdout, "ok: the database can be upgraded to this release\n") ||
		!strings.Contains(stdout, "none applied (dry run)\n") || !strings.HasSuffix(stdout, "nothing was changed\n") {
		t.Fatalf("upgrade -dry-run = %d %q %q", code, stdout, stderr)
	}
	if n := countTables(t, path); n != 0 {
		t.Errorf("the dry run created %d table(s)", n)
	}

	code, stdout, stderr = runCmd(t, upgradeCmd)
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, stepApply, stepUpgrade) ||
		!strings.Contains(stdout, "statement(s) applied\n") ||
		!strings.HasSuffix(stdout, "ok: the database is upgraded to this release\n") {
		t.Fatalf("upgrade = %d %q %q", code, stdout, stderr)
	}
	if n := countTables(t, path); n == 0 {
		t.Error("upgrade created no table")
	}

	// A second run has nothing to do.
	code, stdout, stderr = runCmd(t, upgradeCmd)
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, stepApply, stepUpgrade) ||
		!strings.Contains(stdout, "ok: the database can be upgraded to this release\n") ||
		!strings.Contains(stdout, "ok: the database matches the schema; nothing to apply\n") ||
		!strings.Contains(stdout, "ok: no data upgrade step to run\n") {
		t.Errorf("upgrade again = %d %q %q", code, stdout, stderr)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"now"}, "want 0 argument(s)"},
		{[]string{"-check"}, "flag provided but not defined"},
	} {
		if code, stdout, stderr := runCmd(t, upgradeCmd, tc.args...); code != 2 || stdout != "" || !strings.Contains(stderr, tc.want) {
			t.Errorf("upgrade %v = %d %q %q; want 2 containing %q and no step", tc.args, code, stdout, stderr, tc.want)
		}
	}
}

func TestUpgradeDestructive(t *testing.T) {
	path := newUpgradeEnv(t)
	if code, stdout, stderr := runCmd(t, upgradeCmd); code != 0 {
		t.Fatalf("upgrade = %d %q %q", code, stdout, stderr)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE probe_extra (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	tables := countTables(t, path)

	// Without -allow-destructive the plan is refused, and my db upgrade does not run.
	code, stdout, stderr := runCmd(t, upgradeCmd)
	if code != 1 || !stepsInOrder(stdout, stepConfig, stepCheck, stepApply) ||
		!strings.Contains(stderr, "the plan drops table probe_extra") ||
		!strings.HasSuffix(stderr, "my upgrade: stopped: my schema apply failed, and the steps after it did not run\n") {
		t.Errorf("upgrade of a drop = %d %q %q", code, stdout, stderr)
	}

	// -dry-run changes nothing, also with -allow-destructive.
	code, stdout, stderr = runCmd(t, upgradeCmd, "-dry-run", "-allow-destructive")
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, "==> my schema apply -dry-run -allow-destructive\n") ||
		!strings.Contains(stdout, "DROP TABLE") {
		t.Errorf("upgrade -dry-run -allow-destructive = %d %q %q", code, stdout, stderr)
	}
	if n := countTables(t, path); n != tables {
		t.Errorf("the dry run left %d table(s), want %d", n, tables)
	}

	code, stdout, stderr = runCmd(t, upgradeCmd, "-allow-destructive")
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, "==> my schema apply -allow-destructive\n", stepUpgrade) ||
		!strings.Contains(stdout, "statement(s) applied\n") {
		t.Errorf("upgrade -allow-destructive = %d %q %q", code, stdout, stderr)
	}
	if n := countTables(t, path); n != tables-1 {
		t.Errorf("upgrade -allow-destructive left %d table(s), want %d", n, tables-1)
	}
}

func TestUpgradeStopsAtAFailedConfigCheck(t *testing.T) {
	path := newUpgradeEnv(t)
	t.Setenv(config.EnvPrefix+"PROFILE", "production")
	t.Setenv(config.EnvPrefix+"KEK", base64.StdEncoding.EncodeToString(make([]byte, 32))) // not random
	t.Setenv("LC_ALL", "C")

	code, stdout, stderr := runCmd(t, upgradeCmd)
	if code != 1 || !stepsInOrder(stdout, stepConfig) || !strings.Contains(stderr, "MY_KEK: is not random") ||
		!strings.HasSuffix(stderr, "my upgrade: stopped: my config check failed, and the steps after it did not run\n") {
		t.Errorf("upgrade with an invalid configuration = %d %q %q", code, stdout, stderr)
	}
	if n := countTables(t, path); n != 0 {
		t.Errorf("upgrade after a failed check created %d table(s)", n)
	}
}
