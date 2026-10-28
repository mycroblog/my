// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// subcommand is the parsed command line of `my invite <sub>` or `my registration <sub>`.
type subcommand struct {
	name string
	fs   *flag.FlagSet
	file *string
}

// parseSubcommand parses args (the subcommand's flags and arguments) and checks that there are
// nargs positional arguments. It returns the exit code to stop with, or -1 to go on.
func parseSubcommand(name string, args []string, nargs int, stderr io.Writer, usage func(io.Writer), define func(*flag.FlagSet)) (subcommand, int) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }
	c := subcommand{name: name, fs: fs, file: fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")}
	if define != nil {
		define(fs)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return c, 0
		}
		return c, 2
	}
	if fs.NArg() != nargs {
		_, _ = fmt.Fprintf(stderr, "%s: want %d argument(s), got %d\n\n", name, nargs, fs.NArg())
		usage(stderr)
		return c, 2
	}
	return c, -1
}

// open loads the configuration and builds the App. It returns the exit code to stop with on
// failure.
func (c subcommand) open(ctx context.Context, stderr io.Writer) (*platform.App, int) {
	cfg, _, err := config.Load(config.Source{File: *c.file})
	if err != nil {
		printLoadError(stderr, err)
		return nil, 1
	}
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return nil, 1
	}
	return app, -1
}

// dispatch picks the subcommand of args[0] among subs, printing usage for none, help, or an
// unknown one. It returns the subcommand, or "" with the exit code to stop with.
func dispatch(cmd string, args []string, subs []string, stderr io.Writer, usage func(io.Writer)) (string, int) {
	if len(args) == 0 {
		usage(stderr)
		return "", 2
	}
	switch args[0] {
	case "-h", "-help", "--help":
		usage(stderr)
		return "", 0
	}
	for _, s := range subs {
		if args[0] == s {
			return s, -1
		}
	}
	_, _ = fmt.Fprintf(stderr, "my %s: unknown subcommand %q\n\n", cmd, args[0])
	usage(stderr)
	return "", 2
}

// runInvite implements `my invite <subcommand>`.
func runInvite(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("invite", args, []string{"create", "list", "revoke"}, stderr, inviteUsage)
	if code >= 0 {
		return code
	}
	var (
		maxUses   int
		expiresIn time.Duration
		limit     = 50
		nargs     = 0
	)
	if sub == "revoke" {
		nargs = 1
	}
	c, code := parseSubcommand("my invite "+sub, args[1:], nargs, stderr, inviteUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "create":
			fs.IntVar(&maxUses, "max-uses", 1, "how many sign-ups the invite allows; 0 means no limit")
			fs.DurationVar(&expiresIn, "expires-in", 7*24*time.Hour, "how long the invite works, such as 72h; 0 means forever")
		case "list":
			fs.IntVar(&limit, "limit", limit, "number of invites to show, newest first")
		}
	})
	if code >= 0 {
		return code
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var err error
	switch sub {
	case "create":
		var inv identity.Invite
		inv, err = app.Identity.CreateInvite(ctx, appidentity.CreateInviteInput{MaxUses: maxUses, ExpiresIn: expiresIn})
		if err == nil {
			link := app.Config.ServerOrigin() + authpages.PathSignUp + "?invite=" + url.QueryEscape(inv.Code)
			_, _ = fmt.Fprintf(stdout, "created invite %s (%s, %s)\n%s\n", inv.Code, describeUses(inv.MaxUses), describeExpiry(inv.ExpiresAt), link)
		}
	case "list":
		var list []appidentity.InviteStatus
		if list, err = app.Identity.ListInvites(ctx, "", limit); err == nil {
			err = printInvites(stdout, list, app.Clock.Now())
		}
	case "revoke":
		var inv identity.Invite
		inv, err = app.Identity.RevokeInvite(ctx, c.fs.Arg(0), "")
		if errors.Is(err, appidentity.ErrNotFound) {
			err = fmt.Errorf("no invite has the code %q", c.fs.Arg(0))
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "revoked invite %s\n", inv.Code)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

func describeUses(maxUses int) string {
	switch maxUses {
	case 0:
		return "unlimited uses"
	case 1:
		return "1 use"
	}
	return fmt.Sprintf("%d uses", maxUses)
}

func describeExpiry(at *time.Time) string {
	if at == nil {
		return "never expires"
	}
	return "expires " + at.Format(time.RFC3339)
}

func printInvites(w io.Writer, list []appidentity.InviteStatus, now time.Time) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CODE\tSTATUS\tUSES\tMAX\tEXPIRES\tCREATED\tBY")
	for _, s := range list {
		status := "usable"
		switch {
		case s.Revoked:
			status = "revoked"
		case s.Expired(now):
			status = "expired"
		case s.Exhausted(s.Uses):
			status = "used up"
		case s.State == appidentity.InviteCreatorIneligible:
			status = "inviter cannot invite"
		}
		maxUses, expires, by := "-", "-", s.CreatedByUserID
		if s.CreatorKind == identity.InviteByMember {
			by += " (member)"
		}
		if s.MaxUses > 0 {
			maxUses = fmt.Sprint(s.MaxUses)
		}
		if s.ExpiresAt != nil {
			expires = s.ExpiresAt.Format(time.RFC3339)
		}
		if by == "" {
			by = "cli"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n", s.Code, status, s.Uses, maxUses, expires, s.CreatedAt.Format(time.RFC3339), by)
	}
	return tw.Flush()
}

func inviteUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my invite create [-max-uses <n>] [-expires-in <duration>] [-config <file>]
  my invite list [-limit <n>] [-config <file>]
  my invite revoke [-config <file>] <code>

Manages the staff's invite codes, which let people sign up while the
registration_mode instance setting is "open", "approval" (approved at once), or
"invite"; no invite is created or works while it is "closed"
(docs/operations/registration.md). list also shows the invites that members
created; one whose creator can no longer invite does not work. create prints
the new code and a sign-up link that fills it in; by default an invite allows
1 sign-up and expires after 7 days (-max-uses 0 and -expires-in 0 lift the
limits). Concurrent sign-ups never use an invite more often than it allows.
revoke stops a code from working; revoking it again changes nothing.
`)
}

// runRegistration implements `my registration <subcommand>`.
func runRegistration(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("registration", args, []string{"list", "approve", "reject"}, stderr, registrationUsage)
	if code >= 0 {
		return code
	}
	var (
		state = string(identity.RegistrationPending)
		limit = 50
		note  string
		nargs = 1
	)
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my registration "+sub, args[1:], nargs, stderr, registrationUsage, func(fs *flag.FlagSet) {
		if sub == "list" {
			fs.StringVar(&state, "state", state, "registrations to show: pending, approved, or rejected")
			fs.IntVar(&limit, "limit", limit, "number of registrations to show, longest in the state first")
		} else {
			fs.StringVar(&note, "note", "", "a note for the record")
		}
	})
	if code >= 0 {
		return code
	}
	switch identity.RegistrationState(state) {
	case identity.RegistrationPending, identity.RegistrationApproved, identity.RegistrationRejected:
	default:
		_, _ = fmt.Fprintf(stderr, "%s: -state must be pending, approved, or rejected\n", c.name)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var err error
	if sub == "list" {
		var list []appidentity.RegistrationEntry
		if list, err = app.Identity.ListRegistrations(ctx, identity.RegistrationState(state), limit); err == nil {
			err = printRegistrations(stdout, list)
		}
	} else {
		err = decide(ctx, app, stdout, c.fs.Arg(0), sub, note)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// decide approves or rejects the pending registration of a local username.
func decide(ctx context.Context, app *platform.App, stdout io.Writer, username, sub, note string) error {
	username = strings.TrimPrefix(username, "@")
	actor, err := app.Accounts.FindByHandle(ctx, username, account.LocalDomain)
	if errors.Is(err, appaccount.ErrNotFound) || (err == nil && actor.Local == nil) {
		return fmt.Errorf("no local account has the username %q", username)
	}
	if err != nil {
		return err
	}
	decision := identity.DecisionApproved
	if sub == "reject" {
		decision = identity.DecisionRejected
	}
	_, err = app.Identity.DecideRegistration(ctx, appidentity.DecideInput{UserID: actor.Local.UserID, Decision: decision, Note: note})
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return fmt.Errorf("@%s did not sign up on the registration page; there is nothing to decide", username)
	case errors.Is(err, appidentity.ErrNotPending):
		return fmt.Errorf("the registration of @%s is already decided", username)
	case err != nil:
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s the registration of @%s (user %s)\n", decision, username, actor.Local.UserID)
	return err
}

func printRegistrations(w io.Writer, list []appidentity.RegistrationEntry) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "USERNAME\tEMAIL\tVERIFIED\tSINCE\tUSER\tREASON")
	for _, r := range list {
		verified := "no"
		if r.EmailVerified {
			verified = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Username, r.Email, verified, r.State.Since.Format(time.RFC3339),
			r.Application.UserID, preview(r.Application.Reason))
	}
	return tw.Flush()
}

func registrationUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my registration list [-state pending|approved|rejected] [-limit <n>] [-config <file>]
  my registration approve [-note <text>] [-config <file>] <username>
  my registration reject [-note <text>] [-config <file>] <username>

Reviews the sign-ups from the registration page (docs/operations/registration.md).
In the "approval" registration mode, and in "open" mode while mail = none, new
users wait for a moderator: they can sign in, but cannot use the app until
their registration is approved. list shows the pending registrations by
default, longest waiting first, with the reason the applicant gave. approve and
reject decide one pending registration; a decided registration cannot be
decided again.
`)
}
