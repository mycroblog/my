// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/mycroblog/my/internal/adapter/webpush"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// runConfig implements `my config <subcommand>`.
func runConfig(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 0:
		configUsage(stderr)
		return 2
	case args[0] == "-h" || args[0] == "-help" || args[0] == "--help":
		configUsage(stderr)
		return 0
	case args[0] == "vapid-key":
		return runVAPIDKey(args[1:], stdout, stderr)
	case args[0] != "check":
		_, _ = fmt.Fprintf(stderr, "my config: unknown subcommand %q\n\n", args[0])
		configUsage(stderr)
		return 2
	}
	cfs := flag.NewFlagSet("my config check", flag.ContinueOnError)
	cfs.SetOutput(stderr)
	file := cfs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")
	maint := cfs.Bool("maintenance", false, "check only the settings that the maintenance mode reads")
	inst := cfs.Bool("instance", false, "also check the instance settings in the database, when it can be reached")
	if err := cfs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if cfs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "my config check: unexpected argument %q\n", cfs.Arg(0))
		return 2
	}
	if *maint && *inst {
		_, _ = fmt.Fprintln(stderr, "my config check: -maintenance and -instance cannot be used together")
		return 2
	}

	load := config.Load
	if *maint {
		load = config.LoadMaintenance
	}
	src := config.Source{File: *file}
	cfg, settings, err := load(src)
	for _, name := range config.UnknownEnv(src) {
		_, _ = fmt.Fprintf(stderr, "warning: %s\n", unknownEnvMessage(name))
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tSOURCE")
	for _, s := range settings {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Key, s.Value, s.Origin)
	}
	_ = tw.Flush()
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	if *maint {
		_, _ = fmt.Fprintln(stdout, "configuration OK")
		return 0
	}
	findings := config.Readiness(cfg)
	if *inst {
		findings = append(findings, instanceFindings(context.Background(), cfg)...)
	}
	printReadiness(stderr, findings, os.LookupEnv)
	if slices.ContainsFunc(findings, func(f config.Finding) bool { return f.Severity == config.SeverityError }) {
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "configuration OK")
	return 0
}

// instanceFindings checks the instance settings that a production instance should set, warning
// about each one that is unset. A database that cannot be reached is a warning too, since the
// configuration itself may be fine before the database exists.
func instanceFindings(ctx context.Context, cfg *config.Config) []config.Finding {
	warn := func(format string, args ...any) config.Finding {
		return config.Finding{Severity: config.SeverityWarning, Message: fmt.Sprintf(format, args...)}
	}
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		return []config.Finding{warn("the instance settings were not checked: %v", err)}
	}
	defer func() { _ = app.Close() }()
	settings, err := app.Instance.Settings(ctx)
	if err != nil {
		return []config.Finding{warn("the instance settings were not checked: %v", err)}
	}
	var findings []config.Finding
	for _, t := range []struct {
		key  instance.TextKey
		what string
	}{
		{instance.TextName, "the server's name shown on its pages"},
		{instance.TextShortDescription, "the summary of the server in its information and link previews"},
		{instance.TextTerms, "the terms of service"},
		{instance.TextPrivacyPolicy, "the privacy policy"},
	} {
		text, ok, err := app.Instance.Text(ctx, t.key)
		if err != nil {
			return append(findings, warn("the instance settings were not checked: %v", err))
		}
		if !ok || text.Body == "" {
			findings = append(findings, warn("the instance text %s (%s) is not set", t.key, t.what))
		}
	}
	for _, s := range []struct {
		key  instance.SettingKey
		what string
	}{
		{instance.SettingContactAccount, "the account that users contact about the server"},
		{instance.SettingContactEmail, "the address that users and other servers write to"},
		{instance.SettingSecurityContact, "where to report vulnerabilities, published in /.well-known/security.txt"},
	} {
		if v, _ := settings.Value(s.key); v == "" {
			findings = append(findings, warn("the instance setting %s (%s) is not set", s.key, s.what))
		}
	}
	rules, err := app.Instance.Rules(ctx, false)
	if err != nil {
		return append(findings, warn("the instance settings were not checked: %v", err))
	}
	if len(rules) == 0 {
		findings = append(findings, warn("the server has no rules; add them with `my instance rule add`"))
	}
	return findings
}

func configUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: my config check [-config <file>] [-maintenance | -instance]
       my config vapid-key

check validates the configuration from the environment, the TOML file, and
defaults, listing every problem at once. Exits 1 if the configuration is invalid.
A renamed or removed setting is a problem that names its replacement. An
environment variable that starts with MY_ but sets nothing is ignored with a
warning.
In the production profile it then checks that the valid settings suit a
production instance (docs/operations/production-checklist.md). Errors, such as
a host that other servers cannot reach or a key-encryption key that is not
random, exit 1; warnings, such as mail = none, do not. my serve logs them at
start without refusing to start.
With -instance it also opens the database and warns about the instance
settings that are not set: the name, short description, terms, privacy policy,
contacts, and rules.
With -maintenance it checks only the settings that the maintenance mode reads
(ADR-0084), leaving the adapters' settings unread.

vapid-key generates a key pair for Web Push and prints the private key, the
value of MY_VAPID_PRIVATE_KEY, and the public key that browsers subscribe with.
`)
}

// runVAPIDKey implements `my config vapid-key`: it prints a new VAPID key pair (RFC 8292).
func runVAPIDKey(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		_, _ = fmt.Fprintf(stderr, "my config vapid-key: unexpected argument %q\n", args[0])
		return 2
	}
	priv, pub, err := webpush.GenerateKey()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "my config vapid-key: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "vapid_private_key = %s\nvapid_public_key  = %s\n", priv, pub)
	return 0
}
