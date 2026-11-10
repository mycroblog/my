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

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// runAdminDomainSubscription implements `my admin domain subscription`: the subscriptions to
// domain block lists that others publish, their fetches, and their drafts.
func runAdminDomainSubscription(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminDomainSubscriptionUsage(stderr)
		return 2
	}
	subs := []string{"list", "add", "update", "remove", "fetch", "fetches", "drafts", "accept", "reject"}
	sub, code := dispatch("admin domain subscription", args, subs, stderr, adminDomainSubscriptionUsage)
	if code >= 0 {
		return code
	}
	var (
		format, title, mode, suspendAs, silenceAs string
		rejectMedia, rejectReports, comment       optionalBool
		liftActions, all                          bool
		keys                                      string
		limit                                     = 20
	)
	nargs := 1
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin domain subscription "+sub, args[1:], nargs, stderr, adminDomainSubscriptionUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "add", "update":
			if sub == "add" {
				fs.StringVar(&format, "format", "", "mastodon_csv, mastodon_json, or plain_text (required)")
			}
			fs.StringVar(&title, "title", "", "the name of the subscription (required for add)")
			fs.StringVar(&mode, "mode", "", "draft (the default for add) or automatic")
			fs.StringVar(&suspendAs, "suspend-as", "", "what a suspend of the list issues: suspend (the default for add), limit, or ignore")
			fs.StringVar(&silenceAs, "silence-as", "", "what a silence of the list issues: limit (the default for add) or ignore")
			fs.Var(&rejectMedia, "reject-media", "issue the reject_media of the list's rows (true for add)")
			fs.Var(&rejectReports, "reject-reports", "issue the reject_reports of the list's rows (true for add)")
			fs.Var(&comment, "public-comment", "copy the public comment of the list's rows to the actions (false for add)")
		case "remove":
			fs.BoolVar(&liftActions, "lift-actions", false, "lift the actions the subscription issued; without it, they stay")
		case "fetches":
			fs.IntVar(&limit, "limit", limit, "number of fetches to show, newest first")
		case "accept", "reject":
			fs.BoolVar(&all, "all", false, "every draft of the subscription")
			fs.StringVar(&keys, "keys", "", "the keys of the drafts, separated by commas, as drafts shows them")
		}
	})
	if code >= 0 {
		return code
	}
	switch {
	case sub == "add" && (format == "" || title == ""):
		_, _ = fmt.Fprintf(stderr, "%s: -format and -title are required\n", c.name)
		return 2
	case (sub == "accept" || sub == "reject") && all == (keys != ""):
		_, _ = fmt.Fprintf(stderr, "%s: give either -all or -keys\n", c.name)
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
	svc := app.Moderation
	apply := func(st *moderation.DomainListSettings) {
		if title != "" {
			st.Title = title
		}
		if mode != "" {
			st.Mode = moderation.DomainListMode(mode)
		}
		if suspendAs != "" {
			st.SuspendAs = moderation.DomainListSeverityMapping(suspendAs)
		}
		if silenceAs != "" {
			st.SilenceAs = moderation.DomainListSeverityMapping(silenceAs)
		}
		rejectMedia.apply(&st.ApplyRejectMedia)
		rejectReports.apply(&st.ApplyRejectReports)
		comment.apply(&st.PublicCommentFromList)
	}
	id := c.fs.Arg(0)
	var err error
	switch sub {
	case "list":
		var recs []moderation.DomainListSubscriptionRecord
		if recs, err = svc.DomainListSubscriptions(ctx, "", 200); err == nil {
			err = printDomainListSubscriptions(stdout, recs)
		}
	case "add":
		st := moderation.DefaultDomainListSettings(title)
		apply(&st)
		var rec moderation.DomainListSubscriptionRecord
		rec, err = svc.CreateDomainListSubscription(ctx, appmoderation.DomainListSubscriptionInput{
			URL: id, Format: moderation.DomainListFormat(format), Settings: st,
		})
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "subscribed to %s as %s (%s mode); the worker fetches it now and every day\n",
				rec.Subscription.URL, rec.Subscription.ID, rec.Revision.Settings.Mode)
		}
	case "update":
		var rec moderation.DomainListSubscriptionRecord
		if rec, err = svc.DomainListSubscription(ctx, id); err != nil {
			break
		}
		st := rec.Revision.Settings
		apply(&st)
		if _, err = svc.UpdateDomainListSubscription(ctx, id, st, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "updated subscription %s; the worker applies the settings at its next fetch\n", id)
		}
	case "remove":
		if err = svc.DeleteDomainListSubscription(ctx, id, liftActions, ""); err == nil {
			what := "its actions stay"
			if liftActions {
				what = "its actions are lifted"
			}
			_, _ = fmt.Fprintf(stdout, "removed subscription %s; %s\n", id, what)
		}
	case "fetch":
		var f moderation.DomainListFetch
		if f, err = svc.FetchDomainListNow(ctx, id, ""); err == nil {
			printDomainListFetch(stdout, f)
		}
	case "fetches":
		var fs []moderation.DomainListFetch
		if fs, err = svc.DomainListFetches(ctx, id, "", limit); err == nil {
			err = printDomainListFetches(stdout, fs)
		}
	case "drafts":
		var ds []moderation.DomainListChange
		if ds, err = svc.DomainListDrafts(ctx, id); err == nil {
			err = printDomainListDrafts(stdout, ds)
		}
	case "accept", "reject":
		var ks []string
		if keys != "" {
			ks = strings.Split(keys, ",")
		}
		var res appmoderation.DomainListDraftResult
		if sub == "accept" {
			res, err = svc.AcceptDomainListDrafts(ctx, id, ks, "")
		} else {
			res, err = svc.RejectDomainListDrafts(ctx, id, ks, "")
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "%sed %d draft(s)\n", strings.TrimSuffix(sub, "e"), len(res.Done))
			for _, k := range res.Missing {
				_, _ = fmt.Fprintf(stdout, "no draft has the key %q\n", k)
			}
		}
	}
	if errors.Is(err, appmoderation.ErrNotFound) {
		err = fmt.Errorf("no subscription has the ID %q", id)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// optionalBool is a boolean flag that remembers whether it was given.
type optionalBool struct {
	set, value bool
}

func (b *optionalBool) String() string {
	if b == nil || !b.set {
		return ""
	}
	return fmt.Sprint(b.value)
}

func (b *optionalBool) Set(s string) error {
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		b.set, b.value = true, true
	case "false", "0", "no":
		b.set, b.value = true, false
	default:
		return fmt.Errorf("want true or false, got %q", s)
	}
	return nil
}

// IsBoolFlag lets -flag stand for -flag=true.
func (b *optionalBool) IsBoolFlag() bool { return true }

func (b *optionalBool) apply(dst *bool) {
	if b.set {
		*dst = b.value
	}
}

func printDomainListSubscriptions(w io.Writer, recs []moderation.DomainListSubscriptionRecord) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTITLE\tFORMAT\tMODE\tLAST FETCH\tOUTCOME\tENTRIES\tFAILURES\tURL")
	for _, r := range recs {
		last, outcome, entries := "-", "-", "-"
		if f := r.LastFetch; f != nil {
			last, outcome, entries = f.FetchedAt.Format(time.RFC3339), fetchOutcome(*f), fmt.Sprint(f.EntryCount)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", r.Subscription.ID, oneLine(r.Revision.Settings.Title),
			r.Subscription.Format, r.Revision.Settings.Mode, last, outcome, entries, r.Head.ConsecutiveFailures, r.Subscription.URL)
	}
	return tw.Flush()
}

func fetchOutcome(f moderation.DomainListFetch) string {
	if f.Outcome == moderation.DomainListFailed {
		return "failed: " + appmoderation.DomainListErrorText(f)
	}
	return string(f.Outcome)
}

func printDomainListFetch(w io.Writer, f moderation.DomainListFetch) {
	_, _ = fmt.Fprintf(w, "fetch %s: %s, %d domain(s) listed\n", f.ID, fetchOutcome(f), f.EntryCount)
}

func printDomainListFetches(w io.Writer, fs []moderation.DomainListFetch) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tFETCHED\tOUTCOME\tENTRIES")
	for _, f := range fs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", f.ID, f.FetchedAt.Format(time.RFC3339), fetchOutcome(f), f.EntryCount)
	}
	return tw.Flush()
}

func printDomainListDrafts(w io.Writer, ds []moderation.DomainListChange) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tCHANGE\tDOMAIN\tKIND\tWHY\tCOMMENT")
	for _, d := range ds {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", d.Key(), d.Change, d.Domain, d.Kind, d.Held, oneLine(d.Entry.PublicComment))
	}
	return tw.Flush()
}

func adminDomainSubscriptionUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin domain subscription list [-config <file>]
  my admin domain subscription add -format <format> -title <title> [-mode <mode>]
                                   [-suspend-as <kind>] [-silence-as <kind>]
                                   [-reject-media=<bool>] [-reject-reports=<bool>]
                                   [-public-comment=<bool>] [-config <file>] <url>
  my admin domain subscription update [-title <title>] [-mode <mode>] [-suspend-as <kind>]
                                      [-silence-as <kind>] [-reject-media=<bool>]
                                      [-reject-reports=<bool>] [-public-comment=<bool>]
                                      [-config <file>] <id>
  my admin domain subscription remove [-lift-actions] [-config <file>] <id>
  my admin domain subscription fetch [-config <file>] <id>
  my admin domain subscription fetches [-limit <n>] [-config <file>] <id>
  my admin domain subscription drafts [-config <file>] <id>
  my admin domain subscription accept -all | -keys <keys> [-config <file>] <id>
  my admin domain subscription reject -all | -keys <keys> [-config <file>] <id>

Subscriptions to domain block lists that others publish, at an https URL. The
worker fetches each list every day, and at once after add and update; a fetch
reads at most 2 MiB and 5000 rows. Formats:

  mastodon_csv   Mastodon's domain block CSV, as import reads it
  mastodon_json  the answer of GET /api/v1/instance/domain_blocks of a Mastodon
                 server that publishes its domain blocks; obfuscated domains are
                 skipped
  plain_text     one domain per line, each a suspend

In the draft mode, what the list adds, changes, and removes waits as drafts:
drafts lists them with their keys, accept issues or lifts them, and reject
keeps them out until the list changes the domain's row. In the automatic mode,
each fetch applies them, except a severity the server does not know and a fetch
that would lift more than 10 actions and more than half of the subscription's,
which stay drafts. A subscription never lifts nor replaces an action that it did
not issue, and issues nothing on a domain that an action of the same kind
already covers. If a person lifts an action of a subscription, the subscription
does not issue it again until the list changes the domain's row.

fetch fetches the list now and prints the outcome. A failed fetch changes
nothing. remove keeps the actions the subscription issued unless -lift-actions
is given; the kept actions are then like those a moderator issued.
`)
}
