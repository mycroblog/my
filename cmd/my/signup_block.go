// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// maxSignupBlockListInput bounds the list that `my admin signup-block import` reads: well above
// moderation.MaxSignupBlockImport lines of a domain with a comment.
const maxSignupBlockListInput = 8 << 20

// runAdminSignupBlock implements `my admin signup-block <subcommand>`: the sign-up block list
// (FR-719). The CLI acts as no user: the blocks it adds and lifts record no moderator. stdin
// supplies the list of an import given as "-".
func runAdminSignupBlock(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminSignupBlockUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin signup-block", args, []string{"create", "import", "list", "lift"}, stderr, adminSignupBlockUsage)
	if code >= 0 {
		return code
	}
	var (
		kind, severity, comment, before string
		active                          bool
		duration                        time.Duration
		limit                           = 50
	)
	nargs := 1
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin signup-block "+sub, args[1:], nargs, stderr, adminSignupBlockUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "list":
			fs.StringVar(&kind, "kind", "", "show only the blocks of this kind: email_domain, email_hash, or ip_range")
			fs.BoolVar(&active, "active", false, "show only the blocks in effect now")
			fs.StringVar(&before, "before", "", "show the blocks older than this block ID")
			fs.IntVar(&limit, "limit", limit, "number of blocks to show, newest first")
		case "create", "import":
			fs.StringVar(&kind, "kind", "", "email_domain, email_hash, or ip_range (required)")
			fs.StringVar(&severity, "severity", string(moderation.SignupReject), "reject or require_approval")
			fs.StringVar(&comment, "comment", "", "a note for the moderators")
			fs.DurationVar(&duration, "duration", 0, "how long the block lasts, such as 720h; 0 lasts until lifted")
		}
	})
	if code >= 0 {
		return code
	}
	if (sub == "create" || sub == "import") && kind == "" {
		_, _ = fmt.Fprintf(stderr, "%s: -kind is required\n", c.name)
		return 2
	}
	if limit < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: -limit must be at least 1\n", c.name)
		return 2
	}
	var list string
	if sub == "import" {
		var err error
		if list, err = readSignupBlockList(c.fs.Arg(0), stdin); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
			return 1
		}
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var err error
	switch sub {
	case "list":
		var vs []appmoderation.SignupBlockView
		f := appmoderation.SignupBlockFilter{Kind: moderation.SignupBlockKind(kind), Active: active, Before: before}
		if vs, err = app.Moderation.SignupBlocks(ctx, f, limit); err == nil {
			err = printSignupBlocks(stdout, vs)
		}
	case "create":
		var b moderation.SignupBlock
		b, err = app.Moderation.AddSignupBlock(ctx, appmoderation.SignupBlockInput{
			Kind: moderation.SignupBlockKind(kind), Value: c.fs.Arg(0), Severity: moderation.SignupBlockSeverity(severity),
			Comment: comment, Duration: duration,
		})
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "created %s block on %s %s (block %s, %s)\n", b.Severity, b.Kind, b.Value, b.ID, describeExpiry(b.ExpiresAt))
		}
	case "import":
		var res appmoderation.SignupBlockImportResult
		res, err = app.Moderation.ImportSignupBlocks(ctx, appmoderation.SignupBlockImportInput{
			Kind: moderation.SignupBlockKind(kind), Text: list, Severity: moderation.SignupBlockSeverity(severity),
			Comment: comment, Duration: duration,
		})
		if err == nil {
			printSignupBlockImport(stdout, res)
		}
	case "lift":
		id := c.fs.Arg(0)
		err = app.Moderation.LiftSignupBlock(ctx, id, "")
		if errors.Is(err, appmoderation.ErrNotFound) {
			err = fmt.Errorf("no sign-up block has the ID %q", id)
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "lifted sign-up block %s\n", id)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// readSignupBlockList reads the list of an import from the file path, or from stdin for "-".
func readSignupBlockList(path string, stdin io.Reader) (string, error) {
	r := stdin
	if path != "-" {
		f, err := os.Open(path) //nolint:gosec // the operator names the file to import
		if err != nil {
			return "", err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, maxSignupBlockListInput+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxSignupBlockListInput {
		return "", fmt.Errorf("the list is larger than %d bytes", maxSignupBlockListInput)
	}
	return string(b), nil
}

func printSignupBlockImport(w io.Writer, res appmoderation.SignupBlockImportResult) {
	_, _ = fmt.Fprintf(w, "created %d sign-up block(s), skipped %d line(s)\n", res.Added, len(res.Skipped))
	for _, s := range res.Skipped {
		_, _ = fmt.Fprintf(w, "line %d: %s: %s\n", s.Line, s.Reason, oneLine(s.Value))
	}
}

func printSignupBlocks(w io.Writer, vs []appmoderation.SignupBlockView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tVALUE\tSEVERITY\tSTATE\tCREATED\tEXPIRES\tBY\tBAN\tCOMMENT")
	for _, v := range vs {
		b := v.Block
		expires, by, ban := "-", b.CreatedByUserID, b.ModerationActionID
		if b.ExpiresAt != nil {
			expires = b.ExpiresAt.Format(time.RFC3339)
		}
		if by == "" {
			by = "cli"
		}
		if ban == "" {
			ban = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", b.ID, b.Kind, b.Value, b.Severity, v.State,
			b.CreatedAt.Format(time.RFC3339), expires, by, ban, oneLine(b.Comment))
	}
	return tw.Flush()
}

func adminSignupBlockUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin signup-block list [-kind <kind>] [-active] [-before <id>] [-limit <n>] [-config <file>]
  my admin signup-block create -kind <kind> [-severity reject|require_approval] [-comment <text>]
                            [-duration <d>] [-config <file>] <value>
  my admin signup-block import -kind <kind> [-severity reject|require_approval] [-comment <text>]
                               [-duration <d>] [-config <file>] <file | ->
  my admin signup-block lift [-config <file>] <block id>

Blocks on sign-ups from the registration page, in effect at once:

  email_domain  the domain of the email address and its subdomains; <value> is a
                domain such as spam.example (U-labels such as 例え.jp are
                stored in IDNA A-labels)
  email_hash    one email address, compared in lower case without the part of
                the local part from its first "+"; <value> is the address, which
                is stored only as its SHA-256, or that hash in hex
  ip_range      the client addresses in a CIDR prefix such as 192.0.2.0/24; a
                single address blocks only itself

A sign-up that a block matches is refused (reject, the default) or starts
pending for a moderator whatever the registration mode (require_approval); the
registration page does not say why. Bans can add the email_hash block of the
banned account (my admin account action issue -kind ban -block-email).

import adds a block for each value of a list file ("-" reads stdin), such as a
public list of disposable email domains, all with the same kind and settings:
one value per line, or a CSV row whose first field is the value; blank lines
and lines starting with # are ignored; at most 10000 values. It skips and
prints the lines that are not valid for the kind, repeat an earlier line, or
are already blocked by a block in effect at least as strong; importing the
same list again adds only what is new.

list shows the blocks newest first, lifted and expired ones included unless
-active is given. lift ends one block; lifting a lifted block changes nothing.
Every addition and lift is recorded in the audit log.
`)
}
