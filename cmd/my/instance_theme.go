// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// maxThemeInput bounds what `my instance theme set` reads from stdin.
const maxThemeInput = 256 * 1024

// themeDocument is the JSON form of a token set on the command line: the shape of the admin API's
// SetThemeRequest (api/openapi.yaml), so that a set moves between the two as it is. revision names
// a published set in the output; set ignores it, so that the output of get reads back as it is.
type themeDocument struct {
	Revision string               `json:"revision,omitempty"`
	Tokens   []themeTokenDocument `json:"tokens"`
}

type themeTokenDocument struct {
	Token string `json:"token"`
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
	Value string `json:"value,omitempty"`
}

// runInstanceTheme implements `my instance theme <get|set|publish|restore>` (ADR-0059 §15).
func runInstanceTheme(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	sub, code := dispatch("instance theme", args, []string{"get", "set", "publish", "restore"}, stderr, instanceThemeUsage)
	if code >= 0 {
		return code
	}
	name := "my instance theme " + sub
	fs := newFlagSet(name)
	fs.SetOutput(stderr)
	fs.Usage = func() { instanceThemeUsage(stderr) }
	file := fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")
	published := false
	if sub == "get" {
		fs.BoolVar(&published, "published", false, "print the set in effect instead of the draft")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: want no arguments, got %d\n\n", name, fs.NArg())
		instanceThemeUsage(stderr)
		return 2
	}
	var in themeDocument
	if sub == "set" {
		data, err := io.ReadAll(io.LimitReader(stdin, maxThemeInput+1))
		if err != nil || len(data) > maxThemeInput {
			_, _ = fmt.Fprintf(stderr, "%s: read the theme from standard input: too long or unreadable\n", name)
			return 1
		}
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: the theme is not JSON of {\"tokens\": [...]}: %v\n", name, err)
			return 1
		}
		if _, err := dec.Token(); err != io.EOF {
			_, _ = fmt.Fprintf(stderr, "%s: the theme must be exactly one JSON value, found more content after it\n", name)
			return 1
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

	out, err := instanceThemeCommand(ctx, app, sub, published, in)
	var te *instance.ThemeError
	switch {
	case errors.As(err, &te):
		verb := "published"
		if sub == "set" {
			verb = "saved"
		}
		_, _ = fmt.Fprintf(stderr, "%s: the theme is not %s:\n", name, verb)
		for _, p := range te.Problems {
			_, _ = fmt.Fprintf(stderr, "  %s\n", p)
		}
		return 1
	case errors.Is(err, appinstance.ErrNotFound) && sub == "publish":
		_, _ = fmt.Fprintf(stderr, "%s: no draft is saved; save one with my instance theme set\n", name)
		return 1
	case errors.Is(err, appinstance.ErrNotFound) && sub == "restore":
		_, _ = fmt.Fprintf(stderr, "%s: no other set was published before the one in effect\n", name)
		return 1
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	doc := themeDocument{Revision: out.revision, Tokens: make([]themeTokenDocument, len(out.set.Tokens))}
	for i, v := range out.set.Tokens {
		doc.Tokens[i] = themeTokenDocument{Token: v.Token, Light: v.Light, Dark: v.Dark, Value: v.Value}
	}
	for _, o := range out.set.Stale {
		_, _ = fmt.Fprintf(stderr, "%s: the override of %s no longer applies (%s) and is left out\n", name, o.Token, o.Reason)
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	return 0
}

// themeOutput is the set that a theme command prints, and the revision when it is published.
type themeOutput struct {
	revision string
	set      instance.ThemeSet
}

// instanceThemeCommand runs the subcommand sub of `my instance theme`.
func instanceThemeCommand(ctx context.Context, app *platform.App, sub string, published bool, in themeDocument) (themeOutput, error) {
	revision := func(rev instance.ThemeRevision, err error) (themeOutput, error) {
		return themeOutput{revision: rev.ID, set: rev.Set}, err
	}
	switch sub {
	case "set":
		tokens := make([]instance.ThemeTokenValue, len(in.Tokens))
		for i, t := range in.Tokens {
			tokens[i] = instance.ThemeTokenValue{Token: t.Token, Light: t.Light, Dark: t.Dark, Value: t.Value}
		}
		set, err := app.Instance.SaveThemeDraft(ctx, appinstance.SaveThemeDraftInput{Tokens: tokens})
		return themeOutput{set: set}, err
	case "publish":
		return revision(app.Instance.PublishTheme(ctx, ""))
	case "restore":
		return revision(app.Instance.RestoreTheme(ctx, ""))
	}
	if published {
		rev, _, err := app.Instance.Theme(ctx)
		return revision(rev, err)
	}
	set, _, err := app.Instance.ThemeDraft(ctx)
	return themeOutput{set: set}, err
}

func instanceThemeUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my instance theme get [-published] [-config <file>]
  my instance theme set [-config <file>] < theme.json
  my instance theme publish [-config <file>]
  my instance theme restore [-config <file>]

Reads and changes the theme of the web client: one set of overrides of the
design tokens, as JSON of the admin API's PUT /api/v1/admin/theme/draft:

  {"tokens": [{"token": "color-accent", "light": "#ff8a80", "dark": "#ff8a80"},
              {"token": "radius-control", "value": "0.25rem"}]}

set saves the draft: it replaces the whole set (tokens left out keep their
defaults; no tokens reset them), checks every value and the contrast of the
color pairs in both themes, and prints the draft as saved. The theme in effect
does not change until publish, which publishes the draft under a new revision.
restore publishes again the set published before the one in effect. get prints
the draft, or with -published the set in effect with its revision. Overrides
that an upgrade made stale are named on standard error and left out.
`+docsLine("docs/operations/instance-settings.md"))
}
