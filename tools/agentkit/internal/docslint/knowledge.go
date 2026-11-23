// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// RuleKnowledge is the knowledge of a role out of its shape (ADR-0169 §3, ADR-0170 §5).
const RuleKnowledge = "knowledge"

// The files of a role's knowledge: the index it always reads, its decisions, and its exemplars. The decisions and the
// exemplars are each one file (decisions.md) or area files (decisions/<area>.md), which the role reads only when its
// work touches the area.
const (
	knowledgeIndex     = "index.md"
	knowledgeDecisions = "decisions"
	knowledgeExemplars = "exemplars"
)

// Knowledge is the knowledge of the UI and developer roles: <Dir>/<role>/ for each of Roles, and the logs of the
// decider's picks, <Picks>/<role>.jsonl, which the roles do not read.
type Knowledge struct {
	Dir   string   `yaml:"dir"`
	Roles []string `yaml:"roles"`
	Picks string   `yaml:"picks"`
}

// knowledge checks each role's directory: index.md, and the decisions and the exemplars each as one file or as area
// files, and nothing else. Each file is under a title; no file has a size limit (ADR-0169 §4).
func (l *linter) knowledge(files []string) ([]Finding, error) {
	k := l.c.Knowledge
	var out []Finding
	for _, role := range k.Roles {
		dir := path.Join(k.Dir, role)
		flat := map[string]bool{knowledgeDecisions: true, knowledgeExemplars: true}
		var todo []string
		areas := map[string]bool{}
		for _, f := range files {
			rel, ok := strings.CutPrefix(f, dir+"/")
			if !ok {
				continue
			}
			kind, area, sub := strings.Cut(rel, "/")
			list := flat[kind]
			switch {
			case rel == knowledgeIndex, rel == knowledgeDecisions+".md", rel == knowledgeExemplars+".md":
			case sub && list && !strings.Contains(area, "/") && strings.HasSuffix(area, ".md"):
				areas[kind] = true
				todo = append(todo, f)
			default:
				out = append(out, Finding{f, 0, RuleKnowledge, "a role keeps index.md, and decisions.md or decisions/<area>.md, exemplars.md or exemplars/<area>.md, only (ADR-0169 §3): fold it into one of them, or delete it"})
			}
		}
		if p := path.Join(dir, knowledgeIndex); l.exists[p] {
			todo = append(todo, p)
		} else {
			out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("missing: the %s role keeps %s, under a title (ADR-0169 §3)", role, knowledgeIndex)})
		}
		for _, kind := range []string{knowledgeDecisions, knowledgeExemplars} {
			p := path.Join(dir, kind+".md")
			switch {
			case l.exists[p] && areas[kind]:
				out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("the %s role keeps %s.md or area files in %s/, not both (ADR-0169 §3): move its lines into the area files", role, kind, kind)})
			case l.exists[p]:
				todo = append(todo, p)
			case !areas[kind]:
				out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("missing: the %s role keeps %s.md, or area files %s/<area>.md, under a title (ADR-0169 §3)", role, kind, kind)})
			}
		}
		for _, f := range todo {
			src, err := os.ReadFile(filepath.Join(l.root, filepath.FromSlash(f)))
			if err != nil {
				return nil, err
			}
			out = append(out, knowledgeShape(f, string(src))...)
		}
	}
	return out, nil
}

// knowledgeShape checks one file: a "# " title on the first line.
func knowledgeShape(p, src string) []Finding {
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	var out []Finding
	if !strings.HasPrefix(lines[0], "# ") {
		out = append(out, Finding{p, 1, RuleKnowledge, "the first line is the title: # <role>: " + strings.TrimSuffix(path.Base(p), ".md")})
	}
	return out
}
