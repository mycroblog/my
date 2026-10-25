// SPDX-License-Identifier: AGPL-3.0-only

// Package smtp implements the mailer port over SMTP with the standard library's net/smtp
// (mail = smtp). It always upgrades the connection with STARTTLS and verifies the server's
// certificate before it authenticates or sends anything, and refuses servers that do not offer
// STARTTLS. A message with an HTML body is multipart/alternative with a text/plain and a
// text/html part, one without is text/plain; every body is UTF-8, quoted-printable encoded.
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
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

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
	var out error
	switch {
	case te.Code < 500 || te.Code > 599:
		out = fmt.Errorf("%s: reply %s", command, code)
	case command == "RCPT TO" && strings.HasPrefix(status, "5.1."):
		out = fmt.Errorf("%s: reply %s: %w", command, code, kernel.ErrRecipientRejected)
	default:
		out = fmt.Errorf("%s: reply %s: %w", command, code, kernel.ErrMailRejected)
	}
	if status == "" {
		return out
	}
	return &kernel.StatusError{Status: status, Err: out}
}

// compose returns the message in RFC 5322 form with CRLF line ends, multipart/alternative when
// it has an HTML body. From carries the server's
// name as its display name (RFC 2047), Reply-To the contact address when there is one,
// Message-ID a UUIDv7 at the domain of the sender, and Auto-Submitted marks every message as
// sent by a program so that auto-responders do not answer it (RFC 3834).
func (m *Mailer) compose(msg kernel.MailMessage, now time.Time) ([]byte, error) {
	domain := m.o.From[strings.LastIndexByte(m.o.From, '@')+1:]
	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	header("From", fold((&mail.Address{Name: msg.FromName, Address: m.o.From}).String()))
	header("To", msg.To)
	if msg.ReplyTo != "" {
		header("Reply-To", msg.ReplyTo)
	}
	header("Subject", fold(mime.QEncoding.Encode("utf-8", msg.Subject)))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+uuid.NewV7().String()+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Auto-Submitted", "auto-generated")
	for _, name := range slices.Sorted(maps.Keys(msg.Headers)) {
		header(name, msg.Headers[name])
	}
	if msg.HTML == "" {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		if err := writeQuotedPrintable(&b, msg.Text); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}
	// The text and the HTML version of the same content, the preferred last (RFC 2046 §5.1.4).
	parts := multipart.NewWriter(&b)
	header("Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": parts.Boundary()}))
	b.WriteString("\r\n")
	for _, part := range []struct{ contentType, body string }{
		{"text/plain; charset=utf-8", msg.Text},
		{"text/html; charset=utf-8", msg.HTML},
	} {
		w, err := parts.CreatePart(textproto.MIMEHeader{
			"Content-Type": {part.contentType}, "Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		if err := writeQuotedPrintable(w, part.body); err != nil {
			return nil, err
		}
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// writeQuotedPrintable writes body to w quoted-printable encoded, with CRLF line ends.
func writeQuotedPrintable(w io.Writer, body string) error {
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(strings.ReplaceAll(body, "\r\n", "\n"))); err != nil {
		return err
	}
	return qp.Close()
}

// fold breaks a header value between its encoded words (RFC 2047) so that no header line
// exceeds the SMTP line limit.
func fold(value string) string { return strings.ReplaceAll(value, "?= =?", "?=\r\n =?") }
