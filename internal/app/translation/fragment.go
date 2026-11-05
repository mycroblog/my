// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"fmt"
	"html"
	"strings"
	"unicode"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

// This file turns a post into the fragments that a provider receives, and the provider's answer
// back into the post (ADR-0148 §8). A body is split at the boundaries of its blocks. Whatever must
// be neither translated nor sent stays here and a marker goes in its place: mentions, hashtags,
// links, :shortcode:, code, and text of the post that has the form of a marker. Decoration stays
// here too, as a pair of markers around the words that it covers. So a provider sees no handle,
// no URL, and no markup, and text in its answer is only ever text: restoring escapes it.

// maskedText is one fragment: its text with markers, and what each marker stands for, in the
// encoding of the field that it goes back into.
type maskedText struct {
	fragment string
	replaced map[string]string
	// translated is what restore reads: the fragment until a translation replaces it.
	translated string
}

// sendable reports whether the fragment has words for a provider to translate. Whitespace,
// punctuation, numbers, and markers alone come back as they are, without being sent.
func (t *maskedText) sendable() bool {
	return strings.ContainsFunc(markerPattern.ReplaceAllString(t.fragment, ""), unicode.IsLetter)
}

// restore returns the translation with the markers replaced by what they stand for, and the text
// between them passed through encode.
func (t *maskedText) restore(encode func(string) string) string {
	var b strings.Builder
	last := 0
	for _, loc := range markerPattern.FindAllStringIndex(t.translated, -1) {
		b.WriteString(encode(t.translated[last:loc[0]]))
		b.WriteString(t.replaced[t.translated[loc[0]:loc[1]]])
		last = loc[1]
	}
	b.WriteString(encode(t.translated[last:]))
	return b.String()
}

// masker builds one maskedText. The markers number from 1 in each fragment.
type masker struct {
	encode   func(string) string
	out      strings.Builder
	text     strings.Builder // text not written yet, so that adjacent text nodes read as one
	last     int
	replaced map[string]string
}

func newMasker(encode func(string) string) *masker {
	return &masker{encode: encode, replaced: make(map[string]string)}
}

// item writes a marker that stands for original.
func (m *masker) item(original string) {
	m.flush()
	m.last++
	marker := Marker(m.last)
	m.replaced[marker] = original
	m.out.WriteString(marker)
}

// open writes the opening marker of a pair and returns what writes the closing one.
func (m *masker) open(opening, closing string) func() {
	m.flush()
	m.last++
	n := m.last
	m.replaced[Marker(n)] = opening
	m.replaced[ClosingMarker(n)] = closing
	m.out.WriteString(Marker(n))
	return func() {
		m.flush()
		m.out.WriteString(ClosingMarker(n))
	}
}

// flush writes the pending text, in which it replaces what a provider must not see even when no
// element marks it: a remote body or a content warning may have a handle or a URL as plain text.
func (m *masker) flush() {
	text := m.text.String()
	if text == "" {
		return
	}
	m.text.Reset()
	core := strings.TrimSpace(text) // ParseText drops the white space around the text
	start := strings.Index(text, core)
	m.out.WriteString(text[:start])
	if core != "" {
		for _, seg := range appcontent.ParseText(core).Segments {
			if seg.Kind == appcontent.SegmentText {
				m.plain(seg.Text)
			} else {
				m.item(m.encode(seg.Text))
			}
		}
	}
	m.out.WriteString(text[start+len(core):])
}

// plain writes text without handles and URLs, replacing the text that has the form of a marker,
// which could otherwise pass for a marker that this package wrote, and :shortcode:.
func (m *masker) plain(text string) {
	last := 0
	for _, loc := range markerPattern.FindAllStringIndex(text, -1) {
		m.shortcodes(text[last:loc[0]])
		m.item(m.encode(text[loc[0]:loc[1]]))
		last = loc[1]
	}
	m.shortcodes(text[last:])
}

func (m *masker) shortcodes(text string) {
	last := 0
	for _, span := range appcontent.ShortcodeSpans(text) {
		m.out.WriteString(text[last:span[0]])
		m.item(m.encode(text[span[0]:span[1]]))
		last = span[1]
	}
	m.out.WriteString(text[last:])
}

func (m *masker) finish() *maskedText {
	m.flush()
	f := m.out.String()
	return &maskedText{fragment: f, replaced: m.replaced, translated: f}
}

// maskPlain returns the fragment of a field of plain text: a content warning, a poll option, or
// alt text.
func maskPlain(text string) *maskedText {
	m := newMasker(func(s string) string { return s })
	m.text.WriteString(text)
	return m.finish()
}

// bodyPart is a part of a body: its fragment, or, without one, markup that stays as it is.
type bodyPart struct {
	literal string
	run     *maskedText
}

// maskedBody is the HTML of a body with its inline content taken out as fragments.
type maskedBody struct {
	parts []bodyPart
}

// parseBody splits sanitized HTML. The HTML comes from status_revision_bodies, and so has only the
// elements of the sanitizer's allowlist, with no block inside an inline element.
func parseBody(in string) (*maskedBody, error) {
	nodes, err := nethtml.ParseFragment(strings.NewReader(in), &nethtml.Node{
		Type: nethtml.ElementNode, Data: "body", DataAtom: atom.Body,
	})
	if err != nil {
		return nil, fmt.Errorf("translation: parse the body: %w", err)
	}
	b := &maskedBody{}
	if err := b.blocks(nodes); err != nil {
		return nil, err
	}
	return b, nil
}

// blocks adds nodes, the children of a block or the top level: a run of inline nodes is one
// fragment.
func (b *maskedBody) blocks(nodes []*nethtml.Node) error {
	var run []*nethtml.Node
	for _, n := range nodes {
		if !isBlock(n) {
			run = append(run, n)
			continue
		}
		if err := b.inlineRun(run); err != nil {
			return err
		}
		run = nil
		if n.Data == "pre" {
			pre, err := renderNode(n)
			if err != nil {
				return err
			}
			b.parts = append(b.parts, bodyPart{literal: pre})
			continue
		}
		b.parts = append(b.parts, bodyPart{literal: openingTag(n)})
		if err := b.blocks(childrenOf(n)); err != nil {
			return err
		}
		b.parts = append(b.parts, bodyPart{literal: closingTag(n)})
	}
	return b.inlineRun(run)
}

func (b *maskedBody) inlineRun(run []*nethtml.Node) error {
	if len(run) == 0 {
		return nil
	}
	m := newMasker(html.EscapeString)
	for _, n := range run {
		if err := inline(m, n); err != nil {
			return err
		}
	}
	t := m.finish()
	if strings.TrimSpace(t.fragment) == "" {
		b.parts = append(b.parts, bodyPart{literal: t.fragment})
		return nil
	}
	b.parts = append(b.parts, bodyPart{run: t})
	return nil
}

// inline adds n, which is not a block, to a fragment.
func inline(m *masker, n *nethtml.Node) error {
	switch n.Type {
	case nethtml.TextNode:
		m.text.WriteString(n.Data)
		return nil
	case nethtml.ElementNode:
	default:
		return nil // comments, which sanitized HTML does not have
	}
	if !hasWords(n) {
		item, err := renderNode(n)
		if err != nil {
			return err
		}
		m.item(item)
		return nil
	}
	closePair := m.open(openingTag(n), closingTag(n))
	for _, c := range childrenOf(n) {
		if err := inline(m, c); err != nil {
			return err
		}
	}
	closePair()
	return nil
}

// hasWords reports whether n has text to translate, so that it is decoration around the text
// rather than one item that stays as it is. The content of links, which is how mentions and
// hashtags are written, of code, and of ruby (a word with its reading, which would not survive
// translation) stays as it is, and so does a block inside an inline element, which sanitized HTML
// does not have.
func hasWords(n *nethtml.Node) bool {
	switch n.Type {
	case nethtml.TextNode:
		return strings.TrimSpace(n.Data) != ""
	case nethtml.ElementNode:
	default:
		return false
	}
	switch n.Data {
	case "a", "code", "ruby", "br":
		return false
	}
	if isBlock(n) {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasWords(c) {
			return true
		}
	}
	return false
}

func isBlock(n *nethtml.Node) bool {
	if n.Type != nethtml.ElementNode {
		return false
	}
	switch n.Data {
	case "p", "pre", "blockquote", "ul", "ol", "li":
		return true
	}
	return false
}

func childrenOf(n *nethtml.Node) []*nethtml.Node {
	var out []*nethtml.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}
	return out
}

func openingTag(n *nethtml.Node) string {
	var b strings.Builder
	b.WriteString("<" + n.Data)
	for _, a := range n.Attr {
		b.WriteString(" " + a.Key + `="` + html.EscapeString(a.Val) + `"`)
	}
	b.WriteString(">")
	return b.String()
}

func closingTag(n *nethtml.Node) string { return "</" + n.Data + ">" }

func renderNode(n *nethtml.Node) (string, error) {
	var b strings.Builder
	if err := nethtml.Render(&b, n); err != nil {
		return "", fmt.Errorf("translation: render the body: %w", err)
	}
	return b.String(), nil
}

// render returns the HTML of the body with the translations of its fragments. It is not sanitized
// yet.
func (b *maskedBody) render() string {
	var out strings.Builder
	for _, p := range b.parts {
		if p.run == nil {
			out.WriteString(p.literal)
			continue
		}
		out.WriteString(p.run.restore(html.EscapeString))
	}
	return out.String()
}

// fragments returns the fragments of the body in order.
func (b *maskedBody) fragments() []*maskedText {
	var out []*maskedText
	for _, p := range b.parts {
		if p.run != nil {
			out = append(out, p.run)
		}
	}
	return out
}
