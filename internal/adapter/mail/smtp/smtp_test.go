// SPDX-License-Identifier: AGPL-3.0-only

package smtp_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/mail/internal/mailtest"
	"github.com/mycroblog/my/internal/adapter/mail/smtp"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// The recipients that the server refuses: as unknown at RCPT TO, as spam after DATA, and for
// now at RCPT TO.
const (
	unknownRecipient  = "nobody@example.com"
	rejectedRecipient = "spammer@example.com"
	deferredRecipient = "greylisted@example.com"
)

// server is a minimal SMTP submission server: EHLO, STARTTLS, AUTH PLAIN, MAIL, RCPT, DATA, QUIT.
type server struct {
	t        *testing.T
	ln       net.Listener
	tls      *tls.Config
	starttls bool

	mu        sync.Mutex
	delivered []kernel.MailMessage
	headers   []mail.Header // the headers of the delivered messages
	envelope  []string      // MAIL FROM and RCPT TO arguments
	auth      []string      // decoded AUTH PLAIN credentials
	plainText []string      // commands received before STARTTLS
}

// newServer starts a server with the certificate of an httptest TLS server (valid for
// 127.0.0.1) and returns it with a client TLS configuration that trusts the certificate.
func newServer(t *testing.T, starttls bool) (*server, *tls.Config) {
	t.Helper()
	ts := httptest.NewTLSServer(http.NotFoundHandler())
	certs := ts.TLS.Certificates
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())
	ts.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{t: t, ln: ln, tls: &tls.Config{Certificates: certs, MinVersion: tls.VersionTLS12}, starttls: starttls}
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s, &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
}

func (s *server) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *server) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *server) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	tp := textproto.NewConn(conn)
	secure := false
	rcpt := ""
	_ = tp.PrintfLine("220 test ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		if !secure {
			s.mu.Lock()
			s.plainText = append(s.plainText, line)
			s.mu.Unlock()
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			if s.starttls && !secure {
				_ = tp.PrintfLine("250-test\r\n250-STARTTLS\r\n250 AUTH PLAIN")
			} else {
				_ = tp.PrintfLine("250-test\r\n250 AUTH PLAIN")
			}
		case "STARTTLS":
			_ = tp.PrintfLine("220 go ahead")
			tc := tls.Server(conn, s.tls)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, secure = tc, true
			tp = textproto.NewConn(tc)
		case "AUTH":
			_, cred, _ := strings.Cut(arg, " ")
			b, _ := base64.StdEncoding.DecodeString(cred)
			s.mu.Lock()
			s.auth = append(s.auth, string(b))
			s.mu.Unlock()
			_ = tp.PrintfLine("235 ok")
		case "MAIL", "RCPT":
			s.mu.Lock()
			s.envelope = append(s.envelope, arg)
			s.mu.Unlock()
			rcpt = strings.TrimSuffix(strings.TrimPrefix(arg, "TO:<"), ">")
			switch rcpt {
			case unknownRecipient:
				_ = tp.PrintfLine("550 5.1.1 <%s>: Recipient address rejected: User unknown", rcpt)
			case deferredRecipient:
				_ = tp.PrintfLine("451 4.3.0 <%s>: Temporary lookup failure", rcpt)
			default:
				_ = tp.PrintfLine("250 ok")
			}
		case "DATA":
			_ = tp.PrintfLine("354 go ahead")
			data, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			if rcpt == rejectedRecipient {
				_ = tp.PrintfLine("554 5.7.1 <%s>: Message rejected as spam", rcpt)
				continue
			}
			s.record(data)
			_ = tp.PrintfLine("250 queued")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
	}
}

func (s *server) record(data []byte) {
	delivered, header, err := mailtest.Parse(data)
	if err != nil {
		s.t.Errorf("delivered message: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivered = append(s.delivered, delivered)
	s.headers = append(s.headers, header)
}

func (s *server) messages() []kernel.MailMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]kernel.MailMessage(nil), s.delivered...)
}

func newMailer(t *testing.T, s *server, tc *tls.Config) *smtp.Mailer {
	t.Helper()
	m, err := smtp.New(smtp.Options{
		Host: "127.0.0.1", Port: s.port(), Username: "mailer", Password: "s3cret", From: "noreply@social.example", TLSConfig: tc,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMailerContract(t *testing.T) {
	testkit.MailerContract(t, func(t *testing.T) testkit.MailerHarness {
		s, tc := newServer(t, true)
		return testkit.MailerHarness{
			Mailer: newMailer(t, s, tc), Delivered: s.messages,
			UnknownRecipient: unknownRecipient, RejectedRecipient: rejectedRecipient, DeferredRecipient: deferredRecipient,
		}
	})
}

func TestAuthenticatesOnlyAfterSTARTTLS(t *testing.T) {
	s, tc := newServer(t, true)
	if err := newMailer(t, s, tc).Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.auth) != 1 || s.auth[0] != "\x00mailer\x00s3cret" {
		t.Errorf("AUTH PLAIN = %q", s.auth)
	}
	if strings.Join(s.envelope, " ") != "FROM:<noreply@social.example> TO:<alice@example.com>" {
		t.Errorf("envelope = %q", s.envelope)
	}
	for _, line := range s.plainText {
		if v := strings.ToUpper(line); !strings.HasPrefix(v, "EHLO") && v != "STARTTLS" {
			t.Errorf("sent %q before STARTTLS", line)
		}
	}
}

func TestRefusesServersWithoutSTARTTLS(t *testing.T) {
	s, tc := newServer(t, false)
	err := newMailer(t, s, tc).Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("Send to a server without STARTTLS = %v", err)
	}
	s.mu.Lock()
	auth := len(s.auth)
	s.mu.Unlock()
	if len(s.messages()) != 0 || auth != 0 {
		t.Error("the mailer sent credentials or a message in plain text")
	}
}

func TestRefusesAnUntrustedCertificate(t *testing.T) {
	s, _ := newServer(t, true)
	m := newMailer(t, s, &tls.Config{ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12})
	err := m.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "x"})
	var unknown x509.UnknownAuthorityError
	if !errors.As(err, &unknown) {
		t.Fatalf("Send with an untrusted certificate = %v, want x509.UnknownAuthorityError", err)
	}
}

// The envelope names the server as the sender, marks the message as automatic, identifies it
// by a UUIDv7 at the sender's domain, adds Reply-To only with a contact address, and carries
// the allowed extra headers.
func TestEnvelopeHeaders(t *testing.T) {
	s, tc := newServer(t, true)
	m := newMailer(t, s, tc)
	name := strings.TrimSpace(strings.Repeat("例のソーシャル ", 12))
	if err := m.Send(t.Context(), kernel.MailMessage{
		Category: kernel.MailNotification, To: "alice@example.com", FromName: name, ReplyTo: "admin@social.example",
		Subject: "Hi", Text: "x", Headers: map[string]string{
			kernel.HeaderListUnsubscribe:     "<https://social.example/mail/unsubscribe?token=x>",
			kernel.HeaderListUnsubscribePost: "List-Unsubscribe=One-Click",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: "bob@example.com", Subject: "Hi", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	headers := s.headers
	s.mu.Unlock()
	got := s.messages()
	if len(headers) != 2 || len(got) != 2 {
		t.Fatalf("delivered %d messages, want 2", len(got))
	}
	first, second := headers[0], headers[1]
	from, err := mail.ParseAddress(first.Get("From"))
	if err != nil || from.Name != name || from.Address != "noreply@social.example" {
		t.Errorf("From = %q (%+v, %v), want the server's name and the sender", first.Get("From"), from, err)
	}
	if first.Get("Reply-To") != "admin@social.example" {
		t.Errorf("Reply-To = %q", first.Get("Reply-To"))
	}
	if _, ok := second["Reply-To"]; ok {
		t.Errorf("Reply-To without a contact address = %q", second.Get("Reply-To"))
	}
	if second.Get("From") != "<noreply@social.example>" {
		t.Errorf("From without a name = %q", second.Get("From"))
	}
	for i, h := range headers {
		if h.Get("Auto-Submitted") != "auto-generated" {
			t.Errorf("message %d: Auto-Submitted = %q", i, h.Get("Auto-Submitted"))
		}
		id, ok := strings.CutSuffix(strings.TrimPrefix(h.Get("Message-Id"), "<"), "@social.example>")
		if !ok || !kernel.ValidID(id) {
			t.Errorf("message %d: Message-ID = %q, want <UUIDv7@social.example>", i, h.Get("Message-Id"))
		}
	}
	if headers[0].Get("Message-Id") == headers[1].Get("Message-Id") {
		t.Error("two messages share a Message-ID")
	}
	if first.Get("List-Unsubscribe") != "<https://social.example/mail/unsubscribe?token=x>" ||
		first.Get("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" {
		t.Errorf("List-Unsubscribe headers = %v", first)
	}
}

func TestSendsTextAndHTMLAsAlternatives(t *testing.T) {
	s, tc := newServer(t, true)
	m := newMailer(t, s, tc)
	html := "<!doctype html>\n<html lang=\"ja\" dir=\"ltr\"><body><p>ようこそ。" + strings.Repeat("長い行。", 200) + "</p></body></html>\n"
	if err := m.Send(t.Context(), kernel.MailMessage{
		Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hi", Text: "ようこそ。\n", HTML: html,
	}); err != nil {
		t.Fatal(err)
	}
	got := s.messages()
	if len(got) != 1 {
		t.Fatalf("delivered %d messages, want 1", len(got))
	}
	if got[0].Text != "ようこそ。\n" || got[0].HTML != html {
		t.Errorf("delivered text %q and HTML %q", got[0].Text, got[0].HTML)
	}
	s.mu.Lock()
	h := s.headers[0]
	s.mu.Unlock()
	if mediaType, _, _ := mime.ParseMediaType(h.Get("Content-Type")); mediaType != "multipart/alternative" || h.Get("Content-Transfer-Encoding") != "" {
		t.Errorf("Content-Type = %q, Content-Transfer-Encoding = %q", h.Get("Content-Type"), h.Get("Content-Transfer-Encoding"))
	}
}

func TestNewValidatesOptions(t *testing.T) {
	for _, o := range []smtp.Options{
		{Host: "", Port: 587, From: "a@example.com"},
		{Host: "smtp.example", Port: 0, From: "a@example.com"},
		{Host: "smtp.example", Port: 587, From: "Alice <a@example.com>"},
	} {
		if _, err := smtp.New(o); err == nil {
			t.Errorf("New(%+v) succeeded", o)
		}
	}
}

func TestReportsTheEnhancedStatusOfARejection(t *testing.T) {
	s, tc := newServer(t, true)
	err := newMailer(t, s, tc).Send(t.Context(), kernel.MailMessage{Category: kernel.MailTransactional, To: unknownRecipient, Subject: "Hi", Text: "x"})
	if !errors.Is(err, kernel.ErrRecipientRejected) || kernel.EnhancedStatus(err) != "5.1.1" {
		t.Fatalf("Send to an unknown recipient = %v (status %q), want ErrRecipientRejected with 5.1.1", err, kernel.EnhancedStatus(err))
	}
}
