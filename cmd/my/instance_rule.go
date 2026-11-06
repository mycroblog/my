// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// runInstanceRule implements `my instance rule <subcommand>` (FR-705).
func runInstanceRule(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		instanceRuleUsage(stderr)
		return 2
	}
	sub := args[0]
	var nargs int
	switch sub {
	case "-h", "-help", "--help":
		instanceRuleUsage(stderr)
		return 0
	case "list":
		nargs = 0
	case "create", "retire":
		nargs = 1
	case "edit", "move":
		nargs = 2
	default:
		_, _ = fmt.Fprintf(stderr, "my instance rule: unknown subcommand %q\n\n", sub)
		instanceRuleUsage(stderr)
		return 2
	}
	name := "my instance rule " + sub
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { instanceRuleUsage(stderr) }
	file := fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")
	var hint string
	all := false
	switch sub {
	case "list":
		fs.BoolVar(&all, "all", false, "also list the retired rules")
	case "create", "edit":
		fs.StringVar(&hint, "hint", "", "explanation of the rule (edit keeps the current one unless given)")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != nargs {
		_, _ = fmt.Fprintf(stderr, "%s: want %d argument(s), got %d\n\n", name, nargs, fs.NArg())
		instanceRuleUsage(stderr)
		return 2
	}
	hintSet := false
	fs.Visit(func(f *flag.Flag) { hintSet = hintSet || f.Name == "hint" })
	pos := fs.Args()
	position := 0
	if sub == "move" {
		var err error
		if position, err = strconv.Atoi(pos[1]); err != nil || position < 1 {
			_, _ = fmt.Fprintf(stderr, "%s: position %q is not a number from 1\n", name, pos[1])
			return 2
		}
	}

	cfg, _, err := config.Load(config.Source{File: *file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	ctx := context.Background()
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	defer func() { _ = app.Close() }()
	cmd := instanceCommand{svc: app.Instance, app: app, stdout: stdout}
	var rule instance.Rule
	switch sub {
	case "list":
		err = cmd.listRules(ctx, all)
	case "create":
		rule, err = cmd.svc.AddRule(ctx, appinstance.AddRuleInput{Text: pos[0], Hint: hint})
	case "edit":
		rule, err = cmd.svc.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: pos[0], Text: pos[1], Hint: hint, KeepHint: !hintSet})
	case "move":
		rule, err = cmd.svc.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: pos[0], Position: position})
	case "retire":
		rule, err = cmd.svc.RetireRule(ctx, pos[0], "")
	}
	if err == nil && sub != "list" {
		err = cmd.printRule(sub, rule)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	return 0
}

func (c instanceCommand) printRule(sub string, r instance.Rule) error {
	verb := map[string]string{"create": "created", "edit": "revised", "move": "moved", "retire": "retired"}[sub]
	_, err := fmt.Fprintf(c.stdout, "%s rule %s at position %d (revision %s)\n", verb, r.ID, r.Revision.Position, r.Revision.ID)
	return err
}

func (c instanceCommand) listRules(ctx context.Context, all bool) error {
	rules, err := c.svc.Rules(ctx, all)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RULE\tPOSITION\tSTATE\tTEXT\tHINT")
	for _, r := range rules {
		state := "in effect"
		if r.Retired() {
			state = "retired"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", r.ID, r.Revision.Position, state, preview(r.Revision.Text), preview(r.Revision.Hint))
	}
	return tw.Flush()
}

func instanceRuleUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my instance rule list [-all] [-config <file>]
  my instance rule create [-hint <text>] [-config <file>] <text>
  my instance rule edit [-hint <text>] [-config <file>] <rule> <text>
  my instance rule move [-config <file>] <rule> <position>
  my instance rule retire [-config <file>] <rule>

Manages the instance rules, which GET /api/v1/instance lists and reports cite.
A rule has one line of text and an optional hint, written in any language and
shown as they are in every locale.
Every change is kept as a revision, and a rule keeps its ID for life.

create appends a rule after the rules in effect. edit changes the text of a rule;
the hint stays unless -hint is given. move puts a rule at a position among the
rules in effect, from 1, and numbers them again. retire removes a rule from the
rules in effect for good; reports that cite it still refer to it. list shows
the rules in effect, and with -all the retired ones after them.
`+docsLine("docs/operations/instance-settings.md"))
}
