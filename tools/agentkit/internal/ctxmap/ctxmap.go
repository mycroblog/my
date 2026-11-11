// SPDX-License-Identifier: AGPL-3.0-only

// Package ctxmap reads the context-map.yaml files, each in the directory it describes: the map from the
// repository's paths to the document sections that govern them, the user and operator documents that describe
// them, the checks they need, and the shared files parallel work must not edit at once. Check fails when a path
// has no entry, when a directory under a strict pattern has no file of its own, or when a user or operator
// document describes no entry's paths.
package ctxmap

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mycroblog/my/tools/agentkit/internal/extract"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// File is the name of a map file. The one at the repository root holds the global settings; every other one
// holds the entries of its directory.
const File = "context-map.yaml"

// Map is the parsed context map: the root file's settings and the entries of every file.
type Map struct {
	// Ignore lists paths no entry needs to cover (generated files, fixtures).
	Ignore []string
	// Strict lists directory patterns whose every directory holds a map file of its own, so that a new module,
	// adapter or feature adds one file inside it and edits no other. A directory whose name starts with a dot
	// is exempt.
	Strict []string
	// Userdocs governs the userdocs of the entries.
	Userdocs Userdocs
	Entries  []Entry
	// dirs are the directories that hold a map file ("." for the root).
	dirs map[string]bool
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
	// File is the map file that holds the entry, and Name what names it in output: the file's directory, with
	// the entry's first pattern when the file holds several.
	File, Name string
	Paths      []string // patterns from the repository root (a file writes them relative to its directory)
	Docs       []string // <path>#<anchor> or <path> from the repository root: what an agent reads before changing the area
	// Userdocs are the user and operator documents (<path> or <path>#<anchor>) that describe the behaviour of the
	// area's paths (Impact).
	Userdocs []string
	Checks   []string // contexts of `scripts/task.sh check`
	Shared   bool     // sources many changes edit; never two parallel agents at once (workflow.md §9)
}

// file is one map file as written.
type file struct {
	Ignore   []string  `yaml:"ignore"`
	Strict   []string  `yaml:"strict"`
	Userdocs *Userdocs `yaml:"userdocs"`
	Entries  []struct {
		Paths    []string  `yaml:"paths"`
		Docs     []string  `yaml:"docs"`
		Userdocs []string  `yaml:"userdocs"`
		Checks   []string  `yaml:"checks"`
		Shared   bool      `yaml:"shared"`
		Notes    yaml.Node `yaml:"notes"`
	} `yaml:"entries"`
}

// Of reads the map of the repository r from its tracked and untracked files.
func Of(r *repo.Repo) (*Map, error) {
	files, err := r.Files()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return Load(r.Root, paths)
}

// Load reads the map files among files, the paths of the repository at root.
func Load(root string, files []string) (*Map, error) {
	m := &Map{dirs: map[string]bool{}}
	var maps []string
	for _, f := range files {
		if path.Base(f) == File {
			maps = append(maps, f)
		}
	}
	sort.Strings(maps)
	for _, f := range maps {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return nil, err
		}
		if err := m.add(f, b); err != nil {
			return nil, err
		}
	}
	if !m.dirs["."] {
		return nil, errors.New(File + ": the repository root has no map file (ignore, strict, userdocs, and the entries of the root's files)")
	}
	return m, nil
}

// add parses the map file f with content b into m.
func (m *Map) add(f string, b []byte) error {
	var raw file
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return fmt.Errorf("%s: %w (an entry has paths, docs, userdocs, checks, shared)", f, err)
	}
	dir := path.Dir(f)
	if dir != "." && (raw.Ignore != nil || raw.Strict != nil || raw.Userdocs != nil) {
		return fmt.Errorf("%s: ignore, strict and userdocs.cover/skip are settings of the root %s; this file holds entries only", f, File)
	}
	if len(raw.Entries) == 0 {
		return fmt.Errorf("%s: no entries; give the paths of %s/ an entry, or delete the file", f, dir)
	}
	m.dirs[dir] = true
	if dir == "." {
		m.Ignore, m.Strict = raw.Ignore, raw.Strict
		if raw.Userdocs != nil {
			m.Userdocs = *raw.Userdocs
		}
	}
	for i, e := range raw.Entries {
		if !e.Notes.IsZero() {
			return fmt.Errorf("%s: entry %d: notes are not part of the map: a rule lives in the check that enforces it and in that check's failure message", f, i+1)
		}
		if len(e.Paths) == 0 {
			return fmt.Errorf("%s: entry %d: no paths", f, i+1)
		}
		out := Entry{File: f, Name: dir, Docs: e.Docs, Userdocs: e.Userdocs, Checks: e.Checks, Shared: e.Shared}
		if len(raw.Entries) > 1 {
			out.Name = dir + " (" + e.Paths[0] + ")"
		}
		for _, p := range e.Paths {
			for _, seg := range strings.Split(p, "/") {
				if seg == "" || seg == "." || seg == ".." {
					return fmt.Errorf("%s: entry %d: path %q: patterns are relative to %s/ and stay inside it", f, i+1, p, dir)
				}
			}
			out.Paths = append(out.Paths, path.Join(dir, p))
		}
		m.Entries = append(m.Entries, out)
	}
	return nil
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

// Problem is one failure of Check.
type Problem struct {
	Path, Msg string
}

// Check verifies the map against the tracked files: every file is covered or ignored, every directory under
// a strict pattern holds a map file, every docs and userdocs reference resolves to an existing file and
// heading, every check context exists, and every document of Userdocs.Cover is the userdocs of some entry.
func (m *Map) Check(root string, files []string, contexts []string) []Problem {
	var out []Problem
	dirs := map[string]bool{}
	uncovered := map[string][]string{}
	for _, f := range files {
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
		if path.Base(f) == File || m.ignored(f) || len(m.For(f)) > 0 {
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
		out = append(out, Problem{d + "/", msg + "; add the paths to an entry of the nearest " + File + " at or above it"})
	}
	for _, pat := range m.Strict {
		for _, d := range sortedKeys(dirs) {
			if Match(pat, d) && !strings.HasPrefix(path.Base(d), ".") && !m.ignored(d+"/") && !m.dirs[d] {
				out = append(out, Problem{d + "/", fmt.Sprintf("matches strict pattern %q: add %s/%s with an entry for its paths (paths relative to the directory; docs, userdocs, checks, shared)", pat, d, File)})
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
		for _, c := range e.Checks {
			if !known[c] {
				out = append(out, Problem{e.File, fmt.Sprintf("entry %q: unknown check context %q (one of %s)", e.Name, c, strings.Join(contexts, ", "))})
			}
		}
		for _, ref := range e.Docs {
			if msg := refs.check(ref); msg != "" {
				out = append(out, Problem{e.File, fmt.Sprintf("entry %q: %s", e.Name, msg)})
			}
		}
		for _, ref := range e.Userdocs {
			if msg := refs.check(ref); msg != "" {
				out = append(out, Problem{e.File, fmt.Sprintf("entry %q: userdocs: %s", e.Name, msg)})
			}
			file, _, _ := strings.Cut(ref, "#")
			described[file] = true
		}
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".md") && matchAny(m.Userdocs.Cover, f) && !described[f] {
			out = append(out, Problem{f, "is the userdocs of no entry: name it in the userdocs of the " + File + " entries whose paths it describes"})
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
