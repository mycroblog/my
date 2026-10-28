// SPDX-License-Identifier: AGPL-3.0-only

// Package rewrap re-wraps the sealed values stored in the database under the current
// key-encryption key (KEK), so that a KEK replaced by a rotation can be removed (`my kek`,
// ADR-0019 §6). It owns no tables: the composition root passes the sealed columns of the
// modules, each with the AAD that binds a value to its row, and a Store that the database
// adapters implement on those columns.
//
// Rewrap reads each column in batches ordered by row ID outside transactions, re-wraps the DEK of
// every value that a previous KEK wraps (kernel.Sealer.Rewrap, which keeps the encrypted secret
// and never calls the KeyManager inside a transaction), and writes a batch in one short
// transaction that replaces a value only while it still holds what was read. A value has one
// wrapped DEK, so running Rewrap again after a crash or next to another run re-wraps only what is
// still under a previous KEK, and a value that the server rewrote meanwhile is kept.
package rewrap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// DefaultBatchSize is how many values Rewrap reads and writes at a time.
const DefaultBatchSize = 100

// maxListedFailures is how many failed values a ColumnReport lists.
const maxListedFailures = 20

// Value is a stored sealed value and the ID of its row.
type Value struct {
	RowID  string
	Sealed []byte
}

// Store reads and replaces the sealed values of the columns it knows. The database adapters
// implement it and pass testkit.SealedValueStoreContract.
type Store interface {
	// Columns returns the names of the sealed columns, as "table.column", sorted.
	Columns() []string
	// List returns up to limit values of column whose row IDs are greater than after (the empty
	// string lists from the first), ordered by row ID. It fails with ErrUnknownColumn for a
	// column that Columns does not return.
	List(ctx context.Context, column, after string, limit int) ([]Value, error)
	// Replace sets the value of the row rowID of column to sealed inside tx when the row still
	// holds old, and reports whether it did.
	Replace(ctx context.Context, tx uow.Tx, column, rowID string, old, sealed []byte) (bool, error)
}

// ErrUnknownColumn is returned by a Store for a column it does not know.
var ErrUnknownColumn = errors.New("rewrap: unknown sealed column")

// Column is a sealed column and how to build the AAD that its values are bound to.
type Column struct {
	// Name is the column as "table.column", one of Store.Columns.
	Name string
	// AAD returns the additional authenticated data of the value of row rowID.
	AAD func(rowID string) []byte
}

// Options configure a Service.
type Options struct {
	UnitOfWork uow.UnitOfWork
	Store      Store
	Sealer     *kernel.Sealer
	// Columns are the sealed columns, one for each name of Store.Columns.
	Columns []Column
	// BatchSize is how many values a batch reads and writes; zero means DefaultBatchSize.
	BatchSize int
}

// Service reports and re-wraps the stored sealed values.
type Service struct {
	u       uow.UnitOfWork
	store   Store
	sealer  *kernel.Sealer
	columns []Column
	batch   int
}

// NewService returns the service of opts. It panics when the columns of opts are not exactly
// the columns of the Store, or a column has no AAD, so that no sealed column is left out.
func NewService(opts Options) *Service {
	names := make([]string, 0, len(opts.Columns))
	for _, c := range opts.Columns {
		if c.AAD == nil {
			panic(fmt.Sprintf("rewrap: column %s has no AAD", c.Name))
		}
		names = append(names, c.Name)
	}
	slices.Sort(names)
	if want := opts.Store.Columns(); !slices.Equal(names, want) {
		panic(fmt.Sprintf("rewrap: columns %v differ from the sealed columns of the store %v", names, want))
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = DefaultBatchSize
	}
	return &Service{u: opts.UnitOfWork, store: opts.Store, sealer: opts.Sealer, columns: slices.Clone(opts.Columns), batch: batch}
}

// CurrentKeyID returns the ID of the KEK that Rewrap wraps values under.
func (s *Service) CurrentKeyID() string { return s.sealer.CurrentKeyID() }

// KeyCount is how many values of a column one KEK wraps.
type KeyCount struct {
	KeyID string
	Count int
	// Current reports whether KeyID is the current KEK.
	Current bool
}

// ColumnStatus is how many values of a column each KEK wraps.
type ColumnStatus struct {
	Column string
	// Keys are ordered with the current KEK first, then by key ID.
	Keys []KeyCount
	// Malformed counts the values whose header cannot be read.
	Malformed int
}

// Total returns the number of values of the column.
func (c ColumnStatus) Total() int {
	n := c.Malformed
	for _, k := range c.Keys {
		n += k.Count
	}
	return n
}

// Outdated returns the number of values that are not under the current KEK.
func (c ColumnStatus) Outdated() int {
	n := c.Malformed
	for _, k := range c.Keys {
		if !k.Current {
			n += k.Count
		}
	}
	return n
}

// Status counts the values of each column by the KEK that wraps them, reading only the key IDs
// of their headers: it decrypts nothing.
func (s *Service) Status(ctx context.Context) ([]ColumnStatus, error) {
	current := s.sealer.CurrentKeyID()
	out := make([]ColumnStatus, 0, len(s.columns))
	for _, c := range s.columns {
		counts := map[string]int{}
		st := ColumnStatus{Column: c.Name}
		err := s.scan(ctx, c, func(batch []Value) error {
			for _, v := range batch {
				id, err := kernel.SealedKeyID(v.Sealed)
				if err != nil {
					st.Malformed++
					continue
				}
				counts[id]++
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		for id, n := range counts {
			st.Keys = append(st.Keys, KeyCount{KeyID: id, Count: n, Current: id == current})
		}
		sort.Slice(st.Keys, func(i, j int) bool {
			if st.Keys[i].Current != st.Keys[j].Current {
				return st.Keys[i].Current
			}
			return st.Keys[i].KeyID < st.Keys[j].KeyID
		})
		out = append(out, st)
	}
	return out, nil
}

// Failure is a value that Rewrap could not re-wrap.
type Failure struct {
	RowID string
	// KeyID is the KEK that wraps the value, or empty when its header cannot be read.
	KeyID string
	// Err wraps kernel.ErrUnknownKey when the KeyManager does not hold KeyID, and
	// kernel.ErrDecrypt when the value is malformed or does not authenticate.
	Err error
}

// ColumnReport is what Rewrap did to the values of a column.
type ColumnReport struct {
	Column string
	// Rewrapped counts the values re-wrapped under the current KEK.
	Rewrapped int
	// Current counts the values that the current KEK already wrapped.
	Current int
	// Changed counts the values that were rewritten or deleted between the read and the write;
	// the server writes under the current KEK, so they need nothing.
	Changed int
	// Failed counts the values that could not be re-wrapped; they are kept as they are.
	Failed int
	// Failures lists the first of the failed values.
	Failures []Failure
}

// Rewrap re-wraps every stored value that a previous KEK wraps under the current KEK, column by
// column, and calls done, when not nil, after each column. A value that cannot be re-wrapped,
// because the KeyManager does not hold its KEK or it does not authenticate, is kept and counted
// in the report; the other values are still re-wrapped. Rewrap returns an error only when the
// database fails, with the reports of the columns it completed; running it again resumes.
func (s *Service) Rewrap(ctx context.Context, done func(ColumnReport)) ([]ColumnReport, error) {
	current := s.sealer.CurrentKeyID()
	var reports []ColumnReport
	for _, c := range s.columns {
		r := ColumnReport{Column: c.Name}
		err := s.scan(ctx, c, func(batch []Value) error {
			return s.rewrapBatch(ctx, c, current, batch, &r)
		})
		if err != nil {
			return reports, fmt.Errorf("rewrap %s: %w", c.Name, err)
		}
		if done != nil {
			done(r)
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// replacement is a value of a batch and the value that re-wraps it.
type replacement struct {
	Value
	sealed []byte
}

func (s *Service) rewrapBatch(ctx context.Context, c Column, current string, batch []Value, r *ColumnReport) error {
	var pending []replacement
	for _, v := range batch {
		id, err := kernel.SealedKeyID(v.Sealed)
		if err != nil {
			r.fail(Failure{RowID: v.RowID, Err: err})
			continue
		}
		if id == current {
			r.Current++
			continue
		}
		// Re-wrapping calls the KeyManager, so it runs before the transaction.
		sealed, err := s.sealer.Rewrap(ctx, v.Sealed, c.AAD(v.RowID))
		if err != nil {
			r.fail(Failure{RowID: v.RowID, KeyID: id, Err: err})
			continue
		}
		pending = append(pending, replacement{Value: v, sealed: sealed})
	}
	if len(pending) == 0 {
		return nil
	}
	var replaced int
	err := s.u.Tx(ctx, func(tx uow.Tx) error {
		replaced = 0 // the transaction may run again
		for _, p := range pending {
			ok, err := s.store.Replace(ctx, tx, c.Name, p.RowID, p.Sealed, p.sealed)
			if err != nil {
				return err
			}
			if ok {
				replaced++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Rewrapped += replaced
	r.Changed += len(pending) - replaced
	return nil
}

func (r *ColumnReport) fail(f Failure) {
	r.Failed++
	if len(r.Failures) < maxListedFailures {
		r.Failures = append(r.Failures, f)
	}
}

// scan calls fn with the values of column c, batch by batch in row ID order.
func (s *Service) scan(ctx context.Context, c Column, fn func([]Value) error) error {
	after := ""
	for {
		batch, err := s.store.List(ctx, c.Name, after, s.batch)
		if err != nil {
			return err
		}
		if len(batch) > 0 {
			if err := fn(batch); err != nil {
				return err
			}
			after = batch[len(batch)-1].RowID
		}
		if len(batch) < s.batch {
			return nil
		}
	}
}
