// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
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

// MailMessage is a plain-text email to one recipient.
type MailMessage struct {
	// To is a bare address (local@domain).
	To      string
	Subject string
	// Text is the body in plain text, with "\n" line ends.
	Text string
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

// maxSubjectLength bounds the subject in bytes; longer subjects are an error, not cut.
const maxSubjectLength = 998

// ValidateMail checks that m can be sent without header injection: To is one bare address
// and Subject is one line of UTF-8; Text is UTF-8.
func ValidateMail(m MailMessage) error {
	a, err := mail.ParseAddress(m.To)
	if err != nil || a.Name != "" || a.Address != m.To {
		return fmt.Errorf("%w: the recipient must be a bare address", ErrInvalidMail)
	}
	if m.Subject == "" || len(m.Subject) > maxSubjectLength || !utf8.ValidString(m.Subject) || strings.ContainsAny(m.Subject, "\r\n") {
		return fmt.Errorf("%w: the subject must be one line of 1 to %d bytes of UTF-8", ErrInvalidMail, maxSubjectLength)
	}
	if !utf8.ValidString(m.Text) {
		return fmt.Errorf("%w: the body must be UTF-8", ErrInvalidMail)
	}
	return nil
}
