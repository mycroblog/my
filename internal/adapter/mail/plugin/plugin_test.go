// SPDX-License-Identifier: AGPL-3.0-only

package plugin_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect/v2"

	mailplugin "github.com/mycroblog/my/internal/adapter/mail/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	mailerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

const sender = "noreply@social.example"

func newMailer(t *testing.T, d kind.Destination) *mailplugin.Mailer {
	t.Helper()
	k, ok := kind.ByName(kind.Mailer)
	if !ok {
		t.Fatal("no mailer kind")
	}
	conn, err := client.New(client.Options{Kind: k, Destination: d})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	m, err := mailplugin.New(conn, sender)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func message(to string) kernel.MailMessage {
	return kernel.MailMessage{Category: kernel.MailTransactional, To: to, Subject: "Hello", Text: "The secret link.\n"}
}

// decode turns what a plugin received back into the message of the port.
func decode(req *mailerv1.SendRequest) kernel.MailMessage {
	return kernel.MailMessage{
		To: req.GetTo(), FromName: req.GetFromName(), ReplyTo: req.GetReplyTo(), Subject: req.GetSubject(),
		Text: req.GetText(), HTML: req.GetHtml(), Headers: req.GetHeaders(),
	}
}

// The adapter passes the contract of the port against both fakes, over each protocol they speak.
func TestContract(t *testing.T) {
	t.Parallel()
	const unknown, rejected, deferred = "nobody@example.com", "blocked@example.com", "later@example.com"
	for _, tc := range []struct {
		name     string
		protocol string
		fake     func(fakeplugin.Options) (http.Handler, *fakeplugin.Recorder)
	}{
		{"http_and_json", kind.ProtocolJSON, fakeplugin.JSON},
		{"connect_json", kind.ProtocolJSON, fakeplugin.Connect},
		{"connect_proto", kind.ProtocolProto, fakeplugin.Connect},
		{"connect_grpc", kind.ProtocolGRPC, fakeplugin.Connect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			kerneltest.MailerContract(t, func(t *testing.T) kerneltest.MailerHarness {
				t.Helper()
				box := &fakeplugin.Mailbox{Answers: map[string]*mailerv1.SendResponse{
					unknown:  {Outcome: mailerv1.Outcome_OUTCOME_RECIPIENT_REJECTED, EnhancedStatus: "5.1.1"},
					rejected: {Outcome: mailerv1.Outcome_OUTCOME_REJECTED, EnhancedStatus: "5.7.1"},
					deferred: {Outcome: mailerv1.Outcome_OUTCOME_TEMPORARY, EnhancedStatus: "4.3.0"},
				}}
				h, _ := tc.fake(fakeplugin.Options{Mailbox: box})
				return kerneltest.MailerHarness{
					Mailer: newMailer(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Protocol: tc.protocol}),
					Delivered: func() []kernel.MailMessage {
						var out []kernel.MailMessage
						for _, req := range box.Delivered() {
							out = append(out, decode(req))
						}
						return out
					},
					UnknownRecipient: unknown, RejectedRecipient: rejected, DeferredRecipient: deferred,
				}
			})
		})
	}
}

// Every part of the message arrives, with the sender of the settings, in both forms of a plugin.
func TestSendHandsOverEveryPart(t *testing.T) {
	t.Parallel()
	for name, fake := range map[string]func(fakeplugin.Options) (http.Handler, *fakeplugin.Recorder){"json": fakeplugin.JSON, "connect": fakeplugin.Connect} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			box := new(fakeplugin.Mailbox)
			h, _ := fake(fakeplugin.Options{Mailbox: box})
			m := newMailer(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
			msg := kernel.MailMessage{
				Category: kernel.MailNotification, To: "alice@example.com", FromName: "例のサーバー", ReplyTo: "admin@social.example",
				Subject: "件名", Text: "text\n", HTML: "<p>html</p>",
				Headers: map[string]string{kernel.HeaderListUnsubscribe: "<https://social.example/u>", kernel.HeaderListUnsubscribePost: "List-Unsubscribe=One-Click"},
			}
			if err := m.Send(t.Context(), msg); err != nil {
				t.Fatal(err)
			}
			got := box.Delivered()
			if len(got) != 1 {
				t.Fatalf("delivered %d", len(got))
			}
			req := got[0]
			if req.GetCategory() != mailerv1.Category_CATEGORY_NOTIFICATION || req.GetEnvelopeFrom() != sender || req.GetHtml() != msg.HTML ||
				req.GetTo() != msg.To || req.GetFromName() != msg.FromName || req.GetReplyTo() != msg.ReplyTo || req.GetSubject() != msg.Subject || req.GetText() != msg.Text ||
				req.GetHeaders()[kernel.HeaderListUnsubscribe] != msg.Headers[kernel.HeaderListUnsubscribe] ||
				req.GetHeaders()[kernel.HeaderListUnsubscribePost] != msg.Headers[kernel.HeaderListUnsubscribePost] {
				t.Errorf("request = %v", req)
			}
		})
	}
}

func TestNewRefusesASenderThatIsNotABareAddress(t *testing.T) {
	t.Parallel()
	k, _ := kind.ByName(kind.Mailer)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: "unix:///nowhere"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, from := range []string{"", "not an address", "Name <noreply@social.example>"} {
		if _, err := mailplugin.New(conn, from); err == nil {
			t.Errorf("New(%q) succeeded", from)
		}
	}
}

// ValidateMail runs before the call: an invalid message never reaches the plugin.
func TestSendValidatesBeforeTheCall(t *testing.T) {
	t.Parallel()
	h, rec := fakeplugin.JSON(fakeplugin.Options{Mailbox: new(fakeplugin.Mailbox)})
	m := newMailer(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
	bad := message("alice@example.com")
	bad.Subject = "Hello\r\nBcc: mallory@example.com"
	if err := m.Send(t.Context(), bad); !errors.Is(err, kernel.ErrInvalidMail) {
		t.Errorf("Send = %v", err)
	}
	if rec.Calls() != 0 {
		t.Errorf("%d calls reached the plugin", rec.Calls())
	}
}

// raw serves one fixed answer of a plugin that may be wrong in any way.
func raw(t *testing.T, status int, body string) *mailplugin.Mailer {
	t.Helper()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	return newMailer(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
}

// Each answer maps to the error of the port, and none says anything of the recipient, the body, or the plugin's text.
func TestAnswers(t *testing.T) {
	t.Parallel()
	const to = "alice@example.com"
	leak := func(name string) string {
		return `,"enhancedStatus":"550 ` + name + ` alice@example.com The secret link."`
	}
	for _, tc := range []struct {
		name                string
		status              int
		body                string
		invalid             bool
		recipient, rejected bool
		enhanced            string
	}{
		{name: "invalid", status: 200, body: `{"outcome":"OUTCOME_INVALID"}`, invalid: true},
		{name: "recipient", status: 200, body: `{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}`, recipient: true, rejected: true, enhanced: "5.1.1"},
		{name: "recipient without a code", status: 200, body: `{"outcome":"OUTCOME_RECIPIENT_REJECTED"}`, recipient: true, rejected: true},
		{name: "rejected", status: 200, body: `{"outcome":"OUTCOME_REJECTED","enhancedStatus":"5.7.1"}`, rejected: true, enhanced: "5.7.1"},
		{name: "temporary", status: 200, body: `{"outcome":"OUTCOME_TEMPORARY","enhancedStatus":"4.3.0"}`, enhanced: "4.3.0"},
		{name: "a rejection with text for a code", status: 200, body: `{"outcome":"OUTCOME_REJECTED"` + leak("rejected") + `}`, rejected: true},
		{name: "a recipient with text for a code", status: 200, body: `{"outcome":"OUTCOME_RECIPIENT_REJECTED"` + leak("recipient") + `}`, recipient: true, rejected: true},
		{name: "temporary with text for a code", status: 200, body: `{"outcome":"OUTCOME_TEMPORARY"` + leak("temporary") + `}`},
		{name: "a rejection with a code of the wrong class", status: 200, body: `{"outcome":"OUTCOME_REJECTED","enhancedStatus":"2.0.0"}`, rejected: true},
		{name: "temporary with a code of the wrong class", status: 200, body: `{"outcome":"OUTCOME_TEMPORARY","enhancedStatus":"5.1.1"}`},
		{name: "empty", status: 200, body: `{}`},
		{name: "an unknown name", status: 200, body: `{"outcome":"OUTCOME_LATER"}`},
		{name: "an unknown number", status: 200, body: `{"outcome":99}`},
		{name: "not JSON", status: 200, body: `alice@example.com The secret link.`},
		{name: "a connect error", status: 503, body: `{"code":"unavailable","message":"alice@example.com The secret link."}`},
		{name: "an HTTP error", status: 500, body: `alice@example.com The secret link.`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := raw(t, tc.status, tc.body).Send(t.Context(), message(to))
			if err == nil {
				t.Fatal("Send succeeded")
			}
			if errors.Is(err, kernel.ErrInvalidMail) != tc.invalid || errors.Is(err, kernel.ErrRecipientRejected) != tc.recipient || errors.Is(err, kernel.ErrMailRejected) != tc.rejected {
				t.Errorf("Send = %v", err)
			}
			if got := kernel.EnhancedStatus(err); got != tc.enhanced {
				t.Errorf("EnhancedStatus = %q, want %q", got, tc.enhanced)
			}
			for _, secret := range []string{"alice", "secret", "550"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("Send = %q, which holds %q", err, secret)
				}
			}
		})
	}
}

func TestSentIgnoresTheStatus(t *testing.T) {
	t.Parallel()
	if err := raw(t, 200, `{"outcome":"OUTCOME_SENT","enhancedStatus":"5.1.1"}`).Send(t.Context(), message("alice@example.com")); err != nil {
		t.Errorf("Send = %v", err)
	}
}

// A message over the kind's limit is a temporary failure and never reaches the plugin.
func TestAMessageOverTheLimit(t *testing.T) {
	t.Parallel()
	k, _ := kind.ByName(kind.Mailer)
	box := new(fakeplugin.Mailbox)
	h, _ := fakeplugin.JSON(fakeplugin.Options{Mailbox: box})
	m := newMailer(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
	big := message("alice@example.com")
	big.Text = strings.Repeat("x", k.MaxSend+1)
	err := m.Send(t.Context(), big)
	if err == nil || connect.CodeOf(err) != connect.CodeResourceExhausted || errors.Is(err, kernel.ErrMailRejected) {
		t.Errorf("Send = %v", err)
	}
	if len(box.Requests()) != 0 {
		t.Errorf("%d requests reached the plugin", len(box.Requests()))
	}
}

// A plugin that does not answer stops the call at the deadline, as a temporary failure.
func TestAPluginThatDoesNotAnswer(t *testing.T) {
	t.Parallel()
	h, _ := fakeplugin.JSON(fakeplugin.Options{Mailbox: new(fakeplugin.Mailbox), Block: true})
	m := newMailer(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Timeout: 50 * time.Millisecond})
	err := m.Send(context.Background(), message("alice@example.com"))
	if err == nil || connect.CodeOf(err) != connect.CodeDeadlineExceeded || errors.Is(err, kernel.ErrMailRejected) {
		t.Errorf("Send = %v", err)
	}
}

// The refusal of plain TCP holds on loopback too (ADR-0146 §6).
func TestPlainTCPIsRefused(t *testing.T) {
	t.Parallel()
	k, _ := kind.ByName(kind.Mailer)
	for _, address := range []string{"http://127.0.0.1:8080", "http://[::1]:8080", "http://localhost:8080", "http://mail.internal"} {
		if _, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: address}}); err == nil || !strings.Contains(err.Error(), "must not be http://") {
			t.Errorf("%s: err = %v", address, err)
		}
	}
}
