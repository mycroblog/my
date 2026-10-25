// SPDX-License-Identifier: AGPL-3.0-only

package docsync

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
)

const body = `## Summary

Fix it.

## Documentation

<!-- One line per document: ` + "`- <path>: no change — <reason>`" + `. -->
<!-- - docs/operations/hidden.md: no change — a reason inside a comment does not count -->

- docs/operations/email.md: changed
- ` + "`docs/operations/running.md`" + `: no change — only the log wording changed, which it does not quote
- docs/reference/webhooks.md: No change: the event payloads stay the same
- docs/operations/alerts.md: no change - fine
- docs/operations/crawlers.md: we did not touch robots

## Checklist

- docs/operations/outside.md: no change — a reason outside the section does not count either
`

func TestReasons(t *testing.T) {
	got := Reasons(body)
	want := map[string]string{
		"docs/operations/running.md": "only the log wording changed, which it does not quote",
		"docs/reference/webhooks.md": "the event payloads stay the same",
		"docs/operations/alerts.md":  "fine",
	}
	if len(got) != len(want) {
		t.Fatalf("Reasons = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Reasons[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestVerify(t *testing.T) {
	ims := []ctxmap.Impact{
		{Doc: "docs/operations/alerts.md", Paths: []string{"deploy/monitoring/alerts.yml"}},
		{Doc: "docs/operations/crawlers.md", Paths: []string{"internal/interface/web/crawlers.go"}},
		{Doc: "docs/operations/email.md", Paths: []string{"internal/adapter/mail/smtp/smtp.go"}, Changed: true},
		{Doc: "docs/operations/hidden.md", Paths: []string{"a.go", "b.go", "c.go", "d.go"}},
		{Doc: "docs/operations/running.md", Paths: []string{"internal/platform/log.go"}},
		{Doc: "docs/reference/webhooks.md", Paths: []string{"internal/domain/webhook/webhook.go"}},
	}
	got := Verify(ims, Reasons(body))
	want := []string{
		`docs/operations/alerts.md: the reason "fine" is too short`,
		"docs/operations/crawlers.md describes internal/interface/web/crawlers.go: change it in this pull request, or add the line `- docs/operations/crawlers.md: no change — <reason>`",
		"docs/operations/hidden.md describes a.go, b.go, c.go and 1 more:",
	}
	if len(got) != len(want) {
		t.Fatalf("Verify =\n%s", strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.Contains(got[i], want[i]) {
			t.Errorf("problem %d = %q, want it to contain %q", i, got[i], want[i])
		}
	}
}

func TestSection(t *testing.T) {
	ims := []ctxmap.Impact{
		{Doc: "docs/operations/email.md", Changed: true},
		{Doc: "docs/operations/running.md"},
	}
	section, problems := Section(ims, map[string]string{"docs/operations/running.md": "only the log wording changed"})
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	want := "## Documentation\n\n- docs/operations/email.md: changed\n- docs/operations/running.md: no change — only the log wording changed\n"
	if section != want {
		t.Errorf("Section =\n%s\nwant\n%s", section, want)
	}
	// What Section writes, Verify accepts.
	if p := Verify(ims, Reasons(section)); len(p) != 0 {
		t.Errorf("Verify rejects the section: %v", p)
	}
	if _, problems := Section(ims, nil); len(problems) != 1 {
		t.Errorf("a document with no reason: problems = %v", problems)
	}
	if section, _ := Section(nil, nil); !strings.Contains(section, "No user or operator document") {
		t.Errorf("Section of no impact = %q", section)
	}
}
