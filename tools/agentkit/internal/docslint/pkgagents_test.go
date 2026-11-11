// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"reflect"
	"strings"
	"testing"
)

func TestPackageAgents(t *testing.T) {
	const link = "[Stripe documentation](https://docs.stripe.com/llms.txt)"
	item := "- Verifying a webhook signature: " + link + ", search for: `Stripe-Signature`, tolerance"
	files := map[string]string{
		// The root's file is the index: the rule does not read it.
		"AGENTS.md": "# AGENTS.md\n\nProse, a [link](docs/a.md), and\n\n## Sections\n\n| a |\n| --- |\n",
		"docs/a.md": "# A\n",
		// The shape: a title, then pointers, the limit itself.
		"internal/adapter/ok/AGENTS.md": "# Stripe\n\n" + strings.Repeat(item+"\n", PackageAgentsMaxItems),
		"internal/adapter/bad/AGENTS.md": strings.Join([]string{
			"# Stripe",
			"",
			"The adapter keeps the SDK's types inside the package.", // 3
			"## Layout", // 4
			item,
			"- Reading an RFC: [RFC 7033](https://www.rfc-editor.org/rfc/rfc7033#section-4.4), search for: JRD", // 6
			"- Changing the retry: read `retry.go` first",                                                       // 7
			"- Two documents: [a](https://a.example/) and [b](https://b.example/), search for: x",               // 8
			"- A local document: [ADR](../../../docs/a.md), search for: x",                                      // 9
			"- No terms: " + link,                       // 10
			"- Empty terms: " + link + ", search for: ", // 11
			"- A bare address too: " + link + ", search for: https://docs.stripe.com/webhooks", // 12
			"  - nested: " + link + ", search for: x",                                          // 13
			"| a | b |", // 14
			"```",       // 15
			"code",      // 16
			"```",       // 17
		}, "\n"),
		"internal/adapter/many/AGENTS.md":    "# Stripe\n" + strings.Repeat(item+"\n", PackageAgentsMaxItems+2),
		"internal/adapter/notitle/AGENTS.md": item + "\n" + item + "\n",
		"internal/adapter/empty/AGENTS.md":   "\n",
		// A file of another name, and a fixture, are not read.
		"internal/adapter/ok/README.md":          "# Readme\n\nProse.\n",
		"internal/adapter/ok/testdata/AGENTS.md": "prose\n",
	}
	got := lint(t, &Config{Skip: []string{"**/testdata/**"}}, files)
	var want []string
	for _, n := range []string{"3", "4", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17"} {
		want = append(want, "internal/adapter/bad/AGENTS.md:"+n+":package-agents")
	}
	want = append(want,
		"internal/adapter/empty/AGENTS.md:0:package-agents",
		"internal/adapter/many/AGENTS.md:14:package-agents",
		"internal/adapter/notitle/AGENTS.md:1:package-agents",
	)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestPackageAgentsMessages(t *testing.T) {
	const link = "[Stripe documentation](https://docs.stripe.com/llms.txt)"
	const other = ": " + packageAgentsHolds + "; delete this line, or move a rule into the failure message of its check and anything about the code into the code"
	for src, want := range map[string]string{
		"# T\n- X: [RFC 7033](https://www.rfc-editor.org/rfc/rfc7033#section-4.4), search for: JRD\n": "link the documentation's entry point and name the terms to search for; a link to a section goes stale",
		"# T\nThe adapter retries.\n":                    "a line that is not a pointer" + other,
		"# T\n## Rules\n":                                "a second heading" + other,
		"# T\n| a |\n":                                   "a table" + other,
		"# T\n- X: " + link + "\n":                       "no terms after the link: write the pointer as " + packageAgentsItem + ", with the exact terms the documentation uses",
		"# T\n- X: [ADR](../docs/a.md), search for: x\n": "the link ../docs/a.md is not an https:// link to an external document: " + packageAgentsHolds,
		"# T\n- X: read the code\n":                      "a pointer has exactly one link, to the external document that governs the task: write it as " + packageAgentsItem + "; " + packageAgentsHolds,
		"## T\n":                                         "the first line is not a `# <what the package implements or talks to>` title: " + packageAgentsHolds,
		"# T\n" + strings.Repeat("- X: "+link+", search for: x\n", PackageAgentsMaxItems+1): "more than 12 pointers: " + packageAgentsHolds + "; move the pointers of a subdirectory into its own AGENTS.md",
	} {
		fs := packageAgents("a/AGENTS.md", src)
		if len(fs) != 1 || fs[0].Msg != want {
			t.Errorf("packageAgents(%q) = %v, want the message %q", src, fs, want)
		}
	}
	// A term in a code span may be an address.
	if fs := packageAgents("a/AGENTS.md", "# T\n\n- X: "+link+", search for: `Stripe-Signature`, `https://api.stripe.com/v1/events`\n"); len(fs) != 0 {
		t.Errorf("a pointer: %v", fs)
	}
}
