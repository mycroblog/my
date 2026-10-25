// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

func validMail() kernel.MailMessage {
	return kernel.MailMessage{
		Category: kernel.MailNotification, To: "alice@example.com", FromName: "例のサーバー", ReplyTo: "admin@social.example",
		Subject: "Hello", Text: "x",
		Headers: map[string]string{
			kernel.HeaderListUnsubscribe:     "<https://social.example/mail/unsubscribe?token=x>",
			kernel.HeaderListUnsubscribePost: "List-Unsubscribe=One-Click",
		},
	}
}

func TestValidateMail(t *testing.T) {
	if err := kernel.ValidateMail(validMail()); err != nil {
		t.Fatalf("ValidateMail(valid) = %v", err)
	}
	for name, change := range map[string]func(*kernel.MailMessage){
		"no category":            func(m *kernel.MailMessage) { m.Category = "" },
		"unknown category":       func(m *kernel.MailMessage) { m.Category = "marketing" },
		"named recipient":        func(m *kernel.MailMessage) { m.To = "Alice <alice@example.com>" },
		"two recipients":         func(m *kernel.MailMessage) { m.To = "alice@example.com, mallory@example.com" },
		"line break in subject":  func(m *kernel.MailMessage) { m.Subject = "Hello\r\nBcc: mallory@example.com" },
		"empty subject":          func(m *kernel.MailMessage) { m.Subject = "" },
		"line break in name":     func(m *kernel.MailMessage) { m.FromName = "Example\r\nBcc: mallory@example.com" },
		"newline in name":        func(m *kernel.MailMessage) { m.FromName = "Example\nSocial" },
		"named reply-to":         func(m *kernel.MailMessage) { m.ReplyTo = "Admin <admin@social.example>" },
		"reply-to line break":    func(m *kernel.MailMessage) { m.ReplyTo = "admin@social.example\r\nBcc: mallory@example.com" },
		"header not allowed":     func(m *kernel.MailMessage) { m.Headers = map[string]string{"Bcc": "mallory@example.com"} },
		"line break in a header": func(m *kernel.MailMessage) { m.Headers[kernel.HeaderListUnsubscribe] = "<https://x>\r\nBcc: m@x" },
		"empty header":           func(m *kernel.MailMessage) { m.Headers[kernel.HeaderListUnsubscribePost] = "" },
		"invalid UTF-8 body":     func(m *kernel.MailMessage) { m.Text = "\xff" },
		"invalid UTF-8 HTML":     func(m *kernel.MailMessage) { m.HTML = "\xff" },
		"long subject":           func(m *kernel.MailMessage) { m.Subject = strings.Repeat("a", 999) },
	} {
		m := validMail()
		change(&m)
		if err := kernel.ValidateMail(m); !errors.Is(err, kernel.ErrInvalidMail) {
			t.Errorf("%s: ValidateMail() = %v, want ErrInvalidMail", name, err)
		}
	}
}

// words words the signature's contact and unsubscribe lines.
type words struct{}

func (words) ServerMailSignatureContact(email string) string { return "Contact: " + email }

func (words) ServerMailSignatureUnsubscribe(url string) string { return "Stop these emails: " + url }

func TestMailComposer(t *testing.T) {
	draft := kernel.MailDraft{
		Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Reset your password for Example",
		Paragraphs: []string{"Someone asked to reset the password.", "The link works for 15 minutes, once."},
		Action:     &kernel.MailAction{Label: "Set a new password", URL: "https://social.example/auth/reset?token=x"},
		Reason:     "You get this email because someone asked at social.example.",
	}
	site := kernel.MailSite{Name: "Example", URL: "https://social.example", ContactEmail: "admin@social.example"}
	m, err := kernel.MailComposer{Site: site, Words: words{}}.Compose(draft)
	if err != nil {
		t.Fatal(err)
	}
	want := "Someone asked to reset the password.\n\nThe link works for 15 minutes, once.\n\n" +
		"Set a new password\nhttps://social.example/auth/reset?token=x\n\n" +
		"-- \nExample\nhttps://social.example\nContact: admin@social.example\n\n" +
		"You get this email because someone asked at social.example.\n"
	if m.Text != want {
		t.Errorf("Text =\n%s\nwant\n%s", m.Text, want)
	}
	if m.Category != kernel.MailTransactional || m.To != draft.To || m.Subject != draft.Subject ||
		m.FromName != "Example" || m.ReplyTo != "admin@social.example" || m.HTML != "" || len(m.Headers) != 0 {
		t.Errorf("message = %+v", m)
	}

	// Without a contact address there is no Reply-To and no contact line; without an action
	// there is no link; a name with a line break becomes one line.
	draft.Action = nil
	m, err = kernel.MailComposer{Site: kernel.MailSite{Name: "Example\r\nSocial", URL: "https://social.example"}, Words: words{}}.Compose(draft)
	if err != nil {
		t.Fatal(err)
	}
	if m.ReplyTo != "" || strings.Contains(m.Text, "Contact") || strings.Contains(m.Text, "Set a new password") || m.FromName != "Example  Social" {
		t.Errorf("message without contact = %+v", m)
	}

	// A contact address that is not a bare address is not a Reply-To.
	m, err = kernel.MailComposer{Site: kernel.MailSite{Name: "Example", ContactEmail: "Admin <a@b.example>"}, Words: words{}}.Compose(draft)
	if err != nil || m.ReplyTo != "" {
		t.Errorf("Compose with a named contact = %+v, %v", m, err)
	}

	// A notification with an unsubscribe link carries it in the List-Unsubscribe headers and at
	// the end of the signature.
	const unsubscribe = "https://social.example/mail/unsubscribe?token=my_eut_x"
	notice := kernel.MailDraft{
		Category: kernel.MailNotification, To: "alice@example.com", Subject: "New report",
		Paragraphs: []string{"A new report came in."}, Reason: "You chose these emails.", UnsubscribeURL: unsubscribe,
	}
	m, err = kernel.MailComposer{Site: kernel.MailSite{Name: "Example", URL: "https://social.example"}, Words: words{}}.Compose(notice)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(m.Text, "\nYou chose these emails.\nStop these emails: "+unsubscribe+"\n") {
		t.Errorf("Text without the unsubscribe link:\n%s", m.Text)
	}
	if m.Headers[kernel.HeaderListUnsubscribe] != "<"+unsubscribe+">" ||
		m.Headers[kernel.HeaderListUnsubscribePost] != "List-Unsubscribe=One-Click" || len(m.Headers) != 2 {
		t.Errorf("Headers = %v", m.Headers)
	}

	for name, d := range map[string]kernel.MailDraft{
		"unsubscribe from a transactional email": {
			Category: kernel.MailTransactional, To: "alice@example.com", Subject: "x", Paragraphs: []string{"p"}, Reason: "r",
			UnsubscribeURL: unsubscribe,
		},
		"no paragraph": {Category: kernel.MailTransactional, To: "alice@example.com", Subject: "x", Reason: "r"},
		"no reason":    {Category: kernel.MailTransactional, To: "alice@example.com", Subject: "x", Paragraphs: []string{"p"}},
		"injection":    {Category: kernel.MailTransactional, To: "alice@example.com", Subject: "x\nBcc: m@x", Paragraphs: []string{"p"}, Reason: "r"},
		"no category":  {To: "alice@example.com", Subject: "x", Paragraphs: []string{"p"}, Reason: "r"},
	} {
		if _, err := (kernel.MailComposer{Site: site, Words: words{}}).Compose(d); !errors.Is(err, kernel.ErrInvalidMail) {
			t.Errorf("%s: Compose() = %v, want ErrInvalidMail", name, err)
		}
	}
}
