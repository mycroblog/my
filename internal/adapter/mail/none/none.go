// SPDX-License-Identifier: AGPL-3.0-only

// Package none implements the mailer port by logging messages instead of sending them
// (mail = none). It is for development and for instances without email: the operator reads
// the message, including any link it carries, from the log. No other component logs message
// bodies, so a token in a link appears only in this log line.
package none

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Mailer logs every message at the info level.
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

// Send implements kernel.Mailer.
func (m *Mailer) Send(ctx context.Context, msg kernel.MailMessage) error {
	if err := kernel.ValidateMail(msg); err != nil {
		return err
	}
	m.log.InfoContext(ctx, "mail not sent (mail = none)",
		slog.String("category", string(msg.Category)), slog.String("to", msg.To), slog.String("from_name", msg.FromName),
		slog.String("reply_to", msg.ReplyTo), slog.String("subject", msg.Subject), slog.String("text", msg.Text))
	return nil
}
