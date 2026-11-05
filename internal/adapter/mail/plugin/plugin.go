// SPDX-License-Identifier: AGPL-3.0-only

// Package plugin is the kernel.Mailer that delegates to an operator's plugin (ADR-0146 §11): it hands over the parts of a
// message, and the plugin adds the Message-ID, Date, and Auto-Submitted headers that the other mailers write themselves.
//
// A message may carry a secret, so nothing here logs, and its errors hold neither the recipient, the body, nor anything
// the plugin said: the client drops the text of a failed call, and the only string of a response that reaches an error is
// an enhanced status code that passes identity.EnhancedStatusCode (ADR-0146 §10).
package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	mailerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1/mailerv1connect"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Mailer is the kernel.Mailer of a plugin. It is safe for concurrent use.
type Mailer struct {
	client mailerv1connect.MailerServiceClient
	from   string
}

var _ kernel.Mailer = (*Mailer)(nil)

// New returns the Mailer that reaches its plugin through conn. from is the sender, the bare address of mail_from. It makes
// no call.
func New(conn *client.Conn, from string) (*Mailer, error) {
	if a, err := mail.ParseAddress(from); err != nil || a.Name != "" || a.Address != from {
		return nil, errors.New("mail plugin: the sender must be a bare address")
	}
	return &Mailer{client: mailerv1connect.NewMailerServiceClient(conn.RPC), from: from}, nil
}

var categories = map[kernel.MailCategory]mailerv1.Category{
	kernel.MailTransactional: mailerv1.Category_CATEGORY_TRANSACTIONAL,
	kernel.MailNotification:  mailerv1.Category_CATEGORY_NOTIFICATION,
	kernel.MailAnnouncement:  mailerv1.Category_CATEGORY_ANNOUNCEMENT,
}

// Send implements kernel.Mailer. It checks msg before the call, so that a message that would inject headers never reaches
// a plugin. A failed call, a message over the kind's limit, and an outcome the server does not know are temporary
// failures, which the job retries (ADR-0146 §10, §12, §15).
func (m *Mailer) Send(ctx context.Context, msg kernel.MailMessage) error {
	if err := kernel.ValidateMail(msg); err != nil {
		return err
	}
	res, err := m.client.Send(ctx, &mailerv1.SendRequest{
		Category:     categories[msg.Category],
		To:           msg.To,
		EnvelopeFrom: m.from,
		FromName:     msg.FromName,
		ReplyTo:      msg.ReplyTo,
		Subject:      msg.Subject,
		Text:         msg.Text,
		Html:         msg.HTML,
		Headers:      msg.Headers,
	})
	if err != nil {
		return fmt.Errorf("mail plugin: %w", err)
	}
	switch res.GetOutcome() {
	case mailerv1.Outcome_OUTCOME_SENT:
		return nil
	case mailerv1.Outcome_OUTCOME_INVALID:
		return fmt.Errorf("mail plugin: %w", kernel.ErrInvalidMail)
	case mailerv1.Outcome_OUTCOME_RECIPIENT_REJECTED:
		return refusal(kernel.ErrRecipientRejected, "5.", res.GetEnhancedStatus())
	case mailerv1.Outcome_OUTCOME_REJECTED:
		return refusal(kernel.ErrMailRejected, "5.", res.GetEnhancedStatus())
	case mailerv1.Outcome_OUTCOME_TEMPORARY:
		return refusal(errRefusedForNow, "4.", res.GetEnhancedStatus())
	default:
		return fmt.Errorf("mail plugin: the outcome is %d, which is not a result", int32(res.GetOutcome()))
	}
}

var errRefusedForNow = errors.New("refused for now")

// refusal returns the error of a refusal with the plugin's enhanced status code, which it keeps only when it is one and of
// the class that the outcome means: the code goes into the error and the record of a rejected recipient.
func refusal(err error, class, status string) error {
	status = identity.EnhancedStatusCode(status)
	if !strings.HasPrefix(status, class) {
		return fmt.Errorf("mail plugin: %w", err)
	}
	return &kernel.StatusError{Status: status, Err: fmt.Errorf("mail plugin: %w: %s", err, status)}
}
