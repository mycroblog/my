// SPDX-License-Identifier: AGPL-3.0-only

package deepl

import (
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mycroblog/my/internal/app/translation"
)

// DeepL keeps XML elements with tag_handling = xml, but not the port's markers, which it may translate or drop. So each
// marker goes as an element whose name carries its number: an item, ⟦1⟧, as <m1/>, and a pair, ⟦2⟧…⟦/2⟧, as <p2>…</p2>,
// around the words that DeepL translates. The text between them is escaped, so that text of a post that looks like one
// of these elements stays text.

// markerParts finds the port's markers in a fragment.
var markerParts = regexp.MustCompile(`⟦(/?)([1-9][0-9]*)⟧`)

// toTags returns the fragment in DeepL's form and names with the names of its elements added, each once.
func toTags(fragment string, names []string) (string, []string) {
	var b strings.Builder
	last := 0
	for _, loc := range markerParts.FindAllStringSubmatchIndex(fragment, -1) {
		b.WriteString(escape(fragment[last:loc[0]]))
		last = loc[1]
		closing := loc[3] > loc[2]
		n := fragment[loc[4]:loc[5]]
		num, _ := strconv.Atoi(n)
		switch {
		case closing:
			b.WriteString("</p" + n + ">")
		case strings.Contains(fragment, translation.ClosingMarker(num)):
			b.WriteString("<p" + n + ">")
			names = addName(names, "p"+n)
		default:
			b.WriteString("<m" + n + "/>")
			names = addName(names, "m"+n)
		}
	}
	b.WriteString(escape(fragment[last:]))
	return b.String(), names
}

func addName(names []string, name string) []string {
	if slices.Contains(names, name) {
		return names
	}
	return append(names, name)
}

// escape writes text as the content of an XML element.
func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// element finds an element of toTags in DeepL's answer, also in the forms that an XML writer may give it: <m1></m1>,
// <m1 />, and <p2/> for a pair around nothing.
var element = regexp.MustCompile(`<(/?)([mp])([1-9][0-9]*)\s*(/?)>`)

// fromTags returns DeepL's answer with the port's markers in place of the elements and the text unescaped. Anything else
// that looks like markup stays as text, and the service escapes it.
func fromTags(answer string) string {
	var b strings.Builder
	last := 0
	for _, loc := range element.FindAllStringSubmatchIndex(answer, -1) {
		b.WriteString(html.UnescapeString(answer[last:loc[0]]))
		last = loc[1]
		closing := loc[3] > loc[2]
		pair := answer[loc[4]:loc[5]] == "p"
		num, _ := strconv.Atoi(answer[loc[6]:loc[7]])
		empty := loc[9] > loc[8]
		switch {
		case !pair && closing:
			// The end of <m1></m1>, which the opening tag already wrote.
		case !pair:
			b.WriteString(translation.Marker(num))
		case closing:
			b.WriteString(translation.ClosingMarker(num))
		case empty:
			b.WriteString(translation.Marker(num) + translation.ClosingMarker(num))
		default:
			b.WriteString(translation.Marker(num))
		}
	}
	b.WriteString(html.UnescapeString(answer[last:]))
	return b.String()
}
