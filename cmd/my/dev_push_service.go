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

	pushfake "github.com/mycroblog/my/internal/adapter/pushservice/fake"
	"github.com/mycroblog/my/internal/platform/config"
)

// This file is the only one outside the tests that imports the fake push service, and only the
// test builds with the build tag e2e_fakes have it. scripts/go-build-tags.sh fails when cmd/my
// without the tag depends on the fake, the Dockerfile refuses the tag, and
// scripts/check-release-build.sh refuses a binary built with it.

func init() {
	testBuildCommands["dev"] = runDev
	testBuildUsage = `
Commands of this test build (build tag e2e_fakes), never in a released binary:
  dev       Run the fakes of the end-to-end tests and the demonstration
            (my dev push-service)
`
}

// runDev implements `my dev <subcommand>`.
func runDev(args []string, stdout, stderr io.Writer) int {
	usage := func() {
		_, _ = fmt.Fprint(stderr, `Usage: my dev push-service [flags]

Subcommands (test builds only):
  push-service  Run a fake Web Push service over HTTPS on this machine
`)
	}
	switch {
	case len(args) == 0:
		usage()
		return 2
	case args[0] == "-h" || args[0] == "-help" || args[0] == "--help":
		usage()
		return 0
	case args[0] != "push-service":
		_, _ = fmt.Fprintf(stderr, "my dev: unknown subcommand %q\n\n", args[0])
		usage()
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runDevPushService(ctx, args[1:], stdout, stderr)
}

// runDevPushService implements `my dev push-service`: it serves the fake push service until ctx
// ends. It refuses every profile but development.
func runDevPushService(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("my dev push-service", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("config", "", "TOML config file whose profile must be development (default: $"+config.EnvConfigFile+")")
	listen := fs.String("listen", "127.0.0.1:8930", "address to listen on")
	origin := fs.String("origin", "", "origin of the endpoints and of the VAPID audience (default: https://localhost:<port of -listen>)")
	certFile := fs.String("cert", ".tmp/mailpit/push-service.pem", "TLS certificate (scripts/mailpit-cert.sh writes it)")
	keyFile := fs.String("key", ".tmp/mailpit/push-service.key", "TLS private key of -cert")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "my dev push-service: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	profile, err := config.ProfileRequested(config.Source{File: *file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	if profile != "development" {
		_, _ = fmt.Fprintln(stderr, "my dev push-service: needs profile = development: it is a fake for tests and accepts any push message")
		return 1
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	logger.WarnContext(ctx, "this is a test build (build tag e2e_fakes): my dev push-service is a fake Web Push service for tests")

	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "my dev push-service: the TLS certificate: %v (run scripts/mailpit-cert.sh)\n", err)
		return 1
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *listen)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "my dev push-service: %v\n", err)
		return 1
	}
	if *origin == "" {
		*origin = "https://localhost:" + fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	}
	svc, err := pushfake.New(pushfake.Options{Origin: *origin})
	if err != nil {
		_ = ln.Close()
		_, _ = fmt.Fprintf(stderr, "my dev push-service: %v\n", err)
		return 2
	}
	srv := &http.Server{
		Handler:           svc.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	done := make(chan error, 1)
	go func() { done <- srv.ServeTLS(ln, "", "") }()
	_, _ = fmt.Fprintf(stdout, "my dev push-service: listening on %s (origin %s)\n", ln.Addr(), *origin)
	select {
	case err := <-done:
		_, _ = fmt.Fprintf(stderr, "my dev push-service: %v\n", err)
		return 1
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		_, _ = fmt.Fprintf(stderr, "my dev push-service: %v\n", err)
		return 1
	}
	return 0
}
