// SPDX-License-Identifier: AGPL-3.0-only

package google

import (
	"golang.org/x/text/language"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/i18n"
)

// googleVariants are the codes of Cloud Translation's language support that name more than a language
// (https://cloud.google.com/translate/docs/languages), other than Chinese. Any other tag goes out as its language alone,
// since Google refuses a region it does not list.
var googleVariants = map[string]bool{
	"fa-AF": true, "fr-CA": true, "mni-Mtei": true, "ms-Arab": true, "pa-Arab": true, "pt-PT": true,
}

// toGoogle returns the code of Cloud Translation for a BCP 47 tag, or "" for a tag that is none. Chinese goes out as
// zh-CN (Simplified) or zh-TW (Traditional) by its script, or by its region when it has no script.
func toGoogle(tag string) string {
	c := i18n.Canonicalize(tag)
	if c == "" || c == translation.UndeterminedLanguage {
		return ""
	}
	if googleVariants[c] {
		return c
	}
	t := language.Make(c)
	base, _ := t.Base()
	if base.String() == "zh" {
		script, sc := t.Script()
		region, _ := t.Region()
		if sc == language.Exact && script.String() == "Hant" || sc != language.Exact && traditionalRegions[region.String()] {
			return "zh-TW"
		}
		return "zh-CN"
	}
	return base.String()
}

// traditionalRegions write Chinese in the Traditional script.
var traditionalRegions = map[string]bool{"TW": true, "HK": true, "MO": true}

// fromGoogle returns the BCP 47 tag of the language that Google detected, or the request's source when Google says
// nothing, says something that is no language tag, or does not know a language that the request knows. Chinese comes
// back with its script, which the server compares, rather than a region.
func fromGoogle(code, source string) string {
	c := i18n.Canonicalize(code)
	switch c {
	case "zh-CN":
		return "zh-Hans"
	case "zh-TW":
		return "zh-Hant"
	case "":
		return source
	case translation.UndeterminedLanguage:
		return source
	}
	return c
}
