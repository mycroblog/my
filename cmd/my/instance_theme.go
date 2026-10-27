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

// maxThemeInput bounds what `my instance theme set` reads from stdin: the custom CSS limit and room
// for the tokens.
const maxThemeInput = instance.MaxCustomCSSBytes + 64*1024

// themeDocument is the JSON form of a theme on the command line: the shape of the admin API's
// SetThemeRequest (api/openapi.yaml), so that a theme moves between the two as it is.
type themeDocument struct {
	Tokens    []themeTokenDocument `json:"tokens"`
	CustomCSS string               `json:"custom_css"`
}

type themeTokenDocument struct {
	Token string `json:"token"`
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
	Value string `json:"value,omitempty"`
}

// runInstanceTheme implements `my instance theme <get|set>` (ADR-0059).
func runInstanceTheme(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		instanceThemeUsage(stderr)
		return 2
	}
	sub := args[0]
	switch sub {
	case "-h", "-help", "--help":
		instanceThemeUsage(stderr)
		return 0
	case "get", "set":
	default:
		_, _ = fmt.Fprintf(stderr, "my instance theme: unknown subcommand %q\n\n", sub)
		instanceThemeUsage(stderr)
		return 2
	}
	name := "my instance theme " + sub
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { instanceThemeUsage(stderr) }
	file := fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")
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
			_, _ = fmt.Fprintf(stderr, "%s: the theme is not JSON of {\"tokens\": [...], \"custom_css\": \"...\"}: %v\n", name, err)
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

	var rev instance.ThemeRevision
	if sub == "set" {
		tokens := make([]instance.ThemeTokenValue, len(in.Tokens))
		for i, t := range in.Tokens {
			tokens[i] = instance.ThemeTokenValue{Token: t.Token, Light: t.Light, Dark: t.Dark, Value: t.Value}
		}
		rev, err = app.Instance.SetTheme(ctx, appinstance.SetThemeInput{Tokens: tokens, CustomCSS: in.CustomCSS})
		var te *instance.ThemeError
		if errors.As(err, &te) {
			_, _ = fmt.Fprintf(stderr, "%s: the theme is not saved:\n", name)
			for _, p := range te.Problems {
				_, _ = fmt.Fprintf(stderr, "  %s\n", p)
			}
			return 1
		}
	} else {
		rev, _, err = app.Instance.Theme(ctx)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	out := themeDocument{Tokens: make([]themeTokenDocument, len(rev.Tokens)), CustomCSS: rev.CustomCSS}
	for i, v := range rev.Tokens {
		out.Tokens[i] = themeTokenDocument{Token: v.Token, Light: v.Light, Dark: v.Dark, Value: v.Value}
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	return 0
}

func instanceThemeUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my instance theme get [-config <file>]
  my instance theme set [-config <file>] < theme.json

Reads and replaces the theme of the web client (ADR-0059,
docs/operations/instance-settings.md): overrides of the design tokens and a
custom CSS, as JSON of the admin API's PUT /api/v1/admin/theme:

  {"tokens": [{"token": "color-accent", "light": "#1447e6", "dark": "#92c5fe"},
              {"token": "radius-md", "value": "0.25rem"}],
   "custom_css": ""}

set replaces the whole theme (tokens left out go back to their defaults; no
tokens and no CSS reset it), checks every value and the contrast of the color
pairs in both themes, records a revision, and prints the theme as saved. get
prints the current theme.
`)
}
