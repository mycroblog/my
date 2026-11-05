// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"slices"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

func bodyFragments(t *testing.T, in string) (*maskedBody, []string) {
	t.Helper()
	b, err := parseBody(in)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range b.fragments() {
		out = append(out, f.fragment)
	}
	return b, out
}

func TestFragmentsOfABody(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   string
		want []string
	}{
		{"nothing", "", nil},
		{"one paragraph per fragment", "<p>one</p><p>two</p>", []string{"one", "two"}},
		{"a line break stays in the fragment", "<p>one<br>two</p>", []string{"one⟦1⟧two"}},
		{"list items", "<ul><li>one</li><li>two</li></ul>", []string{"one", "two"}},
		{"text around a nested list", "<ul><li>one<ul><li>two</li></ul>three</li></ul>", []string{"one", "two", "three"}},
		{"a quote", "<blockquote><p>one</p></blockquote><p>two</p>", []string{"one", "two"}},
		{"text outside a block", "one<p>two</p>", []string{"one", "two"}},
		{"decoration is a pair", "<p>a <b>big</b> <em>cat</em></p>", []string{"a ⟦1⟧big⟦/1⟧ ⟦2⟧cat⟦/2⟧"}},
		{"nested decoration", "<p><b>a <i>b</i></b></p>", []string{"⟦1⟧a ⟦2⟧b⟦/2⟧⟦/1⟧"}},
		{"a link is one item", `<p>see <a href="https://example.com/secret">the secret</a></p>`, []string{"see ⟦1⟧"}},
		{
			"a mention, in its span, is one item",
			`<p>hi <span class="h-card"><a href="https://example.com/@bob" class="u-url mention">@<span>bob</span></a></span></p>`,
			[]string{"hi ⟦1⟧"},
		},
		{"code stays", "<p>run <code>rm -rf</code> now</p>", []string{"run ⟦1⟧ now"}},
		{"pre is no fragment", "<p>run</p><pre>rm -rf</pre>", []string{"run"}},
		{"ruby is one item", "<p><ruby>漢<rt>かん</rt></ruby>字</p>", []string{"⟦1⟧字"}},
		{"a shortcode", "<p>nice :blob_cat: one</p>", []string{"nice ⟦1⟧ one"}},
		{"a handle as plain text", "<p>hi @bob@example.com there</p>", []string{"hi ⟦1⟧ there"}},
		{"a URL as plain text", "<p>see https://example.com/a now</p>", []string{"see ⟦1⟧ now"}},
		{"a hashtag as plain text", "<p>about #cats here</p>", []string{"about ⟦1⟧ here"}},
		{"text shaped like a marker", "<p>literally ⟦1⟧ and ⟦/2⟧</p>", []string{"literally ⟦1⟧ and ⟦2⟧"}},
		{"markers of the post do not take a number twice", "<p><b>a</b> ⟦1⟧</p>", []string{"⟦1⟧a⟦/1⟧ ⟦2⟧"}},
		{"look-alikes stay text", "<p>⟦a⟧ ⟦0⟧</p>", []string{"⟦a⟧ ⟦0⟧"}},
		{"a marker shape across a comment", "<p>⟦1<!-- -->⟧</p>", []string{"⟦1⟧"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, got := bodyFragments(t, tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("fragments of %q = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRenderRestoresTheBody checks that a fragment comes back as what it stood for: an untranslated
// post is the post, whatever its form.
func TestRenderRestoresTheBody(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"<p>one</p><p>two</p>",
		"<p>one<br>two</p>",
		`<p>see <a href="https://example.com/a?b=1&amp;c=2" rel="nofollow">the &lt;link&gt;</a> &amp; more</p>`,
		"<ul><li>one<ul><li>two</li></ul>three</li></ul>",
		"<p>a <b>big</b> <em>c<u>a</u>t</em> :blob_cat:</p>",
		"<p>run</p><pre>rm -rf ⟦1⟧ &lt;x&gt;</pre>",
		"<p>literally ⟦1⟧ and <code>⟦2⟧</code> and #cats</p>",
		`<p lang="ja"><span class="invisible">x</span>y</p>`,
	} {
		b, _ := bodyFragments(t, in)
		if got := strings.ReplaceAll(b.render(), "<br/>", "<br>"); got != in {
			t.Errorf("render of %q = %q", in, got)
		}
	}
}

func TestRenderEscapesWhatAProviderReturns(t *testing.T) {
	t.Parallel()
	b, got := bodyFragments(t, "<p>hello <b>there</b></p>")
	if len(got) != 1 {
		t.Fatalf("fragments = %q", got)
	}
	b.fragments()[0].translated = `<script>alert(1)</script> ⟦1⟧<img src=x>⟦/1⟧ & co`
	want := `<p>&lt;script&gt;alert(1)&lt;/script&gt; <b>&lt;img src=x&gt;</b> &amp; co</p>`
	if out := b.render(); out != want {
		t.Errorf("render = %q, want %q", out, want)
	}
}

func TestMaskPlain(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, in, fragment string
		sendable           bool
	}{
		{"empty", "", "", false},
		{"words", "  a warning ", "  a warning ", true},
		{"markup is text", "<b>x</b>", "<b>x</b>", true},
		{"a handle and a shortcode", "@bob@example.com is :blob_cat: sad", "⟦1⟧ is ⟦2⟧ sad", true},
		{"a literal marker", "⟦7⟧ x", "⟦1⟧ x", true},
		{"nothing but items", ":blob_cat: https://example.com/", "⟦1⟧ ⟦2⟧", false},
		{"numbers and punctuation", "1, 2!", "1, 2!", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := maskPlain(tt.in)
			if got.fragment != tt.fragment || got.sendable() != tt.sendable {
				t.Errorf("maskPlain(%q) = %q (sendable %t), want %q (sendable %t)", tt.in, got.fragment, got.sendable(), tt.fragment, tt.sendable)
			}
			if restored := got.restore(func(s string) string { return s }); restored != tt.in {
				t.Errorf("restore = %q, want %q", restored, tt.in)
			}
		})
	}
}

func TestBodyInLanguage(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		bodies []string
		target string
		want   string
	}{
		{"the same tag", []string{"en", "ja"}, "ja", "ja"},
		{"the tag first, then the primary language", []string{"en-GB", "en-US"}, "en-US", "en-US"},
		{"the primary language of the body", []string{"en"}, "en-US", "en"},
		{"the primary language of the target", []string{"en-US"}, "en", "en-US"},
		{"another language", []string{"en"}, "ja", ""},
		{"scripts never mix", []string{"zh-Hant"}, "zh-Hans", ""},
		{"a body with a script is no match for no script", []string{"zh-Hant"}, "zh", ""},
		{"a target with a script is no match for no script", []string{"zh"}, "zh-Hant", ""},
		{"the same script", []string{"zh-Hant"}, "zh-Hant", "zh-Hant"},
		{"a region is no script", []string{"zh-TW"}, "zh", "zh-TW"},
		{"und is no language", []string{"und"}, "ja", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var bodies []appcontent.Body
			for _, l := range tt.bodies {
				bodies = append(bodies, appcontent.Body{Language: l})
			}
			got, ok := bodyInLanguage(bodies, tt.target)
			if ok != (tt.want != "") || got.Language != tt.want {
				t.Errorf("bodyInLanguage(%v, %q) = %q, %t; want %q", tt.bodies, tt.target, got.Language, ok, tt.want)
			}
		})
	}
}
