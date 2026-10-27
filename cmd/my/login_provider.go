// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform"
)

// maxClientSecretInput bounds the line read for a client secret: well above a valid one.
const maxClientSecretInput = 4 * identity.MaxLoginProviderClientSecret

// loginProviderFlags are the settings flags of `my login-provider add` and `set`.
type loginProviderFlags struct {
	kind, name, issuer, clientID string
	scopes, claims, orgs         stringList
	allowSignup, allowLink       bool
}

func (f *loginProviderFlags) define(fs *flag.FlagSet, add bool) {
	if add {
		fs.StringVar(&f.kind, "kind", "", "oidc, github, or mastodon (required)")
	}
	fs.StringVar(&f.name, "name", "", "the display name")
	fs.StringVar(&f.issuer, "issuer", "", "the OIDC issuer URL, or the Mastodon server's domain")
	fs.StringVar(&f.clientID, "client-id", "", "the client ID that the provider gave")
	fs.Var(&f.scopes, "scope", "a scope to ask for besides the ones the kind needs (repeatable)")
	fs.Var(&f.claims, "claim", "an OIDC condition <claim>=<value> (repeatable)")
	fs.Var(&f.orgs, "org", "a GitHub Organization that users must belong to (repeatable)")
	fs.BoolVar(&f.allowSignup, "allow-signup", false, "let new users sign up with the provider")
	fs.BoolVar(&f.allowLink, "allow-link", true, "let users link their account to the provider")
}

// apply sets on s the settings whose flags are in set.
func (f *loginProviderFlags) apply(s *identity.LoginProviderSettings, set map[string]bool) error {
	if set["name"] {
		s.DisplayName = f.name
	}
	if set["issuer"] {
		s.Issuer = f.issuer
	}
	if set["client-id"] {
		s.ClientID = f.clientID
	}
	if set["scope"] {
		s.Scopes = f.scopes
	}
	if set["org"] {
		s.Organizations = f.orgs
	}
	if set["allow-signup"] {
		s.AllowSignup = f.allowSignup
	}
	if set["allow-link"] {
		s.AllowLink = f.allowLink
	}
	if set["claim"] {
		s.Claims = nil
		for _, c := range f.claims {
			if c == "" {
				continue
			}
			name, value, ok := strings.Cut(c, "=")
			if !ok {
				return fmt.Errorf("-claim %q is not <claim>=<value>", c)
			}
			s.Claims = append(s.Claims, identity.ClaimCondition{Claim: name, Values: []string{value}})
		}
	}
	return nil
}

// runLoginProvider implements `my login-provider <subcommand>`: the external login providers,
// through the same operations as the admin API, for the first setup before an administrator
// can reach the administration screens. The CLI acts as no user: its operations are audited as
// the CLI's. stdin supplies the client secret of add and rotate-secret.
func runLoginProvider(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	subs := []string{"add", "list", "set", "rotate-secret", "enable", "disable", "delete"}
	sub, code := dispatch("login-provider", args, subs, stderr, loginProviderUsage)
	if code >= 0 {
		return code
	}
	var f loginProviderFlags
	nargs := 1
	if sub == "add" || sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my login-provider "+sub, args[1:], nargs, stderr, loginProviderUsage, func(fs *flag.FlagSet) {
		if sub == "add" || sub == "set" {
			f.define(fs, sub == "add")
		}
	})
	if code >= 0 {
		return code
	}
	set := map[string]bool{}
	c.fs.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	settings := identity.NewLoginProviderSettings(identity.LoginProviderKind(f.kind))
	if sub == "add" {
		for _, required := range []string{"kind", "name", "client-id"} {
			// A mastodon provider without a client ID registers a client at its server.
			if !set[required] && (required != "client-id" || settings.Kind != identity.LoginProviderMastodon) {
				_, _ = fmt.Fprintf(stderr, "%s: -%s is required\n", c.name, required)
				return 2
			}
		}
		if err := f.apply(&settings, set); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
			return 2
		}
	}
	if sub == "set" && !hasSettingFlag(set) {
		_, _ = fmt.Fprintf(stderr, "%s: name at least one setting to change\n", c.name)
		return 2
	}
	var secret string
	if sub == "rotate-secret" || sub == "add" && settings.ClientID != "" {
		var err error
		if secret, err = readClientSecret(stdin, stderr); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
			return 1
		}
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	id := c.fs.Arg(0)
	var (
		v   appidentity.LoginProviderView
		err error
	)
	switch sub {
	case "list":
		var vs []appidentity.LoginProviderView
		if vs, err = app.Identity.ListLoginProviders(ctx); err == nil {
			err = printLoginProviders(stdout, app, vs)
		}
	case "add":
		if v, err = app.Identity.CreateLoginProvider(ctx, settings, secret, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "added %s login provider %s (%s)\n", v.Provider.Settings.Kind, v.Provider.ID, v.Provider.Settings.DisplayName)
			_, _ = fmt.Fprintf(stdout, "callback URL: %s\n", loginProviderCallbackURL(app, v.Provider.ID))
		}
	case "set":
		if v, err = app.Identity.LoginProvider(ctx, id); err == nil {
			settings = v.Provider.Settings
			if err = f.apply(&settings, set); err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
				return 2
			}
			if v, err = app.Identity.UpdateLoginProvider(ctx, id, settings, ""); err == nil {
				_, _ = fmt.Fprintf(stdout, "updated login provider %s (%s)\n", id, v.Provider.Settings.DisplayName)
			}
		}
	case "rotate-secret":
		if _, err = app.Identity.RotateLoginProviderSecret(ctx, id, secret, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "replaced the client secret of login provider %s\n", id)
		}
	case "enable":
		if _, err = app.Identity.EnableLoginProvider(ctx, id, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "enabled login provider %s\n", id)
		}
	case "disable":
		if v, err = app.Identity.DisableLoginProvider(ctx, id, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "disabled login provider %s; %d linked user(s) can no longer sign in with it\n", id, v.LinkedUsers)
		}
	case "delete":
		if v, err = app.Identity.DeleteLoginProvider(ctx, id, ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "deleted login provider %s; %d linked user(s) can no longer sign in with it\n", id, v.LinkedUsers)
		}
	}
	if errors.Is(err, appidentity.ErrNotFound) {
		err = fmt.Errorf("no login provider has the ID %q", id)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// hasSettingFlag reports whether set names a flag other than -config.
func hasSettingFlag(set map[string]bool) bool {
	for name := range set {
		if name != "config" {
			return true
		}
	}
	return false
}

func loginProviderCallbackURL(app *platform.App, id string) string {
	return app.Config.ServerOrigin() + identity.LoginProviderCallbackPath(id)
}

// readClientSecret reads a client secret: from a prompt without echo when stdin is a terminal,
// and otherwise from the first line of stdin.
func readClientSecret(stdin io.Reader, stderr io.Writer) (string, error) {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		_, _ = fmt.Fprint(stderr, "Client secret: ")
		b, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(stderr)
		if err != nil {
			return "", fmt.Errorf("read the client secret: %w", err)
		}
		return string(b), nil
	}
	line, err := bufio.NewReaderSize(io.LimitReader(stdin, maxClientSecretInput+1), maxClientSecretInput+1).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read the client secret from standard input: %w", err)
	}
	if len(line) > maxClientSecretInput {
		return "", errors.New("the client secret on standard input is too long")
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" {
		return "", errors.New("no client secret on standard input; pipe it in or run in a terminal to be prompted")
	}
	return line, nil
}

func printLoginProviders(w io.Writer, app *platform.App, vs []appidentity.LoginProviderView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tNAME\tSTATE\tISSUER\tCLIENT ID\tSIGNUP\tLINK\tUSERS\tUPDATED\tCALLBACK URL")
	for _, v := range vs {
		p, s := v.Provider, v.Provider.Settings
		issuer := s.Issuer
		if issuer == "" {
			issuer = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%t\t%t\t%d\t%s\t%s\n", p.ID, s.Kind, oneLine(s.DisplayName), p.State, issuer,
			s.ClientID, s.AllowSignup, s.AllowLink, v.LinkedUsers, p.RevisedAt.Format(time.RFC3339), loginProviderCallbackURL(app, p.ID))
	}
	return tw.Flush()
}

func loginProviderUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my login-provider list [-config <file>]
  my login-provider add -kind oidc|github|mastodon -name <name> [-client-id <id>]
                        [-issuer <url or domain>] [-scope <scope>]... [-claim <claim>=<value>]...
                        [-org <organization>]... [-allow-signup] [-allow-link=false] [-config <file>]
  my login-provider set [-name <name>] [-issuer <url or domain>] [-client-id <id>] [-scope <scope>]...
                        [-claim <claim>=<value>]... [-org <organization>]... [-allow-signup=true|false]
                        [-allow-link=true|false] [-config <file>] <provider id>
  my login-provider rotate-secret [-config <file>] <provider id>
  my login-provider enable|disable|delete [-config <file>] <provider id>

External login providers, which users sign in, sign up, and link their accounts
with. The same settings are on the administration screens.

  oidc      an OpenID Connect provider; -issuer is its issuer URL (https),
            exactly as its ID tokens name it
  github    github.com; no -issuer
  mastodon  one Mastodon server (4.3 or later); -issuer is its domain or
            https://<domain>. Without -client-id, add registers a client at
            the server itself; give -client-id and the secret of a client
            registered there when the server does not let others register.
            set does not move it to another server.

add sets up an enabled provider and prints its callback URL, which you register
at the provider as the redirect URI. -client-id is required but for mastodon.
add with -client-id and rotate-secret read the client secret that the provider
gave from standard input (the first line), or prompt for it in a terminal; it
is never shown again.

New users sign up with a provider only with -allow-signup, and only as the
registration mode allows. Users may link their account to it unless
-allow-link=false. -claim (oidc only) lets in only the users whose ID token has
the claim with one of the values given for it, such as -claim hd=example.com
for a Google Workspace domain. -org (github only) lets in only the members of
one of the Organizations, which needs -scope read:org.

set changes only the settings it names; -scope, -claim, and -org replace the
whole list, and -scope '' (or -claim '', -org '') removes it. The kind does not
change.

disable keeps the provider and its links but lets nobody sign in, sign up, or
link with it until enable; delete removes it for good. Both print how many
linked users can no longer sign in with it. Every change is recorded in the
audit log.
`)
}
