// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// The emails of the ordinary notifications and of the moderation notices (ADR-0109 §4): when a
// projector creates a notification whose group the recipient gets by email, the same
// transaction enqueues JobNotificationEmail to run notification.EmailDelay later. The job sends
// one email of the notification only when, by then, the notification is still unread (newer
// than the read marker and not dismissed), still shown (as a Web Push message checks it), and
// the recipient still gets its group by email; it goes to the verified primary address unless
// that address is suppressed (ADR-0086 §5). The moderator notifications are emailed at once
// instead (moderators.go).

// JobNotificationEmail is the job that sends the email of one notification once it stayed
// unread for notification.EmailDelay.
const JobNotificationEmail = "notification.email"

// Mailboxes is where the notification emails of local accounts go. The composition root
// implements it on the identity and account modules.
type Mailboxes interface {
	// Mailbox returns where the notification emails of a local account are sent: its user's
	// primary address once verified and not suppressed, with the user's locale. ok is false when
	// there is no such address.
	Mailbox(ctx context.Context, accountID string) (m Mailbox, ok bool, err error)
	// UnsubscribeLink issues a new one-click unsubscribe link (ADR-0086 §4) that turns off the
	// email of group g for the user of a local account, for one email.
	UnsubscribeLink(ctx context.Context, accountID string, g notification.Group) (string, error)
}

// EmailScheduler enqueues the emails of new notifications inside the transaction that creates
// them. The projectors call it through ProjectorDeps.Email; a nil EmailScheduler emails nothing.
type EmailScheduler struct {
	Outbox queue.Outbox
	Clock  kernel.Clock
}

// enqueue enqueues JobNotificationEmail of n, to run notification.EmailDelay from now, when n's
// group is emailed after the delay and c turns its email on. The idempotency key is the
// notification's, so that a retried transaction enqueues the job once.
func (m *EmailScheduler) enqueue(ctx context.Context, tx uow.Tx, n notification.Notification, c notification.Choices) error {
	if m == nil || m.Outbox == nil {
		return nil
	}
	g, ok := notification.GroupOf(n.Type)
	if !ok || !g.DelayedEmail() || !c.Enabled(g.Group, notification.ChannelEmail) {
		return nil
	}
	payload, err := json.Marshal(jobOf(n, ""))
	if err != nil {
		return err
	}
	return m.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobNotificationEmail, Payload: payload, IdempotencyKey: JobNotificationEmail + ":" + n.ID,
		RunAt: m.Clock.Now().Add(notification.EmailDelay),
	})
}

// EmailNotifications is what the email job reads of the recipient's notifications and settings.
// *Service implements it.
type EmailNotifications interface {
	PushNotifications
	ReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error)
	EmailEnabled(ctx context.Context, accountID string, t notification.Type) (bool, error)
}

// EmailDeps are the ports of the job that sends the notification emails.
type EmailDeps struct {
	Notifications EmailNotifications
	// Audience names the accounts of the emails and checks that the recipient still sees the
	// status of the notification, as for Web Push.
	Audience  PushAudience
	Mailboxes Mailboxes
	Mailer    kernel.Mailer
	// Rejections records the rejected recipients; nil records none.
	Rejections RecipientRejections
	// Messages words the emails; nil means the embedded catalogs (i18n.Load).
	Messages *i18n.Bundle
	// Host is the server's domain name, which the emails name the instance by when MailSite
	// gives no name.
	Host string
	// InstanceName returns the instance's display name, which the subjects of the instance's
	// notices name; nil or "" names it by Host.
	InstanceName func(ctx context.Context) string
	// MailSite returns the instance as its emails name it (ADR-0086 §2, §3); nil names it by
	// Host.
	MailSite func(ctx context.Context) kernel.MailSite
	// WebOrigin is the origin of the web client, whose page of the notification the emails link
	// to.
	WebOrigin string
}

// RegisterEmailJobs registers the handler of JobNotificationEmail on reg. The handler sends
// nothing when the notification was read or dismissed, is no longer shown, its group's email
// was turned off, or the recipient has no usable address; a permanent rejection of the email
// fails the job and any other failure is retried (queue.MailResult). The email's subject is
// what the notification's Web Push message says, and it links to the same page.
func RegisterEmailJobs(reg *queue.JobHandlerRegistry, d EmailDeps) {
	reg.Register(JobNotificationEmail, func(ctx context.Context, j queue.Job) queue.Result {
		var job pushJob
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return queue.Fail(fmt.Errorf("notification: the payload of %s: %w", j.ID, err))
		}
		return queue.MailResult(sendNotificationEmail(ctx, d, j, job))
	})
}

func sendNotificationEmail(ctx context.Context, d EmailDeps, run queue.Job, job pushJob) error {
	g, ok := notification.GroupOf(job.Type)
	if !ok || !g.DelayedEmail() {
		return nil
	}
	if on, err := d.Notifications.EmailEnabled(ctx, job.AccountID, job.Type); err != nil || !on {
		return err
	}
	marker, err := d.Notifications.ReadMarker(ctx, job.AccountID)
	if err != nil || marker.LastReadID >= job.NotificationID {
		return err // read since
	}
	shown, err := stillShown(ctx, d.Notifications, d.Audience, job)
	if err != nil || !shown {
		return err
	}
	box, ok, err := d.Mailboxes.Mailbox(ctx, job.AccountID)
	if err != nil || !ok {
		return err
	}
	p, l, err := describer{audience: d.Audience, messages: d.Messages, host: d.Host, instanceName: d.InstanceName}.describe(ctx, job)
	if errors.Is(err, ErrNotFound) {
		return nil // the account the email names is gone
	}
	if err != nil {
		return err
	}
	site := mailSite(ctx, d.Host, d.MailSite)
	draft, ok := NotificationEmailDraft(l, site.Name, box.Address, p, d.WebOrigin, "")
	if !ok {
		return nil
	}
	// Each email carries its own unsubscribe link, issued only now that it is sent, so that the
	// secret is in the email and nowhere else (ADR-0086 §4).
	if draft.UnsubscribeURL, err = d.Mailboxes.UnsubscribeLink(ctx, job.AccountID, g.Group); err != nil {
		return fmt.Errorf("notification: issue the unsubscribe link of %s: %w", job.NotificationID, err)
	}
	m, err := kernel.MailComposer{Site: site, Words: l}.Compose(draft)
	if err != nil {
		return fmt.Errorf("notification: compose the email of %s: %w", job.NotificationID, err)
	}
	return sendMail(ctx, d.Mailer, d.Rejections, run, box, m, fmt.Sprintf("email of %s", job.NotificationID))
}
