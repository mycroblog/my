// SPDX-License-Identifier: AGPL-3.0-only

package aws

import (
	"strings"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/i18n"
)

// autoDetect is the SourceLanguageCode that has Amazon Translate detect the language.
const autoDetect = "auto"

// regional are the languages that Amazon Translate tells apart by region, by language and region
// of the BCP 47 tag; any other region translates as the language alone.
var regional = map[string]string{
	"fr-CA": "fr-CA",
	"pt-PT": "pt-PT",
	"es-MX": "es-MX",
	"fa-AF": "fa-AF",
}

// languageCode returns the language code of Amazon Translate for a BCP 47 tag, and false for a
// tag that has no code of 2 to 5 characters, which Amazon Translate could not take. Whether
// Amazon Translate translates the language is its answer to give.
//
// Amazon Translate names most languages by their primary subtag; Chinese is "zh" (Simplified) and
// "zh-TW" (Traditional), and Filipino is "tl".
func languageCode(tag string) (string, bool) {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"), "-")
	base := strings.ToLower(parts[0])
	if len(base) < 2 || len(base) > 3 || base == translation.UndeterminedLanguage || !isLetters(base) {
		return "", false
	}
	var script, region string
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4 && isLetters(p) && script == "" && region == "":
			script = strings.ToLower(p)
		case (len(p) == 2 && isLetters(p) || len(p) == 3 && isDigits(p)) && region == "":
			region = strings.ToUpper(p)
		}
	}
	switch base {
	case "zh":
		if script == "hant" || script == "" && (region == "TW" || region == "HK" || region == "MO") {
			return "zh-TW", true
		}
		return "zh", true
	case "fil":
		return "tl", true
	case "iw":
		return "he", true
	}
	if code, ok := regional[base+"-"+region]; ok {
		return code, true
	}
	return base, true
}

// detectedLanguage returns the source language of the answer: the code of Amazon Translate as a
// BCP 47 tag, or the request's source when the answer says nothing that is a language tag.
func detectedLanguage(code, source string) string {
	tag := i18n.Canonicalize(code)
	if tag == "" || tag == translation.UndeterminedLanguage {
		return source
	}
	return tag
}

func isLetters(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return s != ""
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
