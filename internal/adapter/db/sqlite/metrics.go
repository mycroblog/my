// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/kernel"
)

// PoolName is the db.client.connection.pool.name of the pool of DB.
const PoolName = "sqlite"

// RegisterMetrics reports the connection pool on m from database/sql's statistics (ADR-0056
// §1). database/sql does not count the acquisitions that gave up, so there is no timeouts
// gauge.
func (d *DB) RegisterMetrics(m kernel.Meter) {
	sysmetrics.RegisterPool(m, PoolName, func() sysmetrics.PoolStats {
		s := d.db.Stats()
		return sysmetrics.PoolStats{
			Idle: int64(s.Idle), Used: int64(s.InUse), Max: int64(s.MaxOpenConnections),
			Waits: s.WaitCount, WaitTime: s.WaitDuration,
		}
	})
}

// DatabaseSize implements metrics.DatabaseSizer: the size of the database file, pages on the
// free list included (page_count × page_size). The write-ahead log is left out: a checkpoint
// moves it into the file.
func (d *DB) DatabaseSize(ctx context.Context) (int64, error) {
	var n int64
	if err := d.db.QueryRowContext(ctx, "SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()").Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: database size: %w", err)
	}
	return n, nil
}
