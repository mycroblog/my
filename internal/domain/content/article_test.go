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

// TestJoinArticleText checks the separators between the blocks of an article's text (ADR-0160 §6).
func TestJoinArticleText(t *testing.T) {
	p := func(s string) content.ArticleTextBlock { return content.ArticleTextBlock{Text: s} }
	i := func(s string) content.ArticleTextBlock { return content.ArticleTextBlock{Text: s, Item: true} }
	for _, tc := range []struct {
		name   string
		blocks []content.ArticleTextBlock
		want   string
	}{
		{"nothing", nil, ""},
		{"one block", []content.ArticleTextBlock{p("only")}, "only"},
		{"en paragraphs", []content.ArticleTextBlock{p("Mix them"), p("Bake it")}, "Mix them. Bake it"},
		{"ja paragraphs", []content.ArticleTextBlock{p("混ぜる"), p("焼く")}, "混ぜる。焼く"},
		{"en items", []content.ArticleTextBlock{i("eggs"), i("milk"), i("sugar")}, "eggs, milk, sugar"},
		{"ja items after Han", []content.ArticleTextBlock{i("卵"), i("牛乳"), i("砂糖")}, "卵、牛乳、砂糖"},
		{"ja items after Hiragana", []content.ArticleTextBlock{i("たまご"), i("みず")}, "たまご、みず"},
		{"ja items after Katakana", []content.ArticleTextBlock{i("パン"), i("バター")}, "パン、バター"},
		{"the script of the block before", []content.ArticleTextBlock{i("卵"), i("milk"), i("卵")}, "卵、milk, 卵"},
		{"items then a paragraph", []content.ArticleTextBlock{i("eggs"), i("milk"), p("Mix them")}, "eggs, milk. Mix them"},
		{"a paragraph then items", []content.ArticleTextBlock{p("混ぜる"), i("卵"), i("牛乳")}, "混ぜる。卵、牛乳"},
		{"Japanese paragraph then English", []content.ArticleTextBlock{p("日本語"), p("English")}, "日本語。English"},
		{"ends in a number", []content.ArticleTextBlock{p("Version 2"), p("Next")}, "Version 2. Next"},
		{"half-width punctuation", []content.ArticleTextBlock{p("Bake."), p("Done?"), p("Yes!"), p("Ingredients:"), i("a,"), i("b")}, "Bake. Done? Yes! Ingredients: a, b"},
		{"half-width colon in Japanese", []content.ArticleTextBlock{p("材料:"), i("卵"), i("牛乳")}, "材料: 卵、牛乳"},
		{"full-width punctuation", []content.ArticleTextBlock{p("焼く。"), p("本当？"), p("はい！"), i("卵、"), i("牛乳")}, "焼く。本当？はい！卵、牛乳"},
		{"trimmed", []content.ArticleTextBlock{p("  a  "), p("\tb\n")}, "a. b"},
		{"empty blocks dropped", []content.ArticleTextBlock{p("a"), p(""), p("  "), i(""), p("b")}, "a. b"},
		{"empty items dropped", []content.ArticleTextBlock{i("a"), i(" "), i("b")}, "a, b"},
		{"nothing but empty blocks", []content.ArticleTextBlock{p(""), i(" ")}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := content.JoinArticleText(tc.blocks); got != tc.want {
				t.Errorf("JoinArticleText = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestArticleSummaryFromItems checks that a summary cut in a list keeps no separator before its
// ellipsis (ADR-0160 §6).
func TestArticleSummaryFromItems(t *testing.T) {
	items := func(text string, n int) []content.ArticleTextBlock {
		blocks := make([]content.ArticleTextBlock, n)
		for k := range blocks {
			blocks[k] = content.ArticleTextBlock{Text: text, Item: true}
		}
		return blocks
	}
	for _, tc := range []struct {
		name   string
		blocks []content.ArticleTextBlock
		want   string
	}{
		{"en", items("word", 60), strings.Repeat("word, ", 32) + "word…"},
		{"ja", items("卵", 150), strings.Repeat("卵、", 99) + "卵…"},
		{"short list stays", items("卵", 3), "卵、卵、卵"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := content.ArticleSummaryFromBody(content.JoinArticleText(tc.blocks))
			if got != tc.want {
				t.Errorf("ArticleSummaryFromBody = %q, want %q", got, tc.want)
			}
			for _, sep := range []string{",…", "、…", " …"} {
				if strings.HasSuffix(got, sep) {
					t.Errorf("the summary %q ends in %q", got, sep)
				}
			}
			if n := utf8.RuneCountInString(got); n > content.ArticleSummaryLength+1 {
				t.Errorf("the summary has %d characters", n)
			}
		})
	}
}
