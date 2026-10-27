// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package db

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/kv/db/internal/postgreskv"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// NewPostgres returns the store kept in the PostgreSQL database d, whose entries expire by clock.
func NewPostgres(d *postgres.DB, clock kernel.Clock) *Store {
	return &Store{db: d, st: postgresStore{}, clock: clock}
}

type postgresStore struct{}

var _ store = postgresStore{}

func postgresQueries(tx uow.Tx) (*postgreskv.Queries, error) {
	pgTx, err := postgres.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return postgreskv.New(pgTx), nil
}

func (postgresStore) get(ctx context.Context, tx uow.Tx, namespace, key string, now time.Time) ([]byte, bool, error) {
	q, err := postgresQueries(tx)
	if err != nil {
		return nil, false, err
	}
	v, err := q.GetEntry(ctx, postgreskv.GetEntryParams{Namespace: namespace, Key: key, Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return v, err == nil, err
}

func (postgresStore) set(ctx context.Context, tx uow.Tx, namespace, key string, value []byte, expiresAt *time.Time) error {
	q, err := postgresQueries(tx)
	if err != nil {
		return err
	}
	return q.SetEntry(ctx, postgreskv.SetEntryParams{Namespace: namespace, Key: key, Value: value, ExpiresAt: expiresAt})
}

func (postgresStore) del(ctx context.Context, tx uow.Tx, namespace, key string) error {
	q, err := postgresQueries(tx)
	if err != nil {
		return err
	}
	return q.DeleteEntry(ctx, postgreskv.DeleteEntryParams{Namespace: namespace, Key: key})
}

func (postgresStore) incr(ctx context.Context, tx uow.Tx, namespace, key string, initial []byte, expiresAt *time.Time, now time.Time) ([]byte, error) {
	q, err := postgresQueries(tx)
	if err != nil {
		return nil, err
	}
	return q.IncrEntry(ctx, postgreskv.IncrEntryParams{Namespace: namespace, Key: key, Initial: initial, ExpiresAt: expiresAt, Now: now})
}

func (postgresStore) swap(ctx context.Context, tx uow.Tx, namespace, key string, old, value []byte, expiresAt *time.Time, now time.Time) (bool, error) {
	q, err := postgresQueries(tx)
	if err != nil {
		return false, err
	}
	var n int64
	switch {
	case old == nil && value == nil:
		_, found, err := postgresStore{}.get(ctx, tx, namespace, key, now)
		return !found, err
	case old == nil:
		n, err = q.SwapAbsentEntry(ctx, postgreskv.SwapAbsentEntryParams{Namespace: namespace, Key: key, Value: value, ExpiresAt: expiresAt, Now: now})
	case value == nil:
		n, err = q.DeleteEntryIf(ctx, postgreskv.DeleteEntryIfParams{Namespace: namespace, Key: key, Old: old, Now: now})
	default:
		n, err = q.SwapEntry(ctx, postgreskv.SwapEntryParams{Namespace: namespace, Key: key, Old: old, Value: value, ExpiresAt: expiresAt, Now: now})
	}
	return n == 1, err
}

func (postgresStore) deleteExpired(ctx context.Context, tx uow.Tx, now time.Time, limit int) (int64, error) {
	q, err := postgresQueries(tx)
	if err != nil {
		return 0, err
	}
	n := int32(math.MaxInt32)
	if limit >= math.MinInt32 && limit < math.MaxInt32 {
		n = int32(limit)
	}
	return q.DeleteExpiredEntries(ctx, postgreskv.DeleteExpiredEntriesParams{Now: now, BatchLimit: n})
}
