// SPDX-License-Identifier: AGPL-3.0-only

// Package rewrap implements rewrap.Store on SQLite: the sealed columns of the modules, which
// the sub-packages of the modules return (SealedColumns), gathered into one store.
package rewrap

import (
	"context"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqliteinstance "github.com/mycroblog/my/internal/adapter/db/sqlite/instance"
	sqlitewebhook "github.com/mycroblog/my/internal/adapter/db/sqlite/webhook"
	apprewrap "github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

// SealedValueStore implements rewrap.Store on the sealed columns of the modules.
type SealedValueStore struct {
	names   []string
	columns map[string]sqlite.SealedColumn
}

var _ apprewrap.Store = (*SealedValueStore)(nil)

// NewSealedValueStore returns the store of the sealed values on d that `my kek rewrap` re-wraps.
func NewSealedValueStore(d *sqlite.DB) *SealedValueStore {
	s := &SealedValueStore{columns: map[string]sqlite.SealedColumn{}}
	for _, c := range slices.Concat(sqliteinstance.SealedColumns(d), sqliteaccount.SealedColumns(d), sqliteidentity.SealedColumns(d), sqlitewebhook.SealedColumns(d)) {
		s.names = append(s.names, c.Name)
		s.columns[c.Name] = c
	}
	return s
}

// Columns implements rewrap.Store.
func (s *SealedValueStore) Columns() []string { return slices.Clone(s.names) }

// List implements rewrap.Store.
func (s *SealedValueStore) List(ctx context.Context, column, after string, limit int) ([]apprewrap.Value, error) {
	c, ok := s.columns[column]
	if !ok {
		return nil, fmt.Errorf("%w: %q", apprewrap.ErrUnknownColumn, column)
	}
	out, err := c.List(ctx, after, limit)
	return out, sqlite.Wrap(err, "list sealed values of "+column)
}

// Replace implements rewrap.Store.
func (s *SealedValueStore) Replace(ctx context.Context, tx uow.Tx, column, rowID string, old, sealed []byte) (bool, error) {
	c, ok := s.columns[column]
	if !ok {
		return false, fmt.Errorf("%w: %q", apprewrap.ErrUnknownColumn, column)
	}
	n, err := c.Replace(ctx, tx, rowID, old, sealed)
	if err != nil {
		return false, sqlite.Wrap(err, "replace sealed value of "+column)
	}
	return n > 0, nil
}
