// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/mycroblog/my/internal/adapter/webpush"
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

	load := config.Load
	if *maint {
		load = config.LoadMaintenance
	}
	src := config.Source{File: *file}
	_, settings, err := load(src)
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
	_, _ = fmt.Fprintln(stdout, "configuration OK")
	return 0
}

func configUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: my config check [-config <file>] [-maintenance]
       my config vapid-key

check validates the configuration from the environment, the TOML file, and
defaults, listing every problem at once. Exits 1 if the configuration is invalid.
A renamed or removed setting is a problem that names its replacement. An
environment variable that starts with MY_ but sets nothing is ignored with a
warning.
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
