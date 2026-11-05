// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"html"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/mycroblog/my/internal/domain/content"
)

// This file turns the plain text a local author types into the HTML of a status (FR-103,
// ADR-0019 §2): mentions, hashtags, and URLs become links, everything else is escaped, blank
// lines separate paragraphs, and single line breaks become <br>. The output uses only
// <p>, <br>, <span class>, and <a href class rel target>, a subset of the allowlist that remote
// HTML is sanitized to, so local and remote content render through the same rules.
// Parsing and rendering are pure functions; resolving mentions to accounts is the caller's job.

// SegmentKind says what a segment of parsed text is.
type SegmentKind int

// Segment kinds.
const (
	// SegmentText is plain text, rendered escaped.
	SegmentText SegmentKind = iota
	// SegmentMention is @username or @username@domain.
	SegmentMention
	// SegmentHashtag is #name.
	SegmentHashtag
	// SegmentURL is an http or https URL.
	SegmentURL
)

// Segment is one run of parsed text.
type Segment struct {
	Kind SegmentKind
	// Text is the source of the segment as typed, "@alice", "#Go", or "https://example.com".
	Text string
	// Mention is the handle of a SegmentMention.
	Mention MentionHandle
	// Hashtag is the tag of a SegmentHashtag.
	Hashtag Hashtag
	// Href is the URL of a SegmentURL, as url.URL.String() writes it.
	Href string
}

// MentionHandle is a mentioned handle, lowercase. Domain is "" when only @username was typed.
type MentionHandle struct {
	Username string
	Domain   string
}

// Hashtag is a hashtag as typed (Name, without the #) and under NormalizeTag.
type Hashtag struct {
	Name           string
	NameNormalized string
}

// NormalizeTag returns the form under which tags are unique (tags.name_normalized): NFKC, then
// lowercase.
func NormalizeTag(name string) string {
	return strings.ToLower(norm.NFKC.String(name))
}

// ShortcodeSpans returns the byte ranges [start, end) of every :shortcode: in t, colons included.
func ShortcodeSpans(t string) [][2]int { return content.ShortcodeSpans(t) }

// ParsedText is the text of a status split into segments.
type ParsedText struct {
	Segments []Segment
}

// ParseText splits text into plain text, mentions, hashtags, and URLs. Line breaks are
// normalized to "\n" and surrounding white space is dropped.
//
//   - A mention is @ followed by letters, digits, and underscores, optionally followed by @ and a
//     domain; the @ must not follow a word character, "/", or "=" (so e-mail addresses are text).
//   - A hashtag is # followed by letters, marks, digits, and underscores with at least one letter
//     or mark; the # must not follow a word character, "/", "=", ")", "&", or "#".
//   - A URL starts with http:// or https:// not preceded by a word character and runs up to white
//     space, a non-ASCII character, or one of < > " ` { } | \ ^; trailing punctuation and
//     unbalanced closing brackets are not part of it. Strings that do not parse as a URL with a
//     host stay text.
func ParseText(text string) ParsedText {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)

	var p ParsedText
	plainStart := 0
	flush := func(end int) {
		if end > plainStart {
			p.Segments = append(p.Segments, Segment{Kind: SegmentText, Text: text[plainStart:end]})
		}
	}
	var prev rune // the rune before i; 0 at the start
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if seg, n, ok := scanAt(text[i:], prev, r); ok {
			flush(i)
			p.Segments = append(p.Segments, seg)
			i += n
			plainStart = i
			prev, _ = utf8.DecodeLastRuneInString(text[:i])
			continue
		}
		prev = r
		i += size
	}
	flush(len(text))
	return p
}

// scanAt tries to read a mention, hashtag, or URL at the start of s, whose first rune is r and
// which follows prev.
func scanAt(s string, prev, r rune) (Segment, int, bool) {
	switch {
	case r == '@' && !isWord(prev) && prev != '/' && prev != '=':
		return scanMention(s)
	case r == '#' && !isWord(prev) && !strings.ContainsRune("/=)&#", prev):
		return scanHashtag(s)
	case (r == 'h' || r == 'H') && !isWord(prev):
		return scanURL(s)
	}
	return Segment{}, 0, false
}

func scanMention(s string) (Segment, int, bool) {
	n := 1 + spanFunc(s[1:], isUsernameByte)
	if n == 1 {
		return Segment{}, 0, false
	}
	h := MentionHandle{Username: strings.ToLower(s[1:n])}
	if n < len(s) && s[n] == '@' {
		d := spanFunc(s[n+1:], isDomainByte)
		// A domain ends in a letter or digit: "@alice@example.org." keeps the final period as text.
		for d > 0 && !isAlnumByte(s[n+d]) {
			d--
		}
		if d > 0 {
			h.Domain = strings.ToLower(s[n+1 : n+1+d])
			n += 1 + d
		}
	}
	return Segment{Kind: SegmentMention, Text: s[:n], Mention: h}, n, true
}

func scanHashtag(s string) (Segment, int, bool) {
	n, hasLetter := 1, false
	for n < len(s) {
		r, size := utf8.DecodeRuneInString(s[n:])
		if !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		hasLetter = hasLetter || unicode.IsLetter(r) || unicode.IsMark(r)
		n += size
	}
	if !hasLetter {
		return Segment{}, 0, false
	}
	name := s[1:n]
	return Segment{Kind: SegmentHashtag, Text: s[:n], Hashtag: Hashtag{Name: name, NameNormalized: NormalizeTag(name)}}, n, true
}

func scanURL(s string) (Segment, int, bool) {
	lower := strings.ToLower(s[:min(len(s), len("https://"))])
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return Segment{}, 0, false
	}
	n := spanFunc(s, isURLByte)
	n = trimURL(s[:n])
	u, err := url.Parse(s[:n])
	if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil {
		return Segment{}, 0, false
	}
	return Segment{Kind: SegmentURL, Text: s[:n], Href: u.String()}, n, true
}

// trimURL returns the length of u without trailing punctuation and unbalanced closing brackets.
func trimURL(u string) int {
	n := len(u)
	for n > 0 {
		c := u[n-1]
		switch {
		case strings.IndexByte(".,:;!?'\"*", c) >= 0:
			n--
		case c == ')' && strings.Count(u[:n], "(") < strings.Count(u[:n], ")"),
			c == ']' && strings.Count(u[:n], "[") < strings.Count(u[:n], "]"):
			n--
		default:
			return n
		}
	}
	return n
}

func spanFunc(s string, ok func(byte) bool) int {
	n := 0
	for n < len(s) && ok(s[n]) {
		n++
	}
	return n
}

func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

func isAlnumByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isUsernameByte(c byte) bool { return isAlnumByte(c) || c == '_' }

func isDomainByte(c byte) bool { return isAlnumByte(c) || c == '.' || c == '-' }

// isURLByte reports whether c may appear in an autolinked URL: printable ASCII except white
// space and the characters that RFC 3986 never allows unescaped.
func isURLByte(c byte) bool {
	return c > ' ' && c < 0x7f && strings.IndexByte("<>\"`{}|\\^", c) < 0
}

// Mentions returns the mentioned handles, each once, in the order they first appear.
func (p ParsedText) Mentions() []MentionHandle {
	var out []MentionHandle
	for _, s := range p.Segments {
		if s.Kind == SegmentMention && !slices.Contains(out, s.Mention) {
			out = append(out, s.Mention)
		}
	}
	return out
}

// Hashtags returns the hashtags, each normalized name once with the spelling it first appears
// in, in that order.
func (p ParsedText) Hashtags() []Hashtag {
	var out []Hashtag
	for _, s := range p.Segments {
		if s.Kind == SegmentHashtag && !slices.ContainsFunc(out, func(h Hashtag) bool {
			return h.NameNormalized == s.Hashtag.NameNormalized
		}) {
			out = append(out, s.Hashtag)
		}
	}
	return out
}

// MentionTarget is the account a mention links to.
type MentionTarget struct {
	// Username is shown after the @, the account's own spelling.
	Username string
	// URL is the account's profile page.
	URL string
}

// RenderOptions are what HTML needs beyond the text.
type RenderOptions struct {
	// Host is the serving domain of the hashtag pages (ADR-0018).
	Host string
	// Mentions are the resolved mentions. A mention without a target renders as text.
	Mentions map[MentionHandle]MentionTarget
}

// linkRel is the rel of every link (ADR-0019 §2).
const linkRel = `rel="nofollow noopener noreferrer" target="_blank"`

// urlDisplayLength is the number of characters of a URL shown before it is cut with an ellipsis.
const urlDisplayLength = 30

// HTML renders the text as the content of a status. Empty text renders as "".
func (p ParsedText) HTML(o RenderOptions) string {
	if len(p.Segments) == 0 {
		return ""
	}
	w := &htmlWriter{}
	w.b.WriteString("<p>")
	for _, s := range p.Segments {
		switch s.Kind {
		case SegmentMention:
			target, ok := o.Mentions[s.Mention]
			if !ok {
				w.text(s.Text)
				continue
			}
			w.raw(`<span class="h-card"><a href="` + html.EscapeString(target.URL) + `" class="u-url mention" ` + linkRel +
				`>@<span>` + html.EscapeString(target.Username) + `</span></a></span>`)
		case SegmentHashtag:
			w.raw(`<a href="` + html.EscapeString(TagURL(o.Host, s.Hashtag.NameNormalized)) + `" class="mention hashtag" ` + linkRel +
				`>#<span>` + html.EscapeString(s.Hashtag.Name) + `</span></a>`)
		case SegmentURL:
			w.raw(`<a href="` + html.EscapeString(s.Href) + `" ` + linkRel + `>` + urlDisplay(s.Text) + `</a>`)
		default:
			w.text(s.Text)
		}
	}
	w.b.WriteString("</p>")
	return w.b.String()
}

// TagURL returns the HTML page of a hashtag on host (ADR-0018), for a normalized tag name.
func TagURL(host, nameNormalized string) string {
	return "https://" + host + "/tags/" + url.PathEscape(nameNormalized)
}

// urlDisplay returns the visible text of a URL link: the scheme and "www." are hidden, and
// what follows the first urlDisplayLength characters is hidden behind an ellipsis.
func urlDisplay(u string) string {
	prefixLen := strings.Index(u, "://") + len("://")
	if strings.HasPrefix(strings.ToLower(u[prefixLen:]), "www.") {
		prefixLen += len("www.")
	}
	prefix, rest := u[:prefixLen], u[prefixLen:]
	out := `<span class="invisible">` + html.EscapeString(prefix) + `</span>`
	if len(rest) <= urlDisplayLength { // the URL is ASCII, so bytes are characters
		return out + `<span>` + html.EscapeString(rest) + `</span>`
	}
	return out + `<span class="ellipsis">` + html.EscapeString(rest[:urlDisplayLength]) + `</span>` +
		`<span class="invisible">` + html.EscapeString(rest[urlDisplayLength:]) + `</span>`
}

// htmlWriter writes escaped text, turning one line break into <br> and more into a new
// paragraph.
type htmlWriter struct {
	b        strings.Builder
	newlines int
}

func (w *htmlWriter) raw(s string) {
	w.breaks()
	w.b.WriteString(s)
}

func (w *htmlWriter) text(s string) {
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			w.raw(html.EscapeString(s))
			return
		}
		if i > 0 {
			w.raw(html.EscapeString(s[:i]))
		}
		w.newlines++
		s = s[i+1:]
	}
}

func (w *htmlWriter) breaks() {
	switch {
	case w.newlines == 1:
		w.b.WriteString("<br>")
	case w.newlines > 1:
		w.b.WriteString("</p><p>")
	}
	w.newlines = 0
}
