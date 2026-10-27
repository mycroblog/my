// SPDX-License-Identifier: AGPL-3.0-only

// Package gh talks to GitHub through the gh CLI (so that it uses the user's own authentication) and keeps a
// local cache of the issues and merged pull requests in the knowledge graph's database: their text for search,
// the identifiers they mention, and the files each merged pull request changed.
package gh

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

// Repo is the GitHub repository; AGENTKIT_REPO overrides it.
func Repo() string {
	if r := os.Getenv("AGENTKIT_REPO"); r != "" {
		return r
	}
	return "mycroblog/my"
}

// Run runs gh with args against Repo and returns its standard output.
func Run(args ...string) ([]byte, error) {
	return RunIn("", args...)
}

// RunIn runs gh in dir.
func RunIn(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// JSON runs gh and decodes its JSON output into v.
func JSON(v any, args ...string) error {
	out, err := Run(args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, v)
}

// Login is a GitHub user.
type Login struct {
	Login string `json:"login"`
}

// Label is an issue label.
type Label struct {
	Name string `json:"name"`
}

// Comment is an issue comment.
type Comment struct {
	Author    Login  `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

// Issue is an issue with what the context pack needs.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	UpdatedAt string    `json:"updatedAt"`
	Labels    []Label   `json:"labels"`
	Assignees []Login   `json:"assignees"`
	Comments  []Comment `json:"comments"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
}

// HasLabel reports whether the issue has label l.
func (i *Issue) HasLabel(l string) bool {
	for _, x := range i.Labels {
		if x.Name == l {
			return true
		}
	}
	return false
}

// GetIssue fetches an issue with its comments.
func GetIssue(n int) (*Issue, error) {
	var i Issue
	err := JSON(&i, "issue", "view", strconv.Itoa(n), "-R", Repo(), "--json", "number,title,body,state,url,updatedAt,labels,assignees,comments,milestone")
	return &i, err
}

// File is a file a pull request changed.
type File struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// PR is a pull request.
type PR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
	MergedAt    string `json:"mergedAt"`
	UpdatedAt   string `json:"updatedAt"`
	Files       []File `json:"files"`
}

// Cache is the GitHub part of the database.
type Cache struct {
	DB *sql.DB
}

// Synced returns when the cache was last refreshed.
func (c *Cache) Synced() time.Time {
	var v string
	_ = c.DB.QueryRow(`SELECT v FROM meta WHERE k = 'gh:synced'`).Scan(&v)
	t, _ := time.Parse(time.RFC3339, v)
	return t
}

// issueLimit is the most issues one `gh issue list` call returns here.
const issueLimit = 5000

// notTruncated fails when a listing returned exactly its limit: gh cuts a longer list silently, so the cache
// would miss the rest.
func notTruncated(what string, n, limit int) error {
	if n >= limit {
		return fmt.Errorf("gh returned %d %s, the limit of one call: the list may be cut and the cache incomplete", n, what)
	}
	return nil
}

// Sync refreshes the issues and the merged pull requests updated since the last sync (all of them the first
// time) and reports how many it stored.
func (c *Cache) Sync() (int, error) {
	since := c.Synced()
	started := time.Now().UTC()
	search := []string{}
	if !since.IsZero() {
		search = []string{"--search", "updated:>=" + since.Add(-time.Hour).Format("2006-01-02T15:04:05Z")}
	}
	var issues []Issue
	args := append([]string{"issue", "list", "-R", Repo(), "--state", "all", "--limit", strconv.Itoa(issueLimit), "--json", "number,title,body,state,updatedAt,labels"}, search...)
	if err := JSON(&issues, args...); err != nil {
		return 0, err
	}
	if err := notTruncated("issues", len(issues), issueLimit); err != nil {
		return 0, err
	}
	prs, err := mergedPRs(since)
	if err != nil {
		return 0, err
	}
	tx, err := c.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	for i := range issues {
		is := &issues[i]
		if err := put(tx, "issue", is.Number, is.UpdatedAt, is.Title, is.State, is, is.Body, nil); err != nil {
			return 0, err
		}
	}
	for i := range prs {
		pr := &prs[i]
		extra := make([]string, 0, len(pr.Files)+1)
		if i := BranchIssue(pr.HeadRefName); i != "" {
			extra = append(extra, "issue:"+i)
		}
		for _, f := range pr.Files {
			extra = append(extra, "file:"+f.Path)
		}
		if err := put(tx, "pr", pr.Number, pr.UpdatedAt, pr.Title, pr.State, pr, pr.Body, extra); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta(k, v) VALUES ('gh:synced', ?)`, started.Format(time.RFC3339)); err != nil {
		return 0, err
	}
	return len(issues) + len(prs), tx.Commit()
}

const prQuery = `query($owner: String!, $name: String!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: MERGED, first: 50, after: $cursor, orderBy: {field: UPDATED_AT, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes { number title body state url headRefName mergedAt updatedAt files(first: 100) { pageInfo { hasNextPage endCursor } nodes { path additions deletions } } }
    }
  }
}`

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

const filesQuery = `query($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) { files(first: 100, after: $cursor) { pageInfo { hasNextPage endCursor } nodes { path additions deletions } } }
  }
}`

// filesPage reads one page of the files of pull request number after cursor.
func filesPage(number int, cursor string) ([]File, pageInfo, error) {
	owner, name, _ := strings.Cut(Repo(), "/")
	var resp struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					Files struct {
						Nodes    []File   `json:"nodes"`
						PageInfo pageInfo `json:"pageInfo"`
					} `json:"files"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	err := JSON(&resp, "api", "graphql", "-f", "query="+filesQuery, "-F", "owner="+owner, "-F", "name="+name, "-F", "number="+strconv.Itoa(number), "-F", "cursor="+cursor)
	f := resp.Data.Repository.PullRequest.Files
	return f.Nodes, f.PageInfo, err
}

// moreFiles reads the pages of a pull request's files after cursor, so that a pull request of more than one page
// of files does not lose the later ones.
func moreFiles(number int, cursor string, page func(number int, cursor string) ([]File, pageInfo, error)) ([]File, error) {
	var out []File
	for {
		files, pi, err := page(number, cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, files...)
		if !pi.HasNextPage {
			return out, nil
		}
		cursor = pi.EndCursor
	}
}

// mergedPRs pages through the merged pull requests, most recently updated first, down to since.
func mergedPRs(since time.Time) ([]PR, error) {
	owner, name, _ := strings.Cut(Repo(), "/")
	var out []PR
	cursor := ""
	for {
		args := []string{"api", "graphql", "-f", "query=" + prQuery, "-F", "owner=" + owner, "-F", "name=" + name}
		if cursor != "" {
			args = append(args, "-F", "cursor="+cursor)
		}
		var resp struct {
			Data struct {
				Repository struct {
					PullRequests struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							PR
							Files struct {
								Nodes    []File   `json:"nodes"`
								PageInfo pageInfo `json:"pageInfo"`
							} `json:"files"`
						} `json:"nodes"`
					} `json:"pullRequests"`
				} `json:"repository"`
			} `json:"data"`
		}
		if err := JSON(&resp, args...); err != nil {
			return nil, err
		}
		page := resp.Data.Repository.PullRequests
		for _, n := range page.Nodes {
			pr := n.PR
			pr.Files = n.Files.Nodes
			if n.Files.PageInfo.HasNextPage {
				more, err := moreFiles(n.Number, n.Files.PageInfo.EndCursor, filesPage)
				if err != nil {
					return nil, err
				}
				pr.Files = append(pr.Files, more...)
			}
			if t, _ := time.Parse(time.RFC3339, pr.UpdatedAt); !since.IsZero() && t.Before(since.Add(-time.Hour)) {
				return out, nil
			}
			out = append(out, pr)
		}
		if !page.PageInfo.HasNextPage {
			return out, nil
		}
		cursor = page.PageInfo.EndCursor
	}
}

// put stores one item, its search text, and its edges: the identifiers its text mentions and extra.
func put(tx *sql.Tx, kind string, n int, updated, title, state string, v any, body string, extra []string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	rowid := int64(n)*2 + map[string]int64{"issue": 0, "pr": 1}[kind]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT OR REPLACE INTO gh(kind, number, updated, title, state, data) VALUES (?,?,?,?,?,?)`, []any{kind, n, updated, title, state, string(data)}},
		{`DELETE FROM gh_edges WHERE kind = ? AND number = ?`, []any{kind, n}},
		{`DELETE FROM ghfts WHERE rowid = ?`, []any{rowid}},
		{`INSERT INTO ghfts(rowid, title, body) VALUES (?, ?, ?)`, []any{rowid, title, body}},
	} {
		if _, err := tx.Exec(q.sql, q.args...); err != nil {
			return err
		}
	}
	all := title + "\n" + body
	var dsts []string
	dsts = append(dsts, text.Requirements(all)...)
	dsts = append(dsts, text.ADRs(all)...)
	for _, i := range text.Issues(all) {
		dsts = append(dsts, "issue:"+i)
	}
	for _, d := range text.Uniq(append(dsts, extra...)) {
		if _, err := tx.Exec(`INSERT INTO gh_edges(kind, number, dst) VALUES (?, ?, ?)`, kind, n, d); err != nil {
			return err
		}
	}
	return nil
}

// BranchIssue returns the issue number of a branch named <type>/<n>-<slug>, or "".
func BranchIssue(branch string) string {
	_, rest, ok := strings.Cut(branch, "/")
	if !ok {
		return ""
	}
	n, _, _ := strings.Cut(rest, "-")
	if _, err := strconv.Atoi(n); err != nil {
		return ""
	}
	return n
}

// Item is a cached issue or pull request.
type Item struct {
	Kind   string
	Number int
	Title  string
	State  string
	Data   string
	Score  int
}

// Related returns the cached items that mention any of dsts, the items mentioning most of them first.
func (c *Cache) Related(dsts []string, kind string, limit int) ([]Item, error) {
	if len(dsts) == 0 {
		return nil, nil
	}
	args := []any{kind}
	for _, d := range dsts {
		args = append(args, d)
	}
	args = append(args, limit)
	//nolint:gosec // G202: q is constant fragments and placeholders only
	q := `SELECT g.kind, g.number, g.title, g.state, g.data, count(*) AS score
FROM gh_edges e JOIN gh g ON g.kind = e.kind AND g.number = e.number
WHERE e.kind = ? AND e.dst IN (?` + strings.Repeat(",?", len(dsts)-1) + `)
GROUP BY g.kind, g.number ORDER BY score DESC, g.number DESC LIMIT ?`
	rows, err := c.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.Kind, &it.Number, &it.Title, &it.State, &it.Data, &it.Score); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Get returns a cached item.
func (c *Cache) Get(kind string, n int) (Item, bool) {
	var it Item
	err := c.DB.QueryRow(`SELECT kind, number, title, state, data FROM gh WHERE kind = ? AND number = ?`, kind, n).Scan(&it.Kind, &it.Number, &it.Title, &it.State, &it.Data)
	return it, err == nil
}

// Search ranks the cached issues and pull requests whose text contains the words of q.
func (c *Cache) Search(q string, limit int) ([]Item, error) {
	var terms []string
	for _, w := range strings.Fields(q) {
		if len([]rune(w)) >= 3 {
			terms = append(terms, `"`+strings.ReplaceAll(w, `"`, `""`)+`"`)
		}
	}
	if len(terms) == 0 {
		return nil, nil
	}
	rows, err := c.DB.Query(`SELECT g.kind, g.number, g.title, g.state FROM ghfts JOIN gh g ON g.number * 2 + (g.kind = 'pr') = ghfts.rowid
WHERE ghfts MATCH ? ORDER BY bm25(ghfts, 4.0, 1.0) LIMIT ?`, strings.Join(terms, " "), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.Kind, &it.Number, &it.Title, &it.State); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
