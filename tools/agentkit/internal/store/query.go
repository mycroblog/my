// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"strings"
	"unicode/utf8"
)

// Node is a node of the view with the file it lives in.
type Node struct {
	RowID  int64
	ID     string
	Name   string
	Kind   string
	Title  string
	Path   string
	Start  int
	OwnEnd int
	End    int
	Size   int
	Meta   string
}

const nodeCols = `n.id, n.nid, n.name, n.kind, n.title, f.path, n.start, n.own_end, n.end_line, n.size, n.meta`
const nodeFrom = ` FROM nodes n JOIN files f ON f.id = n.file_id WHERE n.file_id IN (SELECT file_id FROM view)`

func scanNodes(rows *sql.Rows, err error) ([]Node, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.RowID, &n.ID, &n.Name, &n.Kind, &n.Title, &n.Path, &n.Start, &n.OwnEnd, &n.End, &n.Size, &n.Meta); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Get returns the nodes with id nid (a table has a node per dialect).
func (s *Store) Get(nid string) ([]Node, error) {
	return scanNodes(s.DB.Query(`SELECT `+nodeCols+nodeFrom+` AND n.nid = ? ORDER BY f.path`, nid))
}

// ByName returns the nodes named name.
func (s *Store) ByName(name string) ([]Node, error) {
	return scanNodes(s.DB.Query(`SELECT `+nodeCols+nodeFrom+` AND n.name = ? ORDER BY f.path, n.start`, name))
}

// ByIDPrefix returns the nodes whose id starts with prefix.
func (s *Store) ByIDPrefix(prefix string, limit int) ([]Node, error) {
	return scanNodes(s.DB.Query(`SELECT `+nodeCols+nodeFrom+` AND n.nid >= ? AND n.nid < ? ORDER BY n.nid LIMIT ?`, prefix, prefix+"￿", limit))
}

// Qualified returns the Go declarations whose id ends with the qualified name q: a method by its receiver
// (Service.Create), a declaration by its package (content.NewService), or both (content.Service.Create).
func (s *Store) Qualified(q string) ([]Node, error) {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	return scanNodes(s.DB.Query(`SELECT `+nodeCols+nodeFrom+` AND n.nid LIKE 'sym:%' AND (n.nid LIKE ? ESCAPE '\' OR n.nid LIKE ? ESCAPE '\' OR n.nid = ?) ORDER BY f.path, n.start LIMIT 50`,
		"%."+esc, "%/"+esc, "sym:"+q))
}

// FileNodes returns the nodes of the file at p in line order.
func (s *Store) FileNodes(p string) ([]Node, error) {
	return scanNodes(s.DB.Query(`SELECT `+nodeCols+nodeFrom+` AND f.path = ? ORDER BY n.start, n.end_line DESC`, p))
}

// Paths returns the indexed paths of the view that end with suffix.
func (s *Store) Paths(suffix string) ([]string, error) {
	rows, err := s.DB.Query(`SELECT path FROM files WHERE id IN (SELECT file_id FROM view) AND path LIKE ? ORDER BY length(path)`, "%"+suffix)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DocDir returns the Markdown files in and below the shortest indexed directory whose path ends with dir (a
// document split into one file per section, such as docs/architecture/data-model/ and its 04-tables/), in
// path order, and that directory.
func (s *Store) DocDir(dir string) ([]string, string, error) {
	dir = strings.Trim(dir, "/")
	rows, err := s.DB.Query(`SELECT path FROM files WHERE id IN (SELECT file_id FROM view) AND (path LIKE ? OR path LIKE ?) ORDER BY path`, "%/"+dir+"/%.md", dir+"/%.md")
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()
	var all []string
	best := ""
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, "", err
		}
		all = append(all, p)
		// The directory is the part of p up to the match of dir: a/dir/b/c.md is under a/dir.
		d := ""
		if strings.HasPrefix(p, dir+"/") {
			d = dir
		} else if i := strings.Index(p, "/"+dir+"/"); i >= 0 {
			d = p[:i+1+len(dir)]
		}
		if d != "" && (best == "" || len(d) < len(best)) {
			best = d
		}
	}
	var out []string
	for _, p := range all {
		if strings.HasPrefix(p, best+"/") {
			out = append(out, p)
		}
	}
	return out, best, rows.Err()
}

// Edge is an edge of the view.
type Edge struct {
	Src, Dst, Rel, Path string
}

func scanEdges(rows *sql.Rows, err error) ([]Edge, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Dst, &e.Rel, &e.Path); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

const edgeFrom = ` FROM edges e JOIN files f ON f.id = e.file_id WHERE e.file_id IN (SELECT file_id FROM view)`

// Out returns the edges leaving src.
func (s *Store) Out(src string) ([]Edge, error) {
	return scanEdges(s.DB.Query(`SELECT e.src, e.dst, e.rel, f.path`+edgeFrom+` AND e.src = ?`, src))
}

// In returns the edges arriving at any of dsts.
func (s *Store) In(dsts ...string) ([]Edge, error) {
	if len(dsts) == 0 {
		return nil, nil
	}
	args := make([]any, len(dsts))
	for i, d := range dsts {
		args[i] = d
	}
	q := `SELECT e.src, e.dst, e.rel, f.path` + edgeFrom + ` AND e.dst IN (?` + strings.Repeat(",?", len(dsts)-1) + `) ORDER BY f.path`
	return scanEdges(s.DB.Query(q, args...))
}

// Locate returns the node an edge leaves from: the node with id src in the edge's file.
func (s *Store) Locate(e Edge) (Node, bool) {
	ns, err := scanNodes(s.DB.Query(`SELECT `+nodeCols+nodeFrom+` AND n.nid = ? AND f.path = ? LIMIT 1`, e.Src, e.Path))
	if err != nil || len(ns) == 0 {
		return Node{}, false
	}
	return ns[0], true
}

// APIPath returns the client API path the OpenAPI path file at p serves, or "".
func (s *Store) APIPath(p string) string {
	es, err := s.Out("file:" + p)
	if err != nil {
		return ""
	}
	for _, e := range es {
		if e.Rel == "serves" {
			return strings.TrimPrefix(e.Dst, "apipath:")
		}
	}
	return ""
}

// Ops returns the operations that serve method and apiPath.
func (s *Store) Ops(method, apiPath string) ([]Node, error) {
	es, err := s.In("apipath:" + apiPath)
	if err != nil {
		return nil, err
	}
	var out []Node
	for _, e := range es {
		ns, err := s.FileNodes(strings.TrimPrefix(e.Src, "file:"))
		if err != nil {
			return nil, err
		}
		for _, n := range ns {
			if n.Kind == "op" && n.Meta == method {
				out = append(out, n)
			}
		}
	}
	return out, nil
}

// Hit is a search result.
type Hit struct {
	Node
	Snippet string
}

// kindWeight ranks kinds of nodes for search: decisions and requirements first, tests and messages last.
const kindWeight = `CASE n.kind WHEN 'requirement' THEN 1.5 WHEN 'adr' THEN 1.4 WHEN 'adr-section' THEN 1.3
  WHEN 'section' THEN 1.2 WHEN 'op' THEN 1.2 WHEN 'table' THEN 1.2 WHEN 'query' THEN 1.0 WHEN 'schema' THEN 0.9
  WHEN 'go' THEN 1.0 WHEN 'ts' THEN 1.0 WHEN 'file' THEN 0.7 WHEN 'msg' THEN 0.5 ELSE 0.6 END`

// Search ranks the nodes whose text contains every word of q. Words of three characters or more go through
// the trigram index; shorter ones (two-character Japanese words) are matched by LIKE.
func (s *Store) Search(q string, kinds []string, pathPrefix string, limit int) ([]Hit, error) {
	var long, short []string
	for _, w := range strings.Fields(q) {
		if utf8.RuneCountInString(w) >= 3 {
			long = append(long, `"`+strings.ReplaceAll(w, `"`, `""`)+`"`)
		} else {
			short = append(short, w)
		}
	}
	if len(long) == 0 && len(short) == 0 {
		return nil, nil
	}
	var where []string
	var args []any
	score := `0`
	snip := `substr(fts.body, 1, 80)`
	if len(long) > 0 {
		where = append(where, `fts MATCH ?`)
		args = append(args, strings.Join(long, " "))
		score = `bm25(fts, 4.0, 1.0)`
		snip = `snippet(fts, 1, '«', '»', '…', 12)`
	}
	for _, w := range short {
		where = append(where, `(fts.title LIKE ? OR fts.body LIKE ?)`)
		args = append(args, "%"+w+"%", "%"+w+"%")
	}
	if len(kinds) > 0 {
		where = append(where, `n.kind IN (?`+strings.Repeat(",?", len(kinds)-1)+`)`)
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	if pathPrefix != "" {
		where = append(where, `f.path LIKE ?`)
		args = append(args, pathPrefix+"%")
	}
	args = append(args, limit)
	//nolint:gosec // G202: query is constant fragments and placeholders only
	query := `SELECT ` + nodeCols + `, ` + snip + ` FROM fts JOIN nodes n ON n.id = fts.rowid JOIN files f ON f.id = n.file_id
WHERE n.file_id IN (SELECT file_id FROM view) AND ` + strings.Join(where, " AND ") + `
ORDER BY ` + score + ` * ` + kindWeight + `, -(` + kindWeight + `) LIMIT ?`
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.RowID, &h.ID, &h.Name, &h.Kind, &h.Title, &h.Path, &h.Start, &h.OwnEnd, &h.End, &h.Size, &h.Meta, &h.Snippet); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
