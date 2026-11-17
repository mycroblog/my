// SPDX-License-Identifier: AGPL-3.0-only

package htmlsanitizer_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/htmlsanitizer"
	appcontent "github.com/mycroblog/my/internal/app/content"
)

const linkAttrs = `rel="nofollow noopener noreferrer" target="_blank"`

func TestSanitizeRemoteArticle(t *testing.T) {
	s := htmlsanitizer.New()
	in := `<h1>Title</h1><p onclick="x">Some <em>text</em> <script>alert(1)</script>here.</p>` +
		`<p><img src="https://a.example/1.png" alt="one" width="10" style="x"><img src=" https://b.example/2.png" alt="two">` +
		`<img src="https://a.example/1.png" alt="again"><img src="https://c.example/3.png" alt="three"><img src="javascript:x" alt="bad"></p>` +
		`<pre><code>code()</code></pre><table><tr><td>cell</td></tr></table><iframe src="https://e.example"></iframe>`
	got := s.SanitizeRemoteArticle(in, 2)
	want := `<h2>Title</h2><p>Some <em>text</em> here.</p>` +
		`<p><img src="https://a.example/1.png" alt="one" width="10" loading="lazy"><img src="https://b.example/2.png" alt="two" loading="lazy">` +
		`<img src="https://a.example/1.png" alt="again" loading="lazy"><a href="https://c.example/3.png" ` + linkAttrs + `>three</a>bad</p>` +
		`<pre><code>code()</code></pre><table><tbody><tr><td>cell</td></tr></tbody></table>`
	if got.HTML != want {
		t.Errorf("HTML = %q, want %q", got.HTML, want)
	}
	wantImages := []appcontent.RemoteArticleImage{{URL: "https://a.example/1.png", Alt: "one"}, {URL: "https://b.example/2.png", Alt: "two"}}
	if !reflect.DeepEqual(got.Images, wantImages) {
		t.Errorf("Images = %+v, want %+v", got.Images, wantImages)
	}
	if text := strings.Join(strings.Fields(got.Text), " "); text != "Some text here. cell" {
		t.Errorf("Text = %q, want the text without the heading, the code, and the images", text)
	}
	if got := s.SanitizeRemoteArticle(got.HTML, 2); got.HTML != want {
		t.Errorf("sanitizing again = %q, want it unchanged", got.HTML)
	}
	if got := s.SanitizeRemoteArticle(`<p><img src="https://a.example/1.png" alt="one"></p>`, 0); got.HTML != `<p><a href="https://a.example/1.png" `+linkAttrs+`>one</a></p>` || got.Images != nil {
		t.Errorf("no images allowed = %+v, want every image a link", got)
	}
}

func TestShowArticleImages(t *testing.T) {
	s := htmlsanitizer.New()
	body := s.SanitizeRemoteArticle(`<p><img src="https://a.example/1.png" alt="one" width="10"><img src="https://b.example/2.png" alt="two"></p>`, 20).HTML
	got := s.ShowArticleImages(body, map[string]string{"https://a.example/1.png": "https://my.example/media/proxy/m1/full?sig=a&b"})
	want := `<p><img src="https://my.example/media/proxy/m1/full?sig=a&amp;b" alt="one" width="10" loading="lazy">` +
		`<a href="https://b.example/2.png" ` + linkAttrs + `>two</a></p>`
	if got != want {
		t.Errorf("ShowArticleImages = %q, want %q", got, want)
	}
}

// TestSanitizeRemoteArticleTextSeparatesBlocks checks that the text of a received body reads
// block by block, with li, th and td as items (ADR-0160 §6, §10).
func TestSanitizeRemoteArticleTextSeparatesBlocks(t *testing.T) {
	s := htmlsanitizer.New()
	for _, tc := range []struct {
		name, in, want string
	}{
		{"ja list and paragraphs", "<p>一行目<br>二行目</p><ul><li>卵</li><li>牛乳</li></ul><p>end</p>", "一行目 二行目。卵、牛乳。end"},
		{"en list and paragraphs", "<p>Ingredients</p><ul><li>eggs</li><li>milk</li></ul><p>Bake.</p><p>Then eat</p>", "Ingredients. eggs, milk. Bake. Then eat"},
		{"br", "<p>line one<br>line two<br/>line three</p>", "line one line two line three"},
		{"inline elements", "<p>a<strong>b</strong><em>c</em><a href=\"https://e.example\">d</a></p><p>e</p>", "abcd. e"},
		{"paragraphs in items", "<ol><li><p>一</p></li><li><p>二</p></li></ol>", "一、二"},
		{"nested list", "<ul><li>a<ul><li>b</li></ul></li><li>c</li></ul>", "a, b, c"},
		{"table", "<table><tr><th>名前</th><th>数</th></tr><tr><td>卵</td><td>二つ</td></tr></table><p>以上</p>", "名前、数、卵、二つ。以上"},
		{"en table", "<table><thead><tr><th>name</th></tr></thead><tbody><tr><td>egg</td></tr></tbody></table>", "name, egg"},
		{"headings skipped", "<h1>T</h1><p>a</p><h2>S</h2><p>b</p>", "a. b"},
		{"code skipped", "<p>before</p><pre><code>code()</code></pre><p>after</p>", "before. after"},
		{"quote", "<blockquote><p>quoted</p></blockquote><p>after</p>", "quoted. after"},
		{"thematic break", "<p>a</p><hr><p>b</p>", "a. b"},
		{"full-width end", "<p>終わり。</p><p>本当？</p><p>次</p>", "終わり。本当？次"},
		{"empty and image only blocks", "<p>a</p><p></p><p> </p><p><img src=\"https://a.example/1.png\" alt=\"x\"></p><p>b</p>", "a. b"},
		{"white space between blocks", "<p>  a  </p>\n\n<p>\n b\n</p>", "a. b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := s.SanitizeRemoteArticle(tc.in, 20).Text
			if text := strings.Join(strings.Fields(got), " "); text != tc.want {
				t.Errorf("Text = %q, want %q", got, tc.want)
			}
		})
	}
}
