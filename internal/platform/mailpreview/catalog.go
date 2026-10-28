// SPDX-License-Identifier: AGPL-3.0-only

// Package mailpreview renders every email that the server sends from sample inputs, without a
// database, a server, or a mail server, so that a developer can look at them in a browser
// (`scripts/task.sh mail:preview`) and a change to one shows in the diff of its golden files.
//
// The catalog names each email by its job type and, for a job that sends several emails, the
// variant; its draft is the module's own draft function (internal/app/<module>/mail_drafts.go)
// with sample values, and Render composes it with the real kernel.MailComposer, the frame that
// MJML compiles from app/mail/frame.mjml, and the embedded locale catalogs. A test fails when a
// mail job type, a draft function, or a variant of one is missing from the catalog.
//
// The package is a development tool: nothing in the server imports it.
package mailpreview

import (
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/internal/i18n"
)

// The sample instance and recipient of every email.
const (
	sampleName      = "Example Social"
	sampleHost      = "social.example"
	sampleBaseURL   = "https://" + sampleHost
	sampleWebOrigin = "https://" + sampleHost
	sampleContact   = "admin@" + sampleHost
	sampleTo        = "alice@mail.example"
	samplePlan      = "Supporter"
	// sampleUnsubscribeURL is the unsubscribe link of a notification email; the token is a
	// placeholder, not a secret.
	sampleUnsubscribeURL = sampleBaseURL + "/mail/unsubscribe?token=sample-unsubscribe-token"
	sampleToken          = "sample-token"
)

// Site is the sample instance as its emails name it.
var Site = kernel.MailSite{Name: sampleName, URL: sampleBaseURL, ContactEmail: sampleContact}

// sampleClosure is the sample schedule of the service closure, with a message from the
// administrators.
var sampleClosure = identity.ServiceClosureView{
	Stage:       identity.ClosureStageAnnounced,
	ClosesAt:    time.Date(2027, 3, 31, 15, 0, 0, 0, time.UTC),
	ExportUntil: time.Date(2027, 4, 30, 15, 0, 0, 0, time.UTC),
	Message:     "Thank you for being here. Please take your posts with you.",
	AnnouncedAt: time.Date(2027, 1, 31, 15, 0, 0, 0, time.UTC),
}

// sampleGraceEndsAt is the end of the grace period of the sample failed payment.
var sampleGraceEndsAt = time.Date(2027, 2, 7, 9, 30, 0, 0, time.UTC)

// Sample is what an entry's draft is worded with: the localizer of one locale.
type Sample struct {
	L *i18n.Localizer
}

// identityWording is the wording of an identity email of s.
func (s Sample) identityWording() appidentity.MailWording {
	return appidentity.MailWording{L: s.L, Name: sampleName, Host: sampleHost}
}

// timeZone is the time zone of the sample user of s's locale.
func (s Sample) timeZone() string {
	if s.L.Locale() == "ja" {
		return "Asia/Tokyo"
	}
	return identity.DefaultTimeZone
}

// Entry is one email of the catalog.
type Entry struct {
	// JobType is the type of the job that sends the email.
	JobType string
	// Variant tells apart the emails of one job type, such as a notification type; "" when the
	// job sends one email.
	Variant string
	// Draft is the email in the sample's locale, through the module's draft function.
	Draft func(s Sample) (kernel.MailDraft, bool)
}

// Name is the entry's unique name, the stem of its files: the job type, and "-" and the variant
// when there is one.
func (e Entry) Name() string {
	if e.Variant == "" {
		return e.JobType
	}
	return e.JobType + "-" + e.Variant
}

// always wraps a draft that every input has.
func always(d kernel.MailDraft) (kernel.MailDraft, bool) { return d, true }

// Catalog returns every email that the server sends, in a stable order.
func Catalog() []Entry {
	entries := []Entry{
		{JobType: appidentity.JobVerificationEmail, Draft: func(s Sample) (kernel.MailDraft, bool) {
			return always(appidentity.VerificationEmailDraft(s.identityWording(), sampleTo,
				sampleBaseURL+appidentity.PathVerifyEmail+"?token="+sampleToken))
		}},
		{JobType: appidentity.JobPasswordResetEmail, Draft: func(s Sample) (kernel.MailDraft, bool) {
			return always(appidentity.PasswordResetEmailDraft(s.identityWording(), sampleTo,
				sampleBaseURL+appidentity.PathResetPassword+"?token="+sampleToken))
		}},
	}
	for _, n := range []appidentity.PasswordNotice{appidentity.PasswordChanged, appidentity.PasswordRemoved} {
		entries = append(entries, Entry{JobType: appidentity.JobPasswordNoticeEmail, Variant: string(n), Draft: func(s Sample) (kernel.MailDraft, bool) {
			return appidentity.PasswordNoticeEmailDraft(s.identityWording(), sampleTo, n, sampleBaseURL+appidentity.PathForgotPassword)
		}})
	}
	for _, e := range identity.ClosureEmails() {
		entries = append(entries, Entry{JobType: appidentity.JobClosureEmail, Variant: string(e), Draft: func(s Sample) (kernel.MailDraft, bool) {
			return appidentity.ClosureEmailDraft(s.identityWording(), sampleTo, e, sampleClosure, s.timeZone(), sampleWebOrigin+"/settings/import-export")
		}})
	}
	for _, t := range []notification.Type{
		notification.TypeAdminReport, notification.TypeAdminSignUp, notification.TypeAdminAppeal,
		notification.TypeAdminWebhookDisabled, notification.TypeAdminCustomEmojiRequest, notification.TypeAdminAccountMoved,
		notification.TypeAdminServiceClosureDue, notification.TypeAdminErrorGroup,
	} {
		entries = append(entries, Entry{JobType: appnotification.JobModeratorEmail, Variant: string(t), Draft: func(s Sample) (kernel.MailDraft, bool) {
			return appnotification.ModeratorEmailDraft(s.L, sampleName, sampleTo, t, sampleWebOrigin, sampleUnsubscribeURL)
		}})
	}
	for _, m := range []struct {
		variant string
		mail    appnotification.SupporterMail
	}{
		{string(notification.TypeSupporterPaymentFailed), appnotification.SupporterMail{Type: notification.TypeSupporterPaymentFailed, PlanName: samplePlan, GraceEndsAt: sampleGraceEndsAt}},
		{string(notification.TypeSupporterSubscriptionEnded) + "-" + string(supporters.EndLapsed), appnotification.SupporterMail{Type: notification.TypeSupporterSubscriptionEnded, PlanName: samplePlan, EndReason: string(supporters.EndLapsed)}},
		{string(notification.TypeSupporterSubscriptionEnded) + "-" + string(supporters.EndCanceled), appnotification.SupporterMail{Type: notification.TypeSupporterSubscriptionEnded, PlanName: samplePlan, EndReason: string(supporters.EndCanceled)}},
		{string(notification.TypeSupporterPlanRetired), appnotification.SupporterMail{Type: notification.TypeSupporterPlanRetired, PlanName: samplePlan}},
		{string(notification.TypeSupporterServiceClosure), appnotification.SupporterMail{Type: notification.TypeSupporterServiceClosure, PlanName: samplePlan}},
	} {
		entries = append(entries, Entry{JobType: appnotification.JobSupporterEmail, Variant: m.variant, Draft: func(s Sample) (kernel.MailDraft, bool) {
			return appnotification.SupporterEmailDraft(s.L, sampleName, sampleTo, m.mail, sampleWebOrigin)
		}})
	}
	return entries
}
