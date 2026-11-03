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
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// runAdminAccount implements `my admin account <subcommand>`.
func runAdminAccount(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminAccountActionUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin account", args, []string{"action", "export-moderation", "disable-two-factor", "password-reset-link"}, stderr, adminAccountActionUsage)
	if code >= 0 {
		return code
	}
	switch sub {
	case "export-moderation":
		return runAdminAccountExport(args[1:], stdout, stderr)
	case "disable-two-factor":
		return runAdminAccountDisableTwoFactor(args[1:], stdout, stderr)
	case "password-reset-link":
		return runAdminAccountPasswordResetLink(args[1:], stdout, stderr)
	}
	return runAdminAccountAction(args[1:], stdout, stderr)
}

// runAdminAccountAction implements `my admin account action <subcommand>`: the moderation
// actions on accounts (FR-703). The CLI acts as no user: the actions it issues and lifts record
// no moderator.
func runAdminAccountAction(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminAccountActionUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin account action", args, []string{"list", "issue", "lift"}, stderr, adminAccountActionUsage)
	if code >= 0 {
		return code
	}
	var (
		kind, category, publicReason, note, report, reason, before string
		active, noNotify, blockEmail                               bool
		duration                                                   time.Duration
		limit                                                      = 50
		statuses                                                   []string
	)
	c, code := parseSubcommand("my admin account action "+sub, args[1:], 1, stderr, adminAccountActionUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "list":
			fs.BoolVar(&active, "active", false, "show only the actions in effect now")
			fs.StringVar(&before, "before", "", "show the actions older than this action ID")
			fs.IntVar(&limit, "limit", limit, "number of actions to show, newest first")
		case "issue":
			fs.StringVar(&kind, "kind", "", "the kind of action (required; see below)")
			fs.StringVar(&category, "category", string(moderation.CategoryOther), "spam, legal, violation, or other")
			fs.StringVar(&publicReason, "public-reason", "", "the reason shown to the account")
			fs.StringVar(&note, "private-note", "", "a note for the moderators")
			fs.StringVar(&report, "report", "", "the ID of the report of the account that the action follows from; an open report is resolved as actioned")
			fs.BoolVar(&noNotify, "no-notify", false, "do not tell the account of the action")
			fs.BoolVar(&blockEmail, "block-email", false, "with a ban, also block sign-ups with the email address of the account's user")
			fs.DurationVar(&duration, "duration", 0, "how long the action lasts, such as 720h; 0 lasts until lifted")
			fs.Func("status", "the ID of a status of the account that delete_statuses or mark_statuses_sensitive acts on; repeat it for several", func(s string) error {
				statuses = append(statuses, s)
				return nil
			})
		case "lift":
			fs.StringVar(&reason, "reason", "", "why the action is lifted")
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
		var id string
		if id, err = resolveAccount(ctx, app, c.fs.Arg(0)); err != nil {
			break
		}
		var vs []appmoderation.AccountActionView
		f := appmoderation.AccountActionFilter{AccountID: id, Active: active, Before: before}
		if vs, err = app.Moderation.AccountActions(ctx, f, limit); err == nil {
			err = printAccountActions(stdout, vs)
		}
	case "issue":
		var id string
		if id, err = resolveAccount(ctx, app, c.fs.Arg(0)); err != nil {
			break
		}
		var a moderation.Action
		a, err = app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: id, Kind: moderation.ActionKind(kind), ReasonCategory: moderation.Category(category),
			PublicReason: publicReason, PrivateNote: note, ReportID: report, NotifyTarget: !noNotify, Duration: duration,
			StatusIDs: statuses, BlockEmail: blockEmail,
		})
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "issued %s on account %s (action %s, %s)\n", a.Kind, a.TargetAccountID, a.ID, describeExpiry(a.ExpiresAt))
			if len(a.StatusIDs) > 0 {
				_, _ = fmt.Fprintf(stdout, "statuses: %s\n", strings.Join(a.StatusIDs, " "))
			}
			if blockEmail {
				_, _ = fmt.Fprintln(stdout, "sign-ups with the account's email address are blocked (my admin signup-block list)")
			}
		}
	case "lift":
		id := c.fs.Arg(0)
		err = app.Moderation.LiftAccountAction(ctx, id, "", reason)
		switch {
		case errors.Is(err, appmoderation.ErrNotFound):
			err = fmt.Errorf("no account action has the ID %q", id)
		case errors.Is(err, appmoderation.ErrNotLiftable):
			err = fmt.Errorf("account action %s is a ban, which is never lifted", id)
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "lifted account action %s\n", id)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// resolveAccount returns the ID of the account that arg names: @username for a local account,
// @username@domain for a remote one, or an account ID.
func resolveAccount(ctx context.Context, app *platform.App, arg string) (string, error) {
	var (
		a   appaccount.Actor
		err error
	)
	if handle, ok := strings.CutPrefix(arg, "@"); ok {
		username, domain, _ := strings.Cut(handle, "@")
		if domain != "" {
			if domain, err = appfederation.CanonicalHost(domain); err != nil {
				return "", fmt.Errorf("the domain of %q: %w", arg, err)
			}
		}
		a, err = app.Accounts.FindByHandle(ctx, username, domain)
	} else {
		a, err = app.Accounts.FindByID(ctx, arg)
	}
	if errors.Is(err, appaccount.ErrNotFound) {
		return "", fmt.Errorf("no account is %q", arg)
	}
	if err != nil {
		return "", err
	}
	return a.Account.ID, nil
}

func printAccountActions(w io.Writer, vs []appmoderation.AccountActionView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tSTATE\tCATEGORY\tISSUED\tEXPIRES\tBY\tREPORT\tREASON")
	for _, v := range vs {
		a := v.Action
		expires, by, report := "-", a.IssuedByUserID, a.ReportID
		if a.ExpiresAt != nil {
			expires = a.ExpiresAt.Format(time.RFC3339)
		}
		if by == "" {
			by = "cli"
		}
		if report == "" {
			report = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Kind, v.State, a.ReasonCategory,
			a.IssuedAt.Format(time.RFC3339), expires, by, report, oneLine(a.PublicReason))
	}
	return tw.Flush()
}

func adminAccountActionUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin account action list [-active] [-before <id>] [-limit <n>] [-config <file>] <account>
  my admin account action issue -kind <kind> [-category <category>] [-public-reason <text>]
                                [-private-note <text>] [-report <report id>] [-no-notify]
                                [-duration <d>] [-status <status id>]... [-block-email]
                                [-config <file>] <account>
  my admin account action lift [-reason <text>] [-config <file>] <action id>

Moderation actions on accounts, in effect at once. <account> is
@username for a local account, @username@domain for a remote one, or an account
ID. Several actions on one account add up. On a local account:

  warn                  only tells the account
  force_sensitive       its media are shown as sensitive
  limit                 its posts, past ones included, are followers-only and
                        stay off the public timelines
  restrict_posting      it cannot post, reply, boost, or edit
  restrict_interaction  it cannot follow, favourite, boost, or reply
  disable_login         its sessions and tokens end and it cannot log in
  suspend               all of the above, and its profile and posts are hidden
  ban                   a suspension that is never lifted; the account is
                        deleted 30 days later
  delete_statuses       deletes the statuses given with -status, for everyone
  mark_statuses_sensitive
                        shows the media of the statuses given with -status as
                        sensitive

delete_statuses and mark_statuses_sensitive need one -status or more (up to
100), and take no -duration; nor does ban. A remote account takes limit,
suspend, and force_sensitive, with the same effects here; suspend also drops
what the account sends. An action on a remote account is not sent to its
server, and -no-notify does not apply to it. -block-email, only with a ban of a
local account, also rejects sign-ups with the email address of the account's
user (a sign-up block of kind email_hash; see my admin signup-block -help).

list shows the actions on the account newest first, lifted and expired ones
included unless -active is given. lift ends one action; the other actions on
the account stay, and so do the statuses that delete_statuses deleted. Lifting
a lifted action changes nothing; a ban is never lifted.
`)
}
