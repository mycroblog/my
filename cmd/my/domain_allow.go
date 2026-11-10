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

	appfederation "github.com/mycroblog/my/internal/app/federation"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// runAdminDomainAllow implements `my admin domain allow <subcommand>`: the allowlist of
// federation (ADR-0159). The CLI acts as no user: the allows it adds record no moderator.
func runAdminDomainAllow(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("admin domain allow", args, []string{"list", "add", "remove"}, stderr, adminDomainAllowUsage)
	if code >= 0 || len(args) == 0 {
		return max(code, 0)
	}
	var comment string
	nargs := 1
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin domain allow "+sub, args[1:], nargs, stderr, adminDomainAllowUsage, func(fs *flag.FlagSet) {
		if sub == "add" {
			fs.StringVar(&comment, "comment", "", "a note for the moderators, never published")
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
	case "list":
		var allows []moderation.DomainAllow
		if allows, err = app.Moderation.DomainAllows(ctx); err == nil {
			err = printDomainAllows(stdout, allows)
		}
	case "add":
		var host string
		if host, err = appfederation.CanonicalHost(c.fs.Arg(0)); err != nil {
			break
		}
		var created bool
		if _, created, err = app.Moderation.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: host, Comment: comment}); err != nil {
			break
		}
		if created {
			_, _ = fmt.Fprintf(stdout, "allowed %s and its subdomains\n", host)
		} else {
			_, _ = fmt.Fprintf(stdout, "%s is already allowed\n", host)
		}
	case "remove":
		var host string
		if host, err = appfederation.CanonicalHost(c.fs.Arg(0)); err != nil {
			break
		}
		var removed bool
		if removed, err = app.Moderation.RemoveDomainAllow(ctx, host, ""); err != nil {
			break
		}
		if removed {
			_, _ = fmt.Fprintf(stdout, "removed the allow of %s\n", host)
		} else {
			_, _ = fmt.Fprintf(stdout, "%s has no allow\n", host)
		}
	}
	if errors.Is(err, appmoderation.ErrTooManyDomainAllows) {
		err = fmt.Errorf("the allowlist holds at most %d domains", moderation.MaxDomainAllows)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

func printDomainAllows(w io.Writer, allows []moderation.DomainAllow) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tALLOWED\tBY\tCOMMENT")
	for _, a := range allows {
		by := a.CreatedByUserID
		if by == "" {
			by = "cli"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Domain, a.CreatedAt.Format(time.RFC3339), by, oneLine(a.Comment))
	}
	return tw.Flush()
}

func adminDomainAllowUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin domain allow list [-config <file>]
  my admin domain allow add [-comment <text>] [-config <file>] <domain>
  my admin domain allow remove [-config <file>] <domain>

The allowlist of federation: while the instance setting federation_scope is
allowlist, the server federates only with the allowed domains and their
subdomains, and treats every other server as suspended. The domain actions on
an allowed domain still apply. While federation_scope is open, the allowlist
changes nothing, and it is kept for the next switch.

add allows a domain and its subdomains; a domain already allowed keeps its
allow. remove takes a domain off the list; while federation_scope is allowlist
and no parent domain is allowed, the domain is cut off at once and the follows
between the local accounts and its accounts end for good, each local user who
loses one being notified. The domain may be given in U-labels (例え.jp); it is
stored in IDNA A-labels. The allowlist holds at most 5000 domains. Every
change is recorded in the audit log.
`)
}

// runAdminFederationScope implements `my admin federation scope`: it shows the instance setting
// federation_scope and, given a value, changes it. Switching to allowlist shows what it would end
// and changes nothing without -apply (ADR-0159 §3).
func runAdminFederationScope(args []string, stdout, stderr io.Writer) int {
	var apply bool
	c, code := parseSubcommand("my admin federation scope", args, -1, stderr, adminFederationScopeUsage, func(fs *flag.FlagSet) {
		fs.BoolVar(&apply, "apply", false, "switch to allowlist after showing what it ends")
	})
	if code >= 0 {
		return code
	}
	value := instance.FederationScope(c.fs.Arg(0))
	switch value {
	case "", instance.FederationScopeOpen, instance.FederationScopeAllowlist:
	default:
		_, _ = fmt.Fprintf(stderr, "%s: the scope must be open or allowlist\n", c.name)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	current, err := app.Instance.FederationScope(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	if value == current {
		_, _ = fmt.Fprintf(stdout, "federation_scope is already %s\n", current)
		return 0
	}
	switch value {
	case "":
		_, _ = fmt.Fprintf(stdout, "federation_scope is %s\n", current)
		return 0
	case instance.FederationScopeAllowlist:
		impact, err := app.Moderation.FederationScopeImpact(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
			return 1
		}
		printScopeImpact(stdout, impact)
		if !apply {
			_, _ = fmt.Fprintln(stdout, "nothing changed; run again with -apply to switch to allowlist")
			return 0
		}
	}
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingFederationScope, Value: string(value)}); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "federation_scope is now %s\n", value)
	return 0
}

func printScopeImpact(w io.Writer, impact appmoderation.ScopeImpact) {
	_, _ = fmt.Fprintf(w, "switching to allowlist ends %d follows of %d local accounts with %d servers\n",
		impact.Follows, impact.LocalAccounts, impact.Domains)
	if len(impact.TopDomains) == 0 {
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tFOLLOWS")
	for _, d := range impact.TopDomains {
		_, _ = fmt.Fprintf(tw, "%s\t%d\n", d.Domain, d.Follows)
	}
	_ = tw.Flush()
}

func adminFederationScopeUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin federation scope [-apply] [-config <file>] [open|allowlist]

The instance setting federation_scope: open federates with every server that no
domain action suspends, allowlist only with the domains of the allowlist (my
admin domain allow) and their subdomains, whose domain actions still apply.
While it is allowlist, ActivityPub GETs need a signature from an allowed
server, whatever authorized_fetch says.

Without a value, scope prints the setting. Switching to allowlist first prints
how many follows between the local accounts and the servers that the allowlist
does not allow it would end, and the servers with the most; nothing changes
without -apply. The switch ends those follows for good, notifying each local
user who loses one; switching back to open restores none of them. Switching to
open needs no -apply. The change is recorded in the audit log.
`)
}
