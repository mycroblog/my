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

// The remote statuses that only a relay brought in (ADR-0061): the federation module records
// them in relayed_statuses when it takes them in, and they are kept for relay_status_retention
// instead of remote_status_retention_days, unless something local refers to them or a local account
// follows their author.

const (
	// JobPurgeRelayedStatuses deletes the relayed statuses past their retention. Its payload is
	// the cursor of the run (none for the first job of a run).
	JobPurgeRelayedStatuses = "content.purge_relayed_statuses"
	// DefaultRelayedStatusRetention is how long a relayed status is kept after its publication
	// (the relay_status_retention setting).
	DefaultRelayedStatusRetention = 7 * 24 * time.Hour
)

// RecordRelayedStatus records in tx that the remote status statusID came in through the relay
// relayID alone, at at. Recording it again changes nothing.
func (s *Service) RecordRelayedStatus(ctx context.Context, tx uow.Tx, statusID, relayID string, at time.Time) error {
	return s.d.Repository.InsertRelayedStatus(ctx, tx, statusID, relayID, at)
}

// RelayedPurgeQuery selects the relayed statuses past their retention.
type RelayedPurgeQuery struct {
	// After is the cursor: a status ID, "" for the first.
	After string
	// Before is the publication time before which a relayed status that nothing local refers to
	// is purged.
	Before time.Time
	Limit  int
}

// FollowedAuthors tells whether a local account follows a remote author, which keeps the
// author's relayed statuses as any other remote status. The composition root implements it on
// the graph module.
type FollowedAuthors interface {
	AuthorFollowed(ctx context.Context, accountID string) (bool, error)
}

// RelayedRetentionDeps are the ports of the purge of the relayed statuses.
type RelayedRetentionDeps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    Repository
	Retention     RetentionRepository
	Notifications NotifiedStatuses
	// Evidence keeps the statuses that the moderation records need; nil keeps none.
	Evidence RetainedStatuses
	Follows  FollowedAuthors
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs  queue.JobQueue
	Clock kernel.Clock
	// Period returns how long a relayed status is kept after its publication (the instance
	// setting relay_status_retention_days), read per run; zero means
	// DefaultRelayedStatusRetention.
	Period func(ctx context.Context) (time.Duration, error)
	// Batch is the number of statuses per transaction; zero means RetentionBatch.
	Batch int
}

// RelayedRetention purges the remote statuses that only a relay brought in, published longer
// ago than the period, that nothing local refers to (as Retention judges it) and whose author no
// local account follows. They are deleted with origin retention, as the retention purge deletes
// remote statuses (ADR-0034); the others stay for that purge.
type RelayedRetention struct{ d RelayedRetentionDeps }

// NewRelayedRetention returns the purge of the relayed statuses on d.
func NewRelayedRetention(d RelayedRetentionDeps) *RelayedRetention {
	if d.Batch <= 0 {
		d.Batch = RetentionBatch
	}
	return &RelayedRetention{d: d}
}

// RegisterJobs registers the JobPurgeRelayedStatuses handler and its hourly schedule.
func (r *RelayedRetention) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeRelayedStatuses, r.handle)
	sched.Add(queue.Schedule{Name: JobPurgeRelayedStatuses, Every: RetentionInterval, Job: queue.NewJob{Type: JobPurgeRelayedStatuses}})
}

func (r *RelayedRetention) handle(ctx context.Context, j queue.Job) queue.Result {
	var pl retentionPayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("content: relayed retention payload %s: %w", j.Payload, err))
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
	if err := r.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobPurgeRelayedStatuses, Payload: b, IdempotencyKey: "purge_relayed_statuses:" + next}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run purges the relayed statuses past their retention after the cursor after ("" for the
// first), in a bounded number of batches. Each batch reads its candidates, keeps those whose
// author no local account follows (read outside the transaction), and deletes them in one short
// transaction that reads the candidates again, so that a status that something local came to
// refer to meanwhile stays. It returns how many it purged and the cursor to continue from, ""
// when it reached the end. It is safe to repeat.
func (r *RelayedRetention) Run(ctx context.Context, after string) (next string, purged int, err error) {
	now := r.d.Clock.Now()
	period, err := r.d.Period(ctx)
	if err != nil {
		return after, 0, fmt.Errorf("content: relayed retention period: %w", err)
	}
	if period <= 0 {
		period = DefaultRelayedStatusRetention
	}
	q := RelayedPurgeQuery{After: after, Before: now.Add(-period), Limit: r.d.Batch}
	for range retentionMaxBatches {
		var candidates []content.Status
		err := r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			candidates, err = r.d.Retention.ListRelayedStatusesToPurge(ctx, tx, q)
			return err
		})
		if err != nil {
			return after, purged, err
		}
		if len(candidates) == 0 {
			return "", purged, nil
		}
		followed := map[string]bool{}
		for _, st := range candidates {
			if _, seen := followed[st.AuthorAccountID]; seen || r.d.Follows == nil {
				continue
			}
			if followed[st.AuthorAccountID], err = r.d.Follows.AuthorFollowed(ctx, st.AuthorAccountID); err != nil {
				return after, purged, err
			}
		}
		var n int
		err = r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = r.purgeBatch(ctx, tx, q, followed, now)
			return err
		})
		if err != nil {
			return after, purged, err
		}
		purged += n
		cursor := candidates[len(candidates)-1].ID
		q.After, after = cursor, cursor
		if len(candidates) < q.Limit {
			return "", purged, nil
		}
	}
	return after, purged, nil
}

// purgeBatch deletes, inside tx, the candidates of q that are still candidates, whose author is
// not followed, and that the moderation records and the notifications do not keep.
func (r *RelayedRetention) purgeBatch(ctx context.Context, tx uow.Tx, q RelayedPurgeQuery, followed map[string]bool, now time.Time) (int, error) {
	statuses, err := r.d.Retention.ListRelayedStatusesToPurge(ctx, tx, q)
	if err != nil {
		return 0, err
	}
	var ids []string
	for _, st := range statuses {
		if f, known := followed[st.AuthorAccountID]; f || (!known && r.d.Follows != nil) {
			continue
		}
		if r.d.Evidence != nil {
			retained, err := r.d.Evidence.StatusRetained(ctx, tx, st.ID, st.AuthorAccountID)
			if err != nil {
				return 0, err
			}
			if retained {
				continue
			}
		}
		notified, err := r.d.Notifications.StatusNotified(ctx, tx, st.ID)
		if err != nil {
			return 0, err
		}
		if notified {
			continue
		}
		d := content.Deletion{
			StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByRetention, DeletedAt: now,
		}
		if err := r.d.Repository.InsertDeletion(ctx, tx, d); err != nil {
			return 0, fmt.Errorf("purge the relayed %s: %w", st.URI, err)
		}
		if err := tx.Emit(ctx, StatusDeleted{Deletion: d, Status: st}); err != nil {
			return 0, err
		}
		ids = append(ids, st.ID)
	}
	if len(ids) > 0 {
		if err := r.d.Retention.PurgeStatuses(ctx, tx, ids); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}
