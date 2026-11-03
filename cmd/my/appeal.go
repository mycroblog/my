// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// runAdminAppeal implements `my admin appeal <subcommand>`: the appeals of the actions on local
// accounts (FR-707). The CLI acts as no user: its decisions record no moderator.
func runAdminAppeal(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminAppealUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin appeal", args, []string{"list", "uphold", "overturn"}, stderr, adminAppealUsage)
	if code >= 0 {
		return code
	}
	var (
		state, before, reason string
		limit                 = 50
	)
	nargs := 1
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin appeal "+sub, args[1:], nargs, stderr, adminAppealUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "list":
			fs.StringVar(&state, "state", "", "show only the appeals in this state: pending, upheld, or overturned")
			fs.StringVar(&before, "before", "", "show the appeals older than this appeal ID")
			fs.IntVar(&limit, "limit", limit, "number of appeals to show, newest first")
		default:
			fs.StringVar(&reason, "reason", "", "the explanation of the decision shown to the user who appealed")
		}
	})
	if code >= 0 {
		return code
	}
	if limit < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: -limit must be at least 1\n", c.name)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	cmd := reportCommand{app: app, stdout: stdout}
	var err error
	switch sub {
	case "list":
		var vs []appmoderation.AppealView
		vs, err = app.Moderation.Appeals(ctx, appmoderation.AppealFilter{State: moderation.AppealState(state), Before: before}, limit)
		if err == nil {
			err = printAppeals(ctx, cmd, vs)
		}
	case "uphold", "overturn":
		decision := moderation.AppealDecisionUpheld
		if sub == "overturn" {
			decision = moderation.AppealDecisionOverturned
		}
		var v appmoderation.AppealView
		v, err = app.Moderation.DecideAppeal(ctx, c.fs.Arg(0), decision, reason, "")
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "appeal %s is %s; action %s is %s\n", v.Appeal.ID, v.State(), v.Action.Action.ID, v.Action.State)
		}
	}
	if errors.Is(err, appmoderation.ErrNotFound) {
		err = fmt.Errorf("no appeal has the ID %q", c.fs.Arg(0))
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

func printAppeals(ctx context.Context, c reportCommand, vs []appmoderation.AppealView) error {
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATE\tSUBMITTED\tACCOUNT\tACTION\tKIND\tACTION STATE\tTEXT")
	for _, v := range vs {
		a := v.Action.Action
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", v.Appeal.ID, v.State(), v.Appeal.SubmittedAt.Format(time.RFC3339),
			c.handle(ctx, v.Appeal.AccountID), a.ID, a.Kind, v.Action.State, oneLine(v.Appeal.Text))
	}
	return tw.Flush()
}

func adminAppealUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin appeal list [-state pending|upheld|overturned] [-before <id>] [-limit <n>] [-config <file>]
  my admin appeal uphold [-reason <text>] [-config <file>] <appeal id>
  my admin appeal overturn [-reason <text>] [-config <file>] <appeal id>

The appeals that local users made of the moderation actions on their accounts.
list shows them newest first. uphold keeps the action; overturn lifts
it at once (the posts that delete_statuses deleted stay deleted); overturning a
ban, appealed within the 30 days before the account's deletion, also cancels
that deletion, and fails once the account is deleted. -reason is
shown to the user, who is notified of the decision. Deciding a decided appeal
the same way changes nothing; deciding it the other way fails.
`)
}
