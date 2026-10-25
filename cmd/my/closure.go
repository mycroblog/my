// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// runAdminClosure implements `my admin closure <subcommand>`: the announcement of the closure of
// the service, its withdrawal, and its state (FR-808, ADR-0088 §1).
func runAdminClosure(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("admin closure", args, []string{"announce", "withdraw", "status"}, stderr, adminClosureUsage)
	if code >= 0 || len(args) == 0 {
		return max(code, 0)
	}
	var closesAt, exportUntil, message string
	c, code := parseSubcommand("my admin closure "+sub, args[1:], 0, stderr, adminClosureUsage, func(fs *flag.FlagSet) {
		if sub != "announce" {
			return
		}
		fs.StringVar(&closesAt, "closes-at", "", "when the server closes, as RFC 3339 or a date (UTC midnight) (required)")
		fs.StringVar(&exportUntil, "export-until", "", "until when the users may export their data, not before -closes-at (default: 30 days after it)")
		fs.StringVar(&message, "message", "", "the operator's message to the users, in plain text")
	})
	if code >= 0 {
		return code
	}
	var in appinstance.AnnounceClosureInput
	if sub == "announce" {
		var err error
		if in.ClosesAt, err = parseClosureTime(closesAt); err != nil || closesAt == "" {
			_, _ = fmt.Fprintf(stderr, "%s: -closes-at must be an RFC 3339 time or a date (2006-01-02)\n", c.name)
			return 2
		}
		if exportUntil != "" {
			if in.ExportUntil, err = parseClosureTime(exportUntil); err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: -export-until must be an RFC 3339 time or a date (2006-01-02)\n", c.name)
				return 2
			}
		}
		in.Message = message
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var closure instance.Closure
	var err error
	switch sub {
	case "announce":
		closure, err = app.Instance.AnnounceClosure(ctx, in)
	case "withdraw":
		closure, err = app.Instance.WithdrawClosure(ctx, "")
	default:
		closure, err = app.Instance.Closure(ctx)
	}
	switch {
	case errors.Is(err, instance.ErrClosureTransition):
		_, _ = fmt.Fprintf(stderr, "%s: the service closure in its current state does not allow that; nothing was changed (%v)\n", c.name, err)
		return 1
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	printClosure(stdout, closure)
	if closure.ShortNotice() && closure.CurrentState() == instance.ClosureAnnounced {
		_, _ = fmt.Fprintf(stderr, "warning: the announcement gives the users less than %d days before the closing date to export their data and move their accounts\n",
			int(instance.ClosureNoticeWarning/(24*time.Hour)))
	}
	return 0
}

// parseClosureTime reads an RFC 3339 time, or a date as its midnight in UTC.
func parseClosureTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.DateOnly, s)
}

func printClosure(w io.Writer, c instance.Closure) {
	_, _ = fmt.Fprintf(w, "state: %s\n", c.CurrentState())
	if c.CurrentState() == instance.ClosureNone {
		return
	}
	_, _ = fmt.Fprintf(w, "closes at: %s\nexport until: %s\nannounced at: %s\n",
		c.Schedule.ClosesAt.UTC().Format(time.RFC3339), c.Schedule.ExportUntil.UTC().Format(time.RFC3339), c.AnnouncedAt.UTC().Format(time.RFC3339))
	if c.Schedule.Message != "" {
		_, _ = fmt.Fprintf(w, "message: %s\n", c.Schedule.Message)
	}
}

func adminClosureUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin closure announce -closes-at <time> [-export-until <time>] [-message <text>] [-config <file>]
  my admin closure withdraw [-config <file>]
  my admin closure status [-config <file>]

The closure of the service (FR-808, ADR-0088, docs/operations/admin-cli.md).

announce announces that the server closes at -closes-at, or changes the dates of
the announcement in effect. The users may export their data until -export-until
(30 days after the closing date by default; not before it). Times are RFC 3339
(2027-01-31T15:00:00Z) or dates (2027-01-31, midnight UTC). The closing date has
no lower bound, but an announcement that gives the users less than 30 days is
warned about on standard error. withdraw withdraws the announcement: the server
runs on as before. status prints the state and the schedule in effect.

Every announcement, change of the dates, and withdrawal is in the audit log as
the CLI's. The server keeps running after the closing date until it is closed.
`)
}
