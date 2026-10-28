// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

func htmlDraft() kernel.MailDraft {
	return kernel.MailDraft{
		Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Reset your password for Example",
		Paragraphs: []string{"Someone asked to reset the password.", "The link works for 15 minutes, once."},
		Action:     &kernel.MailAction{Label: "Set a new password", URL: "https://social.example/auth/reset?token=x"},
		Reason:     "You get this email because someone asked at social.example.",
	}
}

var htmlSite = kernel.MailSite{Name: "Example", URL: "https://social.example", ContactEmail: "admin@social.example"}

func composeHTML(t *testing.T, site kernel.MailSite, w words, d kernel.MailDraft) string {
	t.Helper()
	m, err := kernel.MailComposer{Site: site, Words: w}.Compose(d)
	if err != nil {
		t.Fatal(err)
	}
	return m.HTML
}

func TestMailComposerHTML(t *testing.T) {
	html := composeHTML(t, htmlSite, words{}, htmlDraft())
	for _, want := range []string{
		`<html lang="en" dir="ltr"`,
		"<title>Reset your password for Example</title>",
		">Someone asked to reset the password.</div>",
		">The link works for 15 minutes, once.</div>",
		`href="https://social.example/auth/reset?token=x"`,
		"Set a new password",
		">Example</div>",
		`href="https://social.example"`,
		">Contact: admin@social.example</div>",
		">You get this email because someone asked at social.example.</div>",
		// Light and dark, and Outlook's layout in its conditional comments.
		"color-scheme: light dark",
		"@media (prefers-color-scheme: dark)",
		"<!--[if mso | IE]>",
		"direction:ltr",
		"text-align:left",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML has no %q", want)
		}
	}
	if strings.Contains(html, "{{") {
		t.Error("HTML has a template action left")
	}
}

func TestMailComposerHTMLLogo(t *testing.T) {
	html := composeHTML(t, htmlSite, words{}, htmlDraft())
	if strings.Contains(html, "<img") {
		t.Errorf("the HTML without a logo has an image:\n%s", html)
	}

	site := htmlSite
	site.Name = `Example "&" Co`
	site.Logo = &kernel.MailLogo{URL: "https://social.example/brand/r1/logo-mail.png", Width: 384, Height: 96}
	html = composeHTML(t, site, words{}, htmlDraft())
	want := `<img src="https://social.example/brand/r1/logo-mail.png" width="192" height="48" alt="Example &#34;&amp;&#34; Co" class=""`
	if !strings.Contains(html, want) || strings.Contains(html, `class="mail-logo-dark"`) {
		t.Errorf("the HTML lacks %s:\n%s", want, html)
	}

	site.LogoDark = &kernel.MailLogo{URL: "https://social.example/brand/r2/logo-dark-mail.png", Width: 384, Height: 96}
	html = composeHTML(t, site, words{}, htmlDraft())
	for _, want := range []string{
		`class="mail-logo-light"`,
		`<img src="https://social.example/brand/r2/logo-dark-mail.png" width="192" height="48" alt="Example &#34;&amp;&#34; Co" class="mail-logo-dark" style="display: none;`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the HTML lacks %s:\n%s", want, html)
		}
	}

	// A logo that is not https is left out, and the dark logo with it.
	site.Logo.URL = "http://social.example/brand/r1/logo-mail.png"
	if html := composeHTML(t, site, words{}, htmlDraft()); strings.Contains(html, "<img") {
		t.Errorf("the HTML has a logo that is not https:\n%s", html)
	}
}

// TestMailComposerHTMLLoadsNothing checks that the HTML of a server without a logo refers to
// nothing outside itself: no web font, no image, no style sheet, and no link but those of the email.
func TestMailComposerHTMLLoadsNothing(t *testing.T) {
	html := composeHTML(t, htmlSite, words{}, htmlDraft())
	for _, banned := range []string{"googleapis", "<link", "@import", "<img", "src=", "url(", "<script"} {
		if strings.Contains(html, banned) {
			t.Errorf("HTML has %q", banned)
		}
	}
	for _, u := range regexp.MustCompile(`https?://[^\s"'<>)]+`).FindAllString(html, -1) {
		if !strings.HasPrefix(u, "https://social.example") && !strings.HasPrefix(u, "http://www.w3.org/") {
			t.Errorf("HTML refers to %s", u)
		}
	}
}

func TestMailComposerHTMLEscapes(t *testing.T) {
	d := htmlDraft()
	d.Paragraphs = []string{`<script>alert(1)</script> & "quoted"`}
	d.Action = &kernel.MailAction{Label: `<b>Go</b>`, URL: `https://social.example/a?b="><script>alert(1)</script>`}
	d.Reason = "<i>why</i>"
	site := htmlSite
	site.Name = `<img src=x onerror=alert(1)>`
	html := composeHTML(t, site, words{}, d)
	for _, raw := range []string{"<script>", "<b>Go</b>", "<i>why</i>", "<img", `"><script`} {
		if strings.Contains(html, raw) {
			t.Errorf("HTML has %q unescaped", raw)
		}
	}
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt; &amp; &#34;quoted&#34;", "&lt;b&gt;Go&lt;/b&gt;", "&lt;img src=x onerror=alert(1)&gt;"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML has no %q", want)
		}
	}
}

// TestMailComposerHTMLLinksHTTPSOnly checks that an action URL other than https is never a link:
// a javascript: URL is text, not an href, in either version.
func TestMailComposerHTMLLinksHTTPSOnly(t *testing.T) {
	for _, u := range []string{"javascript:alert(1)", "JavaScript:alert(document.cookie)", "http://social.example/x", "data:text/html,x", "//social.example/x"} {
		d := htmlDraft()
		d.Action = &kernel.MailAction{Label: "Open", URL: u}
		site := htmlSite
		site.URL = "http://social.example"
		html := composeHTML(t, site, words{}, d)
		if n := strings.Count(html, "href="); n != 0 {
			t.Errorf("%s: HTML has %d links", u, n)
		}
		if strings.Contains(strings.ToLower(html), `href="javascript:`) || !strings.Contains(html, ">Open</div>") {
			t.Errorf("%s: HTML = %s", u, html)
		}
	}
}

func TestMailComposerHTMLUnsubscribe(t *testing.T) {
	d := htmlDraft()
	d.Category = kernel.MailNotification
	d.UnsubscribeURL = "https://social.example/mail/unsubscribe?token=t&x=1"
	html := composeHTML(t, htmlSite, words{}, d)
	want := `>Stop these emails: <a href="https://social.example/mail/unsubscribe?token=t&amp;x=1" class="mail-link"`
	if !strings.Contains(html, want) {
		t.Errorf("HTML has no %q", want)
	}
	// A link that is not https is text.
	d.UnsubscribeURL = "http://social.example/mail/unsubscribe?token=t"
	html = composeHTML(t, htmlSite, words{}, d)
	if !strings.Contains(html, ">Stop these emails: http://social.example/mail/unsubscribe?token=t</div>") ||
		strings.Contains(html, `href="http://social.example/mail`) {
		t.Error("HTML links to an unsubscribe URL that is not https")
	}
	// A transactional email has no unsubscribe line.
	if html := composeHTML(t, htmlSite, words{}, htmlDraft()); strings.Contains(html, "Stop these emails") {
		t.Error("transactional HTML has an unsubscribe line")
	}
}

func TestMailComposerHTMLRightToLeft(t *testing.T) {
	html := composeHTML(t, htmlSite, words{locale: "ar", dir: "rtl"}, htmlDraft())
	if !strings.Contains(html, `<html lang="ar" dir="rtl"`) {
		t.Errorf("HTML of ar starts %q", html[:min(len(html), 200)])
	}
	if strings.Contains(html, "direction:ltr") || strings.Contains(html, "text-align:left") || !strings.Contains(html, "text-align:right") {
		t.Error("HTML of ar keeps a left-to-right layout")
	}
	// Without words, the HTML is English.
	m, err := kernel.MailComposer{Site: htmlSite}.Compose(htmlDraft())
	if err != nil || !strings.Contains(m.HTML, `<html lang="en" dir="ltr"`) {
		t.Errorf("Compose without words = %v", err)
	}
}
