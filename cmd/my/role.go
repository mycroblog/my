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

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform"
)

// runAdminRole implements `my admin role grant|revoke|list` (FR-720). The command line acts as
// no user, so it may grant and revoke any role; it still keeps the last admin.
func runAdminRole(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminRoleUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin role", args, []string{"grant", "revoke", "list"}, stderr, adminRoleUsage)
	if code >= 0 {
		return code
	}
	var user string
	nargs := 2
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin role "+sub, args[1:], nargs, stderr, adminRoleUsage, func(fs *flag.FlagSet) {
		if sub == "list" {
			fs.StringVar(&user, "user", "", "show only the roles of the local user with this username")
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
	// The command may run before my serve has started on a new version: show and grant the
	// built-in roles with the permissions this version gives them.
	err := app.Identity.EnsureBuiltinRoles(ctx)
	if err == nil {
		switch sub {
		case "list":
			err = listRoles(ctx, app, stdout, user)
		default:
			err = changeRole(ctx, app, stdout, sub, c.fs.Arg(0), c.fs.Arg(1))
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// localUserID returns the user of the local account with username.
func localUserID(ctx context.Context, app *platform.App, username string) (string, error) {
	username = strings.TrimPrefix(username, "@")
	a, err := app.Accounts.FindByHandle(ctx, username, account.LocalDomain)
	if errors.Is(err, appaccount.ErrNotFound) || (err == nil && a.Local == nil) {
		return "", fmt.Errorf("no local account has the username %q", username)
	}
	if err != nil {
		return "", err
	}
	return a.Local.UserID, nil
}

// changeRole grants or revokes (sub) the role with key on the user of a local username.
func changeRole(ctx context.Context, app *platform.App, stdout io.Writer, sub, username, key string) error {
	userID, err := localUserID(ctx, app, username)
	if err != nil {
		return err
	}
	username = strings.TrimPrefix(username, "@")
	in := appidentity.RoleChangeInput{UserID: userID, RoleKey: key}
	change, done, unchanged := app.Identity.GrantRole, "granted the role "+key+" to", "already holds"
	if sub == "revoke" {
		change, done, unchanged = app.Identity.RevokeRole, "revoked the role "+key+" from", "does not hold"
	}
	changed, err := change(ctx, in)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return fmt.Errorf("no role has the key %q; my admin role list shows the roles", key)
	case errors.Is(err, appidentity.ErrLastAdmin):
		return fmt.Errorf("@%s is the only admin; grant the admin role to another user first", username)
	case err != nil:
		return err
	case !changed:
		_, err = fmt.Fprintf(stdout, "@%s %s the role %s; nothing was changed\n", username, unchanged, key)
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s @%s (user %s)\n", done, username, userID)
	return err
}

// listRoles prints every role with its permissions and holders, or, for a username, the roles
// of that user.
func listRoles(ctx context.Context, app *platform.App, w io.Writer, username string) error {
	roles, err := app.Identity.Roles(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if username != "" {
		userID, err := localUserID(ctx, app, username)
		if err != nil {
			return err
		}
		held, err := app.Identity.UserRoles(ctx, userID)
		if err != nil {
			return err
		}
		ids := map[string]bool{}
		for _, r := range held {
			ids[r.ID] = true
		}
		_, _ = fmt.Fprintln(tw, "ROLE\tPOSITION\tMODERATION\tSERVER\tMEMBER")
		for _, r := range roles {
			if ids[r.ID] {
				_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", r.Key, r.Position, permissionList(r.Permissions, identity.CategoryModeration),
					permissionList(r.Permissions, identity.CategoryServer), permissionList(r.Permissions, identity.CategoryMember))
			}
		}
		return tw.Flush()
	}
	_, _ = fmt.Fprintln(tw, "ROLE\tPOSITION\tMODERATION\tSERVER\tMEMBER\tHOLDERS")
	for _, r := range roles {
		holders, err := app.Identity.RoleHolders(ctx, r.Key)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(holders))
		for _, id := range holders {
			la, err := app.Accounts.LocalAccountOfUser(ctx, id)
			switch {
			case err == nil:
				names = append(names, "@"+la.Username)
			case errors.Is(err, appaccount.ErrNotFound):
				names = append(names, id)
			default:
				return err
			}
		}
		list := strings.Join(names, ",")
		if list == "" {
			list = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n", r.Key, r.Position,
			permissionList(r.Permissions, identity.CategoryModeration), permissionList(r.Permissions, identity.CategoryServer),
			permissionList(r.Permissions, identity.CategoryMember), list)
	}
	return tw.Flush()
}

// permissionList joins the permissions of perms in category (ADR-0058), or "-" for none.
func permissionList(perms []identity.Permission, category identity.PermissionCategory) string {
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if p.Category() == category {
			out = append(out, string(p))
		}
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, ",")
}

func adminRoleUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin role list [-user <username>] [-config <file>]
  my admin role grant [-config <file>] <username> <role>
  my admin role revoke [-config <file>] <username> <role>

The roles of local users. list shows every role, highest position
first, with the permissions it grants (moderation and server administration
apart) and the users who hold it, or with -user
the roles of one user. grant and revoke change one role of one user; the
change applies from the user's next request and is recorded in the audit log
(my admin audit list -target-kind role). Granting a role the user holds, or
revoking one they do not hold, changes nothing. The only holder of the admin
role cannot lose it.

The built-in roles are admin (position 1000, every permission) and moderator
(position 100: manage_reports, manage_accounts, view_audit).
`)
}
