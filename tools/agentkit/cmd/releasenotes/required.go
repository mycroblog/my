// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"regexp"
	"slices"
	"strings"
)

// requiredItem is a fact that the prose of the release notes must name (tools/agentkit/cmd/releaseprose checks it):
// the prose's line about it holds each of Tokens in backticks. It is in required.json, which only this reader writes.
type requiredItem struct {
	Kind   string   `json:"kind"`
	Tokens []string `json:"tokens"`
	Text   string   `json:"text"`
}

var codeSpan = regexp.MustCompile("`([^`]+)`")

func codeSpans(item string) []string {
	var spans []string
	for _, m := range codeSpan.FindAllStringSubmatch(item, -1) {
		spans = append(spans, m[1])
	}
	return spans
}

// requiredItems lists the data upgrades, the settings removed or changed, and the breaking changes of a later release.
// The schema files are not among them: a changed file does not tell that anything broke, and it stays in the facts.
func requiredItems(old, cur facts, api apiComparison, commits []commit) []requiredItem {
	items := []requiredItem{}
	add := func(kind, text string, tokens ...string) {
		items = append(items, requiredItem{Kind: kind, Tokens: tokens, Text: text})
	}
	knownSteps := map[string]bool{}
	for _, s := range old.steps {
		knownSteps[s.key] = true
	}
	for _, s := range cur.steps {
		if !knownSteps[s.key] {
			add("data upgrade", s.description, s.key)
		}
	}
	knownPrereqs := map[string]bool{}
	for _, p := range old.prereqs {
		knownPrereqs[p.key] = true
	}
	for _, p := range cur.prereqs {
		if !knownPrereqs[p.key] {
			add("data upgrade", "this release removes data that it carried over: upgrade through "+p.via+" first", p.key)
		}
	}
	for _, key := range sortedKeys(cur.settings) {
		if _, had := old.settings[key]; !had && cur.settings[key].required == "yes" {
			add("setting", "a new required setting", key)
		}
	}
	for _, item := range settingChanges(old, cur) {
		add("setting", item, codeSpans(item)[0])
	}
	for _, item := range slices.Concat(api.removedOperations, api.removedFields, api.removedScopes, api.changedFields) {
		// The `METHOD /path` of an operation's label stays out: its id names it already.
		tokens := slices.DeleteFunc(codeSpans(item), func(span string) bool { return strings.Contains(span, " ") })
		add("client API", item, tokens...)
	}
	for _, c := range commits {
		add("breaking commit", c.subject, c.sha)
	}
	return items
}
