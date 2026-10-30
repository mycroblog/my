// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/internal/platform/config"
)

// runSchema implements `my schema <subcommand>`.
func runSchema(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("schema", args, []string{"apply"}, stderr, schemaUsage)
	if code >= 0 {
		return code
	}
	name := "my schema " + sub
	var opts schemaapply.Options
	c, code := parseSubcommand(name, args[1:], 0, stderr, schemaUsage, func(fs *flag.FlagSet) {
		fs.BoolVar(&opts.DryRun, "dry-run", false, "print the SQL of the plan and change nothing")
		fs.BoolVar(&opts.AllowDestructive, "allow-destructive", false, "apply a plan that drops a table or a column")
	})
	if code >= 0 {
		return code
	}
	cfg, _, err := config.LoadSchemaApply(config.Source{File: *c.file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	return schemaApply(context.Background(), cfg, opts, name, stdout, stderr)
}

// schemaApply applies the schema of the release to the database of cfg and reports the
// result.
func schemaApply(ctx context.Context, cfg *config.Config, opts schemaapply.Options, name string, stdout, stderr io.Writer) int {
	var (
		r   schemaapply.Result
		err error
	)
	switch cfg.DB {
	case "sqlite":
		r, err = applySQLiteSchema(ctx, cfg.DatabaseURL, opts)
	case "postgres":
		r, err = applyPostgresSchema(ctx, cfg.DatabaseURL, cfg.DatabaseDevURL, opts)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: db is %s; it applies the schema of db = sqlite and db = postgres only\n", name, cfg.DB)
		return 1
	}
	var destructive *schemaapply.DestructiveError
	switch {
	case errors.As(err, &destructive):
		printPlan(stdout, r)
		_, _ = fmt.Fprintf(stderr, "%s: the plan drops %s, which loses their data; nothing was applied.\n"+
			"Back up the database and run it again with -allow-destructive only when the release notes call for it.\n",
			name, strings.Join(destructive.Drops, ", "))
		return 1
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "%s: %v\n%s: nothing was applied\n", name, err, name)
		return 1
	case len(r.Statements) == 0:
		_, _ = fmt.Fprintln(stdout, "ok: the database matches the schema; nothing to apply")
		return 0
	}
	printPlan(stdout, r)
	switch {
	case !r.Applied && len(r.Drops) > 0:
		_, _ = fmt.Fprintf(stdout, "ok: %d statement(s) planned and none applied (dry run); the plan drops %s and needs -allow-destructive\n",
			len(r.Statements), strings.Join(r.Drops, ", "))
	case !r.Applied:
		_, _ = fmt.Fprintf(stdout, "ok: %d statement(s) planned and none applied (dry run)\n", len(r.Statements))
	default:
		_, _ = fmt.Fprintf(stdout, "ok: %d statement(s) applied\n", len(r.Statements))
	}
	return 0
}

// printPlan prints the statements of the plan, each ending with a semicolon.
func printPlan(w io.Writer, r schemaapply.Result) {
	for _, s := range r.Statements {
		_, _ = fmt.Fprintf(w, "%s;\n", s)
	}
}

func schemaUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my schema apply [-dry-run] [-allow-destructive] [-config <file>]

my schema apply brings the database of db and database_url (the server's own
settings; it reads only those and database_dev_url) to the schema of this
release, which the binary holds. It compares
the database with the schema, prints the SQL statements of the difference, and
runs them in one transaction: when one fails, none of them stays. A database
that already matches the schema is left unchanged, so running it again is safe.
Apply the schema before starting a new release's server; "my serve" never
applies it. db = sqlite and db = postgres are supported.

On PostgreSQL it first runs the bootstrap SQL (CREATE EXTENSION IF NOT EXISTS
pg_trgm), so the role needs the privilege to create that extension or the
extension must exist. It builds the schema to compare with in a temporary
schema of the database, which it drops at the end, also on failure; when the
role may not create a schema there, set database_dev_url to another database
where it may. Runs against one database take turns: a second run waits for the
first and then finds nothing to apply.

A plan that drops a table or a column loses the data in it, so my schema apply
refuses it and changes nothing unless -allow-destructive is given. Rebuilding a
SQLite table to change it does not count as a drop.

Flags:
  -dry-run            print the SQL of the plan and change nothing
  -allow-destructive  apply a plan that drops a table or a column
  -config <file>      TOML config file (default: $MY_CONFIG)

It exits with 0 when the database matches the schema afterwards (or, with
-dry-run, when the plan was printed), and with 1 when it applied nothing
because of an error or a refused drop.
`)
}
