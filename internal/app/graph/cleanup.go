// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

const (
	// JobCleanupMutes deletes the mutes that have expired (FR-803); no payload.
	JobCleanupMutes = "graph.cleanup_mutes"
	// DefaultCleanupInterval is the period of the cleanup schedule.
	DefaultCleanupInterval = time.Hour
	// cleanupBatch is the number of mutes one cleanup transaction deletes; cleanupMaxBatches
	// bounds the batches of one run, and the next run continues.
	cleanupBatch      = 500
	cleanupMaxBatches = 20
)

// RegisterJobs registers the graph job handlers on reg (the cleanup of mutes and the migration
// of the relationships of moved accounts) and the cleanup schedule on sched.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobCleanupMutes, s.handleCleanup)
	reg.Register(JobMigrateFollows, s.handleMigrate)
	sched.Add(queue.Schedule{Name: JobCleanupMutes, Every: DefaultCleanupInterval, Job: queue.NewJob{Type: JobCleanupMutes}})
}

func (s *Service) handleCleanup(ctx context.Context, _ queue.Job) queue.Result {
	if _, err := s.DeleteExpiredMutes(ctx); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// DeleteExpiredMutes deletes the mutes that have expired and returns how many it deleted. An
// expired mute already applies nowhere (readers filter it out); deleting it emits EventUnmuted,
// so that other modules' projections drop their rows of it too. Each batch is one short
// transaction, and one run deletes a bounded number of mutes; the next run continues.
func (s *Service) DeleteExpiredMutes(ctx context.Context) (int, error) {
	deleted := 0
	for range cleanupMaxBatches {
		var n int
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			mutes, err := s.d.Repository.DeleteExpiredMutes(ctx, tx, s.d.Clock.Now(), cleanupBatch)
			if err != nil {
				return err
			}
			n = len(mutes)
			for _, m := range mutes {
				if err := tx.Emit(ctx, Unmuted{AccountID: m.AccountID, TargetAccountID: m.TargetAccountID}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return deleted, err
		}
		deleted += n
		if n < cleanupBatch {
			break
		}
	}
	return deleted, nil
}
