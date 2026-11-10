// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	appfederation "github.com/mycroblog/my/internal/app/federation"
)

// runAdminFederation implements `my admin federation <subcommand>`: the follows with remote
// accounts, for the operator who narrows the federation mode (ADR-0006).
func runAdminFederation(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("admin federation", args, []string{"sever", "scope"}, stderr, adminFederationUsage)
	if code >= 0 || len(args) == 0 {
		return max(code, 0)
	}
	if sub == "scope" {
		return runAdminFederationScope(args[1:], stdout, stderr)
	}
	var direction string
	c, code := parseSubcommand("my admin federation "+sub, args[1:], 0, stderr, adminFederationUsage, func(fs *flag.FlagSet) {
		fs.StringVar(&direction, "direction", "", "in (remote followers of local accounts), out (local accounts' follows of remote ones), or both (required)")
	})
	if code >= 0 {
		return code
	}
	dir := appfederation.SeverDirection(direction)
	switch dir {
	case appfederation.SeverIn, appfederation.SeverOut, appfederation.SeverBothWays:
	default:
		_, _ = fmt.Fprintf(stderr, "%s: -direction must be in, out, or both\n", c.name)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	severed, err := app.FederationSever.Sever(ctx, dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "severed %d follows of local accounts by remote ones and %d follows of remote accounts by local ones\n",
		severed.Followers, severed.Following)
	return 0
}

func adminFederationUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin federation sever -direction in|out|both [-config <file>]
  my admin federation scope [-apply] [-config <file>] [open|allowlist]

The follows between the local accounts and the remote ones.

sever ends every pending or active follow with a remote account in the direction
given: in ends the remote accounts' follows of local ones, out the local accounts'
follows of remote ones, and both all of them. They end as a domain suspension ends
them (follow_terminations with the reason moderation) and do not come back when
federation is widened again. Where the federation mode sends them, the remote
side gets a Reject of its Follow (in) or an Undo of the local account's Follow
(out); in a mode that does not, nothing is sent. Changing the federation mode
never severs anything by itself.

scope shows the instance setting federation_scope and, given open or
allowlist, changes it (see my admin federation scope -help).
`)
}
