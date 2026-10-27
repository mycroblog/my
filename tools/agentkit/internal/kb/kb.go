// SPDX-License-Identifier: AGPL-3.0-only

// Package kb answers questions about the repository from the knowledge graph, printing locations and the
// smallest text that answers them: find, show, outline, refs, and trace.
package kb

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/store"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

// KB wraps the store with the presentation rules.
type KB struct {
	S *store.Store
}

// Loc formats a node's location: path:start-end.
func Loc(n store.Node) string {
	if n.Start == n.End {
		return n.Path + ":" + strconv.Itoa(n.Start)
	}
	return n.Path + ":" + strconv.Itoa(n.Start) + "-" + strconv.Itoa(n.End)
}

// Text returns the full text of a node (children included) from the working tree.
func (k *KB) Text(n store.Node) string {
	b, err := os.ReadFile(filepath.Join(k.S.Repo.Root, n.Path))
	if err != nil {
		return ""
	}
	return text.Lines(b, n.Start, n.End)
}

// FileTokens estimates the tokens of the file at p, or -1 when it does not exist.
func (k *KB) FileTokens(p string) int {
	b, err := os.ReadFile(filepath.Join(k.S.Repo.Root, p))
	if err != nil {
		return -1
	}
	return text.Tokens(string(b))
}

// Line prints a node as one line: location, kind, id or title, and its token estimate.
func (k *KB) Line(n store.Node) string {
	label := n.ID
	if n.Title != "" && n.Title != n.ID {
		label += "  " + text.Truncate(text.OneLine(n.Title), 90)
	}
	if n.Kind == "adr" && n.Meta != "" {
		label += "  [" + n.Meta + "]"
	}
	return fmt.Sprintf("%s  %s  %s  (~%s tok)", Loc(n), n.Kind, label, text.Ktok(text.Tokens(k.Text(n))))
}

// Find prints the best matches of q, exact ids and names first.
func (k *KB) Find(w io.Writer, q string, kinds []string, under string, limit int) error {
	seen := map[int64]bool{}
	n := 0
	exact, err := k.Resolve(q)
	if err != nil {
		return err
	}
	for _, x := range exact {
		if n >= limit {
			break
		}
		seen[x.RowID] = true
		fmt.Fprintln(w, k.Line(x))
		n++
	}
	hits, err := k.S.Search(q, kinds, under, limit*2)
	if err != nil {
		return err
	}
	for _, h := range hits {
		if n >= limit {
			break
		}
		if seen[h.RowID] {
			continue
		}
		seen[h.RowID] = true
		fmt.Fprintln(w, k.Line(h.Node))
		if s := text.OneLine(h.Snippet); s != "" {
			fmt.Fprintln(w, "    "+text.Truncate(s, 140))
		}
		n++
	}
	if n == 0 {
		fmt.Fprintln(w, "no match")
	}
	return nil
}

var (
	reLineRange = regexp.MustCompile(`^(.+?):(\d+)(?:-(\d+))?$`)
	reSection   = regexp.MustCompile(`^([\w./-]+?)(?:\.md)?\s*[§#]\s*(\S+)$`)
)

// Resolve turns a reference into nodes. It accepts node ids (ADR-0070, FR-915, op:x, table:x, sym:..., doc:...),
// ADR-0070#decision (the first section of the ADR whose anchor starts with "decision"),
// <file>§4.4 or <file>#<anchor> with any unique suffix of a Markdown path or of a directory of section files
// (data-model§4.4), short names (DeleteOwnAccount, custom_emojis), and the name of a directory of section files
// (web-client), which resolves to its README.md.
func (k *KB) Resolve(ref string) ([]store.Node, error) {
	ref = strings.TrimSpace(ref)
	if ns, err := k.S.Get(ref); err != nil || len(ns) > 0 {
		return ns, err
	}
	for _, prefix := range []string{"table:", "query:", "op:", "schema:"} {
		if ns, err := k.S.Get(prefix + ref); err != nil || len(ns) > 0 {
			return ns, err
		}
	}
	if m := reSection.FindStringSubmatch(ref); m != nil {
		return k.section(m[1], m[2])
	}
	if ns, err := k.S.ByName(ref); err != nil || len(ns) > 0 {
		return ns, err
	}
	if strings.HasPrefix(ref, "sym:") || strings.HasPrefix(ref, "doc:") || strings.HasPrefix(ref, "ts:") {
		return k.S.ByIDPrefix(ref, 20)
	}
	return k.splitDoc(ref)
}

// section resolves file§num or file#anchor; file may be an ADR id, a path suffix, or a directory holding a
// document split into one file per section (web-client§14.2 looks through docs/architecture/web-client/, and
// data-model§4.4 through docs/architecture/data-model/ and its subdirectories).
func (k *KB) section(file, sec string) ([]store.Node, error) {
	var paths []string
	if strings.HasPrefix(file, "ADR-") {
		ns, err := k.S.Get(file)
		if err != nil || len(ns) == 0 {
			return nil, err
		}
		paths = []string{ns[0].Path}
	} else {
		ps, err := k.S.Paths(file + ".md")
		if err != nil {
			return nil, err
		}
		if len(ps) == 0 {
			ps, err = k.S.Paths(file)
			if err != nil {
				return nil, err
			}
		}
		if len(ps) > 0 {
			paths = ps[:1]
		} else if paths, _, err = k.S.DocDir(file); err != nil {
			return nil, err
		}
	}
	sec = strings.TrimPrefix(sec, "§")
	var prefix []store.Node
	for _, p := range paths {
		ns, err := k.S.FileNodes(p)
		if err != nil {
			return nil, err
		}
		for _, n := range ns {
			if !strings.HasPrefix(n.ID, "doc:") {
				continue
			}
			if n.Meta == "§"+sec || n.Name == sec {
				return []store.Node{n}, nil
			}
			if strings.HasPrefix(n.Name, strings.ToLower(sec)) {
				prefix = append(prefix, n)
			}
		}
	}
	if len(prefix) > 0 {
		// The shortest anchor is the closest: decision決定 before decision-drivers判断基準.
		sort.SliceStable(prefix, func(i, j int) bool { return len(prefix[i].Name) < len(prefix[j].Name) })
		return prefix[:1], nil
	}
	return nil, nil
}

// splitDoc resolves the name of a document split into one file per section (web-client, data-model.md,
// docs/architecture/web-client/) to its README.md.
func (k *KB) splitDoc(ref string) ([]store.Node, error) {
	ps, dir, err := k.S.DocDir(strings.TrimSuffix(ref, ".md"))
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p != dir+"/README.md" {
			continue
		}
		ns, err := k.S.FileNodes(p)
		if err != nil || len(ns) == 0 {
			return nil, err
		}
		return ns[:1], nil
	}
	return nil, nil
}

// Show prints what ref points at: a line range, or the text of the node it resolves to. A node bigger than
// limit tokens prints its outline instead, so that the caller picks the part it needs.
func (k *KB) Show(w io.Writer, ref string, limit int) error {
	if m := reLineRange.FindStringSubmatch(ref); m != nil {
		if full, err := k.contained(m[1]); err == nil {
			b, err := os.ReadFile(full)
			if err != nil {
				return err
			}
			s, _ := strconv.Atoi(m[2])
			e := s
			if m[3] != "" {
				e, _ = strconv.Atoi(m[3])
			}
			fmt.Fprint(w, text.Lines(b, s, e))
			return nil
		}
	}
	ns, err := k.Resolve(ref)
	if err != nil {
		return err
	}
	if len(ns) == 0 {
		if _, err := k.contained(ref); err == nil {
			return k.showFile(w, ref, limit)
		}
		return fmt.Errorf("nothing matches %q; try `agentkit find %s`", ref, ref)
	}
	if len(ns) > 1 && ns[0].Kind != "table" {
		fmt.Fprintf(w, "%d matches; show one by id or location:\n", len(ns))
		for _, n := range ns[:min(len(ns), 20)] {
			fmt.Fprintln(w, k.Line(n))
		}
		return nil
	}
	for _, n := range ns {
		body := k.Text(n)
		tok := text.Tokens(body)
		fmt.Fprintf(w, "== %s  %s  (~%s tok)\n", Loc(n), n.ID, text.Ktok(tok))
		if n.Kind == "adr" && n.Meta != "" {
			fmt.Fprintf(w, "status: %s\n", n.Meta)
		}
		if tok > limit {
			fmt.Fprintf(w, "too big to print (max %s); its parts:\n", text.Ktok(limit))
			return k.outlineRange(w, n.Path, n.Start, n.End)
		}
		fmt.Fprint(w, body)
		if !strings.HasSuffix(body, "\n") {
			fmt.Fprintln(w)
		}
	}
	return nil
}

// contained returns the absolute path of the existing file p (relative to the repository root) and refuses a
// path that leaves the root, by `..`, an absolute path, or a symlink.
func (k *KB) contained(p string) (string, error) {
	if !filepath.IsLocal(p) {
		return "", fmt.Errorf("%q is outside the repository", p)
	}
	root, err := filepath.EvalSymlinks(k.S.Repo.Root)
	if err != nil {
		return "", err
	}
	full, err := filepath.EvalSymlinks(filepath.Join(k.S.Repo.Root, p))
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(root, full); err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%q is outside the repository", p)
	}
	return full, nil
}

func (k *KB) showFile(w io.Writer, p string, limit int) error {
	full, err := k.contained(p)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	if tok := text.Tokens(string(b)); tok > limit {
		fmt.Fprintf(w, "== %s  (~%s tok, too big to print; max %s)\n", p, text.Ktok(tok), text.Ktok(limit))
		return k.Outline(w, p)
	}
	_, err = w.Write(b)
	return err
}

// Outline prints the parts of a file with their locations and sizes: the headings of a document, the
// declarations of code.
func (k *KB) Outline(w io.Writer, p string) error {
	return k.outlineRange(w, p, 0, 1<<30)
}

func (k *KB) outlineRange(w io.Writer, p string, start, end int) error {
	ns, err := k.S.FileNodes(p)
	if err != nil {
		return err
	}
	if len(ns) == 0 {
		return fmt.Errorf("%s is not indexed", p)
	}
	b, _ := os.ReadFile(filepath.Join(k.S.Repo.Root, p))
	for _, n := range ns {
		if n.Start < start || n.End > end || strings.HasPrefix(n.ID, "ADR-") || n.Kind == "requirement" {
			continue
		}
		indent := ""
		if strings.HasPrefix(n.ID, "doc:") {
			level := strings.Count(strings.SplitN(text.Lines(b, n.Start, n.Start), " ", 2)[0], "#")
			indent = strings.Repeat("  ", max(level-1, 0))
		}
		id := n.ID
		if strings.HasPrefix(id, "doc:") {
			id = "#" + n.Name
		}
		fmt.Fprintf(w, "%s%d-%d  %s  %s  (~%s tok)\n", indent, n.Start, n.End, text.Truncate(text.OneLine(n.Title), 80), id, text.Ktok(text.Tokens(text.Lines(b, n.Start, n.End))))
	}
	return nil
}

// Refs prints what points at ref and what ref points at.
func (k *KB) Refs(w io.Writer, ref string, limit int) error {
	ns, err := k.Resolve(ref)
	if err != nil {
		return err
	}
	ids := []string{ref}
	names := map[string]bool{}
	for _, n := range ns {
		ids = append(ids, n.ID)
		names[n.Name] = true
	}
	for name := range names {
		ids = append(ids, "name:"+name, "ident:"+name)
	}
	for _, n := range ns {
		if n.Kind == "op" {
			if p := k.S.APIPath(n.Path); p != "" {
				ids = append(ids, "api:"+n.Meta+" "+p)
			}
		}
		if n.Kind == "file" && strings.HasSuffix(n.Path, ".go") {
			ids = append(ids, "pkg:"+path.Dir(n.Path))
		}
		if n.Kind == "file" || strings.HasPrefix(n.ID, "ts:") {
			// TypeScript imports name a module without its extension, or its directory for index.ts.
			noExt := strings.TrimSuffix(n.Path, path.Ext(n.Path))
			ids = append(ids, "file:"+n.Path, "file:"+noExt)
			if path.Base(noExt) == "index" {
				ids = append(ids, "file:"+path.Dir(noExt))
			}
		}
	}
	in, err := k.S.In(text.Uniq(ids)...)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== referenced by (%d)\n", len(in))
	k.printEdges(w, in, limit, true)
	var out []store.Edge
	for _, n := range ns {
		es, err := k.S.Out(n.ID)
		if err != nil {
			return err
		}
		out = append(out, es...)
	}
	fmt.Fprintf(w, "== references (%d)\n", len(out))
	k.printEdges(w, out, limit, false)
	return nil
}

func (k *KB) printEdges(w io.Writer, es []store.Edge, limit int, incoming bool) {
	seen := map[string]bool{}
	var lines []string
	for _, e := range es {
		var l string
		if incoming {
			if n, ok := k.S.Locate(e); ok {
				l = fmt.Sprintf("%s  %s  %s", e.Rel, Loc(n), n.ID)
			} else {
				l = fmt.Sprintf("%s  %s  %s", e.Rel, e.Path, e.Src)
			}
		} else {
			l = e.Rel + "  " + e.Dst
		}
		if !seen[l] {
			seen[l] = true
			lines = append(lines, l)
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return rank(lines[i]) < rank(lines[j]) })
	for i, l := range lines {
		if i == limit {
			fmt.Fprintf(w, "  … %d more (raise --limit)\n", len(lines)-limit)
			break
		}
		fmt.Fprintln(w, "  "+l)
	}
}

// rank puts documents and production code before tests.
func rank(l string) int {
	switch {
	case strings.Contains(l, "_test.go") || strings.Contains(l, ".test.ts"):
		return 2
	case strings.Contains(l, ".md:"):
		return 0
	}
	return 1
}
