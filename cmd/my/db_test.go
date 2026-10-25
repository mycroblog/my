// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

func dbCmd(args []string, out, errOut *bytes.Buffer) int { return runDB(args, out, errOut) }

func TestDBUpgradeWithoutSteps(t *testing.T) {
	newAdminEnv(t)
	if code, stdout, stderr := runCmd(t, dbCmd, "upgrade", "-check"); code != 0 || stdout != "ok: the database can be upgraded to this release\n" {
		t.Errorf("db upgrade -check = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runCmd(t, dbCmd, "upgrade"); code != 0 || stdout != "ok: no data upgrade step to run\n" {
		t.Errorf("db upgrade = %d %q %q", code, stdout, stderr)
	}
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{nil, 2, "Usage:"},
		{[]string{"downgrade"}, 2, `unknown subcommand "downgrade"`},
		{[]string{"upgrade", "now"}, 2, "want 0 argument(s)"},
	} {
		if code, _, stderr := runCmd(t, dbCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("db %v = %d %q; want %d containing %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

func TestDBUpgradeRunsAndChecksSteps(t *testing.T) {
	newAdminEnv(t)
	runs, fail := 0, true
	dbAppOptions = platform.AppOptions{DataUpgrades: &platform.DataUpgrades{
		Steps: []dbupgrade.Step{
			{Key: "test.retired", Description: "carried over by an earlier release"},
			{
				Key: "test.backfill", Description: "fill the new column", Projections: []string{"account"},
				Run: func(context.Context, uow.UnitOfWork) error {
					runs++
					if fail {
						return errors.New("interrupted")
					}
					return nil
				},
			},
		},
		Prerequisites: []dbupgrade.Prerequisite{{Key: "test.retired", Via: "v0.1.3"}},
	}}
	t.Cleanup(func() { dbAppOptions = platform.AppOptions{} })

	code, stdout, stderr := runCmd(t, dbCmd, "upgrade", "-check")
	if code != 1 || stdout != "" ||
		!strings.Contains(stderr, "  test.retired (run by my db upgrade of v0.1.3)\n") ||
		!strings.Contains(stderr, "upgrade through v0.1.3 first") {
		t.Fatalf("db upgrade -check on a database without the prerequisite = %d %q %q", code, stdout, stderr)
	}

	code, stdout, stderr = runCmd(t, dbCmd, "upgrade")
	if code != 1 || !strings.Contains(stdout, "test.retired: done\n") || strings.Contains(stdout, "test.backfill: done") ||
		!strings.Contains(stderr, "step test.backfill: interrupted") || !strings.Contains(stderr, "run my db upgrade again to resume") {
		t.Fatalf("db upgrade with a failing step = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runCmd(t, dbCmd, "upgrade", "--check"); code != 0 {
		t.Errorf("db upgrade --check after the prerequisite ran = %d %q %q", code, stdout, stderr)
	}

	fail = false
	code, stdout, stderr = runCmd(t, dbCmd, "upgrade")
	if want := "test.backfill: running: fill the new column\ntest.backfill: done\nok: 1 data upgrade step(s) run\n"; code != 0 || stdout != want {
		t.Fatalf("db upgrade again = %d %q %q; want %q", code, stdout, stderr, want)
	}
	if code, stdout, stderr := runCmd(t, dbCmd, "upgrade"); code != 0 || stdout != "ok: no data upgrade step to run\n" || runs != 2 {
		t.Errorf("db upgrade a third time = %d %q %q (ran the step %d times); want nothing to run", code, stdout, stderr, runs)
	}

	// A database without any table is new: the check passes.
	t.Setenv(config.EnvPrefix+"DATABASE_URL", filepath.Join(t.TempDir(), "new.db"))
	if code, stdout, stderr := runCmd(t, dbCmd, "upgrade", "-check"); code != 0 || stdout != "ok: the database is new; there is no data to carry over\n" {
		t.Errorf("db upgrade -check on a new database = %d %q %q", code, stdout, stderr)
	}
	if code, _, stderr := runCmd(t, dbCmd, "upgrade"); code != 1 || !strings.Contains(stderr, "the schema of this release is not applied") {
		t.Errorf("db upgrade on a new database = %d %q; want the schema to be applied first", code, stderr)
	}
}
