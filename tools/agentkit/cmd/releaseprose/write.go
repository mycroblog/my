// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// maxAttempts counts the calls of a release, the first one included.
const maxAttempts = 3

// factsOnlyMarker opens a body with the facts alone. release.yml replaces a draft's body only while it has the marker,
// so that a run again never overwrites what the decider has edited. scripts/release-notes.sh writes the same words.
const factsOnlyMarker = "<!-- release-notes: facts only."

// model is the seam of the call: the release job uses claudeCLI, and the tests a fake.
type model interface {
	complete(ctx context.Context, system, user string) (string, error)
}

// outcome is the body of the draft, and a line for the job's summary.
type outcome struct {
	body, status string
}

// write asks the model for the prose until it passes the check, and otherwise ends in the facts alone. Nothing here
// fails: the tags exist by the time this runs, and a release never stops for its prose.
func write(ctx context.Context, m model, in input, req request, wait time.Duration) outcome {
	user := req.user
	var last string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		prose, err := m.complete(ctx, req.system, user)
		if err != nil {
			last = "the call failed"
			fmt.Fprintf(os.Stderr, "attempt %d of %d: the call failed: %v\n", attempt, maxAttempts, err)
			if attempt < maxAttempts {
				time.Sleep(wait)
			}
			continue
		}
		problems := check(prose, in)
		if len(problems) == 0 {
			return outcome{
				body:   withProse(prose, in, req),
				status: fmt.Sprintf("The prose passed the check on attempt %d of %d. %s.", attempt, maxAttempts, req.stats),
			}
		}
		last = "the check failed: " + strings.Join(problems, "; ")
		fmt.Fprintf(os.Stderr, "attempt %d of %d: %s\n", attempt, maxAttempts, last)
		user = req.user + "\n<rejected>\nA program rejected your previous answer. Write the whole answer again, with these problems fixed:\n- " +
			strings.Join(problems, "\n- ") + "\n</rejected>\n"
	}
	reason := fmt.Sprintf("all %d attempts at the prose failed, the last because %s", maxAttempts, last)
	return outcome{body: factsOnly(reason, in), status: "The draft has the facts alone: " + reason + "."}
}

func withProse(prose string, in input, req request) string {
	return fmt.Sprintf("<!-- Draft: Claude wrote the three sections before \"Upgrading\" from the facts and the merged PRs, and a program checked that they name every required item and only merged PRs, not that what they say is true. Read them before publishing. The sections from \"Upgrading\" on are the facts, derived from the repository. %s. -->\n\n%s\n\n%s\n\n%s",
		req.stats, strings.TrimSpace(prose), strings.TrimSpace(in.facts), in.changelog)
}

func factsOnly(reason string, in input) string {
	// The reason names required tokens, and "--" in one of them must not end the comment early.
	reason = strings.ReplaceAll(reason, "--", "- -")
	return fmt.Sprintf("%s %s. -->\n\n%s\n\n%s", factsOnlyMarker, reason, strings.TrimSpace(in.facts), in.changelog)
}
