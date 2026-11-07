// SPDX-License-Identifier: AGPL-3.0-only

package i18n

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/text/language"
)

// SourceLocale is the locale every catalog is written in and the last fallback.
const SourceLocale = "en"

// Pseudo-locales generated from the source locale by tools/i18n (ADR-0009).
const (
	PseudoLocaleLong = "en-XA" // accented and about 40% longer
	PseudoLocaleRTL  = "ar-XB" // mirrored with bidi overrides
)

func isPseudo(locale string) bool { return locale == PseudoLocaleLong || locale == PseudoLocaleRTL }

// formattingLocale returns the locale whose CLDR plural and number rules apply to
// messages of locale. Pseudo-locales keep the source locale's plural categories, so
// they are formatted with its rules; tools/i18n does the same on the FormatJS side.
func formattingLocale(locale string) string {
	if isPseudo(locale) {
		return SourceLocale
	}
	return locale
}

// parentLocales overrides the default parent (drop the last subtag) so that each tag reaches
// the one catalog locale of its language and script (ADR-0009 §2): Simplified Chinese ("zh",
// "zh-SG", "zh-Hans", "zh-Hans-*") resolves to zh-CN, Traditional Chinese ("zh-HK", "zh-MO",
// "zh-Hant", "zh-Hant-*") to zh-TW, and Portuguese ("pt", "pt-PT") to pt-BR. A catalog locale
// with a region ends its chain before SourceLocale, so that a message missing from it falls
// back to SourceLocale and never to another locale (zh-TW never reaches zh-CN).
var parentLocales = map[string]string{
	"zh":      "zh-CN",
	"zh-SG":   "zh-CN",
	"zh-Hans": "zh-CN",
	"zh-CN":   "",
	"zh-Hant": "zh-TW",
	"zh-HK":   "zh-TW",
	"zh-MO":   "zh-TW",
	"zh-TW":   "",
	"pt":      "pt-BR",
	"pt-PT":   "pt-BR",
	"pt-BR":   "",
}

// FallbackChain returns the locales consulted for a message, most specific first,
// always ending with SourceLocale: zh-Hant-HK -> zh-Hant -> zh-TW -> en, ja-JP -> ja -> en.
func FallbackChain(locale string) []string {
	var chain []string
	for l := Canonicalize(locale); l != "" && !slices.Contains(chain, l); {
		chain = append(chain, l)
		if p, ok := parentLocales[l]; ok {
			l = p
			continue
		}
		i := strings.LastIndexByte(l, '-')
		if i < 0 {
			break
		}
		l = l[:i]
	}
	if !slices.Contains(chain, SourceLocale) {
		chain = append(chain, SourceLocale)
	}
	return chain
}

// Aliases returns the tags other than locale that parentLocales resolves to it, longest
// first: zh-Hans, zh-SG and zh for zh-CN. A tag of a region or script under one of them
// ("zh-Hans-SG") resolves to locale too; a tag of a language alone ("zh") does only by
// itself, since another tag of its language may resolve elsewhere ("zh-TW").
func Aliases(locale string) []string {
	locale = Canonicalize(locale)
	if locale == SourceLocale {
		// Every chain ends with SourceLocale; only a tag of its language asks for it.
		return nil
	}
	var tags []string
	for tag := range parentLocales {
		if tag != locale && slices.Contains(FallbackChain(tag), locale) {
			tags = append(tags, tag)
		}
	}
	slices.SortFunc(tags, func(a, b string) int { return cmp.Or(cmp.Compare(len(b), len(a)), cmp.Compare(a, b)) })
	return tags
}

// Canonicalize normalizes the case and separators of a BCP 47 tag and replaces deprecated
// subtags by their preferred ones: "zh_hant_tw" -> "zh-Hant-TW", "iw" -> "he". It returns ""
// for an empty tag and for a tag that is not well-formed or has no language subtag
// ("123", "en-u", "en-US-US", "und-JP", "x-foo"); "und" stays "und". It parses with golang.org/x/text/language,
// as the language preferences of the accounts do, so that the same tag means the same everywhere.
func Canonicalize(tag string) string {
	tag = strings.TrimSpace(strings.ReplaceAll(tag, "_", "-"))
	if tag == "" {
		return ""
	}
	t, err := language.Parse(tag)
	if err != nil {
		return ""
	}
	c := t.String()
	// Base guesses a language for "und-JP" or "x-foo"; only an explicit one is Exact.
	if _, conf := t.Base(); conf != language.Exact && c != "und" {
		return ""
	}
	return c
}

// ParseAcceptLanguage returns the language tags of an Accept-Language header value,
// highest quality first (ties keep header order). Tags with q=0, the wildcard "*",
// and malformed entries are dropped.
func ParseAcceptLanguage(header string) []string {
	type weighted struct {
		tag string
		q   float64
	}
	var tags []weighted
	for entry := range strings.SplitSeq(header, ",") {
		tag, params, _ := strings.Cut(entry, ";")
		tag = Canonicalize(tag)
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		for p := range strings.SplitSeq(params, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if !ok || strings.TrimSpace(k) != "q" {
				continue
			}
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil || f < 0 || f > 1 {
				f = 0
			}
			q = f
		}
		if q > 0 {
			tags = append(tags, weighted{tag, q})
		}
	}
	slices.SortStableFunc(tags, func(a, b weighted) int { return cmp.Compare(b.q, a.q) })
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.tag
	}
	return out
}

// rtlLanguages are the languages written right to left by default.
var rtlLanguages = map[string]bool{
	"ar": true, "ckb": true, "dv": true, "fa": true, "he": true, "ps": true,
	"sd": true, "ug": true, "ur": true, "yi": true,
}

// Direction returns "rtl" or "ltr" for locale, for the root element's dir attribute.
func Direction(locale string) string {
	locale = Canonicalize(locale)
	lang, rest, _ := strings.Cut(locale, "-")
	switch {
	case locale == PseudoLocaleRTL:
		return "rtl"
	case strings.HasPrefix(rest, "Arab") || strings.HasPrefix(rest, "Hebr"):
		return "rtl"
	case rtlLanguages[lang] && !strings.HasPrefix(rest, "Latn"):
		return "rtl"
	}
	return "ltr"
}
