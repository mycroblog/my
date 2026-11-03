// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

const (
	// JobCleanupKV deletes the expired entries of the key-value store (FR-803); no payload.
	JobCleanupKV = "kv.purge_expired"
	// kvCleanupInterval is the period of the key-value store's cleanup schedule.
	kvCleanupInterval = time.Hour
	// kvCleanupBatch is the number of entries one DeleteExpired call (one short transaction)
	// removes; kvCleanupMaxBatches bounds the batches of one run, and the next run continues.
	kvCleanupBatch      = 1000
	kvCleanupMaxBatches = 50
)

// registerKVCleanup registers the key-value store's cleanup job and its hourly schedule. The
// store's entries behave as absent once expired; the job removes them physically, so that keys
// made per window (rate limits) do not pile up.
func (a *App) registerKVCleanup() {
	a.Handlers.Register(JobCleanupKV, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := a.deleteExpiredKV(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	a.Scheduler.Add(queue.Schedule{Name: JobCleanupKV, Every: kvCleanupInterval, Job: queue.NewJob{Type: JobCleanupKV}})
}

// deleteExpiredKV removes expired entries in batches and returns how many it removed.
func (a *App) deleteExpiredKV(ctx context.Context) (int64, error) {
	var deleted int64
	for range kvCleanupMaxBatches {
		n, err := a.KV.DeleteExpired(ctx, kvCleanupBatch)
		if err != nil {
			return deleted, fmt.Errorf("platform: kv cleanup: %w", err)
		}
		deleted += n
		if n < kvCleanupBatch {
			break
		}
	}
	return deleted, nil
}
