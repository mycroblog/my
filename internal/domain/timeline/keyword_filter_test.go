// SPDX-License-Identifier: AGPL-3.0-only

package timeline_test

import (
	"reflect"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/domain/timeline"
)

// identity stands for NFKC: the application layer gives the real one, which its tests cover
// with the half-width and full-width forms.
func identity(s string) string { return s }

func runes(s string) int { return utf8.RuneCountInString(s) }

func TestNormalizeFilterText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello World", "hello world"},
		{"ÉCOLE", "école"},
		{"カタカナ", "かたかな"},
		{"ヴァヽヾ", "ゔぁゝゞ"},
		{"ひらがな", "ひらがな"},
		{"a \n\t b", "a b"},
		{"  lead", " lead"},
		{"漢字ー", "漢字ー"},
	}
	for _, tc := range cases {
		if got := timeline.NormalizeFilterText(tc.in, identity); got != tc.want {
			t.Errorf("NormalizeFilterText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTextOfHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{`<p>Hello <a href="https://spoiler.example/">link</a></p><p>world</p>`, " Hello link  world "},
		{`<p>a<br>b<br/>c</p>`, " a b c "},
		{`<span class="h-card"><a href="https://x.example/@u" class="u-url mention">@<span>user</span></a></span>`, "@user"},
		{`Tom &amp; Jerry &lt;3`, "Tom & Jerry <3"},
		{`<p>cut <b`, " cut "},
	}
	for _, tc := range cases {
		if got := timeline.TextOfHTML(tc.in); got != tc.want {
			t.Errorf("TextOfHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNewFilterWord(t *testing.T) {
	w, err := timeline.NewFilterWord("  Spoiler Alert ", true, identity, runes)
	if err != nil {
		t.Fatal(err)
	}
	if want := (timeline.FilterWord{Word: "Spoiler Alert", Normalized: "spoiler alert", WholeWord: true}); w != want {
		t.Errorf("NewFilterWord = %+v, want %+v", w, want)
	}
	long := ""
	for range timeline.MaxKeywordFilterWordGraphemes {
		long += "a"
	}
	if _, err := timeline.NewFilterWord(long, false, identity, runes); err != nil {
		t.Errorf("a word of %d graphemes: %v", timeline.MaxKeywordFilterWordGraphemes, err)
	}
	for _, bad := range []string{"", "   ", "a\x00b", long + "a", "\xff"} {
		if _, err := timeline.NewFilterWord(bad, false, identity, runes); err != timeline.ErrInvalidFilterWord {
			t.Errorf("NewFilterWord(%q) = %v, want ErrInvalidFilterWord", bad, err)
		}
	}
}

func TestValidFilterTitle(t *testing.T) {
	for _, ok := range []string{"Spoilers", "ネタバレ"} {
		if !timeline.ValidFilterTitle(ok, runes) {
			t.Errorf("ValidFilterTitle(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", " x", "x ", "a\nb", string(make([]rune, 51))} {
		if timeline.ValidFilterTitle(bad, runes) {
			t.Errorf("ValidFilterTitle(%q) = true", bad)
		}
	}
}

func filterOf(id string, words ...timeline.FilterWord) timeline.KeywordFilter {
	return timeline.KeywordFilter{ID: id, Title: "title " + id, Action: timeline.FilterWarn, Words: words}
}

func word(t *testing.T, w string, whole bool) timeline.FilterWord {
	t.Helper()
	fw, err := timeline.NewFilterWord(w, whole, identity, runes)
	if err != nil {
		t.Fatal(err)
	}
	return fw
}

func TestFilterMatcher(t *testing.T) {
	cases := []struct {
		name   string
		word   string
		whole  bool
		fields []string
		match  bool
	}{
		// en: substrings by default, whole words on request.
		{"en substring", "cat", false, []string{"Concatenate"}, true},
		{"en case", "CAT", false, []string{"a cat"}, true},
		{"en whole word", "cat", true, []string{"a cat."}, true},
		{"en whole word at the edges", "cat", true, []string{"cat"}, true},
		{"en whole word inside a word", "cat", true, []string{"concatenate"}, false},
		{"en whole word before a digit", "cat", true, []string{"cat9"}, false},
		{"en whole word after _", "cat", true, []string{"my_cat"}, false},
		{"en whole word accented", "cafe", true, []string{"cafes"}, false},
		{"en whole word non-ASCII neighbour", "caf", true, []string{"café"}, false},
		{"en phrase", "spoiler alert", false, []string{"Big\nSPOILER   alert!"}, true},
		{"en phrase not across fields", "spoiler alert", false, []string{"spoiler", "alert"}, false},
		{"en whole word with punctuation edge", "#tag", true, []string{"x#tags"}, false},
		{"en punctuation edge of the word", "#tag", true, []string{"x#tag y"}, true},
		{"en no match", "dog", false, []string{"a cat"}, false},
		// ja: no spaces between words, so whole words act as substrings.
		{"ja substring", "猫", false, []string{"子猫がかわいい"}, true},
		{"ja whole word without spaces", "猫", true, []string{"子猫がかわいい"}, true},
		{"ja katakana matches hiragana", "ネタバレ", false, []string{"これはねたばれです"}, true},
		{"ja hiragana matches katakana", "ねたばれ", true, []string{"【ネタバレ注意】"}, true},
		{"ja prolonged sound mark", "ゲーム", true, []string{"新作ゲームー"}, true},
		{"ja inside latin", "ネタ", true, []string{"abcネタdef"}, true},
		{"ja latin word next to kana", "neta", true, []string{"これはnetaです"}, true},
		{"ja no match", "犬", false, []string{"子猫"}, false},
		{"th whole word", "แมว", true, []string{"ลูกแมวน่ารัก"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := timeline.NewFilterMatcher([]timeline.KeywordFilter{filterOf("1", word(t, tc.word, tc.whole))}, identity)
			got := m.Match(tc.fields...)
			if (len(got) > 0) != tc.match {
				t.Errorf("Match(%q) = %+v, want match %v", tc.fields, got, tc.match)
			}
		})
	}
}

func TestFilterMatcherManyWords(t *testing.T) {
	a := filterOf("a", word(t, "he", false), word(t, "she", false), word(t, "hers", false))
	b := filterOf("b", word(t, "his", false), word(t, "s", true))
	c := filterOf("c", word(t, "nothing", false))
	m := timeline.NewFilterMatcher([]timeline.KeywordFilter{a, b, c}, identity)
	got := m.Match("ushers", "", "s")
	want := []timeline.FilterMatch{
		{Filter: a, Words: []string{"he", "she", "hers"}},
		{Filter: b, Words: []string{"s"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %+v, want %+v", got, want)
	}
	if got := m.Match("plain text"); got != nil {
		t.Errorf("Match(plain text) = %+v, want nil", got)
	}
}

func TestFilterMatcherNil(t *testing.T) {
	m := timeline.NewFilterMatcher([]timeline.KeywordFilter{filterOf("1")}, identity)
	if m != nil {
		t.Fatalf("NewFilterMatcher without words = %v, want nil", m)
	}
	if got := m.Match("anything"); got != nil {
		t.Errorf("nil Match = %v", got)
	}
}

func TestKeywordFilterActiveAt(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	f := timeline.KeywordFilter{}
	if !f.ActiveAt(now) {
		t.Error("a filter without expiry is not active")
	}
	f.ExpiresAt = now
	if f.ActiveAt(now) {
		t.Error("a filter is active at its expiry")
	}
	f.ExpiresAt = now.Add(time.Second)
	if !f.ActiveAt(now) {
		t.Error("a filter is not active before its expiry")
	}
}

func TestFilterContextValid(t *testing.T) {
	for _, c := range timeline.FilterContexts() {
		if !c.Valid() {
			t.Errorf("%q is not valid", c)
		}
	}
	if timeline.FilterContext("bookmarks").Valid() {
		t.Error("bookmarks is valid")
	}
	if !timeline.FilterHide.Valid() || timeline.FilterAction("blur").Valid() {
		t.Error("FilterAction.Valid")
	}
}
