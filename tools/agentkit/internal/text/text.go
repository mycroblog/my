// SPDX-License-Identifier: AGPL-3.0-only

// Package text holds the small text helpers every part of agentkit shares: token estimates, GitHub heading
// anchors, the identifiers the repository's documents use (FR-xxx, ADR-xxxx, issue numbers), and line slicing.
package text

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tokens estimates the model tokens of s: about four ASCII bytes per token, and one token per other rune
// (Japanese text costs roughly a token a character).
func Tokens(s string) int {
	ascii, other := 0, 0
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return ascii/4 + other
}

// Ktok formats a token estimate compactly: 840, 1.2k, 31k.
func Ktok(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10000:
		return strconv.Itoa(n/1000) + "." + strconv.Itoa(n%1000/100) + "k"
	default:
		return strconv.Itoa(n/1000) + "k"
	}
}

// Slug returns the anchor GitHub generates for a heading: lower case, letters (any script), digits, marks,
// hyphens and underscores kept, spaces turned into hyphens, everything else dropped.
func Slug(heading string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(heading) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

var (
	reFR    = regexp.MustCompile(`\b(?:NFR-[A-Z]+-\d+|NFR-\d{3}|FR-\d{3})\b`)
	reADR   = regexp.MustCompile(`\bADR-(\d{4})\b`)
	reIssue = regexp.MustCompile(`(?:^|[\s(（、,])#(\d{1,5})\b`)
	reIdent = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_.:/{}-]{2,80})`")
	reNum   = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s`)
)

// Requirements returns the FR / NFR identifiers in s, deduplicated, in order of appearance.
func Requirements(s string) []string { return uniq(reFR.FindAllString(s, -1)) }

// ADRs returns the ADR identifiers (ADR-0016) in s.
func ADRs(s string) []string { return uniq(reADR.FindAllString(s, -1)) }

// Issues returns the issue and pull request numbers written as #123 in s.
func Issues(s string) []string {
	var out []string
	for _, m := range reIssue.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return uniq(out)
}

// Idents returns the code spans of s that look like identifiers, paths or routes.
func Idents(s string) []string {
	var out []string
	for _, m := range reIdent.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return uniq(out)
}

// SectionNumber returns the leading section number of a heading ("4.4" of "4.4 content"), or "".
func SectionNumber(heading string) string {
	if m := reNum.FindStringSubmatch(strings.TrimSpace(heading) + " "); m != nil {
		return m[1]
	}
	return ""
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Uniq returns in without duplicates, in order.
func Uniq(in []string) []string { return uniq(append([]string(nil), in...)) }

// SortedKeys returns the keys of m in order.
func SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Lines returns lines start..end (1-based, inclusive) of src, clamped to the file.
func Lines(src []byte, start, end int) string {
	lines := strings.SplitAfter(string(src), "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) || end <= 0 {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "")
}

// Truncate cuts s to at most limit runes, marking the cut.
func Truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return string(r[:limit]) + "…"
}

// OneLine collapses whitespace so that s fits on one line.
func OneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// dependencyWords mark a line of an issue that says it waits for another issue or pull request.
var dependencyWords = []string{"着手しない", "マージされるまで", "の後に着手", "blocked by", "depends on", "stacked on", "after #"}

// sentenceEnd splits a line into sentences, so that a number in the sentence after "Depends on #1."
// is not read as something the work waits for.
var sentenceEnd = regexp.MustCompile(`[.。]\s+|。`)

// Dependencies returns the issue and pull request numbers that sentences of body say the work waits for.
func Dependencies(body string) []string {
	var out []string
	for _, l := range strings.Split(body, "\n") {
		for _, sentence := range sentenceEnd.Split(l, -1) {
			lower := strings.ToLower(sentence)
			for _, w := range dependencyWords {
				if strings.Contains(lower, w) {
					out = append(out, Issues(sentence)...)
					break
				}
			}
		}
	}
	return uniq(out)
}
