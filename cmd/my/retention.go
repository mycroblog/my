// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// runAdminRetention implements `my admin retention <subcommand>`: the holds that stop the erasure
// of an account's records after their retention (ADR-0038). The CLI acts as no user: the holds
// it places and releases record no operator.
func runAdminRetention(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminRetentionUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin retention", args, []string{"hold", "release", "list"}, stderr, adminRetentionUsage)
	if code >= 0 {
		return code
	}
	var reason, account string
	nargs := 1
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin retention "+sub, args[1:], nargs, stderr, adminRetentionUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "hold":
			fs.StringVar(&reason, "reason", "", "why the records are preserved, such as a court order (required)")
		case "list":
			fs.StringVar(&account, "account", "", "show only the holds on this account")
		}
	})
	if code >= 0 {
		return code
	}
	if sub == "hold" && reason == "" {
		_, _ = fmt.Fprintf(stderr, "%s: -reason is required\n", c.name)
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
	case "hold":
		var id string
		if id, err = resolveAccount(ctx, app, c.fs.Arg(0)); err != nil {
			break
		}
		var h moderation.RetentionHold
		if h, err = app.Moderation.PlaceRetentionHold(ctx, id, reason, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "placed retention hold %s on account %s\n", h.ID, h.AccountID)
		}
	case "release":
		id := c.fs.Arg(0)
		var released bool
		_, released, err = app.Moderation.ReleaseRetentionHold(ctx, id, "")
		switch {
		case errors.Is(err, appmoderation.ErrNotFound):
			err = fmt.Errorf("no retention hold has the ID %q", id)
		case err == nil && released:
			_, _ = fmt.Fprintf(stdout, "released retention hold %s\n", id)
		case err == nil:
			_, _ = fmt.Fprintf(stdout, "retention hold %s was already released\n", id)
		}
	case "list":
		accountID := ""
		if account != "" {
			if accountID, err = resolveAccount(ctx, app, account); err != nil {
				break
			}
		}
		var holds []moderation.RetentionHoldRecord
		if holds, err = app.Moderation.RetentionHolds(ctx, accountID); err == nil {
			err = printRetentionHolds(stdout, holds)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

func printRetentionHolds(w io.Writer, holds []moderation.RetentionHoldRecord) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tACCOUNT\tSTATE\tPLACED\tRELEASED\tBY\tREASON")
	for _, r := range holds {
		h := r.Hold
		state, released, by := "active", "-", h.CreatedByUserID
		if r.Release != nil {
			state, released = "released", r.Release.ReleasedAt.Format(time.RFC3339)
		}
		if by == "" {
			by = "cli"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", h.ID, h.AccountID, state, h.CreatedAt.Format(time.RFC3339), released, by, oneLine(h.Reason))
	}
	return tw.Flush()
}

// runAdminAccountExport implements `my admin account export-moderation`: every moderation record
// about an account as JSON on stdout, for a request of access (ADR-0038). The private notes are
// in it; the reporters' identities are not.
func runAdminAccountExport(args []string, stdout, stderr io.Writer) int {
	c, code := parseSubcommand("my admin account export-moderation", args, 1, stderr, adminAccountExportUsage, nil)
	if code >= 0 {
		return code
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	id, err := resolveAccount(ctx, app, c.fs.Arg(0))
	if err == nil {
		var x appmoderation.RecordExport
		if x, err = app.Moderation.ExportRecords(ctx, id); err == nil {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			err = enc.Encode(x)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

func adminAccountExportUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: my admin account export-moderation [-config <file>] <account>

Writes every moderation record about the account to standard output as JSON,
to answer a request of access (GDPR art. 15, APPI art. 33): the actions on it
with their private notes, lifts, and appeals; the reports of it with the
moderators' notes, assignments, and resolutions; the reports it filed; and the
audit log entries on it and on its reports and appeals. The reporters of the
reports of the account, and the servers they came from, are left out.
<account> is @username for a local account, @username@domain for a remote one,
or an account ID.
`)
}

func adminRetentionUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin retention hold -reason <text> [-config <file>] <account>
  my admin retention release [-config <file>] <hold id>
  my admin retention list [-account <account>] [-config <file>]

Holds on the erasure of an account's records after their retention:
while a hold is in effect, the purges keep the account's moderation records
(the actions on it and their appeals, the reports of it or by it, and the audit
log entries on them), its sign-in and registration IP addresses, and its posts.
Use one for a court order to preserve the records or while a request of access
or disclosure is handled. An account may have several holds; each is released
on its own, and the purges erase what is past its retention at their next run
after the last one is released. Releasing a released hold changes nothing.
list shows the holds newest first, released ones included.
<account> is @username for a local account, @username@domain for a remote one,
or an account ID.
`)
}
