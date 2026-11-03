// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

const (
	// RetentionDeletionRetention is how long a deletion by retention (status_deletions) is kept.
	RetentionDeletionRetention = 30 * 24 * time.Hour
	// JobPurgeRemoteStatuses deletes the remote statuses past their retention (ADR-0034,
	// FR-803). Its payload is the cursor of the run (none for the first job of a run).
	JobPurgeRemoteStatuses = "content.purge_remote_statuses"
	// DefaultRemoteStatusRetention is how long a remote status that nothing local refers to is
	// kept after its publication (the remote_status_retention setting).
	DefaultRemoteStatusRetention = 90 * 24 * time.Hour
	// RetentionInterval is the period of the purge's schedule.
	RetentionInterval = time.Hour
	// RetentionBatch is the number of statuses one purge transaction deletes;
	// retentionMaxBatches bounds the batches of one job, and a job that used them up enqueues
	// the next with its cursor.
	RetentionBatch      = 100
	retentionMaxBatches = 10
)

// RetentionRepository is the part of the content module's persistence that the retention purge
// uses. The database adapters' status repositories implement it and pass
// testkit.RetentionContract.
type RetentionRepository interface {
	// ListRemoteStatusesToPurge returns up to q.Limit remote statuses that are not deleted, in
	// ID order after q.After, that were published before q.Before regardless of references, or
	// before q.UnreferencedBefore while no local status replies to them or shares their thread
	// and no local account boosts (unretracted), favourites, bookmarks, or pins them.
	ListRemoteStatusesToPurge(ctx context.Context, tx uow.Tx, q PurgeQuery) ([]content.Status, error)
	// ListRelayedStatusesToPurge returns up to q.Limit statuses that only a relay brought in and
	// that are not deleted, in ID order after q.After, published before q.Before while nothing
	// local refers to them (as ListRemoteStatusesToPurge judges it).
	ListRelayedStatusesToPurge(ctx context.Context, tx uow.Tx, q RelayedPurgeQuery) ([]content.Status, error)
	// PurgeStatuses deletes the rows of deleted statuses: their revisions with every child and
	// poll, the boosts of remote accounts, the record of a relay that brought them in, and the
	// statuses. The deletions stay, and so do local accounts' boosts, favourites, bookmarks, and
	// pins.
	PurgeStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error
	// DeleteRetentionDeletionsBefore deletes at most limit deletions by retention recorded before
	// t whose status has no row and no boost, favourite, reaction, bookmark, or pin, and returns
	// how many it deleted.
	DeleteRetentionDeletionsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
}

// PurgeQuery selects the remote statuses past their retention.
type PurgeQuery struct {
	// After is the cursor: a status ID, "" for the first.
	After string
	// UnreferencedBefore is the publication time before which a status that nothing local
	// refers to is purged.
	UnreferencedBefore time.Time
	// Before is the publication time before which every status is purged; zero for none.
	Before time.Time
	Limit  int
}

// NotifiedStatuses tells whether a notification is about a status, which keeps the status from
// the retention purge; *notification.Service implements it.
type NotifiedStatuses interface {
	StatusNotified(ctx context.Context, tx uow.Tx, statusID string) (bool, error)
}

// RetainedStatuses tells whether the moderation records keep a status from the purge: a report
// or an action points at it, or a retention hold preserves its author (ADR-0038). The
// composition root implements it on the moderation module, which imports this one.
type RetainedStatuses interface {
	StatusRetained(ctx context.Context, tx uow.Tx, statusID, authorAccountID string) (bool, error)
}

// RetentionDeps are the ports of the retention purge.
type RetentionDeps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    Repository
	Retention     RetentionRepository
	Notifications NotifiedStatuses
	// Evidence keeps the statuses that the moderation records need, past the maximum period
	// too; nil keeps none.
	Evidence RetainedStatuses
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs  queue.JobQueue
	Clock kernel.Clock
	// Period is how long an unreferenced remote status is kept after its publication; zero
	// means DefaultRemoteStatusRetention.
	Period time.Duration
	// MaxPeriod is how long any remote status is kept after its publication; zero keeps the
	// referenced ones without limit.
	MaxPeriod time.Duration
	// Batch is the number of statuses per transaction; zero means RetentionBatch.
	Batch int
}

// Retention purges the remote statuses past their retention (ADR-0034). A remote status
// published longer ago than the retention period that nothing local refers to (a live local
// reply, a live local status in its thread, an unretracted local boost, a favourite, a bookmark,
// a pin, or a notification) is deleted with origin retention; with a maximum period, any remote
// status published longer ago than it is. A status that a report or a moderation action points
// at, or whose author a retention hold preserves, is kept regardless (ADR-0038). The deletion leaves every projection as the author's
// deletion would, and its rows are purged in the same transaction; local accounts' boosts,
// favourites, bookmarks, and pins stay, and readers treat their status as deleted. A deletion by
// retention does not keep the URI from being fetched again, and goes after
// RetentionDeletionRetention once no row names its status.
type Retention struct{ d RetentionDeps }

// NewRetention returns the retention purge on d.
func NewRetention(d RetentionDeps) *Retention {
	if d.Period <= 0 {
		d.Period = DefaultRemoteStatusRetention
	}
	if d.MaxPeriod < 0 {
		d.MaxPeriod = 0
	}
	if d.Batch <= 0 {
		d.Batch = RetentionBatch
	}
	return &Retention{d: d}
}

// retentionPayload is the payload of JobPurgeRemoteStatuses: the cursor of the run.
type retentionPayload struct {
	After string `json:"after,omitempty"`
}

// RegisterJobs registers the JobPurgeRemoteStatuses handler and its hourly schedule.
func (r *Retention) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeRemoteStatuses, r.handle)
	sched.Add(queue.Schedule{Name: JobPurgeRemoteStatuses, Every: RetentionInterval, Job: queue.NewJob{Type: JobPurgeRemoteStatuses}})
}

func (r *Retention) handle(ctx context.Context, j queue.Job) queue.Result {
	var pl retentionPayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("content: retention payload %s: %w", j.Payload, err))
		}
	}
	next, _, err := r.Run(ctx, pl.After)
	if err != nil {
		return queue.Retry(0, err)
	}
	if next == "" || r.d.Jobs == nil {
		return queue.Done()
	}
	b, err := json.Marshal(retentionPayload{After: next})
	if err != nil {
		return queue.Fail(err)
	}
	if err := r.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobPurgeRemoteStatuses, Payload: b, IdempotencyKey: "purge_remote_statuses:" + next}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run purges the remote statuses past their retention after the cursor after ("" for the
// first), in a bounded number of batches of one short transaction each. It returns how many it
// purged and the cursor to continue from, "" when it reached the end. It is safe to repeat.
func (r *Retention) Run(ctx context.Context, after string) (next string, purged int, err error) {
	now := r.d.Clock.Now()
	if after == "" {
		if err := r.purgeDeletions(ctx, now.Add(-RetentionDeletionRetention)); err != nil {
			return after, 0, err
		}
	}
	q := PurgeQuery{After: after, UnreferencedBefore: now.Add(-r.d.Period), Limit: r.d.Batch}
	if r.d.MaxPeriod > 0 {
		q.Before = now.Add(-r.d.MaxPeriod)
	}
	for range retentionMaxBatches {
		var (
			n, examined int
			cursor      string
		)
		// The transaction may run again (a retry); it starts from the same cursor each time.
		err := r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, examined, cursor, err = r.purgeBatch(ctx, tx, q, now)
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

// purgeDeletions deletes the records of past purges, a bounded batch per transaction; the first
// job of the next run continues.
func (r *Retention) purgeDeletions(ctx context.Context, before time.Time) error {
	for range retentionMaxBatches {
		var n int64
		err := r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = r.d.Retention.DeleteRetentionDeletionsBefore(ctx, tx, before, r.d.Batch)
			return err
		})
		if err != nil {
			return err
		}
		if n < int64(r.d.Batch) {
			return nil
		}
	}
	return nil
}

// purgeBatch deletes one batch inside tx and returns how many statuses it deleted, how many it
// examined, and the cursor after them.
func (r *Retention) purgeBatch(ctx context.Context, tx uow.Tx, q PurgeQuery, now time.Time) (deleted, examined int, cursor string, err error) {
	statuses, err := r.d.Retention.ListRemoteStatusesToPurge(ctx, tx, q)
	if err != nil {
		return 0, 0, q.After, err
	}
	cursor = q.After
	var ids []string
	for _, st := range statuses {
		cursor = st.ID
		// The evidence of a report or an action, and the statuses of a held author, stay.
		if r.d.Evidence != nil {
			retained, err := r.d.Evidence.StatusRetained(ctx, tx, st.ID, st.AuthorAccountID)
			if err != nil {
				return 0, 0, cursor, err
			}
			if retained {
				continue
			}
		}
		// A notification refers to the status; past the maximum period it does not count.
		if q.Before.IsZero() || !st.PublishedAt.Before(q.Before) {
			notified, err := r.d.Notifications.StatusNotified(ctx, tx, st.ID)
			if err != nil {
				return 0, 0, cursor, err
			}
			if notified {
				continue
			}
		}
		d := content.Deletion{
			StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByRetention, DeletedAt: now,
		}
		if err := r.d.Repository.InsertDeletion(ctx, tx, d); err != nil {
			if errors.Is(err, ErrAlreadyDeleted) {
				err = fmt.Errorf("purge %s, deleted meanwhile: %w", st.URI, err)
			}
			return 0, 0, cursor, err
		}
		if err := tx.Emit(ctx, StatusDeleted{Deletion: d, Status: st}); err != nil {
			return 0, 0, cursor, err
		}
		ids = append(ids, st.ID)
	}
	if len(ids) > 0 {
		if err := r.d.Retention.PurgeStatuses(ctx, tx, ids); err != nil {
			return 0, 0, cursor, err
		}
	}
	return len(ids), len(statuses), cursor, nil
}
