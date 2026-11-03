// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/platform"
)

// runAdminRefetch implements `my admin refetch <subcommand>`: the refetch of remote posts from
// their servers (FR-407, federation.md §6.4). The CLI acts as no user: the request is recorded
// in the audit log as the command line's.
func runAdminRefetch(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("admin refetch", args, []string{"status", "account", "domain"}, stderr, adminRefetchUsage)
	if code >= 0 || len(args) == 0 {
		return max(code, 0)
	}
	c, code := parseSubcommand("my admin refetch "+sub, args[1:], 1, stderr, adminRefetchUsage, nil)
	if code >= 0 {
		return code
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	arg := c.fs.Arg(0)
	var err error
	switch sub {
	case "status":
		var id string
		if id, err = resolveStatus(ctx, app, arg); err == nil {
			if err = app.Refetches.RefetchStatus(ctx, id, ""); err == nil {
				_, _ = fmt.Fprintf(stdout, "queued the refetch of status %s\n", id)
			}
		}
	case "account":
		var id string
		if id, err = resolveAccount(ctx, app, arg); err == nil {
			if err = app.Refetches.RefetchAccount(ctx, id, ""); err == nil {
				_, _ = fmt.Fprintf(stdout, "queued the refetch of the newest %d statuses of account %s\n", appfederation.RefetchMaxStatuses, id)
			}
		}
	case "domain":
		var host string
		if host, err = app.Refetches.RefetchDomain(ctx, arg, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "queued the refetch of the statuses of the accounts of %s (%d per account, %d in all)\n",
				host, appfederation.RefetchStatusesPerAccount, appfederation.RefetchMaxStatuses)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", c.name, describeRefetchError(err, arg))
		return 1
	}
	return 0
}

// resolveStatus returns the ID of the status that arg names: its ID, or its URI.
func resolveStatus(ctx context.Context, app *platform.App, arg string) (string, error) {
	if !strings.HasPrefix(arg, "https://") && !strings.HasPrefix(arg, "http://") {
		return arg, nil
	}
	st, err := app.Content.FindStatusByURI(ctx, arg)
	if errors.Is(err, appcontent.ErrNotFound) {
		return "", fmt.Errorf("%w: no status has the URI %q", appfederation.ErrNotFound, arg)
	}
	return st.ID, err
}

// describeRefetchError turns a failure of the refetch use cases into a message for the operator.
func describeRefetchError(err error, arg string) string {
	switch {
	case errors.Is(err, appfederation.ErrNotFound):
		return fmt.Sprintf("no remote status or account is %q, or it is deleted", arg)
	case errors.Is(err, appfederation.ErrNotRemote):
		return fmt.Sprintf("%q is not remote: only remote statuses, remote accounts, and other servers are refetched", arg)
	case errors.Is(err, appfederation.ErrInvalidHost):
		return fmt.Sprintf("%q is not a domain", arg)
	}
	return err.Error()
}

func adminRefetchUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  my admin refetch status [-config <file>] <status id or URI>
  my admin refetch account [-config <file>] <@username@domain or account id>
  my admin refetch domain [-config <file>] <domain>

Queues the refetch of stored remote posts from their servers. Each
post is fetched again with the rules of every fetch (signed by the instance
actor, never from a suspended or unavailable server) and recorded as a received
Update would be: a newer revision, the counts of its poll, or its deletion when
the server answers 410 Gone or serves a Tombstone. A post that cannot be
fetched is left as it is. Nothing refetches posts on a schedule.

status refetches one remote post. account refetches the newest %d posts of a
remote account that are not deleted. domain refetches the newest %d posts of
each account of a remote server, newest accounts first, %d in all. The posts
of account and domain are fetched in batches a minute apart. Every request is
recorded in the audit log.
`, appfederation.RefetchMaxStatuses, appfederation.RefetchStatusesPerAccount, appfederation.RefetchMaxStatuses)
}
