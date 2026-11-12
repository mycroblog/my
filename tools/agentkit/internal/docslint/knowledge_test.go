// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"reflect"
	"strings"
	"testing"
)

func knowledgeConfig() *Config {
	c := &Config{}
	c.Knowledge.Dir = ".claude/knowledge"
	c.Knowledge.Roles = []string{"copy", "walker"}
	c.Knowledge.Limits.Index = 64
	c.Knowledge.Limits.Decisions = 512
	c.Knowledge.Limits.Exemplars = 256
	return c
}

const (
	goodDecision = "- Write sign in — one name — the decider, PR #1 — 2026-11-12 — held 2 / missed 0\n"
	goodExemplar = "- The sign-in screen — one field per line — the decider's choice — 2026-11-12\n"
)

func TestKnowledgeInShape(t *testing.T) {
	got := lint(t, knowledgeConfig(), map[string]string{
		".claude/knowledge/copy/index.md":          "# copy: index\n\nRead the decisions.\n",
		".claude/knowledge/copy/decisions.md":      "# copy: decisions\n\nOne line each.\n\n" + goodDecision + goodDecision,
		".claude/knowledge/copy/exemplars.md":      "# copy: exemplars\n\n" + goodExemplar,
		".claude/knowledge/walker/index.md":        "# walker: index\n",
		".claude/knowledge/walker/decisions.md":    "# walker: decisions\n",
		".claude/knowledge/walker/exemplars.md":    "# walker: exemplars\n",
		".claude/knowledge/interop-advisor/mx.md":  "# Not a UI role\n\nFree prose.\n",
		".claude/knowledge/interop-advisor/foo.md": "Anything\n",
	})
	if len(got) != 0 {
		t.Errorf("findings = %v, want none", got)
	}
}

func TestKnowledgeOutOfShape(t *testing.T) {
	got := lint(t, knowledgeConfig(), map[string]string{
		".claude/knowledge/copy/index.md": "No title, and longer than the limit of the index, which is sixty-four bytes.\n",
		".claude/knowledge/copy/decisions.md": "# copy: decisions\n\n" + goodDecision +
			"- A decision without its counts — why — from — 2026-11-12\n" +
			"- A bad date — why — from — 2026-13-01 — held 0 / missed 0\n" +
			"- Bad counts — why — from — 2026-11-12 — held some / missed 0\n" +
			"- Empty — — from — 2026-11-12 — held 0 / missed 0\n" +
			"\nA paragraph after the items.\n" +
			"  continued on a second line\n",
		".claude/knowledge/copy/exemplars.md": "# copy: exemplars\n\n" + strings.Repeat(goodExemplar, 4),
		".claude/knowledge/copy/notes.md":     "# notes\n",
		".claude/knowledge/copy/old/a.md":     "# a\n",
		".claude/knowledge/walker/index.md":   "# walker: index\n",
	})
	want := []string{
		".claude/knowledge/copy/decisions.md:4:knowledge",
		".claude/knowledge/copy/decisions.md:5:knowledge",
		".claude/knowledge/copy/decisions.md:6:knowledge",
		".claude/knowledge/copy/decisions.md:7:knowledge",
		".claude/knowledge/copy/decisions.md:9:knowledge",
		".claude/knowledge/copy/decisions.md:10:knowledge",
		".claude/knowledge/copy/exemplars.md:0:knowledge",
		".claude/knowledge/copy/index.md:0:knowledge",
		".claude/knowledge/copy/index.md:1:knowledge",
		".claude/knowledge/copy/notes.md:0:knowledge",
		".claude/knowledge/copy/old/a.md:0:knowledge",
		".claude/knowledge/walker/decisions.md:0:knowledge",
		".claude/knowledge/walker/exemplars.md:0:knowledge",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings =\n%v\nwant\n%v", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestKnowledgeItem(t *testing.T) {
	for ln, want := range map[string]string{
		goodDecision[:len(goodDecision)-1]:                           "",
		"- a — b — c — 2026-11-12 — held 1 / missed 02":              `the counts "held 1 / missed 02" are not held N / missed N`,
		"- a — b — c — 12/11/2026 — held 1 / missed 0":               `the date "12/11/2026" is not YYYY-MM-DD`,
		"- a — b — c — d — 2026-11-12 — held 1 / missed 0":           "6 fields, not 5",
		"- a with — inside — b — c — 2026-11-12 — held 0 / missed 0": "6 fields, not 5",
	} {
		if got := knowledgeItem(ln, 5, true); got != want {
			t.Errorf("knowledgeItem(%q) = %q, want %q", ln, got, want)
		}
	}
}
