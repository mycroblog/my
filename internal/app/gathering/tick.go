// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// JobTick is the periodic job of the events (ADR-0161 §12): it reads only the events whose next
// due time came (gathering_dues), enqueues through the outbox the jobs of the reminders and of the
// start that came since, and writes the next due time. No payload.
const JobTick = "gathering.tick"

// TickInterval is the period of the JobTick schedule. A reminder or a start is late by up to the
// period of the platform's tick, as docs/operations/running.md says per platform.
const TickInterval = time.Minute

// Bounds of one run of JobTick: it handles tickBatch events at a time, and at most tickBatches
// batches, so that a run stays short; the events left wait for the next run.
const (
	tickBatch   = 100
	tickBatches = 10
)

// Tick runs JobTick: it handles every event whose next due time came, a batch at a time. Running
// it again enqueues nothing new: an event handled has its next due time ahead, and the jobs have
// the idempotency keys of their notifications.
func (s *Service) Tick(ctx context.Context) error {
	now := s.now()
	for range tickBatches {
		dues, err := s.d.Repository.DueGatherings(ctx, nil, now, tickBatch)
		if err != nil {
			return err
		}
		for _, d := range dues {
			if err := s.tickOne(ctx, d.GatheringID, now); err != nil {
				return err
			}
		}
		if len(dues) < tickBatch {
			return nil
		}
	}
	return nil
}

// tickOne handles the event id at now in one transaction: the jobs of what came between its due
// time and now, and its next due time.
func (s *Service) tickOne(ctx context.Context, id string, now time.Time) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
			return err
		}
		since, ok, err := s.d.Repository.FindDue(ctx, tx, id)
		if err != nil || !ok || since.After(now) {
			return err // handled meanwhile
		}
		rec, err := s.d.Repository.FindGathering(ctx, tx, id)
		if err != nil {
			return err
		}
		if rec.Cancellation != nil || !rec.Scheduled() {
			return s.d.Repository.DeleteDue(ctx, tx, id)
		}
		dues, err := duesOf(ctx, tx, s.d.Repository, rec)
		if err != nil {
			return err
		}
		var ps []notifyPayload
		for _, d := range gathering.DuesBetween(dues, since, now) {
			p := notifyPayload{Gathering: id, Ref: rec.Confirmation.ID, Source: rec.OrganizerAccountID, At: d.At}
			switch d.Kind {
			case gathering.DueReminder:
				p.Kind, p.Minutes = appnotification.EventReminder, d.MinutesBefore
			case gathering.DueStart:
				p.Kind = appnotification.EventStarted
			}
			ps = append(ps, p)
		}
		if err := s.enqueueNotify(ctx, tx, ps...); err != nil {
			return err
		}
		next, ok := gathering.NextDue(dues, now)
		if !ok {
			return s.d.Repository.DeleteDue(ctx, tx, id)
		}
		return s.d.Repository.SetDue(ctx, tx, id, next)
	})
}
