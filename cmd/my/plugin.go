// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/contract"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
)

// runPlugin implements `my plugin <subcommand>`.
func runPlugin(args []string, stdout, stderr io.Writer) int {
	if _, code := dispatch("plugin", args, []string{"test"}, stderr, pluginUsage); code >= 0 {
		return code
	}
	return pluginTest(args[1:], stdout, stderr)
}

// pluginTest implements `my plugin test`: it runs the contract cases of a kind against a running plugin and reports each
// one. It reads no configuration, so that a plugin's author needs nothing but the binary (ADR-0146 §16).
func pluginTest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("my plugin test", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { pluginUsage(stderr) }
	name := fs.String("kind", "", "the kind of the plugin: "+strings.Join(kind.Names(), ", "))
	var d kind.Destination
	fs.StringVar(&d.Protocol, "protocol", "", "json, proto, or grpc (default: the kind's)")
	tokenFile := fs.String("token-file", "", "a file that holds the bearer token")
	fs.StringVar(&d.TLSCAFile, "tls-ca-file", "", "verify the plugin against this CA (default: the system's roots)")
	fs.StringVar(&d.TLSCertFile, "tls-cert-file", "", "the client certificate of mTLS")
	fs.StringVar(&d.TLSKeyFile, "tls-key-file", "", "the key of the client certificate")
	fs.DurationVar(&d.Timeout, "timeout", 0, "the deadline of one call (default: the kind's)")
	billed := fs.Bool("billed", false, "--kind billing: also run the case that may cost money at the plugin's providers")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "my plugin test: want the address of the plugin, got %d argument(s)\n\n", fs.NArg())
		pluginUsage(stderr)
		return 2
	}
	k, ok := kind.ByName(*name)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "my plugin test: --kind must be one of %s\n", strings.Join(kind.Names(), ", "))
		return 2
	}
	d.Address = fs.Arg(0)
	if *tokenFile != "" {
		token, err := os.ReadFile(*tokenFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "my plugin test: read the token: %v\n", err)
			return 1
		}
		d.Token = strings.TrimSpace(string(token))
	}
	for _, p := range k.Check(d) {
		_, _ = fmt.Fprintf(stderr, "my plugin test: %s %s\n", p.Suffix, p.Message)
	}
	if len(k.Check(d)) > 0 {
		return 1
	}
	for _, p := range k.Warnings(d) {
		_, _ = fmt.Fprintf(stderr, "warning: %s %s\n", p.Suffix, p.Message)
	}
	conn, err := client.New(client.Options{Kind: k, Destination: d, KeepDetail: true})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "my plugin test: %v\n", err)
		return 1
	}
	defer conn.Close()

	protocol := d.Protocol
	if protocol == "" {
		protocol = k.Protocol
	}
	counts := map[contract.Outcome]int{}
	results := contract.Run(context.Background(), k, contract.Env{Conn: conn, Protocol: protocol, Billed: *billed}, func(r contract.Result) {
		counts[r.Outcome]++
		line := string(r.Outcome) + " " + r.Case
		if r.Detail != "" {
			line += ": " + r.Detail
		}
		_, _ = fmt.Fprintln(stdout, line)
	})
	_, _ = fmt.Fprintf(stdout, "%d passed, %d failed, %d skipped\n", counts[contract.Pass], counts[contract.Fail], counts[contract.Skip])
	if !contract.Passed(results) {
		return 1
	}
	return 0
}

func pluginUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: my plugin test --kind <kind> [flags] <address>

Run the contract cases of a kind against a running plugin, and report PASS, FAIL, or
SKIP per case. Exits 1 when a case fails. The plugin may be a real one: a case with a
side effect beyond the call runs only when a flag asks for it.

<address> is unix:///path/to.sock, https://host[:port], or http://host[:port].

Flags:
  --kind           %s
  --protocol       json, proto, or grpc (default: the kind's)
  --token-file     a file that holds the bearer token (https:// and unix:// only)
  --tls-ca-file    verify the plugin against this CA (default: the system's roots)
  --tls-cert-file  the client certificate of mTLS
  --tls-key-file   the key of the client certificate
  --timeout        the deadline of one call, such as 5s (default: the kind's)
  --billed         --kind billing: also run the case that may cost money at the plugin's
                   providers

The flags mean what the settings plugin_<kind>_* mean, and this command reads no
configuration.
`, strings.Join(kind.Names(), ", "))
}
