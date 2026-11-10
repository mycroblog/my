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
