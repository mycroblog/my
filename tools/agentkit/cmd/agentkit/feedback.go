// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/feedback"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

const feedbackUsage = `usage: agentkit feedback wait|show|reply (ADR-0168)

The decider writes notes on the page of the dev server (scripts/task.sh web:catalog) and sends them as one batch,
which the dev server writes to .tmp/feedback/<batch>.json.

  feedback wait [--timeout 0] [--interval 500ms]
        block until a batch arrives that no wait has delivered, print it, and mark it delivered; run it in the
        background, so that a send notifies the session, and again after each batch
  feedback show [<batch>|<note>]
        print a batch (default: the newest) or one note: its source, layer, state, and accessibility outline
  feedback reply <note> [--kind fixed|question] <text>
        answer a note (<batch>.<n>, or <n> in the newest batch); the overlay shows the answer next to it`

func runFeedback(r *repo.Repo, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(feedbackUsage)
		return nil
	}
	dir := feedback.Dir(r.Root)
	switch args[0] {
	case "wait":
		fs := flag.NewFlagSet("feedback wait", flag.ContinueOnError)
		timeout := fs.Duration("timeout", 0, "give up after (0: never)")
		interval := fs.Duration("interval", 500*time.Millisecond, "poll interval")
		if _, err := parse(fs, args[1:]); err != nil {
			return err
		}
		ctx := context.Background()
		if *timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, *timeout)
			defer cancel()
		}
		b, err := feedback.Wait(ctx, dir, *interval, time.Now)
		if err != nil {
			if ctx.Err() != nil {
				return exitError{124, "feedback wait: no batch arrived before the timeout"}
			}
			return err
		}
		feedback.Print(os.Stdout, b)
		return nil
	case "show":
		if len(args) > 2 {
			return fmt.Errorf("%s", feedbackUsage)
		}
		var b *feedback.Batch
		var err error
		ref := ""
		if len(args) == 2 {
			ref = args[1]
		}
		batch, _, isNote := strings.Cut(ref, ".")
		if ref == "" {
			b, err = feedback.Latest(dir)
		} else {
			b, err = feedback.Load(dir, batch)
		}
		if err != nil {
			return err
		}
		if !isNote {
			feedback.Print(os.Stdout, b)
			return nil
		}
		for i := range b.Notes {
			if b.Notes[i].ID == ref {
				feedback.PrintNote(os.Stdout, &b.Notes[i])
				return nil
			}
		}
		return fmt.Errorf("batch %s has no note %s", batch, ref)
	case "reply":
		fs := flag.NewFlagSet("feedback reply", flag.ContinueOnError)
		kind := fs.String("kind", "fixed", "fixed or question")
		pos, err := parse(fs, args[1:])
		if err != nil || len(pos) < 2 {
			return fmt.Errorf("usage: agentkit feedback reply <note> [--kind fixed|question] <text>")
		}
		if !slices.Contains(feedback.Kinds, *kind) {
			return fmt.Errorf("--kind %q: expected one of %s", *kind, strings.Join(feedback.Kinds, ", "))
		}
		id, err := feedback.Answer(dir, pos[0], *kind, strings.Join(pos[1:], " "), time.Now())
		if err != nil {
			return err
		}
		fmt.Printf("answered %s (%s)\n", id, *kind)
		return nil
	}
	return fmt.Errorf("unknown feedback command %q\n%s", args[0], feedbackUsage)
}
