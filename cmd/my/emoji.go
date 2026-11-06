// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/platform"
)

// stringList is a flag that may be given more than once.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// runAdminEmoji implements `my admin emoji <subcommand>`: the custom emoji (ADR-0051 §5, FR-111),
// through the same operations as the admin API. The CLI acts as no user: its operations are
// audited as the CLI's, and the images it registers are uploads of the instance actor.
func runAdminEmoji(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		adminEmojiUsage(stderr)
		return 2
	}
	sub, code := dispatch("admin emoji", args, []string{"create", "list", "update", "replace-image", "enable", "disable", "copy", "delete", "requests"}, stderr, adminEmojiUsage)
	if code >= 0 {
		return code
	}
	if sub == "requests" {
		return runAdminEmojiRequests(args[1:], stdout, stderr)
	}
	var (
		shortcode, category, domain, before string
		aliases                             stringList
		local, remote, hidden, noAliases    bool
		limit                               = 50
	)
	nargs := map[string]int{"list": 0, "replace-image": 2}[sub]
	if sub != "list" && nargs == 0 {
		nargs = 1
	}
	c, code := parseSubcommand("my admin emoji "+sub, args[1:], nargs, stderr, adminEmojiUsage, func(fs *flag.FlagSet) {
		switch sub {
		case "create":
			fs.StringVar(&shortcode, "shortcode", "", "2 to 30 letters, digits, and underscores (required)")
			fs.StringVar(&category, "category", "", "the category of the picker")
			fs.Var(&aliases, "alias", "another name that the picker finds the emoji by; repeat for more")
			fs.BoolVar(&hidden, "hidden", false, "keep the emoji out of the picker; texts can still use it")
		case "list":
			fs.BoolVar(&local, "local", false, "show only the emoji of this instance")
			fs.BoolVar(&remote, "remote", false, "show only the emoji of other servers")
			fs.StringVar(&domain, "domain", "", "show only the emoji of this remote domain")
			fs.StringVar(&shortcode, "shortcode", "", "show only the emoji whose shortcode contains this text")
			fs.StringVar(&before, "before", "", "show the emoji older than this emoji ID")
			fs.IntVar(&limit, "limit", limit, "number of emoji to show, newest first")
		case "update":
			fs.StringVar(&category, "category", "", `the category of the picker; "" removes it`)
			fs.Var(&aliases, "alias", "another name that the picker finds the emoji by; repeat for more (replaces the aliases)")
			fs.BoolVar(&noAliases, "no-aliases", false, "remove every alias")
			fs.BoolVar(&hidden, "hidden", false, "keep the emoji out of the picker; -hidden=false shows it there again")
		case "copy":
			fs.StringVar(&shortcode, "shortcode", "", "the shortcode of the local emoji (default: the remote emoji's)")
		}
	})
	if code >= 0 {
		return code
	}
	var update appcontent.CustomEmojiUpdate
	if sub == "update" {
		c.fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "category":
				update.Category = &category
			case "alias":
				update.Aliases = (*[]string)(&aliases)
			case "no-aliases":
				if noAliases {
					update.Aliases = &[]string{}
				}
			case "hidden":
				visible := !hidden
				update.VisibleInPicker = &visible
			}
		})
	}
	switch {
	case sub == "create" && shortcode == "":
		_, _ = fmt.Fprintf(stderr, "%s: -shortcode is required\n", c.name)
		return 2
	case local && remote:
		_, _ = fmt.Fprintf(stderr, "%s: -local and -remote cannot be given together\n", c.name)
		return 2
	case limit < 1:
		_, _ = fmt.Fprintf(stderr, "%s: -limit must be at least 1\n", c.name)
		return 2
	case noAliases && len(aliases) > 0:
		_, _ = fmt.Fprintf(stderr, "%s: -alias and -no-aliases cannot be given together\n", c.name)
		return 2
	case sub == "update" && update.Category == nil && update.Aliases == nil && update.VisibleInPicker == nil:
		_, _ = fmt.Fprintf(stderr, "%s: give at least one of -category, -alias, -no-aliases, and -hidden\n", c.name)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var err error
	switch sub {
	case "create":
		var e appcontent.AdminCustomEmoji
		e, err = createEmoji(ctx, app, c.fs.Arg(0), appcontent.CreateCustomEmojiInput{
			Shortcode: shortcode, Category: category, Aliases: aliases, VisibleInPicker: !hidden,
		})
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "created :%s: (emoji %s)\n", e.Emoji.Shortcode, e.Emoji.ID)
		}
	case "list":
		q := appcontent.CustomEmojiQuery{Domain: domain, Shortcode: shortcode, Before: before}
		switch {
		case local:
			q.Locality = appcontent.LocalEmoji
		case remote:
			q.Locality = appcontent.RemoteEmoji
		}
		var list []appcontent.AdminCustomEmoji
		if list, err = app.CustomEmojis.List(ctx, q, limit); err == nil {
			err = printEmojis(stdout, list)
		}
	case "update":
		var e appcontent.AdminCustomEmoji
		if e, err = app.CustomEmojis.Update(ctx, c.fs.Arg(0), update); err == nil {
			_, _ = fmt.Fprintf(stdout, "updated %s (emoji %s)\n", e.Emoji.Label(), e.Emoji.ID)
		}
	case "replace-image":
		var e appcontent.AdminCustomEmoji
		if e, err = replaceEmojiImage(ctx, app, c.fs.Arg(0), c.fs.Arg(1)); err == nil {
			_, _ = fmt.Fprintf(stdout, "replaced the image of %s (emoji %s)\n", e.Emoji.Label(), e.Emoji.ID)
		}
	case "copy":
		var src, e appcontent.AdminCustomEmoji
		if src, err = app.CustomEmojis.Get(ctx, c.fs.Arg(0)); err == nil {
			e, err = copyEmoji(ctx, app, src.Emoji.ID, shortcode)
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "copied %s as %s (emoji %s)\n", src.Emoji.Label(), e.Emoji.Label(), e.Emoji.ID)
		}
	case "enable", "disable":
		enabled := sub == "enable"
		var e appcontent.AdminCustomEmoji
		e, err = app.CustomEmojis.Update(ctx, c.fs.Arg(0), appcontent.CustomEmojiUpdate{Enabled: &enabled})
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "%sd %s (emoji %s)\n", sub, e.Emoji.Label(), e.Emoji.ID)
		}
	case "delete":
		var e appcontent.AdminCustomEmoji
		if e, err = app.CustomEmojis.Get(ctx, c.fs.Arg(0)); err == nil {
			err = app.CustomEmojis.Delete(ctx, e.Emoji.ID, "")
		}
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "deleted %s (emoji %s)\n", e.Emoji.Label(), e.Emoji.ID)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", c.name, describeEmojiError(err, c.fs.Arg(0)))
		return 1
	}
	return 0
}

// createEmoji registers the image file at path as a local emoji.
func createEmoji(ctx context.Context, app *platform.App, path string, in appcontent.CreateCustomEmojiInput) (appcontent.AdminCustomEmoji, error) {
	if !content.ValidLocalShortcode(in.Shortcode) {
		return appcontent.AdminCustomEmoji{}, &content.CustomEmojiError{Reason: content.EmojiInvalidShortcode, Value: in.Shortcode}
	}
	var err error
	if in.MediaID, in.OwnerAccountID, err = uploadEmojiImage(ctx, app, path); err != nil {
		return appcontent.AdminCustomEmoji{}, err
	}
	return app.CustomEmojis.Create(ctx, in)
}

// replaceEmojiImage makes the image file at path the new image of the local emoji id.
func replaceEmojiImage(ctx context.Context, app *platform.App, id, path string) (appcontent.AdminCustomEmoji, error) {
	// An unknown or remote emoji is refused before the image is uploaded.
	e, err := app.CustomEmojis.Get(ctx, id)
	if err != nil {
		return appcontent.AdminCustomEmoji{}, err
	}
	if !e.Emoji.Local() {
		return appcontent.AdminCustomEmoji{}, &content.CustomEmojiError{Reason: content.EmojiRemote, Value: id}
	}
	mediaID, owner, err := uploadEmojiImage(ctx, app, path)
	if err != nil {
		return appcontent.AdminCustomEmoji{}, err
	}
	return app.CustomEmojis.ReplaceImage(ctx, e.Emoji.ID, owner, mediaID, "")
}

// copyEmoji registers the image of the remote emoji id as a new local emoji, uploaded as the
// instance actor's.
func copyEmoji(ctx context.Context, app *platform.App, id, shortcode string) (appcontent.AdminCustomEmoji, error) {
	owner, err := app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		return appcontent.AdminCustomEmoji{}, err
	}
	return app.CustomEmojis.Copy(ctx, appcontent.CopyCustomEmojiInput{ID: id, Shortcode: shortcode, OwnerAccountID: owner.ID})
}

// uploadEmojiImage uploads the image file at path as the instance actor's and processes it at
// once for the emoji's variants. It returns the media file and the instance actor's account.
func uploadEmojiImage(ctx context.Context, app *platform.App, path string) (mediaID, ownerAccountID string, err error) {
	f, err := os.Open(path) //nolint:gosec // the operator names the image to register
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()
	if st, err := f.Stat(); err == nil && st.Size() > app.CustomEmojis.MaxBytes() {
		return "", "", &content.CustomEmojiError{Reason: content.EmojiTooLarge, Value: path}
	}
	owner, err := app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		return "", "", err
	}
	m, err := app.Media.Upload(ctx, appcontent.UploadInput{
		OwnerAccountID: owner.ID, Content: f, Purpose: content.PurposeEmoji,
	})
	if err != nil {
		return "", "", err
	}
	if res := app.Media.Process(ctx, m.File.ID, content.PurposeEmoji); !res.IsDone() {
		return "", "", fmt.Errorf("process the image: %v", res)
	}
	return m.File.ID, owner.ID, nil
}

// describeEmojiError explains a failure to the operator.
func describeEmojiError(err error, arg string) string {
	var ce *content.CustomEmojiError
	switch {
	case errors.As(err, &ce):
		return map[content.CustomEmojiReason]string{
			content.EmojiInvalidShortcode:  "a shortcode is 2 to 30 letters (A to Z), digits, and underscores",
			content.EmojiShortcodeTaken:    "a custom emoji of this instance already has the shortcode",
			content.EmojiInvalidAlias:      "an alias is 2 to 30 letters (A to Z), digits, and underscores, and an emoji has at most 10",
			content.EmojiInvalidCategory:   "a category is at most 64 characters on one line",
			content.EmojiUnsupportedFormat: "the image must be a PNG, a GIF, or a WebP",
			content.EmojiTooLarge:          "the image is larger than custom_emoji_max_bytes",
			content.EmojiTooBig:            "the image must be at most 1,024 pixels on each side",
			content.EmojiRemote:            "this can be done only with the custom emoji of this instance",
			content.EmojiLocal:             "only the custom emoji of other servers can be copied",
			content.EmojiFetchFailed:       "the image could not be fetched from the emoji's server",
			content.EmojiMediaNotFound:     "the image was not found",
			content.EmojiMediaNotReady:     "the image is not processed yet",
			content.EmojiLimitReached:      "the user has as many custom emoji and pending requests as their limit",
			content.EmojiRequestPending:    "the emoji already has a pending request for a new image",
			content.EmojiInvalidReason:     "a reason is at most 500 characters",
		}[ce.Reason] + "; nothing was changed"
	case errors.Is(err, appcontent.ErrNotFound):
		return fmt.Sprintf("no custom emoji has the ID %q", arg)
	}
	return err.Error()
}

func printEmojis(w io.Writer, list []appcontent.AdminCustomEmoji) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSHORTCODE\tDOMAIN\tSTATE\tPICKER\tCATEGORY\tALIASES\tUPDATED")
	for _, e := range list {
		domain, state, picker, category, aliases := e.Emoji.Domain, "enabled", "shown", e.Settings.Category, strings.Join(e.Aliases, ",")
		if domain == "" {
			domain = "-"
		}
		if !e.Settings.Enabled {
			state = "disabled"
		}
		if !e.Settings.VisibleInPicker {
			picker = "hidden"
		}
		if !e.Emoji.Local() {
			picker = "-"
		}
		if category == "" {
			category = "-"
		}
		if aliases == "" {
			aliases = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.Emoji.ID, e.Emoji.Shortcode, domain, state, picker,
			oneLine(category), aliases, e.Image.RevisedAt.UTC().Format(time.RFC3339))
	}
	return tw.Flush()
}

func adminEmojiUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin emoji create -shortcode <name> [-category <text>] [-alias <name>]... [-hidden]
                        [-config <file>] <image file>
  my admin emoji list [-local | -remote] [-domain <domain>] [-shortcode <text>] [-before <id>]
                      [-limit <n>] [-config <file>]
  my admin emoji update [-category <text>] [-alias <name>]... [-no-aliases]
                        [-hidden | -hidden=false] [-config <file>] <emoji id>
  my admin emoji replace-image [-config <file>] <emoji id> <image file>
  my admin emoji enable|disable [-config <file>] <emoji id>
  my admin emoji copy [-shortcode <name>] [-config <file>] <emoji id>
  my admin emoji delete [-config <file>] <emoji id>
  my admin emoji requests list|approve|reject ...   (see my admin emoji requests -h)

The custom emoji of this instance, which posts and profiles use as
:shortcode:, and those of other servers that posts and profiles brought in.

create registers a local emoji with an image: a PNG, a GIF, or a WebP of at most
custom_emoji_max_bytes (256 KiB by default) and 1,024 pixels on each side. The
image is re-encoded without metadata into variants of up to 256 pixels; an APNG
shows only its first frame. GoToSocial does not fetch emoji larger than 100 KiB
by default. -hidden keeps the emoji out of the picker.

update changes what it is given of a local emoji: -category ("" removes it), the
aliases (-alias replaces them all, -no-aliases removes them), and whether the
picker shows it (-hidden, or -hidden=false). replace-image gives a local emoji a
new image, checked as for create; the servers that have the emoji fetch it again.

disable shows the emoji nowhere (its :shortcode: stays text) until enable; both
work on remote emoji too. copy registers a new local emoji with the image of a
remote one, fetched under the limits of the media proxy (never from a
reject_media domain), with the remote shortcode or -shortcode, in the picker,
without a category or aliases. delete deletes a local emoji with its images for
good; the posts that used it show its :shortcode: as text.

Every operation is recorded in the audit log as the CLI's, as the same operation
of the admin screen and the admin API (/api/v1/admin/custom_emojis) is.

requests reviews the requests of the users whose roles grant
create_custom_emojis.
`)
}
