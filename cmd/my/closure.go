// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform/config"
)

// runAdminClosure implements `my admin closure <subcommand>`: the announcement of the closure of
// the service, its withdrawal, its state, the checks before the closure, and the closure (FR-808,
// FR-809, ADR-0088 §1), and the static closure page that the domain serves after the server
// stops (ADR-0088 §6). stdin answers the confirmations of close.
func runAdminClosure(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	sub, code := dispatch("admin closure", args, []string{"announce", "withdraw", "status", "check", "close", "export-html"}, stderr, adminClosureUsage)
	if code >= 0 || len(args) == 0 {
		return max(code, 0)
	}
	var closesAt, exportUntil, message, confirmDomain, out, langs string
	c, code := parseSubcommand("my admin closure "+sub, args[1:], 0, stderr, adminClosureUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "announce":
			fs.StringVar(&closesAt, "closes-at", "", "when the server closes, as RFC 3339 or a date (UTC midnight) (required)")
			fs.StringVar(&exportUntil, "export-until", "", "until when the users may export their data, not before -closes-at (default: 30 days after it)")
			fs.StringVar(&message, "message", "", "the operator's message to the users, in plain text")
		case "close":
			fs.StringVar(&confirmDomain, "confirm-domain", "", "the domain of the server, which confirms the closure without the questions (for automation)")
		case "export-html":
			fs.StringVar(&out, "out", "", "directory to write the pages and the configuration examples to (required)")
			fs.StringVar(&langs, "lang", "", "comma-separated locales, in the order of index.html (default: every locale, the instance's default locale first)")
		}
	})
	if code >= 0 {
		return code
	}
	if sub == "export-html" {
		return exportClosureHTML(c, out, langs, stdout, stderr)
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
	case "check":
		check, err := app.Instance.CheckClosure(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
			return 1
		}
		printClosureCheck(stdout, stderr, check)
		return 0
	case "close":
		return closeService(ctx, c, app.Instance, confirmDomain, stdin, stdout, stderr)
	default:
		closure, err = app.Instance.Closure(ctx)
	}
	if code := closureError(c.name, err, stderr); code >= 0 {
		return code
	}
	printClosure(stdout, closure)
	if sub == "status" {
		progress, ok, err := app.Instance.ClosureProgress(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
			return 1
		}
		if ok {
			printClosureProgress(stdout, progress)
		}
	}
	if closure.ShortNotice() && closure.CurrentState() == instance.ClosureAnnounced {
		_, _ = fmt.Fprintf(stderr, "warning: the announcement gives the users less than %d days before the closing date to export their data and move their accounts\n",
			int(instance.ClosureNoticeWarning/(24*time.Hour)))
	}
	return 0
}

// closureError prints a refused or failed change of the closure and returns its exit code, or -1
// for no error.
func closureError(name string, err error, stderr io.Writer) int {
	switch {
	case err == nil:
		return -1
	case errors.Is(err, instance.ErrClosureTransition):
		_, _ = fmt.Fprintf(stderr, "%s: the service closure in its current state does not allow that; nothing was changed (%v)\n", name, err)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
	}
	return 1
}

// closureService is what `my admin closure close` uses of the instance module.
type closureService interface {
	Closure(ctx context.Context) (instance.Closure, error)
	CheckClosure(ctx context.Context) (appinstance.ClosureCheck, error)
	CloseService(ctx context.Context, byUserID string) (instance.Closure, error)
}

// closeService closes the server after its checks and two confirmations (ADR-0088 §1): "y" to
// the question and the domain of the server typed on stdin, or -confirm-domain for automation.
// It refuses at once a closure that the state does not allow, before asking anything.
func closeService(ctx context.Context, c subcommand, svc closureService, confirmDomain string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, _, err := config.Load(config.Source{File: *c.file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	current, err := svc.Closure(ctx)
	if code := closureError(c.name, err, stderr); code >= 0 {
		return code
	}
	if code := closureError(c.name, current.Check(instance.ClosureEventClosed, time.Now()), stderr); code >= 0 {
		return code
	}
	check, err := svc.CheckClosure(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	printClosure(stdout, current)
	printClosureCheck(stdout, stderr, check)
	if confirmDomain == "" {
		in := bufio.NewReader(stdin)
		_, _ = fmt.Fprintf(stderr, "Close %s for good? This cannot be undone. [y/N] ", cfg.Host)
		if answer := readAnswer(in); answer != "y" && answer != "yes" {
			_, _ = fmt.Fprintf(stderr, "%s: not confirmed; nothing was changed\n", c.name)
			return 1
		}
		_, _ = fmt.Fprint(stderr, "Type the domain of the server to confirm: ")
		confirmDomain = readAnswer(in)
	}
	if !strings.EqualFold(strings.TrimSpace(confirmDomain), cfg.Host) {
		_, _ = fmt.Fprintf(stderr, "%s: the domain does not match %s; nothing was changed\n", c.name, cfg.Host)
		return 1
	}
	closure, err := svc.CloseService(ctx, "")
	if code := closureError(c.name, err, stderr); code >= 0 {
		return code
	}
	printClosure(stdout, closure)
	return 0
}

// readAnswer reads one line of an answer, trimmed and in lower case; "" at the end of the input.
func readAnswer(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(line))
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

// printClosureProgress prints how far the closure's Deletes have gone (ADR-0088 §3): the
// accounts whose Delete is recorded, the fan-outs left, and the deliveries by state.
func printClosureProgress(w io.Writer, p appinstance.ClosureProgress) {
	_, _ = fmt.Fprintf(w, "accounts with a Delete: %d\nfan-outs pending: %d\n", p.Accounts, p.FanOuts)
	for _, st := range []string{"pending", "succeeded", "permanent_failure", "skipped", "dead"} {
		_, _ = fmt.Fprintf(w, "deliveries %s: %d\n", st, p.Deliveries[st])
	}
}

// printClosureCheck prints the checks before the closure, and on stderr the warnings about the
// account migrations that have not ended (ADR-0089) and the retention holds (ADR-0038).
func printClosureCheck(w, stderr io.Writer, c appinstance.ClosureCheck) {
	_, _ = fmt.Fprintf(w, "local accounts: %d\ndelivery inboxes: %d\nestimated Delete deliveries: %d\nlive supporter subscriptions: %d\npending migrations: %d\nretention holds: %d\n",
		c.LocalAccounts, c.DeliveryInboxes, c.EstimatedDeletes, c.LiveSupporterSubscriptions, c.PendingMigrations, c.RetentionHolds)
	if c.PendingMigrations > 0 {
		_, _ = fmt.Fprintf(stderr, "warning: %d account migrations (Move) have not ended; let them end before closing, or their followers may not move\n", c.PendingMigrations)
	}
	if c.RetentionHolds > 0 {
		_, _ = fmt.Fprintf(stderr, "warning: %d accounts are under a retention hold; export their records (my admin account export-moderation) before discarding the data\n", c.RetentionHolds)
	}
}

func adminClosureUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin closure announce -closes-at <time> [-export-until <time>] [-message <text>] [-config <file>]
  my admin closure withdraw [-config <file>]
  my admin closure status [-config <file>]
  my admin closure check [-config <file>]
  my admin closure close [-confirm-domain <domain>] [-config <file>]
  my admin closure export-html -out <dir> [-lang en,ja] [-config <file>]

The closure of the service (FR-808, FR-809, ADR-0088, docs/operations/service-closure.md).

announce announces that the server closes at -closes-at, or changes the dates of
the announcement in effect. The users may export their data until -export-until
(30 days after the closing date by default; not before it). Times are RFC 3339
(2027-01-31T15:00:00Z) or dates (2027-01-31, midnight UTC). The closing date has
no lower bound, but an announcement that gives the users less than 30 days is
warned about on standard error. withdraw withdraws the announcement: the server
runs on as before. status prints the state and the schedule in effect, and once
the server is closed, the progress of the closure's Deletes: the accounts whose
Delete is recorded, the fan-outs left, and the deliveries by state. The
deliveries need the workers running; delivery_completed is recorded when they
have all ended.

check prints what the closure would end: the local accounts and the inboxes
that their Delete goes to, the supporter subscriptions that it cancels, the
account migrations that have not ended, and the accounts under a retention hold.

close closes the server for good once the closing date has passed: it cannot be
undone. It prints the checks, then asks "y" and the domain of the server on
standard input; -confirm-domain gives the domain instead, without the questions.
From then on, the users can only sign in and export their data.

export-html writes the static closure page, for the domain to serve once the
server is stopped after the export deadline: <dir>/<locale>.html for each locale
of -lang (every locale by default, the instance's default locale first) and
<dir>/index.html with every locale one after another, in the template of the
maintenance page (my maintenance export), with the server's name, the closing
date, and the operator's message. It also writes examples of the front end's
configuration (nginx.conf, Caddyfile, cloudfront-function.js) that answer 410
Gone to /ap/*, /inbox, /.well-known/webfinger, host-meta, NodeInfo, and /@*,
and the closure page to every other path. It needs the database, and the server
closed; before delivery_completed it warns that the Deletes are still being
delivered.

Every change is in the audit log as the CLI's. The server keeps running after
the closing date until it is closed.
`)
}
