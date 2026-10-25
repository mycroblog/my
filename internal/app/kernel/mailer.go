// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Mailer sends email (overview §3, the Mailer port). The adapters live in internal/adapter/mail:
// mail/none logs messages instead of sending them, and mail/smtp sends them over SMTP. Every
// adapter passes testkit.MailerContract.
//
// Only job handlers call Send (ADR-0086 §1): a use case enqueues a job of its purpose in the
// transaction of the fact that the email tells, and the handler builds the message and sends
// it outside transactions (ADR-0003). A message may carry a secret, such as the link of a
// verification email, so adapters never log a message except mail/none, whose purpose is to
// show it to the operator, and their errors never contain the recipient, the body, or the
// server's reply text: only the SMTP command, the reply code, and the enhanced status code.
type Mailer interface {
	// Send delivers m or fails. It fails with an error wrapping ErrInvalidMail when m does
	// not pass ValidateMail, with one wrapping ErrRecipientRejected when the server refuses
	// the recipient as an address that does not exist, and with one wrapping ErrMailRejected
	// when the server refuses the message for good otherwise. Any other error is temporary:
	// sending again later may succeed.
	Send(ctx context.Context, m MailMessage) error
}

// MailCategory classifies an email (ADR-0086 §3). Only notifications and announcements carry
// the List-Unsubscribe headers; transactional email, such as a verification or a password
// reset link, cannot be unsubscribed from.
type MailCategory string

// The categories of email.
const (
	MailTransactional MailCategory = "transactional"
	MailNotification  MailCategory = "notification"
	MailAnnouncement  MailCategory = "announcement"
)

// Valid reports whether c is one of the categories.
func (c MailCategory) Valid() bool {
	switch c {
	case MailTransactional, MailNotification, MailAnnouncement:
		return true
	}
	return false
}

// The extra headers that a MailMessage may carry (MailMessage.Headers).
const (
	HeaderListUnsubscribe     = "List-Unsubscribe"
	HeaderListUnsubscribePost = "List-Unsubscribe-Post"
)

// MailMessage is an email to one recipient. MailComposer builds it; the adapter adds the
// envelope (From with FromName, Reply-To, Message-ID, Date, Auto-Submitted).
type MailMessage struct {
	Category MailCategory
	// To is a bare address (local@domain).
	To string
	// FromName is the display name of the sender, the server's name; "" sends the bare
	// address of the adapter's sender.
	FromName string
	// ReplyTo is a bare address that replies go to, the instance's contact_email; "" adds no
	// Reply-To.
	ReplyTo string
	Subject string
	// Text is the body in plain text, with "\n" line ends.
	Text string
	// HTML is the body in HTML; "" sends the plain text only.
	HTML string
	// Headers are extra headers by their canonical name: only HeaderListUnsubscribe and
	// HeaderListUnsubscribePost.
	Headers map[string]string
}

// ErrInvalidMail reports a message that ValidateMail rejects.
var ErrInvalidMail = errors.New("invalid mail message")

// ErrMailRejected reports that the mail server refused a message for good: an SMTP reply of
// 5xx to RCPT TO or DATA, or a provider's rejection of the content. Sending it again fails the
// same way.
var ErrMailRejected = errors.New("mail rejected")

// ErrRecipientRejected reports that the mail server refused the recipient as an address that
// does not exist: an SMTP reply of 5xx to RCPT TO with an enhanced status code 5.1.x. It wraps
// ErrMailRejected.
var ErrRecipientRejected = fmt.Errorf("%w: recipient rejected", ErrMailRejected)

// StatusError is an error of a Mailer that carries the enhanced status code (RFC 3463) of the
// server's reply, such as 5.1.1, which the record of a rejected recipient keeps (ADR-0086 §5).
type StatusError struct {
	// Status is the enhanced status code.
	Status string
	Err    error
}

func (e *StatusError) Error() string { return e.Err.Error() }
func (e *StatusError) Unwrap() error { return e.Err }

// EnhancedStatus returns the enhanced status code of the first StatusError in err's chain, ""
// without one.
func EnhancedStatus(err error) string {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status
	}
	return ""
}

// maxHeaderLength bounds the subject, the sender's name, and the values of the extra headers in
// bytes; longer values are an error, not cut.
const maxHeaderLength = 998

// ValidateMail checks that m can be sent without header injection: Category is one of the
// categories; To is one bare address and ReplyTo is empty or one; Subject, FromName, and the
// values of Headers are one line of UTF-8 without control characters; Headers names only the
// List-Unsubscribe headers; Text and HTML are UTF-8.
func ValidateMail(m MailMessage) error {
	if !m.Category.Valid() {
		return fmt.Errorf("%w: the category must be transactional, notification, or announcement", ErrInvalidMail)
	}
	if !bareAddress(m.To) {
		return fmt.Errorf("%w: the recipient must be a bare address", ErrInvalidMail)
	}
	if m.ReplyTo != "" && !bareAddress(m.ReplyTo) {
		return fmt.Errorf("%w: the reply-to must be a bare address", ErrInvalidMail)
	}
	if m.Subject == "" || !headerValue(m.Subject) {
		return fmt.Errorf("%w: the subject must be one line of 1 to %d bytes of UTF-8", ErrInvalidMail, maxHeaderLength)
	}
	if m.FromName != "" && !headerValue(m.FromName) {
		return fmt.Errorf("%w: the sender's name must be one line of at most %d bytes of UTF-8", ErrInvalidMail, maxHeaderLength)
	}
	for name, value := range m.Headers {
		if name != HeaderListUnsubscribe && name != HeaderListUnsubscribePost {
			return fmt.Errorf("%w: the header %q is not allowed", ErrInvalidMail, name)
		}
		if value == "" || !headerValue(value) {
			return fmt.Errorf("%w: the header %s must be one line of 1 to %d bytes of UTF-8", ErrInvalidMail, name, maxHeaderLength)
		}
	}
	if !utf8.ValidString(m.Text) || !utf8.ValidString(m.HTML) {
		return fmt.Errorf("%w: the body must be UTF-8", ErrInvalidMail)
	}
	return nil
}

// bareAddress reports whether s is one address without a display name.
func bareAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

// headerValue reports whether s fits in one header line: UTF-8 of at most maxHeaderLength bytes
// without control characters, which rules out line breaks.
func headerValue(s string) bool {
	if len(s) > maxHeaderLength || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}
