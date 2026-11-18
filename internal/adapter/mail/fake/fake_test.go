// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/mail/fake"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

// logBuffer collects the logger's JSON lines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// messages decodes the logged messages.
func (b *logBuffer) messages(t *testing.T) []kernel.MailMessage {
	t.Helper()
	var out []kernel.MailMessage
	dec := json.NewDecoder(strings.NewReader(b.String()))
	for dec.More() {
		var line struct {
			To       string `json:"to"`
			FromName string `json:"from_name"`
			ReplyTo  string `json:"reply_to"`
			Subject  string `json:"subject"`
			Text     string `json:"text"`
		}
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		out = append(out, kernel.MailMessage{To: line.To, FromName: line.FromName, ReplyTo: line.ReplyTo, Subject: line.Subject, Text: line.Text})
	}
	return out
}

func TestMailerContract(t *testing.T) {
	kerneltest.MailerContract(t, func(t *testing.T) kerneltest.MailerHarness {
		var out logBuffer
		m := fake.New(slog.New(slog.NewJSONHandler(&out, nil)))
		return kerneltest.MailerHarness{Mailer: m, Delivered: func() []kernel.MailMessage { return out.messages(t) }}
	})
}

// TestSendLogsTheWholeMessage keeps the whole text, links and tokens included, in one log line that
// a test reads as the person would open the mail.
func TestSendLogsTheWholeMessage(t *testing.T) {
	var out logBuffer
	m := fake.New(slog.New(slog.NewJSONHandler(&out, nil)))
	text := "Confirm your address:\nhttps://example.test/auth/verify_email?token=secret-token\n\nSee you."
	err := m.Send(t.Context(), kernel.MailMessage{
		To: "alice@example.test", FromName: "Example Name", ReplyTo: "admin@example.test",
		Subject: "Example subject", Text: text, Category: kernel.MailTransactional,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimSpace(out.String()), "\n"); n != 0 {
		t.Fatalf("the message took %d lines, want 1:\n%s", n+1, out.String())
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(out.String()), &line); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"msg": fake.LogMessage, "category": string(kernel.MailTransactional), "to": "alice@example.test",
		"from_name": "Example Name", "reply_to": "admin@example.test", "subject": "Example subject", "text": text,
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("log attribute %s = %v, want %v", k, line[k], v)
		}
	}
	if fake.LogMessage != "mail not sent (mail = fake)" {
		t.Errorf("LogMessage = %q", fake.LogMessage)
	}
}

// TestSendRefusesAnInvalidMessageWithoutLogging checks that a message that is refused leaves no line.
func TestSendRefusesAnInvalidMessageWithoutLogging(t *testing.T) {
	var out logBuffer
	m := fake.New(slog.New(slog.NewJSONHandler(&out, nil)))
	err := m.Send(t.Context(), kernel.MailMessage{To: "alice@example.test", Subject: "Line\r\nBcc: eve@example.test", Text: "x", Category: kernel.MailTransactional})
	if !errors.Is(err, kernel.ErrInvalidMail) {
		t.Errorf("Send(subject with a line break) = %v, want ErrInvalidMail", err)
	}
	if out.String() != "" {
		t.Errorf("a refused message was logged: %s", out.String())
	}
}
