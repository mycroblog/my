// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// runUpgrade implements `my upgrade`: the migration steps of an upgrade to this release, run in
// one process and in order, so that one one-off job carries the whole migration. Each step is
// the existing command run with the same arguments, so its behaviour and output stay those of
// the command.
func runUpgrade(args []string, stdout, stderr io.Writer) int {
	var dryRun, allowDestructive bool
	c, code := parseSubcommand("my upgrade", args, 0, stderr, upgradeUsage, func(fs *flag.FlagSet) {
		fs.BoolVar(&dryRun, "dry-run", false, "run the checks and print the SQL of the schema's plan, and change nothing")
		fs.BoolVar(&allowDestructive, "allow-destructive", false, "apply a plan that drops a table or a column")
	})
	if code >= 0 {
		return code
	}
	cfg := []string{"-config", *c.file}
	apply := []string{"apply"}
	if dryRun {
		apply = append(apply, "-dry-run")
	}
	if allowDestructive {
		apply = append(apply, "-allow-destructive")
	}
	steps := []upgradeStep{
		{"config", runConfig, []string{"check"}},
		{"db", runDB, []string{"upgrade", "-check"}},
		{"schema", runSchema, apply},
	}
	if !dryRun {
		steps = append(steps, upgradeStep{"db", runDB, []string{"upgrade"}})
	}
	for _, s := range steps {
		name := "my " + s.command + " " + strings.Join(s.args, " ")
		_, _ = fmt.Fprintf(stdout, "==> %s\n", name)
		if code := s.run(slices.Concat(s.args, cfg), stdout, stderr); code != 0 {
			_, _ = fmt.Fprintf(stderr, "my upgrade: stopped: %s failed, and the steps after it did not run\n", name)
			return code
		}
	}
	if dryRun {
		_, _ = fmt.Fprintln(stdout, "ok: the upgrade was checked and its plan printed (dry run); nothing was changed")
		return 0
	}
	_, _ = fmt.Fprintln(stdout, "ok: the database is upgraded to this release")
	return 0
}

// upgradeStep is one step of my upgrade: the command my <command> run with args and -config.
type upgradeStep struct {
	command string
	run     func(args []string, stdout, stderr io.Writer) int
	args    []string
}

func upgradeUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: my upgrade [-dry-run] [-allow-destructive] [-config <file>]

Runs the migration of an upgrade to this release in one process, in this
order, and stops at the first step that fails, exiting with its code:

  1. my config check
  2. my db upgrade -check
  3. my schema apply, with -allow-destructive when it is given
  4. my db upgrade

Each step prints a line that names it, such as "==> my db upgrade -check",
before it starts, and then the output of that command, which ends with
"configuration OK" or an "ok: ..." line when the step succeeds. The steps after
a failed one do not run, so the output ends where the upgrade stopped. Back up
the database before an upgrade, and start the new release's server once this
exits with 0. "my serve" never runs it, and only one may run against a database
at a time. A second run after a successful one finds nothing to apply and no
data upgrade step to run.

With -dry-run it runs steps 1 and 2 and "my schema apply -dry-run", which prints
the SQL of the plan, and changes nothing.

Flags:
  -dry-run            run the checks and print the SQL of the plan, and change nothing
  -allow-destructive  apply a plan that drops a table or a column
  -config <file>      TOML config file (default: $MY_CONFIG)
`+docsLine("docs/operations/database.md"))
}
