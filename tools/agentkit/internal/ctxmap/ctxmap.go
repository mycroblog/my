// SPDX-License-Identifier: AGPL-3.0-only

// Package ctxmap reads tools/agentkit/context-map.yaml, the map from the repository's paths to the document
// sections that govern them, the user and operator documents that describe them, the checks they need, and the
// shared files parallel work must not edit at once. Coding agents keep it current: Check fails when a path, or a
// directory under a strict pattern, has no entry, or when a user or operator document describes no entry's paths.
package ctxmap

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mycroblog/my/tools/agentkit/internal/extract"
)

// File is where the map lives, relative to the repository root.
const File = "tools/agentkit/context-map.yaml"

// Map is the parsed context map.
type Map struct {
	// Ignore lists paths no entry needs to cover (generated files, fixtures).
	Ignore []string `yaml:"ignore"`
	// Strict lists directory patterns whose every directory must be named by an entry, so that a new module,
	// adapter or feature gets its own entry with the documents that govern it.
	Strict []string `yaml:"strict"`
	// Userdocs governs the userdocs of the entries.
	Userdocs Userdocs `yaml:"userdocs"`
	Entries  []Entry  `yaml:"entries"`
}

// Userdocs says which documents must describe some entry's paths, and which changed paths never make a
// document stale.
type Userdocs struct {
	// Cover lists the user and operator documents each of which must be the userdocs of some entry.
	Cover []string `yaml:"cover"`
	// Skip lists the changed paths that Impact passes over: tests and their fixtures, which no document describes.
	Skip []string `yaml:"skip"`
}

// Entry describes one area of the repository.
type Entry struct {
	Name  string   `yaml:"name"`
	Paths []string `yaml:"paths"`
	Docs  []string `yaml:"docs"` // <path>#<anchor> or <path>: what an agent reads before changing the area
	// Userdocs are the user and operator documents (<path> or <path>#<anchor>) that describe the behaviour of the
	// area's paths: a change of the paths changes them too, or says why they need no change (Impact).
	Userdocs []string `yaml:"userdocs"`
	Checks   []string `yaml:"checks"` // contexts of `scripts/task.sh check`
	Shared   bool     `yaml:"shared"` // sources many changes edit; never two parallel agents at once (workflow.md §9)
	Notes    []string `yaml:"notes"`  // short rules or steps worth printing in every context pack of the area
}

// Load reads the map of the repository at root.
func Load(root string) (*Map, error) {
	b, err := os.ReadFile(filepath.Join(root, File))
	if err != nil {
		return nil, err
	}
	var m Map
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return &m, nil
}

// Match reports whether the glob pattern matches the slash-separated path p. `*` matches within one segment,
// `**` any number of segments (zero included).
func Match(pattern, p string) bool {
	return matchSegs(strings.Split(pattern, "/"), strings.Split(p, "/"))
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

// For returns the entries whose paths match p.
func (m *Map) For(p string) []Entry {
	var out []Entry
	for _, e := range m.Entries {
		for _, pat := range e.Paths {
			if Match(pat, p) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func (m *Map) ignored(p string) bool {
	return matchAny(m.Ignore, p)
}

// names reports whether some entry names directory d: a pattern whose segments up to d's depth match d
// without `**`, and whose segment at d's last position is d's name, literally.
func (m *Map) names(d string) bool {
	segs := strings.Split(d, "/")
	for _, e := range m.Entries {
		for _, pat := range e.Paths {
			ps := strings.Split(pat, "/")
			if len(ps) < len(segs) || ps[len(segs)-1] != segs[len(segs)-1] {
				continue
			}
			ok := true
			for i := range segs {
				if m, _ := path.Match(ps[i], segs[i]); ps[i] == "**" || !m {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
	}
	return false
}

// Problem is one failure of Check.
type Problem struct {
	Path, Msg string
}

// Check verifies the map against the tracked files: every file is covered or ignored, every directory under
// a strict pattern is named by an entry, every docs and userdocs reference resolves to an existing file and
// heading, every check context exists, and every document of Userdocs.Cover is the userdocs of some entry.
func (m *Map) Check(root string, files []string, contexts []string) []Problem {
	var out []Problem
	dirs := map[string]bool{}
	uncovered := map[string][]string{}
	for _, f := range files {
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
		if m.ignored(f) || len(m.For(f)) > 0 {
			continue
		}
		d := path.Dir(f)
		uncovered[d] = append(uncovered[d], path.Base(f))
	}
	for _, d := range sortedKeys(uncovered) {
		fs := uncovered[d]
		msg := fmt.Sprintf("no entry covers %s", strings.Join(fs[:min(len(fs), 3)], ", "))
		if len(fs) > 3 {
			msg += fmt.Sprintf(" and %d more", len(fs)-3)
		}
		out = append(out, Problem{d + "/", msg})
	}
	for _, pat := range m.Strict {
		for _, d := range sortedKeys(dirs) {
			if Match(pat, d) && !m.ignored(d+"/") && !m.names(d) {
				out = append(out, Problem{d + "/", fmt.Sprintf("matches strict pattern %q but no entry names it; add an entry with the docs that govern it", pat)})
			}
		}
	}
	known := map[string]bool{}
	for _, c := range contexts {
		known[c] = true
	}
	refs := &refChecker{root: root, anchors: map[string]map[string]bool{}}
	described := map[string]bool{}
	for _, e := range m.Entries {
		if e.Name == "" || len(e.Paths) == 0 {
			out = append(out, Problem{File, fmt.Sprintf("entry %q needs a name and paths", e.Name)})
		}
		for _, c := range e.Checks {
			if !known[c] {
				out = append(out, Problem{File, fmt.Sprintf("entry %q: unknown check context %q", e.Name, c)})
			}
		}
		for _, ref := range e.Docs {
			if msg := refs.check(ref); msg != "" {
				out = append(out, Problem{File, fmt.Sprintf("entry %q: %s", e.Name, msg)})
			}
		}
		for _, ref := range e.Userdocs {
			if msg := refs.check(ref); msg != "" {
				out = append(out, Problem{File, fmt.Sprintf("entry %q: userdocs: %s", e.Name, msg)})
			}
			file, _, _ := strings.Cut(ref, "#")
			described[file] = true
		}
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".md") && matchAny(m.Userdocs.Cover, f) && !described[f] {
			out = append(out, Problem{f, "is the userdocs of no entry: add it to the userdocs of the entries whose paths it describes"})
		}
	}
	return out
}

// refChecker resolves the doc references of the map, reading each document's headings once.
type refChecker struct {
	root    string
	anchors map[string]map[string]bool // nil for a file that does not exist
}

// check returns why ref (<path> or <path>#<anchor>) does not resolve, or "".
func (c *refChecker) check(ref string) string {
	file, anchor, _ := strings.Cut(ref, "#")
	a, ok := c.anchors[file]
	if !ok {
		if b, err := os.ReadFile(filepath.Join(c.root, file)); err == nil {
			a = map[string]bool{}
			for _, n := range extract.Markdown(file, b).Nodes {
				if strings.HasPrefix(n.ID, "doc:") {
					a[n.Name] = true
				}
			}
		}
		c.anchors[file] = a
	}
	switch {
	case a == nil:
		return file + " does not exist"
	case anchor != "" && !a[anchor]:
		return fmt.Sprintf("%s has no heading #%s (agentkit outline %s)", file, anchor, file)
	}
	return ""
}

func matchAny(patterns []string, p string) bool {
	for _, pat := range patterns {
		if Match(pat, p) {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
