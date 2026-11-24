// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/kernel"
)

// PoolName is the db.client.connection.pool.name of the pool of DB.
const PoolName = "postgres"

// RegisterMetrics reports the connection pool on m from pgxpool's statistics (ADR-0055 §10).
func (d *DB) RegisterMetrics(m kernel.Meter) {
	sysmetrics.RegisterPool(m, PoolName, func() sysmetrics.PoolStats {
		s := d.pool.Stat()
		timeouts := s.CanceledAcquireCount()
		return sysmetrics.PoolStats{
			Idle: int64(s.IdleConns()), Used: int64(s.AcquiredConns()), Max: int64(s.MaxConns()),
			Waits: s.EmptyAcquireCount(), WaitTime: s.EmptyAcquireWaitTime(), Timeouts: &timeouts,
		}
	})
}

// DatabaseSize implements metrics.DatabaseSizer: the bytes of the relations of the current schema
// (pg_total_relation_size of its tables, materialized views and sequences, with their indexes and
// TOAST), so that instances that share one database, each in its own schema, count their own.
func (d *DB) DatabaseSize(ctx context.Context) (int64, error) {
	var n int64
	if err := d.conns.QueryRow(ctx, schemaSizeSQL).Scan(&n); err != nil {
		return 0, fmt.Errorf("postgres: database size: %w", err)
	}
	return n, nil
}

// schemaSizeSQL sums the relations of current_schema(); 0 when no schema of the search_path exists.
const schemaSizeSQL = `SELECT COALESCE(sum(pg_total_relation_size(c.oid)), 0)::bigint
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'm', 'S')`
