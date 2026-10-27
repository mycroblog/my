// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/kv/db/internal/sqlitekv"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// NewSQLite returns the store kept in the SQLite database d, whose entries expire by clock.
func NewSQLite(d *sqlite.DB, clock kernel.Clock) *Store {
	return &Store{db: d, st: sqliteStore{d}, clock: clock}
}

// sqliteStore stores timestamps as Unix milliseconds (data-model §1.2).
type sqliteStore struct{ db *sqlite.DB }

var _ store = sqliteStore{}

func (s sqliteStore) queries(tx uow.Tx) (*sqlitekv.Queries, error) {
	sqlTx, err := s.db.SQLTx(tx)
	if err != nil {
		return nil, err
	}
	return sqlitekv.New(sqlTx), nil
}

func millis(t time.Time) sql.NullInt64 { return sql.NullInt64{Int64: t.UnixMilli(), Valid: true} }

func nullMillis(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return millis(*t)
}

func (s sqliteStore) get(ctx context.Context, tx uow.Tx, namespace, key string, now time.Time) ([]byte, bool, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, false, err
	}
	v, err := q.GetEntry(ctx, sqlitekv.GetEntryParams{Namespace: namespace, Key: key, Now: millis(now)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return v, err == nil, err
}

func (s sqliteStore) set(ctx context.Context, tx uow.Tx, namespace, key string, value []byte, expiresAt *time.Time) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.SetEntry(ctx, sqlitekv.SetEntryParams{Namespace: namespace, Key: key, Value: value, ExpiresAt: nullMillis(expiresAt)})
}

func (s sqliteStore) del(ctx context.Context, tx uow.Tx, namespace, key string) error {
	q, err := s.queries(tx)
	if err != nil {
		return err
	}
	return q.DeleteEntry(ctx, sqlitekv.DeleteEntryParams{Namespace: namespace, Key: key})
}

func (s sqliteStore) incr(ctx context.Context, tx uow.Tx, namespace, key string, initial []byte, expiresAt *time.Time, now time.Time) ([]byte, error) {
	q, err := s.queries(tx)
	if err != nil {
		return nil, err
	}
	err = q.DeleteExpiredEntry(ctx, sqlitekv.DeleteExpiredEntryParams{Namespace: namespace, Key: key, Now: millis(now)})
	if err != nil {
		return nil, err
	}
	return q.IncrEntry(ctx, sqlitekv.IncrEntryParams{Namespace: namespace, Key: key, Initial: initial, ExpiresAt: nullMillis(expiresAt)})
}

// swap reads and writes in one transaction, which BEGIN IMMEDIATE makes atomic.
func (s sqliteStore) swap(ctx context.Context, tx uow.Tx, namespace, key string, old, value []byte, expiresAt *time.Time, now time.Time) (bool, error) {
	cur, found, err := s.get(ctx, tx, namespace, key, now)
	switch {
	case err != nil:
		return false, err
	case old == nil && found, old != nil && (!found || !bytes.Equal(cur, old)):
		return false, nil
	case value == nil:
		return true, s.del(ctx, tx, namespace, key)
	}
	return true, s.set(ctx, tx, namespace, key, value, expiresAt)
}

func (s sqliteStore) deleteExpired(ctx context.Context, tx uow.Tx, now time.Time, limit int) (int64, error) {
	q, err := s.queries(tx)
	if err != nil {
		return 0, err
	}
	return q.DeleteExpiredEntries(ctx, sqlitekv.DeleteExpiredEntriesParams{Now: millis(now), BatchLimit: int64(limit)})
}
