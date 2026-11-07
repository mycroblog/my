// SPDX-License-Identifier: AGPL-3.0-only

package google

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"

	"github.com/mycroblog/my/internal/app/translation"
)

// marker matches a marker of the port and captures its slash and its number.
var marker = regexp.MustCompile(`⟦(/?)([1-9][0-9]*)⟧`)

// markerNumberPattern matches the number of a marker.
var markerNumberPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// markerAttr names the attribute of a span that carries the number of its marker.
const markerAttr = "data-m"

// toHTML writes a fragment as the HTML that Cloud Translation translates with the MIME type text/html. A marker that
// stands alone becomes a span that Google leaves untranslated (translate="no" and the class notranslate), holding the
// marker; a pair becomes a span around its words; a line break becomes br, since HTML collapses white space; and the text
// is escaped.
func toHTML(fragment string) string {
	pairs := pairedNumbers(fragment)
	var b strings.Builder
	last := 0
	for _, m := range marker.FindAllStringSubmatchIndex(fragment, -1) {
		writeText(&b, fragment[last:m[0]])
		last = m[1]
		closing, n := fragment[m[2]:m[3]] == "/", fragment[m[4]:m[5]]
		switch {
		case closing:
			b.WriteString("</span>")
		case pairs[n]:
			b.WriteString(`<span ` + markerAttr + `="` + n + `">`)
		default:
			b.WriteString(`<span translate="no" class="notranslate" ` + markerAttr + `="` + n + `">` + translation.Marker(atoi(n)) + `</span>`)
		}
	}
	writeText(&b, fragment[last:])
	return b.String()
}

func writeText(b *strings.Builder, text string) {
	b.WriteString(strings.ReplaceAll(html.EscapeString(text), "\n", "<br>"))
}

// pairedNumbers returns the numbers of the markers of the fragment that close a pair.
func pairedNumbers(fragment string) map[string]bool {
	pairs := map[string]bool{}
	for _, m := range marker.FindAllStringSubmatch(fragment, -1) {
		if m[1] == "/" {
			pairs[m[2]] = true
		}
	}
	return pairs
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s) // the pattern allows digits alone
	return n
}

// fromHTML reads the translation of toHTML(fragment) back into the form of the port: the spans of the markers become the
// markers again, br a line break, and the text is unescaped. Any other markup of the answer is dropped, since the post
// never had it; what is left of a marker that Google wrote as text stays, for MarkersIntact to judge.
func fromHTML(fragment, translated string) string {
	pairs := pairedNumbers(fragment)
	type open struct {
		closing  string // the closing marker of a pair
		withheld bool   // a span of a marker that stands alone, whose content is not text
	}
	var (
		b        strings.Builder
		stack    []open
		withheld int
	)
	z := nethtml.NewTokenizer(strings.NewReader(translated))
	for {
		tt := z.Next()
		if tt == nethtml.ErrorToken {
			break
		}
		tok := z.Token()
		switch tt {
		case nethtml.TextToken:
			if withheld == 0 {
				b.WriteString(tok.Data)
			}
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			if tok.Data == "br" {
				if withheld == 0 {
					b.WriteString("\n")
				}
				continue
			}
			if tok.Data != "span" {
				continue
			}
			n, ok := markerNumber(tok)
			switch {
			case !ok:
				if tt == nethtml.StartTagToken {
					stack = append(stack, open{})
				}
			case pairs[n]:
				if withheld == 0 {
					b.WriteString(translation.Marker(atoi(n)))
				}
				if tt == nethtml.StartTagToken {
					stack = append(stack, open{closing: translation.ClosingMarker(atoi(n))})
				} else if withheld == 0 {
					b.WriteString(translation.ClosingMarker(atoi(n)))
				}
			default:
				if withheld == 0 {
					b.WriteString(translation.Marker(atoi(n)))
				}
				if tt == nethtml.StartTagToken {
					stack = append(stack, open{withheld: true})
					withheld++
				}
			}
		case nethtml.EndTagToken:
			if tok.Data != "span" || len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if top.withheld {
				withheld--
			}
			if top.closing != "" && withheld == 0 {
				b.WriteString(top.closing)
			}
		}
	}
	return b.String()
}

// markerNumber returns the number of the marker that a span carries.
func markerNumber(tok nethtml.Token) (string, bool) {
	for _, a := range tok.Attr {
		if a.Key == markerAttr && markerNumberPattern.MatchString(a.Val) {
			return a.Val, true
		}
	}
	return "", false
}
