// SPDX-License-Identifier: AGPL-3.0-only

// Package smtp implements the mailer port over SMTP with the standard library's net/smtp
// (mail = smtp). It always upgrades the connection with STARTTLS and verifies the server's
// certificate before it authenticates or sends anything, and refuses servers that do not offer
// STARTTLS. The message is the one that package message assembles: multipart/alternative
// with a text/plain and a text/html part when it has an HTML body, text/plain otherwise, with
// the addresses in 7-bit ASCII, since the mailer does not ask for SMTPUTF8.
//
// A reply of 5xx to RCPT TO with an enhanced status code 5.1.x (RFC 3463: the address does
// not exist) fails with kernel.ErrRecipientRejected, any other 5xx to RCPT TO or DATA with
// kernel.ErrMailRejected, and everything else, such as a 4xx reply, a refused AUTH, or a
// connection that fails, as a temporary failure. The errors name the SMTP command, the reply
// code, and the enhanced status code, never the server's reply text, which often repeats the
// recipient's address.
package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/adapter/mail/internal/message"
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
	from, err := message.ASCIIAddress(o.From)
	if err != nil {
		return nil, fmt.Errorf("smtp: the sender: %w", err)
	}
	o.From = from
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
	body, to, err := message.Compose(m.o.From, msg, time.Now())
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
	if err := m.dialogue(c, to, body); err != nil {
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
