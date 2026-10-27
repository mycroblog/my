// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/schema"
)

// ProjectionStore implements projection.Store on the tables of schema/sqlite. Its queries
// are built at run time, since they serve any table; table names come from the schema and
// column names from the table, and both are quoted.
type ProjectionStore struct {
	db     *DB
	tables []string

	mu      sync.Mutex
	columns map[string][]string
}

var _ projection.Store = (*ProjectionStore)(nil)

// ProjectionStore returns the store of the projection tables.
func (d *DB) ProjectionStore() *ProjectionStore {
	return &ProjectionStore{db: d, tables: schema.Tables(schema.SQLite), columns: map[string][]string{}}
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
		return nil, nil, wrap(err, "read "+table)
	}
	defer func() { _ = rows.Close() }()
	var out [][]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, wrap(err, "read "+table)
		}
		out = append(out, values)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, wrap(err, "read "+table)
	}
	return cols, out, nil
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
		return 0, wrap(err, what)
	}
	defer func() { _ = stmt.Close() }()
	var total int64
	for _, r := range rows {
		res, err := stmt.ExecContext(ctx, r...)
		if err != nil {
			return total, wrap(err, what)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, wrap(err, what)
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
		return nil, wrap(err, "read the columns of "+table)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, wrap(err, "read the columns of "+table)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, wrap(err, "read the columns of "+table)
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
