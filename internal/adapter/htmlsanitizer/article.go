// SPDX-License-Identifier: AGPL-3.0-only

package htmlsanitizer

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The allowlist of articles, the long posts in Markdown (ADR-0160 §4). It is its own, apart from
// that of remote HTML and that of the instance's documents: articles keep their headings below
// <h1> (the title is the page's <h1>), rules, tables, and the images that the caller lets
// through. Sanitizing runs in two passes, as SanitizeHTML does: the first rewrites the tree and
// writes only allowlisted elements and attributes, and the second runs the same allowlist through
// bluemonday.

// articleElements are the elements of an article written as they are; h1 is written as h2.
var articleElements = []string{
	"p", "br", "hr", "h2", "h3", "h4", "h5", "h6", "strong", "em", "del", "code", "pre", "blockquote",
	"ul", "ol", "li", "a", "img", "table", "thead", "tbody", "tr", "th", "td", "span",
}

// articleSpanClasses and articleLinkClasses are the classes that spans and links of an article
// keep: those of mentions, hashtags, and shortened URLs.
var (
	articleSpanClasses = []string{"mention", "hashtag", "invisible", "ellipsis"}
	articleLinkClasses = []string{"u-url", "mention", "hashtag"}
)

var (
	imageSize   = regexp.MustCompile(`^[0-9]{1,5}$`)
	cellAlign   = regexp.MustCompile(`^(?:left|center|right)$`)
	imageScheme = []string{"http", "https"}
)

// articlePolicy is the second pass of SanitizeArticle.
var articlePolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(articleElements...)
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src").OnElements("img")
	p.RequireParseableURLs(true)
	p.AllowRelativeURLs(false)
	p.AllowURLSchemes(linkSchemes...)
	p.AllowAttrs("rel").Matching(regexp.MustCompile(`^` + LinkRel + `$`)).OnElements("a")
	p.AllowAttrs("target").Matching(regexp.MustCompile(`^_blank$`)).OnElements("a")
	p.AllowAttrs("class").Matching(classesOf(articleLinkClasses)).OnElements("a")
	p.AllowAttrs("class").Matching(classesOf(articleSpanClasses)).OnElements("span")
	p.AllowAttrs("alt").OnElements("img")
	p.AllowAttrs("width", "height").Matching(imageSize).OnElements("img")
	p.AllowAttrs("loading").Matching(regexp.MustCompile(`^lazy$`)).OnElements("img")
	p.AllowAttrs("start").Matching(integerValue).OnElements("ol")
	p.AllowAttrs("align").Matching(cellAlign).OnElements("th", "td")
	return p
}()

// classesOf matches a class attribute made of classes.
func classesOf(classes []string) *regexp.Regexp {
	one := `(?:` + strings.Join(classes, "|") + `)`
	return regexp.MustCompile(`^` + one + `(?: ` + one + `)*$`)
}

// SanitizeArticle returns the HTML fragment of the body of an article reduced to the article
// allowlist. Every link gets rel="nofollow noopener noreferrer" and target="_blank", and a link
// whose scheme the allowlist of remote HTML does not have becomes its text; <h1> becomes <h2>. An
// image is kept only when keepImage reports true for its src, an http or https URL, with its alt
// text, size, and loading="lazy"; any other image becomes a link to its src with its alt text as
// the text (outside a link, and when the src is a link the allowlist has), else its alt text. Raw
// markup, comments, styles, and every other attribute are dropped.
func SanitizeArticle(in string, keepImage func(src string) bool) string {
	return sanitizeArticle(in, func(src, _ string) (string, bool) { return src, keepImage != nil && keepImage(src) }).html
}

// articleOutput is what sanitizeArticle writes: the HTML, and the text of the body without its
// headings, code, and images (and the links that images became).
type articleOutput struct {
	html, text string
}

// sanitizeArticle is SanitizeArticle with image, which returns the src that an image with src and
// alt is written with and whether it is kept as an image. It is called for the images in document
// order.
func sanitizeArticle(in string, image func(src, alt string) (string, bool)) articleOutput {
	nodes, err := nethtml.ParseFragment(strings.NewReader(in), &nethtml.Node{
		Type: nethtml.ElementNode, Data: "body", DataAtom: atom.Body,
	})
	if err != nil { // only a failing reader makes the parser fail; a strings.Reader does not
		return articleOutput{}
	}
	w := articleWriter{image: image}
	for _, n := range nodes {
		w.node(n, false)
	}
	return articleOutput{html: articlePolicy.Sanitize(w.b.String()), text: w.text.String()}
}

type articleWriter struct {
	b     strings.Builder
	image func(src, alt string) (string, bool)
	// text is the text of the body outside headings, code, and images; quiet counts the
	// elements around the current node whose text it leaves out.
	text  strings.Builder
	quiet int
}

// articleQuietElements are the elements whose text a summary made from the body leaves out.
var articleQuietElements = []string{"h1", "h2", "h3", "h4", "h5", "h6", "pre", "code"}

// articleBlocks are the elements after which the text of the body has a space.
var articleBlocks = []string{
	"p", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "pre", "blockquote", "ul", "ol", "li", "table", "tr", "th", "td",
}

// node writes n and its descendants; inLink is true inside a link, where links are unwrapped.
func (w *articleWriter) node(n *nethtml.Node, inLink bool) {
	switch n.Type {
	case nethtml.TextNode:
		w.b.WriteString(escape(n.Data))
		if w.quiet == 0 {
			w.text.WriteString(n.Data)
		}
		return
	case nethtml.ElementNode:
	default: // comments, doctypes
		return
	}
	name := n.Data
	quiet := slices.Contains(articleQuietElements, name)
	if quiet {
		w.quiet++
	}
	switch {
	case n.Namespace != "", slices.Contains(droppedElements, name):
	case name == "a":
		href, ok := linkHref(n)
		if !ok || inLink {
			w.children(n, inLink)
			break
		}
		w.b.WriteString(`<a href="` + escape(href) + `"`)
		if c := keepClasses(n, articleLinkClasses); c != "" {
			w.b.WriteString(` class="` + c + `"`)
		}
		w.b.WriteString(` rel="` + LinkRel + `" target="_blank">`)
		w.children(n, true)
		w.b.WriteString("</a>")
	case name == "img":
		w.writeImage(n, inLink)
	case name == "h1":
		w.element(n, "h2", inLink)
	case slices.Contains(articleElements, name):
		w.element(n, name, inLink)
	default: // other elements are unwrapped
		w.children(n, inLink)
	}
	if quiet {
		w.quiet--
	}
	if slices.Contains(articleBlocks, name) {
		w.text.WriteByte(' ')
	}
}

// element writes the allowlisted element n as name, with the attributes it keeps.
func (w *articleWriter) element(n *nethtml.Node, name string, inLink bool) {
	w.b.WriteString("<" + name)
	switch name {
	case "span":
		if c := keepClasses(n, articleSpanClasses); c != "" {
			w.b.WriteString(` class="` + c + `"`)
		}
	case "ol":
		if v, ok := attr(n, "start"); ok && integerValue.MatchString(v) {
			w.b.WriteString(` start="` + v + `"`)
		}
	case "th", "td":
		if v, ok := attr(n, "align"); ok && cellAlign.MatchString(v) {
			w.b.WriteString(` align="` + v + `"`)
		}
	}
	w.b.WriteString(">")
	if name == "br" || name == "hr" {
		return
	}
	if name == "pre" && n.FirstChild != nil && n.FirstChild.Type == nethtml.TextNode && strings.HasPrefix(n.FirstChild.Data, "\n") {
		w.b.WriteString("\n") // the parser drops the first line break after <pre>
	}
	w.children(n, inLink)
	w.b.WriteString("</" + name + ">")
}

// writeImage writes an image that w.image keeps, with the src it returns; any other image is
// written as a link to its src with its alt text as the text, or as its alt text inside a link
// or when the src is no link.
func (w *articleWriter) writeImage(n *nethtml.Node, inLink bool) {
	alt, _ := attr(n, "alt")
	src, ok := attr(n, "src")
	if !ok {
		w.b.WriteString(escape(alt))
		return
	}
	if shown, keep := w.image(src, alt); keep {
		if u, ok := imageURL(shown); ok {
			w.b.WriteString(`<img src="` + escape(u) + `" alt="` + escape(alt) + `"`)
			for _, key := range []string{"width", "height"} {
				if v, ok := attr(n, key); ok && imageSize.MatchString(v) {
					w.b.WriteString(` ` + key + `="` + v + `"`)
				}
			}
			w.b.WriteString(` loading="lazy">`)
			return
		}
	}
	href, ok := linkHref(&nethtml.Node{Type: nethtml.ElementNode, Data: "a", Attr: []nethtml.Attribute{{Key: "href", Val: src}}})
	if !ok || inLink {
		w.b.WriteString(escape(alt))
		return
	}
	text := alt
	if strings.TrimSpace(text) == "" {
		text = href
	}
	w.b.WriteString(`<a href="` + escape(href) + `" rel="` + LinkRel + `" target="_blank">` + escape(text) + `</a>`)
}

// imageURL returns src as the src of an image: an http or https URL with a host.
func imageURL(src string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(src))
	if err != nil || !slices.Contains(imageScheme, u.Scheme) || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

func (w *articleWriter) children(n *nethtml.Node, inLink bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.node(c, inLink)
	}
}

// keepClasses returns the classes of n's class attribute that are among allowed.
func keepClasses(n *nethtml.Node, allowed []string) string {
	v, ok := attr(n, "class")
	if !ok {
		return ""
	}
	var kept []string
	for _, c := range strings.FieldsFunc(v, isHTMLSpace) {
		if slices.Contains(allowed, c) && !slices.Contains(kept, c) {
			kept = append(kept, c)
		}
	}
	return strings.Join(kept, " ")
}
