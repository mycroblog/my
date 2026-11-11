// SPDX-License-Identifier: AGPL-3.0-only

// Package ctxmap reads the context-map.yaml files. Each describes its directory as a whole, and what is below it
// down to the next map file: the document sections that govern it, the user and operator documents that describe
// it, the checks it needs, and whether parallel work treats it as shared files. The docs, userdocs and checks of
// a map add to those of the maps above it; shared is the nearest map's alone. Check fails when a path is under
// no map, when a map repeats a map above it, or when a user or operator document describes no map's directory.
package ctxmap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mycroblog/my/tools/agentkit/internal/extract"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// File is the name of a map file. The one at the repository root holds the global settings and the entries of
// the root's files; every other one describes its directory.
const File = "context-map.yaml"

// whole is why a map file takes nothing but docs, userdocs, checks and shared.
const whole = "a context map describes its directory as a whole (docs, userdocs, checks, shared); what governs one file is said by the code and the check that enforces it"

// Map is the parsed context map: the root file's settings and entries, and the map of every directory.
type Map struct {
	// Ignore lists paths no map needs to cover (generated files, fixtures).
	Ignore []string
	// Userdocs governs the userdocs of the maps.
	Userdocs Userdocs
	// Root are the entries of the root file, for the root's files and dot-directories: the first whose Paths
	// match a path under no directory's map describes it.
	Root []Entry
	// Dirs are the maps as written, by directory.
	Dirs map[string]Entry
}

// Userdocs says which documents must describe some map's directory, and which changed paths never make a
// document stale.
type Userdocs struct {
	// Cover lists the user and operator documents each of which must be the userdocs of some map.
	Cover []string `yaml:"cover"`
	// Skip lists the changed paths that Impact passes over: tests and their fixtures, which no document describes.
	Skip []string `yaml:"skip"`
}

// Entry describes one area of the repository: a directory, or the root files of one entry of the root file.
type Entry struct {
	// File is the map file that describes the area, and Name what names it in output: the directory, or the
	// first pattern of an entry of the root file.
	File, Name string `yaml:"-"`
	// Paths are the patterns of an entry of the root file, from the repository root; a directory's map has none.
	Paths []string `yaml:"paths"`
	// Docs are <path>#<anchor> or <path> from the repository root: what an agent reads before changing the area.
	Docs []string `yaml:"docs"`
	// Userdocs are the user and operator documents (<path> or <path>#<anchor>) that describe the behaviour of the
	// area (Impact).
	Userdocs []string `yaml:"userdocs"`
	Checks   []string `yaml:"checks"` // contexts of `scripts/task.sh check`
	Shared   bool     `yaml:"shared"` // sources many changes edit; never two parallel agents at once (workflow.md §9)
}

// dirFile is the map file of a directory as written.
type dirFile struct {
	Docs     []string `yaml:"docs"`
	Userdocs []string `yaml:"userdocs"`
	Checks   []string `yaml:"checks"`
	Shared   bool     `yaml:"shared"`
}

// rootFile is the root map file as written.
type rootFile struct {
	Ignore   []string `yaml:"ignore"`
	Userdocs Userdocs `yaml:"userdocs"`
	Entries  []Entry  `yaml:"entries"`
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
	m := &Map{Dirs: map[string]Entry{}}
	hasRoot := false
	for _, f := range files {
		if path.Base(f) != File {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return nil, err
		}
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		if f == File {
			hasRoot = true
			if err := m.addRoot(dec); err != nil {
				return nil, err
			}
			continue
		}
		var raw dirFile
		if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: %w: %s", f, err, whole)
		}
		dir := path.Dir(f)
		if len(raw.Docs)+len(raw.Userdocs)+len(raw.Checks) == 0 && !raw.Shared {
			return nil, fmt.Errorf("%s: says nothing: delete the file, and the map above describes %s/", f, dir)
		}
		m.Dirs[dir] = Entry{File: f, Name: dir, Docs: raw.Docs, Userdocs: raw.Userdocs, Checks: raw.Checks, Shared: raw.Shared}
	}
	if !hasRoot {
		return nil, errors.New(File + ": the repository root has no map file (ignore, userdocs, and the entries of the root's files)")
	}
	return m, nil
}

// addRoot parses the root map file into m.
func (m *Map) addRoot(dec *yaml.Decoder) error {
	var raw rootFile
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: %w (the root file has ignore, userdocs.cover/skip, and entries with paths, docs, userdocs, checks, shared)", File, err)
	}
	m.Ignore, m.Userdocs = raw.Ignore, raw.Userdocs
	for i, e := range raw.Entries {
		if len(e.Paths) == 0 {
			return fmt.Errorf("%s: entry %d: no paths", File, i+1)
		}
		for _, p := range e.Paths {
			if strings.Contains(p, "/") && !strings.HasPrefix(p, ".") {
				return fmt.Errorf("%s: entry %d: path %q: the root file names the root's files and dot-directories only; %s", File, i+1, p, whole)
			}
		}
		e.File, e.Name = File, e.Paths[0]
		m.Root = append(m.Root, e)
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

// above returns the nearest directory at or above dir that holds a map, or "".
func (m *Map) above(dir string) string {
	for ; dir != "."; dir = path.Dir(dir) {
		if _, ok := m.Dirs[dir]; ok {
			return dir
		}
	}
	return ""
}

// For returns what describes p, a file or a directory: the nearest map at or above it, with the docs, userdocs
// and checks of the maps above that one added after its own. A path under no map is described by the first
// entry of the root file that matches it.
func (m *Map) For(p string) (Entry, bool) {
	dir := m.above(path.Clean(p))
	if dir == "" {
		for _, e := range m.Root {
			if matchAny(e.Paths, p) {
				return e, true
			}
		}
		return Entry{}, false
	}
	out := Entry{File: m.Dirs[dir].File, Name: dir, Shared: m.Dirs[dir].Shared}
	for ; dir != ""; dir = m.above(path.Dir(dir)) {
		e := m.Dirs[dir]
		out.Docs = appendNew(out.Docs, e.Docs...)
		out.Userdocs = appendNew(out.Userdocs, e.Userdocs...)
		out.Checks = appendNew(out.Checks, e.Checks...)
	}
	return out, true
}

func (m *Map) ignored(p string) bool {
	return matchAny(m.Ignore, p)
}

// Problem is one failure of Check.
type Problem struct {
	Path, Msg string
}

// Check verifies the map against the tracked files: every file is under a map, matches an entry of the root
// file, or is ignored; no map repeats what a map above it says; every docs and userdocs reference resolves to
// an existing file and heading; every check context exists; and every document of Userdocs.Cover is the
// userdocs of some map.
func (m *Map) Check(root string, files []string, contexts []string) []Problem {
	var out []Problem
	uncovered := map[string][]string{}
	for _, f := range files {
		if path.Base(f) == File || m.ignored(f) {
			continue
		}
		if _, ok := m.For(f); !ok {
			d := path.Dir(f)
			uncovered[d] = append(uncovered[d], path.Base(f))
		}
	}
	for _, d := range sortedKeys(uncovered) {
		fs := uncovered[d]
		msg := fmt.Sprintf("no map describes %s", strings.Join(fs[:min(len(fs), 3)], ", "))
		if len(fs) > 3 {
			msg += fmt.Sprintf(" and %d more", len(fs)-3)
		}
		fix := "name them in an entry of the root " + File
		if d != "." && !strings.HasPrefix(d, ".") {
			top, _, _ := strings.Cut(d, "/")
			fix = fmt.Sprintf("add %s/%s with what holds for the whole directory (docs, userdocs, checks, shared)", top, File)
		}
		out = append(out, Problem{d + "/", msg + "; " + fix})
	}
	known := map[string]bool{}
	for _, c := range contexts {
		known[c] = true
	}
	entries := slices.Clone(m.Root)
	for _, d := range sortedKeys(m.Dirs) {
		e := m.Dirs[d]
		entries = append(entries, e)
		if m.above(path.Dir(d)) == "" {
			continue
		}
		up, _ := m.For(path.Dir(d))
		for _, x := range slices.Concat(e.Docs, e.Userdocs, e.Checks) {
			if slices.Contains(slices.Concat(up.Docs, up.Userdocs, up.Checks), x) {
				out = append(out, Problem{e.File, fmt.Sprintf("repeats %q, which a %s above it says for this directory too: delete it here", x, File)})
			}
		}
	}
	refs := &refChecker{root: root, anchors: map[string]map[string]bool{}}
	described := map[string]bool{}
	for _, e := range entries {
		for _, c := range e.Checks {
			if !known[c] {
				out = append(out, Problem{e.File, fmt.Sprintf("%s: unknown check context %q (one of %s)", e.Name, c, strings.Join(contexts, ", "))})
			}
		}
		for _, ref := range e.Docs {
			if msg := refs.check(ref); msg != "" {
				out = append(out, Problem{e.File, fmt.Sprintf("%s: %s", e.Name, msg)})
			}
		}
		for _, ref := range e.Userdocs {
			if msg := refs.check(ref); msg != "" {
				out = append(out, Problem{e.File, fmt.Sprintf("%s: userdocs: %s", e.Name, msg)})
			}
			file, _, _ := strings.Cut(ref, "#")
			described[file] = true
		}
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".md") && matchAny(m.Userdocs.Cover, f) && !described[f] {
			out = append(out, Problem{f, "is the userdocs of no map: name it in the " + File + " of the directory whose behaviour it describes"})
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

// appendNew appends to list the strings it does not hold yet.
func appendNew(list []string, ss ...string) []string {
	for _, s := range ss {
		if !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	return list
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
