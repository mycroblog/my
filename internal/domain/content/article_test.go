// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/domain/content"
)

func TestArticleSummaryFromBody(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"short text stays", "  A short\n body.  ", "A short body."},
		{"cut at a word", strings.Repeat("word ", 60), strings.TrimSpace(strings.Repeat("word ", 40)) + "…"},
		{"cut at a sentence", strings.Repeat("あ", 150) + "。" + strings.Repeat("い", 100), strings.Repeat("あ", 150) + "。…"},
		{"cut at the length", strings.Repeat("x", 250), strings.Repeat("x", 200) + "…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := content.ArticleSummaryFromBody(tc.in)
			if got != tc.want {
				t.Errorf("ArticleSummaryFromBody = %q, want %q", got, tc.want)
			}
			if n := utf8.RuneCountInString(got); n > content.ArticleSummaryLength+1 {
				t.Errorf("the summary has %d characters", n)
			}
		})
	}
}

func TestArticleVisibility(t *testing.T) {
	for _, v := range []content.Visibility{content.VisibilityPublic, content.VisibilityLocal, content.VisibilityUnlisted, content.VisibilityPrivate, content.VisibilityDirect} {
		if got, want := content.ArticleVisibility(v), v != content.VisibilityDirect; got != want {
			t.Errorf("ArticleVisibility(%s) = %v", v, got)
		}
	}
	if content.ArticleVisibility("everyone") {
		t.Error("ArticleVisibility accepts a visibility that is none")
	}
}
