// SPDX-License-Identifier: AGPL-3.0-only

package message_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/mail/internal/mailtest"
	"github.com/mycroblog/my/internal/adapter/mail/internal/message"
	"github.com/mycroblog/my/internal/app/kernel"
)

func TestASCIIAddress(t *testing.T) {
	for in, want := range map[string]string{
		"alice@example.com":      "alice@example.com",
		"alice@例え.jp":            "alice@xn--r8jz45g.jp",
		"alice@BÜCHER.example":   "alice@xn--bcher-kva.example",
		"noreply@social.example": "noreply@social.example",
	} {
		if got, err := message.ASCIIAddress(in); err != nil || got != want {
			t.Errorf("ASCIIAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"アリス@example.com", "alice@exa mple.例え", "example.com"} {
		if got, err := message.ASCIIAddress(in); !errors.Is(err, kernel.ErrInvalidMail) {
			t.Errorf("ASCIIAddress(%q) = %q, %v; want ErrInvalidMail", in, got, err)
		}
	}
}

// The headers carry the addresses in ASCII and the name and the subject as encoded words, so
// that the whole message is 7-bit apart from its quoted-printable bodies.
func TestComposeWritesASCIIHeaders(t *testing.T) {
	data, to, err := message.Compose("noreply@例え.jp", kernel.MailMessage{
		Category: kernel.MailTransactional, To: "alice@bücher.example", ReplyTo: "admin@例え.jp",
		FromName: "例のソーシャル", Subject: "ようこそ", Text: "こんにちは。\n",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if to != "alice@xn--bcher-kva.example" {
		t.Errorf("recipient = %q", to)
	}
	for i, c := range data {
		if c >= 0x80 {
			t.Fatalf("byte %d of the message is not ASCII: %q", i, data)
		}
	}
	msg, header, err := mailtest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if msg.To != to || msg.ReplyTo != "admin@xn--r8jz45g.jp" || msg.FromName != "例のソーシャル" || msg.Subject != "ようこそ" || msg.Text != "こんにちは。\n" {
		t.Errorf("composed %+v", msg)
	}
	if !strings.HasSuffix(header.Get("Message-Id"), "@xn--r8jz45g.jp>") {
		t.Errorf("Message-ID = %q", header.Get("Message-Id"))
	}
}
