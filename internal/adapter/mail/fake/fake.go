// SPDX-License-Identifier: AGPL-3.0-only

// Package fake implements the mailer port for the end-to-end tests and the local demonstration
// (mail = fake): a mailer that counts as one that delivers, so that the server asks its users to
// confirm their addresses and mails password reset links as with mail = smtp, and that sends
// nothing. Only the test builds (build tag e2e_fakes) link it, and only with the development
// profile.
//
// It keeps each message it was asked to send in the server's log, whole: one line with the
// message LogMessage and the attributes category, to, from_name, reply_to, subject, and text. A
// test reads the mail of its own person there, as the person would open it; a token of a link
// appears in that line alone.
package fake

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
)

// LogMessage is the message of the log line of each mail. It starts as the line of mail = none
// does, so that one reader takes the mail of both from a log.
const LogMessage = "mail not sent (mail = fake)"

// Mailer logs every message at the info level instead of sending it.
type Mailer struct {
	log *slog.Logger
}

var _ kernel.Mailer = (*Mailer)(nil)

// New returns a Mailer that logs to logger; nil means slog.Default().
func New(logger *slog.Logger) *Mailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Mailer{log: logger}
}

// Send implements kernel.Mailer. It refuses a message that no mailer may send
// (kernel.ErrInvalidMail) and accepts every other one, whatever its recipient.
func (m *Mailer) Send(ctx context.Context, msg kernel.MailMessage) error {
	if err := kernel.ValidateMail(msg); err != nil {
		return err
	}
	m.log.InfoContext(ctx, LogMessage,
		slog.String("category", string(msg.Category)), slog.String("to", msg.To), slog.String("from_name", msg.FromName),
		slog.String("reply_to", msg.ReplyTo), slog.String("subject", msg.Subject), slog.String("text", msg.Text))
	return nil
}
