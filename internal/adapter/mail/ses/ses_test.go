// SPDX-License-Identifier: AGPL-3.0-only

package ses_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/mycroblog/my/internal/adapter/mail/internal/mailtest"
	"github.com/mycroblog/my/internal/adapter/mail/ses"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// The recipients for which the stand-in of SES answers with an error, by its code.
var refusals = map[string]struct {
	status int
	code   string
}{
	"spammer@example.com":    {http.StatusBadRequest, "MessageRejected"},
	"throttled@example.com":  {http.StatusTooManyRequests, "TooManyRequestsException"},
	"paused@example.com":     {http.StatusBadRequest, "SendingPausedException"},
	"unverified@example.com": {http.StatusBadRequest, "MailFromDomainNotVerifiedException"},
	"suspended@example.com":  {http.StatusBadRequest, "AccountSuspendedException"},
	"broken@example.com":     {http.StatusInternalServerError, "InternalFailure"},
}

// request is the body of SendEmail (POST /v2/email/outbound-emails) that the server reads.
type request struct {
	FromEmailAddress     string
	ConfigurationSetName string
	Destination          struct{ ToAddresses []string }
	Content              struct {
		Raw    *struct{ Data []byte }
		Simple json.RawMessage
	}
}

// server stands in for the SES v2 API: it accepts SendEmail with raw content and records the
// messages, or answers with the error of refusals for their recipient.
type server struct {
	t *testing.T

	mu        sync.Mutex
	requests  []request
	delivered []kernel.MailMessage
	headers   []mail.Header
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v2/email/outbound-emails" {
		http.NotFound(w, r)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test/") ||
		!strings.Contains(r.Header.Get("Authorization"), "/eu-west-1/ses/aws4_request") {
		s.t.Errorf("Authorization = %q, want a SigV4 signature for ses in eu-west-1", r.Header.Get("Authorization"))
	}
	var in request
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Content.Raw == nil || in.Content.Simple != nil {
		s.t.Errorf("SendEmail body: %+v, %v", in, err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, in)
	s.mu.Unlock()
	if len(in.Destination.ToAddresses) == 1 {
		if refusal, ok := refusals[in.Destination.ToAddresses[0]]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Amzn-ErrorType", refusal.code)
			w.WriteHeader(refusal.status)
			_, _ = w.Write([]byte(`{"message":"Refused the message to ` + in.Destination.ToAddresses[0] + `"}`))
			return
		}
	}
	msg, header, err := mailtest.Parse(in.Content.Raw.Data)
	if err != nil {
		s.t.Errorf("raw content: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !slices.Equal(in.Destination.ToAddresses, []string{msg.To}) {
		s.t.Errorf("destination %q, want the To of the message %q", in.Destination.ToAddresses, msg.To)
	}
	s.mu.Lock()
	s.delivered = append(s.delivered, msg)
	s.headers = append(s.headers, header)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"MessageId":"0100018f-test"}`))
}

func (s *server) messages() []kernel.MailMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.delivered)
}

func newMailer(t *testing.T, o ses.Options) (*ses.Mailer, *server) {
	t.Helper()
	s := &server{t: t}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	if o.From == "" {
		o.From = "noreply@social.example"
	}
	m, err := ses.NewFromConfig(aws.Config{
		Region:       "eu-west-1",
		BaseEndpoint: aws.String(srv.URL),
		HTTPClient:   srv.Client(),
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test", Source: "test"}, nil
		}),
		RetryMaxAttempts: 1,
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	return m, s
}

func TestMailerContract(t *testing.T) {
	testkit.MailerContract(t, func(t *testing.T) testkit.MailerHarness {
		m, s := newMailer(t, ses.Options{})
		return testkit.MailerHarness{
			Mailer: m, Delivered: s.messages,
			// SES accepts every address and reports the ones that do not exist later as bounces.
			RejectedRecipient: "spammer@example.com", DeferredRecipient: "throttled@example.com",
		}
	})
}

func TestClassifiesTheErrorsOfSES(t *testing.T) {
	m, s := newMailer(t, ses.Options{})
	for to, refusal := range refusals {
		err := m.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: to, Subject: "Hi", Text: "x"})
		rejected := refusal.code == "MessageRejected"
		if err == nil || errors.Is(err, kernel.ErrMailRejected) != rejected || errors.Is(err, kernel.ErrRecipientRejected) ||
			!strings.Contains(err.Error(), refusal.code) {
			t.Errorf("Send(%s) = %v; want an error naming %s, ErrMailRejected %t", refusal.code, err, refusal.code, rejected)
		}
		if local, _, _ := strings.Cut(to, "@"); err != nil && strings.Contains(err.Error(), local) {
			t.Errorf("Send(%s) = %v, which names the recipient", refusal.code, err)
		}
	}
	if got := s.messages(); len(got) != 0 {
		t.Errorf("delivered %d refused messages", len(got))
	}
}

func TestATransportFailureIsTemporary(t *testing.T) {
	// A server that closes every connection without an answer.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	m, err := ses.NewFromConfig(aws.Config{
		Region: "eu-west-1", BaseEndpoint: aws.String(srv.URL), HTTPClient: srv.Client(), RetryMaxAttempts: 1,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test", Source: "test"}, nil
		}),
	}, ses.Options{From: "noreply@social.example"})
	if err != nil {
		t.Fatal(err)
	}
	err = m.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"})
	if err == nil || errors.Is(err, kernel.ErrMailRejected) {
		t.Errorf("Send to an unreachable endpoint = %v, want a temporary failure", err)
	}
}

// The request names the sender, the one recipient in ASCII, and the configuration set, and
// carries the message as raw content, multipart when it has an HTML body.
func TestSendsRawContent(t *testing.T) {
	m, s := newMailer(t, ses.Options{From: "noreply@例え.jp", ConfigurationSet: "my-events"})
	html := "<!doctype html>\n<html lang=\"ja\" dir=\"ltr\"><body><p>ようこそ。</p></body></html>\n"
	if err := m.Send(t.Context(), kernel.MailMessage{
		Category: kernel.MailNotification, To: "alice@bücher.example", FromName: "例のソーシャル", Subject: "ようこそ",
		Text: "ようこそ。\n", HTML: html, Headers: map[string]string{
			kernel.HeaderListUnsubscribe:     "<https://social.example/mail/unsubscribe?token=x>",
			kernel.HeaderListUnsubscribePost: "List-Unsubscribe=One-Click",
		},
	}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) != 1 || len(s.delivered) != 1 {
		t.Fatalf("%d requests, %d messages", len(s.requests), len(s.delivered))
	}
	in, got, h := s.requests[0], s.delivered[0], s.headers[0]
	if in.FromEmailAddress != "noreply@xn--r8jz45g.jp" || in.ConfigurationSetName != "my-events" ||
		!slices.Equal(in.Destination.ToAddresses, []string{"alice@xn--bcher-kva.example"}) {
		t.Errorf("request = %+v", in)
	}
	from, err := mail.ParseAddress(h.Get("From"))
	if err != nil || from.Name != "例のソーシャル" || from.Address != "noreply@xn--r8jz45g.jp" {
		t.Errorf("From = %q (%+v, %v)", h.Get("From"), from, err)
	}
	if got.Subject != "ようこそ" || got.Text != "ようこそ。\n" || got.HTML != html {
		t.Errorf("delivered %+v", got)
	}
	if h.Get("Auto-Submitted") != "auto-generated" || h.Get("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" {
		t.Errorf("headers = %v", h)
	}
}

func TestSendsWithoutAConfigurationSet(t *testing.T) {
	m, s := newMailer(t, ses.Options{})
	if err := m.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) != 1 || s.requests[0].ConfigurationSetName != "" {
		t.Errorf("requests = %+v", s.requests)
	}
}

func TestRefusesALocalPartThatIsNotASCII(t *testing.T) {
	m, s := newMailer(t, ses.Options{})
	err := m.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "アリス@example.com", Subject: "Hi", Text: "x"})
	if !errors.Is(err, kernel.ErrInvalidMail) {
		t.Errorf("Send to a local part that is not ASCII = %v, want ErrInvalidMail", err)
	}
	if len(s.messages()) != 0 {
		t.Error("sent the message")
	}
}

func TestNewValidatesOptions(t *testing.T) {
	for _, from := range []string{"", "Alice <a@example.com>", "アリス@example.com"} {
		if _, err := ses.New(nil, ses.Options{From: from}); err == nil {
			t.Errorf("New(From %q) succeeded", from)
		}
	}
	if _, err := ses.NewFromConfig(aws.Config{}, ses.Options{From: "a@example.com"}); err == nil {
		t.Error("NewFromConfig without a region succeeded")
	}
}
