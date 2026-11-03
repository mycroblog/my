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

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// runAdminEmojiRequests implements `my admin emoji requests <subcommand>`: the requests of the
// holders of create_custom_emojis (ADR-0070 §2, §6), through the same operations as the admin
// API. The CLI's approvals and rejections are audited as the CLI's.
func runAdminEmojiRequests(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminEmojiRequestsUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin emoji requests", args, []string{"list", "approve", "reject"}, stderr, adminEmojiRequestsUsage)
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
	c, code := parseSubcommand("my admin emoji requests "+sub, args[1:], nargs, stderr, adminEmojiRequestsUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "list":
			fs.StringVar(&state, "state", string(content.RequestPending), "show the requests in this state: pending, approved, rejected, withdrawn, lapsed, or all")
			fs.StringVar(&before, "before", "", "show the requests older than this request ID")
			fs.IntVar(&limit, "limit", limit, "number of requests to show, newest first")
		case "reject":
			fs.StringVar(&reason, "reason", "", "why, up to 500 characters, which the requester sees")
		}
	})
	if code >= 0 {
		return code
	}
	q := appcontent.CustomEmojiRequestQuery{Before: before}
	if state != "all" {
		q.State = content.CustomEmojiRequestState(state)
	}
	switch {
	case sub == "list" && state != "all" && !q.State.Valid():
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
	var r appcontent.CustomEmojiRequestView
	switch sub {
	case "list":
		var list []appcontent.CustomEmojiRequestView
		if list, err = app.CustomEmojis.ListRequests(ctx, q, limit); err == nil {
			err = printEmojiRequests(stdout, list)
		}
	case "approve":
		if r, err = app.CustomEmojis.ApproveRequest(ctx, c.fs.Arg(0), ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "approved %s (request %s, emoji %s)\n", r.Request.Label(), r.Request.ID, r.Outcome.EmojiID)
		}
	case "reject":
		if r, err = app.CustomEmojis.RejectRequest(ctx, c.fs.Arg(0), reason, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "rejected %s (request %s)\n", r.Request.Label(), r.Request.ID)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", c.name, describeEmojiRequestError(err, c.fs.Arg(0)))
		return 1
	}
	return 0
}

// describeEmojiRequestError explains a failure to the operator.
func describeEmojiRequestError(err error, arg string) string {
	var conflict *appcontent.CustomEmojiRequestConflict
	switch {
	case errors.As(err, &conflict):
		return map[appcontent.CustomEmojiRequestConflictReason]string{
			appcontent.RequestConflictNotPending:            "the request is not pending",
			appcontent.RequestConflictRequesterCannotCreate: "the requester can no longer create custom emoji; reject the request instead",
			appcontent.RequestConflictShortcodeTaken:        "a custom emoji of this instance already has the shortcode; reject the request instead",
		}[conflict.Reason] + "; nothing was changed"
	case errors.Is(err, appcontent.ErrNotFound):
		return fmt.Sprintf("no custom emoji request has the ID %q", arg)
	case errors.Is(err, content.ErrInvalidCustomEmoji):
		var ce *content.CustomEmojiError
		if errors.As(err, &ce) && ce.Reason == content.EmojiInvalidReason {
			return "a reason is at most 500 characters; nothing was changed"
		}
		return describeEmojiError(err, arg)
	}
	return err.Error()
}

func printEmojiRequests(w io.Writer, list []appcontent.CustomEmojiRequestView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tSHORTCODE\tSTATE\tREQUESTER\tCREATED\tREASON")
	for _, r := range list {
		reason := "-"
		if r.Outcome != nil && r.Outcome.Reason != "" {
			reason = oneLine(r.Outcome.Reason)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Request.ID, r.Request.Kind, r.Request.Shortcode, r.State(),
			r.Request.RequesterUserID, r.Request.CreatedAt.UTC().Format(time.RFC3339), reason)
	}
	return tw.Flush()
}

func adminEmojiRequestsUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin emoji requests list [-state <state>|all] [-before <id>] [-limit <n>] [-config <file>]
  my admin emoji requests approve [-config <file>] <request id>
  my admin emoji requests reject [-reason <text>] [-config <file>] <request id>

The requests of the users whose roles grant create_custom_emojis: a
request for a new local custom emoji (kind create) or for a new image of one the
requester made (kind image). A request is no emoji until it is approved.

list shows the pending requests, newest first, or those in -state (approved,
rejected, withdrawn, lapsed, or all); REQUESTER is the requester's user ID.
approve makes the emoji, without a category, in the picker, whose creator is the
requester, or gives the emoji the new image; the image becomes an upload of the
instance actor. It fails when the requester can no longer create custom emoji or
a local emoji has the shortcode: reject the request then. reject rejects it with
an optional -reason that the requester sees.

The approvals and rejections are recorded in the audit log as the CLI's and are
notified to the requester, as those of the admin screen and the admin API
(/api/v1/admin/custom_emoji_requests) are.
`)
}
