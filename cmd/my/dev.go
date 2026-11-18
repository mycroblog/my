// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// my dev runs the fakes that the end-to-end tests and the demonstration start beside my serve
// (ADR-0015 §9). Only the test builds with the build tag e2e_fakes have it: scripts/go-fakes.sh
// fails when cmd/my without the tag depends on a fake, the Dockerfile refuses the tag, and
// scripts/check-release-build.sh refuses a binary built with it.

func init() {
	testBuildCommands["dev"] = runDev
	testBuildUsage = `
Commands of this test build (build tag e2e_fakes), never in a released binary:
  dev       Run the fakes of the end-to-end tests and the demonstration
            (my dev push-service|oidc-provider)
`
}

// runDev implements `my dev <subcommand>`.
func runDev(args []string, stdout, stderr io.Writer) int {
	usage := func() {
		_, _ = fmt.Fprint(stderr, `Usage: my dev push-service|oidc-provider [flags]

Subcommands (test builds only):
  push-service   Run a fake Web Push service over HTTPS on this machine
  oidc-provider  Run a fake OpenID Connect provider over HTTPS on this machine
`)
	}
	var serve func(ctx context.Context, args []string, stdout, stderr io.Writer) int
	switch {
	case len(args) == 0:
		usage()
		return 2
	case args[0] == "-h" || args[0] == "-help" || args[0] == "--help":
		usage()
		return 0
	case args[0] == "push-service":
		serve = runDevPushService
	case args[0] == "oidc-provider":
		serve = runDevOIDCProvider
	default:
		_, _ = fmt.Fprintf(stderr, "my dev: unknown subcommand %q\n\n", args[0])
		usage()
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, args[1:], stdout, stderr)
}

// devFake is one fake of my dev: an HTTPS server on this machine that refuses every profile but
// development.
type devFake struct {
	// name is the command, such as "my dev push-service".
	name string
	// what tells what the fake is, and risk why only the development profile may run it.
	what, risk string
	// originFlag is the name of the flag of the origin, and originUsage its usage.
	originFlag, originUsage string
	// listen is the default address, and cert the default certificate's name in .tmp/mailpit.
	listen, cert string
	// flags defines the fake's own flags.
	flags func(fs *flag.FlagSet)
	// handler returns the fake's handler for its origin.
	handler func(origin string) (http.Handler, error)
}

// run parses args and serves the fake until ctx ends.
func (f devFake) run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet(f.name)
	fs.SetOutput(stderr)
	file := fs.String("config", "", "TOML config file whose profile must be development (default: $"+config.EnvConfigFile+")")
	listen := fs.String("listen", f.listen, "address to listen on")
	origin := fs.String(f.originFlag, "", f.originUsage+" (default: https://localhost:<port of -listen>)")
	certFile := fs.String("cert", ".tmp/mailpit/"+f.cert+".pem", "TLS certificate (scripts/mailpit-cert.sh writes it)")
	keyFile := fs.String("key", ".tmp/mailpit/"+f.cert+".key", "TLS private key of -cert")
	if f.flags != nil {
		f.flags(fs)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: unexpected argument %q\n", f.name, fs.Arg(0))
		return 2
	}
	profile, err := config.ProfileRequested(config.Source{File: *file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	if profile != "development" {
		_, _ = fmt.Fprintf(stderr, "%s: needs profile = development: it is a fake for tests and %s\n", f.name, f.risk)
		return 1
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	logger.WarnContext(ctx, fmt.Sprintf("this is a test build (build tag e2e_fakes): %s is %s for tests", f.name, f.what))

	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: the TLS certificate: %v (run scripts/mailpit-cert.sh)\n", f.name, err)
		return 1
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *listen)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", f.name, err)
		return 1
	}
	if *origin == "" {
		*origin = "https://localhost:" + fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	}
	handler, err := f.handler(*origin)
	if err != nil {
		_ = ln.Close()
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", f.name, err)
		return 2
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	done := make(chan error, 1)
	kernel.Go(ctx, "cli.dev.serve", nil, func(context.Context) { done <- srv.ServeTLS(ln, "", "") })
	_, _ = fmt.Fprintf(stdout, "%s: listening on %s (%s %s)\n", f.name, ln.Addr(), f.originFlag, *origin)
	select {
	case err := <-done:
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", f.name, err)
		return 1
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", f.name, err)
		return 1
	}
	return 0
}
