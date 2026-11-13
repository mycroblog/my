// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
)

func problemsOf(t *testing.T, err error) []instance.ThemeProblem {
	t.Helper()
	var te *instance.ThemeError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want a *ThemeError", err)
	}
	if !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("err does not wrap ErrInvalidValue")
	}
	return te.Problems
}

func TestThemeDefaultsMeetTheirContrastPairs(t *testing.T) {
	// The generated defaults are rounded to #rrggbb, which must not take a pair below its minimum.
	if _, err := instance.NormalizeTheme(nil, ""); err != nil {
		t.Fatalf("the defaults: %v", err)
	}
	for _, p := range instance.ThemeContrastPairs() {
		for _, name := range []string{p.FG, p.BG} {
			if tok, ok := instance.LookupThemeToken(name); !ok || tok.Kind != instance.ThemeTokenColor {
				t.Errorf("pair %s on %s names %s, which is not a color token", p.FG, p.BG, name)
			}
		}
	}
}

func TestThemeDefaultsAreValidOverrides(t *testing.T) {
	// An administrator may send the defaults back as they are.
	var tokens []instance.ThemeTokenValue
	for _, tok := range instance.ThemeTokens() {
		tokens = append(tokens, instance.ThemeTokenValue{Token: tok.Name, Light: tok.Light, Dark: tok.Dark, Value: tok.Default})
	}
	if _, err := instance.NormalizeTheme(tokens, ""); err != nil {
		t.Fatalf("the defaults as overrides: %v", err)
	}
}

func TestContrastRatio(t *testing.T) {
	for _, tt := range []struct {
		fg, bg string
		want   float64
	}{
		{"#000000", "#ffffff", 21},
		{"#ffffff", "#ffffff", 1},
		{"#777777", "#ffffff", 4.48},
		// Half-transparent black over white is the gray #7f7f7f (gamma-space compositing).
		{"#00000080", "#ffffff", 4.00},
	} {
		if got := instance.ContrastRatio(tt.fg, tt.bg); math.Abs(got-tt.want) > 0.01 {
			t.Errorf("ContrastRatio(%s, %s) = %.3f, want %.2f", tt.fg, tt.bg, got, tt.want)
		}
	}
}

func TestNormalizeThemeCanonicalizes(t *testing.T) {
	got, err := instance.NormalizeTheme([]instance.ThemeTokenValue{
		{Token: "radius-control", Value: " 4px "},
		{Token: "color-accent", Light: "#FF8A80", Dark: "#FF8A80"},
		{Token: "font-sans", Value: `"Noto Sans JP",  sans-serif`},
		{Token: "duration-spin", Value: "0.5s"},
		{Token: "text-body--line-height", Value: "1.8"}, //nolint:gosec // a design token, not a credential
	}, "a { color: red }")
	if err != nil {
		t.Fatal(err)
	}
	want := []instance.ThemeTokenValue{
		{Token: "font-sans", Value: `"Noto Sans JP", sans-serif`},
		{Token: "text-body--line-height", Value: "1.8"}, //nolint:gosec // a design token, not a credential
		{Token: "color-accent", Light: "#ff8a80", Dark: "#ff8a80"},
		{Token: "radius-control", Value: "4px"},
		{Token: "duration-spin", Value: "0.5s"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestNormalizeThemeRejects(t *testing.T) {
	for _, tt := range []struct {
		name  string
		token instance.ThemeTokenValue
		code  instance.ThemeProblemCode
	}{
		{"unknown token", instance.ThemeTokenValue{Token: "palette-blue-600", Value: "#000000"}, instance.ThemeUnknownToken},
		{"fixed shadow", instance.ThemeTokenValue{Token: "shadow-overlay", Value: "none"}, instance.ThemeUnknownToken},
		{"fixed avatar backing", instance.ThemeTokenValue{Token: "color-avatar-backing", Light: "#000000", Dark: "#000000"}, instance.ThemeUnknownToken},
		{"fixed component size", instance.ThemeTokenValue{Token: "control-form-height", Value: "2rem"}, instance.ThemeUnknownToken},
		{"named color", instance.ThemeTokenValue{Token: "color-accent", Light: "red", Dark: "#000000"}, instance.ThemeInvalidValue},
		{"missing dark", instance.ThemeTokenValue{Token: "color-accent", Light: "#155dfc"}, instance.ThemeInvalidValue},
		{"translucent color", instance.ThemeTokenValue{Token: "color-surface", Light: "#ffffff80", Dark: "#000000"}, instance.ThemeInvalidValue},
		{"injection in color", instance.ThemeTokenValue{Token: "color-accent", Light: "#155dfc;}", Dark: "#155dfc"}, instance.ThemeInvalidValue},
		{"length unit", instance.ThemeTokenValue{Token: "radius-control", Value: "1em"}, instance.ThemeInvalidValue},
		{"length injection", instance.ThemeTokenValue{Token: "radius-control", Value: "1px;} body{display:none"}, instance.ThemeInvalidValue},
		{"radius too large", instance.ThemeTokenValue{Token: "radius-control", Value: "40px"}, instance.ThemeOutOfRange},
		{"text too small", instance.ThemeTokenValue{Token: "text-caption", Value: "8px"}, instance.ThemeOutOfRange},
		{"line height", instance.ThemeTokenValue{Token: "text-body--line-height", Value: "3"}, instance.ThemeOutOfRange}, //nolint:gosec // a design token, not a credential
		{"duration", instance.ThemeTokenValue{Token: "duration-spin", Value: "2s"}, instance.ThemeOutOfRange},
		{"font url", instance.ThemeTokenValue{Token: "font-sans", Value: `url(https://example.com/f.woff)`}, instance.ThemeInvalidValue},
		{"font comment", instance.ThemeTokenValue{Token: "font-sans", Value: `a /* b */`}, instance.ThemeInvalidValue},
		{"font escape", instance.ThemeTokenValue{Token: "font-sans", Value: `"a\"; }"`}, instance.ThemeInvalidValue},
		{"empty font", instance.ThemeTokenValue{Token: "font-sans", Value: ""}, instance.ThemeInvalidValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := instance.NormalizeTheme([]instance.ThemeTokenValue{tt.token}, "")
			ps := problemsOf(t, err)
			if len(ps) == 0 || ps[0].Code != tt.code || ps[0].Token != tt.token.Token {
				t.Errorf("problems = %+v, want %s for %s", ps, tt.code, tt.token.Token)
			}
		})
	}
}

func TestNormalizeThemeRejectsDuplicates(t *testing.T) {
	v := instance.ThemeTokenValue{Token: "radius-control", Value: "4px"}
	ps := problemsOf(t, func() error { _, err := instance.NormalizeTheme([]instance.ThemeTokenValue{v, v}, ""); return err }())
	if len(ps) != 1 || ps[0].Code != instance.ThemeDuplicateToken {
		t.Errorf("problems = %+v", ps)
	}
}

func TestNormalizeThemeChecksContrastWithDefaults(t *testing.T) {
	// A dark light-scheme surface fails under the default text (near black) and the other default
	// foregrounds, in the light scheme only.
	_, err := instance.NormalizeTheme([]instance.ThemeTokenValue{{Token: "color-surface", Light: "#222222", Dark: "#221c1b"}}, "")
	ps := problemsOf(t, err)
	var pairs []string
	for _, p := range ps {
		if p.Code != instance.ThemeLowContrast || p.Scheme != instance.SchemeLight || p.Ratio >= p.Minimum {
			t.Errorf("unexpected problem %+v", p)
		}
		pairs = append(pairs, p.Token+"/"+p.Background)
	}
	for _, want := range []string{"color-text/color-surface", "color-accent-text/color-surface"} {
		if !slices.Contains(pairs, want) {
			t.Errorf("pairs %v lack %s", pairs, want)
		}
	}
	// Changing the text along with it fixes the pair of the text on the surface; the pairs with the
	// default muted text still fail.
	_, err = instance.NormalizeTheme([]instance.ThemeTokenValue{
		{Token: "color-surface", Light: "#222222", Dark: "#221c1b"},
		{Token: "color-text", Light: "#ffffff", Dark: "#f2e9e7"},
	}, "")
	ps = problemsOf(t, err)
	for _, p := range ps {
		if p.Token == "color-text" && p.Background == "color-surface" {
			t.Errorf("text on surface still fails: %+v", p)
		}
	}
	if !slices.ContainsFunc(ps, func(p instance.ThemeProblem) bool { return p.Token == "color-muted-text" }) {
		t.Errorf("problems %+v lack the muted text", ps)
	}
}

func TestNormalizeThemeChecksCustomCSS(t *testing.T) {
	if _, err := instance.NormalizeTheme(nil, strings.Repeat("a", instance.MaxCustomCSSBytes)); err != nil {
		t.Errorf("CSS at the limit: %v", err)
	}
	for css, code := range map[string]instance.ThemeProblemCode{
		strings.Repeat("a", instance.MaxCustomCSSBytes+1): instance.ThemeCSSTooLong,
		"a\x00":   instance.ThemeCSSInvalid,
		"a\xff{}": instance.ThemeCSSInvalid,
	} {
		ps := problemsOf(t, func() error { _, err := instance.NormalizeTheme(nil, css); return err }())
		if len(ps) != 1 || ps[0].Code != code {
			t.Errorf("problems = %+v, want %s", ps, code)
		}
	}
}

func TestThemeStylesheet(t *testing.T) {
	r := instance.ThemeRevision{
		ID: testID(1),
		Tokens: []instance.ThemeTokenValue{
			{Token: "color-surface", Light: "#fffdf5", Dark: "#101010"},
			{Token: "radius-control", Value: "2px"},
		},
		CustomCSS: ".x { color: red }",
	}
	want := "/* The theme of this server (revision " + testID(1) + "). */\n" +
		":root {\n  --color-surface: light-dark(#fffdf5, #101010);\n  --radius-control: 2px;\n}\n" +
		".x { color: red }\n"
	if got := string(r.Stylesheet()); got != want {
		t.Errorf("Stylesheet() =\n%s\nwant\n%s", got, want)
	}
	if got := string((instance.ThemeRevision{ID: testID(1), CustomCSS: "a{}\n"}).Stylesheet()); strings.Contains(got, ":root") {
		t.Errorf("a theme without tokens has a :root rule: %s", got)
	}
}

func TestThemeTokenRows(t *testing.T) {
	values := []instance.ThemeTokenValue{
		{Token: "radius-control", Value: "2px"},
		{Token: "color-text", Light: "#000000", Dark: "#ffffff"},
	}
	var rows []instance.ThemeTokenRow
	for _, v := range values {
		rows = append(rows, v.Rows()...)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	got := instance.ThemeTokenValuesFromRows(rows)
	// Back in the order of the tokens: the colors come before the radii in @my/design-tokens.
	if !slices.Equal(got, []instance.ThemeTokenValue{values[1], values[0]}) {
		t.Errorf("ThemeTokenValuesFromRows = %+v", got)
	}
}

func TestThemeColor(t *testing.T) {
	tokens := []instance.ThemeTokenValue{{Token: "color-surface", Light: "#fffdf5", Dark: "#101010"}}
	if got := instance.ThemeColor(tokens, "color-surface", instance.SchemeDark); got != "#101010" {
		t.Errorf("overridden = %s", got)
	}
	def, _ := instance.LookupThemeToken("color-text")
	if got := instance.ThemeColor(tokens, "color-text", instance.SchemeLight); got != def.Light {
		t.Errorf("default = %s, want %s", got, def.Light)
	}
	if got := instance.ThemeColor(tokens, "radius-control", instance.SchemeLight); got != "" {
		t.Errorf("non-color = %q", got)
	}
}
