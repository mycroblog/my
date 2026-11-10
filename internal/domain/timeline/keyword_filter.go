// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"errors"
	"html"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Keyword filters (FR-209, ADR-0157): an account registers words, and the posts whose text, content
// warning, poll choices, or alt text match one of them are folded (FilterWarn) or left out
// (FilterHide) on the contexts the filter names. The server matches them, so every client gets
// the same result.

// FilterContext is where a keyword filter applies (ADR-0157 §6).
type FilterContext string

// The contexts of a keyword filter.
const (
	// FilterContextHome is the home timeline and the timelines of lists.
	FilterContextHome FilterContext = "home"
	// FilterContextNotifications is the notifications: their list, their stream, Web Push, and
	// email.
	FilterContextNotifications FilterContext = "notifications"
	// FilterContextPublic is the local, federated, and hashtag timelines and their streams.
	FilterContextPublic FilterContext = "public"
	// FilterContextThread is a thread: the statuses before and after one.
	FilterContextThread FilterContext = "thread"
	// FilterContextAccount is the profile timeline and the pinned statuses.
	FilterContextAccount FilterContext = "account"
)

// FilterContexts lists every context of a keyword filter, in the order the API shows them.
func FilterContexts() []FilterContext {
	return []FilterContext{
		FilterContextHome, FilterContextNotifications, FilterContextPublic, FilterContextThread, FilterContextAccount,
	}
}

// Valid reports whether c is a known context.
func (c FilterContext) Valid() bool { return slices.Contains(FilterContexts(), c) }

// FilterAction is what a keyword filter does to a status that matches it.
type FilterAction string

// The actions of a keyword filter.
const (
	// FilterWarn marks the status as filtered, and clients fold it behind the filter's title.
	FilterWarn FilterAction = "warn"
	// FilterHide leaves the status out, as if the viewer could not see it.
	FilterHide FilterAction = "hide"
)

// Valid reports whether a is a known action.
func (a FilterAction) Valid() bool { return a == FilterWarn || a == FilterHide }

// The limits of keyword filters (ADR-0157 §9). The total of words bounds the matcher that a read
// builds and the time it takes to scan a page; they are constants, not settings.
const (
	// MaxKeywordFilters is the most filters an account has.
	MaxKeywordFilters = 100
	// MaxKeywordFilterWords is the most words a filter has.
	MaxKeywordFilterWords = 100
	// MaxKeywordFilterWordsTotal is the most words of all the filters of an account.
	MaxKeywordFilterWordsTotal = 1000
	// MaxKeywordFilterTitleGraphemes is the longest title of a filter, in grapheme clusters.
	MaxKeywordFilterTitleGraphemes = 50
	// MaxKeywordFilterWordGraphemes is the longest word, normalized, in grapheme clusters.
	MaxKeywordFilterWordGraphemes = 100
	// maxKeywordFilterLength bounds the code points of a title or word, whatever its grapheme
	// clusters.
	maxKeywordFilterLength = 400
)

// Errors of the keyword filter rules.
var (
	// ErrInvalidFilterTitle reports a title that ValidFilterTitle rejects.
	ErrInvalidFilterTitle = errors.New("timeline: invalid keyword filter title")
	// ErrInvalidFilterWord reports a word that NewFilterWord rejects.
	ErrInvalidFilterWord = errors.New("timeline: invalid keyword filter word")
	// ErrInvalidFilterContext reports a filter without contexts or with an unknown one.
	ErrInvalidFilterContext = errors.New("timeline: invalid keyword filter context")
	// ErrInvalidFilterAction reports an unknown action.
	ErrInvalidFilterAction = errors.New("timeline: invalid keyword filter action")
	// ErrNoFilterWords reports a filter without words.
	ErrNoFilterWords = errors.New("timeline: a keyword filter needs a word")
	// ErrTooManyKeywordFilters reports a filter beyond MaxKeywordFilters.
	ErrTooManyKeywordFilters = errors.New("timeline: too many keyword filters")
	// ErrTooManyFilterWords reports the words of a filter beyond MaxKeywordFilterWords, or of an
	// account beyond MaxKeywordFilterWordsTotal.
	ErrTooManyFilterWords = errors.New("timeline: too many keyword filter words")
)

// KeywordFilter is a keyword filter of an account (keyword_filters with its current settings,
// contexts, and words).
type KeywordFilter struct {
	ID        string
	AccountID string
	Title     string
	Action    FilterAction
	// Contexts are where the filter applies, at least one, sorted.
	Contexts []FilterContext
	// Words are the filter's words, in the order they were given; a status matches the filter
	// when it matches any of them.
	Words []FilterWord
	// ExpiresAt is when the filter stops applying, zero for never. An expired filter is kept and
	// applies again when its expiry is moved later.
	ExpiresAt time.Time
	CreatedAt time.Time
}

// ActiveAt reports whether the filter applies at now: it has no expiry or expires after now.
func (f KeywordFilter) ActiveAt(now time.Time) bool {
	return f.ExpiresAt.IsZero() || f.ExpiresAt.After(now)
}

// AppliesTo reports whether the filter names the context c.
func (f KeywordFilter) AppliesTo(c FilterContext) bool { return slices.Contains(f.Contexts, c) }

// FilterWord is a word of a keyword filter: what the user typed, its normalized form that it is
// matched by, and whether it matches whole words only.
type FilterWord struct {
	Word       string
	Normalized string
	WholeWord  bool
}

// NFKC returns the Unicode normalization form KC of s. The application layer gives one; this
// package uses only the standard library.
type NFKC func(s string) string

// ValidFilterTitle reports whether title is a valid filter title: 1 to
// MaxKeywordFilterTitleGraphemes grapheme clusters of valid UTF-8 on one line, without
// surrounding white space or control characters.
func ValidFilterTitle(title string, graphemes GraphemeCounter) bool {
	if title == "" || strings.TrimSpace(title) != title || !utf8.ValidString(title) ||
		strings.ContainsFunc(title, unicode.IsControl) || utf8.RuneCountInString(title) > maxKeywordFilterLength {
		return false
	}
	return graphemes(title) <= MaxKeywordFilterTitleGraphemes
}

// NewFilterWord returns the filter word of what the user typed, trimmed, with its normalized
// form. It fails with ErrInvalidFilterWord for a word that is empty once normalized, longer than
// MaxKeywordFilterWordGraphemes grapheme clusters normalized, not valid UTF-8, or with control
// characters other than white space.
func NewFilterWord(word string, wholeWord bool, nfkc NFKC, graphemes GraphemeCounter) (FilterWord, error) {
	word = strings.TrimSpace(word)
	if !utf8.ValidString(word) || utf8.RuneCountInString(word) > maxKeywordFilterLength ||
		strings.ContainsFunc(word, func(r rune) bool { return unicode.IsControl(r) && !unicode.IsSpace(r) }) {
		return FilterWord{}, ErrInvalidFilterWord
	}
	normalized := strings.TrimSpace(NormalizeFilterText(word, nfkc))
	if normalized == "" || graphemes(normalized) > MaxKeywordFilterWordGraphemes {
		return FilterWord{}, ErrInvalidFilterWord
	}
	return FilterWord{Word: word, Normalized: normalized, WholeWord: wholeWord}, nil
}

// NormalizeFilterText brings s to the form that keyword filters match in (ADR-0157 §4): NFKC,
// then simple lower case, then katakana to hiragana, then every run of white space (line breaks
// too) to one space. Words are stored and texts are matched in this form, so that half-width
// and full-width forms, upper and lower case, and katakana and hiragana match each other.
func NormalizeFilterText(s string, nfkc NFKC) string {
	s = strings.ToLower(nfkc(s))
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		switch {
		case r >= 0x30A1 && r <= 0x30F6:
			r -= 0x60
		case r == 0x30FD || r == 0x30FE:
			r -= 0x60
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TextOfHTML returns the text of the sanitized HTML of a status: its text nodes, with the
// character references decoded, and a space where a line break or the end of a block parts them.
// Attributes, such as the href of a link, are not text. Mentions, hashtags, and :shortcode: are
// text as they show.
func TextOfHTML(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return s
	}
	var b strings.Builder
	for len(s) > 0 {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		s = s[i:]
		j := strings.IndexByte(s, '>')
		if j < 0 {
			break // an unterminated tag: drop the rest
		}
		if blockBoundary(s[1:j]) {
			b.WriteByte(' ')
		}
		s = s[j+1:]
	}
	return html.UnescapeString(b.String())
}

// blockBoundary reports whether the tag body (between < and >) parts the text around it: a line
// break, or the start or end of a block.
func blockBoundary(body string) bool {
	body = strings.TrimPrefix(strings.TrimSpace(body), "/")
	end := strings.IndexAny(body, " \t\n\r/")
	if end >= 0 {
		body = body[:end]
	}
	switch strings.ToLower(body) {
	case "br", "p", "div", "li", "ul", "ol", "blockquote", "pre", "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}

// FilterMatcher matches texts against the words of a set of keyword filters at once: one scan of a
// text finds every word it contains (Aho-Corasick over the normalized runes). Build one with
// NewFilterMatcher; a nil *FilterMatcher matches nothing.
type FilterMatcher struct {
	filters []KeywordFilter
	nfkc    NFKC
	nodes   []acNode
	words   []matchWord
}

// acNode is a state of the automaton: its transitions, the state of its longest proper suffix
// that is also a prefix of a word (fail), and the words that end at it, its own and those of its
// suffixes.
type acNode struct {
	next map[rune]int
	fail int
	out  []int
}

// matchWord is a word of the matcher: its filter and position, its length in runes, and whether
// it must stand alone at each end.
type matchWord struct {
	filter, word int
	length       int
	wordStart    bool
	wordEnd      bool
}

// FilterMatch is a filter that a status matched and the words it matched, as the user typed
// them, in the filter's order.
type FilterMatch struct {
	Filter KeywordFilter
	Words  []string
}

// NewFilterMatcher returns a matcher of the words of filters, which the caller has narrowed to
// those that apply (the context, the expiry). It returns nil when they have no words.
func NewFilterMatcher(filters []KeywordFilter, nfkc NFKC) *FilterMatcher {
	m := &FilterMatcher{filters: filters, nfkc: nfkc, nodes: []acNode{{}}}
	for fi, f := range filters {
		for wi, w := range f.Words {
			rs := []rune(w.Normalized)
			if len(rs) == 0 {
				continue
			}
			mw := matchWord{filter: fi, word: wi, length: len(rs)}
			if w.WholeWord {
				mw.wordStart, mw.wordEnd = wordRune(rs[0]), wordRune(rs[len(rs)-1])
			}
			m.add(rs, len(m.words))
			m.words = append(m.words, mw)
		}
	}
	if len(m.words) == 0 {
		return nil
	}
	m.link()
	return m
}

// add puts the word rs, numbered id, in the trie.
func (m *FilterMatcher) add(rs []rune, id int) {
	cur := 0
	for _, r := range rs {
		nx, ok := m.nodes[cur].next[r]
		if !ok {
			if m.nodes[cur].next == nil {
				m.nodes[cur].next = map[rune]int{}
			}
			nx = len(m.nodes)
			m.nodes = append(m.nodes, acNode{})
			m.nodes[cur].next[r] = nx
		}
		cur = nx
	}
	m.nodes[cur].out = append(m.nodes[cur].out, id)
}

// link sets the fail links breadth first and gathers the words of each suffix into out.
func (m *FilterMatcher) link() {
	queue := make([]int, 0, len(m.nodes))
	for _, nx := range m.nodes[0].next {
		queue = append(queue, nx)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for r, nx := range m.nodes[cur].next {
			f := m.nodes[cur].fail
			for {
				if to, ok := m.nodes[f].next[r]; ok && to != nx {
					m.nodes[nx].fail = to
					break
				}
				if f == 0 {
					m.nodes[nx].fail = 0
					break
				}
				f = m.nodes[f].fail
			}
			m.nodes[nx].out = append(m.nodes[nx].out, m.nodes[m.nodes[nx].fail].out...)
			queue = append(queue, nx)
		}
	}
}

// Match returns the filters that the fields match, in the order of the filters given to
// NewFilterMatcher, each with its words that matched. Each field is matched on its own: a word
// never matches across two fields.
func (m *FilterMatcher) Match(fields ...string) []FilterMatch {
	if m == nil {
		return nil
	}
	hit := make([]bool, len(m.words))
	for _, field := range fields {
		if field != "" {
			m.scan([]rune(NormalizeFilterText(field, m.nfkc)), hit)
		}
	}
	var out []FilterMatch
	for i, w := range m.words {
		if !hit[i] {
			continue
		}
		word := m.filters[w.filter].Words[w.word].Word
		if n := len(out); n > 0 && out[n-1].Filter.ID == m.filters[w.filter].ID {
			out[n-1].Words = append(out[n-1].Words, word)
			continue
		}
		out = append(out, FilterMatch{Filter: m.filters[w.filter], Words: []string{word}})
	}
	return out
}

// scan marks in hit the words that the normalized text contains.
func (m *FilterMatcher) scan(text []rune, hit []bool) {
	cur := 0
	for i, r := range text {
		for {
			if nx, ok := m.nodes[cur].next[r]; ok {
				cur = nx
				break
			}
			if cur == 0 {
				break
			}
			cur = m.nodes[cur].fail
		}
		for _, id := range m.nodes[cur].out {
			if hit[id] {
				continue
			}
			w := m.words[id]
			start := i - w.length + 1
			if w.wordStart && start > 0 && wordRune(text[start-1]) {
				continue
			}
			if w.wordEnd && i+1 < len(text) && wordRune(text[i+1]) {
				continue
			}
			hit[id] = true
		}
	}
}

// unspacedScripts are the scripts that do not part words with spaces (ADR-0157 §5): their
// characters are not word characters, so that "whole word only" works as a match anywhere in
// them.
var unspacedScripts = []*unicode.RangeTable{
	unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Bopomofo,
	unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar, unicode.Tai_Tham, unicode.New_Tai_Lue,
}

// wordRune reports whether r is a character of a word, which a whole-word match must not touch: a
// letter, a digit, a combining mark, or _, except the characters of the scripts that do not part
// words with spaces and the prolonged sound mark that goes with them.
func wordRune(r rune) bool {
	if r == '_' {
		return true
	}
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r) {
		return false
	}
	if r == 0x30FC || r == 0x3005 { // ー and 々 belong to no script of the list
		return false
	}
	return !unicode.In(r, unspacedScripts...)
}
