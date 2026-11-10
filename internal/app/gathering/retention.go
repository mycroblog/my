// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// The retention of the answers and the invitations of the events (ADR-0161 §14): the hourly
// JobPurgeRecords erases them event_records_retention_days after an event ended or was cancelled,
// except those of an event that a report points at and those that name an account that a
// retention hold preserves. The events, their revisions, dates, co-organizers, and cancellations
// stay.

const (
	// JobPurgeRecords erases the answers, approvals, and invitations of the events that ended or
	// were cancelled longer than event_records_retention_days ago (ADR-0161 §14); no payload.
	JobPurgeRecords = "gathering.purge_records"
	// RecordPurgeInterval is the period of the purge's schedule.
	RecordPurgeInterval = time.Hour
	// RecordPurgeBatch is the number of events one transaction of the purge erases the records
	// of; recordPurgeMaxBatches bounds the batches of one run, and the next run continues.
	RecordPurgeBatch      = 100
	recordPurgeMaxBatches = 20
)

// Retention reads what the purge of the records keeps; the composition root implements it on
// the instance and moderation modules, so that a change applies at the next run.
type Retention interface {
	// RecordsRetention is the instance setting event_records_retention_days.
	RecordsRetention(ctx context.Context) (time.Duration, error)
	// ReportedGatherings returns which of gatheringIDs a report points at.
	ReportedGatherings(ctx context.Context, gatheringIDs []string) (map[string]bool, error)
	// HeldAccountIDs returns the accounts that a retention hold in effect preserves.
	HeldAccountIDs(ctx context.Context) ([]string, error)
	// ReportedGatheringsTx and HeldAccountIDsTx are the reads above inside tx.
	ReportedGatheringsTx(ctx context.Context, tx uow.Tx, gatheringIDs []string) (map[string]bool, error)
	HeldAccountIDsTx(ctx context.Context, tx uow.Tx) ([]string, error)
}

// RecordsRepository is the part of Repository that the purge of the records uses.
type RecordsRepository interface {
	// GatheringsToPurge returns at most limit events, in ID order after the event after ("" for
	// the first), that ended or were cancelled before endedBefore and still have answers or
	// invitations.
	GatheringsToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, after string, limit int) ([]string, error)
	// RecordAccounts returns the accounts that the answers, the approvals, and the invitations of
	// an event name, as the one who answered or was invited or as the one who acted.
	RecordAccounts(ctx context.Context, tx uow.Tx, gatheringID string) ([]string, error)
	// PurgeRecords deletes the answers with their approvals and attendee rows, and the
	// invitations with their revocations, of the events.
	PurgeRecords(ctx context.Context, tx uow.Tx, gatheringIDs []string) error
}

// kvPurge keys the cursor of JobPurgeRecords: the last event that a run looked at, so that the
// next run goes on after it however many events the earlier runs kept. It lasts purgeCursorLease;
// a lost cursor starts again from the first event.
const (
	kvPurge          = "gathering.purge"
	kvPurgeCursor    = "cursor"
	purgeCursorLease = 30 * 24 * time.Hour
)

// PurgeRecords erases the answers, the approvals, and the invitations of the events that ended or
// were cancelled longer than event_records_retention_days ago, and returns how many events it
// erased them of. It keeps those of an event that a report points at, until the moderation module
// erases the report after its retention, and those of an event whose records name an account that
// a retention hold preserves. Each batch is one short transaction that locks its events and reads
// the holds and the reports again before it deletes; the heads are derived again, so that an
// event whose answers are gone counts nobody going. One run looks at a bounded number of events in
// ID order from where the last run stopped (the cursor in Deps.Store) and wraps around to the
// first event at the end, so the events that it keeps never starve the others. It is safe to
// repeat. Without Deps.Retention it erases nothing.
func (s *Service) PurgeRecords(ctx context.Context) (int, error) {
	r := s.d.Retention
	if r == nil {
		return 0, nil
	}
	period, err := r.RecordsRetention(ctx)
	if err != nil {
		return 0, err
	}
	heldIDs, err := r.HeldAccountIDs(ctx)
	if err != nil {
		return 0, err
	}
	before := s.now().Add(-period)
	start, err := s.purgeCursor(ctx)
	if err != nil {
		return 0, err
	}
	after, wrapped, total := start, start == "", 0
	for range recordPurgeMaxBatches {
		ids, err := s.d.Repository.GatheringsToPurge(ctx, nil, before, after, RecordPurgeBatch)
		if err != nil {
			return total, err
		}
		if len(ids) > 0 {
			after = ids[len(ids)-1]
			n, err := s.purgeBatch(ctx, ids, heldIDs)
			total += n
			if err != nil {
				return total, err
			}
		}
		if len(ids) < RecordPurgeBatch {
			// The end of the events: the next batch, or the next run, starts from the first.
			after = ""
			if wrapped {
				break
			}
			wrapped = true
		}
	}
	return total, s.savePurgeCursor(ctx, after)
}

// purgeBatch erases the records of the events ids that no report points at and whose records
// name none of heldIDs, read before the transaction, and checks both again in it after locking
// the events, so that a hold or a report made meanwhile keeps its event.
func (s *Service) purgeBatch(ctx context.Context, ids, heldIDs []string) (int, error) {
	candidates, err := s.purgeable(ctx, nil, ids, heldIDs)
	if err != nil || len(candidates) == 0 {
		return 0, err
	}
	n := 0
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		n = 0
		for _, id := range candidates {
			if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
				return err
			}
		}
		held, err := s.d.Retention.HeldAccountIDsTx(ctx, tx)
		if err != nil {
			return err
		}
		purge, err := s.purgeable(ctx, tx, candidates, held)
		if err != nil || len(purge) == 0 {
			return err
		}
		if err := s.d.Repository.PurgeRecords(ctx, tx, purge); err != nil {
			return err
		}
		for _, id := range purge {
			if err := s.d.Repository.RefreshHead(ctx, tx, id); err != nil {
				return err
			}
		}
		n = len(purge)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("gathering: purge the records of the events: %w", err)
	}
	return n, nil
}

// purgeable returns the events of ids that no report points at and whose records name none of
// heldIDs, reading the records in tx (outside any transaction when tx is nil).
func (s *Service) purgeable(ctx context.Context, tx uow.Tx, ids, heldIDs []string) ([]string, error) {
	var reported map[string]bool
	var err error
	if tx == nil {
		reported, err = s.d.Retention.ReportedGatherings(ctx, ids)
	} else {
		reported, err = s.d.Retention.ReportedGatheringsTx(ctx, tx, ids)
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if reported[id] {
			continue
		}
		held, err := s.namesHeld(ctx, tx, id, heldIDs)
		if err != nil {
			return nil, err
		}
		if !held {
			out = append(out, id)
		}
	}
	return out, nil
}

// namesHeld reports whether the records of an event name one of heldIDs.
func (s *Service) namesHeld(ctx context.Context, tx uow.Tx, gatheringID string, heldIDs []string) (bool, error) {
	if len(heldIDs) == 0 {
		return false, nil
	}
	ids, err := s.d.Repository.RecordAccounts(ctx, tx, gatheringID)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(ids, func(id string) bool { return slices.Contains(heldIDs, id) }), nil
}

// purgeCursor returns the event after which JobPurgeRecords goes on, "" for the first.
func (s *Service) purgeCursor(ctx context.Context) (string, error) {
	if s.d.Store == nil {
		return "", nil
	}
	b, err := s.d.Store.Get(ctx, kvPurge, kvPurgeCursor)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return "", nil
	}
	return string(b), err
}

// savePurgeCursor keeps the event after which the next run of JobPurgeRecords goes on.
func (s *Service) savePurgeCursor(ctx context.Context, after string) error {
	if s.d.Store == nil {
		return nil
	}
	if after == "" {
		return s.d.Store.Delete(ctx, kvPurge, kvPurgeCursor)
	}
	return s.d.Store.Set(ctx, kvPurge, kvPurgeCursor, []byte(after), purgeCursorLease)
}

// registerPurgeJob registers the JobPurgeRecords handler and its hourly schedule.
func (s *Service) registerPurgeJob(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeRecords, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.PurgeRecords(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	if sched != nil {
		sched.Add(queue.Schedule{Name: JobPurgeRecords, Every: RecordPurgeInterval, Job: queue.NewJob{Type: JobPurgeRecords}})
	}
}
