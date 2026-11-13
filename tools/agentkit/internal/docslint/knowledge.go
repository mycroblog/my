// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// RuleKnowledge is the knowledge of a UI role out of its shape or over its limit (ADR-0169 §3, §4).
const RuleKnowledge = "knowledge"

// The files of a role's knowledge: the index it always reads, its decisions, and its exemplars.
const (
	knowledgeIndex     = "index.md"
	knowledgeDecisions = "decisions.md"
	knowledgeExemplars = "exemplars.md"
)

// Knowledge is the knowledge of the UI roles: <Dir>/<role>/ for each of Roles, each file at most its limit in bytes,
// and the logs of the decider's picks, <Picks>/<role>.jsonl, which the roles do not read.
type Knowledge struct {
	Dir    string   `yaml:"dir"`
	Roles  []string `yaml:"roles"`
	Picks  string   `yaml:"picks"`
	Limits struct {
		Index     int `yaml:"index"`
		Decisions int `yaml:"decisions"`
		Exemplars int `yaml:"exemplars"`
	} `yaml:"limits"`
}

// The line each list file holds, by its separators " — " and its last fields.
var knowledgeLines = map[string]struct {
	fields int
	counts bool
	shape  string
}{
	knowledgeDecisions: {5, true, "- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N"},
	knowledgeExemplars: {4, false, "- <the model screen, or the options that lost> — <why> — <from: the decider's choice> — <YYYY-MM-DD>"},
}

// knowledge checks each role's directory: its three files and no other, each within its limit, the index a page
// under a title, and the decisions and the exemplars a title and a lead followed by one item per line.
func (l *linter) knowledge(files []string) ([]Finding, error) {
	k := l.c.Knowledge
	var out []Finding
	limits := map[string]int{knowledgeIndex: k.Limits.Index, knowledgeDecisions: k.Limits.Decisions, knowledgeExemplars: k.Limits.Exemplars}
	for _, role := range k.Roles {
		dir := path.Join(k.Dir, role)
		for _, f := range files {
			if path.Dir(f) == dir {
				if _, ok := limits[path.Base(f)]; !ok {
					out = append(out, Finding{f, 0, RuleKnowledge, "a role keeps index.md, decisions.md and exemplars.md only (ADR-0169 §3): fold it into one of them, or delete it"})
				}
			} else if strings.HasPrefix(f, dir+"/") {
				out = append(out, Finding{f, 0, RuleKnowledge, "a role's knowledge has no subdirectories (ADR-0169 §3): fold it into index.md, decisions.md or exemplars.md"})
			}
		}
		for _, name := range []string{knowledgeIndex, knowledgeDecisions, knowledgeExemplars} {
			p := path.Join(dir, name)
			if !l.exists[p] {
				out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("missing: the %s role keeps %s, under a title (ADR-0169 §3)", role, name)})
				continue
			}
			src, err := os.ReadFile(filepath.Join(l.root, filepath.FromSlash(p)))
			if err != nil {
				return nil, err
			}
			if n := len(src); n > limits[name] {
				out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("%d bytes, over the limit of %d: merge similar lines, delete the lines that keep missing or go unused, or turn a line the decider repeats into a check (ADR-0169 §4); only the decider changes the limit (%s knowledge.limits)", n, limits[name], File)})
			}
			out = append(out, knowledgeShape(p, string(src))...)
		}
	}
	return out, nil
}

// knowledgeShape checks one file: a "# " title on the first line and, in a list file, after the lead before the
// first item, items of the file's shape only.
func knowledgeShape(p, src string) []Finding {
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	var out []Finding
	if !strings.HasPrefix(lines[0], "# ") {
		out = append(out, Finding{p, 1, RuleKnowledge, "the first line is the title: # <role>: " + strings.TrimSuffix(path.Base(p), ".md")})
	}
	want, list := knowledgeLines[path.Base(p)]
	if !list {
		return out
	}
	items := false
	for i, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "- "):
			items = true
			if msg := knowledgeItem(ln, want.fields, want.counts); msg != "" {
				out = append(out, Finding{p, i + 1, RuleKnowledge, msg + "; the line is " + want.shape})
			}
		case items && strings.TrimSpace(ln) != "":
			out = append(out, Finding{p, i + 1, RuleKnowledge, "no paragraphs among the items: one per line, " + want.shape + " (ADR-0169 §3)"})
		}
	}
	return out
}

// knowledgeItem says what is wrong with one item, or "" when it has its fields, a real date and, for a decision, its
// counts.
func knowledgeItem(ln string, fields int, counts bool) string {
	parts := strings.Split(strings.TrimPrefix(ln, "- "), " — ")
	if len(parts) != fields {
		return fmt.Sprintf("%d fields, not %d", len(parts), fields)
	}
	for _, f := range parts {
		if strings.TrimSpace(f) == "" {
			return "an empty field"
		}
	}
	date := parts[3]
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return fmt.Sprintf("the date %q is not YYYY-MM-DD", date)
	}
	if counts {
		var held, missed int
		if n, err := fmt.Sscanf(parts[4], "held %d / missed %d", &held, &missed); n != 2 || err != nil || held < 0 || missed < 0 ||
			parts[4] != fmt.Sprintf("held %d / missed %d", held, missed) {
			return fmt.Sprintf("the counts %q are not held N / missed N", parts[4])
		}
	}
	return ""
}
