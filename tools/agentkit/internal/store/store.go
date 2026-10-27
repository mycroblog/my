// SPDX-License-Identifier: AGPL-3.0-only

// Package store keeps the knowledge graph in SQLite under the git directory every worktree shares.
//
// Extraction results are stored per (path, blob id), so worktrees share them and a file is parsed once per
// content. Each process computes its working tree's view — the (path, blob id) pairs it currently has —
// in a temporary table, and every query reads through that view. Results nobody has used for two weeks are
// pruned.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite" // the CGO-free SQLite driver

	"github.com/mycroblog/my/tools/agentkit/internal/extract"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

const schemaVersion = "1"

const schema = `
CREATE TABLE IF NOT EXISTS meta(k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS files(id INTEGER PRIMARY KEY, path TEXT NOT NULL, sha TEXT NOT NULL, kind TEXT NOT NULL,
  last_used INTEGER NOT NULL, UNIQUE(path, sha));
CREATE TABLE IF NOT EXISTS nodes(id INTEGER PRIMARY KEY, file_id INTEGER NOT NULL, nid TEXT NOT NULL, name TEXT NOT NULL,
  kind TEXT NOT NULL, title TEXT NOT NULL, start INTEGER NOT NULL, own_end INTEGER NOT NULL, end_line INTEGER NOT NULL,
  size INTEGER NOT NULL, meta TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS nodes_nid ON nodes(nid);
CREATE INDEX IF NOT EXISTS nodes_name ON nodes(name);
CREATE INDEX IF NOT EXISTS nodes_file ON nodes(file_id);
CREATE TABLE IF NOT EXISTS edges(file_id INTEGER NOT NULL, src TEXT NOT NULL, dst TEXT NOT NULL, rel TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS edges_src ON edges(src);
CREATE INDEX IF NOT EXISTS edges_dst ON edges(dst);
CREATE INDEX IF NOT EXISTS edges_file ON edges(file_id);
CREATE VIRTUAL TABLE IF NOT EXISTS fts USING fts5(title, body, tokenize='trigram');
CREATE TABLE IF NOT EXISTS gh(kind TEXT NOT NULL, number INTEGER NOT NULL, updated TEXT NOT NULL, title TEXT NOT NULL,
  state TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(kind, number));
CREATE TABLE IF NOT EXISTS gh_edges(kind TEXT NOT NULL, number INTEGER NOT NULL, dst TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS gh_edges_dst ON gh_edges(dst);
CREATE VIRTUAL TABLE IF NOT EXISTS ghfts USING fts5(title, body, tokenize='trigram');
`

// Store is the graph seen through one working tree.
type Store struct {
	DB   *sql.DB
	Repo *repo.Repo
}

// Path returns where the database of r lives.
func Path(r *repo.Repo) string { return filepath.Join(r.Common, "agentkit", "kb.db") }

// Open opens the database and brings the view of r's working tree up to date.
func Open(r *repo.Repo) (*Store, error) {
	p := Path(r)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+p+"?_pragma=busy_timeout(20000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // the view is a temporary table, which lives on one connection
	s := &Store{DB: db, Repo: r}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.sync(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	want := schemaVersion + "/" + extract.Version
	var got string
	err := s.DB.QueryRow(`SELECT v FROM meta WHERE k = 'version'`).Scan(&got)
	if err == nil && got == want {
		return nil
	}
	// A new database, or one another version built: start over (the GitHub cache refills on the next sync).
	for _, t := range []string{"meta", "files", "nodes", "edges", "fts", "gh", "gh_edges", "ghfts"} {
		if _, err := s.DB.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
			return err
		}
	}
	if _, err := s.DB.Exec(schema); err != nil {
		return fmt.Errorf("create the schema: %w", err)
	}
	_, err = s.DB.Exec(`INSERT OR REPLACE INTO meta(k, v) VALUES ('version', ?)`, want)
	return err
}

// sync extracts the files of the working tree the database does not have yet and fills the view.
func (s *Store) sync() error {
	files, err := s.Repo.Files()
	if err != nil {
		return err
	}
	known := map[[2]string]int64{}
	rows, err := s.DB.Query(`SELECT id, path, sha FROM files`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var p, sha string
		if err := rows.Scan(&id, &p, &sha); err != nil {
			_ = rows.Close()
			return err
		}
		known[[2]string{p, sha}] = id
	}
	_ = rows.Close()
	var view []int64
	var todo []repo.File
	for _, f := range files {
		if extract.Kind(f.Path) == "" || s.isSymlink(f.Path) {
			continue
		}
		if id, ok := known[[2]string{f.Path, f.SHA}]; ok {
			view = append(view, id)
		} else {
			todo = append(todo, f)
		}
	}
	ids, err := s.index(todo)
	if err != nil {
		return err
	}
	view = append(view, ids...)
	if _, err := s.DB.Exec(`CREATE TEMP TABLE IF NOT EXISTS view(file_id INTEGER PRIMARY KEY); DELETE FROM view`); err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	st, err := tx.Prepare(`INSERT OR IGNORE INTO view(file_id) VALUES (?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, id := range view {
		if _, err := st.Exec(id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.touch()
}

// isSymlink reports whether p is a symlink: indexing follows none, so that a link out of the repository cannot
// bring the files it points at into the index.
func (s *Store) isSymlink(p string) bool {
	fi, err := os.Lstat(filepath.Join(s.Repo.Root, p))
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// index extracts files in parallel and stores the results in one transaction.
func (s *Store) index(todo []repo.File) ([]int64, error) {
	if len(todo) == 0 {
		return nil, nil
	}
	results := make([]extract.Result, len(todo))
	var wg sync.WaitGroup
	ch := make(chan int)
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				b, err := os.ReadFile(filepath.Join(s.Repo.Root, todo[i].Path))
				if err == nil {
					results[i] = extract.Extract(todo[i].Path, b)
				}
			}
		}()
	}
	for i := range todo {
		ch <- i
	}
	close(ch)
	wg.Wait()

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var insFile, insNode, insFTS, insEdge *sql.Stmt
	for _, p := range []struct {
		st **sql.Stmt
		q  string
	}{
		{&insFile, `INSERT INTO files(path, sha, kind, last_used) VALUES (?, ?, ?, ?) ON CONFLICT(path, sha) DO NOTHING RETURNING id`},
		{&insNode, `INSERT INTO nodes(file_id, nid, name, kind, title, start, own_end, end_line, size, meta) VALUES (?,?,?,?,?,?,?,?,?,?)`},
		{&insFTS, `INSERT INTO fts(rowid, title, body) VALUES (?, ?, ?)`},
		{&insEdge, `INSERT INTO edges(file_id, src, dst, rel) VALUES (?, ?, ?, ?)`},
	} {
		if *p.st, err = tx.Prepare(p.q); err != nil {
			return nil, err
		}
	}
	now := time.Now().Unix()
	var ids []int64
	for i, f := range todo {
		r := results[i]
		var fid int64
		err := insFile.QueryRow(f.Path, f.SHA, extract.Kind(f.Path), now).Scan(&fid)
		if errors.Is(err, sql.ErrNoRows) {
			// Another worktree indexed the same content meanwhile.
			if err := tx.QueryRow(`SELECT id FROM files WHERE path = ? AND sha = ?`, f.Path, f.SHA).Scan(&fid); err != nil {
				return nil, err
			}
			ids = append(ids, fid)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("index %s: %w", f.Path, err)
		}
		ids = append(ids, fid)
		for _, n := range r.Nodes {
			res, err := insNode.Exec(fid, n.ID, n.Name, n.Kind, n.Title, n.Start, n.OwnEnd, n.End, len(n.Body), n.Meta)
			if err != nil {
				return nil, err
			}
			if n.Body != "" {
				rowid, _ := res.LastInsertId()
				if _, err := insFTS.Exec(rowid, n.Title, n.Body); err != nil {
					return nil, err
				}
			}
		}
		for _, e := range r.Edges {
			if _, err := insEdge.Exec(fid, e.Src, e.Dst, e.Rel); err != nil {
				return nil, err
			}
		}
	}
	return ids, tx.Commit()
}

// touch marks the view as used once a day and prunes what no worktree used for two weeks.
func (s *Store) touch() error {
	now := time.Now().Unix()
	var last string
	_ = s.DB.QueryRow(`SELECT v FROM meta WHERE k = 'touched:' || ?`, s.Repo.Root).Scan(&last)
	if t, _ := strconv.ParseInt(last, 10, 64); now-t < 86400 {
		return nil
	}
	if _, err := s.DB.Exec(`UPDATE files SET last_used = ? WHERE id IN (SELECT file_id FROM view)`, now); err != nil {
		return err
	}
	cutoff := now - 14*86400
	stale := `SELECT id FROM files WHERE last_used < ` + strconv.FormatInt(cutoff, 10)
	for _, q := range []string{
		`DELETE FROM fts WHERE rowid IN (SELECT id FROM nodes WHERE file_id IN (` + stale + `))`,
		`DELETE FROM nodes WHERE file_id IN (` + stale + `)`,
		`DELETE FROM edges WHERE file_id IN (` + stale + `)`,
		`DELETE FROM files WHERE last_used < ` + strconv.FormatInt(cutoff, 10),
	} {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO meta(k, v) VALUES ('touched:' || ?, ?)`, s.Repo.Root, strconv.FormatInt(now, 10))
	return err
}
