// SPDX-License-Identifier: AGPL-3.0-only

package smtp_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"mime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/mail/internal/mailtest"
	"github.com/mycroblog/my/internal/adapter/mail/smtp"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/mailpittest"
)

// The integration tests on a Mailpit server (pkg/testkit/mailpittest), which run only with
// TEST_MAIL=container (scripts/task.sh go:test-mail; CI in the slow suite): what the adapter
// sends, read back from a real SMTP server that parses it with a parser of its own.

// mailpitMailer returns a Mailer that sends to s, as the user and password when user is set.
func mailpitMailer(t *testing.T, s *mailpittest.Server, user, password string, tc *tls.Config) *smtp.Mailer {
	t.Helper()
	m, err := smtp.New(smtp.Options{
		Host: s.Host, Port: s.Port, Username: user, Password: password, From: "noreply@social.example", TLSConfig: tc,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// mailpitDelivered returns the messages that s received, the oldest first, decoded from their
// source.
func mailpitDelivered(t *testing.T, s *mailpittest.Server) []kernel.MailMessage {
	t.Helper()
	list := s.Messages(t)
	slices.Reverse(list)
	out := make([]kernel.MailMessage, 0, len(list))
	for _, m := range list {
		msg, _, err := mailtest.Parse(s.Raw(t, m.ID))
		if err != nil {
			t.Fatalf("message %s: %v", m.ID, err)
		}
		out = append(out, msg)
	}
	return out
}

func TestMailpitMailerContract(t *testing.T) {
	testkit.MailerContract(t, func(t *testing.T) testkit.MailerHarness {
		s := mailpittest.New(t, mailpittest.Options{STARTTLS: true})
		// Mailpit accepts every recipient, so the cases of the rejections skip.
		return testkit.MailerHarness{
			Mailer:    mailpitMailer(t, s, "", "", s.TLSConfig),
			Delivered: func() []kernel.MailMessage { return mailpitDelivered(t, s) },
		}
	})
}

// The headers and the body of a message with a Japanese subject, a long line, and a link
// longer than a line of quoted-printable, as the adapter sends them and as Mailpit reads them.
func TestMailpitMessageForm(t *testing.T) {
	s := mailpittest.New(t, mailpittest.Options{STARTTLS: true})
	link := "https://social.example/auth/verify_email?token=my_evt_" + strings.Repeat("Ab0_-", 9)
	msg := kernel.MailMessage{
		Category: kernel.MailTransactional,
		To:       "alice@example.com",
		FromName: "例のソーシャル",
		ReplyTo:  "admin@social.example",
		Subject:  "メールアドレスの確認 — " + strings.Repeat("例のソーシャルへようこそ。", 6),
		Text: "ようこそ。\n\n次のリンクを開いてください。\n\n" + link + "\n\n" +
			strings.Repeat("改行のない長い行。", 40) + "\n",
		HTML: "<!doctype html>\n<html lang=\"ja\" dir=\"ltr\"><body><p>ようこそ。</p>\n" +
			"<p><a href=\"" + link + "\">メールアドレスを確認する</a></p>\n<p>" + strings.Repeat("長い行。", 200) + "</p></body></html>\n",
	}
	sent := time.Now()
	if err := mailpitMailer(t, s, "", "", s.TLSConfig).Send(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	got := s.WaitFor(t, msg.To, 1, 10*time.Second)
	raw := s.Raw(t, got.ID)

	t.Run("CRLF line ends", func(t *testing.T) {
		if n := bytes.Count(raw, []byte("\n")); n == 0 || n != bytes.Count(raw, []byte("\r\n")) {
			t.Errorf("%d line feeds, %d of them after a carriage return", n, bytes.Count(raw, []byte("\r\n")))
		}
		head, body, _ := bytes.Cut(raw, []byte("\r\n\r\n"))
		for line := range strings.Lines(string(body)) {
			// Quoted-printable lines are at most 76 characters (RFC 2045 §6.7).
			if n := len(strings.TrimRight(line, "\r\n")); n > 76 {
				t.Errorf("a body line of %d bytes: %q", n, line)
			}
		}
		if !bytes.Contains(body, []byte("=\r\n")) {
			t.Error("the long lines have no soft line break")
		}
		for line := range strings.Lines(string(head)) {
			if !sevenBit(line) {
				t.Errorf("a header line with 8-bit bytes: %q", line)
			}
		}
	})

	decoded, header, err := mailtest.Parse(raw)
	if err != nil {
		t.Fatalf("parse the source: %v\n%s", err, raw)
	}

	t.Run("headers", func(t *testing.T) {
		// The subject is Q-encoded words (RFC 2047), folded between the words in the source.
		if v := header.Get("Subject"); !strings.HasPrefix(v, "=?utf-8?q?") || !bytes.Contains(raw, []byte("?=\r\n =?utf-8?q?")) {
			t.Errorf("Subject = %q, want Q-encoded words folded between them", v)
		}
		if decoded.Subject != msg.Subject || got.Subject != msg.Subject {
			t.Errorf("Subject decodes to %q, and to %q in Mailpit, want %q", decoded.Subject, got.Subject, msg.Subject)
		}
		if got.From != (mailpittest.Address{Name: msg.FromName, Address: "noreply@social.example"}) || decoded.FromName != msg.FromName {
			t.Errorf("From = %+v in Mailpit, %q decoded, want %q <noreply@social.example>", got.From, decoded.FromName, msg.FromName)
		}
		if len(got.To) != 1 || got.To[0].Address != msg.To || header.Get("To") != msg.To {
			t.Errorf("To = %+v in Mailpit, %q in the source, want %s", got.To, header.Get("To"), msg.To)
		}
		if len(got.ReplyTo) != 1 || got.ReplyTo[0].Address != msg.ReplyTo {
			t.Errorf("Reply-To = %+v in Mailpit, want %s", got.ReplyTo, msg.ReplyTo)
		}
		date, err := header.Date()
		if err != nil || date.Before(sent.Add(-time.Minute)) || date.After(time.Now().Add(time.Minute)) || !got.Date.Equal(date) {
			t.Errorf("Date = %q (%v), %v in Mailpit, want the time of sending", header.Get("Date"), err, got.Date)
		}
		id, ok := strings.CutSuffix(strings.TrimPrefix(header.Get("Message-Id"), "<"), "@social.example>")
		if !ok || !kernel.ValidID(id) || got.MessageID != id+"@social.example" {
			t.Errorf("Message-ID = %q, %q in Mailpit, want <UUIDv7@social.example>", header.Get("Message-Id"), got.MessageID)
		}
		if mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type")); err != nil || mediaType != "multipart/alternative" || params["boundary"] == "" {
			t.Errorf("Content-Type = %q", header.Get("Content-Type"))
		}
		// mailtest.Parse checked that each part is "text/plain; charset=utf-8" or
		// "text/html; charset=utf-8" with Content-Transfer-Encoding: quoted-printable.
		if v := header.Get("Content-Transfer-Encoding"); v != "" {
			t.Errorf("Content-Transfer-Encoding of the multipart = %q, want none", v)
		}
		if header.Get("Mime-Version") != "1.0" || header.Get("Auto-Submitted") != "auto-generated" {
			t.Errorf("MIME-Version = %q, Auto-Submitted = %q", header.Get("Mime-Version"), header.Get("Auto-Submitted"))
		}
	})

	t.Run("body", func(t *testing.T) {
		if decoded.Text != msg.Text || decoded.HTML != msg.HTML {
			t.Errorf("decoded text %q and HTML %q, want %q and %q", decoded.Text, decoded.HTML, msg.Text, msg.HTML)
		}
		text := strings.ReplaceAll(got.Text, "\r\n", "\n")
		if strings.TrimRight(text, "\n") != strings.TrimRight(msg.Text, "\n") {
			t.Errorf("Mailpit's text = %q, want %q", text, msg.Text)
		}
		if !strings.Contains(text, "\n"+link+"\n") || !strings.Contains(got.HTML, `href="`+link+`"`) {
			t.Errorf("the link %s is not intact in Mailpit's text %q or HTML %q", link, text, got.HTML)
		}
	})
}

// sevenBit reports whether s is 7-bit.
func sevenBit(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Authentication with Mailpit's AUTH PLAIN after STARTTLS: the right password sends, and a wrong
// one or none sends nothing.
func TestMailpitAuthentication(t *testing.T) {
	s := mailpittest.New(t, mailpittest.Options{STARTTLS: true, Username: "mailer", Password: "s3cret"})
	msg := kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"}
	if err := mailpitMailer(t, s, "mailer", "wrong", s.TLSConfig).Send(t.Context(), msg); err == nil || !strings.Contains(err.Error(), "AUTH") ||
		errors.Is(err, kernel.ErrMailRejected) {
		t.Errorf("Send with a wrong password = %v, want a temporary AUTH error", err)
	}
	if err := mailpitMailer(t, s, "", "", s.TLSConfig).Send(t.Context(), msg); err == nil {
		t.Error("Send without authenticating to a server that requires it succeeded")
	}
	if n := len(s.Messages(t)); n != 0 {
		t.Fatalf("Mailpit received %d messages without a valid authentication", n)
	}
	if err := mailpitMailer(t, s, "mailer", "s3cret", s.TLSConfig).Send(t.Context(), msg); err != nil {
		t.Fatalf("Send with the right password = %v", err)
	}
	if got := mailpitDelivered(t, s); len(got) != 1 || got[0].To != msg.To {
		t.Errorf("delivered %+v, want the message", got)
	}
}

// A server that offers no AUTH does not get the message without the authentication that the
// configuration asks for.
func TestMailpitWithoutAuthenticationOffered(t *testing.T) {
	s := mailpittest.New(t, mailpittest.Options{STARTTLS: true})
	msg := kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"}
	if err := mailpitMailer(t, s, "mailer", "s3cret", s.TLSConfig).Send(t.Context(), msg); err == nil || !strings.Contains(err.Error(), "AUTH") {
		t.Errorf("Send with credentials to a server without AUTH = %v, want an AUTH error", err)
	}
	if n := len(s.Messages(t)); n != 0 {
		t.Errorf("Mailpit received %d messages", n)
	}
}

// The adapter refuses a server without STARTTLS, and one whose certificate it does not trust,
// before it sends anything.
func TestMailpitRequiresTrustedSTARTTLS(t *testing.T) {
	msg := kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"}
	t.Run("plain SMTP", func(t *testing.T) {
		s := mailpittest.New(t, mailpittest.Options{})
		tc := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
		if err := mailpitMailer(t, s, "", "", tc).Send(t.Context(), msg); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
			t.Errorf("Send to a server without STARTTLS = %v", err)
		}
		if n := len(s.Messages(t)); n != 0 {
			t.Errorf("Mailpit received %d messages over plain SMTP", n)
		}
	})
	t.Run("untrusted certificate", func(t *testing.T) {
		s := mailpittest.New(t, mailpittest.Options{STARTTLS: true})
		tc := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool()}
		var unknown x509.UnknownAuthorityError
		if err := mailpitMailer(t, s, "", "", tc).Send(t.Context(), msg); !errors.As(err, &unknown) {
			t.Errorf("Send with an untrusted certificate = %v, want x509.UnknownAuthorityError", err)
		}
		if n := len(s.Messages(t)); n != 0 {
			t.Errorf("Mailpit received %d messages", n)
		}
	})
}
