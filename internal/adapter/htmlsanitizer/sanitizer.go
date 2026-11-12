// SPDX-License-Identifier: AGPL-3.0-only

// Package htmlsanitizer implements kernel.HTMLSanitizer with the allowlist of ADR-0019 §2,
// which follows Mastodon's Sanitize::Config::MASTODON_STRICT (lib/sanitize_ext/sanitize_config.rb)
// and adds noreferrer to the rel of links (ADR-0019 §2).
//
// Sanitizing runs in two passes. The first parses the fragment into a tree with
// golang.org/x/net/html (the HTML5 parsing algorithm, so that the tree is the one a browser
// builds) and rewrites it as Mastodon's transformers do: headings become <p><strong>,
// other block elements become <p>, <math> becomes its TeX annotation, links with a scheme
// outside the allowlist become their text, and class and translate are filtered. It writes
// only allowlisted elements and attributes. The second pass runs the same allowlist through
// bluemonday (ADR-0019 §2), so that a mistake in the first pass cannot let markup through.
// bluemonday is used only in this package.
package htmlsanitizer

import (
	"html"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/mycroblog/my/internal/app/kernel"
)

// PolicyVersion is the version of the policy, stored as html_policy_version next to the
// HTML it produced (remote_status_revisions.html_policy_version). Increase it with every change that changes outputs.
const PolicyVersion = 1

// LinkRel is the rel every link gets (ADR-0019 §2); local links use the same value.
const LinkRel = "nofollow noopener noreferrer"

// linkSchemes are the schemes a link may have; any other link, relative ones included,
// becomes its text.
var linkSchemes = []string{"http", "https", "dat", "dweb", "ipfs", "ipns", "ssb", "gopher", "xmpp", "magnet", "gemini"}

// allowedElements are the elements written as they are.
var allowedElements = []string{
	"p", "br", "span", "a", "del", "s", "pre", "blockquote", "code", "b", "strong", "u", "i", "em",
	"ul", "ol", "li", "ruby", "rt", "rp",
}

// blockElements are the allowlisted elements that are blocks. Together with headings and
// converted blocks, they are never written inside a paragraph or an inline element.
var blockElements = []string{"p", "pre", "blockquote", "ul", "ol", "li"}

// headingElements become <p><strong>…</strong></p>.
var headingElements = []string{"h1", "h2", "h3", "h4", "h5", "h6"}

// convertedBlocks are the block elements outside the allowlist that become <p> so that the
// text keeps its paragraph breaks.
var convertedBlocks = []string{
	"address", "article", "aside", "caption", "center", "dd", "details", "dialog", "dir", "div", "dl", "dt",
	"fieldset", "figcaption", "figure", "footer", "form", "header", "hgroup", "legend", "main", "menu", "nav",
	"section", "summary", "table", "tbody", "td", "tfoot", "th", "thead", "tr",
}

// droppedElements lose their contents as well as their tags: their text is not meant to be
// read as part of the content (Sanitize's remove_contents, plus a few embedding elements).
var droppedElements = []string{
	"script", "style", "template", "iframe", "frame", "frameset", "object", "embed", "noembed", "noframes",
	"noscript", "plaintext", "svg", "xmp", "title", "textarea", "select", "head",
}

var (
	// integerValue is the value of ol[start] and li[value].
	integerValue = regexp.MustCompile(`^-?[0-9]{1,9}$`)
	// classList is a class attribute that the first pass can write.
	classList = regexp.MustCompile(`^(?:(?:h|p|u|dt|e)-\S*|mention|hashtag|ellipsis|invisible|quote-inline)(?: (?:(?:h|p|u|dt|e)-\S*|mention|hashtag|ellipsis|invisible|quote-inline))*$`)
	// microformat is a microformats class (h-card, u-url, ...).
	microformat = regexp.MustCompile(`^(?:h|p|u|dt|e)-`)
)

// Sanitizer is the kernel.HTMLSanitizer. The zero value is not usable; use New. It is safe
// for concurrent use.
type Sanitizer struct {
	final *bluemonday.Policy
	plain *bluemonday.Policy
}

var _ kernel.HTMLSanitizer = (*Sanitizer)(nil)

// New returns the sanitizer.
func New() *Sanitizer {
	final := bluemonday.NewPolicy()
	final.AllowElements(allowedElements...)
	final.AllowAttrs("lang").Globally()
	final.AllowAttrs("href").OnElements("a")
	final.RequireParseableURLs(true)
	final.AllowRelativeURLs(false)
	final.AllowURLSchemes(linkSchemes...)
	final.AllowAttrs("rel").Matching(regexp.MustCompile(`^` + LinkRel + `$`)).OnElements("a")
	final.AllowAttrs("target").Matching(regexp.MustCompile(`^_blank$`)).OnElements("a")
	final.AllowAttrs("class").Matching(classList).OnElements("a", "span", "p")
	final.AllowAttrs("translate").Matching(regexp.MustCompile(`^no$`)).OnElements("a", "span")
	final.AllowAttrs("start").Matching(integerValue).OnElements("ol")
	final.AllowAttrs("reversed").Matching(regexp.MustCompile(`^$`)).OnElements("ol")
	final.AllowAttrs("value").Matching(integerValue).OnElements("li")
	return &Sanitizer{final: final, plain: bluemonday.StrictPolicy()}
}

// PolicyVersion implements kernel.HTMLSanitizer.
func (*Sanitizer) PolicyVersion() int { return PolicyVersion }

// PlainText implements kernel.HTMLSanitizer.
func (s *Sanitizer) PlainText(in string) string {
	return html.UnescapeString(s.plain.Sanitize(in))
}

// SanitizeHTML implements kernel.HTMLSanitizer.
func (s *Sanitizer) SanitizeHTML(in string) string {
	nodes, err := nethtml.ParseFragment(strings.NewReader(in), &nethtml.Node{
		Type: nethtml.ElementNode, Data: "body", DataAtom: atom.Body,
	})
	if err != nil { // only a failing reader makes the parser fail; a strings.Reader does not
		return ""
	}
	var w strings.Builder
	for _, n := range nodes {
		writeNode(&w, n, context{})
	}
	return s.final.Sanitize(w.String())
}

// context is where the first pass writes a node. The output nests elements only in ways
// that the HTML parser rebuilds as they are, so that sanitizing an output again returns it
// unchanged: no block inside a paragraph or an inline element, no <li> outside a list, and
// no <rt> or <rp> outside <ruby>.
type context struct {
	// inline is true inside p, pre, a heading, or an inline element; blocks are unwrapped there.
	inline bool
	// parent is the element written around the node, or "" at the top.
	parent string
}

func (c context) in(parent string, inline bool) context {
	return context{inline: c.inline || inline, parent: parent}
}

// writeNode writes n and its descendants.
func writeNode(w *strings.Builder, n *nethtml.Node, c context) {
	switch n.Type {
	case nethtml.TextNode:
		w.WriteString(escape(n.Data))
		return
	case nethtml.ElementNode:
	default: // comments, doctypes
		return
	}
	name := n.Data
	if n.Namespace != "" && name != "math" { // other SVG and MathML elements
		return
	}
	if name == "li" && c.parent != "ul" && c.parent != "ol" {
		name = "div" // a list item outside a list is a block like any other
	}
	switch {
	case slices.Contains(droppedElements, name):
	case name == "math":
		w.WriteString(escape(mathText(n)))
	case c.inline && isBlockName(name):
		writeChildren(w, n, c)
	case slices.Contains(headingElements, name) || slices.Contains(convertedBlocks, name):
		if hasBlock(n) {
			writeBlockChildren(w, n, c)
			return
		}
		var inner strings.Builder
		writeChildren(&inner, n, c.in("p", true))
		switch {
		case inner.Len() == 0:
		case slices.Contains(headingElements, name):
			w.WriteString("<p><strong>" + inner.String() + "</strong></p>")
		default:
			w.WriteString("<p>" + inner.String() + "</p>")
		}
	case name == "a":
		href, ok := linkHref(n)
		if !ok {
			w.WriteString(escape(textContent(n)))
			return
		}
		w.WriteString(`<a href="` + escape(href) + `"`)
		writeCommonAttrs(w, n)
		w.WriteString(` rel="` + LinkRel + `" target="_blank">`)
		writeChildren(w, n, c.in("a", true))
		w.WriteString("</a>")
	case (name == "rt" || name == "rp") && c.parent != "ruby":
		writeChildren(w, n, c)
	case slices.Contains(allowedElements, name):
		if !c.inline && !isBlockName(name) && name != "br" && hasBlock(n) {
			// An inline element around blocks: keep the blocks, drop the inline element.
			writeBlockChildren(w, n, c)
			return
		}
		if name == "p" {
			var inner strings.Builder
			writeChildren(&inner, n, c.in("p", true))
			if inner.Len() > 0 { // drop empty paragraphs, such as the one a stray </p> makes
				writeStartTag(w, n, name)
				w.WriteString(inner.String() + "</p>")
			}
			return
		}
		writeStartTag(w, n, name)
		if name == "br" {
			return
		}
		if name == "pre" && n.FirstChild != nil && n.FirstChild.Type == nethtml.TextNode && strings.HasPrefix(n.FirstChild.Data, "\n") {
			w.WriteString("\n") // the parser drops the first line break after <pre>
		}
		inline := name != "blockquote" && name != "ul" && name != "ol" && name != "li"
		writeChildren(w, n, c.in(name, inline))
		w.WriteString("</" + name + ">")
	default: // other elements are unwrapped
		writeChildren(w, n, c)
	}
}

// writeStartTag writes the start tag of an allowlisted element other than a.
func writeStartTag(w *strings.Builder, n *nethtml.Node, name string) {
	w.WriteString("<" + name)
	switch name {
	case "span", "p":
		writeCommonAttrs(w, n)
	case "ol":
		writeLang(w, n)
		if v, ok := attr(n, "start"); ok && integerValue.MatchString(v) {
			w.WriteString(` start="` + v + `"`)
		}
		if _, ok := attr(n, "reversed"); ok {
			w.WriteString(` reversed=""`)
		}
	case "li":
		writeLang(w, n)
		if v, ok := attr(n, "value"); ok && integerValue.MatchString(v) {
			w.WriteString(` value="` + v + `"`)
		}
	default:
		writeLang(w, n)
	}
	w.WriteString(">")
}

func writeChildren(w *strings.Builder, n *nethtml.Node, c context) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		writeNode(w, child, c)
	}
}

// writeBlockChildren writes the children of an element that is unwrapped because it contains
// blocks: each run of children between the blocks becomes a <p>, unless it has no text.
func writeBlockChildren(w *strings.Builder, n *nethtml.Node, c context) {
	var run strings.Builder
	flush := func() {
		if strings.TrimFunc(run.String(), isHTMLSpace) != "" {
			w.WriteString("<p>" + run.String() + "</p>")
		}
		run.Reset()
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if isBlock(child) || hasBlock(child) {
			flush()
			writeNode(w, child, c)
			continue
		}
		writeNode(&run, child, c.in("p", true))
	}
	flush()
}

// writeCommonAttrs writes the class, lang, and translate attributes of a, span, and p.
func writeCommonAttrs(w *strings.Builder, n *nethtml.Node) {
	if v, ok := attr(n, "class"); ok {
		if c := filterClass(v); c != "" {
			w.WriteString(` class="` + escape(c) + `"`)
		}
	}
	writeLang(w, n)
	if n.Data != "p" {
		if v, ok := attr(n, "translate"); ok && v == "no" {
			w.WriteString(` translate="no"`)
		}
	}
}

func writeLang(w *strings.Builder, n *nethtml.Node) {
	if v, ok := attr(n, "lang"); ok {
		w.WriteString(` lang="` + escape(v) + `"`)
	}
}

// filterClass keeps the classes of Mastodon's ALLOWED_CLASS_TRANSFORMER.
func filterClass(v string) string {
	var kept []string
	for _, c := range strings.FieldsFunc(v, isHTMLSpace) {
		switch {
		case microformat.MatchString(c), c == "mention", c == "hashtag", c == "ellipsis", c == "invisible", c == "quote-inline":
			kept = append(kept, c)
		}
	}
	return strings.Join(kept, " ")
}

func isHTMLSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\f' || r == '\r'
}

// linkHref returns the href of a link in the form bluemonday writes it, or false when the
// link has no href with an allowlisted scheme.
func linkHref(n *nethtml.Node) (string, bool) {
	v, ok := attr(n, "href")
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if strings.ContainsAny(v, " \t\n") {
		return "", false
	}
	u, err := url.Parse(v)
	if err != nil || !slices.Contains(linkSchemes, u.Scheme) {
		return "", false
	}
	return u.String(), true
}

// mathText returns the text a <math> element is shown as (Mastodon's MATH_TRANSFORMER,
// FEP-dc88): its TeX annotation between dollar signs, else its plain-text annotation, else
// nothing.
func mathText(n *nethtml.Node) string {
	semantics := firstElementChild(n)
	if semantics == nil || semantics.Data != "semantics" {
		return ""
	}
	if a := annotation(semantics, "application/x-tex"); a != nil {
		if v, _ := attr(n, "display"); v == "block" {
			return "$$" + textContent(a) + "$$"
		}
		return "$" + textContent(a) + "$"
	}
	if a := annotation(semantics, "text/plain"); a != nil {
		return textContent(a)
	}
	return ""
}

func annotation(semantics *nethtml.Node, encoding string) *nethtml.Node {
	for c := semantics.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == nethtml.ElementNode && c.Data == "annotation" {
			if v, _ := attr(c, "encoding"); v == encoding {
				return c
			}
		}
	}
	return nil
}

func firstElementChild(n *nethtml.Node) *nethtml.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == nethtml.ElementNode {
			return c
		}
	}
	return nil
}

// hasBlock reports whether a descendant of n is written as a block.
func hasBlock(n *nethtml.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != nethtml.ElementNode || c.Namespace != "" || slices.Contains(droppedElements, c.Data) {
			continue
		}
		if isBlock(c) || hasBlock(c) {
			return true
		}
	}
	return false
}

// isBlock reports whether n is written as a block.
func isBlock(n *nethtml.Node) bool {
	return n.Type == nethtml.ElementNode && n.Namespace == "" && isBlockName(n.Data)
}

func isBlockName(name string) bool {
	return slices.Contains(blockElements, name) || slices.Contains(headingElements, name) || slices.Contains(convertedBlocks, name)
}

// textContent returns the text of n's descendants, without the contents of dropped elements.
func textContent(n *nethtml.Node) string {
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type == nethtml.TextNode:
				b.WriteString(c.Data)
			case c.Type == nethtml.ElementNode && !slices.Contains(droppedElements, c.Data):
				walk(c)
			}
		}
	}
	walk(n)
	return b.String()
}

// escape escapes text and attribute values as bluemonday writes them. A carriage return,
// which only a character reference can put into the tree, becomes a line feed, because the
// parser would turn a written one into a line feed anyway.
func escape(s string) string {
	return html.EscapeString(strings.ReplaceAll(s, "\r", "\n"))
}

func attr(n *nethtml.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}
