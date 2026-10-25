// SPDX-License-Identifier: AGPL-3.0-only

// Package mailtest decodes the messages that the mail adapters send, for their tests: the
// stand-ins of the SMTP server and of the SES API hand what they receive to Parse.
package mailtest

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Parse decodes data, a message in RFC 5322 form, back into a MailMessage, with its headers.
// It fails when the message lacks a Message-ID or a Date, has a line longer than 998 bytes, or
// is not text/plain or multipart/alternative of a text/plain and a text/html part, each UTF-8
// and quoted-printable. The bodies have "\n" line ends.
func Parse(data []byte) (kernel.MailMessage, mail.Header, error) {
	var msg kernel.MailMessage
	m, err := mail.ReadMessage(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return msg, nil, fmt.Errorf("read the message: %w", err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil {
		return msg, nil, fmt.Errorf("decode the subject %q: %w", m.Header.Get("Subject"), err)
	}
	if m.Header.Get("Message-Id") == "" || m.Header.Get("Date") == "" {
		return msg, nil, fmt.Errorf("no Message-ID or Date in %v", m.Header)
	}
	for line := range strings.Lines(string(data)) {
		if n := len(strings.TrimRight(line, "\r\n")); n > 998 {
			return msg, nil, fmt.Errorf("a line of %d bytes", n)
		}
	}
	from, err := mail.ParseAddress(m.Header.Get("From"))
	if err != nil {
		return msg, nil, fmt.Errorf("parse From %q: %w", m.Header.Get("From"), err)
	}
	msg = kernel.MailMessage{To: m.Header.Get("To"), FromName: from.Name, ReplyTo: m.Header.Get("Reply-To"), Subject: subject}
	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	switch {
	case err != nil:
		return msg, nil, fmt.Errorf("Content-Type %q: %w", m.Header.Get("Content-Type"), err)
	case mediaType == "multipart/alternative":
		// The plain text first and the HTML last, the order of preference (RFC 2046 §5.1.4).
		r := multipart.NewReader(m.Body, params["boundary"])
		for i, want := range []string{"text/plain; charset=utf-8", "text/html; charset=utf-8"} {
			p, err := r.NextRawPart()
			if err != nil {
				return msg, nil, fmt.Errorf("part %d: %w", i, err)
			}
			if p.Header.Get("Content-Type") != want || p.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
				return msg, nil, fmt.Errorf("part %d headers = %v, want %s", i, p.Header, want)
			}
			body, err := decode(p)
			if err != nil {
				return msg, nil, err
			}
			if i == 0 {
				msg.Text = body
			} else {
				msg.HTML = body
			}
		}
		if _, err := r.NextRawPart(); !errors.Is(err, io.EOF) {
			return msg, nil, fmt.Errorf("a third part: %w", err)
		}
	default:
		if m.Header.Get("Content-Type") != "text/plain; charset=utf-8" || m.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
			return msg, nil, fmt.Errorf("headers = %v", m.Header)
		}
		if msg.Text, err = decode(m.Body); err != nil {
			return msg, nil, err
		}
	}
	return msg, m.Header, nil
}

// decode returns the quoted-printable body r with "\n" line ends.
func decode(r io.Reader) (string, error) {
	body, err := io.ReadAll(quotedprintable.NewReader(r))
	if err != nil {
		return "", fmt.Errorf("decode a body: %w", err)
	}
	return strings.ReplaceAll(string(body), "\r\n", "\n"), nil
}
