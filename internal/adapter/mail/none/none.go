// SPDX-License-Identifier: AGPL-3.0-only

// Package none implements the mailer port for instances that send no email (mail = none): it
// logs that a message was not sent instead of sending it. In production the log line holds only
// the message's category, since a user's address and a token in a link do not belong in a log.
// In development it holds the whole message, so that a developer can take a link from the log;
// no other component logs message bodies, so a token appears only in this line.
package none

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Options configures a Mailer.
type Options struct {
	// LogMessages also logs each message's addresses, subject, and text (profile = development).
	LogMessages bool
}

// Mailer logs every message at the info level.
type Mailer struct {
	log         *slog.Logger
	logMessages bool
}

var _ kernel.Mailer = (*Mailer)(nil)

// New returns a Mailer that logs to logger; nil means slog.Default().
func New(logger *slog.Logger, opts Options) *Mailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Mailer{log: logger, logMessages: opts.LogMessages}
}

// Send implements kernel.Mailer.
func (m *Mailer) Send(ctx context.Context, msg kernel.MailMessage) error {
	if err := kernel.ValidateMail(msg); err != nil {
		return err
	}
	attrs := []any{slog.String("category", string(msg.Category))}
	if m.logMessages {
		attrs = append(attrs, slog.String("to", msg.To), slog.String("from_name", msg.FromName),
			slog.String("reply_to", msg.ReplyTo), slog.String("subject", msg.Subject), slog.String("text", msg.Text))
	}
	m.log.InfoContext(ctx, "mail not sent (mail = none)", attrs...)
	return nil
}
