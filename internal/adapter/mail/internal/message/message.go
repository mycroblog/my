// SPDX-License-Identifier: AGPL-3.0-only

// Package message assembles the RFC 5322 form of a kernel.MailMessage that the mail adapters
// send (mail/smtp over SMTP, mail/ses as the raw content of SES v2 SendEmail), so that every
// adapter writes the same message (ADR-0086 §3, §6).
//
// The adapters do not ask for SMTPUTF8 (SES does not offer it), so the addresses are 7-bit
// ASCII: the domain of an internationalized address in Punycode, and a local part that is not
// ASCII is refused with kernel.ErrInvalidMail. The display name and the subject are MIME
// encoded-words (RFC 2047), and every body is UTF-8, quoted-printable encoded.
package message

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"slices"
	"strings"
	"time"
	"uuid"

	"golang.org/x/net/idna"

	"github.com/mycroblog/my/internal/app/kernel"
)

// domainProfile converts the domain of an address to its ASCII form (IDNA 2008, as for lookup).
var domainProfile = idna.New(idna.MapForLookup(), idna.Transitional(false), idna.BidiRule())

// ASCIIAddress returns the bare address addr in 7-bit ASCII, its domain in Punycode. It fails
// with kernel.ErrInvalidMail when the local part is not ASCII, which no server without
// SMTPUTF8 accepts, or the domain is not a valid name.
func ASCIIAddress(addr string) (string, error) {
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 {
		return "", fmt.Errorf("%w: not an address", kernel.ErrInvalidMail)
	}
	local, domain := addr[:at], addr[at+1:]
	if !ascii(local) {
		return "", fmt.Errorf("%w: the local part of an address must be ASCII", kernel.ErrInvalidMail)
	}
	if ascii(domain) {
		return addr, nil
	}
	d, err := domainProfile.ToASCII(domain)
	if err != nil {
		return "", fmt.Errorf("%w: the domain of an address is not a valid name", kernel.ErrInvalidMail)
	}
	return local + "@" + d, nil
}

func ascii(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Compose returns msg, validated by the caller with kernel.ValidateMail, in RFC 5322 form with
// CRLF line ends, sent by the bare address from, multipart/alternative when it has an HTML
// body. From carries the server's name as its display name (RFC 2047), Reply-To the contact
// address when there is one, Message-ID a UUIDv7 at the domain of the sender, and
// Auto-Submitted marks every message as sent by a program so that auto-responders do not
// answer it (RFC 3834). It also returns the recipient in ASCII, for the envelope.
func Compose(from string, msg kernel.MailMessage, now time.Time) (body []byte, to string, err error) {
	if from, err = ASCIIAddress(from); err != nil {
		return nil, "", err
	}
	if to, err = ASCIIAddress(msg.To); err != nil {
		return nil, "", err
	}
	replyTo := ""
	if msg.ReplyTo != "" {
		if replyTo, err = ASCIIAddress(msg.ReplyTo); err != nil {
			return nil, "", err
		}
	}
	domain := from[strings.LastIndexByte(from, '@')+1:]
	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	header("From", fold((&mail.Address{Name: msg.FromName, Address: from}).String()))
	header("To", to)
	if replyTo != "" {
		header("Reply-To", replyTo)
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
			return nil, "", err
		}
		return b.Bytes(), to, nil
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
			return nil, "", err
		}
		if err := writeQuotedPrintable(w, part.body); err != nil {
			return nil, "", err
		}
	}
	if err := parts.Close(); err != nil {
		return nil, "", err
	}
	return b.Bytes(), to, nil
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
