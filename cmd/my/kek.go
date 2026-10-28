// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/rewrap"
)

// runKEK implements `my kek <subcommand>`: count the stored secrets by the key-encryption key
// that wraps them, and re-wrap them under the current one after a rotation (ADR-0019 §6).
func runKEK(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("kek", args, []string{"status", "rewrap"}, stderr, kekUsage)
	if code >= 0 {
		return code
	}
	c, code := parseSubcommand("my kek "+sub, args[1:], 0, stderr, kekUsage, nil)
	if code >= 0 {
		return code
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	if sub == "status" {
		return kekStatus(ctx, app.Rewrap, c.name, stdout, stderr)
	}
	return kekRewrap(ctx, app.Rewrap, c.name, stdout, stderr)
}

// kekStatus prints how many secrets of each column each KEK wraps.
func kekStatus(ctx context.Context, svc *rewrap.Service, name string, stdout, stderr io.Writer) int {
	st, err := svc.Status(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "COLUMN\tKEY ID\tSECRETS")
	outdated := 0
	for _, c := range st {
		for _, k := range c.Keys {
			id := k.KeyID
			if k.Current {
				id += " (current)"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\n", c.Column, id, k.Count)
		}
		if c.Malformed > 0 {
			_, _ = fmt.Fprintf(tw, "%s\t(unreadable)\t%d\n", c.Column, c.Malformed)
		}
		outdated += c.Outdated()
	}
	_ = tw.Flush()
	if outdated > 0 {
		_, _ = fmt.Fprintf(stdout, "%d secret(s) are not wrapped with the current KEK (%s); run my kek rewrap\n", outdated, svc.CurrentKeyID())
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "every secret is wrapped with the current KEK (%s); the database no longer needs kek_previous\n", svc.CurrentKeyID())
	return 0
}

// kekRewrap re-wraps the secrets under the current KEK and reports each column. It exits with 1
// when a secret could not be re-wrapped or the database failed.
func kekRewrap(ctx context.Context, svc *rewrap.Service, name string, stdout, stderr io.Writer) int {
	failed := 0
	_, err := svc.Rewrap(ctx, func(r rewrap.ColumnReport) {
		_, _ = fmt.Fprintf(stdout, "%s: %d rewrapped, %d already current", r.Column, r.Rewrapped, r.Current)
		if r.Changed > 0 {
			_, _ = fmt.Fprintf(stdout, ", %d rewritten by the server meanwhile", r.Changed)
		}
		if r.Failed > 0 {
			_, _ = fmt.Fprintf(stdout, ", %d failed", r.Failed)
		}
		_, _ = fmt.Fprintln(stdout)
		failed += r.Failed
		for _, f := range r.Failures {
			_, _ = fmt.Fprintf(stderr, "%s: %s row %s: %s\n", name, r.Column, f.RowID, describeRewrapFailure(f))
		}
		if n := r.Failed - len(r.Failures); n > 0 {
			_, _ = fmt.Fprintf(stderr, "%s: %s: %d more failed\n", name, r.Column, n)
		}
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v; nothing is lost: run it again to resume\n", name, err)
		return 1
	}
	if failed > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: %d secret(s) were not rewrapped and still need their KEK; keep it in kek_previous\n", name, failed)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "every secret is wrapped with the current KEK (%s)\n", svc.CurrentKeyID())
	return 0
}

// describeRewrapFailure explains why a secret was not re-wrapped.
func describeRewrapFailure(f rewrap.Failure) string {
	switch {
	case f.KeyID == "":
		return "the stored value is not a sealed secret"
	case errors.Is(f.Err, kernel.ErrUnknownKey):
		return fmt.Sprintf("it is wrapped with the KEK %s, which is neither kek nor in kek_previous", f.KeyID)
	default:
		return fmt.Sprintf("it does not decrypt with the KEK %s: it was altered or moved from another row", f.KeyID)
	}
}

func kekUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my kek status [-config <file>]
  my kek rewrap [-config <file>]

Manages the secrets that the database stores encrypted with the key-encryption
key (KEK), such as the private keys of local actors.

status counts the stored secrets by the KEK that wraps them, by its key ID,
without decrypting anything.

rewrap re-wraps every secret that a KEK of kek_previous wraps with the current
kek, keeping the encrypted secret as it is. Run it after restarting every
process with the new kek and the old one in kek_previous. It works in small
batches and can be stopped and run again at any time: it skips the secrets
that are already wrapped with the current KEK. A secret that it cannot
re-wrap is left as it is and reported, and the command exits with 1. Once it
exits with 0, no stored secret needs the keys of kek_previous.
`)
}
