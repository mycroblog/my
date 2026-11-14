// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The instance theme (ADR-0059, FR-605): one set of overrides of the web client's design tokens per
// server, stored as one versioned document and replaced as a whole (§15). The overridable tokens,
// their kinds, defaults, and bounds, and the contrast pairs of the colors are generated from
// @my/design-tokens (theme_tokens.gen.go, committed).

// ThemeTokenKind is the kind of value a token holds, which decides the shape of its overrides.
type ThemeTokenKind string

// Kinds of theme tokens.
const (
	// ThemeTokenColor is a color with a value per color scheme, each #rrggbb.
	ThemeTokenColor ThemeTokenKind = "color"
	// ThemeTokenLength is 0 or a rem or px length, bounded in rem (16px each).
	ThemeTokenLength ThemeTokenKind = "length"
	// ThemeTokenNumber is a unitless number, such as a line height.
	ThemeTokenNumber ThemeTokenKind = "number"
	// ThemeTokenFontFamily is a comma-separated list of font family names.
	ThemeTokenFontFamily ThemeTokenKind = "font-family"
	// ThemeTokenDuration is a time in ms or s, bounded in ms.
	ThemeTokenDuration ThemeTokenKind = "duration"
)

// Limits of a theme.
const (
	// MaxFontFamilies bounds the families of a font-family value.
	MaxFontFamilies = 30
	// MaxFontFamilyLength bounds a font-family value, in characters.
	MaxFontFamilyLength = 1000
)

// ThemeToken is a design token that a theme may override, named without the leading "--".
type ThemeToken struct {
	Name string
	Kind ThemeTokenKind
	// Light and Dark are the defaults of a color token, as #rrggbb.
	Light, Dark string
	// Default is the default of a token of another kind, as @my/design-tokens writes it.
	Default string
	// Min and Max bound a length (in rem), a number, or a duration (in ms), inclusive. Both are 0
	// for the other kinds.
	Min, Max float64
}

// ContrastPair is a foreground color token drawn on a background color token, and the minimum
// contrast ratio it needs in both color schemes.
type ContrastPair struct {
	FG, BG  string
	Minimum float64
}

// ThemeTokens returns the overridable tokens, in the order of @my/design-tokens.
func ThemeTokens() []ThemeToken { return slices.Clone(themeTokens) }

// ThemeContrastPairs returns the contrast pairs of the color tokens.
func ThemeContrastPairs() []ContrastPair { return slices.Clone(themeContrastPairs) }

// LookupThemeToken returns the token named name, or false when it is not overridable.
func LookupThemeToken(name string) (ThemeToken, bool) {
	i := slices.IndexFunc(themeTokens, func(t ThemeToken) bool { return t.Name == name })
	if i < 0 {
		return ThemeToken{}, false
	}
	return themeTokens[i], true
}

// ColorScheme is the color scheme of a color value or a contrast pair.
type ColorScheme string

// Color schemes. SchemeAll is the scheme of a value of another kind in the rows of the releases
// that stored a theme row by row (ThemeTokenRow).
const (
	SchemeLight ColorScheme = "light"
	SchemeDark  ColorScheme = "dark"
	SchemeAll   ColorScheme = "all"
)

// ThemeTokenValue is the override of one token: Light and Dark for a color token, Value for the
// other kinds.
type ThemeTokenValue struct {
	Token string `json:"token"`
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
	Value string `json:"value,omitempty"`
}

// ThemeTokenRow is the override of a token in one color scheme, as the releases before the token
// set stored it (instance_theme_tokens), which the data upgrade step theme.token_sets reads.
type ThemeTokenRow struct {
	Token  string
	Scheme ColorScheme
	Value  string
}

// ThemeTokenValuesFromRows folds stored rows back into token values, in the order of the tokens,
// then the names the list no longer has, by name.
func ThemeTokenValuesFromRows(rows []ThemeTokenRow) []ThemeTokenValue {
	byToken := map[string]*ThemeTokenValue{}
	for _, r := range rows {
		v, ok := byToken[r.Token]
		if !ok {
			v = &ThemeTokenValue{Token: r.Token}
			byToken[r.Token] = v
		}
		switch r.Scheme {
		case SchemeLight:
			v.Light = r.Value
		case SchemeDark:
			v.Dark = r.Value
		default:
			v.Value = r.Value
		}
	}
	out := make([]ThemeTokenValue, 0, len(byToken))
	for _, t := range themeTokens {
		if v, ok := byToken[t.Name]; ok {
			out = append(out, *v)
			delete(byToken, t.Name)
		}
	}
	rest := make([]string, 0, len(byToken))
	for name := range byToken {
		rest = append(rest, name)
	}
	slices.Sort(rest)
	for _, name := range rest {
		out = append(out, *byToken[name])
	}
	return out
}

// ThemeSetVersion is the version of the document of a token set (ThemeSet.Document).
const ThemeSetVersion = 1

// MaxThemeOverrides bounds the overrides of a token set.
const MaxThemeOverrides = 200

// ThemeSet is one saved token set of the theme (instance_theme_sets): the overrides of the light and
// dark values, saved as a draft (§15). A set never changes; saving makes a new one.
type ThemeSet struct {
	ID string
	// Tokens are the overrides that apply, in the order of ThemeTokens.
	Tokens []ThemeTokenValue
	// Stale are the stored overrides that no longer apply, which the stylesheet leaves out and the
	// administrator is shown (ReadThemeSet).
	Stale []StaleOverride
	// SavedByUserID is the user who saved the set, or "" for the CLI and the data upgrade.
	SavedByUserID string
	SavedAt       time.Time
}

// StaleOverride is a stored override that no longer applies: its token left the list, or its value
// no longer has the shape or the bounds of its token.
type StaleOverride struct {
	ThemeTokenValue
	// Reason is ThemeUnknownToken, ThemeDuplicateToken, ThemeInvalidValue, or ThemeOutOfRange.
	Reason ThemeProblemCode
}

// Empty reports whether the set overrides nothing, so the web client needs no stylesheet.
func (s ThemeSet) Empty() bool { return len(s.Tokens) == 0 }

// themeSetDocument is the stored form of a token set.
type themeSetDocument struct {
	Version int               `json:"version"`
	Tokens  []ThemeTokenValue `json:"tokens"`
}

// Document returns the stored form of the set: {"version": 1, "tokens": [...]}, the overrides
// that apply and then the stale ones, each {"token", "light", "dark"} or {"token", "value"}.
func (s ThemeSet) Document() []byte {
	tokens := make([]ThemeTokenValue, 0, len(s.Tokens)+len(s.Stale))
	tokens = append(tokens, s.Tokens...)
	for _, o := range s.Stale {
		tokens = append(tokens, o.ThemeTokenValue)
	}
	b, err := json.Marshal(themeSetDocument{Version: ThemeSetVersion, Tokens: tokens})
	if err != nil {
		panic(fmt.Sprintf("instance: encode a theme set: %v", err)) // strings only; never fails
	}
	return b
}

// ReadThemeSet decodes the stored overrides of a set (ThemeSet.Document) and drops those that no
// longer apply into the stale ones, so that a token renamed or retired by an upgrade never fails a
// read (§15). Contrast is not checked again: it is a property of the whole set when it was saved.
// It fails only on a document that is not one of a version this release reads.
func ReadThemeSet(doc []byte) (tokens []ThemeTokenValue, stale []StaleOverride, err error) {
	var d themeSetDocument
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, nil, fmt.Errorf("instance: read a theme set: %w", err)
	}
	if d.Version != ThemeSetVersion {
		return nil, nil, fmt.Errorf("instance: read a theme set: version %d, want %d", d.Version, ThemeSetVersion)
	}
	byToken := map[string]ThemeTokenValue{}
	for _, v := range d.Tokens {
		t, ok := LookupThemeToken(v.Token)
		switch {
		case !ok:
			stale = append(stale, StaleOverride{ThemeTokenValue: v, Reason: ThemeUnknownToken})
			continue
		case hasToken(byToken, v.Token):
			stale = append(stale, StaleOverride{ThemeTokenValue: v, Reason: ThemeDuplicateToken})
			continue
		}
		out, problems := normalizeTokenValue(t, v)
		if len(problems) > 0 {
			stale = append(stale, StaleOverride{ThemeTokenValue: v, Reason: problems[0].Code})
			continue
		}
		byToken[t.Name] = out
	}
	return inTokenOrder(byToken), stale, nil
}

func hasToken(m map[string]ThemeTokenValue, name string) bool {
	_, ok := m[name]
	return ok
}

// inTokenOrder returns the values of byToken in the order of ThemeTokens.
func inTokenOrder(byToken map[string]ThemeTokenValue) []ThemeTokenValue {
	out := make([]ThemeTokenValue, 0, len(byToken))
	for _, t := range themeTokens {
		if v, ok := byToken[t.Name]; ok {
			out = append(out, v)
		}
	}
	return out
}

// ThemePublication is one publication of a token set (instance_theme_publications): from then on
// the set is the theme in effect. Its ID is the revision of the theme, which names its stylesheet.
type ThemePublication struct {
	ID    string
	SetID string
	// PublishedByUserID is the user who published the set, or "" for the CLI and the data upgrade.
	PublishedByUserID string
	PublishedAt       time.Time
}

// ThemeRevision is a publication with its set: the theme in effect from PublishedAt until the next
// publication. The publication with the greatest ID is the current theme.
type ThemeRevision struct {
	ThemePublication
	Set ThemeSet
}

// Empty reports whether the revision overrides nothing, so the web client needs no stylesheet.
func (r ThemeRevision) Empty() bool { return r.Set.Empty() }

// ThemeHead points at the current theme revision (the instance_theme projection).
type ThemeHead struct {
	RevisionID string
}

// Supersedes reports whether r should replace the head h, which is the zero value when no set
// has been published.
func (r ThemeRevision) Supersedes(h ThemeHead) bool {
	return h.RevisionID == "" || r.ID > h.RevisionID
}

// ThemeProblemCode says what is wrong with a theme.
type ThemeProblemCode string

// Theme problem codes.
const (
	// ThemeUnknownToken: the token is not overridable.
	ThemeUnknownToken ThemeProblemCode = "unknown_token"
	// ThemeDuplicateToken: the token is overridden twice.
	ThemeDuplicateToken ThemeProblemCode = "duplicate_token"
	// ThemeInvalidValue: the value does not have the shape of the token's kind.
	ThemeInvalidValue ThemeProblemCode = "invalid_value"
	// ThemeOutOfRange: the value is outside the token's bounds (ThemeProblem.Min, Max).
	ThemeOutOfRange ThemeProblemCode = "out_of_range"
	// ThemeLowContrast: a contrast pair is below its minimum in a color scheme.
	ThemeLowContrast ThemeProblemCode = "low_contrast"
	// ThemeTooManyOverrides: the set has more than MaxThemeOverrides overrides.
	ThemeTooManyOverrides ThemeProblemCode = "too_many_overrides"
)

// ThemeProblem is one reason a theme cannot be saved.
type ThemeProblem struct {
	Code ThemeProblemCode
	// Token is the token the problem is about (the foreground of a contrast pair), "" for the set.
	Token string
	// Scheme is the color scheme of a color value or a contrast pair, "" otherwise.
	Scheme ColorScheme
	// Background is the background token of a contrast pair.
	Background string
	// Ratio and Minimum are the contrast of the pair and the minimum it needs.
	Ratio, Minimum float64
	// Min and Max are the bounds of an out-of-range value, in rem for a length and ms for a
	// duration; Max is MaxThemeOverrides for too many overrides.
	Min, Max float64
}

func (p ThemeProblem) String() string {
	switch p.Code {
	case ThemeLowContrast:
		return fmt.Sprintf("%s on %s is %.2f:1 in the %s scheme; it needs %g:1", p.Token, p.Background, p.Ratio, p.Scheme, p.Minimum)
	case ThemeOutOfRange:
		return fmt.Sprintf("%s is outside %g..%g", p.Token, p.Min, p.Max)
	case ThemeTooManyOverrides:
		return fmt.Sprintf("more than %g overrides", p.Max)
	}
	if p.Scheme != "" {
		return fmt.Sprintf("%s (%s): %s", p.Token, p.Scheme, p.Code)
	}
	return fmt.Sprintf("%s: %s", p.Token, p.Code)
}

// ThemeError lists every reason a theme cannot be saved. It wraps ErrInvalidValue.
type ThemeError struct {
	Problems []ThemeProblem
}

func (e *ThemeError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.String()
	}
	return "theme: " + strings.Join(parts, "; ")
}

// Unwrap makes errors.Is(err, ErrInvalidValue) hold.
func (e *ThemeError) Unwrap() error { return ErrInvalidValue }

// NormalizeTheme validates a whole token set (ADR-0059 §4) and returns its overrides in their
// canonical form (lowercase colors, single spaces), in the order of ThemeTokens. It fails with a
// *ThemeError that lists every problem: too many overrides, unknown or repeated tokens, values
// without the shape or outside the bounds of their kind, and contrast pairs below their minimum in
// either color scheme with the defaults of the tokens that are not overridden.
func NormalizeTheme(tokens []ThemeTokenValue) ([]ThemeTokenValue, error) {
	if len(tokens) > MaxThemeOverrides {
		return nil, &ThemeError{Problems: []ThemeProblem{{Code: ThemeTooManyOverrides, Max: MaxThemeOverrides}}}
	}
	var problems []ThemeProblem
	byToken := map[string]ThemeTokenValue{}
	seen := map[string]bool{}
	for _, v := range tokens {
		t, ok := LookupThemeToken(v.Token)
		switch {
		case !ok:
			problems = append(problems, ThemeProblem{Code: ThemeUnknownToken, Token: v.Token})
			continue
		case seen[v.Token]:
			problems = append(problems, ThemeProblem{Code: ThemeDuplicateToken, Token: v.Token})
			continue
		}
		seen[v.Token] = true
		out, ps := normalizeTokenValue(t, v)
		problems = append(problems, ps...)
		if len(ps) == 0 {
			byToken[t.Name] = out
		}
	}
	out := inTokenOrder(byToken)
	// Contrast is checked only when every value parsed, so that a malformed color is reported once.
	if len(problems) == 0 {
		problems = append(problems, checkContrast(out)...)
	}
	if len(problems) > 0 {
		return nil, &ThemeError{Problems: problems}
	}
	return out, nil
}

func normalizeTokenValue(t ThemeToken, v ThemeTokenValue) (ThemeTokenValue, []ThemeProblem) {
	out := ThemeTokenValue{Token: t.Name}
	if t.Kind == ThemeTokenColor {
		var problems []ThemeProblem
		for _, side := range []struct {
			scheme ColorScheme
			in     string
			out    *string
		}{{SchemeLight, v.Light, &out.Light}, {SchemeDark, v.Dark, &out.Dark}} {
			c, ok := normalizeColor(side.in)
			if !ok {
				problems = append(problems, ThemeProblem{Code: ThemeInvalidValue, Token: t.Name, Scheme: side.scheme})
				continue
			}
			*side.out = c
		}
		return out, problems
	}
	value, code, ok := normalizeValue(t, v.Value)
	if !ok {
		p := ThemeProblem{Code: code, Token: t.Name}
		if code == ThemeOutOfRange {
			p.Min, p.Max = t.Min, t.Max
		}
		return out, []ThemeProblem{p}
	}
	out.Value = value
	return out, nil
}

var (
	hexColorPattern     = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	lengthPattern       = regexp.MustCompile(`^(?:0|(\d{1,3}(?:\.\d{1,4})?|\.\d{1,4})(rem|px))$`)
	numberPattern       = regexp.MustCompile(`^\d{1,2}(?:\.\d{1,4})?$`)
	durationPattern     = regexp.MustCompile(`^(\d{1,5}(?:\.\d{1,3})?)(ms|s)$`)
	quotedFamilyPattern = regexp.MustCompile(`^"[^"\\;{}<>\x00-\x1f]{1,100}"$`)
	// An unquoted family name is a CSS identifier, which may start with a hyphen (-apple-system).
	plainFamilyPattern = regexp.MustCompile(`^-?[A-Za-z][A-Za-z0-9-]*(?: [A-Za-z0-9-]+)*$`)
)

func normalizeColor(s string) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(s))
	return c, hexColorPattern.MatchString(c)
}

// lengthRem parses a length value in rem; px counts 16 to the rem.
func lengthRem(m []string) float64 {
	if m[1] == "" {
		return 0 // the bare 0
	}
	n, _ := strconv.ParseFloat(m[1], 64)
	if m[2] == "px" {
		return n / 16
	}
	return n
}

func inRange(t ThemeToken, n float64) bool { return n >= t.Min && n <= t.Max }

// normalizeValue returns the canonical form of the value of a token that is not a color, or the
// problem with it.
func normalizeValue(t ThemeToken, s string) (string, ThemeProblemCode, bool) {
	v := strings.Join(strings.Fields(s), " ")
	switch t.Kind {
	case ThemeTokenLength:
		m := lengthPattern.FindStringSubmatch(v)
		if m == nil {
			return "", ThemeInvalidValue, false
		}
		if !inRange(t, lengthRem(m)) {
			return "", ThemeOutOfRange, false
		}
	case ThemeTokenNumber:
		if !numberPattern.MatchString(v) {
			return "", ThemeInvalidValue, false
		}
		n, _ := strconv.ParseFloat(v, 64)
		if !inRange(t, n) {
			return "", ThemeOutOfRange, false
		}
	case ThemeTokenDuration:
		m := durationPattern.FindStringSubmatch(v)
		if m == nil {
			return "", ThemeInvalidValue, false
		}
		n, _ := strconv.ParseFloat(m[1], 64)
		if m[2] == "s" {
			n *= 1000
		}
		if !inRange(t, n) {
			return "", ThemeOutOfRange, false
		}
	case ThemeTokenFontFamily:
		if !validFontFamily(v) {
			return "", ThemeInvalidValue, false
		}
	default:
		return "", ThemeInvalidValue, false
	}
	return v, "", true
}

func validFontFamily(v string) bool {
	if v == "" || utf8.RuneCountInString(v) > MaxFontFamilyLength {
		return false
	}
	families := strings.Split(v, ",")
	if len(families) > MaxFontFamilies {
		return false
	}
	for _, f := range families {
		f = strings.TrimSpace(f)
		if !quotedFamilyPattern.MatchString(f) && (!plainFamilyPattern.MatchString(f) || len(f) > 100) {
			return false
		}
	}
	return true
}

// rgba is a color in linear-light sRGB with straight alpha.
type rgba struct{ r, g, b, a float64 }

func parseHexColor(s string) rgba {
	channel := func(i int) float64 {
		n, _ := strconv.ParseUint(s[i:i+2], 16, 8)
		return float64(n) / 255
	}
	c := rgba{r: toLinear(channel(1)), g: toLinear(channel(3)), b: toLinear(channel(5)), a: 1}
	if len(s) == 9 {
		c.a = channel(7)
	}
	return c
}

func toLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func toGamma(c float64) float64 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

func luminance(c rgba) float64 { return 0.2126*c.r + 0.7152*c.g + 0.0722*c.b }

// ContrastRatio returns the WCAG 2.2 contrast ratio of the color fg drawn on the opaque color bg,
// both #rrggbb or #rrggbbaa, from 1 to 21. A translucent fg is composited over bg in gamma-encoded
// sRGB, as browsers paint it.
func ContrastRatio(fg, bg string) float64 {
	f, b := parseHexColor(fg), parseHexColor(bg)
	if f.a < 1 {
		mix := func(t, u float64) float64 { return toLinear(toGamma(t)*f.a + toGamma(u)*(1-f.a)) }
		f = rgba{r: mix(f.r, b.r), g: mix(f.g, b.g), b: mix(f.b, b.b), a: 1}
	}
	l1, l2 := luminance(f), luminance(b)
	return (max(l1, l2) + 0.05) / (min(l1, l2) + 0.05)
}

// ThemeColor returns the value of the color token name in scheme with the overrides of tokens, or
// its default. It returns "" for a name that is not a color token.
func ThemeColor(tokens []ThemeTokenValue, name string, scheme ColorScheme) string {
	t, ok := LookupThemeToken(name)
	if !ok || t.Kind != ThemeTokenColor {
		return ""
	}
	light, dark := t.Light, t.Dark
	if i := slices.IndexFunc(tokens, func(v ThemeTokenValue) bool { return v.Token == name }); i >= 0 {
		light, dark = tokens[i].Light, tokens[i].Dark
	}
	if scheme == SchemeDark {
		return dark
	}
	return light
}

// DefaultThemeColor returns the default of the color token name in the dark color scheme, or in
// the light one, or "" for a name that is not a color token.
func DefaultThemeColor(name string, dark bool) string {
	if dark {
		return ThemeColor(nil, name, SchemeDark)
	}
	return ThemeColor(nil, name, SchemeLight)
}

func checkContrast(tokens []ThemeTokenValue) []ThemeProblem {
	var problems []ThemeProblem
	for _, scheme := range []ColorScheme{SchemeLight, SchemeDark} {
		for _, p := range themeContrastPairs {
			ratio := ContrastRatio(ThemeColor(tokens, p.FG, scheme), ThemeColor(tokens, p.BG, scheme))
			if ratio < p.Minimum {
				problems = append(problems, ThemeProblem{
					Code: ThemeLowContrast, Token: p.FG, Background: p.BG, Scheme: scheme,
					Ratio: math.Floor(ratio*100) / 100, Minimum: p.Minimum,
				})
			}
		}
	}
	return problems
}

// Stylesheet returns the stylesheet of the revision (ADR-0059 §5).
func (r ThemeRevision) Stylesheet() []byte {
	return themeStylesheet("The theme of this server (revision "+r.ID+").", r.Set.Tokens)
}

// Stylesheet returns the stylesheet of the set, which previews a draft before it is published.
func (s ThemeSet) Stylesheet() []byte {
	return themeStylesheet("A draft theme of this server (set "+s.ID+"), not published.", s.Tokens)
}

// themeStylesheet returns a :root rule with the token overrides, colors as light-dark(), outside
// any cascade layer, so that it wins over the defaults and the utilities that Tailwind puts in its
// layers. The names come from the generated token list and the values were validated against their
// kinds, so they cannot end the rule. A set without overrides is the comment alone.
func themeStylesheet(comment string, tokens []ThemeTokenValue) []byte {
	var b strings.Builder
	b.WriteString("/* " + comment + " */\n")
	var decls []string
	for _, v := range tokens {
		t, ok := LookupThemeToken(v.Token)
		if !ok {
			continue
		}
		value := v.Value
		if t.Kind == ThemeTokenColor {
			value = "light-dark(" + v.Light + ", " + v.Dark + ")"
		}
		decls = append(decls, "  --"+t.Name+": "+value+";\n")
	}
	if len(decls) > 0 {
		b.WriteString(":root {\n")
		for _, d := range decls {
			b.WriteString(d)
		}
		b.WriteString("}\n")
	}
	return []byte(b.String())
}
