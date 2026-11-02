// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// runAdminAccountPasswordResetLink implements `my admin account password-reset-link <account>`:
// it issues a link that sets a new password for a local user who forgot theirs (FR-002), for the
// operator to hand to the user, such as on a server that sends no email. The CLI acts as no
// user; the audit log records it as the CLI's.
func runAdminAccountPasswordResetLink(args []string, stdout, stderr io.Writer) int {
	c, code := parseSubcommand("my admin account password-reset-link", args, 1, stderr, adminAccountPasswordResetLinkUsage, nil)
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
	var link appidentity.PasswordResetLink
	if err == nil {
		link, err = app.Identity.IssuePasswordResetLink(ctx, id, "")
		if errors.Is(err, appidentity.ErrNotLocalAccount) {
			err = fmt.Errorf("%s is not the account of a local user", c.fs.Arg(0))
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "%s\n", link.URL)
	_, _ = fmt.Fprintf(stderr, "the link sets a new password for account %s once, until %s\n", id, link.ExpiresAt.UTC().Format(time.RFC3339))
	if link.LoginProvidersOnly {
		_, _ = fmt.Fprintln(stderr, "warning: sign-in is limited to login providers, so the link does not work until the limit ends (my instance set login_providers_only false)")
	}
	return 0
}

func adminAccountPasswordResetLinkUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: my admin account password-reset-link [-config <file>] <account>

Prints a link that sets a new password for a local account's user who forgot
it, such as on a server that sends no email. The link works once, for one hour,
and stops working when the password changes in another way. Setting the new
password signs the user out everywhere; passkeys and two-factor authentication
stay. Hand the link to the account's owner only, after making sure the request
comes from them. The audit log records it as password_reset_link_issued.

While sign-in is limited to login providers (the instance setting
login_providers_only), the link does not work until the limit ends; the command
still prints it and warns about it.
<account> is @username or an account ID.
`)
}
