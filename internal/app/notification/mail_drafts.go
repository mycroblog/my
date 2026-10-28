// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// The drafts of the notification module's emails. Each is a pure function of what the email
// says: the job handlers of moderators.go and supporters.go call them with what they read, and
// the email preview (internal/platform/mailpreview) calls them with sample values, so that what
// a developer previews is what a user gets.

// ModeratorEmailDraft is the email to to of a moderator notification of type t from the instance
// named name, worded by l. It names only the kind of notification and links to the
// notifications page on webOrigin, never the report or the user it is about. unsubscribeURL is
// the email's one-click unsubscribe link, which the handler issues once it knows the email is
// sent. ok is false for a type without an email.
func ModeratorEmailDraft(l *i18n.Localizer, name, to string, t notification.Type, webOrigin, unsubscribeURL string) (d kernel.MailDraft, ok bool) {
	g, _ := notification.GroupOf(t)
	d = kernel.MailDraft{
		Category: kernel.MailNotification, To: to,
		Action:         &kernel.MailAction{Label: l.ServerMailActionOpenNotifications(), URL: strings.TrimSuffix(webOrigin, "/") + "/notifications"},
		Reason:         l.ServerMailReasonNotification(GroupLabel(l, g.Group)),
		UnsubscribeURL: unsubscribeURL,
	}
	switch t {
	case notification.TypeAdminReport:
		d.Subject, d.Paragraphs = l.ServerMailAdminReportSubject(name), []string{l.ServerMailAdminReportBody(name)}
	case notification.TypeAdminSignUp:
		d.Subject, d.Paragraphs = l.ServerMailAdminSignUpSubject(name), []string{l.ServerMailAdminSignUpBody(name)}
	case notification.TypeAdminAppeal:
		d.Subject, d.Paragraphs = l.ServerMailAdminAppealSubject(name), []string{l.ServerMailAdminAppealBody(name)}
	case notification.TypeAdminWebhookDisabled:
		d.Subject = l.ServerMailAdminWebhookDisabledSubject(name)
		d.Paragraphs = []string{l.ServerMailAdminWebhookDisabledBody(name), l.ServerMailAdminWebhookDisabledDetail()}
	case notification.TypeAdminCustomEmojiRequest:
		d.Subject, d.Paragraphs = l.ServerMailAdminCustomEmojiRequestSubject(name), []string{l.ServerMailAdminCustomEmojiRequestBody(name)}
	case notification.TypeAdminAccountMoved:
		d.Subject, d.Paragraphs = l.ServerMailAdminAccountMovedSubject(name), []string{l.ServerMailAdminAccountMovedBody(name)}
	case notification.TypeAdminServiceClosureDue:
		d.Subject, d.Paragraphs = l.ServerMailAdminServiceClosureDueSubject(name), []string{l.ServerMailAdminServiceClosureDueBody(name)}
	case notification.TypeAdminErrorGroup:
		d.Subject, d.Paragraphs = l.ServerMailAdminErrorGroupSubject(name), []string{l.ServerMailAdminErrorGroupBody(name)}
	default:
		return kernel.MailDraft{}, false
	}
	return d, true
}

// UnreadEmailDraft is the email to to of the notification p, worded by
// DescribeNotification in the locale of l, from the instance named name: its subject is what
// the notification's Web Push message says, and it links to the same page on webOrigin.
// unsubscribeURL is the email's one-click unsubscribe link, which the handler issues once it
// knows the email is sent. ok is false for a notification whose group is not emailed after
// notification.EmailDelay (notification.GroupSpec.DelayedEmail).
func UnreadEmailDraft(l *i18n.Localizer, name, to string, p PushPayload, webOrigin, unsubscribeURL string) (d kernel.MailDraft, ok bool) {
	g, ok := notification.GroupOf(p.Type)
	if !ok || !g.DelayedEmail() {
		return kernel.MailDraft{}, false
	}
	return kernel.MailDraft{
		Category: kernel.MailNotification, To: to, Subject: p.Title,
		Paragraphs:     []string{l.ServerMailNotificationBody(name)},
		Action:         &kernel.MailAction{Label: l.ServerMailActionOpenNotification(), URL: strings.TrimSuffix(webOrigin, "/") + p.Path},
		Reason:         l.ServerMailReasonNotification(GroupLabel(l, g.Group)),
		UnsubscribeURL: unsubscribeURL,
	}, true
}

// SupporterMail is what the email of a supporter notification says, as of the transition that
// made it.
type SupporterMail struct {
	Type     notification.Type `json:"type"`
	PlanName string            `json:"plan_name"`
	// GraceEndsAt is the end of the grace period of a supporter_payment_failed.
	GraceEndsAt time.Time `json:"grace_ends_at,omitzero"`
	// EndReason is why a supporter_subscription_ended ended (supporters.EndReason).
	EndReason string `json:"end_reason,omitempty"`
}

// graceLayout is how an email writes the end of a grace period: in UTC, which the text says.
const graceLayout = "2006-01-02 15:04"

// SupporterEmailDraft is the email to to of the supporter notification m from the instance named
// name, worded by l. The emails are about the user's paid subscription, so they are
// transactional: no preference turns them off (ADR-0086 §3). The email of a failed payment links
// to the supporter page of the web client on webOrigin, where the payment method is fixed; the
// others link to the notifications. ok is false for a type without an email.
func SupporterEmailDraft(l *i18n.Localizer, name, to string, m SupporterMail, webOrigin string) (d kernel.MailDraft, ok bool) {
	origin := strings.TrimSuffix(webOrigin, "/")
	d = kernel.MailDraft{
		Category: kernel.MailTransactional, To: to,
		Action: &kernel.MailAction{Label: l.ServerMailActionOpenNotifications(), URL: origin + "/notifications"},
		Reason: l.ServerMailReasonSupporter(name),
	}
	switch m.Type {
	case notification.TypeSupporterPaymentFailed:
		d.Subject = l.ServerMailSupporterPaymentFailedSubject(name)
		d.Paragraphs = []string{
			l.ServerMailSupporterPaymentFailedBody(name, m.PlanName),
			l.ServerMailSupporterPaymentFailedDetail(m.GraceEndsAt.UTC().Format(graceLayout)),
		}
		d.Action = &kernel.MailAction{Label: l.ServerMailSupporterPaymentFailedAction(), URL: origin + "/settings/supporter"}
	case notification.TypeSupporterSubscriptionEnded:
		d.Subject = l.ServerMailSupporterSubscriptionEndedSubject(name)
		d.Paragraphs = []string{l.ServerMailSupporterSubscriptionEndedBody(name, m.PlanName, m.EndReason)}
	case notification.TypeSupporterPlanRetired:
		d.Subject = l.ServerMailSupporterPlanRetiredSubject(name)
		d.Paragraphs = []string{l.ServerMailSupporterPlanRetiredBody(name, m.PlanName), l.ServerMailSupporterPlanRetiredDetail()}
	case notification.TypeSupporterServiceClosure:
		d.Subject = l.ServerMailSupporterServiceClosureSubject(name)
		d.Paragraphs = []string{l.ServerMailSupporterServiceClosureBody(name, m.PlanName), l.ServerMailSupporterServiceClosureDetail()}
	default:
		return kernel.MailDraft{}, false
	}
	return d, true
}
