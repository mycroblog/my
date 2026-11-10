// SPDX-License-Identifier: AGPL-3.0-only

package htmlsanitizer_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/htmlsanitizer"
)

func TestSanitizeArticle(t *testing.T) {
	keep := func(src string) bool { return src == "https://media.example/a.webp" }
	for _, tc := range []struct{ name, in, want string }{
		{"headings", "<h1>A</h1><h3>B</h3>", "<h2>A</h2><h3>B</h3>"},
		{"kept image", `<p><img src="https://media.example/a.webp" alt="cat" width="10" height="20" loading="eager" style="x"></p>`,
			`<p><img src="https://media.example/a.webp" alt="cat" width="10" height="20" loading="lazy"></p>`},
		{"other image", `<p>x<img src="https://evil.example/a.png" alt="dog">y</p>`,
			`<p>x<a href="https://evil.example/a.png" rel="nofollow noopener noreferrer" target="_blank">dog</a>y</p>`},
		{"other image without alt text", `<img src="https://evil.example/a.png">`,
			`<a href="https://evil.example/a.png" rel="nofollow noopener noreferrer" target="_blank">https://evil.example/a.png</a>`},
		{"other image in a link", `<a href="https://e.example"><img src="https://evil.example/a.png" alt="dog"></a>`,
			`<a href="https://e.example" rel="nofollow noopener noreferrer" target="_blank">dog</a>`},
		{"image of no link", `<img src="data:image/png;base64,AAAA" alt="dot">`, "dot"},
		{"links", `<a href="https://e.example" class="u-url mention evil" onclick="x">e</a>`,
			`<a href="https://e.example" class="u-url mention" rel="nofollow noopener noreferrer" target="_blank">e</a>`},
		{"bad scheme", `<a href="javascript:alert(1)">x</a><a href="mailto:a@b.example">m</a>`, "xm"},
		{"nested links", `<a href="https://a.example"><a href="https://b.example">b</a></a>`,
			`<a href="https://a.example" rel="nofollow noopener noreferrer" target="_blank"></a><a href="https://b.example" rel="nofollow noopener noreferrer" target="_blank">b</a>`},
		{"spans", `<span class="h-card invisible">x</span>`, `<span class="invisible">x</span>`},
		{"tables", `<table><tr><th align="center" style="x">a</th><td align="justify">b</td></tr></table>`,
			`<table><tbody><tr><th align="center">a</th><td>b</td></tr></tbody></table>`},
		{"dropped", `<script>x</script><style>y</style><!-- c --><iframe src="https://e.example"></iframe><div>d</div>`, "d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := htmlsanitizer.SanitizeArticle(tc.in, keep); got != tc.want {
				t.Errorf("SanitizeArticle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
