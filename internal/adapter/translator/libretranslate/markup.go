// SPDX-License-Identifier: AGPL-3.0-only

package libretranslate

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"

	"github.com/mycroblog/my/internal/app/translation"
)

// The port's markers ⟦1⟧ and ⟦/1⟧ are text to LibreTranslate: its models see brackets and
// digits that they may translate, drop, or reorder like any other characters. LibreTranslate
// keeps markup with format=html instead: it translates the text nodes and puts the elements back
// around them, and leaves the content of code and of an element with translate="no" as it is.
// So a fragment goes out as HTML, where
//
//   - an item ⟦n⟧ is an empty <code id="mN" translate="no"></code>: there is nothing in it to
//     translate or to send, and both ways of keeping it from translation apply, and
//   - a pair ⟦n⟧…⟦/n⟧ is <span id="mN">…</span>: the words inside are translated and the span
//     stays around them, wherever the target language puts them.
//
// The text of the fragment is escaped, so nothing in a post becomes markup on the way out, and
// the answer is read back token by token: an element with an id of the request becomes its
// marker again, any other element is dropped, and text is unescaped. Whatever LibreTranslate
// did to the elements, MarkersIntact then decides whether the translation is usable.

// markerPattern is the form of translation.Marker and translation.ClosingMarker.
var markerPattern = regexp.MustCompile(`⟦(/?)([1-9][0-9]*)⟧`)

// markers are the markers of one fragment: true for the number of a pair, false for an item.
type markers map[int]bool

// toHTML returns the fragment as the HTML that LibreTranslate receives, and its markers.
func toHTML(fragment string) (string, markers) {
	ms := markers{}
	for _, m := range markerPattern.FindAllStringSubmatch(fragment, -1) {
		n, _ := strconv.Atoi(m[2])
		ms[n] = ms[n] || m[1] == "/"
	}
	var b strings.Builder
	last := 0
	for _, loc := range markerPattern.FindAllStringSubmatchIndex(fragment, -1) {
		b.WriteString(html.EscapeString(fragment[last:loc[0]]))
		closing := loc[3] > loc[2]
		id := fragment[loc[4]:loc[5]]
		switch {
		case closing:
			b.WriteString("</span>")
		case ms[atoi(id)]:
			b.WriteString(`<span id="m` + id + `">`)
		default:
			b.WriteString(`<code id="m` + id + `" translate="no"></code>`)
		}
		last = loc[1]
	}
	b.WriteString(html.EscapeString(fragment[last:]))
	return b.String(), ms
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// open is an element of the answer that has not closed yet.
type open struct {
	tag    string
	marker int  // 0 for an element that no marker stands for
	pair   bool // the marker opens a pair, which the end tag closes
}

// fromHTML returns the answer of LibreTranslate to a fragment with the markers ms as text with
// the port's markers. The content of an item's element is dropped: the item stands for it.
func fromHTML(answer string, ms markers) string {
	var b strings.Builder
	var stack []open
	inItem := 0 // the depth of item elements around the current token
	z := nethtml.NewTokenizer(strings.NewReader(answer))
	for {
		tt := z.Next()
		switch tt {
		case nethtml.ErrorToken:
			return b.String()
		case nethtml.TextToken:
			if inItem == 0 {
				b.WriteString(z.Token().Data)
			}
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			tok := z.Token()
			n := markerOf(tok, ms)
			pair := n > 0 && ms[n]
			if n > 0 && inItem == 0 {
				b.WriteString(translation.Marker(n))
			}
			if tt == nethtml.SelfClosingTagToken || isVoid(tok.Data) {
				continue
			}
			if n > 0 && !pair {
				inItem++
			}
			stack = append(stack, open{tag: tok.Data, marker: n, pair: pair})
		case nethtml.EndTagToken:
			tok := z.Token()
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].tag != tok.Data {
					continue
				}
				e := stack[i]
				stack = append(stack[:i], stack[i+1:]...)
				switch {
				case e.marker > 0 && !e.pair:
					inItem--
				case e.pair && inItem == 0:
					b.WriteString(translation.ClosingMarker(e.marker))
				}
				break
			}
		}
	}
}

// markerOf returns the number of the marker that the element stands for, or 0.
func markerOf(tok nethtml.Token, ms markers) int {
	for _, a := range tok.Attr {
		if a.Key != "id" || !strings.HasPrefix(a.Val, "m") {
			continue
		}
		n, err := strconv.Atoi(a.Val[1:])
		if err != nil || n <= 0 {
			return 0
		}
		if _, ok := ms[n]; ok {
			return n
		}
		return 0
	}
	return 0
}

func isVoid(tag string) bool {
	switch tag {
	case "br", "hr", "img", "wbr", "input", "meta", "link", "area", "base", "col", "embed", "source", "track":
		return true
	}
	return false
}

// hasWords reports whether the fragment has text to translate besides its markers.
func hasWords(fragment string) bool {
	return strings.TrimSpace(markerPattern.ReplaceAllString(fragment, "")) != ""
}
