// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// The supporter notifications (ADR-0072 §16) tell a subscriber of their supporter subscription:
// supporter_payment_failed when it enters past_due, supporter_subscription_ended when it loses
// the plan's role by entering lapsed, expired, or revoked, supporter_plan_retired when its plan
// is retired, and supporter_service_closure when the announcement of the service closure cancels
// it (ADR-0088 §2). Each also comes by email, which the program needs to be enabled: a job that
// the same transaction enqueues sends it once the notification is committed. Their text never
// leads to the subscription screen: the Web Push messages and the notifications open the
// notifications, and only the web client links them to /settings/supporter, which the Tauri app
// does not show (§14).

// SupporterMailboxes reads where the supporter notifications of a local account are sent by
// email. The composition root implements it on the identity and account modules.
type SupporterMailboxes interface {
	// SupporterMailbox returns the primary address and the locale of the user of a local
	// account; ok is false without a usable address.
	SupporterMailbox(ctx context.Context, accountID string) (m Mailbox, ok bool, err error)
}

// JobSupporterEmail is the job that sends a supporter notification by email.
const JobSupporterEmail = "notification.supporter_email"

// supporterEmail is the payload of JobSupporterEmail: what the email says, as of the transition.
type supporterEmail struct {
	NotificationID string            `json:"notification_id"`
	AccountID      string            `json:"account_id"`
	Type           notification.Type `json:"type"`
	PlanName       string            `json:"plan_name"`
	// GraceEndsAt is the end of the grace period of a supporter_payment_failed.
	GraceEndsAt time.Time `json:"grace_ends_at,omitzero"`
	// EndReason is why a supporter_subscription_ended ended (supporters.EndReason).
	EndReason string `json:"end_reason,omitempty"`
}

// SupporterDeps are the ports of the supporter notifications.
type SupporterDeps struct {
	Repository Repository
	IDs        kernel.IDGenerator
	// Outbox enqueues the emails in the transaction of the notifications; nil sends none.
	Outbox queue.Outbox
	// Push enqueues the Web Push messages of the new notifications; nil pushes none.
	Push *Pusher
}

// RegisterSupporterProjectors registers the synchronous projectors that create the supporter
// notifications in the transaction of the event:
//
//   - supporters.subscription_transitioned that enters past_due: supporter_payment_failed, once
//     per transition (notification.SupporterTransitionKey).
//   - supporters.subscription_transitioned that ends the subscription
//     (appsupporters.SubscriptionTransitioned.Ended): supporter_subscription_ended, once per
//     transition.
//   - supporters.plan_retired: supporter_plan_retired, to each subscriber who had paid, once per
//     subscription (notification.SupporterPlanRetiredKey).
//   - supporters.closure_notice_cancellations: supporter_service_closure, to each subscriber who
//     had paid and whose subscription the announcement of the service closure cancels, once per
//     subscription and announcement (notification.SupporterServiceClosureKey).
//
// They come from the instance and have no source account; a subscriber without a local account
// gets nothing. Each new notification enqueues its Web Push messages and, with d.Outbox,
// JobSupporterEmail.
func RegisterSupporterProjectors(r *uow.Projectors, d SupporterDeps) {
	p := &supporterProjector{d: d}
	r.Register(appsupporters.EventSubscriptionTransitioned, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appsupporters.SubscriptionTransitioned)
		mail := supporterEmail{AccountID: ev.AccountID, PlanName: ev.PlanName}
		switch {
		case ev.PaymentFailed():
			mail.Type, mail.GraceEndsAt = notification.TypeSupporterPaymentFailed, ev.GraceEndsAt()
		case ev.Ended():
			mail.Type, mail.EndReason = notification.TypeSupporterSubscriptionEnded, string(ev.EndReason())
		default:
			return nil
		}
		return p.notify(ctx, tx, notification.Notification{
			RecipientAccountID: ev.AccountID, Type: mail.Type, SupporterSubscriptionID: ev.SubscriptionID,
			DedupeKey: notification.SupporterTransitionKey(mail.Type, ev.TransitionID), CreatedAt: ev.At,
		}, mail)
	})
	r.Register(appsupporters.EventPlanRetired, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appsupporters.PlanRetired)
		for _, sub := range ev.Subscriptions {
			err := p.notify(ctx, tx, notification.Notification{
				RecipientAccountID: sub.AccountID, Type: notification.TypeSupporterPlanRetired, SupporterSubscriptionID: sub.SubscriptionID,
				DedupeKey: notification.SupporterPlanRetiredKey(ev.PlanID, sub.SubscriptionID), CreatedAt: ev.At,
			}, supporterEmail{AccountID: sub.AccountID, Type: notification.TypeSupporterPlanRetired, PlanName: ev.PlanName})
			if err != nil {
				return err
			}
		}
		return nil
	})
	r.Register(appsupporters.EventClosureNoticeCancellations, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appsupporters.ClosureNoticeCancellations)
		for _, sub := range ev.Subscriptions {
			err := p.notify(ctx, tx, notification.Notification{
				RecipientAccountID: sub.AccountID, Type: notification.TypeSupporterServiceClosure, SupporterSubscriptionID: sub.SubscriptionID,
				ServiceClosureEventID: ev.ClosureEventID,
				DedupeKey:             notification.SupporterServiceClosureKey(ev.ClosureEventID, sub.SubscriptionID), CreatedAt: ev.At,
			}, supporterEmail{AccountID: sub.AccountID, Type: notification.TypeSupporterServiceClosure, PlanName: sub.PlanName})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

type supporterProjector struct {
	d SupporterDeps
}

// notify inserts n and, when it is new, enqueues its Web Push messages and its email.
func (p *supporterProjector) notify(ctx context.Context, tx uow.Tx, n notification.Notification, mail supporterEmail) error {
	if n.RecipientAccountID == "" {
		return nil
	}
	n.ID = p.d.IDs.NewID()
	if err := n.Validate(); err != nil {
		return err
	}
	created, err := p.d.Repository.InsertNotification(ctx, tx, n)
	if err != nil || !created {
		return err
	}
	if err := p.d.Push.enqueue(ctx, tx, n); err != nil {
		return err
	}
	if p.d.Outbox == nil {
		return nil
	}
	mail.NotificationID = n.ID
	payload, err := json.Marshal(mail)
	if err != nil {
		return err
	}
	return p.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobSupporterEmail, Payload: payload, IdempotencyKey: JobSupporterEmail + ":" + n.ID})
}

// SupporterMailDeps are the ports of the job that sends the supporter notifications by email.
type SupporterMailDeps struct {
	Mailboxes SupporterMailboxes
	Mailer    kernel.Mailer
	// Rejections records the rejected recipients; nil records none.
	Rejections RecipientRejections
	// Messages words the emails; nil means the embedded catalogs (i18n.Load).
	Messages *i18n.Bundle
	// Host is the server's domain name, which the emails name the instance by when it has no
	// display name.
	Host string
	// InstanceName returns the instance's display name; nil or "" names it by Host.
	InstanceName func(ctx context.Context) string
	// WebOrigin is the origin of the web client that the emails link to.
	WebOrigin string
}

// RegisterSupporterJobs registers the handler of JobSupporterEmail on reg. It sends nothing to a
// user without a usable address; a permanent rejection of the email fails the job and any
// other failure is retried (queue.MailResult). The email of a failed payment links to the
// supporter page of the web client, where the payment method is fixed; the others link to the
// notifications.
func RegisterSupporterJobs(reg *queue.JobHandlerRegistry, d SupporterMailDeps) {
	reg.Register(JobSupporterEmail, func(ctx context.Context, j queue.Job) queue.Result {
		var job supporterEmail
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return queue.Fail(fmt.Errorf("notification: the payload of %s: %w", j.ID, err))
		}
		return queue.MailResult(sendSupporterEmail(ctx, d, j, job))
	})
}

// graceLayout is how an email writes the end of a grace period: in UTC, which the text says.
const graceLayout = "2006-01-02 15:04"

func sendSupporterEmail(ctx context.Context, d SupporterMailDeps, run queue.Job, job supporterEmail) error {
	box, ok, err := d.Mailboxes.SupporterMailbox(ctx, job.AccountID)
	if err != nil || !ok {
		return err
	}
	bundle := d.Messages
	if bundle == nil {
		if bundle, err = i18n.Load(); err != nil {
			return err
		}
	}
	users := &i18n.Resolver{Bundle: bundle, UserLocale: func(context.Context) string { return box.Locale }}
	l := users.Localizer(ctx, "")
	origin := strings.TrimSuffix(d.WebOrigin, "/")
	name := d.Host
	if d.InstanceName != nil {
		if n := d.InstanceName(ctx); n != "" {
			name = n
		}
	}
	m := kernel.MailMessage{To: box.Address}
	switch job.Type {
	case notification.TypeSupporterPaymentFailed:
		m.Subject = l.ServerMailSupporterPaymentFailedSubject(name)
		m.Text = l.ServerMailSupporterPaymentFailedBody(job.GraceEndsAt.UTC().Format(graceLayout), name, job.PlanName, origin+"/settings/supporter")
	case notification.TypeSupporterSubscriptionEnded:
		m.Subject = l.ServerMailSupporterSubscriptionEndedSubject(name)
		m.Text = l.ServerMailSupporterSubscriptionEndedBody(name, job.PlanName, job.EndReason, origin+"/notifications")
	case notification.TypeSupporterPlanRetired:
		m.Subject = l.ServerMailSupporterPlanRetiredSubject(name)
		m.Text = l.ServerMailSupporterPlanRetiredBody(name, job.PlanName, origin+"/notifications")
	case notification.TypeSupporterServiceClosure:
		m.Subject = l.ServerMailSupporterServiceClosureSubject(name)
		m.Text = l.ServerMailSupporterServiceClosureBody(name, job.PlanName, origin+"/notifications")
	default:
		return nil
	}
	return sendMail(ctx, d.Mailer, d.Rejections, run, box, m, fmt.Sprintf("%s email of %s", job.Type, job.NotificationID))
}
