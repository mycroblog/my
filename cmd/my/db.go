// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// dbAppOptions are the options of the App that `my db` builds. Tests set DataUpgrades.
var dbAppOptions platform.AppOptions

// runDB implements `my db <subcommand>`.
func runDB(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("db", args, []string{"upgrade", "backup"}, stderr, dbUsage)
	if code >= 0 {
		return code
	}
	name := "my db " + sub
	if sub == "backup" {
		return dbBackup(name, args[1:], stdout, stderr)
	}
	var check *bool
	c, code := parseSubcommand(name, args[1:], 0, stderr, dbUsage, func(fs *flag.FlagSet) {
		check = fs.Bool("check", false, "check that the database ran the data upgrade steps that this release's schema requires, and change nothing")
	})
	if code >= 0 {
		return code
	}
	ctx := context.Background()
	cfg, _, err := config.Load(config.Source{File: *c.file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	app, err := platform.NewApp(ctx, cfg, dbAppOptions)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	defer func() { _ = app.Close() }()
	if *check {
		return dbUpgradeCheck(ctx, app.DataUpgrades, name+" -check", stdout, stderr)
	}
	return dbUpgrade(ctx, app.DataUpgrades, name, stdout, stderr)
}

// dbUpgradeCheck implements `my db upgrade -check`: it exits with 1 when the database lacks a
// prerequisite, naming the releases to upgrade through first.
func dbUpgradeCheck(ctx context.Context, svc *dbupgrade.Service, name string, stdout, stderr io.Writer) int {
	r, err := svc.Check(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	switch {
	case r.NewDatabase:
		_, _ = fmt.Fprintln(stdout, "ok: the database is new; there is no data to carry over")
		return 0
	case len(r.Missing) == 0:
		_, _ = fmt.Fprintln(stdout, "ok: the database can be upgraded to this release")
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "%s: the database has not run data upgrade steps that this release requires:\n", name)
	var vias []string
	for _, p := range r.Missing {
		_, _ = fmt.Fprintf(stderr, "  %s (run by my db upgrade of %s)\n", p.Key, p.Via)
		if !slices.Contains(vias, p.Via) {
			vias = append(vias, p.Via)
		}
	}
	for _, via := range vias {
		_, _ = fmt.Fprintf(stderr, "upgrade through %s first: apply its schema and run its my db upgrade; do not apply this release's schema\n", via)
	}
	return 1
}

// dbUpgrade implements `my db upgrade`: it runs the pending steps and exits with 1 at the first
// that fails.
func dbUpgrade(ctx context.Context, svc *dbupgrade.Service, name string, stdout, stderr io.Writer) int {
	done, err := svc.Upgrade(ctx, func(s dbupgrade.Step) {
		_, _ = fmt.Fprintf(stdout, "%s: running: %s\n", s.Key, s.Description)
	})
	for _, s := range done {
		_, _ = fmt.Fprintf(stdout, "%s: done\n", s.Key)
	}
	switch {
	case errors.Is(err, dbupgrade.ErrNoRecord), errors.Is(err, dbupgrade.ErrEmptyDatabase):
		_, _ = fmt.Fprintf(stderr, "%s: the schema of this release is not applied; apply it first\n", name)
		return 1
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "%s: %v\n%s: the steps that completed are recorded; run my db upgrade again to resume\n", name, err, name)
		return 1
	case len(done) == 0:
		_, _ = fmt.Fprintln(stdout, "ok: no data upgrade step to run")
	default:
		_, _ = fmt.Fprintf(stdout, "ok: %d data upgrade step(s) run\n", len(done))
	}
	return 0
}

// dbBackup implements `my db backup <file>`: a consistent copy of the SQLite database, taken
// while the server keeps running.
func dbBackup(name string, args []string, stdout, stderr io.Writer) int {
	c, code := parseSubcommand(name, args, 1, stderr, dbUsage, nil)
	if code >= 0 {
		return code
	}
	cfg, _, err := config.Load(config.Source{File: *c.file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	if cfg.DB != "sqlite" {
		_, _ = fmt.Fprintf(stderr, "%s: db is %s; it backs up only a SQLite database (back up PostgreSQL with pg_dump)\n", name, cfg.DB)
		return 1
	}
	// sqlite.Open would create a missing file; a backup of an empty database is a wrong path.
	file, _, _ := strings.Cut(strings.TrimPrefix(cfg.DatabaseURL, "file:"), "?")
	if _, err := os.Stat(file); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: the database: %v\n", name, err)
		return 1
	}
	ctx := context.Background()
	db, err := sqlite.Open(ctx, cfg.DatabaseURL, uow.Options{})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	defer func() { _ = db.Close() }()
	path := c.fs.Arg(0)
	if err := db.BackupTo(ctx, path); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "ok: the database is copied to %s\n", path)
	return 0
}

func dbUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my db upgrade [-config <file>]
  my db upgrade -check [-config <file>]
  my db backup [-config <file>] <file>

Upgrading to a new release runs, in this order: back up the database, run the
new release's "my db upgrade -check", apply the new release's schema, run the
new release's "my db upgrade", then start the new release's server. Both
commands are one-off jobs; "my serve" never runs them, and only one of them may
run against a database at a time. Each release supports upgrades from the same
minor version and from the latest patch release of the previous minor version.

my db upgrade -check runs before the schema is applied and changes nothing. It
exits with 0 when the database can take this release's schema, and with 1 when
the database has not run data upgrade steps that this release's schema relies
on: the schema would drop data that those steps carry over. It then lists the
missing steps and the release whose "my db upgrade" runs each; upgrade through
that release first. A database without any table is new and passes.

my db upgrade runs after the schema is applied. It runs, in order, each data
upgrade step of this release that the database has not recorded, rebuilds the
projections that the step names, and records the step. A second run does
nothing. It exits with 0 when every step has run, and with 1 at the first step
that fails; the steps that completed stay recorded, and running it again
resumes from the failed step. The server reports not ready (503 on /readyz)
until every step of its release is recorded.

my db backup copies a SQLite database (db = sqlite) to the new <file>, which
must not exist, in one consistent snapshot: the server may keep running and
writing meanwhile, and the copy holds every write committed before the backup
started. The copy is one file; restore it by putting it in place of the
database file, with the server stopped and the old -wal and -shm files
removed. Back up PostgreSQL with pg_dump instead.
`)
}
