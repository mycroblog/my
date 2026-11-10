// SPDX-License-Identifier: AGPL-3.0-only

package markdown

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/mycroblog/my/internal/adapter/htmlsanitizer"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// ArticleRenderer implements appcontent.ArticleRenderer: it renders the Markdown of articles
// (ADR-0160 §4) with its own configuration, apart from the documents' Renderer, and sanitizes the
// result with the article allowlist of internal/adapter/htmlsanitizer. It is safe for concurrent
// use.
//
// It renders CommonMark with the tables, strikethrough, and autolinks of GitHub Flavored
// Markdown. Raw HTML is never rendered, a line break inside a paragraph is <br>, and headings
// start at <h2>. An image whose destination the resolution names becomes that image; any other
// image becomes a link to its destination with its alt text as the text, so that no reader's
// browser loads an image from elsewhere. A mention (@user or @user@domain, outside code and links)
// that resolves becomes a link to the account; #name stays text.
type ArticleRenderer struct {
	md goldmark.Markdown
}

var _ appcontent.ArticleRenderer = (*ArticleRenderer)(nil)

// NewArticleRenderer returns an ArticleRenderer.
func NewArticleRenderer() *ArticleRenderer {
	return &ArticleRenderer{md: goldmark.New(
		goldmark.WithExtensions(
			extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
			extension.Strikethrough, extension.Linkify,
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(headingShift{}, 100)),
			parser.WithInlineParsers(util.Prioritized(mentionParser{}, 600)),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			renderer.WithNodeRenderers(util.Prioritized(mentionRenderer{}, 100)),
		),
	)}
}

// ArticleReferences implements appcontent.ArticleRenderer.
func (r *ArticleRenderer) ArticleReferences(source string) appcontent.ArticleReferences {
	src := []byte(source)
	doc := r.md.Parser().Parse(text.NewReader(src))
	var out appcontent.ArticleReferences
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Image:
			if d := string(n.Destination); !contains(out.Images, d) {
				out.Images = append(out.Images, d)
			}
		case *mentionNode:
			if !contains(out.Mentions, n.handle) {
				out.Mentions = append(out.Mentions, n.handle)
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// RenderArticle implements appcontent.ArticleRenderer.
func (r *ArticleRenderer) RenderArticle(source string, res appcontent.ArticleResolution) appcontent.ArticleRendering {
	src := []byte(source)
	doc := r.md.Parser().Parse(text.NewReader(src))
	var out appcontent.ArticleRendering
	keep := map[string]bool{}
	var images []*ast.Image
	var mentions []*mentionNode
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n := n.(type) {
			case *ast.Image:
				images = append(images, n)
			case *mentionNode:
				mentions = append(mentions, n)
			}
		}
		return ast.WalkContinue, nil
	})
	// linkedImages are the images that became links; their alt text is no text of the body.
	linkedImages := map[ast.Node]bool{}
	for _, img := range images {
		alt := plainText(img, src)
		found, ok := res.Images[string(img.Destination)]
		if !ok {
			link := ast.NewLink()
			link.Destination = img.Destination
			for c := img.FirstChild(); c != nil; {
				next := c.NextSibling()
				link.AppendChild(link, c)
				c = next
			}
			if !link.HasChildren() {
				link.AppendChild(link, ast.NewString(img.Destination))
			}
			img.Parent().ReplaceChild(img.Parent(), img, link)
			linkedImages[link] = true
			continue
		}
		img.Destination = []byte(found.URL)
		img.Title = nil
		if found.Width > 0 && found.Height > 0 {
			img.SetAttributeString("width", []byte(strconv.Itoa(found.Width)))
			img.SetAttributeString("height", []byte(strconv.Itoa(found.Height)))
		}
		img.SetAttributeString("loading", []byte("lazy"))
		keep[found.URL] = true
		if !containsMedia(out.Images, found.MediaID) {
			out.Images = append(out.Images, content.ArticleImage{Position: len(out.Images), MediaID: found.MediaID, Description: alt})
		}
	}
	for _, m := range mentions {
		target, ok := res.Mentions[m.handle]
		var replacement ast.Node
		if ok {
			link := ast.NewLink()
			link.Destination = []byte(target.URL)
			link.SetAttributeString("class", []byte("u-url mention"))
			link.AppendChild(link, ast.NewString([]byte("@"+target.Username)))
			replacement = link
		} else {
			replacement = ast.NewTextSegment(m.segment)
		}
		m.Parent().ReplaceChild(m.Parent(), m, replacement)
	}
	out.Text = bodyText(doc, src, linkedImages)
	var b bytes.Buffer
	if err := r.md.Renderer().Render(&b, src, doc); err != nil {
		// Rendering into a bytes.Buffer fails only when writing fails, which it does not.
		return out
	}
	out.HTML = strings.TrimSpace(htmlsanitizer.SanitizeArticle(b.String(), func(s string) bool { return keep[s] }))
	return out
}

// bodyText returns the text of doc without its headings, code, raw HTML, and images (and the
// images that became links), with a space between blocks and lines.
func bodyText(doc ast.Node, src []byte, skip map[ast.Node]bool) string {
	var b strings.Builder
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if n.Type() == ast.TypeBlock {
				b.WriteByte(' ')
			}
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading, *ast.CodeBlock, *ast.FencedCodeBlock, *ast.CodeSpan, *ast.HTMLBlock, *ast.RawHTML, *ast.Image:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			b.Write(n.Value(src))
			if n.SoftLineBreak() || n.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(n.Value)
		case *ast.AutoLink:
			b.Write(n.Label(src))
		default:
			if skip[n] {
				return ast.WalkSkipChildren, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// plainText returns the text of n's descendants, as goldmark writes an image's alt text.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Value(src))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

func contains[T comparable](list []T, v T) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsMedia(images []content.ArticleImage, mediaID string) bool {
	for _, img := range images {
		if img.MediaID == mediaID {
			return true
		}
	}
	return false
}

// mentionNode is a mention in the text of an article, @user or @user@domain, which RenderArticle
// turns into a link or back into text.
type mentionNode struct {
	ast.BaseInline
	handle  appcontent.MentionHandle
	segment text.Segment
}

// kindMention is the kind of mentionNode.
var kindMention = ast.NewNodeKind("Mention")

func (n *mentionNode) Kind() ast.NodeKind { return kindMention }

func (n *mentionNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Handle": n.handle.Username + "@" + n.handle.Domain}, nil)
}

// mentionParser reads a mention where a text would link one (appcontent.ParseText): @ that does
// not follow a word character, "/", or "=", then a username and optionally @ and a domain.
// Inline parsers do not run inside code, and a mention inside a link stays text.
type mentionParser struct{}

func (mentionParser) Trigger() []byte { return []byte{'@'} }

func (mentionParser) Parse(_ ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if pc.IsInLinkLabel() {
		return nil
	}
	if prev := block.PrecendingCharacter(); unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_' || prev == '/' || prev == '=' {
		return nil
	}
	line, segment := block.PeekLine()
	parsed := appcontent.ParseText(string(line))
	if len(parsed.Segments) == 0 || parsed.Segments[0].Kind != appcontent.SegmentMention || !bytes.HasPrefix(line, []byte(parsed.Segments[0].Text)) {
		return nil
	}
	n := len(parsed.Segments[0].Text)
	block.Advance(n)
	return &mentionNode{handle: parsed.Segments[0].Mention, segment: segment.WithStop(segment.Start + n)}
}

// mentionRenderer writes a mentionNode left in the tree as its text.
type mentionRenderer struct{}

func (mentionRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMention, func(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.Write(util.EscapeHTML(n.(*mentionNode).segment.Value(src)))
		}
		return ast.WalkContinue, nil
	})
}
