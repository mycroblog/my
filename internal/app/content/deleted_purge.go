// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

const (
	// JobPurgeDeletedStatuses deletes the rows of the statuses whose purge is due
	// (status_content_purges). It has no payload.
	JobPurgeDeletedStatuses = "content.purge_deleted_statuses"
	// DeletedContentRetention is how long a deleted status keeps its rows after its deletion.
	// The Delete of a local status carries its own document, but a job recorded before the
	// deletion (the fan-out of an Update) still reads the rows, and every delivery retries on
	// federation.DeliveryRetryPolicy for about 4 days; 7 days lets them all end first.
	DeletedContentRetention = 7 * 24 * time.Hour
	// DeletedContentRecheck is how much later the purge of a status that the moderation records
	// keep comes due again. A report, an action, or a hold keeps a status for days or months, so
	// a daily look finds it soon enough once it ends, and the hourly run does not read the kept
	// statuses again each time.
	DeletedContentRecheck = 24 * time.Hour
)

// PurgeDue returns when the rows of the status that d deletes are due to go, and false
// for a deletion by retention or by the erasure of an account, which purge the rows themselves.
// The database adapters record the purge with the deletion (InsertDeletion, InsertDeletions).
func PurgeDue(d content.Deletion) (time.Time, bool) {
	switch d.Origin {
	case content.DeletedByRetention, content.DeletedByAccountDeletion:
		return time.Time{}, false
	default:
		return d.DeletedAt.Add(DeletedContentRetention), true
	}
}

// DueContentPurge is a purge of a deleted status's rows that is due.
type DueContentPurge struct {
	StatusID        string
	AuthorAccountID string
	// Present is whether the status still has its row; an erasure may have removed it first.
	Present bool
}

// DeletedPurgeRepository is the part of the content module's persistence that the purge of
// deleted statuses uses; the database adapters' status repositories implement it.
type DeletedPurgeRepository interface {
	// ListDueContentPurges returns up to limit purges due at dueBefore or earlier, the earliest
	// first.
	ListDueContentPurges(ctx context.Context, tx uow.Tx, dueBefore time.Time, limit int) ([]DueContentPurge, error)
	// PurgeDeletedStatuses deletes what RetentionRepository.PurgeStatuses deletes of deleted
	// statuses (their purges included), and their local rows. The deletions stay.
	PurgeDeletedStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error
	// PostponeContentPurges moves the purges of the statuses to dueAt.
	PostponeContentPurges(ctx context.Context, tx uow.Tx, statusIDs []string, dueAt time.Time) error
	// DeleteContentPurges deletes the purges of the statuses.
	DeleteContentPurges(ctx context.Context, tx uow.Tx, statusIDs []string) error
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
// statuses' own rows. The deletion stays as the tombstone of the URI. It reads only the purges
// that are due (status_content_purges), so a run costs what it has to do, however many
// tombstones there are. A status that a report or a moderation action points at, or whose author
// a retention hold preserves, keeps its rows, and its purge comes due again
// DeletedContentRecheck later (ADR-0058 §8).
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
	more, _, err := p.Run(ctx)
	if err != nil {
		return queue.Retry(0, err)
	}
	if !more || p.d.Jobs == nil {
		return queue.Done()
	}
	// The key names the job that continues, so that a retry of this one enqueues it once.
	if err := p.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobPurgeDeletedStatuses, IdempotencyKey: "purge_deleted_statuses:" + j.ID}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run handles the purges due now, in a bounded number of batches of one short transaction each,
// and returns whether more may be due and how many statuses it purged. Each batch reads the
// earliest due purges and takes every one of them out of the due ones (purged, or postponed to
// after now), so the batches end. It is safe to repeat.
func (p *DeletedPurge) Run(ctx context.Context) (more bool, purged int, err error) {
	now := kernel.Normalize(p.d.Clock.Now())
	for range retentionMaxBatches {
		var n, handled int
		err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, handled, err = p.purgeBatch(ctx, tx, now)
			return err
		})
		if err != nil {
			return false, purged, err
		}
		purged += n
		if handled < p.d.Batch {
			return false, purged, nil
		}
	}
	return true, purged, nil
}

// purgeBatch handles one batch of the purges due at now inside tx and returns how many statuses
// it purged and how many purges it handled.
func (p *DeletedPurge) purgeBatch(ctx context.Context, tx uow.Tx, now time.Time) (purged, handled int, err error) {
	due, err := p.d.Repository.ListDueContentPurges(ctx, tx, now, p.d.Batch)
	if err != nil {
		return 0, 0, err
	}
	var ids, kept, gone []string
	for _, d := range due {
		if !d.Present {
			gone = append(gone, d.StatusID)
			continue
		}
		// The evidence of a report or an action, and the statuses of a held author, stay.
		if p.d.Evidence != nil {
			retained, err := p.d.Evidence.StatusRetained(ctx, tx, d.StatusID, d.AuthorAccountID)
			if err != nil {
				return 0, 0, err
			}
			if retained {
				kept = append(kept, d.StatusID)
				continue
			}
		}
		ids = append(ids, d.StatusID)
	}
	if len(gone) > 0 {
		if err := p.d.Repository.DeleteContentPurges(ctx, tx, gone); err != nil {
			return 0, 0, err
		}
	}
	if len(kept) > 0 {
		if err := p.d.Repository.PostponeContentPurges(ctx, tx, kept, now.Add(DeletedContentRecheck)); err != nil {
			return 0, 0, err
		}
	}
	if len(ids) > 0 {
		if err := p.d.Repository.PurgeDeletedStatuses(ctx, tx, ids); err != nil {
			return 0, 0, err
		}
	}
	return len(ids), len(due), nil
}
