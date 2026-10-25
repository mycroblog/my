// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

// MailerHarness is one mailer under test with a view of what it delivered.
type MailerHarness struct {
	Mailer kernel.Mailer
	// Delivered returns the messages delivered so far, decoded back into MailMessages, in order.
	Delivered func() []kernel.MailMessage
	// The recipients that the mailer's backend refuses, for the cases of the rejections; ""
	// skips a case, for an adapter whose backend refuses nothing (mail/none).
	// UnknownRecipient is refused as an address that does not exist (SMTP: 550 5.1.1 to RCPT
	// TO), RejectedRecipient is refused for good for another reason (SMTP: 554 5.7.1 to DATA),
	// and DeferredRecipient is refused for now (SMTP: 451 4.3.0 to RCPT TO).
	UnknownRecipient  string
	RejectedRecipient string
	DeferredRecipient string
}

// MailerContract verifies a kernel.Mailer adapter: it delivers a plain-text message with a
// non-ASCII subject, sender's name, and body intact, with its Reply-To, and refuses messages
// that would inject headers (a line break in the subject, the sender's name, the Reply-To, or
// an extra header, a header that is not allowed, a recipient with a display name or a second
// address) with kernel.ErrInvalidMail without delivering anything. It classifies the backend's refusals: an
// address that does not exist as kernel.ErrRecipientRejected (which is kernel.ErrMailRejected),
// another permanent refusal as kernel.ErrMailRejected only, and a temporary one as neither,
// with errors that never contain the recipient's address.
func MailerContract(t *testing.T, newHarness func(t *testing.T) MailerHarness) {
	t.Helper()
	t.Run("DeliversAMessage", func(t *testing.T) {
		h := newHarness(t)
		m := kernel.MailMessage{
			Category: kernel.MailTransactional,
			To:       "alice@example.com",
			FromName: "例のソーシャル \"social.example\"",
			ReplyTo:  "admin@social.example",
			Subject:  "Confirm your email address for social.example — メールアドレスの確認",
			Text: "Welcome.\n\nOpen this link:\n\nhttps://social.example/auth/verify_email?token=my_evt_" + strings.Repeat("A", 43) +
				"\n\n" + strings.Repeat("A long line that a transfer encoding has to wrap. ", 4) + "\nこんにちは。\n",
		}
		if err := h.Mailer.Send(t.Context(), m); err != nil {
			t.Fatalf("Send() = %v", err)
		}
		got := h.Delivered()
		if len(got) != 1 {
			t.Fatalf("delivered %d messages, want 1", len(got))
		}
		if got[0].To != m.To || got[0].Subject != m.Subject || got[0].FromName != m.FromName || got[0].ReplyTo != m.ReplyTo ||
			strings.TrimRight(got[0].Text, "\n") != strings.TrimRight(m.Text, "\n") {
			t.Errorf("delivered %+v, want %+v", got[0], m)
		}
	})
	t.Run("RefusesHeaderInjection", func(t *testing.T) {
		h := newHarness(t)
		const tr = kernel.MailTransactional
		for _, m := range []kernel.MailMessage{
			{Category: tr, To: "alice@example.com", Subject: "Hello\r\nBcc: mallory@example.com", Text: "x"},
			{Category: tr, To: "Alice <alice@example.com>", Subject: "Hello", Text: "x"},
			{Category: tr, To: "alice@example.com, mallory@example.com", Subject: "Hello", Text: "x"},
			{Category: tr, To: "alice@example.com\r\nBcc: mallory@example.com", Subject: "Hello", Text: "x"},
			{Category: tr, To: "alice@example.com", Subject: "", Text: "x"},
			{Category: tr, To: "alice@example.com", FromName: "Example\r\nBcc: mallory@example.com", Subject: "Hello", Text: "x"},
			{Category: tr, To: "alice@example.com", ReplyTo: "admin@example.com\r\nBcc: mallory@example.com", Subject: "Hello", Text: "x"},
			{Category: tr, To: "alice@example.com", Subject: "Hello", Text: "x", Headers: map[string]string{"Bcc": "mallory@example.com"}},
			{Category: tr, To: "alice@example.com", Subject: "Hello", Text: "x", Headers: map[string]string{
				kernel.HeaderListUnsubscribe: "<https://social.example/>\r\nBcc: mallory@example.com",
			}},
			{To: "alice@example.com", Subject: "Hello", Text: "x"},
		} {
			if err := h.Mailer.Send(t.Context(), m); !errors.Is(err, kernel.ErrInvalidMail) {
				t.Errorf("Send(%q, %q) = %v, want ErrInvalidMail", m.To, m.Subject, err)
			}
		}
		if got := h.Delivered(); len(got) != 0 {
			t.Errorf("delivered %d invalid messages", len(got))
		}
	})
	t.Run("ClassifiesRejections", func(t *testing.T) {
		h := newHarness(t)
		for _, c := range []struct {
			to                  string
			recipient, rejected bool
		}{
			{h.UnknownRecipient, true, true},
			{h.RejectedRecipient, false, true},
			{h.DeferredRecipient, false, false},
		} {
			if c.to == "" {
				continue
			}
			err := h.Mailer.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: c.to, Subject: "Hello", Text: "x"})
			if err == nil {
				t.Errorf("Send(%q) succeeded", c.to)
				continue
			}
			if errors.Is(err, kernel.ErrRecipientRejected) != c.recipient || errors.Is(err, kernel.ErrMailRejected) != c.rejected ||
				errors.Is(err, kernel.ErrInvalidMail) {
				t.Errorf("Send(%q) = %v; want ErrRecipientRejected %t, ErrMailRejected %t", c.to, err, c.recipient, c.rejected)
			}
			if local, _, _ := strings.Cut(c.to, "@"); strings.Contains(err.Error(), local) {
				t.Errorf("Send(%q) = %v, which names the recipient", c.to, err)
			}
		}
		if got := h.Delivered(); len(got) != 0 {
			t.Errorf("delivered %d refused messages", len(got))
		}
	})
}
