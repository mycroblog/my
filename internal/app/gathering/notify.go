// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The notifications of the events (ADR-0161 §11). A command, or gathering.tick, enqueues
// JobNotify through the outbox in its transaction; the job decides the recipients when it runs,
// up to RecipientsPerJob of them per job, and notifies in each transaction only those who see the
// event (gathering.CanSee), through the notification module, whose exclusions apply. A job of an
// old date, of an invitation revoked, or of an event cancelled meanwhile notifies nobody, and the
// dedupe keys of the notifications make every run after the first notify nobody twice.

// Notifier creates the notifications of the events inside the transactions of their jobs.
// *appnotification.GatheringNotifier implements it.
type Notifier interface {
	NotifyEventTx(ctx context.Context, tx uow.Tx, n appnotification.EventNotice, recipientIDs []string) error
}

// JobNotify notifies the recipients of one notification of an event. Its payload names the kind,
// the event, and what the notification is about; its idempotency key is that of the notification.
const JobNotify = "gathering.notify"

// RecipientsPerJob is how many recipients one job of JobNotify covers at most: an event with more
// splits them into jobs of this many (ADR-0161 §12).
const RecipientsPerJob = 500

// notifyPayload is the payload of JobNotify.
type notifyPayload struct {
	queue.FanOutStep
	Kind      appnotification.EventKind `json:"kind"`
	Gathering string                    `json:"gathering"`
	// Ref is what the notification is about: the invitation of an invited, the revision or the
	// confirmation of a changed, and the confirmation of a reminder and of a started.
	Ref string `json:"ref,omitempty"`
	// To is the invitee of an invited, and the organizer of a room_unavailable.
	To string `json:"to,omitempty"`
	// Source is the account the notification is from, and Actor the account that made the change,
	// which is not notified of it.
	Source string `json:"source"`
	Actor  string `json:"actor,omitempty"`
	// Minutes is how long before the start a reminder is.
	Minutes int `json:"minutes,omitempty"`
	// Reason and Checkpoint are why the room of a room_unavailable cannot open, or could not, and
	// when the server found it out.
	Reason     gathering.RoomReason     `json:"reason,omitempty"`
	Checkpoint gathering.RoomCheckpoint `json:"checkpoint,omitempty"`
	At         time.Time                `json:"at"`
}

// key returns the idempotency key of the first job of p.
func (p notifyPayload) key() string {
	switch p.Kind {
	case appnotification.EventChanged:
		// A revision of the places and a new date that one update makes are one change.
		return JobNotify + ":" + string(p.Kind) + ":" + p.Gathering + ":" + strconv.FormatInt(p.At.UnixMilli(), 10)
	case appnotification.EventReminder:
		return JobNotify + ":" + string(p.Kind) + ":" + p.Ref + ":" + strconv.Itoa(p.Minutes)
	case appnotification.EventCancelled:
		return JobNotify + ":" + string(p.Kind) + ":" + p.Gathering
	case appnotification.EventRoomUnavailable:
		return JobNotify + ":" + string(p.Kind) + ":" + p.Ref + ":" + string(p.Reason) + ":" + string(p.Checkpoint)
	}
	return JobNotify + ":" + string(p.Kind) + ":" + p.Ref
}

// invitees reports whether the notification of kind goes to the invitees too (ADR-0161 §11).
func invitees(kind appnotification.EventKind) bool {
	return kind == appnotification.EventChanged || kind == appnotification.EventCancelled
}

// job returns the job of p.
func (p notifyPayload) job() (queue.NewJob, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{Type: JobNotify, Payload: b, IdempotencyKey: queue.StepKey(p.key(), p.FanOutStep)}, nil
}

// enqueueNotify enqueues the first jobs of ps in tx.
func (s *Service) enqueueNotify(ctx context.Context, tx uow.Tx, ps ...notifyPayload) error {
	if s.d.Notifier == nil || s.d.Outbox == nil || len(ps) == 0 {
		return nil
	}
	jobs := make([]queue.NewJob, 0, len(ps))
	for _, p := range ps {
		j, err := p.job()
		if err != nil {
			return err
		}
		jobs = append(jobs, j)
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, jobs...)
}

// RegisterNotificationProjectors registers the projectors that enqueue the notifications of the
// commands of the events in their transactions: an invitation, the confirmation of a date poll, a
// new date, a revision that changes the places, and a cancellation. A revision of only the title, the description, or the other
// settings notifies nobody.
func (s *Service) RegisterNotificationProjectors(r *uow.Projectors) {
	r.Register(EventInvited, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		i := e.(Invited).Invitation
		return s.enqueueNotify(ctx, tx, notifyPayload{
			Kind: appnotification.EventInvited, Gathering: i.GatheringID, Ref: i.ID, To: i.InviteeAccountID,
			Source: i.InvitedByAccountID, At: i.InvitedAt,
		})
	})
	r.Register(EventRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(Revised)
		if slices.Equal(ev.Revision.Settings.Places, ev.Previous.Settings.Places) {
			return nil
		}
		return s.enqueueChanged(ctx, tx, ev.Revision.GatheringID, ev.Revision.ID, ev.Revision.RevisedByAccountID, ev.Revision.RevisedAt)
	})
	r.Register(EventRescheduled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(Rescheduled).Confirmation
		return s.enqueueChanged(ctx, tx, c.GatheringID, c.ID, c.ConfirmedByAccountID, c.ConfirmedAt)
	})
	r.Register(EventCancelled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(Cancelled).Cancellation
		rec, err := s.d.Repository.FindGathering(ctx, tx, c.GatheringID)
		if err != nil {
			return err
		}
		return s.enqueueNotify(ctx, tx, notifyPayload{
			Kind: appnotification.EventCancelled, Gathering: c.GatheringID, Ref: c.GatheringID,
			Source: rec.OrganizerAccountID, Actor: c.CancelledByAccountID, At: c.CancelledAt,
		})
	})
	s.registerPollNotificationProjectors(r)
}

// enqueueChanged enqueues the event_changed of the change ref of the event id by actor.
func (s *Service) enqueueChanged(ctx context.Context, tx uow.Tx, id, ref, actor string, at time.Time) error {
	rec, err := s.d.Repository.FindGathering(ctx, tx, id)
	if err != nil {
		return err
	}
	return s.enqueueNotify(ctx, tx, notifyPayload{
		Kind: appnotification.EventChanged, Gathering: id, Ref: ref, Source: rec.OrganizerAccountID, Actor: actor, At: at,
	})
}

// RegisterNotificationJobs registers the handlers of JobNotify and JobTick on reg, and the
// schedule of JobTick on sched unless it is nil.
func (s *Service) RegisterNotificationJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobNotify, s.Notify)
	reg.Register(JobTick, func(ctx context.Context, _ queue.Job) queue.Result {
		if err := s.Tick(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	if sched != nil {
		sched.Add(queue.Schedule{Name: JobTick, Every: TickInterval, Job: queue.NewJob{Type: JobTick}})
	}
}

// Notify runs JobNotify.
func (s *Service) Notify(ctx context.Context, j queue.Job) queue.Result {
	var p notifyPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("decode %s: %w", JobNotify, err))
	}
	if s.d.Notifier == nil {
		return queue.Done()
	}
	if err := s.notify(ctx, p); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

func (s *Service) notify(ctx context.Context, p notifyPayload) error {
	if p.Kind == appnotification.EventInvited || p.Kind == appnotification.EventRoomUnavailable {
		return s.notifyBatch(ctx, p, []string{p.To})
	}
	fan := queue.FanOut{
		Size: RecipientsPerJob,
		Page: queue.PageByLastKey(func(ctx context.Context, after string, limit int) ([]string, error) {
			return s.audience(ctx, p, after, limit)
		}),
		Handle: func(ctx context.Context, ids []string) error { return s.notifyBatch(ctx, p, ids) },
		Enqueue: func(ctx context.Context, steps []queue.FanOutStep) error {
			jobs := make([]queue.NewJob, 0, len(steps))
			for _, step := range steps {
				next := p
				next.FanOutStep = step
				j, err := next.job()
				if err != nil {
					return err
				}
				jobs = append(jobs, j)
			}
			return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, jobs...) })
		},
	}
	return fan.Run(ctx, p.FanOutStep)
}

// current reports whether the notification p is still to be sent for the event rec at now: the
// invitation stands, the date is the one it is about, and the event was not cancelled (or, for a
// cancellation, was).
func current(p notifyPayload, rec Record, inv *InvitationRecord, now time.Time) bool {
	cancelled := rec.Cancellation != nil
	switch p.Kind {
	case appnotification.EventCancelled:
		return cancelled
	case appnotification.EventInvited:
		return !cancelled && inv != nil && inv.ID == p.Ref && !inv.Revoked
	case appnotification.EventReminder:
		// A reminder that would arrive after the start is of no use.
		return !cancelled && rec.Confirmation.ID == p.Ref && now.Before(rec.Confirmation.Schedule.StartsAt)
	case appnotification.EventStarted:
		return !cancelled && rec.Confirmation.ID == p.Ref && now.Before(rec.Confirmation.Schedule.EndsAt)
	case appnotification.EventRoomUnavailable:
		// A room that the organizer removed, or of an old date, is of no concern.
		return !cancelled && rec.Confirmation.ID == p.Ref && rec.HasVoice()
	}
	return !cancelled
}

// notifyBatch notifies, in one transaction, those of ids who are to get the notification p and
// see the event.
func (s *Service) notifyBatch(ctx context.Context, p notifyPayload, ids []string) error {
	rec, err := s.d.Repository.FindGathering(ctx, nil, p.Gathering)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ids = slices.DeleteFunc(slices.Clone(ids), func(id string) bool {
		if p.Kind == appnotification.EventRoomUnavailable {
			// The one notification to the organizer, of its own event (ADR-0161 §11).
			return id != rec.OrganizerAccountID
		}
		// Nobody is notified of what they did, and the organizer of its own event.
		return id == "" || id == p.Source || id == p.Actor || id == rec.OrganizerAccountID
	})
	if p.Kind == appnotification.EventReminder && len(ids) > 0 {
		chosen, err := s.d.Repository.Reminders(ctx, nil, ids)
		if err != nil {
			return err
		}
		ids = slices.DeleteFunc(ids, func(id string) bool { return !slices.Contains(remindersOf(chosen, id), p.Minutes) })
	}
	if len(ids) == 0 {
		return nil
	}
	sees, err := s.seesAmong(ctx, rec, ids)
	if err != nil {
		return err
	}
	ids = slices.DeleteFunc(ids, func(id string) bool { return !sees[id] })
	if len(ids) == 0 {
		return nil
	}
	now := s.now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.d.Repository.FindGathering(ctx, tx, p.Gathering)
		if err != nil {
			return err
		}
		var inv *InvitationRecord
		if p.Kind == appnotification.EventInvited {
			found, err := s.d.Repository.FindInvitation(ctx, tx, p.Gathering, p.To)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			if err == nil {
				inv = &found
			}
		}
		if !current(p, rec, inv, now) {
			return nil
		}
		return s.d.Notifier.NotifyEventTx(ctx, tx, noticeOf(p, rec), ids)
	})
}

// noticeOf returns the notice of p about the event rec, worded as of now.
func noticeOf(p notifyPayload, rec Record) appnotification.EventNotice {
	co := make([]string, 0, len(rec.Coorganizers))
	for _, c := range rec.Coorganizers {
		co = append(co, c.AccountID)
	}
	return appnotification.EventNotice{
		Kind: p.Kind, GatheringID: rec.ID, SourceAccountID: p.Source, Ref: p.Ref, At: p.At,
		Reason: string(p.Reason), Checkpoint: string(p.Checkpoint),
		Mail: appnotification.EventMail{
			Title: rec.Revision.Settings.Title, StartsAt: rec.Confirmation.Schedule.StartsAt, TimeZone: rec.Revision.Settings.TimeZone,
			OrganizerID: rec.OrganizerAccountID, CoorganizerIDs: co, MinutesBefore: p.Minutes,
		},
	}
}

// seesAmong reports, by account, whether each of ids sees the event rec now (gathering.CanSee),
// reading the facts of all of them together.
func (s *Service) seesAmong(ctx context.Context, rec Record, ids []string) (map[string]bool, error) {
	own, err := s.d.Repository.AudienceFacts(ctx, nil, rec.ID, ids)
	if err != nil {
		return nil, err
	}
	blocked := map[string]bool{}
	for _, org := range rec.organizerIDs() {
		among, err := s.d.Relations.BlockedAmong(ctx, org, ids)
		if err != nil {
			return nil, err
		}
		for id, b := range among {
			// The organizers do not block themselves.
			if b && id != org {
				blocked[id] = true
			}
		}
	}
	follows, err := s.d.Relations.FollowersAmong(ctx, rec.OrganizerAccountID, ids)
	if err != nil {
		return nil, err
	}
	followed, err := s.d.Relations.Following(ctx, rec.OrganizerAccountID, ids)
	if err != nil {
		return nil, err
	}
	e := rec.Event()
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		viewer, err := s.viewer(ctx, id)
		if err != nil {
			return nil, err
		}
		f := gathering.Facts{
			Blocked: blocked[id], Coorganizer: rec.Coorganizes(id), Invited: own[id].Invited, Answered: own[id].Answer != "",
			Approved: own[id].Approved, Voted: own[id].Voted, FollowsOrganizer: follows[id], FollowedByOrganizer: followed[id],
		}
		out[id] = gathering.CanSee(e, viewer, f)
	}
	return out, nil
}
