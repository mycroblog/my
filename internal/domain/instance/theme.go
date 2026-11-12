// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The instance theme (ADR-0059, FR-605): overrides of the web client's design tokens and one custom
// stylesheet of the administrator. The overridable tokens, their kinds, defaults, and bounds, and the
// contrast pairs of the colors are generated from the design tokens (theme_tokens.gen.go, committed).

// ThemeTokenKind is the kind of value a token holds, which decides the shape of its overrides.
type ThemeTokenKind string

// Kinds of theme tokens.
const (
	// ThemeTokenColor is a color with a value per color scheme, each #rrggbb (#rrggbbaa when
	// ThemeToken.Alpha).
	ThemeTokenColor ThemeTokenKind = "color"
	// ThemeTokenLength is 0 or a rem or px length, bounded in rem (16px each).
	ThemeTokenLength ThemeTokenKind = "length"
	// ThemeTokenNumber is a unitless number, such as a line height.
	ThemeTokenNumber ThemeTokenKind = "number"
	// ThemeTokenFontFamily is a comma-separated list of font family names.
	ThemeTokenFontFamily ThemeTokenKind = "font-family"
	// ThemeTokenShadow is none or up to MaxShadowLayers box shadows.
	ThemeTokenShadow ThemeTokenKind = "shadow"
	// ThemeTokenDuration is a time in ms or s, bounded in ms.
	ThemeTokenDuration ThemeTokenKind = "duration"
	// ThemeTokenEasing is an easing keyword or a cubic-bezier() function.
	ThemeTokenEasing ThemeTokenKind = "easing"
)

// Limits of a theme.
const (
	// MaxCustomCSSBytes bounds the custom stylesheet of a theme.
	MaxCustomCSSBytes = 64 * 1024
	// MaxFontFamilies bounds the families of a font-family value.
	MaxFontFamilies = 30
	// MaxFontFamilyLength bounds a font-family value, in characters.
	MaxFontFamilyLength = 1000
	// MaxShadowLayers bounds the layers of a shadow value.
	MaxShadowLayers = 4
	// MaxShadowOffsetRem bounds every length of a shadow, in rem.
	MaxShadowOffsetRem = 4
)

// ThemeToken is a design token that a theme may override, named without the leading "--".
type ThemeToken struct {
	Name string
	Kind ThemeTokenKind
	// Light and Dark are the defaults of a color token, as #rrggbb (#rrggbbaa when Alpha).
	Light, Dark string
	// Alpha reports whether a color override may be translucent.
	Alpha bool
	// Default is the default of a token of another kind, as theme.css writes it.
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

// ThemeTokens returns the overridable tokens, in the order of theme.css.
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

// ColorScheme is the color scheme of a token value: light or dark for a color, all for the
// other kinds (instance_theme_tokens.scheme).
type ColorScheme string

// Color schemes.
const (
	SchemeLight ColorScheme = "light"
	SchemeDark  ColorScheme = "dark"
	SchemeAll   ColorScheme = "all"
)

// ThemeTokenValue is the override of one token: Light and Dark for a color token, Value for the
// other kinds.
type ThemeTokenValue struct {
	Token       string
	Light, Dark string
	Value       string
}

// ThemeTokenRow is the override of a token in one color scheme, as the database stores it.
type ThemeTokenRow struct {
	Token  string
	Scheme ColorScheme
	Value  string
}

// Rows returns the rows that store v: one per color scheme for a color, one for SchemeAll otherwise.
func (v ThemeTokenValue) Rows() []ThemeTokenRow {
	if t, ok := LookupThemeToken(v.Token); ok && t.Kind == ThemeTokenColor {
		return []ThemeTokenRow{{Token: v.Token, Scheme: SchemeLight, Value: v.Light}, {Token: v.Token, Scheme: SchemeDark, Value: v.Dark}}
	}
	return []ThemeTokenRow{{Token: v.Token, Scheme: SchemeAll, Value: v.Value}}
}

// ThemeTokenValuesFromRows folds stored rows back into token values, in the order of the tokens.
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
	// Tokens that a later version no longer has stay at the end, by name; they are not rendered.
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

// ThemeRevision is one version of the instance theme (instance_theme_revisions with its
// instance_theme_tokens). The revision with the greatest ID is the current theme; a revision with
// no tokens and no custom CSS resets the theme to the defaults.
type ThemeRevision struct {
	ID string
	// Tokens are the overridden tokens, in the order of ThemeTokens.
	Tokens []ThemeTokenValue
	// CustomCSS is the administrator's stylesheet, which the server serves without reading it.
	CustomCSS string
	// RevisedByUserID is the user who made the change, or "" for a change from the CLI.
	RevisedByUserID string
	RevisedAt       time.Time
}

// Empty reports whether the revision changes nothing, so the web client needs no stylesheet.
func (r ThemeRevision) Empty() bool { return len(r.Tokens) == 0 && r.CustomCSS == "" }

// ThemeHead points at the current theme revision (the instance_theme projection).
type ThemeHead struct {
	RevisionID string
}

// Supersedes reports whether r should replace the head h, which is the zero value when the theme
// has never been saved.
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
	// ThemeCSSTooLong: the custom CSS is longer than MaxCustomCSSBytes.
	ThemeCSSTooLong ThemeProblemCode = "css_too_long"
	// ThemeCSSInvalid: the custom CSS is not valid UTF-8 or contains U+0000.
	ThemeCSSInvalid ThemeProblemCode = "css_invalid"
)

// ThemeProblem is one reason a theme cannot be saved.
type ThemeProblem struct {
	Code ThemeProblemCode
	// Token is the token the problem is about (the foreground of a contrast pair), "" for the CSS.
	Token string
	// Scheme is the color scheme of a color value or a contrast pair, "" otherwise.
	Scheme ColorScheme
	// Background is the background token of a contrast pair.
	Background string
	// Ratio and Minimum are the contrast of the pair and the minimum it needs.
	Ratio, Minimum float64
	// Min and Max are the bounds of an out-of-range value, in rem for a length and ms for a duration.
	Min, Max float64
}

func (p ThemeProblem) String() string {
	switch p.Code {
	case ThemeLowContrast:
		return fmt.Sprintf("%s on %s is %.2f:1 in the %s scheme; it needs %g:1", p.Token, p.Background, p.Ratio, p.Scheme, p.Minimum)
	case ThemeOutOfRange:
		return fmt.Sprintf("%s is outside %g..%g", p.Token, p.Min, p.Max)
	case ThemeCSSTooLong, ThemeCSSInvalid:
		return string(p.Code)
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

// NormalizeTheme validates the overrides and the custom CSS of a theme and returns the overrides in
// their canonical form (lowercase colors, single spaces), in the order of ThemeTokens. It fails with a
// *ThemeError that lists every problem: unknown or repeated tokens, values without the shape or
// outside the bounds of their kind, contrast pairs below their minimum in either color scheme with the
// defaults of the tokens that are not overridden, and a custom CSS that is too long or not text.
func NormalizeTheme(tokens []ThemeTokenValue, customCSS string) ([]ThemeTokenValue, error) {
	var problems []ThemeProblem
	seen := map[string]bool{}
	byToken := map[string]ThemeTokenValue{}
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
	switch {
	case len(customCSS) > MaxCustomCSSBytes:
		problems = append(problems, ThemeProblem{Code: ThemeCSSTooLong, Max: MaxCustomCSSBytes})
	case !utf8.ValidString(customCSS) || strings.ContainsRune(customCSS, 0):
		problems = append(problems, ThemeProblem{Code: ThemeCSSInvalid})
	}
	out := make([]ThemeTokenValue, 0, len(byToken))
	for _, t := range themeTokens {
		if v, ok := byToken[t.Name]; ok {
			out = append(out, v)
		}
	}
	// Contrast is checked only when every color parsed, so that a malformed color is reported once.
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
			c, ok := normalizeColor(side.in, t.Alpha)
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
	hexColorPattern      = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	hexAlphaColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}(?:[0-9a-f]{2})?$`)
	lengthPattern        = regexp.MustCompile(`^(?:0|(\d{1,3}(?:\.\d{1,4})?|\.\d{1,4})(rem|px))$`)
	numberPattern        = regexp.MustCompile(`^\d{1,2}(?:\.\d{1,4})?$`)
	durationPattern      = regexp.MustCompile(`^(\d{1,5}(?:\.\d{1,3})?)(ms|s)$`)
	cubicBezierPattern   = regexp.MustCompile(`^cubic-bezier\(\s*(-?\d*\.?\d+)\s*,\s*(-?\d*\.?\d+)\s*,\s*(-?\d*\.?\d+)\s*,\s*(-?\d*\.?\d+)\s*\)$`)
	quotedFamilyPattern  = regexp.MustCompile(`^"[^"\\;{}<>\x00-\x1f]{1,100}"$`)
	plainFamilyPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(?: [A-Za-z0-9-]+)*$`)
	shadowLengthPattern  = regexp.MustCompile(`^(?:0|-?(\d{1,3}(?:\.\d{1,4})?|\.\d{1,4})(rem|px))$`)
)

var easingKeywords = []string{"linear", "ease", "ease-in", "ease-out", "ease-in-out"}

func normalizeColor(s string, alpha bool) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(s))
	if alpha {
		return c, hexAlphaColorPattern.MatchString(c)
	}
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
	case ThemeTokenEasing:
		if !validEasing(v) {
			return "", ThemeInvalidValue, false
		}
	case ThemeTokenFontFamily:
		if !validFontFamily(v) {
			return "", ThemeInvalidValue, false
		}
	case ThemeTokenShadow:
		out, ok := normalizeShadow(v)
		if !ok {
			return "", ThemeInvalidValue, false
		}
		v = out
	default:
		return "", ThemeInvalidValue, false
	}
	return v, "", true
}

func validEasing(v string) bool {
	if slices.Contains(easingKeywords, v) {
		return true
	}
	m := cubicBezierPattern.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	for i, s := range m[1:] {
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return false
		}
		// The x coordinates (1st and 3rd) must be in [0, 1]; the y coordinates may overshoot a little.
		if i%2 == 0 && (n < 0 || n > 1) || i%2 == 1 && (n < -2 || n > 3) {
			return false
		}
	}
	return true
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

// normalizeShadow checks a shadow value: none, or up to MaxShadowLayers comma-separated layers of
// [inset] <x> <y> [<blur> [<spread>]] <#color>, each length 0 or rem / px within MaxShadowOffsetRem.
func normalizeShadow(v string) (string, bool) {
	if v == "none" {
		return v, true
	}
	layers := strings.Split(v, ",")
	if len(layers) > MaxShadowLayers {
		return "", false
	}
	out := make([]string, len(layers))
	for i, layer := range layers {
		parts := strings.Fields(layer)
		if len(parts) > 0 && parts[0] == "inset" {
			parts = parts[1:]
		}
		if len(parts) < 3 || len(parts) > 5 {
			return "", false
		}
		if _, ok := normalizeColor(parts[len(parts)-1], true); !ok {
			return "", false
		}
		for _, p := range parts[:len(parts)-1] {
			m := shadowLengthPattern.FindStringSubmatch(p)
			if m == nil || lengthRem(m) > MaxShadowOffsetRem {
				return "", false
			}
		}
		out[i] = strings.ToLower(strings.Join(strings.Fields(layer), " "))
	}
	return strings.Join(out, ", "), true
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

// Stylesheet returns the stylesheet of the theme (ADR-0059 §5): a :root rule with the token
// overrides, colors as light-dark(), then the custom CSS. Neither is in a cascade layer, so both win
// over the defaults and the utilities that Tailwind puts in its layers. The names come from the
// generated token list and the values were validated against their kinds, so they cannot end the
// rule; the custom CSS is the administrator's and is copied as it is.
func (r ThemeRevision) Stylesheet() []byte {
	var b strings.Builder
	b.WriteString("/* The theme of this server (revision " + r.ID + "). */\n")
	var decls []string
	for _, v := range r.Tokens {
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
	if r.CustomCSS != "" {
		b.WriteString(r.CustomCSS)
		if !strings.HasSuffix(r.CustomCSS, "\n") {
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}
