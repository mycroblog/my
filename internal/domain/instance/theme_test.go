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
	if _, err := instance.NormalizeTheme(nil); err != nil {
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
	if _, err := instance.NormalizeTheme(tokens); err != nil {
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

// tokenValue is the value of a token that is not a color. Built here, gosec's G101, which takes a field named Token
// set to a constant for a credential, does not read the table's font values as one.
func tokenValue(token, value string) instance.ThemeTokenValue {
	return instance.ThemeTokenValue{Token: token, Value: value}
}

func TestNormalizeThemeCanonicalizes(t *testing.T) {
	got, err := instance.NormalizeTheme([]instance.ThemeTokenValue{
		{Token: "radius-control", Value: " 4px "},
		{Token: "color-accent", Light: "#FF8A80", Dark: "#FF8A80"},
		tokenValue("font-sans-override", `"Noto Sans JP",  Inter`),
		{Token: "duration-spin", Value: "0.5s"},
		{Token: "text-body--line-height", Value: "1.8"}, //nolint:gosec // a design token, not a credential
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []instance.ThemeTokenValue{
		tokenValue("font-sans-override", `"Noto Sans JP", Inter`),
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
		{"fixed data color", instance.ThemeTokenValue{Token: "color-data-1", Light: "#000000", Dark: "#000000"}, instance.ThemeUnknownToken},
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
		{"font url", tokenValue("font-sans-override", `url(https://example.com/f.woff)`), instance.ThemeInvalidValue},
		{"font comment", tokenValue("font-sans-override", `a /* b */`), instance.ThemeInvalidValue},
		{"font escape", tokenValue("font-sans-override", `"a\"; }"`), instance.ThemeInvalidValue},
		{"empty font", tokenValue("font-sans-override", ""), instance.ThemeInvalidValue},
		// The stacks are fixed (ADR-0059 §2): a theme adds its fonts through the overrides alone.
		{"fixed sans stack", tokenValue("font-sans", `"Noto Sans JP", sans-serif`), instance.ThemeUnknownToken},
		{"fixed mono stack", tokenValue("font-mono", `"Fira Code", monospace`), instance.ThemeUnknownToken},
		// An override names families: a generic family would draw before the emoji and CJK fonts' fallbacks, and a
		// CSS-wide keyword would break the stack it sits in.
		{"generic family", tokenValue("font-sans-override", "sans-serif"), instance.ThemeInvalidValue},
		{"generic family in a list", tokenValue("font-sans-override", "Inter, system-ui"), instance.ThemeInvalidValue},
		{"generic mono family", tokenValue("font-mono-override", "monospace"), instance.ThemeInvalidValue},
		{"keyword inherit", tokenValue("font-sans-override", "inherit"), instance.ThemeInvalidValue},
		{"keyword initial", tokenValue("font-sans-override", "initial"), instance.ThemeInvalidValue},
		{"keyword unset", tokenValue("font-mono-override", "unset"), instance.ThemeInvalidValue},
		{"keyword revert", tokenValue("font-sans-override", "revert"), instance.ThemeInvalidValue},
		{"keyword revert-layer", tokenValue("font-sans-override", "revert-layer"), instance.ThemeInvalidValue},
		{"keyword in capitals", tokenValue("font-sans-override", "INHERIT"), instance.ThemeInvalidValue},
		// Every word of an unquoted name is an identifier and no CSS-wide keyword, or the declaration that holds the
		// stack is invalid and the page draws with the browser's default font.
		{"word starting with a digit", tokenValue("font-sans-override", "M PLUS 1p"), instance.ThemeInvalidValue},
		{"number word", tokenValue("font-sans-override", "Font Awesome 6 Free"), instance.ThemeInvalidValue},
		{"keyword as a later word", tokenValue("font-sans-override", "Foo inherit"), instance.ThemeInvalidValue},
		{"default as a later word", tokenValue("font-mono-override", "Arial Default"), instance.ThemeInvalidValue},
		{"keyword word in a list", tokenValue("font-sans-override", "Inter, Noto unset"), instance.ThemeInvalidValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := instance.NormalizeTheme([]instance.ThemeTokenValue{tt.token})
			ps := problemsOf(t, err)
			if len(ps) == 0 || ps[0].Code != tt.code || ps[0].Token != tt.token.Token {
				t.Errorf("problems = %+v, want %s for %s", ps, tt.code, tt.token.Token)
			}
		})
	}
}

func TestNormalizeThemeAcceptsFontOverrideFamilies(t *testing.T) {
	// A list of family names, quoted or not, and a quoted name that only looks like a generic family or a keyword.
	for _, tt := range []instance.ThemeTokenValue{
		tokenValue("font-sans-override", `"Liberation Sans", Inter, "Noto Sans JP"`),
		tokenValue("font-sans-override", `"sans-serif"`),
		tokenValue("font-sans-override", `"inherit"`),
		tokenValue("font-mono-override", `"Fira Code", Menlo`),
		tokenValue("font-sans-override", `'Noto Sans JP', "Liberation Sans"`),
		tokenValue("font-sans-override", `"M PLUS 1p", "Foo inherit"`),
		tokenValue("font-sans-override", `Foo serif, -apple-system`),
	} {
		t.Run(tt.Token+" "+tt.Value, func(t *testing.T) {
			got, err := instance.NormalizeTheme([]instance.ThemeTokenValue{tt})
			if err != nil {
				t.Fatalf("NormalizeTheme(%s: %s) = %v", tt.Token, tt.Value, err)
			}
			if len(got) != 1 || got[0].Token != tt.Token {
				t.Errorf("got %+v, want the one override %s", got, tt.Token)
			}
		})
	}
}

func TestNormalizeThemeRejectsDuplicates(t *testing.T) {
	v := instance.ThemeTokenValue{Token: "radius-control", Value: "4px"}
	ps := problemsOf(t, func() error { _, err := instance.NormalizeTheme([]instance.ThemeTokenValue{v, v}); return err }())
	if len(ps) != 1 || ps[0].Code != instance.ThemeDuplicateToken {
		t.Errorf("problems = %+v", ps)
	}
}

func TestNormalizeThemeBoundsTheSet(t *testing.T) {
	tokens := make([]instance.ThemeTokenValue, instance.MaxThemeOverrides+1)
	for i := range tokens {
		tokens[i] = instance.ThemeTokenValue{Token: "radius-control", Value: "4px"}
	}
	ps := problemsOf(t, func() error { _, err := instance.NormalizeTheme(tokens); return err }())
	if len(ps) != 1 || ps[0].Code != instance.ThemeTooManyOverrides {
		t.Errorf("problems = %+v", ps)
	}
}

func TestNormalizeThemeChecksContrastWithDefaults(t *testing.T) {
	// A dark light-scheme surface fails under the default text (near black) and the other default
	// foregrounds, in the light scheme only.
	_, err := instance.NormalizeTheme([]instance.ThemeTokenValue{{Token: "color-surface", Light: "#222222", Dark: "#221c1b"}})
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
	})
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

func TestThemeSetDocumentRoundTrips(t *testing.T) {
	set := instance.ThemeSet{ID: testID(1), Tokens: []instance.ThemeTokenValue{
		{Token: "color-surface", Light: "#fffdf5", Dark: "#101010"},
		{Token: "radius-control", Value: "2px"},
	}}
	doc := set.Document()
	if want := `{"version":1,"tokens":[{"token":"color-surface","light":"#fffdf5","dark":"#101010"},{"token":"radius-control","value":"2px"}]}`; string(doc) != want {
		t.Errorf("Document() = %s\nwant %s", doc, want)
	}
	tokens, stale, err := instance.ReadThemeSet(doc)
	if err != nil || !slices.Equal(tokens, set.Tokens) || len(stale) != 0 {
		t.Errorf("ReadThemeSet = %+v, %+v, %v", tokens, stale, err)
	}
}

// An override that no longer applies after an upgrade is dropped on read and kept as stale,
// never failing the read; the document keeps it.
func TestReadThemeSetDropsWhatNoLongerApplies(t *testing.T) {
	doc := []byte(`{"version":1,"tokens":[
		{"token":"elevation-card","value":"0 1px 2px #000"},
		{"token":"radius-control","value":"2px"},
		{"token":"radius-control","value":"3px"},
		{"token":"radius-overlay","value":"9rem"},
		{"token":"color-accent","value":"#ff0000"},
		{"token":"color-surface","light":"#FFFDF5","dark":"#101010"}
	]}`)
	tokens, stale, err := instance.ReadThemeSet(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []instance.ThemeTokenValue{
		{Token: "color-surface", Light: "#fffdf5", Dark: "#101010"},
		{Token: "radius-control", Value: "2px"},
	}
	if !slices.Equal(tokens, want) {
		t.Errorf("tokens = %+v, want %+v", tokens, want)
	}
	reasons := map[string]instance.ThemeProblemCode{}
	for _, s := range stale {
		reasons[s.Token+"="+s.Value] = s.Reason
	}
	wantReasons := map[string]instance.ThemeProblemCode{
		"elevation-card=0 1px 2px #000": instance.ThemeUnknownToken,
		"radius-control=3px":            instance.ThemeDuplicateToken,
		"radius-overlay=9rem":           instance.ThemeOutOfRange,
		"color-accent=#ff0000":          instance.ThemeInvalidValue,
	}
	if len(reasons) != len(wantReasons) {
		t.Errorf("stale = %+v", stale)
	}
	for k, v := range wantReasons {
		if reasons[k] != v {
			t.Errorf("stale %s = %q, want %q", k, reasons[k], v)
		}
	}
	// The stale overrides stay in the document of the set.
	again, staleAgain, err := instance.ReadThemeSet(instance.ThemeSet{Tokens: tokens, Stale: stale}.Document())
	if err != nil || !slices.Equal(again, want) || len(staleAgain) != len(stale) {
		t.Errorf("read again = %+v, %+v, %v", again, staleAgain, err)
	}
	if strings.Contains(string(instance.ThemeRevision{Set: instance.ThemeSet{Tokens: tokens, Stale: stale}}.Stylesheet()), "elevation") {
		t.Error("the stylesheet has a stale override")
	}
}

func TestReadThemeSetRefusesWhatIsNotADocument(t *testing.T) {
	for _, doc := range []string{``, `[]`, `{"version":2,"tokens":[]}`, `{"tokens":[]}`} {
		if _, _, err := instance.ReadThemeSet([]byte(doc)); err == nil {
			t.Errorf("ReadThemeSet(%s) = nil error", doc)
		}
	}
}

func TestThemeStylesheet(t *testing.T) {
	set := instance.ThemeSet{ID: testID(2), Tokens: []instance.ThemeTokenValue{
		{Token: "color-surface", Light: "#fffdf5", Dark: "#101010"},
		{Token: "radius-control", Value: "2px"},
	}}
	r := instance.ThemeRevision{ThemePublication: instance.ThemePublication{ID: testID(1), SetID: set.ID}, Set: set}
	rules := ":root {\n  --color-surface: light-dark(#fffdf5, #101010);\n  --radius-control: 2px;\n}\n"
	if got, want := string(r.Stylesheet()), "/* The theme of this server (revision "+testID(1)+"). */\n"+rules; got != want {
		t.Errorf("Stylesheet() =\n%s\nwant\n%s", got, want)
	}
	if got, want := string(set.Stylesheet()), "/* A draft theme of this server (set "+testID(2)+"), not published. */\n"+rules; got != want {
		t.Errorf("ThemeSet.Stylesheet() =\n%s\nwant\n%s", got, want)
	}
	if got := string((instance.ThemeSet{ID: testID(2)}).Stylesheet()); strings.Contains(got, ":root") {
		t.Errorf("a set without overrides has a :root rule: %s", got)
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
