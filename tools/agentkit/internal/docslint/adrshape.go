// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// RuleADRShape is an ADR that transcribes the implementation instead of recording a decision.
const RuleADRShape = "adr-shape"

// The limits of an ADR: the decision-level ADRs fit well inside them, and a table of more rows lists what the code
// defines (jobs, keys, tables).
const (
	ADRMaxChars         = 6000
	ADRMaxDecisionChars = 4000
	ADRMaxTableRows     = 8
)

// adrHolds is what the findings tell the author.
const adrHolds = "an ADR holds the decision, its reasons (what was rejected included), and the conditions to revisit it"

var (
	// A file by its extension: source code, and the YAML of the tasks and workflows.
	reSourceFile = regexp.MustCompile(`^[\w.<>*\[\]{}/-]*[\w>*\]}]\.(?:go|ts|tsx|js|jsx|mjs|cjs|sh|py|sql|proto|tsp|tf|css|yml|yaml)$`)
	// pkg.Exported, with what follows it (a method, a call).
	reGoQualified = regexp.MustCompile(`^([a-z][a-z0-9]*)\.[A-Z]\w*(?:\.\w+)*(?:\(.*\))?$`)
	// A Go error value.
	reGoErr = regexp.MustCompile(`^Err[A-Z]\w*$`)
)

// artifacts is what of the repository a code span can name.
type artifacts struct {
	top   map[string]bool // the top-level directories
	pkgs  map[string]bool // the names of the directories with Go files
	tasks map[string]bool // the namespaced tasks (docs:rules)
}

// newArtifacts reads the names from files, and the tasks from the Taskfile when the repository has one.
func newArtifacts(root string, files []string) (*artifacts, error) {
	a := &artifacts{top: map[string]bool{}, pkgs: map[string]bool{}, tasks: map[string]bool{}}
	taskfile := false
	for _, f := range files {
		if i := strings.IndexByte(f, '/'); i > 0 {
			a.top[f[:i]] = true
		}
		if strings.HasSuffix(f, ".go") && strings.Contains(f, "/") {
			a.pkgs[path.Base(path.Dir(f))] = true
		}
		taskfile = taskfile || f == "Taskfile.yml"
	}
	if !taskfile {
		return a, nil
	}
	ts, err := tasks(root, nil)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		if strings.Contains(t.Name, ":") {
			a.tasks[t.Name] = true
		}
	}
	return a, nil
}

// kind says what of the implementation the code span names: "a path", "a file", "a task", "an identifier", or ""
// for anything else. It reads only what cannot be a term of the product: a URL, a route (/statuses/{id}), an npm
// package (@my/ui), an option (--base), a host or a module path, a snake_case or CamelCase word, and an environment
// variable are not artifacts to it.
func (a *artifacts) kind(span string) string {
	span = strings.TrimSpace(span)
	if m := reGoQualified.FindStringSubmatch(span); m != nil && a.pkgs[m[1]] {
		return "an identifier"
	}
	if reGoErr.MatchString(span) {
		return "an identifier"
	}
	for _, tok := range strings.Fields(span) {
		tok = strings.TrimPrefix(strings.Trim(tok, `"'(),;`), "./")
		if tok == "" || strings.ContainsAny(tok[:1], "/@-$") || strings.Contains(tok, "://") {
			continue
		}
		// A host or a module path (example.com/a/b.js, golang.org/x/net).
		if host, _, ok := strings.Cut(tok, "/"); ok && strings.Contains(host[1:], ".") {
			continue
		}
		// A directory of the top two levels (internal/domain, pkg/mykit/) is a layer or a component, which a decision
		// may be about; a longer path is where the code is today.
		if i := strings.IndexByte(tok, '/'); i > 0 && a.top[tok[:i]] && strings.Count(strings.TrimSuffix(tok, "/"), "/") >= 2 {
			return "a path"
		}
		if reSourceFile.MatchString(tok) {
			return "a file"
		}
		if a.tasks[tok] {
			return "a task"
		}
	}
	return ""
}

// ADRShape is the measures of one ADR that the shape rule reads.
type ADRShape struct {
	Chars, DecisionChars int
	Artifacts            int // code spans that name an implementation artifact
	TableRows            int // the rows of its longest table
}

// adrShape measures d, whose text is src, and returns the findings of the rule.
func adrShape(d *document, src string, a *artifacts) (ADRShape, []Finding) {
	var s ADRShape
	var out []Finding
	add := func(n int, format string, args ...any) {
		out = append(out, Finding{d.path, n, RuleADRShape, fmt.Sprintf(format, args...)})
	}
	s.Chars = utf8.RuneCountInString(src)
	if s.Chars > ADRMaxChars {
		add(0, "%d characters, over the %d of an ADR: %s; what the code says stays in the code", s.Chars, ADRMaxChars, adrHolds)
	}
	raw := strings.Split(src, "\n")
	decision, rows, table := 0, 0, 0
	endTable := func() {
		if rows-2 > s.TableRows {
			s.TableRows = rows - 2
		}
		if rows-2 > ADRMaxTableRows {
			add(table, "a table of %d rows, over %d: a list of jobs, keys, or tables transcribes the code; keep what was decided about them", rows-2, ADRMaxTableRows)
		}
		rows = 0
	}
	for _, ln := range d.lines {
		if m := reHeading.FindStringSubmatch(ln.shown); !ln.fence && m != nil && len(m[1]) <= 2 {
			if decision > 0 {
				s.DecisionChars = utf8.RuneCountInString(strings.Join(raw[decision:ln.n-1], "\n"))
				decision = 0
			}
			if len(m[1]) == 2 && strings.HasPrefix(strings.ToLower(headingText(m[2])), "decision") {
				decision = ln.n
			}
		}
		if !ln.fence && strings.HasPrefix(strings.TrimSpace(ln.shown), "|") {
			if rows == 0 {
				table = ln.n
			}
			rows++
		} else {
			endTable()
		}
		if ln.fence {
			continue
		}
		for _, m := range reCodeSpan.FindAllStringSubmatch(ln.shown, -1) {
			if k := a.kind(m[1]); k != "" {
				s.Artifacts++
				add(ln.n, "`%s` names %s of the implementation: %s; the names of files, functions, tasks, and keys belong to the code, so name the concept", m[1], k, adrHolds)
			}
		}
	}
	endTable()
	if decision > 0 {
		s.DecisionChars = utf8.RuneCountInString(strings.Join(raw[decision:], "\n"))
	}
	if s.DecisionChars > ADRMaxDecisionChars {
		add(0, "the Decision section has %d characters, over %d: state what was decided and why; how it is built is in the code", s.DecisionChars, ADRMaxDecisionChars)
	}
	return s, out
}
