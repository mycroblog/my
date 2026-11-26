// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/uow"
)

// ProjectionStore implements projection.Store on the tables of the schema. Its
// queries are built at run time, since they serve any table; table names come from the schema
// and column names from the table, and both are quoted.
type ProjectionStore struct {
	db     *DB
	tables []string

	mu      sync.Mutex
	columns map[string][]string
}

var _ projection.Store = (*ProjectionStore)(nil)

// ProjectionStore returns the store of the projection tables among tables, the tables of the
// schema (schema.Tables(schema.Postgres)).
func (d *DB) ProjectionStore(tables []string) *ProjectionStore {
	return &ProjectionStore{db: d, tables: tables, columns: map[string][]string{}}
}

// Rows implements projection.Store. Timestamps are in UTC.
func (s *ProjectionStore) Rows(ctx context.Context, tx uow.Tx, table string, offset, limit int) ([]string, [][]any, error) {
	pgTx, err := s.db.txOf(tx)
	if err != nil {
		return nil, nil, err
	}
	cols, err := s.tableColumns(ctx, pgTx, table)
	if err != nil {
		return nil, nil, err
	}
	list := quoteAll(cols)
	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT $1 OFFSET $2", list, quote(table), list)
	rows, err := pgTx.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, nil, Wrap(err, "read "+table)
	}
	out, err := collectValues(rows)
	if err != nil {
		return nil, nil, Wrap(err, "read "+table)
	}
	return cols, out, nil
}

// collectValues returns the values of rows, timestamps in UTC and uuids in their logical form,
// and closes rows.
func collectValues(rows pgx.Rows) ([][]any, error) {
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		for i, v := range values {
			switch x := v.(type) {
			case time.Time:
				values[i] = x.UTC()
			case [16]byte:
				// A uuid column; the logical type id is its 36-character lowercase form.
				values[i] = uuidString(x)
			}
		}
		out = append(out, values)
	}
	return out, rows.Err()
}

// Trial implements projection.Store.
func (s *ProjectionStore) Trial(ctx context.Context, fn func(tx uow.Tx) error) error {
	return s.db.trial(ctx, fn)
}

// Shadow implements projection.Store: a temporary table LIKE the table, with its defaults,
// constraints, and indexes, and a copy of its rows. A temporary table comes first in the search
// path, also for the statements that the connection prepared before it existed (#3535).
func (s *ProjectionStore) Shadow(ctx context.Context, tx uow.Tx, tables []string) error {
	pgTx, err := s.db.txOf(tx)
	if err != nil {
		return err
	}
	var schema string
	if err := pgTx.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		return Wrap(err, "read the schema")
	}
	for _, table := range tables {
		if _, err := s.tableColumns(ctx, pgTx, table); err != nil {
			return err
		}
		live := quote(schema) + "." + quote(table)
		for _, q := range []string{
			fmt.Sprintf("CREATE TEMPORARY TABLE %s (LIKE %s INCLUDING ALL) ON COMMIT DROP", quote(table), live),
			fmt.Sprintf("INSERT INTO pg_temp.%s SELECT * FROM %s", quote(table), live),
		} {
			if _, err := pgTx.Exec(ctx, q); err != nil {
				return Wrap(err, "shadow "+table)
			}
		}
	}
	return nil
}

// Diff implements projection.Store, with EXCEPT ALL, which counts a repeated row.
func (s *ProjectionStore) Diff(ctx context.Context, tx uow.Tx, table string, limit int) ([]string, int, [][]any, [][]any, bool, error) {
	pgTx, err := s.db.txOf(tx)
	if err != nil {
		return nil, 0, nil, nil, false, err
	}
	cols, err := s.tableColumns(ctx, pgTx, table)
	if err != nil {
		return nil, 0, nil, nil, false, err
	}
	var schema string
	if err := pgTx.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		return nil, 0, nil, nil, false, Wrap(err, "read the schema")
	}
	live, shadow := quote(schema)+"."+quote(table), "pg_temp."+quote(table)
	var n int
	if err := pgTx.QueryRow(ctx, "SELECT count(*) FROM "+live).Scan(&n); err != nil {
		return nil, 0, nil, nil, false, Wrap(err, "count "+table)
	}
	list := quoteAll(cols)
	var more bool
	side := func(from, except string) ([][]any, error) {
		// NULL is LIMIT ALL.
		var bound any
		if limit > 0 {
			bound = limit + 1
		}
		q := fmt.Sprintf("SELECT %[1]s FROM (SELECT %[1]s FROM %[2]s EXCEPT ALL SELECT %[1]s FROM %[3]s) d ORDER BY %[1]s LIMIT $1", list, from, except)
		rows, err := pgTx.Query(ctx, q, bound)
		if err != nil {
			return nil, err
		}
		out, err := collectValues(rows)
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
	pgTx, err := s.db.txOf(tx)
	if err != nil {
		return 0, err
	}
	if err := s.checkColumns(ctx, pgTx, table, columns); err != nil {
		return 0, err
	}
	conds := make([]string, len(columns))
	for i, c := range columns {
		conds[i] = fmt.Sprintf("%s IS NOT DISTINCT FROM $%d", quote(c), i+1)
	}
	q := fmt.Sprintf("DELETE FROM %s WHERE %s", quote(table), strings.Join(conds, " AND "))
	return execRows(ctx, pgTx, q, rows, "delete from "+table)
}

// Insert implements projection.Store.
func (s *ProjectionStore) Insert(ctx context.Context, tx uow.Tx, table string, columns []string, rows [][]any) (int64, error) {
	pgTx, err := s.db.txOf(tx)
	if err != nil {
		return 0, err
	}
	if err := s.checkColumns(ctx, pgTx, table, columns); err != nil {
		return 0, err
	}
	marks := make([]string, len(columns))
	for i := range columns {
		marks[i] = fmt.Sprintf("$%d", i+1)
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING", quote(table), quoteAll(columns), strings.Join(marks, ", "))
	return execRows(ctx, pgTx, q, rows, "insert into "+table)
}

// execRows runs q once per row, all in one round trip (ADR-0172 §4).
func execRows(ctx context.Context, tx pgx.Tx, q string, rows [][]any, what string) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, r...)
	}
	res := tx.SendBatch(ctx, b)
	var total int64
	for range rows {
		tag, err := res.Exec()
		if err != nil {
			_ = res.Close()
			return total, Wrap(err, what)
		}
		total += tag.RowsAffected()
	}
	if err := res.Close(); err != nil {
		return total, Wrap(err, what)
	}
	return total, nil
}

// checkColumns fails unless columns are exactly the columns of table, in order.
func (s *ProjectionStore) checkColumns(ctx context.Context, tx pgx.Tx, table string, columns []string) error {
	cols, err := s.tableColumns(ctx, tx, table)
	if err != nil {
		return err
	}
	if !slices.Equal(cols, columns) {
		return fmt.Errorf("postgres: %s has the columns %v, not %v", table, cols, columns)
	}
	return nil
}

// tableColumns returns the columns of a table of the schema, in order.
func (s *ProjectionStore) tableColumns(ctx context.Context, tx pgx.Tx, table string) ([]string, error) {
	if !slices.Contains(s.tables, table) {
		return nil, fmt.Errorf("postgres: the schema has no table %q", table)
	}
	s.mu.Lock()
	cols, ok := s.columns[table]
	s.mu.Unlock()
	if ok {
		return cols, nil
	}
	rows, err := tx.Query(ctx, `SELECT column_name::text FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, Wrap(err, "read the columns of "+table)
	}
	cols, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, Wrap(err, "read the columns of "+table)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("postgres: table %q does not exist", table)
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

// uuidString returns the canonical lowercase form of a uuid value.
func uuidString(b [16]byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, c := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hexDigits[c>>4], hexDigits[c&0xf])
	}
	return string(out)
}
