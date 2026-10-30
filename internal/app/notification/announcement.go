// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The announcement notifications (ADR-0125 §2): once a server-wide announcement starts to show
// (instance.Announcement.ShowsFrom), every local user who is neither suspended nor deleted gets
// one announcement, unless they turned it off in the app. The transaction that publishes the
// announcement, or that moves the start of its window before it has shown, enqueues
// JobNotifyAnnouncement to run at ShowsFrom; the job notifies only while the announcement is
// published, within its window, and past ShowsFrom. A revision after it has shown notifies
// nobody again, and the reads hide the notifications of a withdrawn or ended announcement
// (Hidden.ShownAnnouncements).

// JobNotifyAnnouncement creates the announcement notifications of an announcement.
const JobNotifyAnnouncement = "instance.notify_announcement"

// announcementBatch is how many recipients one transaction of JobNotifyAnnouncement notifies.
const announcementBatch = 100

// AnnouncementRecipients pages the recipients of the announcement notifications. The
// composition root implements it on the account and moderation modules.
type AnnouncementRecipients interface {
	// LocalUserAccountIDs is ClosureRecipients.LocalUserAccountIDs.
	LocalUserAccountIDs(ctx context.Context, after string, limit int) (ids []string, next string, err error)
	// Notifiable reports, inside tx, whether a local account gets the announcements: it is
	// neither suspended nor deleted.
	Notifiable(ctx context.Context, tx uow.Tx, accountID string) (bool, error)
}

// Announcements reads an announcement: *appinstance.Service implements it.
type Announcements interface {
	// Announcement returns the announcement in any state; ErrNotFound (of the instance module)
	// for an unknown one.
	Announcement(ctx context.Context, id string) (instance.Announcement, error)
}

// AnnouncementDeps are the ports of the announcement notifications.
type AnnouncementDeps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    Repository
	Recipients    AnnouncementRecipients
	Announcements Announcements
	IDs           kernel.IDGenerator
	Clock         kernel.Clock
	// Outbox enqueues JobNotifyAnnouncement in the transaction of the announcement.
	Outbox queue.Outbox
	// Push enqueues the Web Push messages of the new notifications; nil pushes none.
	Push *Pusher
}

// announcementJob is the payload of JobNotifyAnnouncement.
type announcementJob struct {
	AnnouncementID string `json:"announcement_id"`
}

// RegisterAnnouncementProjectors registers the projector that, in the transaction that
// schedules an announcement (appinstance.AnnouncementScheduled), enqueues JobNotifyAnnouncement
// to run at its ShowsFrom. The idempotency key names the announcement and the time, so that a
// new start of the window enqueues a job of its own; the dedupe key of the notifications keeps
// one per announcement and recipient however many jobs run.
func RegisterAnnouncementProjectors(r *uow.Projectors, d AnnouncementDeps) {
	r.Register(appinstance.EventAnnouncementScheduled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appinstance.AnnouncementScheduled)
		payload, err := json.Marshal(announcementJob{AnnouncementID: ev.AnnouncementID})
		if err != nil {
			return err
		}
		at := ev.ShowsFrom.UTC()
		return d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
			Type: JobNotifyAnnouncement, Payload: payload, RunAt: at,
			IdempotencyKey: JobNotifyAnnouncement + ":" + ev.AnnouncementID + ":" + at.Format(time.RFC3339Nano),
		})
	})
}

// RegisterAnnouncementJobs registers the handler of JobNotifyAnnouncement on reg.
func RegisterAnnouncementJobs(reg *queue.JobHandlerRegistry, d AnnouncementDeps) {
	reg.Register(JobNotifyAnnouncement, func(ctx context.Context, j queue.Job) queue.Result {
		var p announcementJob
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.AnnouncementID == "" {
			return queue.Fail(fmt.Errorf("notification: the payload of %s: %w", j.ID, err))
		}
		return notifyAnnouncement(ctx, d, p.AnnouncementID)
	})
}

// notifyAnnouncement creates the announcement notifications of the announcement id when it is
// published, within its window, and past ShowsFrom; it waits for ShowsFrom when the job runs
// early, and does nothing for a draft, a withdrawn announcement, or an ended one.
func notifyAnnouncement(ctx context.Context, d AnnouncementDeps, id string) queue.Result {
	a, err := d.Announcements.Announcement(ctx, id)
	switch {
	case errors.Is(err, appinstance.ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case a.State != instance.AnnouncementPublished:
		return queue.Done()
	}
	now := kernel.Normalize(d.Clock.Now())
	if from := a.ShowsFrom(); now.Before(from) {
		return queue.Retry(from.Sub(now), nil)
	}
	if !a.ShowsAt(now) {
		return queue.Done() // ended before the job ran
	}
	if err := fanOutAnnouncement(ctx, d, a, now); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// fanOutAnnouncement notifies the local users of a, announcementBatch of them per transaction.
func fanOutAnnouncement(ctx context.Context, d AnnouncementDeps, a instance.Announcement, now time.Time) error {
	title := a.Revision.Title
	after := ""
	for {
		ids, next, err := d.Recipients.LocalUserAccountIDs(ctx, after, announcementBatch)
		if err != nil {
			return fmt.Errorf("notification: the recipients of announcement %s: %w", a.ID, err)
		}
		err = d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			for _, id := range ids {
				if err := notifyOfAnnouncement(ctx, d, tx, id, a.ID, title, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("notification: notify announcement %s: %w", a.ID, err)
		}
		if next == "" {
			return nil
		}
		after = next
	}
}

// notifyOfAnnouncement creates the announcement of recipientID, unless it is suspended or
// deleted or turned announcements off in the app, and enqueues its Web Push messages.
func notifyOfAnnouncement(ctx context.Context, d AnnouncementDeps, tx uow.Tx, recipientID, announcementID, title string, now time.Time) error {
	ok, err := d.Recipients.Notifiable(ctx, tx, recipientID)
	if err != nil || !ok {
		return err
	}
	choices, err := choicesTx(ctx, d.Repository, tx, recipientID)
	if err != nil {
		return err
	}
	if !choices.TypeEnabled(notification.TypeAnnouncement, notification.ChannelInApp) {
		return nil // turned off in the app: not created (ADR-0109 §3)
	}
	n := notification.Notification{
		ID: d.IDs.NewID(), RecipientAccountID: recipientID, Type: notification.TypeAnnouncement,
		AnnouncementID: announcementID, DedupeKey: notification.AnnouncementKey(announcementID), CreatedAt: now,
	}
	if err := n.Validate(); err != nil {
		return err
	}
	created, err := d.Repository.InsertNotification(ctx, tx, n)
	if err != nil || !created {
		return err
	}
	return d.Push.enqueueWith(ctx, tx, n, func(j *pushJob) { j.AnnouncementTitle = title })
}
