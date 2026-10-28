// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// The drafts of the identity module's emails. Each is a pure function of what the email says:
// the job handlers of mail_jobs.go and closure_mail.go call them with what they read and the
// secrets they issue, and the email preview (internal/platform/mailpreview) calls them with
// sample values, so that what a developer previews is what a user gets.

// MailWording is what words an email to a user: the localizer of the user's locale, and the
// instance's name and domain name.
type MailWording struct {
	L *i18n.Localizer
	// Name is the instance's display name, or its domain name when it has none.
	Name string
	// Host is the instance's domain name.
	Host string
}

// PasswordNotice is what happened to a user's password, for the email that tells them.
type PasswordNotice string

const (
	// PasswordChanged is a password set, by a reset or a change.
	PasswordChanged PasswordNotice = "changed"
	// PasswordRemoved is a password removed by going passwordless.
	PasswordRemoved PasswordNotice = "removed"
)

// VerificationEmailDraft is the email to to with link, the URL that verifies the address.
func VerificationEmailDraft(w MailWording, to, link string) kernel.MailDraft {
	return kernel.MailDraft{
		Category: kernel.MailTransactional, To: to,
		Subject: w.L.ServerMailVerifyEmailSubject(w.Name),
		Paragraphs: []string{
			w.L.ServerMailVerifyEmailBody(w.Name),
			w.L.ServerMailVerifyEmailDetail(int(EmailVerificationTTL / time.Hour)),
		},
		Action: &kernel.MailAction{Label: w.L.ServerMailVerifyEmailAction(), URL: link},
		Reason: w.L.ServerMailVerifyEmailReason(w.Host),
	}
}

// PasswordResetEmailDraft is the email to to with link, the URL that sets a new password.
func PasswordResetEmailDraft(w MailWording, to, link string) kernel.MailDraft {
	return kernel.MailDraft{
		Category: kernel.MailTransactional, To: to,
		Subject: w.L.ServerMailPasswordResetSubject(w.Name),
		Paragraphs: []string{
			w.L.ServerMailPasswordResetBody(w.Name),
			w.L.ServerMailPasswordResetDetail(int(PasswordResetTTL / time.Minute)),
		},
		Action: &kernel.MailAction{Label: w.L.ServerMailActionSetNewPassword(), URL: link},
		Reason: w.L.ServerMailPasswordResetReason(w.Host),
	}
}

// PasswordNoticeEmailDraft is the email to to that tells that notice happened to their
// password, with forgotLink, the URL of the page that asks for a reset in case they did not do
// it. ok is false for an unknown notice.
func PasswordNoticeEmailDraft(w MailWording, to string, notice PasswordNotice, forgotLink string) (d kernel.MailDraft, ok bool) {
	d = kernel.MailDraft{
		Category: kernel.MailTransactional, To: to,
		Action: &kernel.MailAction{Label: w.L.ServerMailActionSetNewPassword(), URL: forgotLink},
		Reason: w.L.ServerMailReasonAccount(w.Name),
	}
	switch notice {
	case PasswordChanged:
		d.Subject = w.L.ServerMailPasswordChangedSubject(w.Name)
		d.Paragraphs = []string{w.L.ServerMailPasswordChangedBody(w.Name), w.L.ServerMailPasswordChangedDetail()}
	case PasswordRemoved:
		d.Subject = w.L.ServerMailPasswordRemovedSubject(w.Name)
		d.Paragraphs = []string{w.L.ServerMailPasswordRemovedBody(w.Name), w.L.ServerMailPasswordRemovedDetail()}
	default:
		return kernel.MailDraft{}, false
	}
	return d, true
}

// ClosureEmailDraft is the email e to to about the service closure view, with its dates in the
// time zone zone (UTC when it is "" or unknown) and exportLink, the URL of the web client's page
// that exports the user's data. ok is false for an unknown e.
func ClosureEmailDraft(w MailWording, to string, e identity.ClosureEmail, view identity.ServiceClosureView, zone, exportLink string) (d kernel.MailDraft, ok bool) {
	loc, err := time.LoadLocation(zone)
	if zone == "" || err != nil {
		loc, zone = time.UTC, identity.DefaultTimeZone
	}
	at := func(t time.Time) string { return t.In(loc).Format(closureMailLayout) + " (" + zone + ")" }
	closesAt, exportUntil := at(view.ClosesAt), at(view.ExportUntil)
	l := w.L
	exportAction := &kernel.MailAction{Label: l.ServerMailServiceClosureActionExport(), URL: exportLink}
	d = kernel.MailDraft{Category: kernel.MailTransactional, To: to, Reason: l.ServerMailServiceClosureReason(w.Name), Action: exportAction}
	// What the announcement says, in every email of the notice period but the withdrawal.
	notice := func(first string) []string {
		p := []string{first, l.ServerMailServiceClosureExport(exportUntil), l.ServerMailServiceClosureMove()}
		if msg := strings.TrimSpace(view.Message); msg != "" {
			p = append(p, l.ServerMailServiceClosureMessage(msg))
		}
		return p
	}
	switch e {
	case identity.ClosureEmailAnnounced:
		d.Subject = l.ServerMailServiceClosureAnnouncedSubject(w.Name, closesAt)
		d.Paragraphs = notice(l.ServerMailServiceClosureAnnouncedBody(w.Name, closesAt))
	case identity.ClosureEmailRescheduled:
		d.Subject = l.ServerMailServiceClosureRescheduledSubject(w.Name, closesAt)
		d.Paragraphs = notice(l.ServerMailServiceClosureRescheduledBody(w.Name, closesAt))
	case identity.ClosureEmailWeekBefore:
		d.Subject = l.ServerMailServiceClosureWeekBeforeSubject(w.Name, closesAt)
		d.Paragraphs = notice(l.ServerMailServiceClosureWeekBeforeBody(w.Name, closesAt))
	case identity.ClosureEmailDayBefore:
		d.Subject = l.ServerMailServiceClosureDayBeforeSubject(w.Name, closesAt)
		d.Paragraphs = notice(l.ServerMailServiceClosureDayBeforeBody(w.Name, closesAt))
	case identity.ClosureEmailWithdrawn:
		d.Subject = l.ServerMailServiceClosureWithdrawnSubject(w.Name)
		d.Paragraphs = []string{l.ServerMailServiceClosureWithdrawnBody(w.Name)}
		d.Action = nil
	case identity.ClosureEmailClosed:
		d.Subject = l.ServerMailServiceClosureClosedSubject(w.Name)
		d.Paragraphs = []string{l.ServerMailServiceClosureClosedBody(w.Name, exportUntil), l.ServerMailServiceClosureClosedDetail()}
	case identity.ClosureEmailExportWeekBefore:
		d.Subject = l.ServerMailServiceClosureExportWeekBeforeSubject(w.Name)
		d.Paragraphs = []string{l.ServerMailServiceClosureExportWeekBeforeBody(w.Name, exportUntil), l.ServerMailServiceClosureClosedDetail()}
	default:
		return kernel.MailDraft{}, false
	}
	return d, true
}
