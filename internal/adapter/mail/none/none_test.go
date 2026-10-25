// SPDX-License-Identifier: AGPL-3.0-only

package none_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/mail/none"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
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
	testkit.MailerContract(t, func(t *testing.T) testkit.MailerHarness {
		var out logBuffer
		m := none.New(slog.New(slog.NewJSONHandler(&out, nil)))
		return testkit.MailerHarness{Mailer: m, Delivered: func() []kernel.MailMessage { return out.messages(t) }}
	})
}
