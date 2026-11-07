// SPDX-License-Identifier: AGPL-3.0-only

package aws

import (
	"errors"
	"html"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	nethtml "golang.org/x/net/html"
)

// markerPattern matches the markers of the port (translation.Marker, translation.ClosingMarker).
var markerPattern = regexp.MustCompile(`⟦/?[1-9][0-9]*⟧`)

const (
	// keepOpen and keepClose surround a marker: Amazon Translate keeps the content of an element
	// with translate="no" as it is.
	keepOpen  = `<span translate="no">`
	keepClose = `</span>`
	// unitOpen and unitClose surround each fragment, or piece of one, of a call.
	unitOpen  = "<div>"
	unitClose = "</div>"
	lineBreak = "<br>"
)

// encodeText escapes plain text for the HTML of a call, with its line breaks as <br>, which HTML
// would otherwise fold into a space.
func encodeText(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", lineBreak)
}

// encode returns the HTML of a fragment: its text escaped and each marker in an element that
// Amazon Translate does not translate.
func encode(fragment string) string {
	var b strings.Builder
	last := 0
	for _, loc := range markerPattern.FindAllStringIndex(fragment, -1) {
		b.WriteString(encodeText(fragment[last:loc[0]]))
		b.WriteString(keepOpen + fragment[loc[0]:loc[1]] + keepClose)
		last = loc[1]
	}
	b.WriteString(encodeText(fragment[last:]))
	return b.String()
}

// unitSize is the number of bytes that a piece of a fragment takes in the text of a call.
func unitSize(piece string) int {
	return len(unitOpen) + len(encode(piece)) + len(unitClose)
}

// errUnits reports an answer whose <div> elements are not the ones that the call sent.
var errUnits = errors.New("the answer does not keep the division into fragments")

// decode splits the HTML of an answer at its top-level <div> elements and returns the plain text
// of each, with the content of the translate="no" elements as it is and <br> as a line break.
// Other markup that the answer may have is dropped, so no markup of the provider reaches a
// translation, and text outside the <div> elements other than white space fails it.
func decode(answer string) ([]string, error) {
	z := nethtml.NewTokenizer(strings.NewReader(answer))
	var (
		out   []string
		unit  *strings.Builder
		spans []bool // for each open span, whether it is a translate="no" one
		keep  int    // the translate="no" spans that are open
	)
	for {
		tt := z.Next()
		switch tt {
		case nethtml.ErrorToken:
			if !errors.Is(z.Err(), io.EOF) {
				return nil, errUnits
			}
			if unit != nil {
				return nil, errUnits
			}
			return out, nil
		case nethtml.TextToken:
			text := string(z.Text())
			if unit == nil {
				if strings.TrimSpace(text) != "" {
					return nil, errUnits
				}
				continue
			}
			if keep > 0 {
				// A marker holds no white space, so any around it is the provider's.
				text = strings.TrimSpace(text)
			}
			unit.WriteString(text)
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "div":
				if unit != nil || tt == nethtml.SelfClosingTagToken {
					return nil, errUnits
				}
				unit = &strings.Builder{}
				spans, keep = nil, 0
			case "br":
				if unit != nil {
					unit.WriteString("\n")
				}
			case "span":
				if tt == nethtml.SelfClosingTagToken {
					continue
				}
				no := hasAttr && translateNo(z)
				spans = append(spans, no)
				if no {
					keep++
				}
			}
		case nethtml.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "div":
				if unit == nil {
					return nil, errUnits
				}
				out = append(out, unit.String())
				unit = nil
			case "span":
				if len(spans) > 0 {
					if spans[len(spans)-1] {
						keep--
					}
					spans = spans[:len(spans)-1]
				}
			}
		}
	}
}

// translateNo reports whether the attributes of the tag under the tokenizer hold translate="no".
func translateNo(z *nethtml.Tokenizer) bool {
	for {
		key, val, more := z.TagAttr()
		if string(key) == "translate" && strings.EqualFold(string(val), "no") {
			return true
		}
		if !more {
			return false
		}
	}
}

// split cuts a fragment into pieces whose unitSize is at most limit. It cuts after the last line
// break, else after the last end of a sentence, else after the last space in the second half of
// what fits, and only where none is, between two characters; never inside a marker. The pieces put together are the fragment.
func split(fragment string, limit int) []string {
	var out []string
	for unitSize(fragment) > limit {
		cut := cutPoint(fragment, limit-len(unitOpen)-len(unitClose))
		if cut <= 0 || cut >= len(fragment) {
			break // cannot happen while limit holds a marker or a character
		}
		out = append(out, fragment[:cut])
		fragment = fragment[cut:]
	}
	return append(out, fragment)
}

// cutPoint returns the end of the longest prefix of fragment, ending where a piece may end, whose
// HTML takes at most budget bytes, preferring the breaks that split describes.
func cutPoint(fragment string, budget int) int {
	size, fits := 0, 0
	newline, sentence, space := 0, 0, 0
	markers := markerPattern.FindAllStringIndex(fragment, -1)
	prev := rune(0)
	for i := 0; i < len(fragment); {
		var end, cost int
		var r rune
		if len(markers) > 0 && markers[0][0] == i {
			end = markers[0][1]
			cost = len(keepOpen) + end - i + len(keepClose)
			markers = markers[1:]
		} else {
			var n int
			r, n = utf8.DecodeRuneInString(fragment[i:])
			end = i + n
			cost = len(encodeText(fragment[i:end]))
		}
		if size+cost > budget {
			break
		}
		size += cost
		fits = end
		switch r {
		case '\n':
			newline = end
		case '。', '！', '？':
			sentence = end
		case ' ', '\t':
			space = end
			if prev == '.' || prev == '!' || prev == '?' {
				sentence = end
			}
		}
		prev = r
		i = end
	}
	// A break in the first half of what fits would only make more calls.
	for _, c := range []int{newline, sentence, space} {
		if c > fits/2 {
			return c
		}
	}
	return fits
}
