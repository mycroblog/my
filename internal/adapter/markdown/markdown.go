// SPDX-License-Identifier: AGPL-3.0-only

// Package markdown implements instance.DocumentRenderer: it renders the Markdown of the
// instance's documents (the terms of service and the privacy policy) with goldmark (CommonMark,
// with the GitHub Flavored Markdown tables, strikethrough, and autolinks) and sanitizes the
// result with the document allowlist of internal/adapter/htmlsanitizer. ArticleRenderer, with a
// configuration of its own, renders articles (article.go).
//
// Raw HTML in the source is never rendered (goldmark escapes it unless told otherwise, and the
// sanitizer drops what is left). A line break inside a paragraph is a line break (<br>), as
// administrators who are not used to Markdown expect. Headings start at <h2>, below the page's
// own <h1>: "#" is <h2> and "#####" or deeper is <h6>. Images are dropped.
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/mycroblog/my/internal/adapter/htmlsanitizer"
)

// Renderer renders documents. It is safe for concurrent use.
type Renderer struct {
	md goldmark.Markdown
}

// New returns a Renderer.
func New() *Renderer {
	return &Renderer{md: goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify),
		goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(headingShift{}, 100))),
		goldmark.WithRendererOptions(html.WithHardWraps()),
	)}
}

// RenderDocument implements instance.DocumentRenderer.
func (r *Renderer) RenderDocument(src string) string {
	var b bytes.Buffer
	if err := r.md.Convert([]byte(src), &b); err != nil {
		// Converting into a bytes.Buffer fails only when writing fails, which it does not.
		return ""
	}
	return htmlsanitizer.SanitizeDocument(b.String())
}

// headingShift moves every heading one level down, so that the document's headings come under
// the page's <h1>.
type headingShift struct{}

func (headingShift) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering {
			h.Level = min(h.Level+1, 6)
		}
		return ast.WalkContinue, nil
	})
}
