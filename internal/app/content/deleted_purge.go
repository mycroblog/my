// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

const (
	// JobPurgeDeletedStatuses deletes the rows of the statuses deleted longer ago than
	// DeletedContentRetention. Its payload is the cursor of the run (none for the first job of a
	// run).
	JobPurgeDeletedStatuses = "content.purge_deleted_statuses"
	// DeletedContentRetention is how long a deleted status keeps its rows after its deletion.
	// The Delete of a local status carries its own document, but a job recorded before the
	// deletion (the fan-out of an Update) still reads the rows, and every delivery retries on
	// federation.DeliveryRetryPolicy for about 4 days; 7 days lets them all end first.
	DeletedContentRetention = 7 * 24 * time.Hour
)

// DeletedPurgeRepository is the part of the content module's persistence that the purge of
// deleted statuses uses; the database adapters' status repositories implement it.
type DeletedPurgeRepository interface {
	// ListDeletedStatusesToPurge returns up to q.Limit deletions recorded before q.Before, in
	// status ID order after q.After, whose status still has its row; a deletion by the erasure of
	// an account is left to that erasure.
	ListDeletedStatusesToPurge(ctx context.Context, tx uow.Tx, q DeletedPurgeQuery) ([]content.Deletion, error)
	// PurgeDeletedStatuses deletes what RetentionRepository.PurgeStatuses deletes of deleted
	// statuses, and their local rows. The deletions stay.
	PurgeDeletedStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error
}

// DeletedPurgeQuery selects the deleted statuses whose rows go.
type DeletedPurgeQuery struct {
	// After is the cursor: a status ID, "" for the first.
	After string
	// Before is the time of deletion before which a status's rows go.
	Before time.Time
	Limit  int
}

// DeletedPurgeDeps are the ports of the purge of deleted statuses.
type DeletedPurgeDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository DeletedPurgeRepository
	// Evidence keeps the statuses that the moderation records need; nil keeps none.
	Evidence RetainedStatuses
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs  queue.JobQueue
	Clock kernel.Clock
	// Batch is the number of statuses per transaction; zero means RetentionBatch.
	Batch int
}

// DeletedPurge deletes the rows of the statuses deleted longer ago than DeletedContentRetention
// by their author, a moderator, the automatic deletion of old posts, or the Delete of a remote
// author: their revisions with every child and poll, as the retention purge does, and the
// statuses' own rows. The deletion stays as the tombstone of the URI. A status that a report or
// a moderation action points at, or whose author a retention hold preserves, keeps its rows
// until a later run finds neither (ADR-0058 §8).
type DeletedPurge struct{ d DeletedPurgeDeps }

// NewDeletedPurge returns the purge of deleted statuses on d.
func NewDeletedPurge(d DeletedPurgeDeps) *DeletedPurge {
	if d.Batch <= 0 {
		d.Batch = RetentionBatch
	}
	return &DeletedPurge{d: d}
}

// RegisterJobs registers the JobPurgeDeletedStatuses handler and its hourly schedule.
func (p *DeletedPurge) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeDeletedStatuses, p.handle)
	sched.Add(queue.Schedule{Name: JobPurgeDeletedStatuses, Every: RetentionInterval, Job: queue.NewJob{Type: JobPurgeDeletedStatuses}})
}

func (p *DeletedPurge) handle(ctx context.Context, j queue.Job) queue.Result {
	var pl retentionPayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("content: deleted purge payload %s: %w", j.Payload, err))
		}
	}
	next, _, err := p.Run(ctx, pl.After)
	if err != nil {
		return queue.Retry(0, err)
	}
	if next == "" || p.d.Jobs == nil {
		return queue.Done()
	}
	b, err := json.Marshal(retentionPayload{After: next})
	if err != nil {
		return queue.Fail(err)
	}
	if err := p.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobPurgeDeletedStatuses, Payload: b, IdempotencyKey: "purge_deleted_statuses:" + next}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run deletes the rows of the statuses deleted before DeletedContentRetention ago after the
// cursor after ("" for the first), in a bounded number of batches of one short transaction
// each. It returns how many statuses it purged and the cursor to continue from, "" when it
// reached the end. It is safe to repeat.
func (p *DeletedPurge) Run(ctx context.Context, after string) (next string, purged int, err error) {
	q := DeletedPurgeQuery{After: after, Before: p.d.Clock.Now().Add(-DeletedContentRetention), Limit: p.d.Batch}
	for range retentionMaxBatches {
		var (
			n, examined int
			cursor      string
		)
		// The transaction may run again (a retry); it starts from the same cursor each time.
		err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, examined, cursor, err = p.purgeBatch(ctx, tx, q)
			return err
		})
		if err != nil {
			return after, purged, err
		}
		purged += n
		q.After, after = cursor, cursor
		if examined < q.Limit {
			return "", purged, nil
		}
	}
	return after, purged, nil
}

// purgeBatch deletes one batch inside tx and returns how many statuses it purged, how many it
// examined, and the cursor after them.
func (p *DeletedPurge) purgeBatch(ctx context.Context, tx uow.Tx, q DeletedPurgeQuery) (purged, examined int, cursor string, err error) {
	deletions, err := p.d.Repository.ListDeletedStatusesToPurge(ctx, tx, q)
	if err != nil {
		return 0, 0, q.After, err
	}
	cursor = q.After
	var ids []string
	for _, d := range deletions {
		cursor = d.StatusID
		// The evidence of a report or an action, and the statuses of a held author, stay.
		if p.d.Evidence != nil {
			retained, err := p.d.Evidence.StatusRetained(ctx, tx, d.StatusID, d.AuthorAccountID)
			if err != nil {
				return 0, 0, cursor, err
			}
			if retained {
				continue
			}
		}
		ids = append(ids, d.StatusID)
	}
	if len(ids) > 0 {
		if err := p.d.Repository.PurgeDeletedStatuses(ctx, tx, ids); err != nil {
			return 0, 0, cursor, err
		}
	}
	return len(ids), len(deletions), cursor, nil
}
