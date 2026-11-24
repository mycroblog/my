// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
)

// profiles takes the CPU, block and mutex profiles of s over seconds at once and writes each to
// dir as <prefix>-<kind>.pprof; it returns the files' names.
func profiles(ctx context.Context, s *server, seconds int, dir, prefix string) ([]string, error) {
	// The path of each profile below /debug/pprof/, and the name of its file.
	kinds := [][2]string{{"profile", "cpu"}, {"block", "block"}, {"mutex", "mutex"}}
	names := make([]string, len(kinds))
	errs := make([]error, len(kinds))
	var wg sync.WaitGroup
	for i, kind := range kinds {
		wg.Go(func() {
			names[i] = prefix + "-" + kind[1] + ".pprof"
			errs[i] = fetchTo(ctx, fmt.Sprintf("%s/debug/pprof/%s?seconds=%d", s.pprof, kind[0], seconds), filepath.Join(dir, names[i]))
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return names, nil
}

func fetchTo(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := local.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", url, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	f, err := os.Create(path) //nolint:gosec // a file of the run's own directory
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// traceEnv returns the settings that send every trace of a server to the receiver at origin.
func traceEnv(origin string) []string {
	return []string{
		"MY_TRACES_EXPORTER=otlp",
		"MY_TRACES_SAMPLE_RATIO=1",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=" + origin + "/v1/traces",
	}
}

// The names and attributes of the server's spans that summarize reads (internal/platform/tracing.go,
// internal/app/queue/metrics.go, internal/adapter/telemetry/dbtrace).
const (
	attrOperation = "db.operation.name"
	attrJobType   = "my.job.type"
	spanTx        = "my.db.transaction"
	// rootOther is the root of the traces that are neither a request nor a job.
	rootOther = "(no request or job)"
)

// summarize groups spans by trace and returns the mean trace of each root, and the queries by
// the time they took, most first.
func summarize(spans []otlptest.Span, requests int) *TraceResult {
	byTrace := map[string][]otlptest.Span{}
	for _, s := range spans {
		byTrace[s.TraceID] = append(byTrace[s.TraceID], s)
	}
	type acc struct {
		count, statements, txs int
		db, total              time.Duration
		queries                map[string]int
	}
	roots := map[string]*acc{}
	queries := map[string]*QueryStats{}
	for _, trace := range byTrace {
		ids := map[string]bool{}
		for _, s := range trace {
			ids[s.SpanID] = true
		}
		var root *otlptest.Span
		for i := range trace {
			if trace[i].ParentSpanID == "" || !ids[trace[i].ParentSpanID] {
				root = &trace[i]
				break
			}
		}
		if root == nil {
			continue
		}
		key := root.Name
		switch t, job := root.Attrs[attrJobType]; {
		case job:
			key = "job " + t
		case root.Kind != "server":
			// The statements of no request or job: the job runner's claims, the start-up.
			key = rootOther
		}
		a := roots[key]
		if a == nil {
			a = &acc{queries: map[string]int{}}
			roots[key] = a
		}
		a.count++
		a.total += spanTime(*root)
		for _, s := range trace {
			switch {
			case s.Name == spanTx:
				a.txs++
			case s.Attrs[attrOperation] != "":
				name := s.Attrs[attrOperation]
				a.statements++
				a.queries[name]++
				d := spanTime(s)
				a.db += d
				q := queries[name]
				if q == nil {
					q = &QueryStats{Name: name}
					queries[name] = q
				}
				q.Calls++
				q.TotalMillis += ms(d)
			}
		}
	}
	out := &TraceResult{Requests: requests}
	for _, key := range slices.Sorted(maps.Keys(roots)) {
		a := roots[key]
		n := float64(a.count)
		rs := RootStats{
			Root: key, Count: a.count, Statements: float64(a.statements) / n, Transactions: float64(a.txs) / n,
			DBMillis: ms(a.db) / n, Millis: ms(a.total) / n,
		}
		for name, c := range a.queries {
			rs.Queries = append(rs.Queries, QueryCount{Name: name, PerRequest: float64(c) / n})
		}
		slices.SortFunc(rs.Queries, func(x, y QueryCount) int {
			return cmp.Or(cmp.Compare(y.PerRequest, x.PerRequest), cmp.Compare(x.Name, y.Name))
		})
		out.Roots = append(out.Roots, rs)
	}
	for _, q := range queries {
		q.MeanMillis = q.TotalMillis / float64(q.Calls)
		out.Queries = append(out.Queries, *q)
	}
	slices.SortFunc(out.Queries, func(x, y QueryStats) int {
		return cmp.Or(cmp.Compare(y.TotalMillis, x.TotalMillis), cmp.Compare(x.Name, y.Name))
	})
	return out
}

func spanTime(s otlptest.Span) time.Duration {
	if s.EndUnixNano < s.StartUnixNano {
		return 0
	}
	return time.Duration(s.EndUnixNano - s.StartUnixNano) //nolint:gosec // G115: a span lasts less than 292 years
}

// querySQL returns the SQL of every sqlc query of kind (sqlite or postgres) by name, read from the
// generated code under root: each constant whose text starts with "-- name: <Name>".
func querySQL(root, kind string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, "internal", "adapter"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".sql.go") || !strings.Contains(path, kind) {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				for _, v := range spec.(*ast.ValueSpec).Values {
					lit, ok := v.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					text, err := strconv.Unquote(lit.Value)
					if err != nil {
						continue
					}
					rest, ok := strings.CutPrefix(text, "-- name: ")
					if !ok {
						continue
					}
					name, _, _ := strings.Cut(rest, " ")
					if _, seen := out[name]; !seen {
						out[name] = text
					}
				}
			}
		}
		return nil
	})
	return out, err
}

// plans returns the plans on db of the first n queries, by name, whose SQL sqlText holds.
func plans(ctx context.Context, db *database, queries []QueryStats, sqlText map[string]string, n int) ([]Plan, error) {
	conn, err := db.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	var out []Plan
	for _, q := range queries {
		if len(out) == n {
			break
		}
		text, ok := sqlText[q.Name]
		if !ok {
			continue // a statement of no sqlc query: BEGIN, COMMIT, or one built in code
		}
		p := Plan{Name: q.Name, SQL: text}
		p.Plan, err = explain(ctx, conn, db.kind, text)
		if err != nil {
			p.Error = err.Error()
		}
		out = append(out, p)
	}
	return out, nil
}

// explain returns the plan of text on conn, its parameters unbound: a generic plan on PostgreSQL,
// and on SQLite the plan with every parameter NULL.
func explain(ctx context.Context, conn *sql.DB, kind, text string) (string, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if kind == "postgres" {
		rows, err = conn.QueryContext(ctx, "EXPLAIN (GENERIC_PLAN) "+text)
	} else {
		rows, err = conn.QueryContext(ctx, "EXPLAIN QUERY PLAN "+text, make([]any, sqliteParams(text))...)
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		if kind == "postgres" {
			fmt.Fprintf(&b, "%v\n", vals[0])
			continue
		}
		// id, parent, notused, detail: the detail, indented by its depth.
		fmt.Fprintf(&b, "%v %v %v\n", vals[0], vals[1], vals[len(vals)-1])
	}
	return b.String(), rows.Err()
}

// sqliteParams returns the number of parameters of a SQLite statement: the highest ?NNN, or the
// count of bare ?, outside quoted text.
func sqliteParams(text string) int {
	bare, highest := 0, 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '-' && i+1 < len(text) && text[i+1] == '-':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == '?':
			j := i + 1
			for j < len(text) && text[j] >= '0' && text[j] <= '9' {
				j++
			}
			if j == i+1 {
				bare++
			} else if n, _ := strconv.Atoi(text[i+1 : j]); n > highest {
				highest = n
			}
			i = j - 1
		}
	}
	return max(bare, highest)
}
