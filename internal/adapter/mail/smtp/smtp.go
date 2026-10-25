// SPDX-License-Identifier: AGPL-3.0-only

// Package smtp implements the mailer port over SMTP with the standard library's net/smtp
// (mail = smtp). It always upgrades the connection with STARTTLS and verifies the server's
// certificate before it authenticates or sends anything, and refuses servers that do not offer
// STARTTLS. Messages are plain text in UTF-8, quoted-printable encoded.
//
// A reply of 5xx to RCPT TO with an enhanced status code 5.1.x (RFC 3463: the address does
// not exist) fails with kernel.ErrRecipientRejected, any other 5xx to RCPT TO or DATA with
// kernel.ErrMailRejected, and everything else, such as a 4xx reply, a refused AUTH, or a
// connection that fails, as a temporary failure. The errors name the SMTP command, the reply
// code, and the enhanced status code, never the server's reply text, which often repeats the
// recipient's address.
package smtp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// DefaultTimeout bounds one Send: connecting, the TLS handshake, and the whole SMTP dialogue.
const DefaultTimeout = 30 * time.Second

// Options configure a Mailer.
type Options struct {
	// Host and Port are the SMTP server (submission, usually port 587).
	Host string
	Port int
	// Username and Password authenticate with AUTH PLAIN after STARTTLS; an empty Username
	// sends without authenticating.
	Username string
	Password string
	// From is the sender address.
	From string
	// HelloName is the name sent with EHLO; empty means "localhost".
	HelloName string
	// TLSConfig verifies the server after STARTTLS; nil verifies Host against the system roots.
	TLSConfig *tls.Config
	// Timeout bounds one Send; zero means DefaultTimeout.
	Timeout time.Duration
}

// Mailer sends messages over SMTP. It opens one connection per message and is safe for
// concurrent use.
type Mailer struct {
	o Options
}

var _ kernel.Mailer = (*Mailer)(nil)

// New returns a Mailer on o. It fails when From is not a bare address or Host is empty.
func New(o Options) (*Mailer, error) {
	if o.Host == "" || o.Port <= 0 || o.Port > 65535 {
		return nil, errors.New("smtp: a host and a port are required")
	}
	if a, err := mail.ParseAddress(o.From); err != nil || a.Name != "" || a.Address != o.From {
		return nil, errors.New("smtp: the sender must be a bare address")
	}
	if o.HelloName == "" {
		o.HelloName = "localhost"
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.TLSConfig == nil {
		o.TLSConfig = &tls.Config{ServerName: o.Host, MinVersion: tls.VersionTLS12}
	}
	return &Mailer{o: o}, nil
}

// Send implements kernel.Mailer.
func (m *Mailer) Send(ctx context.Context, msg kernel.MailMessage) error {
	if err := kernel.ValidateMail(msg); err != nil {
		return err
	}
	body, err := m.compose(msg, time.Now())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, m.o.Timeout)
	defer cancel()
	addr := net.JoinHostPort(m.o.Host, strconv.Itoa(m.o.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp: connect to %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, m.o.Host)
	if err != nil {
		return fmt.Errorf("smtp: greeting from %s: %w", addr, err)
	}
	defer func() { _ = c.Close() }()
	if err := m.dialogue(c, msg.To, body); err != nil {
		return fmt.Errorf("smtp: %s: %w", addr, err)
	}
	return nil
}

func (m *Mailer) dialogue(c *smtp.Client, to string, body []byte) error {
	if err := c.Hello(m.o.HelloName); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return errors.New("the server does not offer STARTTLS")
	}
	if err := c.StartTLS(m.o.TLSConfig.Clone()); err != nil {
		return fmt.Errorf("STARTTLS: %w", err)
	}
	if m.o.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.o.Username, m.o.Password, m.o.Host)); err != nil {
			return fmt.Errorf("AUTH: %w", err)
		}
	}
	if err := c.Mail(m.o.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return reply("RCPT TO", err)
	}
	w, err := c.Data()
	if err != nil {
		return reply("DATA", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if err := w.Close(); err != nil {
		return reply("DATA", err)
	}
	return c.Quit()
}

// enhancedStatus matches the enhanced status code (RFC 3463) at the start of a reply's text.
var enhancedStatus = regexp.MustCompile(`^[245]\.\d{1,3}\.\d{1,3}\b`)

// reply returns the error of a reply to the recipient or the message (command is "RCPT TO" or
// "DATA"), classified as kernel.ErrRecipientRejected, kernel.ErrMailRejected, or temporary,
// with the reply code and the enhanced status code but without the reply's text.
func reply(command string, err error) error {
	var te *textproto.Error
	if !errors.As(err, &te) {
		return fmt.Errorf("%s: %w", command, err)
	}
	status := enhancedStatus.FindString(te.Msg)
	code := strconv.Itoa(te.Code)
	if status != "" {
		code += " " + status
	}
	switch {
	case te.Code < 500 || te.Code > 599:
		return fmt.Errorf("%s: reply %s", command, code)
	case command == "RCPT TO" && strings.HasPrefix(status, "5.1."):
		return fmt.Errorf("%s: reply %s: %w", command, code, kernel.ErrRecipientRejected)
	}
	return fmt.Errorf("%s: reply %s: %w", command, code, kernel.ErrMailRejected)
}

// compose returns the message in RFC 5322 form with CRLF line ends.
func (m *Mailer) compose(msg kernel.MailMessage, now time.Time) ([]byte, error) {
	id := make([]byte, 16)
	_, _ = rand.Read(id) // crypto/rand.Read never returns an error
	domain := m.o.From[strings.LastIndexByte(m.o.From, '@')+1:]
	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	header("From", m.o.From)
	header("To", msg.To)
	// Fold between encoded words so that no header line exceeds the SMTP line limit.
	header("Subject", strings.ReplaceAll(mime.QEncoding.Encode("utf-8", msg.Subject), "?= =?", "?=\r\n =?"))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(msg.Text, "\r\n", "\n"))); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
