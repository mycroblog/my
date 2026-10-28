// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// runAdminDomain implements `my admin domain <subcommand>`: the moderation actions on domains
// (FR-702) and their list in Mastodon's CSV (FR-706, domain_block.go). The CLI acts as no user:
// the actions it issues and lifts record no moderator. stdin supplies the CSV of an import given
// as "-".
func runAdminDomain(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminDomainUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin domain", args, []string{"list", "issue", "lift", "export", "import"}, stderr, adminDomainUsage)
	if code >= 0 {
		return code
	}
	if sub == "export" || sub == "import" {
		return runAdminDomainCSV(sub, args[1:], stdin, stdout, stderr)
	}
	var (
		domain, kind, comment, note, before string
		active, obfuscate                   bool
		duration                            time.Duration
		limit                               = 50
	)
	nargs := 1
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin domain "+sub, args[1:], nargs, stderr, adminDomainUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "list":
			fs.StringVar(&domain, "domain", "", "show only the actions on this domain")
			fs.BoolVar(&active, "active", false, "show only the actions in effect now")
			fs.StringVar(&before, "before", "", "show the actions older than this action ID")
			fs.IntVar(&limit, "limit", limit, "number of actions to show, newest first")
		case "issue":
			fs.StringVar(&kind, "kind", "", "limit, suspend, reject_media, reject_reports, or force_sensitive (required)")
			fs.StringVar(&comment, "public-comment", "", "the reason that may be shown publicly")
			fs.StringVar(&note, "private-note", "", "a note for the moderators")
			fs.BoolVar(&obfuscate, "obfuscate", false, "hide part of the domain where domain actions are listed publicly")
			fs.DurationVar(&duration, "duration", 0, "how long the action lasts, such as 720h; 0 lasts until lifted")
		}
	})
	if code >= 0 {
		return code
	}
	if sub == "issue" && kind == "" {
		_, _ = fmt.Fprintf(stderr, "%s: -kind is required\n", c.name)
		return 2
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
	var err error
	switch sub {
	case "list":
		f := appmoderation.DomainActionFilter{Active: active, Before: before}
		if domain != "" {
			if f.Domain, err = appfederation.CanonicalHost(domain); err != nil {
				break
			}
		}
		var vs []appmoderation.DomainActionView
		if vs, err = app.Moderation.DomainActions(ctx, f, limit); err == nil {
			err = printDomainActions(stdout, vs)
		}
	case "issue":
		var host string
		if host, err = appfederation.CanonicalHost(c.fs.Arg(0)); err != nil {
			break
		}
		var a moderation.DomainAction
		a, err = app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{
			Domain: host, Kind: moderation.DomainActionKind(kind), PublicComment: comment, PrivateNote: note,
			Obfuscate: obfuscate, Duration: duration,
		})
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "issued %s on %s and its subdomains (action %s, %s)\n", a.Kind, a.Domain, a.ID, describeExpiry(a.ExpiresAt))
		}
	case "lift":
		id := c.fs.Arg(0)
		err = app.Moderation.LiftDomainAction(ctx, id, "")
		if errors.Is(err, appmoderation.ErrNotFound) {
			err = fmt.Errorf("no domain action has the ID %q", id)
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "lifted domain action %s\n", id)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

func printDomainActions(w io.Writer, vs []appmoderation.DomainActionView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tDOMAIN\tKIND\tSTATE\tISSUED\tEXPIRES\tBY\tCOMMENT")
	for _, v := range vs {
		a := v.Action
		expires, by := "-", a.IssuedByUserID
		if a.ExpiresAt != nil {
			expires = a.ExpiresAt.Format(time.RFC3339)
		}
		if by == "" {
			by = "cli"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Domain, a.Kind, v.State, a.IssuedAt.Format(time.RFC3339), expires, by, oneLine(a.PublicComment))
	}
	return tw.Flush()
}

// oneLine returns s on one line and cut to 60 characters, for a table cell.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 60 {
		return string(r[:59]) + "…"
	}
	return s
}

func adminDomainUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin domain list [-domain <domain>] [-active] [-before <id>] [-limit <n>] [-config <file>]
  my admin domain issue -kind <kind> [-public-comment <text>] [-private-note <text>]
                        [-obfuscate] [-duration <d>] [-config <file>] <domain>
  my admin domain lift [-config <file>] <action id>
  my admin domain export [-config <file>]
  my admin domain import [-apply] [-only <domains>] [-private-note <text>]
                         [-config <file>] <file | ->

Moderation actions on remote domains and their subdomains (FR-702), in effect at
once for federation and the timelines:

  suspend          no deliveries to the domain, no fetches from it, and no
                   activities accepted from it; its accounts and posts are hidden
  limit            its posts stay off the public timelines and its accounts reach
                   only their followers
  reject_media     its media are not fetched
  reject_reports   the reports it sends are dropped
  force_sensitive  its media are marked sensitive

The domain may be given in U-labels (例え.jp); it is stored in IDNA A-labels.
list shows the actions newest first, lifted and expired ones included unless
-active is given. lift ends one action; the other actions on the domain stay.
Lifting a lifted action changes nothing.

export writes the actions in effect to stdout as Mastodon's domain block CSV
(#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate),
one row per domain: severity suspend for a suspend, silence for a limit, noop
otherwise. Domains with only force_sensitive, which Mastodon cannot hold, are
left out.

import reads a CSV that Mastodon exported ("-" reads stdin; a file without the
#domain header is a list of domains to suspend; at most 5000 rows) and prints
what each row would issue: suspend for suspend, limit for silence, nothing for
noop, plus reject_media and reject_reports. Rows whose actions are already in
effect on the domain or a parent domain, duplicates, and invalid rows issue
nothing. Nothing changes without -apply; -only keeps the rows of the listed
domains. Imported actions last until lifted and are audited like any other.
`)
}

// runAdminReport implements `my admin report <subcommand>`: the reports in the moderators'
// queue (FR-701). The CLI acts as no user: what it records names no moderator.
func runAdminReport(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminReportUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin report", args, []string{"list", "show", "assign", "unassign", "resolve", "note", "reopen"}, stderr, adminReportUsage)
	if code >= 0 {
		return code
	}
	var (
		state, before, outcome string
		limit                  = 50
	)
	nargs := 1
	switch sub {
	case "list":
		nargs = 0
	case "assign", "note":
		nargs = 2
	}
	c, code := parseSubcommand("my admin report "+sub, args[1:], nargs, stderr, adminReportUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "list":
			fs.StringVar(&state, "state", "", "show only the reports in this state: open or resolved")
			fs.StringVar(&before, "before", "", "show the reports older than this report ID")
			fs.IntVar(&limit, "limit", limit, "number of reports to show, newest first")
		case "resolve":
			fs.StringVar(&outcome, "outcome", "", "actioned or dismissed (required)")
		}
	})
	if code >= 0 {
		return code
	}
	if sub == "resolve" && outcome == "" {
		_, _ = fmt.Fprintf(stderr, "%s: -outcome is required\n", c.name)
		return 2
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
		err = cmd.list(ctx, appmoderation.ReportQuery{State: moderation.ReportState(state), After: before}, limit)
	case "show":
		err = cmd.show(ctx, c.fs.Arg(0))
	case "assign":
		err = cmd.assign(ctx, c.fs.Arg(0), c.fs.Arg(1))
	case "unassign":
		err = cmd.assign(ctx, c.fs.Arg(0), "")
	case "resolve":
		var rec moderation.ReportRecord
		rec, err = app.Moderation.ResolveReport(ctx, c.fs.Arg(0), moderation.ReportOutcome(outcome), "")
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "report %s is resolved (%s)\n", rec.Report.ID, rec.Status.Outcome)
		}
	case "note":
		var n moderation.ReportNote
		n, err = app.Moderation.AddReportNote(ctx, c.fs.Arg(0), c.fs.Arg(1), "")
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "note %s is added to report %s\n", n.ID, n.ReportID)
		}
	case "reopen":
		var rec moderation.ReportRecord
		rec, err = app.Moderation.ReopenReport(ctx, c.fs.Arg(0), "")
		if errors.Is(err, appmoderation.ErrReportNotResolved) {
			err = fmt.Errorf("report %s is open; only a resolved report can be reopened", c.fs.Arg(0))
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "report %s is open again\n", rec.Report.ID)
		}
	}
	if errors.Is(err, appmoderation.ErrNotFound) {
		err = fmt.Errorf("no report has the ID %q", c.fs.Arg(0))
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

type reportCommand struct {
	app    *platform.App
	stdout io.Writer
}

// handle returns the handle of an account, or its ID when it is gone.
func (c reportCommand) handle(ctx context.Context, id string) string {
	a, err := c.app.Accounts.FindByID(ctx, id)
	switch {
	case err != nil:
		return id
	case a.Local != nil:
		return "@" + a.Local.Username
	case a.Handle != nil:
		return "@" + a.Handle.Username + "@" + a.Handle.Domain
	}
	return a.Account.URI
}

// moderator returns the username of a user, or "-" for none.
func (c reportCommand) moderator(ctx context.Context, userID string) string {
	if userID == "" {
		return "-"
	}
	la, err := c.app.Accounts.LocalAccountOfUser(ctx, userID)
	if err != nil {
		return userID
	}
	return "@" + la.Username
}

func describeReportState(st moderation.ReportStatus) string {
	if st.State == moderation.ReportResolved {
		return fmt.Sprintf("resolved (%s)", st.Outcome)
	}
	return string(st.State)
}

func (c reportCommand) list(ctx context.Context, q appmoderation.ReportQuery, limit int) error {
	recs, err := c.app.Moderation.Reports(ctx, q, limit)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATE\tFILED\tTARGET\tREPORTER\tASSIGNEE\tSTATUSES\tCOMMENT")
	for _, rec := range recs {
		r := rec.Report
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", r.ID, describeReportState(rec.Status), r.FiledAt.Format(time.RFC3339),
			c.handle(ctx, r.TargetAccountID), c.handle(ctx, r.ReporterAccountID), c.moderator(ctx, rec.Status.AssigneeUserID),
			len(r.StatusIDs), oneLine(r.Comment))
	}
	return tw.Flush()
}

func (c reportCommand) show(ctx context.Context, id string) error {
	d, err := c.app.Moderation.ReportDetail(ctx, id)
	if err != nil {
		return err
	}
	r, st := d.Report, d.Status
	uri := r.URI
	if uri == "" {
		uri = "-"
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	for _, row := range [][2]string{
		{"id", r.ID},
		{"state", describeReportState(st)},
		{"since", st.Since.Format(time.RFC3339)},
		{"assignee", c.moderator(ctx, st.AssigneeUserID)},
		{"filed", r.FiledAt.Format(time.RFC3339)},
		{"uri", uri},
		{"category", string(r.Category)},
		{"target", c.handle(ctx, r.TargetAccountID) + " (" + r.TargetAccountID + ")"},
		{"reporter", c.handle(ctx, r.ReporterAccountID) + " (" + r.ReporterAccountID + ")"},
		{"statuses", strings.Join(r.StatusIDs, " ")},
		{"history", fmt.Sprintf("%d other reports, %d actions", d.History.Reports, d.History.Actions)},
	} {
		_, _ = fmt.Fprintf(tw, "%s:\t%s\n", row[0], row[1])
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.stdout, "\n%s\n", r.Comment); err != nil {
		return err
	}
	for _, n := range d.Notes {
		_, err := fmt.Fprintf(c.stdout, "\nnote %s by %s at %s:\n%s\n", n.ID, c.moderator(ctx, n.AuthorUserID), n.WrittenAt.Format(time.RFC3339), n.Body)
		if err != nil {
			return err
		}
	}
	return nil
}

// assign makes the local user with username handle the report, or nobody for "". The user's
// roles must grant manage_reports.
func (c reportCommand) assign(ctx context.Context, id, username string) error {
	userID := ""
	if username != "" {
		a, err := c.app.Accounts.FindByHandle(ctx, strings.TrimPrefix(username, "@"), "")
		if errors.Is(err, appaccount.ErrNotFound) || (err == nil && a.Local == nil) {
			return fmt.Errorf("no local account has the username %q", username)
		}
		if err != nil {
			return err
		}
		ok, err := c.app.Identity.HasPermission(ctx, a.Local.UserID, identity.PermManageReports)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("the roles of @%s do not grant %s", a.Local.Username, identity.PermManageReports)
		}
		userID = a.Local.UserID
	}
	rec, err := c.app.Moderation.AssignReport(ctx, id, userID, "")
	if err != nil {
		return err
	}
	if userID == "" {
		_, err = fmt.Fprintf(c.stdout, "report %s is assigned to nobody\n", rec.Report.ID)
	} else {
		_, err = fmt.Fprintf(c.stdout, "report %s is assigned to %s\n", rec.Report.ID, c.moderator(ctx, userID))
	}
	return err
}

func adminReportUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin report list [-state open|resolved] [-before <id>] [-limit <n>] [-config <file>]
  my admin report show [-config <file>] <report id>
  my admin report assign [-config <file>] <report id> <username>
  my admin report unassign [-config <file>] <report id>
  my admin report resolve -outcome actioned|dismissed [-config <file>] <report id>
  my admin report note [-config <file>] <report id> <text>
  my admin report reopen [-config <file>] <report id>

The reports in the moderators' queue (FR-701), including those received from
other servers. list shows them newest first; show adds the moderators' notes
and the number of the target's other reports and actions. assign makes a local
user whose roles grant manage_reports handle a report; unassign leaves it to
nobody. resolve closes an open report with its outcome; resolving a resolved
report changes nothing. note adds an internal note (up to 2,000 characters)
that only the moderators see. reopen puts a resolved report back in the queue.
`)
}
