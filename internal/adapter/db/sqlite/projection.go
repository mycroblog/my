// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/uow"
)

// ProjectionStore implements projection.Store on the tables of the schema. Its queries
// are built at run time, since they serve any table; table names come from the schema and
// column names from the table, and both are quoted.
type ProjectionStore struct {
	db     *DB
	tables []string

	mu      sync.Mutex
	columns map[string][]string
}

var _ projection.Store = (*ProjectionStore)(nil)

// ProjectionStore returns the store of the projection tables among tables, the tables of the
// schema (schema.Tables(schema.SQLite)).
func (d *DB) ProjectionStore(tables []string) *ProjectionStore {
	return &ProjectionStore{db: d, tables: tables, columns: map[string][]string{}}
}

// Rows implements projection.Store.
func (s *ProjectionStore) Rows(ctx context.Context, tx uow.Tx, table string, offset, limit int) ([]string, [][]any, error) {
	sqlTx, err := s.db.txOf(tx)
	if err != nil {
		return nil, nil, err
	}
	cols, err := s.tableColumns(ctx, sqlTx, table)
	if err != nil {
		return nil, nil, err
	}
	list := quoteAll(cols)
	//nolint:gosec // the table and its columns come from the schema and are quoted
	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT ? OFFSET ?", list, quote(table), list)
	rows, err := sqlTx.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, nil, Wrap(err, "read "+table)
	}
	out, err := collectValues(rows, len(cols))
	if err != nil {
		return nil, nil, Wrap(err, "read "+table)
	}
	return cols, out, nil
}

// collectValues returns the values of rows, of n columns each, and closes rows.
func collectValues(rows *sql.Rows, n int) ([][]any, error) {
	defer func() { _ = rows.Close() }()
	var out [][]any
	for rows.Next() {
		values := make([]any, n)
		ptrs := make([]any, n)
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out = append(out, values)
	}
	return out, rows.Err()
}

// Trial implements projection.Store.
func (s *ProjectionStore) Trial(ctx context.Context, fn func(tx uow.Tx) error) error {
	return s.db.trial(ctx, fn)
}

// createTable and createIndex match the start of the statements that sqlite_schema keeps for
// a table and its indexes, up to the name of the table or the index.
var (
	createTable = regexp.MustCompile(`(?is)^\s*CREATE\s+TABLE\s+`)
	createIndex = regexp.MustCompile(`(?is)^(\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?)`)
)

// Shadow implements projection.Store: a temporary table made by the statements that created
// the table and its indexes, and a copy of its rows. A temporary table comes first, also for the
// statements that the connection prepared before it existed, which SQLite prepares again (#3535).
func (s *ProjectionStore) Shadow(ctx context.Context, tx uow.Tx, tables []string) error {
	sqlTx, err := s.db.txOf(tx)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if _, err := s.tableColumns(ctx, sqlTx, table); err != nil {
			return err
		}
		rows, err := sqlTx.QueryContext(ctx,
			"SELECT type, sql FROM main.sqlite_schema WHERE tbl_name = ? AND sql IS NOT NULL ORDER BY type = 'index', name", table)
		if err != nil {
			return Wrap(err, "shadow "+table)
		}
		var stmts []string
		for rows.Next() {
			var kind, q string
			if err := rows.Scan(&kind, &q); err != nil {
				_ = rows.Close()
				return Wrap(err, "shadow "+table)
			}
			switch kind {
			case "table":
				stmts = append(stmts, createTable.ReplaceAllString(q, "CREATE TEMPORARY TABLE "))
			case "index":
				stmts = append(stmts, createIndex.ReplaceAllString(q, "${1}temp."))
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return Wrap(err, "shadow "+table)
		}
		_ = rows.Close()
		if len(stmts) == 0 || !strings.Contains(strings.ToUpper(stmts[0]), "TEMPORARY") {
			return fmt.Errorf("sqlite: shadow %s: no statement creates it", table)
		}
		stmts = append(stmts, fmt.Sprintf("INSERT INTO temp.%s SELECT * FROM main.%s", quote(table), quote(table)))
		for _, q := range stmts {
			if _, err := sqlTx.ExecContext(ctx, q); err != nil {
				return Wrap(err, "shadow "+table)
			}
		}
	}
	return nil
}

// Diff implements projection.Store. SQLite has no EXCEPT ALL: a row that a table without a key
// holds twice counts once.
func (s *ProjectionStore) Diff(ctx context.Context, tx uow.Tx, table string, limit int) ([]string, int, [][]any, [][]any, bool, error) {
	sqlTx, err := s.db.txOf(tx)
	if err != nil {
		return nil, 0, nil, nil, false, err
	}
	cols, err := s.tableColumns(ctx, sqlTx, table)
	if err != nil {
		return nil, 0, nil, nil, false, err
	}
	live, shadow := "main."+quote(table), "temp."+quote(table)
	var n int
	if err := sqlTx.QueryRowContext(ctx, "SELECT count(*) FROM "+live).Scan(&n); err != nil {
		return nil, 0, nil, nil, false, Wrap(err, "count "+table)
	}
	list := quoteAll(cols)
	var more bool
	side := func(from, except string) ([][]any, error) {
		bound := -1 // no limit
		if limit > 0 {
			bound = limit + 1
		}
		//nolint:gosec // the table and its columns come from the schema and are quoted
		q := fmt.Sprintf("SELECT %[1]s FROM %[2]s EXCEPT SELECT %[1]s FROM %[3]s ORDER BY %[1]s LIMIT ?", list, from, except)
		rows, err := sqlTx.QueryContext(ctx, q, bound)
		if err != nil {
			return nil, err
		}
		out, err := collectValues(rows, len(cols))
		if err != nil {
			return nil, err
		}
		if limit > 0 && len(out) > limit {
			out, more = out[:limit], true
		}
		return out, nil
	}
	extra, err := side(live, shadow)
	if err != nil {
		return nil, 0, nil, nil, false, Wrap(err, "compare "+table)
	}
	missing, err := side(shadow, live)
	if err != nil {
		return nil, 0, nil, nil, false, Wrap(err, "compare "+table)
	}
	return cols, n, extra, missing, more, nil
}

// Delete implements projection.Store.
func (s *ProjectionStore) Delete(ctx context.Context, tx uow.Tx, table string, columns []string, rows [][]any) (int64, error) {
	sqlTx, err := s.db.txOf(tx)
	if err != nil {
		return 0, err
	}
	if err := s.checkColumns(ctx, sqlTx, table, columns); err != nil {
		return 0, err
	}
	conds := make([]string, len(columns))
	for i, c := range columns {
		conds[i] = quote(c) + " IS ?"
	}
	q := fmt.Sprintf("DELETE FROM %s WHERE %s", quote(table), strings.Join(conds, " AND "))
	return execRows(ctx, sqlTx, q, rows, "delete from "+table)
}

// Insert implements projection.Store.
func (s *ProjectionStore) Insert(ctx context.Context, tx uow.Tx, table string, columns []string, rows [][]any) (int64, error) {
	sqlTx, err := s.db.txOf(tx)
	if err != nil {
		return 0, err
	}
	if err := s.checkColumns(ctx, sqlTx, table, columns); err != nil {
		return 0, err
	}
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING", quote(table), quoteAll(columns), marks)
	return execRows(ctx, sqlTx, q, rows, "insert into "+table)
}

func execRows(ctx context.Context, tx *sql.Tx, q string, rows [][]any, what string) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return 0, Wrap(err, what)
	}
	defer func() { _ = stmt.Close() }()
	var total int64
	for _, r := range rows {
		res, err := stmt.ExecContext(ctx, r...)
		if err != nil {
			return total, Wrap(err, what)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, Wrap(err, what)
		}
		total += n
	}
	return total, nil
}

// checkColumns fails unless columns are exactly the columns of table, in order.
func (s *ProjectionStore) checkColumns(ctx context.Context, tx *sql.Tx, table string, columns []string) error {
	cols, err := s.tableColumns(ctx, tx, table)
	if err != nil {
		return err
	}
	if !slices.Equal(cols, columns) {
		return fmt.Errorf("sqlite: %s has the columns %v, not %v", table, cols, columns)
	}
	return nil
}

// tableColumns returns the columns of a table of the schema, in order.
func (s *ProjectionStore) tableColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	if !slices.Contains(s.tables, table) {
		return nil, fmt.Errorf("sqlite: the schema has no table %q", table)
	}
	s.mu.Lock()
	cols, ok := s.columns[table]
	s.mu.Unlock()
	if ok {
		return cols, nil
	}
	rows, err := tx.QueryContext(ctx, "SELECT name FROM pragma_table_info(?) ORDER BY cid", table)
	if err != nil {
		return nil, Wrap(err, "read the columns of "+table)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, Wrap(err, "read the columns of "+table)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, Wrap(err, "read the columns of "+table)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("sqlite: table %q does not exist", table)
	}
	s.mu.Lock()
	s.columns[table] = cols
	s.mu.Unlock()
	return cols, nil
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func quoteAll(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = quote(n)
	}
	return strings.Join(q, ", ")
}
