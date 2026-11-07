// SPDX-License-Identifier: AGPL-3.0-only

package google

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mycroblog/my/internal/app/translation"
)

func TestToGoogle(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]string{
		"en": "en", "en-US": "en", "ja-JP": "ja", "pt-BR": "pt", "pt-PT": "pt-PT", "fr-CA": "fr-CA", "iw": "he",
		"zh": "zh-CN", "zh-Hans": "zh-CN", "zh-CN": "zh-CN", "zh-SG": "zh-CN", "zh-Hant": "zh-TW", "zh-TW": "zh-TW",
		"zh-HK": "zh-TW", "zh-Hans-HK": "zh-CN", "mni-Mtei": "mni-Mtei", "und": "", "": "", "not a tag": "",
	} {
		if got := toGoogle(tag); got != want {
			t.Errorf("toGoogle(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestFromGoogle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ code, source, want string }{
		{"fr", "und", "fr"},
		{"", "en", "en"},
		{"", "und", "und"},
		{"und", "en", "en"},
		{"iw", "und", "he"},
		{"zh-CN", "und", "zh-Hans"},
		{"zh-TW", "und", "zh-Hant"},
		{"<b>fr</b>", "und", "und"},
	} {
		if got := fromGoogle(tc.code, tc.source); got != tc.want {
			t.Errorf("fromGoogle(%q, %q) = %q, want %q", tc.code, tc.source, got, tc.want)
		}
	}
}

func TestMapError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		want error
	}{
		{status.Error(codes.ResourceExhausted, "Quota exceeded"), translation.ErrProviderLimit},
		{status.Error(codes.InvalidArgument, "Target language is invalid."), translation.ErrUnsupportedLanguage},
		{status.Error(codes.InvalidArgument, "Unsupported language pair."), translation.ErrUnsupportedLanguage},
		{status.Error(codes.InvalidArgument, "Too many contents."), translation.ErrTemporary},
		{status.Error(codes.Unavailable, "down"), translation.ErrTemporary},
		{status.Error(codes.DeadlineExceeded, "slow"), translation.ErrTemporary},
		{context.DeadlineExceeded, translation.ErrTemporary},
		{errors.New("dial failed"), translation.ErrTemporary},
	} {
		got := mapError(context.Background(), tc.err)
		for _, k := range []error{translation.ErrTemporary, translation.ErrProviderLimit, translation.ErrUnsupportedLanguage} {
			if errors.Is(got, k) != errors.Is(tc.want, k) {
				t.Errorf("mapError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		}
	}
}

// The markers survive what Google may do to the HTML: move the spans, add space, self-close an empty one, add markup of
// its own, or leave a marker as text when it drops a span.
func TestMarkupRoundTrip(t *testing.T) {
	t.Parallel()
	const fragment = "Hi ⟦1⟧, see ⟦2⟧this <b> & that⟦/2⟧\n⟦3⟧"
	out := toHTML(fragment)
	want := `Hi <span translate="no" class="notranslate" data-m="1">⟦1⟧</span>, see <span data-m="2">this &lt;b&gt; &amp; that</span><br><span translate="no" class="notranslate" data-m="3">⟦3⟧</span>`
	if out != want {
		t.Fatalf("toHTML() = %q, want %q", out, want)
	}
	if got := fromHTML(fragment, out); got != fragment {
		t.Errorf("fromHTML(toHTML()) = %q", got)
	}
	for answer, want := range map[string]string{
		`<span data-m="2"> ceci &lt;b&gt; </span> voir <span class="notranslate" translate="no" data-m="1">⟦1⟧</span> <br/> <span data-m="3"/>`: "⟦2⟧ ceci <b> ⟦/2⟧ voir ⟦1⟧ \n ⟦3⟧",
		`<font><span data-m="2">x</span></font> ⟦1⟧ ⟦3⟧`:                                                                                        "⟦2⟧x⟦/2⟧ ⟦1⟧ ⟦3⟧",
		`<span data-m="1">٣</span><span data-m="2">x<span>y</span></span><span data-m="3"></span>`:                                              "⟦1⟧⟦2⟧xy⟦/2⟧⟦3⟧",
	} {
		got := fromHTML(fragment, answer)
		if got != want {
			t.Errorf("fromHTML(%q) = %q, want %q", answer, got, want)
		}
		if !translation.MarkersIntact(fragment, got) {
			t.Errorf("fromHTML(%q) = %q, markers not intact", answer, got)
		}
	}
}

func TestBatches(t *testing.T) {
	t.Parallel()
	long := string(make([]rune, maxCodePoints))
	got := batches([]string{"a", long, "b", "c"})
	if len(got) != 3 || len(got[0]) != 1 || len(got[1]) != 1 || len(got[2]) != 2 {
		t.Errorf("batches = %d of %v", len(got), got)
	}
}
