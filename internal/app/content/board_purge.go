// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

const (
	// JobPurgeBoards deletes the posts of the boards of the events whose records are past their
	// retention (ADR-0161 §14, §17); no payload.
	JobPurgeBoards = "content.purge_boards"
	// BoardPurgeInterval is the period of the purge's schedule.
	BoardPurgeInterval = time.Hour
	// BoardPurgeBatch is how many posts one transaction of JobPurgeBoards deletes, and how many
	// events one read of the boards takes.
	BoardPurgeBatch = 100
)

// BoardPurgeDeps are the ports a BoardPurge uses.
type BoardPurgeDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	// Boards tells which events' boards go now; nil purges none.
	Boards Boards
	Clock  kernel.Clock
}

// BoardPurge deletes the posts of the boards of the events with the records of the events
// (ADR-0161 §17): event_records_retention_days after the event ended or was cancelled, except
// while a report points at the event. Each run reads the events whose boards still have posts
// (status_gathering_boards, which loses its rows as the purged posts go), asks the gathering
// module which of them are due (Boards.BoardsToPurge), and deletes their posts a batch per
// transaction, each with its deletion and BoardStatusesDeleted; so the posts of an event go
// whether or not anyone answered it. The deleted posts' rows go with the purge of deleted
// statuses, which keeps those that a report or a hold keeps.
type BoardPurge struct{ d BoardPurgeDeps }

// NewBoardPurge returns a BoardPurge on d.
func NewBoardPurge(d BoardPurgeDeps) *BoardPurge { return &BoardPurge{d: d} }

// RegisterJobs registers the JobPurgeBoards handler and its hourly schedule.
func (b *BoardPurge) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeBoards, b.handle)
	sched.Add(queue.Schedule{Name: JobPurgeBoards, Every: BoardPurgeInterval, Job: queue.NewJob{Type: JobPurgeBoards}})
}

func (b *BoardPurge) handle(ctx context.Context, _ queue.Job) queue.Result {
	if _, err := b.Run(ctx); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run deletes the posts of every board whose event is due (Boards.BoardsToPurge) and returns how
// many it deleted. It is safe to repeat.
func (b *BoardPurge) Run(ctx context.Context) (int, error) {
	if b.d.Boards == nil {
		return 0, nil
	}
	total, after := 0, ""
	for {
		ids, err := b.d.Repository.ListBoardGatheringIDs(ctx, after, BoardPurgeBatch)
		if err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		after = ids[len(ids)-1]
		due, err := b.d.Boards.BoardsToPurge(ctx, ids)
		if err != nil {
			return total, err
		}
		n, err := b.Purge(ctx, due)
		total += n
		if err != nil {
			return total, err
		}
		if len(ids) < BoardPurgeBatch {
			return total, nil
		}
	}
}

// Purge deletes every post of the boards of gatheringIDs that is not deleted, BoardPurgeBatch
// posts per transaction, and returns how many it deleted. It is safe to repeat.
func (b *BoardPurge) Purge(ctx context.Context, gatheringIDs []string) (int, error) {
	if len(gatheringIDs) == 0 {
		return 0, nil
	}
	total := 0
	for {
		n := 0
		err := b.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = b.purgeBatch(ctx, tx, gatheringIDs)
			return err
		})
		total += n
		if err != nil {
			return total, fmt.Errorf("content: purge the boards of %d events: %w", len(gatheringIDs), err)
		}
		if n < BoardPurgeBatch {
			return total, nil
		}
	}
}

// purgeBatch deletes up to BoardPurgeBatch posts inside tx and returns how many it deleted.
func (b *BoardPurge) purgeBatch(ctx context.Context, tx uow.Tx, gatheringIDs []string) (int, error) {
	ids, err := b.d.Repository.ListLiveBoardStatusIDsOf(ctx, tx, gatheringIDs, BoardPurgeBatch)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	statuses, err := b.d.Repository.FindStatuses(ctx, ids)
	if err != nil {
		return 0, err
	}
	now := b.d.Clock.Now()
	ds := make([]content.Deletion, 0, len(ids))
	events := make([]StatusDeleted, 0, len(ids))
	for _, id := range ids {
		st, ok := statuses[id]
		if !ok {
			continue
		}
		d := content.Deletion{
			StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByEventRecords, DeletedAt: now,
		}
		ds = append(ds, d)
		events = append(events, StatusDeleted{Deletion: d, Status: st})
	}
	if len(ds) == 0 {
		return 0, nil
	}
	if err := b.d.Repository.InsertDeletions(ctx, tx, ds); err != nil {
		return 0, err
	}
	if err := tx.Emit(ctx, BoardStatusesDeleted{Deletions: events}); err != nil {
		return 0, err
	}
	return len(ds), nil
}
