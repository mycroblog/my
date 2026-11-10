// SPDX-License-Identifier: AGPL-3.0-only

package markdown_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/markdown"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

const ownImage = "https://media.example/large.webp"

var resolution = appcontent.ArticleResolution{
	Images: map[string]appcontent.ArticleImageSource{
		"media:m1": {MediaID: "m1", URL: ownImage, Width: 800, Height: 600},
	},
	Mentions: map[appcontent.MentionHandle]appcontent.MentionTarget{
		{Username: "alice"}: {Username: "alice", URL: "https://a.example/@alice"},
	},
}

func TestRenderArticle(t *testing.T) {
	r := markdown.NewArticleRenderer()
	for _, tc := range []struct {
		name, in string
		want     []string // substrings of the HTML
		not      []string // substrings that must not be in it
	}{
		{"headings move down", "# One\n\n##### Five\n\n###### Six", []string{"<h2>One</h2>", "<h6>Five</h6>", "<h6>Six</h6>"}, []string{"<h1"}},
		{"line breaks", "a\nb", []string{"<p>a<br>\nb</p>"}, nil},
		{"raw HTML", "<script>alert(1)</script>\n\n<img src=\"https://evil.example/x.png\" onerror=\"x\">\n\nhi <b onclick=\"x\">b</b>", []string{"hi"}, []string{"<script", "alert", "evil.example", "onerror", "onclick", "<b"}},
		{"own image", "![a cat](media:m1)", []string{`<img src="` + ownImage + `" alt="a cat" width="800" height="600" loading="lazy">`}, nil},
		{"external image", "![a dog](https://elsewhere.example/dog.png)", []string{`<a href="https://elsewhere.example/dog.png" rel="nofollow noopener noreferrer" target="_blank">a dog</a>`}, []string{"<img"}},
		{"unknown upload", "![x](media:other)", []string{"x"}, []string{"<img", "media:other"}},
		{"links", "[site](https://example.com) and https://auto.example/path", []string{
			`<a href="https://example.com" rel="nofollow noopener noreferrer" target="_blank">site</a>`,
			`<a href="https://auto.example/path" rel="nofollow noopener noreferrer" target="_blank">https://auto.example/path</a>`,
		}, nil},
		{"dangerous link", "[x](javascript:alert(1))", []string{"<p>x</p>"}, []string{"javascript", "<a"}},
		{"mentions", "hi @alice and @bob, `@alice`", []string{
			`<a href="https://a.example/@alice" class="u-url mention" rel="nofollow noopener noreferrer" target="_blank">@alice</a>`,
			"@bob", "<code>@alice</code>",
		}, nil},
		{"hashtags stay text", "about #go", []string{"<p>about #go</p>"}, []string{"hashtag"}},
		{"tables", "| a | b |\n|:-|-:|\n| 1 | 2 |", []string{"<table>", `<th align="left">a</th>`, `<td align="right">2</td>`}, nil},
		{"strikethrough", "~~gone~~", []string{"<del>gone</del>"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := r.RenderArticle(tc.in, resolution).HTML
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("HTML = %q, want it to contain %q", got, w)
				}
			}
			for _, w := range tc.not {
				if strings.Contains(got, w) {
					t.Errorf("HTML = %q, want it without %q", got, w)
				}
			}
		})
	}
}

func TestRenderArticleImagesAndText(t *testing.T) {
	r := markdown.NewArticleRenderer()
	in := "# Title\n\nFirst ![one](media:m1) and ![again](media:m1).\n\n```\ncode here\n```\n\n- item `x`\n\n![link](https://e.example/a.png)"
	got := r.RenderArticle(in, resolution)
	if want := []content.ArticleImage{{Position: 0, MediaID: "m1", Description: "one"}}; !slices.Equal(got.Images, want) {
		t.Errorf("Images = %+v, want %+v", got.Images, want)
	}
	text := strings.Join(strings.Fields(got.Text), " ")
	if want := "First and . item"; text != want {
		t.Errorf("Text = %q, want %q", text, want)
	}
}

func TestArticleReferences(t *testing.T) {
	r := markdown.NewArticleRenderer()
	got := r.ArticleReferences("@alice @bob@b.example `@carol` [@dave](https://x.example) mail@e.example\n\n![a](media:m1) ![b](media:m1) ![c](https://e.example/c.png)")
	if want := []string{"media:m1", "https://e.example/c.png"}; !slices.Equal(got.Images, want) {
		t.Errorf("Images = %q, want %q", got.Images, want)
	}
	want := []appcontent.MentionHandle{{Username: "alice"}, {Username: "bob", Domain: "b.example"}}
	if !slices.Equal(got.Mentions, want) {
		t.Errorf("Mentions = %+v, want %+v", got.Mentions, want)
	}
}

// TestRenderArticleIsStable checks that rendering the same source twice gives the same HTML, so
// that the preview and the published article are the same (ADR-0160 §4).
func TestRenderArticleIsStable(t *testing.T) {
	r := markdown.NewArticleRenderer()
	in := "# A\n\ntext ![i](media:m1) @alice"
	if a, b := r.RenderArticle(in, resolution).HTML, r.RenderArticle(in, resolution).HTML; a != b {
		t.Errorf("two renderings differ:\n%s\n%s", a, b)
	}
}
