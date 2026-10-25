// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"fmt"
	"strings"
	"text/template"
	"unicode"
)

// MailSite is the server as the envelope and the signature of its emails name it (ADR-0086 §2,
// §3).
type MailSite struct {
	// Name is the instance's display name, or its domain name when it has none.
	Name string
	// URL is the server's origin, such as https://social.example.
	URL string
	// ContactEmail is the instance's contact_email setting, "" when unset. It is the Reply-To
	// and appears in the signature.
	ContactEmail string
}

// MailWords words the parts of the signature that the composer adds. The generated
// *i18n.Localizer implements it, so a use case passes the localizer of the recipient's locale.
type MailWords interface {
	// ServerMailSignatureContact words the line of the signature with the contact address.
	ServerMailSignatureContact(email string) string
}

// MailAction is the one thing an email asks the recipient to do: a button in HTML, and a line
// with the label and the URL on its own line in plain text.
type MailAction struct {
	Label string
	URL   string
}

// MailDraft is what a use case says in an email; MailComposer adds the rest.
type MailDraft struct {
	Category MailCategory
	// To is the recipient's bare address.
	To      string
	Subject string
	// Paragraphs are the body's paragraphs, each one message without line breaks of its own.
	Paragraphs []string
	// Action is the email's link, nil for none.
	Action *MailAction
	// Reason is the one sentence of the signature that says why the recipient gets the email.
	Reason string
}

// MailComposer builds every email that the server sends from a MailDraft (ADR-0086 §2): the
// plain-text body with the common signature (the server's name and URL, the contact address
// when there is one, and why the email comes), the sender's display name, and the Reply-To.
type MailComposer struct {
	Site  MailSite
	Words MailWords
}

// oneLine returns name with its control characters, line breaks among them, as spaces, so that
// an instance's name always fits in the From header.
func oneLine(name string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name))
}

// mailText is the plain-text layout. Its values are text, not HTML, so text/template is right.
var mailText = template.Must(template.New("mail").Parse(
	`{{range .Paragraphs}}{{.}}

{{end}}{{with .Action}}{{.Label}}
{{.URL}}

{{end}}{{"-- "}}
{{.Site.Name}}
{{.Site.URL}}
{{with .Contact}}{{.}}
{{end}}
{{.Reason}}
`))

// Compose returns the message of d. It fails with an error wrapping ErrInvalidMail when d has
// no paragraph or no reason, and the message does not pass ValidateMail.
func (c MailComposer) Compose(d MailDraft) (MailMessage, error) {
	if len(d.Paragraphs) == 0 || d.Reason == "" {
		return MailMessage{}, fmt.Errorf("%w: an email needs a paragraph and a reason", ErrInvalidMail)
	}
	data := struct {
		MailDraft
		Site    MailSite
		Contact string
	}{MailDraft: d, Site: c.Site}
	if c.Site.ContactEmail != "" && c.Words != nil {
		data.Contact = c.Words.ServerMailSignatureContact(c.Site.ContactEmail)
	}
	var b strings.Builder
	if err := mailText.Execute(&b, data); err != nil {
		return MailMessage{}, err
	}
	m := MailMessage{Category: d.Category, To: d.To, FromName: oneLine(c.Site.Name), Subject: d.Subject, Text: b.String()}
	// A contact address that is not one bare address leaves the replies to the sender rather
	// than failing every email.
	if bareAddress(c.Site.ContactEmail) {
		m.ReplyTo = c.Site.ContactEmail
	}
	if err := ValidateMail(m); err != nil {
		return MailMessage{}, err
	}
	return m, nil
}
