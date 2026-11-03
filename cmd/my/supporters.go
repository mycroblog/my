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

	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// runAdminSupporters implements `my admin supporters <subcommand>`: the supporter program
// (ADR-0072 §2, §9, §10, §18), through the same operations as the admin API. Enabling,
// disabling, and the cancellations are audited as the CLI's.
func runAdminSupporters(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminSupportersUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin supporters", args, []string{"status", "enable", "disable", "plans", "subscriptions", "cancel"}, stderr, adminSupportersUsage)
	if code >= 0 {
		return code
	}
	var (
		planID, state, before string
		limit                 = 50
	)
	nargs := 0
	if sub == "cancel" {
		nargs = 1
	}
	c, code := parseSubcommand("my admin supporters "+sub, args[1:], nargs, stderr, adminSupportersUsage, func(fs *flag.FlagSet) {
		if sub != "subscriptions" {
			return
		}
		fs.StringVar(&planID, "plan", "", "show the subscriptions of this plan ID")
		fs.StringVar(&state, "state", "", "show the subscriptions in this state: pending, active, canceling, past_due, lapsed, expired, revoked, or abandoned")
		fs.StringVar(&before, "before", "", "show the subscriptions older than this subscription ID")
		fs.IntVar(&limit, "limit", limit, "number of subscriptions to show, newest first")
	})
	if code >= 0 {
		return code
	}
	switch {
	case state != "" && !supporters.State(state).Valid():
		_, _ = fmt.Fprintf(stderr, "%s: unknown -state %q\n", c.name, state)
		return 2
	case limit < 1:
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
	var p appsupporters.Program
	switch sub {
	case "status":
		if p, err = app.Supporters.Program(ctx); err == nil {
			printSupporterProgram(stdout, p)
		}
	case "enable":
		if p, err = app.Supporters.SetProgram(ctx, true, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "enabled the supporter program with %s\n", p.Provider)
		}
	case "disable":
		if _, err = app.Supporters.SetProgram(ctx, false, ""); err == nil {
			_, _ = fmt.Fprintln(stdout, "disabled the supporter program")
		}
	case "plans":
		var plans []appsupporters.PlanView
		if plans, err = app.Supporters.Plans(ctx); err == nil {
			err = printSupporterPlans(stdout, plans)
		}
	case "subscriptions":
		var list []appsupporters.SubscriptionView
		f := appsupporters.SubscriptionFilter{PlanID: planID, State: supporters.State(state), Before: before, Limit: limit}
		if list, err = app.Supporters.Subscriptions(ctx, f); err == nil {
			err = printSupporterSubscriptions(stdout, list)
		}
	case "cancel":
		if err = app.Supporters.CancelByStaff(ctx, c.fs.Arg(0), ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "canceled subscription %s\n", c.fs.Arg(0))
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", c.name, describeSupportersError(err, c.fs.Arg(0)))
		return 1
	}
	return 0
}

// describeSupportersError explains a failure to the operator.
func describeSupportersError(err error, arg string) string {
	switch {
	case errors.Is(err, appsupporters.ErrNoProvider):
		return "no payment provider is configured (payments = none); nothing was changed"
	case errors.Is(err, appsupporters.ErrNoMail):
		return "the server sends no email (mail = none), which the program needs; nothing was changed"
	case errors.Is(err, appsupporters.ErrLiveSubscriptions):
		return "subscriptions go on; retire the plans and wait for the subscriptions to end first; nothing was changed"
	case errors.Is(err, appsupporters.ErrNoLiveSubscription):
		return fmt.Sprintf("subscription %s already ended; nothing was changed", arg)
	case errors.Is(err, appsupporters.ErrNotFound):
		return fmt.Sprintf("no subscription has the ID %q", arg)
	}
	return err.Error()
}

func printSupporterProgram(w io.Writer, p appsupporters.Program) {
	state := "disabled"
	if p.Enabled {
		state = "enabled with " + p.Provider
	}
	configured := p.ConfiguredProvider
	if configured == "" {
		configured = "none"
	}
	_, _ = fmt.Fprintf(w, "program: %s\npayments: %s\nemail: %t\nlive subscriptions: %d\n", state, configured, p.SendsEmail, p.LiveSubscriptions)
	if p.ProviderMismatch() {
		_, _ = fmt.Fprintln(w, "warning: the configured payment provider is not the program's; new subscriptions are refused until the program is disabled and enabled again")
	}
}

func printSupporterPlans(w io.Writer, plans []appsupporters.PlanView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tRANK\tMONTHLY\tDISCOUNT\tVISIBILITY\tSTATE\tMEMBERS\tROLE")
	for _, p := range plans {
		state := "ready"
		switch {
		case p.Retired:
			state = "retired"
		case !p.Ready():
			state = "preparing"
		}
		monthly := finance.Money{Minor: p.Revision.MonthlyAmountMinor, Currency: finance.Currency(p.Currency)}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s %s\t%d%%\t%s\t%s\t%d\t%s\n", p.ID, oneLine(p.Revision.Name), p.Revision.Rank,
			monthly.Decimal(), p.Currency, p.Revision.AnnualDiscountPercent, p.Revision.Visibility, state, p.Members, p.Role.Key)
	}
	return tw.Flush()
}

func printSupporterSubscriptions(w io.Writer, list []appsupporters.SubscriptionView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tUSER\tPLAN\tINTERVAL\tSTATE\tPERIOD ENDS\tSTARTED")
	for _, s := range list {
		ends := "-"
		if !s.Current.PeriodEndsAt.IsZero() {
			ends = s.Current.PeriodEndsAt.UTC().Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, s.UserID, oneLine(s.Plan.Revision.Name), s.Price.Interval,
			s.Current.State, ends, s.CreatedAt.UTC().Format(time.RFC3339))
	}
	return tw.Flush()
}

func adminSupportersUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin supporters status [-config <file>]
  my admin supporters enable [-config <file>]
  my admin supporters disable [-config <file>]
  my admin supporters plans [-config <file>]
  my admin supporters subscriptions [-plan <id>] [-state <state>] [-before <id>] [-limit <n>] [-config <file>]
  my admin supporters cancel [-config <file>] <subscription id>

The supporter program (ADR-0072): paid plans that local users subscribe to on the
web client, through the payment provider of payments.

status shows whether the program is enabled and what enabling it needs. enable
enables it with the configured payment provider; it needs a provider and email
(mail other than none). disable disables it; it fails while subscriptions go on:
retire the plans first (on the admin screen) and wait for them to end.

plans lists every plan, retired ones too, by rank: STATE is ready, preparing
(its prices are not made at the provider yet), or retired, and MEMBERS counts the
subscriptions that hold its role. subscriptions lists the subscriptions, newest
first, of a -plan or in a -state; USER is the subscriber's user ID.

cancel ends a subscription at once, without a refund: the plan's role goes, and
the provider's subscription is canceled at once by a job.

Enabling, disabling, and cancelling are recorded in the audit log as the CLI's,
as those of the admin screen and the admin API (/api/v1/admin/supporter_program,
/api/v1/admin/supporter_subscriptions) are.
`)
}
