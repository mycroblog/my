// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	nethtml "golang.org/x/net/html"

	"github.com/mycroblog/my/internal/app/instance"
)

// documentElements and documentAttrs are the allowlist that DocumentRendererContract checks
// every output against.
var (
	documentElements = []string{
		"p", "br", "hr", "h2", "h3", "h4", "h5", "h6", "strong", "em", "del", "code", "pre", "blockquote",
		"ul", "ol", "li", "table", "thead", "tbody", "tr", "th", "td", "a",
	}
	documentAttrs = map[string][]string{"a": {"href", "rel"}, "ol": {"start"}}
)

// DocumentRendererContract verifies an instance.DocumentRenderer adapter: Markdown becomes the
// expected elements, headings start at <h2>, line breaks are kept, raw HTML and images are
// dropped, links keep only safe URLs and never pass the referrer, and every output holds only
// allowlisted elements and attributes.
func DocumentRendererContract(t *testing.T, newRenderer func(t *testing.T) instance.DocumentRenderer) {
	t.Helper()
	for _, tc := range []struct {
		name, src string
		// has and hasNot are substrings of the output.
		has, hasNot []string
	}{
		{name: "Empty", src: ""},
		{name: "Headings", src: "# Terms\n\n## Accounts\n\n###### Deep", has: []string{"<h2>Terms</h2>", "<h3>Accounts</h3>", "<h6>Deep</h6>"}, hasNot: []string{"<h1"}},
		{name: "Paragraphs", src: "One\ntwo\n\nThree", has: []string{"<p>One<br", "two</p>", "<p>Three</p>"}},
		{name: "Emphasis", src: "**bold** *em* ~~gone~~ `code`", has: []string{"<strong>bold</strong>", "<em>em</em>", "<del>gone</del>", "<code>code</code>"}},
		{name: "Lists", src: "- a\n- b\n\n3. c\n4. d", has: []string{"<ul>", "<li>a</li>", `<ol start="3">`, "<li>c</li>"}},
		{name: "Quote", src: "> quoted", has: []string{"<blockquote>", "quoted"}},
		{name: "Rule", src: "a\n\n---\n\nb", has: []string{"<hr"}},
		{name: "Table", src: "| a | b |\n|:--|--:|\n| 1 | 2 |", has: []string{"<table>", "<th>a</th>", "<td>2</td>"}, hasNot: []string{"align", "style"}},
		{name: "Link", src: "[site](https://example.com/a?b=1)", has: []string{`href="https://example.com/a?b=1"`, "noreferrer", "nofollow", ">site</a>"}},
		{name: "Mail", src: "[mail](mailto:admin@example.com)", has: []string{`href="mailto:admin@example.com"`}},
		{name: "Autolink", src: "See https://example.com/x.", has: []string{`href="https://example.com/x"`}},
		{name: "JavaScriptLink", src: "[x](javascript:alert(1))", hasNot: []string{"javascript", "<a "}},
		{name: "DataLink", src: "[x](data:text/html,<script>alert(1)</script>)", hasNot: []string{"data:", "<script"}},
		{name: "RawHTML", src: "<script>alert(1)</script>\n\n<b onclick=\"x()\">hi</b> <iframe src=\"https://example.com\"></iframe>", hasNot: []string{"<script", "onclick", "<iframe", "<b"}},
		{name: "Image", src: "![alt](https://example.com/a.png) <img src=x onerror=alert(1)>", hasNot: []string{"<img", "onerror"}},
		{name: "Style", src: "<style>body{}</style>\n\ntext", has: []string{"text"}, hasNot: []string{"<style"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := newRenderer(t).RenderDocument(tc.src)
			for _, s := range tc.has {
				if !strings.Contains(got, s) {
					t.Errorf("RenderDocument(%q) = %q, want it to contain %q", tc.src, got, s)
				}
			}
			for _, s := range tc.hasNot {
				if strings.Contains(got, s) {
					t.Errorf("RenderDocument(%q) = %q, want it without %q", tc.src, got, s)
				}
			}
			checkDocumentHTML(t, got)
		})
	}
}

// checkDocumentHTML fails unless h holds only the elements and attributes of the document
// allowlist, and no comments.
func checkDocumentHTML(t *testing.T, h string) {
	t.Helper()
	z := nethtml.NewTokenizer(strings.NewReader(h))
	for {
		switch z.Next() {
		case nethtml.ErrorToken:
			if !errors.Is(z.Err(), io.EOF) {
				t.Fatalf("tokenize %q: %v", h, z.Err())
			}
			return
		case nethtml.CommentToken, nethtml.DoctypeToken:
			t.Fatalf("comment or doctype in %q", h)
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken, nethtml.EndTagToken:
			tok := z.Token()
			if !slices.Contains(documentElements, tok.Data) {
				t.Fatalf("element <%s> is not allowlisted in %q", tok.Data, h)
			}
			for _, a := range tok.Attr {
				if !slices.Contains(documentAttrs[tok.Data], a.Key) {
					t.Fatalf("attribute %s of <%s> is not allowlisted in %q", a.Key, tok.Data, h)
				}
			}
		}
	}
}
