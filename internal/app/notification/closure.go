// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The service closure notifications (ADR-0088 §1, §2): service_closure tells every local user of
// the announcement of the closure, of the change of its dates, and of its withdrawal; and once
// the closing date of the announcement in effect has passed, admin_service_closure_due asks the
// holders of manage_service_closure to close the server, which never closes by itself. Both come
// from jobs that the transaction of the event enqueues: the first goes to every local user, and
// the second waits for the closing date. The emails of the closure are not these notifications'.

// ClosureRecipients pages the local accounts of users, the recipients of service_closure. The
// composition root implements it on the account module.
type ClosureRecipients interface {
	// LocalUserAccountIDs returns a page of the local accounts of users in ID order, after the
	// cursor after ("" for the first page), reading up to limit accounts, and the cursor of the
	// next page, "" after the last one. The instance actor is left out.
	LocalUserAccountIDs(ctx context.Context, after string, limit int) (ids []string, next string, err error)
}

// ClosureAnnouncements reads the announcement of the service closure in effect:
// *instance.Service implements it.
type ClosureAnnouncements interface {
	// AnnouncementInEffect reports whether announcementID is the announcement in effect, the
	// server not closed yet, with its closing date.
	AnnouncementInEffect(ctx context.Context, announcementID string) (closesAt time.Time, ok bool, err error)
}

// Jobs of the service closure notifications.
const (
	// JobServiceClosureFanout creates the service_closure notification of every local user.
	JobServiceClosureFanout = "notification.service_closure_fanout"
	// JobServiceClosureDue creates admin_service_closure_due once the closing date of an
	// announcement has passed, while it is still in effect.
	JobServiceClosureDue = "notification.service_closure_due"
)

// closureFanoutBatch is how many recipients one transaction of JobServiceClosureFanout notifies.
const closureFanoutBatch = 100

// closureFanout is the payload of JobServiceClosureFanout.
type closureFanout struct {
	EventID string                     `json:"event_id"`
	Change  notification.ClosureChange `json:"change"`
	At      time.Time                  `json:"at"`
}

// closureDue is the payload of JobServiceClosureDue.
type closureDue struct {
	AnnouncementID string `json:"announcement_id"`
}

// ClosureDeps are the ports of the service closure notifications.
type ClosureDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Recipients ClosureRecipients
	Closures   ClosureAnnouncements
	IDs        kernel.IDGenerator
	Clock      kernel.Clock
	// Outbox enqueues the jobs in the transaction of the closure event.
	Outbox queue.Outbox
	// Push enqueues the Web Push messages of the new notifications; nil pushes none.
	Push *Pusher
	// Moderators creates admin_service_closure_due as the other moderator notifications, with
	// their emails.
	Moderators ModeratorDeps
}

// closureChange returns what a closure event tells the users of, and false for an event they
// are not told of by a service_closure (the closure itself, whose email tells them).
func closureChange(ev appinstance.ClosureRecorded) (notification.ClosureChange, bool) {
	switch {
	case ev.Starts():
		return notification.ClosureAnnounced, true
	case ev.Reschedules():
		return notification.ClosureRescheduled, true
	case ev.Withdraws():
		return notification.ClosureWithdrawn, true
	}
	return "", false
}

// RegisterClosureProjectors registers the projector that, in the transaction of a service
// closure event, enqueues JobServiceClosureFanout for an announcement, a change of its dates, or
// a withdrawal, and, for an announcement or a change of its dates, JobServiceClosureDue to run at
// its closing date.
func RegisterClosureProjectors(r *uow.Projectors, d ClosureDeps) {
	r.Register(appinstance.EventClosureRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appinstance.ClosureRecorded)
		change, ok := closureChange(ev)
		if !ok {
			return nil
		}
		fan, err := json.Marshal(closureFanout{EventID: ev.Event.ID, Change: change, At: ev.Event.OccurredAt})
		if err != nil {
			return err
		}
		jobs := []queue.NewJob{{Type: JobServiceClosureFanout, Payload: fan, IdempotencyKey: JobServiceClosureFanout + ":" + ev.Event.ID}}
		if change != notification.ClosureWithdrawn {
			due, err := json.Marshal(closureDue{AnnouncementID: ev.Event.ID})
			if err != nil {
				return err
			}
			jobs = append(jobs, queue.NewJob{
				Type: JobServiceClosureDue, Payload: due, IdempotencyKey: JobServiceClosureDue + ":" + ev.Event.ID, RunAt: ev.ClosesAt(),
			})
		}
		return d.Outbox.EnqueueTx(ctx, tx, jobs...)
	})
}

// RegisterClosureJobs registers the handlers of JobServiceClosureFanout and
// JobServiceClosureDue on reg.
func RegisterClosureJobs(reg *queue.JobHandlerRegistry, d ClosureDeps) {
	reg.Register(JobServiceClosureFanout, func(ctx context.Context, j queue.Job) queue.Result {
		var p closureFanout
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.EventID == "" || !p.Change.Valid() {
			return queue.Fail(fmt.Errorf("notification: the payload of %s: %w", j.ID, err))
		}
		if err := fanOutClosure(ctx, d, p); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register(JobServiceClosureDue, func(ctx context.Context, j queue.Job) queue.Result {
		var p closureDue
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.AnnouncementID == "" {
			return queue.Fail(fmt.Errorf("notification: the payload of %s: %w", j.ID, err))
		}
		return notifyClosureDue(ctx, d, p.AnnouncementID)
	})
}

// fanOutClosure creates the service_closure of every local user, page by page. The dedupe key
// names the event, so a job run again notifies nobody twice.
func fanOutClosure(ctx context.Context, d ClosureDeps, p closureFanout) error {
	after := ""
	for {
		ids, next, err := d.Recipients.LocalUserAccountIDs(ctx, after, closureFanoutBatch)
		if err != nil {
			return fmt.Errorf("notification: the recipients of the service closure: %w", err)
		}
		err = d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			for _, id := range ids {
				n := notification.Notification{
					ID: d.IDs.NewID(), RecipientAccountID: id, Type: notification.TypeServiceClosure,
					ServiceClosureEventID: p.EventID, ServiceClosureChange: p.Change,
					DedupeKey: notification.ServiceClosureKey(p.EventID), CreatedAt: p.At,
				}
				if err := n.Validate(); err != nil {
					return err
				}
				created, err := d.Repository.InsertNotification(ctx, tx, n)
				if err != nil {
					return err
				}
				if created {
					if err := d.Push.enqueue(ctx, tx, n); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("notification: notify the service closure: %w", err)
		}
		if next == "" {
			return nil
		}
		after = next
	}
}

// notifyClosureDue creates admin_service_closure_due when the announcement is still in effect
// and its closing date has passed; it waits for the date when the job runs early, and does
// nothing once the announcement was changed or withdrawn or the server closed.
func notifyClosureDue(ctx context.Context, d ClosureDeps, announcementID string) queue.Result {
	closesAt, ok, err := d.Closures.AnnouncementInEffect(ctx, announcementID)
	switch {
	case err != nil:
		return queue.Retry(0, err)
	case !ok:
		return queue.Done()
	}
	now := kernel.Normalize(d.Clock.Now())
	if now.Before(closesAt) {
		return queue.Retry(closesAt.Sub(now), nil)
	}
	p := &moderatorProjector{d: d.Moderators}
	err = d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminServiceClosureDue, ServiceClosureEventID: announcementID,
			DedupeKey: notification.AdminServiceClosureDueKey(announcementID), CreatedAt: now,
		})
	})
	if err != nil {
		return queue.Retry(0, fmt.Errorf("notification: notify that the closing date has passed: %w", err))
	}
	return queue.Done()
}
