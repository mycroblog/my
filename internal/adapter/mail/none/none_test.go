// SPDX-License-Identifier: AGPL-3.0-only

package none_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/mail/none"
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

// messages decodes the logged messages.
func (b *logBuffer) messages(t *testing.T) []kernel.MailMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []kernel.MailMessage
	dec := json.NewDecoder(bytes.NewReader(b.buf.Bytes()))
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
		m := none.New(slog.New(slog.NewJSONHandler(&out, nil)), none.Options{LogMessages: true})
		return kerneltest.MailerHarness{Mailer: m, Delivered: func() []kernel.MailMessage { return out.messages(t) }}
	})
}

// TestSendLogsOnlyTheCategoryByDefault keeps addresses and the tokens in links out of a production log.
func TestSendLogsOnlyTheCategoryByDefault(t *testing.T) {
	var out logBuffer
	m := none.New(slog.New(slog.NewJSONHandler(&out, nil)), none.Options{})
	err := m.Send(t.Context(), kernel.MailMessage{
		To: "alice@example.test", FromName: "Example Name", ReplyTo: "admin@example.test",
		Subject: "Example subject", Text: "https://example.test/auth/verify_email?token=secret-token", Category: kernel.MailTransactional,
	})
	if err != nil {
		t.Fatal(err)
	}
	line := out.buf.String()
	for _, leaked := range []string{"alice@example.test", "Example Name", "admin@example.test", "Example subject", "secret-token"} {
		if strings.Contains(line, leaked) {
			t.Errorf("the log line holds %q: %s", leaked, line)
		}
	}
	if !strings.Contains(line, "mail not sent (mail = none)") || !strings.Contains(line, `"category":"`+string(kernel.MailTransactional)+`"`) {
		t.Errorf("the log line lacks the message or the category: %s", line)
	}
}
