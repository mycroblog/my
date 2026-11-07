// SPDX-License-Identifier: AGPL-3.0-only

// Package projection rebuilds and verifies the projections (data-model §9). It knows no table
// of its own: each Projection names the tables that one module's rebuild function derives, and
// the module's function does the deriving. The service runs that function inside a
// transaction that it rolls back, reads what the function produced, and compares it with the
// rows the projection holds now:
//
//   - Verify reports the difference and changes nothing.
//   - Rebuild also writes the difference to the projection's tables, a bounded number of rows
//     per transaction (ADR-0003), so that readers never see an emptied projection and rows
//     that are right are not rewritten.
//
// The scheduled job JobVerify verifies every projection and corrects the ones that drifted
// (data-model §2.3).
package projection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/uow"
)

// DefaultChunkSize is how many rows Rebuild writes per transaction and how many rows a read
// of a projection returns at a time, well under the 3,000 rows per transaction that ADR-0003
// allows.
const DefaultChunkSize = 500

// Projection is a set of projection tables that one rebuild function derives together.
type Projection struct {
	// Name identifies the projection on the command line, for example "content".
	Name string
	// Module is the module that owns the tables.
	Module string
	// Tables are the projection tables that Rebuild derives, parents before children.
	Tables []string
	// Rebuild empties Tables and derives them again from their sources through u. The
	// service passes a UnitOfWork whose transactions all run inside one transaction that it
	// rolls back, so Rebuild must reach the database only through u.
	Rebuild func(ctx context.Context, u uow.UnitOfWork) error
}

// Store is the port through which the service reads and writes projection tables without
// knowing their columns. The database adapters implement it for the tables of their schema
// and pass projectiontest.ProjectionStoreContract. A table that the schema does not declare is an
// error.
type Store interface {
	// Rows returns up to limit rows of table inside tx, skipping offset rows, in the order of
	// all their columns, with the names of the columns. Values are what the driver scans into
	// an any: they go back to Delete and Insert unchanged.
	Rows(ctx context.Context, tx uow.Tx, table string, offset, limit int) (columns []string, rows [][]any, err error)
	// Delete deletes inside tx the rows of table whose every column equals the row's value
	// (NULL equal to NULL), and returns how many it deleted.
	Delete(ctx context.Context, tx uow.Tx, table string, columns []string, rows [][]any) (int64, error)
	// Insert inserts the rows into table inside tx, skipping a row that conflicts with one the
	// table holds, and returns how many it inserted.
	Insert(ctx context.Context, tx uow.Tx, table string, columns []string, rows [][]any) (int64, error)
}

// ErrUnknown is returned for a name that is neither a projection nor one of its tables.
var ErrUnknown = errors.New("projection: unknown projection")

// Row is a row of a projection table.
type Row struct {
	Values []any
	key    string
}

// String returns the row as column=value pairs.
func (r Row) String() string { return r.key }

// TableDiff is the difference between a table and what a rebuild derives for it.
type TableDiff struct {
	Table   string
	Columns []string
	// Rows is how many rows the table holds.
	Rows int
	// Extra are the rows that the table holds and a rebuild does not derive.
	Extra []Row
	// Missing are the rows that a rebuild derives and the table does not hold.
	Missing []Row
}

// Empty reports whether the table holds what a rebuild derives.
func (d TableDiff) Empty() bool { return len(d.Extra) == 0 && len(d.Missing) == 0 }

// Report is the result of verifying or rebuilding one projection.
type Report struct {
	Projection Projection
	// Tables has one entry per table of the projection, in its order.
	Tables []TableDiff
	// Deleted and Inserted count the rows that Rebuild wrote; both are zero for Verify.
	Deleted, Inserted int64
}

// Drifted reports whether any table differed from what a rebuild derives.
func (r Report) Drifted() bool {
	for _, t := range r.Tables {
		if !t.Empty() {
			return true
		}
	}
	return false
}

// Options configure a Service.
type Options struct {
	UnitOfWork  uow.UnitOfWork
	Store       Store
	Projections []Projection
	// ChunkSize is DefaultChunkSize when zero or less.
	ChunkSize int
	// Logger records the drift that JobVerify corrects; nil means slog.Default().
	Logger *slog.Logger
}

// Service rebuilds and verifies the projections.
type Service struct {
	u           uow.UnitOfWork
	store       Store
	projections []Projection
	chunk       int
	logger      *slog.Logger
}

// NewService returns a service of the projections in opts. It panics on a projection without
// a name, tables, or a rebuild function, and on a name or a table that two projections share.
func NewService(opts Options) *Service {
	seen := map[string]bool{}
	for _, p := range opts.Projections {
		if p.Name == "" || len(p.Tables) == 0 || p.Rebuild == nil {
			panic("projection: a projection needs a name, tables, and a rebuild function")
		}
		// A projection may be named after its only table; any other repeat is ambiguous.
		if len(slices.Compact(slices.Sorted(slices.Values(p.Tables)))) < len(p.Tables) {
			panic(fmt.Sprintf("projection: %q lists a table twice", p.Name))
		}
		names := slices.Compact(slices.Sorted(slices.Values(append([]string{p.Name}, p.Tables...))))
		for _, n := range names {
			if seen[n] {
				panic(fmt.Sprintf("projection: %q is named twice", n))
			}
			seen[n] = true
		}
	}
	chunk := opts.ChunkSize
	if chunk <= 0 {
		chunk = DefaultChunkSize
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		u: opts.UnitOfWork, store: opts.Store, projections: slices.Clone(opts.Projections), chunk: chunk, logger: logger,
	}
}

// List returns the projections, in registration order.
func (s *Service) List() []Projection { return slices.Clone(s.projections) }

// Lookup returns the projection named name, or the one that holds the table name. It returns
// an error wrapping ErrUnknown when there is none.
func (s *Service) Lookup(name string) (Projection, error) {
	for _, p := range s.projections {
		if p.Name == name || slices.Contains(p.Tables, name) {
			return p, nil
		}
	}
	return Projection{}, fmt.Errorf("%w %q", ErrUnknown, name)
}

// Verify compares the tables of p with what its rebuild function derives from their sources
// now, and changes nothing.
func (s *Service) Verify(ctx context.Context, p Projection) (Report, error) {
	return s.compare(ctx, p)
}

// Rebuild derives the tables of p again and writes the difference from what they hold:
// first it deletes the extra rows, then it inserts the missing ones, at most the chunk size
// of rows per transaction. A row that changes between the comparison and the write is left
// as it is (a delete matches whole rows, an insert skips a row whose key exists), so a
// rebuild that runs while the instance writes to the projection leaves at most the drift of
// those rows, which the next verification finds.
func (s *Service) Rebuild(ctx context.Context, p Projection) (Report, error) {
	r, err := s.compare(ctx, p)
	if err != nil {
		return r, err
	}
	// Delete children before parents and insert parents before children.
	for i := len(r.Tables) - 1; i >= 0; i-- {
		t := r.Tables[i]
		n, err := s.write(ctx, t.Table, t.Columns, t.Extra, s.store.Delete)
		r.Deleted += n
		if err != nil {
			return r, fmt.Errorf("rebuild %s: delete from %s: %w", p.Name, t.Table, err)
		}
	}
	for _, t := range r.Tables {
		n, err := s.write(ctx, t.Table, t.Columns, t.Missing, s.store.Insert)
		r.Inserted += n
		if err != nil {
			return r, fmt.Errorf("rebuild %s: insert into %s: %w", p.Name, t.Table, err)
		}
	}
	return r, nil
}

type writeFunc func(ctx context.Context, tx uow.Tx, table string, columns []string, rows [][]any) (int64, error)

func (s *Service) write(ctx context.Context, table string, columns []string, rows []Row, fn writeFunc) (int64, error) {
	var total int64
	for chunk := range slices.Chunk(rows, s.chunk) {
		values := make([][]any, len(chunk))
		for i, r := range chunk {
			values[i] = r.Values
		}
		var n int64
		err := s.u.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = fn(ctx, tx, table, columns, values)
			return err
		})
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// errRollback rolls back the transaction of a comparison.
var errRollback = errors.New("projection: roll back")

// compare reads the tables of p, runs its rebuild function, reads them again, and rolls all of
// it back, in one transaction.
func (s *Service) compare(ctx context.Context, p Projection) (Report, error) {
	var r Report
	err := s.u.Tx(ctx, func(tx uow.Tx) error {
		r = Report{Projection: p}
		before := make([]table, len(p.Tables))
		for i, name := range p.Tables {
			t, err := s.read(ctx, tx, name)
			if err != nil {
				return err
			}
			before[i] = t
		}
		if err := p.Rebuild(ctx, within{tx}); err != nil {
			return err
		}
		for i, name := range p.Tables {
			after, err := s.read(ctx, tx, name)
			if err != nil {
				return err
			}
			r.Tables = append(r.Tables, diff(before[i], after))
		}
		return errRollback
	})
	if errors.Is(err, errRollback) {
		return r, nil
	}
	if err != nil {
		return Report{Projection: p}, fmt.Errorf("verify %s: %w", p.Name, err)
	}
	return r, errors.New("projection: the comparison was not rolled back")
}

type table struct {
	name    string
	columns []string
	rows    []Row
}

func (s *Service) read(ctx context.Context, tx uow.Tx, name string) (table, error) {
	t := table{name: name}
	for offset := 0; ; offset += s.chunk {
		cols, rows, err := s.store.Rows(ctx, tx, name, offset, s.chunk)
		if err != nil {
			return table{}, fmt.Errorf("read %s: %w", name, err)
		}
		t.columns = cols
		for _, v := range rows {
			t.rows = append(t.rows, Row{Values: v, key: format(cols, v)})
		}
		if len(rows) < s.chunk {
			return t, nil
		}
	}
}

// diff returns the rows of before that after lacks and the rows of after that before lacks,
// counting repeated rows.
func diff(before, after table) TableDiff {
	d := TableDiff{Table: before.name, Columns: after.columns, Rows: len(before.rows)}
	if d.Columns == nil {
		d.Columns = before.columns
	}
	count := map[string]int{}
	for _, r := range after.rows {
		count[r.key]++
	}
	for _, r := range before.rows {
		if count[r.key] > 0 {
			count[r.key]--
			continue
		}
		d.Extra = append(d.Extra, r)
	}
	for _, r := range after.rows {
		if count[r.key] > 0 {
			count[r.key]--
			d.Missing = append(d.Missing, r)
		}
	}
	return d
}

func format(columns []string, values []any) string {
	var b strings.Builder
	for i, v := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		if i < len(columns) {
			b.WriteString(columns[i])
			b.WriteByte('=')
		}
		b.WriteString(formatValue(v))
	}
	return b.String()
}

func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case string:
		return fmt.Sprintf("%q", x)
	case []byte:
		return fmt.Sprintf("x'%x'", x)
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

// within is a UnitOfWork whose transactions run inside tx, so that a rebuild function's work
// rolls back with the comparison.
type within struct{ tx uow.Tx }

func (w within) Tx(_ context.Context, fn func(uow.Tx) error) error { return fn(w.tx) }
