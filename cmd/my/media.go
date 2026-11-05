// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

// runAdminMedia implements `my admin media <subcommand>`: the processing of uploaded images again
// after the media settings change (ADR-0005 §7).
func runAdminMedia(args []string, stdout, stderr io.Writer) int {
	if _, code := dispatch("admin media", args, []string{"reprocess"}, stderr, adminMediaUsage); code >= 0 || len(args) == 0 {
		return max(code, 0)
	}
	var beforeText *string
	c, code := parseSubcommand("my admin media reprocess", args[1:], 0, stderr, adminMediaUsage, func(fs *flag.FlagSet) {
		beforeText = fs.String("before", "", "reprocess the images last processed before this time, in RFC 3339 (required)")
	})
	if code >= 0 {
		return code
	}
	before, err := time.Parse(time.RFC3339, *beforeText)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: -before wants a time such as 2027-01-31T09:00:00Z, got %q\n\n", c.name, *beforeText)
		adminMediaUsage(stderr)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	if err := app.Media.RequestReprocess(ctx, before); err != nil {
		if errors.Is(err, appcontent.ErrReprocessBeforeInFuture) {
			_, _ = fmt.Fprintf(stderr, "%s: -before must be a time that has passed: images processed from now on would be processed again\n", c.name)
			return 2
		}
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "queued the processing of the images last processed before %s (%d per minute)\n", before.UTC().Format(time.RFC3339), appcontent.ReprocessBatch)
	return 0
}

func adminMediaUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin media reprocess [-config <file>] -before <time>

Queues the processing again, with the current settings, of the uploaded images
last processed before the time (RFC 3339, such as 2027-01-31T09:00:00Z). Use the
time when the media settings changed. The worker takes the images in batches a
minute apart and processes each one in a job of its own.

Running it again with the same time is safe: an image processed since is left
alone, and the images the first run did not reach are picked up. Originals are
not kept, so each processing re-encodes the large version: it cannot bring back
detail, and every run with a later time loses a little quality. Video and audio
files are never re-encoded and are left alone.

An image's new version has new URLs. Its old files stay for
media_replaced_retention, and are deleted by the hourly cleanup.
`)
}
