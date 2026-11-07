// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
)

// HTMLSanitizerContract verifies a kernel.HTMLSanitizer adapter against the policy of
// federation.md §5 (ADR-0019 §2): script injection is neutralized (javascript: and data:
// links, event handler attributes, style, svg, nested links), the transformations of the
// policy give the expected markup, the HTML of local statuses is returned unchanged, every
// output is a fixed point, and plain-text fields lose their tags.
func HTMLSanitizerContract(t *testing.T, newSanitizer func(t *testing.T) kernel.HTMLSanitizer) {
	t.Helper()
	t.Run("PolicyVersion", func(t *testing.T) {
		if v := newSanitizer(t).PolicyVersion(); v < 1 {
			t.Errorf("PolicyVersion() = %d, want a positive version", v)
		}
	})
	t.Run("NeutralizesScriptInjection", func(t *testing.T) {
		s := newSanitizer(t)
		for _, tc := range htmlSanitizerXSSCases {
			got := s.SanitizeHTML(tc.in)
			if got != tc.want {
				t.Errorf("%s: SanitizeHTML(%q)\n got %q\nwant %q", tc.name, tc.in, got, tc.want)
			}
			CheckSanitizedHTML(t, got)
		}
	})
	t.Run("Transforms", func(t *testing.T) {
		s := newSanitizer(t)
		for _, tc := range htmlSanitizerTransformCases {
			if got := s.SanitizeHTML(tc.in); got != tc.want {
				t.Errorf("%s: SanitizeHTML(%q)\n got %q\nwant %q", tc.name, tc.in, got, tc.want)
			}
		}
	})
	t.Run("KeepsLocalHTML", func(t *testing.T) {
		s := newSanitizer(t)
		for _, text := range LocalStatusTexts {
			h := content.ParseText(text).HTML(content.RenderOptions{Host: "social.example", Mentions: LocalStatusMentions})
			if got := s.SanitizeHTML(h); got != h {
				t.Errorf("SanitizeHTML changed local HTML\n got %q\nwant %q", got, h)
			}
		}
	})
	t.Run("IsAFixedPoint", func(t *testing.T) {
		s := newSanitizer(t)
		for _, tc := range slices.Concat(htmlSanitizerXSSCases, htmlSanitizerTransformCases) {
			once := s.SanitizeHTML(tc.in)
			if twice := s.SanitizeHTML(once); twice != once {
				t.Errorf("%s: sanitizing %q again gave %q", tc.name, once, twice)
			}
		}
	})
	t.Run("PlainText", func(t *testing.T) {
		s := newSanitizer(t)
		for _, tc := range []struct{ in, want string }{
			{"Alice", "Alice"},
			{"アリス :blobcat:", "アリス :blobcat:"},
			{"<b>Alice</b> <i>in</i> Wonderland", "Alice in Wonderland"},
			{"Tom &amp; Jerry &lt;3", "Tom & Jerry <3"},
			{"a < b > c", "a < b > c"},
			{`<script>alert(1)</script>Bob<style>*{}</style>`, "Bob"},
			{`<img src=x onerror=alert(1)>Carol`, "Carol"},
			{`<a href="javascript:alert(1)">Dave</a>`, "Dave"},
		} {
			if got := s.PlainText(tc.in); got != tc.want {
				t.Errorf("PlainText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})
}

type htmlSanitizerCase struct{ name, in, want string }

// linkAttrs are the rel and target every sanitized link carries.
const linkAttrs = ` rel="nofollow noopener noreferrer" target="_blank"`

var htmlSanitizerXSSCases = []htmlSanitizerCase{
	{"script", `<p>hi<script>alert(1)</script></p>`, `<p>hi</p>`},
	{"javascript link", `<a href="javascript:alert(1)">x</a>`, `x`},
	{"javascript link with case and space", `<a href="  JaVaScRiPt:alert(1)">x</a>`, `x`},
	{"javascript link with entities", `<a href="javascript&#58;alert(1)">x</a>`, `x`},
	{"javascript link with a tab", "<a href=\"java\tscript:alert(1)\">x</a>", `x`},
	{"data link", `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`, `x`},
	{"vbscript link", `<a href="vbscript:msgbox(1)">x</a>`, `x`},
	{"relative link", `<a href="/settings">x</a>`, `x`},
	{"event handler", `<p onclick="alert(1)">x</p><span onmouseover="alert(1)">y</span>`, `<p>x</p><span>y</span>`},
	{"image with an error handler", `<img src=x onerror=alert(1)>`, ``},
	{"style attribute", `<p style="background:url(javascript:alert(1))">x</p>`, `<p>x</p>`},
	{"style element", `<style>body{display:none}</style>x`, `x`},
	{"svg", `<svg onload="alert(1)"><script>alert(1)</script><text>t</text></svg>x`, `x`},
	{"svg link", `<svg><a href="javascript:alert(1)"><text>t</text></a></svg>x`, `x`},
	{"nested links", `<a href="https://a.example/">a<a href="https://b.example/">b</a></a>`,
		`<a href="https://a.example/"` + linkAttrs + `>a</a><a href="https://b.example/"` + linkAttrs + `>b</a>`},
	{"link with a disallowed link inside", `<a href="javascript:alert(1)"><span>a</span><a href="https://b.example/">b</a></a>`,
		`a<a href="https://b.example/"` + linkAttrs + `>b</a>`},
	{"iframe", `<iframe src="https://evil.example/"></iframe>x`, `x`},
	{"object and embed", `<object data="x.swf"></object><embed src="x.swf">x`, `x`},
	{"form", `<form action="https://evil.example/"><input name="p"><button>go</button></form>`, `<p>go</p>`},
	{"meta refresh", `<meta http-equiv="refresh" content="0;url=javascript:alert(1)">x`, `x`},
	{"base", `<base href="https://evil.example/">x`, `x`},
	{"comment", `<!--<script>alert(1)</script>-->x`, `x`},
	{"broken markup", `<p <script>alert(1)</script>>x`, `<p>alert(1)&gt;x</p>`},
	{"attribute break-out", `<a href="https://a.example/&quot;onclick=&quot;alert(1)">x</a>`,
		`<a href="https://a.example/%22onclick=%22alert%281%29"` + linkAttrs + `>x</a>`},
	{"target and rel from the source", `<a href="https://a.example/" target="_self" rel="opener">x</a>`,
		`<a href="https://a.example/"` + linkAttrs + `>x</a>`},
	{"escaped markup stays text", `<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>`, `<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>`},
}

var htmlSanitizerTransformCases = []htmlSanitizerCase{
	{"allowlisted elements", `<p>a<br><del>b</del><s>c</s><b>d</b><strong>e</strong><u>f</u><i>g</i><em>h</em><code>i</code></p>` +
		`<pre>j</pre><blockquote>k</blockquote><ul><li>l</li></ul><ruby>漢<rp>(</rp><rt>kan</rt><rp>)</rp></ruby>`,
		`<p>a<br><del>b</del><s>c</s><b>d</b><strong>e</strong><u>f</u><i>g</i><em>h</em><code>i</code></p>` +
			`<pre>j</pre><blockquote>k</blockquote><ul><li>l</li></ul><ruby>漢<rp>(</rp><rt>kan</rt><rp>)</rp></ruby>`},
	{"other inline elements are unwrapped", `<p><font color="red">a</font><sub>b</sub><abbr title="t">c</abbr><img alt="d" src="x.png"></p>`,
		`<p>abc</p>`},
	{"headings", `<h1>Title</h1><h6 lang="en">Sub</h6>`, `<p><strong>Title</strong></p><p><strong>Sub</strong></p>`},
	{"block elements", `<div>a</div><section>b</section><table><tr><td>c</td><td>d</td></tr></table>`,
		`<p>a</p><p>b</p><p>c</p><p>d</p>`},
	{"block elements around blocks", `<div>a<p>b</p>c<ul><li>d</li></ul></div>`, `<p>a</p><p>b</p><p>c</p><ul><li>d</li></ul>`},
	{"math with TeX", `<math><semantics><mrow><mi>x</mi></mrow><annotation encoding="application/x-tex">x^2</annotation></semantics></math>`, `$x^2$`},
	{"display math with TeX", `<math display="block"><semantics><mi>x</mi><annotation encoding="application/x-tex">\frac{1}{2}</annotation></semantics></math>`,
		`$$\frac{1}{2}$$`},
	{"math with plain text", `<math><semantics><mi>x</mi><annotation encoding="text/plain">x squared</annotation></semantics></math>`, `x squared`},
	{"math without annotation", `a<math><mi>x</mi></math>b`, `ab`},
	{"class", `<p class="quote-inline foo"><span class="h-card p-name bar">a</span><a href="https://a.example/" class="u-url mention hashtag ellipsis invisible baz">b</a></p>`,
		`<p class="quote-inline"><span class="h-card p-name">a</span><a href="https://a.example/" class="u-url mention hashtag ellipsis invisible"` + linkAttrs + `>b</a></p>`},
	{"class with nothing kept", `<span class="foo">a</span>`, `<span>a</span>`},
	{"translate", `<span translate="no">a</span><span translate="yes">b</span><a href="https://a.example/" translate="no">c</a>`,
		`<span translate="no">a</span><span>b</span><a href="https://a.example/" translate="no"` + linkAttrs + `>c</a>`},
	{"lang", `<p lang="ja">a</p><blockquote lang="en">b</blockquote>`, `<p lang="ja">a</p><blockquote lang="en">b</blockquote>`},
	{"list attributes", `<ol start="3" reversed type="a"><li value="7">a</li></ol><ol start="x"><li value="y">b</li></ol>`,
		`<ol start="3" reversed=""><li value="7">a</li></ol><ol><li>b</li></ol>`},
	{"link schemes", `<a href="gemini://a.example/">a</a><a href="ipfs://b">b</a><a href="xmpp:c@a.example">c</a><a href="mailto:d@a.example">d</a>`,
		`<a href="gemini://a.example/"` + linkAttrs + `>a</a><a href="ipfs://b"` + linkAttrs + `>b</a><a href="xmpp:c@a.example"` + linkAttrs + `>c</a>d`},
	{"link without href", `<a name="x">a</a>`, `a`},
	{"non-ASCII link", `<a href="https://a.example/tags/日本語">#日本語</a>`,
		`<a href="https://a.example/tags/%E6%97%A5%E6%9C%AC%E8%AA%9E"` + linkAttrs + `>#日本語</a>`},
	{"text escaping", `<p>Tom &amp; Jerry 'q' "d" &nbsp;é</p>`, "<p>Tom &amp; Jerry &#39;q&#39; &#34;d&#34;  é</p>"},
	{"empty", ``, ``},
}

// LocalStatusTexts are texts a local author may write; their HTML (content.ParseText) is
// allowlisted markup that a sanitizer returns unchanged.
var LocalStatusTexts = []string{
	"hello",
	"こんにちは、世界\n二行目\n\n二段落目",
	"hi @alice and @alice@social.example, see https://example.com/a/very/long/path/that/is/cut?q=1&r=<2>",
	"#Go #日本語 #ＧＯ @bob https://www.example.com/",
	`Tom & Jerry 'single' "double" <b>not bold</b> javascript:alert(1)`,
	"@ALICE\r\nhttps://example.com/(x)",
}

// LocalStatusMentions resolve the mentions of LocalStatusTexts.
var LocalStatusMentions = map[content.MentionHandle]content.MentionTarget{
	{Username: "alice"}:                           {Username: "Alice", URL: "https://social.example/@Alice"},
	{Username: "alice", Domain: "social.example"}: {Username: "Alice", URL: "https://social.example/@Alice"},
}

var (
	sanitizedTag  = regexp.MustCompile(`^<(/?)([a-z]+)((?: [a-z]+="[^"<>]*")*)>`)
	sanitizedAttr = regexp.MustCompile(` ([a-z]+)="([^"<>]*)"`)
	// sanitizedAttrs are the attributes each element may carry besides lang (federation.md §5).
	sanitizedAttrs = map[string][]string{
		"p": {"class"}, "br": nil, "span": {"class", "translate"}, "a": {"href", "class", "translate", "rel", "target"},
		"del": nil, "s": nil, "pre": nil, "blockquote": nil, "code": nil, "b": nil, "strong": nil, "u": nil, "i": nil,
		"em": nil, "ul": nil, "ol": {"start", "reversed"}, "li": {"value"}, "ruby": nil, "rt": nil, "rp": nil,
	}
	sanitizedClass   = regexp.MustCompile(`^(?:(?:h|p|u|dt|e)-\S*|mention|hashtag|ellipsis|invisible|quote-inline)$`)
	sanitizedSchemes = []string{"http", "https", "dat", "dweb", "ipfs", "ipns", "ssb", "gopher", "xmpp", "magnet", "gemini"}
)

// CheckSanitizedHTML fails unless every tag of h is an allowlisted element with allowlisted
// attributes and values (federation.md §5), links have an allowlisted scheme and do not nest,
// and no "<" or ">" appears outside a tag.
func CheckSanitizedHTML(t *testing.T, h string) {
	t.Helper()
	inLink := false
	for i := 0; i < len(h); {
		switch h[i] {
		case '>':
			t.Fatalf("stray > at %d in %s", i, h)
		case '<':
			m := sanitizedTag.FindStringSubmatch(h[i:])
			if m == nil {
				t.Fatalf("stray < at %d in %s", i, h)
			}
			closing, name := m[1] == "/", m[2]
			allowed, ok := sanitizedAttrs[name]
			if !ok {
				t.Fatalf("element <%s> is not allowlisted in %s", name, h)
			}
			if name == "a" {
				if !closing && inLink {
					t.Fatalf("nested link in %s", h)
				}
				inLink = !closing
			}
			for _, a := range sanitizedAttr.FindAllStringSubmatch(m[3], -1) {
				checkSanitizedAttr(t, h, name, allowed, a[1], a[2])
			}
			i += len(m[0])
		default:
			i++
		}
	}
}

func checkSanitizedAttr(t *testing.T, h, element string, allowed []string, name, value string) {
	t.Helper()
	if name != "lang" && !slices.Contains(allowed, name) {
		t.Fatalf("attribute %s of <%s> is not allowlisted in %s", name, element, h)
	}
	switch name {
	case "class":
		for _, c := range strings.Split(value, " ") {
			if !sanitizedClass.MatchString(c) {
				t.Fatalf("class %q is not allowlisted in %s", c, h)
			}
		}
	case "href":
		scheme, _, ok := strings.Cut(value, ":")
		if !ok || !slices.Contains(sanitizedSchemes, scheme) {
			t.Fatalf("href %q has no allowlisted scheme in %s", value, h)
		}
	case "rel":
		if value != "nofollow noopener noreferrer" {
			t.Fatalf("rel %q in %s", value, h)
		}
	case "target":
		if value != "_blank" {
			t.Fatalf("target %q in %s", value, h)
		}
	case "translate":
		if value != "no" {
			t.Fatalf("translate %q in %s", value, h)
		}
	}
}
