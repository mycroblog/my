// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// headings are the sections of the prose, in order.
var headings = []string{"## Main changes", "## For operators", "## For API and library users"}

// bodyLimit is what GitHub takes in a Release's body, in characters; bytes are never fewer. scripts/release-notes.sh
// holds the facts to the same limit.
const bodyLimit = 125_000

// noteRoom is left for the comment that opens the body.
const noteRoom = 1_000

const repositoryURL = "https://github.com/mycroblog/my/"

var (
	number     = regexp.MustCompile(`#(\d+)\b`)
	pullLink   = regexp.MustCompile(`/(?:pull|issues)/(\d+)\b`)
	inlineCode = regexp.MustCompile("`[^`\n]*`")
	anyURL     = regexp.MustCompile(`(?i)(?:\b[a-z][a-z0-9+.-]*://|\bwww\.|//)[^\s)>\]]*`)
	linkTarget = regexp.MustCompile(`\]\(\s*([^)\s]*)`)
	image      = regexp.MustCompile(`!\[`)
	html       = regexp.MustCompile(`<[a-zA-Z!/?]`)
	mention    = regexp.MustCompile(`(?:^|[^\w/.-])@[a-zA-Z0-9]`)
)

// check returns what is wrong with the prose, in words that repeat nothing of it but numbers: the problems go back to
// the model and to the log. Its inputs come from the repository and from GitHub's list of merged PRs, never from the
// text of a PR, so that no such text can change what passes.
func check(prose string, in input) []string {
	if strings.TrimSpace(prose) == "" {
		return []string{"the answer is empty"}
	}
	var problems []string
	if room := bodyLimit - len(in.facts) - len(in.changelog) - noteRoom; len(prose) > room {
		problems = append(problems, fmt.Sprintf("the answer has %d bytes, and the limit is %d", len(prose), room))
	}
	if got := headingLines(prose); !slices.Equal(got, headings) {
		problems = append(problems, "the headings of level one and two must be exactly these, in order: "+strings.Join(headings, ", "))
	}
	for _, n := range unknownNumbers(prose, in.known) {
		problems = append(problems, fmt.Sprintf("#%d is not a pull request merged in this release", n))
	}
	lines := strings.Split(prose, "\n")
	for _, item := range in.required {
		if problem := checkItem(item, lines, in.known); problem != "" {
			problems = append(problems, problem)
		}
	}
	// Text in backticks shows as written, so it can neither hide anything nor mention anyone.
	text := inlineCode.ReplaceAllString(prose, "")
	if outside := outsideLinks(text); outside > 0 {
		problems = append(problems, fmt.Sprintf("%d links lead outside %s", outside, repositoryURL))
	}
	if image.MatchString(text) {
		problems = append(problems, "the answer has an image")
	}
	if html.MatchString(text) {
		problems = append(problems, "the answer has HTML outside backticks")
	}
	if mention.MatchString(text) {
		problems = append(problems, "the answer has an @mention outside backticks")
	}
	return problems
}

// headingLines lists the headings of level one and two outside code fences, where # starts a comment.
func headingLines(prose string) []string {
	var found []string
	fenced := false
	for line := range strings.SplitSeq(prose, "\n") {
		line = strings.TrimRight(line, " \t\r")
		switch {
		case strings.HasPrefix(line, "```"):
			fenced = !fenced
		case !fenced && (strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ")):
			found = append(found, line)
		}
	}
	return found
}

func unknownNumbers(prose string, known map[int]bool) []int {
	var unknown []int
	for _, re := range []*regexp.Regexp{number, pullLink} {
		for _, m := range re.FindAllStringSubmatch(prose, -1) {
			n, err := strconv.Atoi(m[1])
			if (err != nil || !known[n]) && !slices.Contains(unknown, n) {
				unknown = append(unknown, n)
			}
		}
	}
	return unknown
}

// checkItem wants a line with every token of the item in backticks and a merged PR, and returns the problem otherwise.
func checkItem(item requiredItem, lines []string, known map[int]bool) string {
	named := false
	for _, line := range lines {
		if !hasTokens(line, item.Tokens) {
			continue
		}
		named = true
		for _, m := range number.FindAllStringSubmatch(line, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && known[n] {
				return ""
			}
		}
	}
	tokens := "`" + strings.Join(item.Tokens, "`, `") + "`"
	if named {
		return "the line about " + tokens + " names no merged pull request"
	}
	return "no line holds " + tokens + " in backticks (" + item.Kind + ")"
}

func hasTokens(line string, tokens []string) bool {
	for _, token := range tokens {
		if !strings.Contains(line, "`"+token+"`") {
			return false
		}
	}
	return true
}

func outsideLinks(text string) int {
	outside := 0
	for _, url := range anyURL.FindAllString(text, -1) {
		if !strings.HasPrefix(url, repositoryURL) {
			outside++
		}
	}
	for _, m := range linkTarget.FindAllStringSubmatch(text, -1) {
		// A relative target, which anyURL does not see, would resolve against wherever the notes are shown.
		if !anyURL.MatchString(m[1]) {
			outside++
		}
	}
	return outside
}
