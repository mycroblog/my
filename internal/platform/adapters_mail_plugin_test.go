// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	mailerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// mail = plugin builds the adapter, which reaches the plugin only when it sends, and a plugin that is down does not stop
// the start (ADR-0146 §15).
func TestNewMailerOfAPlugin(t *testing.T) {
	box := &fakeplugin.Mailbox{Answers: map[string]*mailerv1.SendResponse{
		"nobody@example.com": {Outcome: mailerv1.Outcome_OUTCOME_RECIPIENT_REJECTED, EnhancedStatus: "5.1.1"},
	}}
	h, rec := fakeplugin.JSON(fakeplugin.Options{Mailbox: box})
	cfg := &config.Config{
		Mail: "plugin", MailFrom: "noreply@social.example",
		Plugins: map[string]kind.Destination{kind.Mailer: {Address: fakeplugin.ServeUnix(t, h)}},
	}
	mailer, err := newMailer(t.Context(), cfg, nil, kernel.NopMeter{})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Calls() != 0 {
		t.Errorf("building the mailer made %d calls", rec.Calls())
	}
	m := kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hello", Text: "x"}
	if err := mailer.Send(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if got := box.Delivered(); len(got) != 1 || got[0].GetEnvelopeFrom() != cfg.MailFrom {
		t.Errorf("delivered = %v", got)
	}
	m.To = "nobody@example.com"
	if err := mailer.Send(t.Context(), m); !errors.Is(err, kernel.ErrRecipientRejected) || kernel.EnhancedStatus(err) != "5.1.1" {
		t.Errorf("Send = %v", err)
	}

	cfg.Plugins[kind.Mailer] = kind.Destination{Address: "unix:///nowhere/mailer.sock"}
	mailer, err = newMailer(t.Context(), cfg, nil, kernel.NopMeter{})
	if err != nil {
		t.Fatalf("a plugin that is down stopped the start: %v", err)
	}
	if err := mailer.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hello", Text: "x"}); err == nil || errors.Is(err, kernel.ErrMailRejected) {
		t.Errorf("Send to a plugin that is down = %v", err)
	}

	cfg.Plugins = nil
	if _, err := newMailer(t.Context(), cfg, nil, kernel.NopMeter{}); err == nil {
		t.Error("no address: no error")
	}
}
