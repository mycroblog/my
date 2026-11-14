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

// RuleKnowledge is the knowledge of a role out of its shape or over its limit (ADR-0169 §3, §4, ADR-0170 §5).
const RuleKnowledge = "knowledge"

// The files of a role's knowledge: the index it always reads, its decisions, and its exemplars. The decisions and the
// exemplars are each one file (decisions.md) or area files (decisions/<area>.md), which the role reads only when its
// work touches the area.
const (
	knowledgeIndex     = "index.md"
	knowledgeDecisions = "decisions"
	knowledgeExemplars = "exemplars"
)

// Knowledge is the knowledge of the UI and developer roles: <Dir>/<role>/ for each of Roles, each file at most its limit in bytes,
// and the logs of the decider's picks, <Picks>/<role>.jsonl, which the roles do not read.
type Knowledge struct {
	Dir    string   `yaml:"dir"`
	Roles  []string `yaml:"roles"`
	Picks  string   `yaml:"picks"`
	Limits struct {
		Index     int `yaml:"index"`
		Decisions int `yaml:"decisions"`
		Exemplars int `yaml:"exemplars"`
		Area      int `yaml:"area"`
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

// knowledgeFile is one file of a role's knowledge to check: its limit and, for a list file, its kind.
type knowledgeFile struct {
	path, kind string
	limit      int
}

// knowledge checks each role's directory: index.md, and the decisions and the exemplars each as one file or as area
// files, and nothing else. Each file is within its limit, the index a page under a title, and every list file a title
// and a lead followed by one item per line. The area files have no limit on their total.
func (l *linter) knowledge(files []string) ([]Finding, error) {
	k := l.c.Knowledge
	var out []Finding
	for _, role := range k.Roles {
		dir := path.Join(k.Dir, role)
		flat := map[string]int{knowledgeDecisions: k.Limits.Decisions, knowledgeExemplars: k.Limits.Exemplars}
		var todo []knowledgeFile
		areas := map[string]bool{}
		for _, f := range files {
			rel, ok := strings.CutPrefix(f, dir+"/")
			if !ok {
				continue
			}
			kind, area, sub := strings.Cut(rel, "/")
			_, list := flat[kind]
			switch {
			case rel == knowledgeIndex, rel == knowledgeDecisions+".md", rel == knowledgeExemplars+".md":
			case sub && list && !strings.Contains(area, "/") && strings.HasSuffix(area, ".md"):
				areas[kind] = true
				todo = append(todo, knowledgeFile{f, kind, k.Limits.Area})
			default:
				out = append(out, Finding{f, 0, RuleKnowledge, "a role keeps index.md, and decisions.md or decisions/<area>.md, exemplars.md or exemplars/<area>.md, only (ADR-0169 §3): fold it into one of them, or delete it"})
			}
		}
		if p := path.Join(dir, knowledgeIndex); l.exists[p] {
			todo = append(todo, knowledgeFile{p, "", k.Limits.Index})
		} else {
			out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("missing: the %s role keeps %s, under a title (ADR-0169 §3)", role, knowledgeIndex)})
		}
		for _, kind := range []string{knowledgeDecisions, knowledgeExemplars} {
			p := path.Join(dir, kind+".md")
			switch {
			case l.exists[p] && areas[kind]:
				out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("the %s role keeps %s.md or area files in %s/, not both (ADR-0169 §3): move its lines into the area files", role, kind, kind)})
			case l.exists[p]:
				todo = append(todo, knowledgeFile{p, kind, flat[kind]})
			case !areas[kind]:
				out = append(out, Finding{p, 0, RuleKnowledge, fmt.Sprintf("missing: the %s role keeps %s.md, or area files %s/<area>.md, under a title (ADR-0169 §3)", role, kind, kind)})
			}
		}
		for _, f := range todo {
			src, err := os.ReadFile(filepath.Join(l.root, filepath.FromSlash(f.path)))
			if err != nil {
				return nil, err
			}
			if n := len(src); n > f.limit {
				out = append(out, Finding{f.path, 0, RuleKnowledge, fmt.Sprintf("%d bytes, over the limit of %d: merge similar lines, delete the lines that keep missing or go unused, or turn a line the decider repeats into a check (ADR-0169 §4); only the decider changes the limit (%s knowledge.limits)", n, f.limit, File)})
			}
			out = append(out, knowledgeShape(f.path, f.kind, string(src))...)
		}
	}
	return out, nil
}

// knowledgeShape checks one file: a "# " title on the first line and, in a list file (kind decisions or exemplars),
// after the lead before the first item, items of the kind's shape only.
func knowledgeShape(p, kind, src string) []Finding {
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	var out []Finding
	if !strings.HasPrefix(lines[0], "# ") {
		out = append(out, Finding{p, 1, RuleKnowledge, "the first line is the title: # <role>: " + strings.TrimSuffix(path.Base(p), ".md")})
	}
	want, list := knowledgeLines[kind]
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
