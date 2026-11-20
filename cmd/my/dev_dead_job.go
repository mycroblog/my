// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// runDevDeadJob implements `my dev dead-job`: it enqueues, through the queue of the server's
// database, a job of a real type whose payload its handler cannot read, which the server's worker
// then fails for good at once, its one attempt recorded (ADR-0004). It prints the job's idempotency
// key, by which the admin API finds the job. The end-to-end tests make a failed job with it, as no
// operation of the server fails a job. It refuses every profile but development.
func runDevDeadJob(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	const name = "my dev dead-job"
	fs := newFlagSet(name)
	fs.SetOutput(stderr)
	file := fs.String("config", "", "TOML config file whose profile must be development (default: $"+config.EnvConfigFile+")")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: unexpected argument %q\n", name, fs.Arg(0))
		return 2
	}
	profile, err := config.ProfileRequested(config.Source{File: *file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	if profile != "development" {
		_, _ = fmt.Fprintf(stderr, "%s: needs profile = development: it puts a job that fails into the queue\n", name)
		return 1
	}
	c := subcommand{name: name, fs: fs, file: file}
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var random [8]byte
	_, _ = rand.Read(random[:])
	key := "dev.dead-job:" + hex.EncodeToString(random[:])
	// A JSON string where the handler reads an object: decoding it fails, which fails the job for good.
	err = app.Jobs.Enqueue(ctx, queue.NewJob{
		Type:           notification.JobNotifyPostSubscribers,
		Payload:        json.RawMessage(`"not a post"`),
		IdempotencyKey: key,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, key)
	return 0
}
