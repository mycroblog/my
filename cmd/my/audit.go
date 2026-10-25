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
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// runAdminAudit implements `my admin audit list`: the audit log (FR-704, ADR-0037). The audit
// log is read-only; no subcommand changes or deletes an entry.
func runAdminAudit(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminAuditUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin audit", args, []string{"list"}, stderr, adminAuditUsage)
	if code >= 0 {
		return code
	}
	var (
		actor, action, targetKind, targetID, before string
		limit                                       = 50
	)
	c, code := parseSubcommand("my admin audit "+sub, args[1:], 0, stderr, adminAuditUsage, func(fs *flag.FlagSet) {
		fs.StringVar(&actor, "actor", "", "show only the operations of the local user with this username")
		fs.StringVar(&action, "action", "", "show only this kind of operation, such as domain_action_issued")
		fs.StringVar(&targetKind, "target-kind", "", "show only the operations on this kind of target, such as domain")
		fs.StringVar(&targetID, "target-id", "", "show only the operations on this target (needs -target-kind)")
		fs.StringVar(&before, "before", "", "show the entries older than this entry ID")
		fs.IntVar(&limit, "limit", limit, "number of entries to show, newest first")
	})
	if code >= 0 {
		return code
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
	q := appmoderation.AuditQuery{
		Action: moderation.AuditAction(action), TargetKind: moderation.AuditTargetKind(targetKind), TargetID: targetID, Before: before,
	}
	err := func() error {
		if actor != "" {
			a, err := app.Accounts.FindByHandle(ctx, strings.TrimPrefix(actor, "@"), "")
			if errors.Is(err, appaccount.ErrNotFound) || (err == nil && a.Local == nil) {
				return fmt.Errorf("no local account has the username %q", actor)
			}
			if err != nil {
				return err
			}
			q.ActorUserID = a.Local.UserID
		}
		entries, err := app.Moderation.AuditLog(ctx, q, limit)
		if err != nil {
			return err
		}
		return printAuditLog(ctx, app, stdout, entries)
	}()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

func printAuditLog(ctx context.Context, app *platform.App, w io.Writer, entries []moderation.AuditEntry) error {
	cmd := reportCommand{app: app}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTIME\tACTOR\tACTION\tTARGET\tTARGET ID\tSOURCE")
	for _, e := range entries {
		by := string(e.ActorKind)
		if e.ActorKind == moderation.AuditActorUser {
			by = cmd.moderator(ctx, e.ActorUserID)
		}
		source := e.SourceEventID
		if source == "" {
			source = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s %s\t%s\t%s\n", e.ID, e.OccurredAt.Format(time.RFC3339), by, e.Action,
			e.TargetKind, oneLine(e.TargetLabel), e.TargetID, source)
	}
	return tw.Flush()
}

func adminAuditUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin audit list [-actor <username>] [-action <action>] [-target-kind <kind>]
                      [-target-id <id>] [-before <id>] [-limit <n>] [-config <file>]

The audit log (FR-704): the operations of the moderators and administrators,
newest first, with who performed them (a user, or cli for this command line),
the target, and the ID of the operation's own record, which holds its reasons
and notes. The target's name is the one it had at the time, so entries stay
readable after the target is gone. The audit log cannot be changed or deleted.

Actions: account_action_issued, account_action_lifted,
account_deletion_cancelled, revealed_private_data, two_factor_disabled,
password_reset_link_issued, account_moved (target kind account); domain_action_issued, domain_action_lifted (domain);
report_assigned, report_unassigned, report_resolved, report_reopened,
report_note_added (report); registration_approved, registration_rejected
(user); setting_changed, text_changed (setting); rule_added, rule_revised,
rule_moved, rule_retired (rule); role_granted, role_revoked, role_created,
role_updated, role_deleted (role); appeal_upheld, appeal_overturned (appeal);
signup_block_added, signup_block_lifted (signup_block);
custom_emoji_created, custom_emoji_image_replaced, custom_emoji_copied,
custom_emoji_updated, custom_emoji_enabled, custom_emoji_disabled,
custom_emoji_deleted (custom_emoji); webhook_created, webhook_updated, webhook_deleted,
webhook_enabled, webhook_disabled, webhook_secret_rotated (webhook);
announcement_created, announcement_updated, announcement_published,
announcement_withdrawn (announcement); invite_created, invite_revoked (invite).
`)
}
