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
	"unicode/utf8"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// maxTextInput bounds what `my instance set <key> -` reads from stdin; it is above the longest
// text (100000 characters of up to 4 bytes each).
const maxTextInput = 400 * 1024

// runInstance implements `my instance <subcommand>`. stdin supplies a value given as "-".
func runInstance(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		instanceUsage(stderr)
		return 2
	}
	sub := args[0]
	var nargs int
	switch sub {
	case "-h", "-help", "--help":
		instanceUsage(stderr)
		return 0
	case "rule":
		return runInstanceRule(args[1:], stdout, stderr)
	case "theme":
		return runInstanceTheme(args[1:], stdin, stdout, stderr)
	case "brand":
		return runInstanceBrand(args[1:], stdout, stderr)
	case "list":
		nargs = 0
	case "get", "history":
		nargs = 1
	case "set":
		nargs = 2
	default:
		_, _ = fmt.Fprintf(stderr, "my instance: unknown subcommand %q\n\n", sub)
		instanceUsage(stderr)
		return 2
	}
	name := "my instance " + sub
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { instanceUsage(stderr) }
	file := fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")
	limit := 20
	owner := ""
	if sub == "set" {
		fs.StringVar(&owner, "owner", "", "local username that owns an image file given as a media setting's value")
	}
	if sub == "history" {
		fs.IntVar(&limit, "limit", limit, "number of revisions to show, newest first")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != nargs {
		_, _ = fmt.Fprintf(stderr, "%s: want %d argument(s), got %d\n\n", name, nargs, fs.NArg())
		instanceUsage(stderr)
		return 2
	}
	pos := fs.Args()
	var key instanceKey
	if nargs > 0 {
		var err error
		if key, err = parseInstanceKey(pos[0]); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return 2
		}
		if owner != "" && !key.media() {
			_, _ = fmt.Fprintf(stderr, "%s: -owner applies only to media keys, not to %s\n", name, pos[0])
			return 2
		}
	}
	value := ""
	if sub == "set" {
		value = pos[1]
		if value == "-" {
			b, err := io.ReadAll(io.LimitReader(stdin, maxTextInput+1))
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: read the value from standard input: %v\n", name, err)
				return 1
			}
			if len(b) > maxTextInput {
				_, _ = fmt.Fprintf(stderr, "%s: the value on standard input is too long\n", name)
				return 1
			}
			value = strings.TrimRight(string(b), "\r\n")
		}
	}

	cfg, _, err := config.Load(config.Source{File: *file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	ctx := context.Background()
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	defer func() { _ = app.Close() }()
	cmd := instanceCommand{svc: app.Instance, app: app, stdout: stdout}
	switch sub {
	case "list":
		err = cmd.list(ctx)
	case "get":
		err = cmd.get(ctx, key)
	case "set":
		err = cmd.set(ctx, key, owner, value)
	case "history":
		err = cmd.history(ctx, key, limit)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	return 0
}

// instanceKey is a setting key or a text key; exactly one field is set.
type instanceKey struct {
	setting instance.SettingKey
	text    instance.TextKey
}

// media reports whether the key is a setting whose value is a media ID.
func (k instanceKey) media() bool {
	d, err := instance.Lookup(k.setting)
	return err == nil && d.Kind == instance.KindMedia
}

func parseInstanceKey(name string) (instanceKey, error) {
	if d, err := instance.Lookup(instance.SettingKey(name)); err == nil {
		return instanceKey{setting: d.Key}, nil
	}
	if d, err := instance.LookupText(instance.TextKey(name)); err == nil {
		return instanceKey{text: d.Key}, nil
	}
	return instanceKey{}, fmt.Errorf("unknown key %q; `my instance list` shows every key", name)
}

type instanceCommand struct {
	svc    *appinstance.Service
	app    *platform.App
	stdout io.Writer
}

func (c instanceCommand) list(ctx context.Context) error {
	s, err := c.svc.Settings(ctx)
	if err != nil {
		return err
	}
	texts, err := c.svc.Texts(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tSOURCE")
	for _, d := range instance.Definitions() {
		v, stored := s.Value(d.Key)
		source := "default"
		if stored {
			source = "set"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Key, v, source)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(c.stdout)
	tw = tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TEXT\tREVISED\tBODY")
	for _, t := range texts {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Key, t.RevisedAt.Format(time.RFC3339), preview(t.Body))
	}
	return tw.Flush()
}

func (c instanceCommand) get(ctx context.Context, key instanceKey) error {
	if key.setting != "" {
		s, err := c.svc.Settings(ctx)
		if err != nil {
			return err
		}
		v, _ := s.Value(key.setting)
		_, err = fmt.Fprintln(c.stdout, v)
		return err
	}
	t, ok, err := c.svc.Text(ctx, key.text)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not set", key.text)
	}
	_, err = fmt.Fprintln(c.stdout, t.Body)
	return err
}

func (c instanceCommand) set(ctx context.Context, key instanceKey, owner, value string) error {
	if owner != "" {
		id, err := c.upload(ctx, owner, value)
		if err != nil {
			return err
		}
		value = id
	}
	if key.setting != "" {
		rev, err := c.svc.SetSetting(ctx, appinstance.SetSettingInput{Key: key.setting, Value: value})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(c.stdout, "set %s = %s (revision %s)\n", rev.Key, rev.Value, rev.ID)
		return err
	}
	rev, err := c.svc.SetText(ctx, appinstance.SetTextInput{Key: key.text, Body: value})
	if err != nil {
		return err
	}
	verb := "set"
	if rev.Body == "" {
		verb = "cleared"
	}
	_, err = fmt.Fprintf(c.stdout, "%s %s (revision %s)\n", verb, rev.Key, rev.ID)
	return err
}

// upload uploads the image file at path as a media file of the local account owner, and
// returns its ID. The worker processes it like any upload; until then, previews keep the
// built-in image.
func (c instanceCommand) upload(ctx context.Context, owner, path string) (string, error) {
	a, err := c.app.Accounts.FindByHandle(ctx, owner, "")
	if err != nil {
		return "", fmt.Errorf("owner %s: %w", owner, err)
	}
	if !a.Account.IsLocal() || a.Local == nil {
		return "", fmt.Errorf("owner %s is not a local account", owner)
	}
	f, err := os.Open(path) //nolint:gosec // the operator names the file to upload
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	m, err := c.app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: a.Account.ID, Content: f})
	if err != nil {
		return "", fmt.Errorf("upload %s: %w", path, err)
	}
	_, err = fmt.Fprintf(c.stdout, "uploaded %s as media %s; previews use it once the worker has processed it\n", path, m.File.ID)
	return m.File.ID, err
}

func (c instanceCommand) history(ctx context.Context, key instanceKey, limit int) error {
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "REVISION\tREVISED\tBY\tVALUE")
	by := func(userID string) string {
		if userID == "" {
			return "cli"
		}
		return userID
	}
	if key.setting != "" {
		revs, err := c.svc.SettingHistory(ctx, key.setting, limit)
		if err != nil {
			return err
		}
		for _, r := range revs {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.RevisedAt.Format(time.RFC3339), by(r.RevisedByUserID), r.Value)
		}
		return tw.Flush()
	}
	revs, err := c.svc.TextHistory(ctx, key.text, limit)
	if err != nil {
		return err
	}
	for _, r := range revs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.RevisedAt.Format(time.RFC3339), by(r.RevisedByUserID), preview(r.Body))
	}
	return tw.Flush()
}

// preview shows a text body on one line, quoted and cut to 60 characters.
func preview(body string) string {
	const n = 60
	if utf8.RuneCountInString(body) > n {
		body = string([]rune(body)[:n]) + "…"
	}
	return fmt.Sprintf("%q", body)
}

func instanceUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my instance list [-config <file>]
  my instance get [-config <file>] <key>
  my instance set [-config <file>] <key> <value>
  my instance set -owner <username> [-config <file>] <preview_image|thumbnail> <image file>
  my instance history [-limit <n>] [-config <file>] <key>
  my instance rule <list|add|edit|move|retire> ...  (my instance rule -h)
  my instance theme <get|set> ...  (my instance theme -h)
  my instance brand <show|clear> ...  (my instance brand -h)

Reads and changes the instance settings and texts stored in the
database (docs/operations/instance-settings.md). Every change is kept as a
revision; history shows them, newest first. The database schema must already
be applied with Atlas.

Setting keys hold one value each; a key that has never been set reads as its
default. Text keys (name, short_description, description, terms,
privacy_policy, preview_image_alt, thumbnail_alt) hold one text each, written
in any language and shown as it is in every locale. An empty value clears a
text. A value of "-" is read from standard input.

preview_image is the media ID of the image of link previews for profiles and
posts without images (empty: the built-in image), and the text
preview_image_alt is its alt text; thumbnail and thumbnail_alt are the image
that represents the instance and its alt text. With -owner, the value of a
media key is an image file instead: it is uploaded as a media file of that
local account and used once the worker has processed it.

contact_account (the username of a local account) and contact_email are how
people reach the operators.

Deployment configuration (adapters, host, secrets) is not an instance setting;
it comes from the environment and the config file (my config check).
`)
}
