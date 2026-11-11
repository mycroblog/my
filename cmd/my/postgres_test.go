// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

// TestMain stops the PostgreSQL server of the tests, if one started.
func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

// TestPostgresUpgrade runs my upgrade on a new PostgreSQL database, twice. Applying the schema
// needs a real server (scripts/task.sh go:test-postgres).
func TestPostgresUpgrade(t *testing.T) {
	pgtest.RequireRealServer(t)
	newUpgradeEnv(t)
	t.Setenv(config.EnvPrefix+"DATABASE", "postgres")
	t.Setenv(config.EnvPrefix+"DATABASE_URL", pgtest.NewEmptyDatabase(t))

	code, stdout, stderr := runCmd(t, upgradeCmd, "-dry-run")
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, stepDryRun) || !strings.Contains(stdout, "none applied (dry run)\n") {
		t.Fatalf("upgrade -dry-run = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd(t, upgradeCmd)
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, stepApply, stepUpgrade) ||
		!strings.Contains(stdout, "ok: the database can be upgraded to this release\n") ||
		!strings.Contains(stdout, "statement(s) applied\n") {
		t.Fatalf("upgrade = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd(t, upgradeCmd)
	if code != 0 || !stepsInOrder(stdout, stepConfig, stepCheck, stepApply, stepUpgrade) ||
		!strings.Contains(stdout, "ok: the database matches the schema; nothing to apply\n") ||
		!strings.Contains(stdout, "ok: no data upgrade step to run\n") {
		t.Errorf("upgrade again = %d %q %q", code, stdout, stderr)
	}
}
