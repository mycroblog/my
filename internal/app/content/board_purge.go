// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

const (
	// JobPurgeBoards deletes the posts of the boards of events whose records the gathering module
	// erased (ADR-0161 §14, §17); the payload names the events.
	JobPurgeBoards = "content.purge_boards"
	// BoardPurgeBatch is how many posts one transaction of JobPurgeBoards deletes.
	BoardPurgeBatch = 100
)

// BoardPurgeDeps are the ports a BoardPurge uses.
type BoardPurgeDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	// Outbox enqueues JobPurgeBoards in the transaction that erased the records.
	Outbox queue.Outbox
	Clock  kernel.Clock
}

// BoardPurge deletes the posts of the boards of events with the records of the events (ADR-0161
// §17). The gathering module erases the records and emits its domain event; the composition root
// enqueues the purge from it in the same transaction (EnqueueTx), so that this module never waits
// on the other, and the job deletes the posts a batch per transaction, each with its deletion and
// BoardStatusesDeleted. The deleted posts' rows go with the purge of deleted statuses, which keeps
// those that a report or a hold keeps.
type BoardPurge struct{ d BoardPurgeDeps }

// NewBoardPurge returns a BoardPurge on d.
func NewBoardPurge(d BoardPurgeDeps) *BoardPurge { return &BoardPurge{d: d} }

type purgeBoardsPayload struct {
	GatheringIDs []string `json:"gathering_ids"`
}

// EnqueueTx enqueues in tx the deletion of the posts of the boards of gatheringIDs.
func (b *BoardPurge) EnqueueTx(ctx context.Context, tx uow.Tx, gatheringIDs []string) error {
	if len(gatheringIDs) == 0 || b.d.Outbox == nil {
		return nil
	}
	payload, err := json.Marshal(purgeBoardsPayload{GatheringIDs: gatheringIDs})
	if err != nil {
		return err
	}
	return b.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobPurgeBoards, Payload: payload})
}

// RegisterJobs registers the JobPurgeBoards handler.
func (b *BoardPurge) RegisterJobs(reg *queue.JobHandlerRegistry) {
	reg.Register(JobPurgeBoards, b.handle)
}

func (b *BoardPurge) handle(ctx context.Context, j queue.Job) queue.Result {
	var p purgeBoardsPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("content: purge boards payload %s: %w", j.Payload, err))
	}
	if _, err := b.Purge(ctx, p.GatheringIDs); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Purge deletes every post of the boards of gatheringIDs that is not deleted, BoardPurgeBatch
// posts per transaction, and returns how many it deleted. It is safe to repeat.
func (b *BoardPurge) Purge(ctx context.Context, gatheringIDs []string) (int, error) {
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
