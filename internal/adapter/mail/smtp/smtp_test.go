// SPDX-License-Identifier: AGPL-3.0-only

package smtp_test

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"

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
	envelope  []string // MAIL FROM and RCPT TO arguments
	auth      []string // decoded AUTH PLAIN credentials
	plainText []string // commands received before STARTTLS
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
	m, err := mail.ReadMessage(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		s.t.Errorf("ReadMessage: %v", err)
		return
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil {
		s.t.Errorf("decode subject %q: %v", m.Header.Get("Subject"), err)
	}
	if m.Header.Get("Content-Transfer-Encoding") != "quoted-printable" || m.Header.Get("Content-Type") != "text/plain; charset=utf-8" ||
		m.Header.Get("Message-Id") == "" || m.Header.Get("Date") == "" {
		s.t.Errorf("headers = %v", m.Header)
	}
	for _, line := range strings.Split(string(data), "\r\n") {
		if len(line) > 998 {
			s.t.Errorf("line of %d bytes", len(line))
		}
	}
	body, err := io.ReadAll(quotedprintable.NewReader(m.Body))
	if err != nil {
		s.t.Errorf("decode body: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivered = append(s.delivered, kernel.MailMessage{
		To: m.Header.Get("To"), Subject: subject, Text: strings.ReplaceAll(string(body), "\r\n", "\n"),
	})
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
	if err := newMailer(t, s, tc).Send(t.Context(), kernel.MailMessage{To: "alice@example.com", Subject: "Hi", Text: "x"}); err != nil {
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
	err := newMailer(t, s, tc).Send(t.Context(), kernel.MailMessage{To: "alice@example.com", Subject: "Hi", Text: "x"})
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
	err := m.Send(t.Context(), kernel.MailMessage{To: "alice@example.com", Subject: "Hi", Text: "x"})
	var unknown x509.UnknownAuthorityError
	if !errors.As(err, &unknown) {
		t.Fatalf("Send with an untrusted certificate = %v, want x509.UnknownAuthorityError", err)
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
	err := newMailer(t, s, tc).Send(t.Context(), kernel.MailMessage{To: unknownRecipient, Subject: "Hi", Text: "x"})
	if !errors.Is(err, kernel.ErrRecipientRejected) || kernel.EnhancedStatus(err) != "5.1.1" {
		t.Fatalf("Send to an unknown recipient = %v (status %q), want ErrRecipientRejected with 5.1.1", err, kernel.EnhancedStatus(err))
	}
}
