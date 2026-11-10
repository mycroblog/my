// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func TestPlainText(t *testing.T) {
	tests := []struct{ in, want string }{
		{`<p>Hello <a href="https://example.org/@bob" class="u-url mention">@<span>bob</span></a>!</p>`, "Hello @bob!"},
		{`<p>one<br>two<br />three</p><p>four</p>`, "one two three four"},
		{`<p>&lt;script&gt;alert(1)&lt;/script&gt; &amp; &quot;q&quot; &#39;s&#39;</p>`, `<script>alert(1)</script> & "q" 's'`},
		{"<p>  spaced\n\tout  </p>", "spaced out"},
		{`<p>a<b`, "a"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := plainText(tt.in); got != tt.want {
			t.Errorf("plainText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDescriptionKeepsGraphemes(t *testing.T) {
	if got := description("short text"); got != "short text" {
		t.Errorf("description(short) = %q", got)
	}
	exact := strings.Repeat("a", descriptionLength)
	if got := description(exact); got != exact {
		t.Errorf("a text of exactly %d characters was truncated: %q", descriptionLength, got)
	}
	// Family emoji (ZWJ sequences), a flag (regional indicators), a combining accent, and a
	// skin tone modifier: each is one grapheme cluster of several code points.
	for _, unit := range []string{"👨‍👩‍👧‍👦", "🇯🇵", "é", "👍🏽", "あ"} {
		got := description(strings.Repeat(unit, descriptionLength+10))
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("%q: not truncated: %q", unit, got)
		}
		if n := uniseg.GraphemeClusterCount(got); n != descriptionLength {
			t.Errorf("%q: %d grapheme clusters, want %d", unit, n, descriptionLength)
		}
		if body := strings.TrimSuffix(got, "…"); strings.Repeat(unit, strings.Count(body, unit)) != body {
			t.Errorf("%q: a cluster was cut: %q", unit, got)
		}
	}
}

func TestCard(t *testing.T) {
	tests := []struct {
		img  *Image
		want string
	}{
		{nil, cardSummary},
		{&Image{URL: "u", Width: 1200, Height: 600}, cardSummaryLarge},
		{&Image{URL: "u", Width: 1920, Height: 1080}, cardSummaryLarge},
		{&Image{URL: "u", Width: 1500, Height: 500}, cardSummaryLarge},
		{&Image{URL: "u", Width: 1200, Height: 900}, cardSummary},
		{&Image{URL: "u", Width: 400, Height: 400}, cardSummary},
		{&Image{URL: "u", Width: 280, Height: 140}, cardSummary},
		{&Image{URL: "u"}, cardSummary},
	}
	for _, tt := range tests {
		if got := card(tt.img); got != tt.want {
			t.Errorf("card(%+v) = %q, want %q", tt.img, got, tt.want)
		}
	}
}

func TestOGLocale(t *testing.T) {
	for in, want := range map[string]string{
		"ja": "ja_JP", "en": "en_US", "en-US": "en_US", "en-GB": "en_GB", "zh-Hant-TW": "zh_TW", "zh-Hant": "zh_TW",
		"zh-Hans": "zh_CN", "pt_br": "pt_BR", "pt": "pt_BR", "de": "de_DE", "es": "es_ES", "fr": "fr_FR", "ar": "ar_EG",
		"en-XA": "en", "ar-XB": "ar", "eo": "eo", "und": "und",
	} {
		if got := ogLocale(in); got != want {
			t.Errorf("ogLocale(%q) = %q, want %q", in, got, want)
		}
	}
}
