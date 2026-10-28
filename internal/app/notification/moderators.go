// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// The moderator notifications (FR-712, data-model §4.7): admin_report of the first open report
// of an account, admin_sign_up of a registration that awaits approval, admin_appeal of an
// appeal of a moderation action, admin_webhook_disabled of a webhook endpoint that the server
// disabled, admin_custom_emoji_request of a custom emoji request, and admin_error_group of a
// new or regressed error group (errors.go), to the local accounts
// whose users' roles grant the permission the type needs (notification.ModeratorPermission).
// The recipients are those who hold it in the transaction that creates the notifications; the
// list leaves out the notifications of a recipient who no longer holds it (Service.List). A
// recipient who wants the notifications by email gets an email from a job that the same
// transaction enqueues, so it is sent only once the notification is committed.

// Staff is what the moderator notifications read of the roles and preferences of the users of
// local accounts (the identity module). The composition root implements it on the identity and
// account modules.
type Staff interface {
	// PermissionHolders returns inside tx the local accounts whose users' roles grant p.
	PermissionHolders(ctx context.Context, tx uow.Tx, p notification.Permission) ([]StaffMember, error)
	// GrantedPermissions returns the permissions that the roles of the user of a local account
	// grant now; none for any other account.
	GrantedPermissions(ctx context.Context, accountID string) ([]notification.Permission, error)
	// Mailbox returns where the moderator notifications of a local account are sent by email;
	// ok is false when its user does not want them by email or has no usable address.
	Mailbox(ctx context.Context, accountID string) (m Mailbox, ok bool, err error)
	// UnsubscribeLink issues a new one-click unsubscribe link (ADR-0086 §4) that turns off the
	// moderator notification emails of the user of a local account, for one email.
	UnsubscribeLink(ctx context.Context, accountID string) (string, error)
}

// StaffMember is a local account whose user holds a permission.
type StaffMember struct {
	AccountID string
	// Email reports that the user wants the moderator notifications also by email.
	Email bool
}

// Mailbox is the address and the locale of the moderator notification emails to a user.
type Mailbox struct {
	// EmailAddressID identifies Address, for the record of a rejected recipient.
	EmailAddressID string
	Address        string
	Locale         string
}

// RecipientRejections records the recipients that the mail server rejects as hard bounces,
// which stop the later emails to them (ADR-0086 §5). *identity.Service implements it.
type RecipientRejections interface {
	// RecordRecipientRejection records rejection, the error of the email that the run of a
	// job sent to an address, when it is kernel.ErrRecipientRejected.
	RecordRecipientRejection(ctx context.Context, run queue.Job, emailAddressID string, rejection error) error
}

// sendMail sends m, the email of the run of a job to box, and records a rejected recipient
// through rejections (nil records nothing). what names the email in the errors; when the
// rejection cannot be recorded, the error is that one, which retries the job.
func sendMail(ctx context.Context, mailer kernel.Mailer, rejections RecipientRejections, run queue.Job, box Mailbox, m kernel.MailMessage, what string) error {
	err := mailer.Send(ctx, m)
	if err == nil {
		return nil
	}
	if rejections != nil {
		if rerr := rejections.RecordRecipientRejection(ctx, run, box.EmailAddressID, err); rerr != nil {
			return fmt.Errorf("notification: record the rejected %s: %w", what, rerr)
		}
	}
	return fmt.Errorf("notification: send the %s: %w", what, err)
}

// JobModeratorEmail is the job that sends a moderator notification by email.
const JobModeratorEmail = "notification.moderator_email"

// moderatorEmail is the payload of JobModeratorEmail.
type moderatorEmail struct {
	NotificationID string            `json:"notification_id"`
	AccountID      string            `json:"account_id"`
	Type           notification.Type `json:"type"`
}

// ModeratorDeps are the ports of the moderator notifications.
type ModeratorDeps struct {
	Repository Repository
	Staff      Staff
	IDs        kernel.IDGenerator
	// Outbox enqueues the emails in the transaction of the notifications; nil sends none.
	Outbox queue.Outbox
	// Push enqueues the Web Push messages of the new notifications; nil pushes none.
	Push *Pusher
}

// RegisterModeratorProjectors registers the synchronous projectors that create the moderator
// notifications in the transaction of the event (data-model §4.7):
//
//   - moderation.report_filed with FirstOpen: admin_report, to the holders of manage_reports
//     other than the reporter and the reported account.
//   - identity.registration_pending: admin_sign_up, to the holders of manage_accounts.
//   - moderation.appeal_filed: admin_appeal, to the holders of manage_accounts other than the
//     account that appealed.
//   - webhook.endpoint_disabled: admin_webhook_disabled, to the holders of manage_webhooks
//     (ADR-0060).
//   - content.custom_emoji_request_recorded of a submission: admin_custom_emoji_request, to the
//     holders of manage_custom_emojis other than the requester (ADR-0070 §9).
//   - account.moved of an account that moderation limits: admin_account_moved, to the holders
//     of manage_accounts, since the target does not inherit the limit (ADR-0089).
//   - diagnostics.error_group_created, _confirmed, and _regressed: admin_error_group, to the
//     holders of manage_errors, with the hourly cap of errors.go (ADR-0103 §8).
//
// The dedupe keys (notification.AdminReportKey, AdminSignUpKey, AdminAppealKey,
// AdminWebhookDisabledKey, AdminCustomEmojiRequestKey, AdminAccountMovedKey, AdminErrorGroupKey,
// AdminErrorGroupOthersKey) make projecting an event again
// create nothing more. For each recipient who wants them by email and whose notification is
// new, the projector enqueues JobModeratorEmail through d.Outbox.
func RegisterModeratorProjectors(r *uow.Projectors, d ModeratorDeps) {
	p := &moderatorProjector{d: d}
	r.Register(appmoderation.EventReportFiled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appmoderation.ReportFiled)
		if !ev.FirstOpen {
			return nil
		}
		rep := ev.Report
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminReport, ReportID: rep.ID, DedupeKey: notification.AdminReportKey(rep.ID), CreatedAt: rep.FiledAt,
		}, rep.ReporterAccountID, rep.TargetAccountID)
	})
	r.Register(appidentity.EventRegistrationPending, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appidentity.RegistrationPending)
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminSignUp, SubjectUserID: ev.UserID, DedupeKey: notification.AdminSignUpKey(ev.UserID), CreatedAt: ev.SubmittedAt,
		}, ev.AccountID)
	})
	r.Register(appmoderation.EventAppealFiled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		a := e.(appmoderation.AppealFiled).Appeal
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminAppeal, AppealID: a.ID, DedupeKey: notification.AdminAppealKey(a.ID), CreatedAt: a.SubmittedAt,
		}, a.AccountID)
	})
	r.Register(appwebhook.EventEndpointDisabled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appwebhook.EndpointDisabled)
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminWebhookDisabled, DedupeKey: notification.AdminWebhookDisabledKey(ev.StateChangeID), CreatedAt: ev.At,
		})
	})
	r.Register(appaccount.EventAccountMoved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appaccount.Moved)
		if !ev.SourceLimited {
			return nil
		}
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminAccountMoved, AccountMoveID: ev.Move.ID,
			DedupeKey: notification.AdminAccountMovedKey(ev.Move.ID), CreatedAt: ev.Move.OccurredAt,
		}, ev.Move.AccountID, ev.Move.TargetAccountID)
	})
	r.Register(appcontent.EventCustomEmojiRequestRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.CustomEmojiRequestRecorded)
		if ev.Action != appcontent.CustomEmojiRequestSubmitted {
			return nil
		}
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminCustomEmojiRequest, CustomEmojiRequestID: ev.Request.ID,
			DedupeKey: notification.AdminCustomEmojiRequestKey(ev.Request.ID), CreatedAt: ev.At,
		}, ev.RequesterAccountID)
	})
	p.registerErrorGroupProjectors(r)
}

type moderatorProjector struct {
	d ModeratorDeps
}

// notify inserts n for every holder of the permission of n.Type except the accounts in except,
// and enqueues the emails of the new ones.
func (p *moderatorProjector) notify(ctx context.Context, tx uow.Tx, n notification.Notification, except ...string) error {
	perm, ok := notification.ModeratorPermission(n.Type)
	if !ok || p.d.Staff == nil {
		return nil
	}
	members, err := p.d.Staff.PermissionHolders(ctx, tx, perm)
	if err != nil {
		return fmt.Errorf("notification: the holders of %s: %w", perm, err)
	}
	var jobs []queue.NewJob
	for _, m := range members {
		if slices.Contains(except, m.AccountID) {
			continue
		}
		n.ID, n.RecipientAccountID = p.d.IDs.NewID(), m.AccountID
		if err := n.Validate(); err != nil {
			return err
		}
		created, err := p.d.Repository.InsertNotification(ctx, tx, n)
		if err != nil {
			return err
		}
		if created {
			if err := p.d.Push.enqueue(ctx, tx, n); err != nil {
				return err
			}
		}
		if !created || !m.Email || p.d.Outbox == nil {
			continue
		}
		payload, err := json.Marshal(moderatorEmail{NotificationID: n.ID, AccountID: m.AccountID, Type: n.Type})
		if err != nil {
			return err
		}
		jobs = append(jobs, queue.NewJob{Type: JobModeratorEmail, Payload: payload, IdempotencyKey: JobModeratorEmail + ":" + n.ID})
	}
	if len(jobs) == 0 {
		return nil
	}
	return p.d.Outbox.EnqueueTx(ctx, tx, jobs...)
}

// MailDeps are the ports of the job that sends the moderator notifications by email.
type MailDeps struct {
	Staff  Staff
	Mailer kernel.Mailer
	// Rejections records the rejected recipients; nil records none.
	Rejections RecipientRejections
	// Messages words the emails; nil means the embedded catalogs (i18n.Load).
	Messages *i18n.Bundle
	// Host is the server's domain name, which the emails name the instance by when MailSite
	// gives no name.
	Host string
	// MailSite returns the instance as its emails name it (ADR-0086 §2, §3); nil names it by
	// Host.
	MailSite func(ctx context.Context) kernel.MailSite
	// WebOrigin is the origin of the web client, whose notifications page the emails link to.
	WebOrigin string
}

// mailSite returns the instance as an email names it: site's answer, with Host for the name
// and https://Host for the URL when it gives none.
func mailSite(ctx context.Context, host string, site func(context.Context) kernel.MailSite) kernel.MailSite {
	var s kernel.MailSite
	if site != nil {
		s = site(ctx)
	}
	if s.Name == "" {
		s.Name = host
	}
	if s.URL == "" {
		s.URL = "https://" + host
	}
	return s
}

// RegisterModeratorJobs registers the handler of JobModeratorEmail on reg. The handler sends
// nothing when the recipient no longer holds the notification's permission or no longer wants
// it by email; a permanent rejection of the email fails the job and any other failure is
// retried (queue.MailResult). An email names only the kind of notification and links to the
// notifications page, never the report or the user it is about.
func RegisterModeratorJobs(reg *queue.JobHandlerRegistry, d MailDeps) {
	reg.Register(JobModeratorEmail, func(ctx context.Context, j queue.Job) queue.Result {
		var job moderatorEmail
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return queue.Fail(fmt.Errorf("notification: the payload of %s: %w", j.ID, err))
		}
		return queue.MailResult(sendModeratorEmail(ctx, d, j, job))
	})
}

func sendModeratorEmail(ctx context.Context, d MailDeps, run queue.Job, job moderatorEmail) error {
	perm, ok := notification.ModeratorPermission(job.Type)
	if !ok {
		return nil
	}
	granted, err := d.Staff.GrantedPermissions(ctx, job.AccountID)
	if err != nil || !slices.Contains(granted, perm) {
		return err
	}
	box, ok, err := d.Staff.Mailbox(ctx, job.AccountID)
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
	site := mailSite(ctx, d.Host, d.MailSite)
	draft, ok := ModeratorEmailDraft(l, site.Name, box.Address, job.Type, d.WebOrigin, "")
	if !ok {
		return nil
	}
	// Each email carries its own unsubscribe link, issued only now that it is sent, so that the
	// secret is in the email and nowhere else (ADR-0086 §4).
	if draft.UnsubscribeURL, err = d.Staff.UnsubscribeLink(ctx, job.AccountID); err != nil {
		return fmt.Errorf("notification: issue the unsubscribe link of %s: %w", job.NotificationID, err)
	}
	m, err := kernel.MailComposer{Site: site, Words: l}.Compose(draft)
	if err != nil {
		return fmt.Errorf("notification: compose the %s email of %s: %w", job.Type, job.NotificationID, err)
	}
	return sendMail(ctx, d.Mailer, d.Rejections, run, box, m, fmt.Sprintf("%s email of %s", job.Type, job.NotificationID))
}
