// SPDX-License-Identifier: AGPL-3.0-only

// Package schemaapply is what the SQLite and PostgreSQL adapters share to apply the embedded
// desired-state schema with the Atlas library (`my schema apply`, ADR-0126): the diff of the
// inspected schemas, the plan, and the refusal of destructive changes. The dialect drivers
// stay in the adapters, so that the build tags no_sqlite and no_postgres leave them out.
//
// A change counts as destructive by its type (a dropped schema, table, or column), never by
// the SQL of the plan: SQLite rebuilds a table with DROP TABLE without losing any row.
package schemaapply

import (
	"context"
	"fmt"
	"strings"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/schema"
)

// Options are the flags of `my schema apply`.
type Options struct {
	// DryRun plans the changes and applies none.
	DryRun bool
	// AllowDestructive applies a plan that drops a schema, a table, or a column.
	AllowDestructive bool
}

// Result is what one run planned and did.
type Result struct {
	// Statements are the SQL statements of the plan, in order; none when the database matches
	// the schema.
	Statements []string
	// Drops name what the plan drops ("table t", "column t.c"), in the order of the changes.
	Drops []string
	// Applied reports whether the statements ran (and committed).
	Applied bool
}

// DestructiveError is the refusal of a plan that drops something without
// Options.AllowDestructive. Nothing ran.
type DestructiveError struct {
	Drops []string
}

func (e *DestructiveError) Error() string {
	return "the plan drops " + strings.Join(e.Drops, ", ")
}

// Drops returns what changes drop: schemas, tables, and the columns of modified tables.
func Drops(changes []schema.Change) []string {
	var out []string
	for _, c := range changes {
		switch c := c.(type) {
		case *schema.DropSchema:
			out = append(out, "schema "+c.S.Name)
		case *schema.DropTable:
			out = append(out, "table "+c.T.Name)
		case *schema.ModifyTable:
			for _, tc := range c.Changes {
				if d, ok := tc.(*schema.DropColumn); ok {
					out = append(out, "column "+c.T.Name+"."+d.C.Name)
				}
			}
		}
	}
	return out
}

// Run diffs current (the target database) with desired on drv, plans the changes, and, unless
// there are none, opts.DryRun is set, or the plan is destructive without
// opts.AllowDestructive, calls apply with them. apply runs the changes in one transaction.
func Run(ctx context.Context, drv migrate.Driver, current, desired *schema.Schema, opts Options, apply func([]schema.Change) error) (Result, error) {
	changes, err := drv.SchemaDiff(current, desired)
	if err != nil {
		return Result{}, fmt.Errorf("diff: %w", err)
	}
	if len(changes) == 0 {
		return Result{}, nil
	}
	plan, err := drv.PlanChanges(ctx, "my_schema_apply", changes)
	if err != nil {
		return Result{}, fmt.Errorf("plan: %w", err)
	}
	r := Result{Drops: Drops(changes)}
	for _, c := range plan.Changes {
		r.Statements = append(r.Statements, c.Cmd)
	}
	if opts.DryRun {
		return r, nil
	}
	if len(r.Drops) > 0 && !opts.AllowDestructive {
		return r, &DestructiveError{Drops: r.Drops}
	}
	if err := apply(changes); err != nil {
		return r, fmt.Errorf("apply: %w", err)
	}
	r.Applied = true
	return r, nil
}
