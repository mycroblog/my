// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"regexp"
	"time"
)

// The suppression of email addresses that do not receive email (ADR-0086 §5, data-model §4.1):
// a failure of delivery (email_delivery_failures) suppresses its address until a lift
// (email_suppression_lifts); email_suppressions holds the addresses suppressed now.

// EmailFailureKind is what a delivery failure tells about its address.
type EmailFailureKind string

const (
	// EmailHardBounce: the address does not exist or refuses email for good.
	EmailHardBounce EmailFailureKind = "hard_bounce"
	// EmailComplaint: the recipient marked an email as spam.
	EmailComplaint EmailFailureKind = "complaint"
)

// EmailFailureSource is where the fact of a delivery failure came from.
type EmailFailureSource string

const (
	// EmailFailureRejection: the mail server refused the recipient when the email was sent.
	EmailFailureRejection EmailFailureSource = "rejection"
	// EmailFailureSES: a bounce or complaint notification of Amazon SES.
	EmailFailureSES EmailFailureSource = "ses"
)

// EmailDeliveryFailure is an event: an email to an address bounced for good or drew a
// complaint. FeedbackKey is unique, so that the same fact recorded again is one event.
type EmailDeliveryFailure struct {
	ID             string
	EmailAddressID string
	Kind           EmailFailureKind
	Source         EmailFailureSource
	FeedbackKey    string
	// EnhancedStatus is the RFC 3463 code that the source named, such as 5.1.1, or "".
	EnhancedStatus string
	OccurredAt     time.Time
}

// enhancedStatusCode is the form of an RFC 3463 enhanced status code.
var enhancedStatusCode = regexp.MustCompile(`^[245]\.\d{1,3}\.\d{1,3}$`)

// EnhancedStatusCode returns code when it is an RFC 3463 enhanced status code, "" otherwise.
func EnhancedStatusCode(code string) string {
	if !enhancedStatusCode.MatchString(code) {
		return ""
	}
	return code
}

// EmailLiftReason is why the suppression of an address ended.
type EmailLiftReason string

const (
	// EmailLiftReverified: the user verified the address again.
	EmailLiftReverified EmailLiftReason = "reverified"
	// EmailLiftModerator: a moderator lifted the suppression.
	EmailLiftModerator EmailLiftReason = "moderator"
)

// EmailSuppressionLift is an event: the failures of an address up to LiftedAt no longer
// suppress it.
type EmailSuppressionLift struct {
	ID             string
	EmailAddressID string
	Reason         EmailLiftReason
	LiftedAt       time.Time
}

// EmailSuppression is the projection of a suppressed address: Kind is EmailHardBounce when a
// hard bounce happened after the last lift, EmailComplaint otherwise, and FailureID and Since
// are the earliest failure of that kind after the last lift.
type EmailSuppression struct {
	EmailAddressID string
	Kind           EmailFailureKind
	FailureID      string
	Since          time.Time
}

// MailClass is the kind of an email, which decides whether a suppression stops it.
type MailClass int

const (
	// MailVerification is a verification email that the user asked for, which every
	// suppression lets through, so that the user can show that the address works again.
	MailVerification MailClass = iota
	// MailTransactional is an email that the user needs, such as a password reset link or a
	// notice that the password changed.
	MailTransactional
	// MailNotification is a notification or announcement email.
	MailNotification
)

// Stops reports whether the suppression s stops an email of class c: a hard bounce stops every
// email but a verification, and a complaint stops the notifications and announcements only.
func (s EmailSuppression) Stops(c MailClass) bool {
	switch s.Kind {
	case EmailHardBounce:
		return c != MailVerification
	case EmailComplaint:
		return c == MailNotification
	}
	return false
}
