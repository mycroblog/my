// SPDX-License-Identifier: AGPL-3.0-only

package mailpreview

import (
	_ "embed"
	"fmt"
	htmltemplate "html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
	_ "time/tzdata" // the sample time zones, whatever the machine has, as the server binary embeds them

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// Locales are the locales that the preview renders: the source locale, Japanese, and the
// pseudo-locales that stretch the text (en-XA) and mirror it (ar-XB).
var Locales = []string{"en", "ja", i18n.PseudoLocaleLong, i18n.PseudoLocaleRTL}

// sampleFrom is the sender's address of the preview's From header.
const sampleFrom = "noreply@" + sampleHost

// Rendered is an entry composed in one locale.
type Rendered struct {
	Entry   Entry
	Locale  string
	Message kernel.MailMessage
}

// Headers are the message's headers as the Mailer writes them, in order: the envelope that
// kernel.MailMessage describes, without the ones that each send makes (Date, Message-ID, MIME).
func (r Rendered) Headers() [][2]string {
	m := r.Message
	h := [][2]string{
		{"From", fmt.Sprintf("%q <%s>", m.FromName, sampleFrom)},
		{"To", m.To},
	}
	if m.ReplyTo != "" {
		h = append(h, [2]string{"Reply-To", m.ReplyTo})
	}
	h = append(h, [2]string{"Subject", m.Subject}, [2]string{"Auto-Submitted", "auto-generated"})
	keys := make([]string, 0, len(m.Headers))
	for k := range m.Headers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		h = append(h, [2]string{k, m.Headers[k]})
	}
	return append(h, [2]string{"Category", string(m.Category)})
}

// Load returns the bundle of the embedded catalogs with the pseudo-locales.
func Load() (*i18n.Bundle, error) { return i18n.Load(i18n.WithPseudoLocales()) }

// Render composes every entry of the catalog in locale.
func Render(bundle *i18n.Bundle, locale string) ([]Rendered, error) {
	l := bundle.Localizer(locale)
	if l.Locale() != locale {
		return nil, fmt.Errorf("mailpreview: the catalogs have no locale %s", locale)
	}
	c := kernel.MailComposer{Site: Site, Words: l}
	var out []Rendered
	for _, e := range Catalog() {
		d, ok := e.Draft(Sample{L: l})
		if !ok {
			return nil, fmt.Errorf("mailpreview: %s has no email", e.Name())
		}
		m, err := c.Compose(d)
		if err != nil {
			return nil, fmt.Errorf("mailpreview: compose %s in %s: %w", e.Name(), locale, err)
		}
		out = append(out, Rendered{Entry: e, Locale: locale, Message: m})
	}
	return out, nil
}

// darkMedia is the query of the frame's dark styles.
const darkMedia = "@media (prefers-color-scheme: dark)"

// Dark returns html with its dark styles applied whatever the browser prefers, for the preview's
// dark mode: the frame's prefers-color-scheme query matches always.
func Dark(html string) string {
	return strings.ReplaceAll(strings.ReplaceAll(html, darkMedia, "@media all"),
		"color-scheme: light dark;", "color-scheme: dark;")
}

//go:embed index.html.tmpl
var indexSource string

var indexTemplate = htmltemplate.Must(htmltemplate.New("index").Parse(indexSource))

// indexEmail is an email in the data of index.html.
type indexEmail struct {
	Locale  string      `json:"locale"`
	Name    string      `json:"name"`
	Dir     string      `json:"dir"`
	Subject string      `json:"subject"`
	Headers [][2]string `json:"headers"`
	Text    string      `json:"text"`
	// HTML and Dark are the paths of the HTML version and its dark version, relative to
	// index.html.
	HTML string `json:"html"`
	Dark string `json:"dark"`
}

// Write renders every entry of the catalog in locales and writes, in dir, the HTML version
// (<locale>/<name>.html), its dark version (<locale>/<name>.dark.html), and the text version
// (<locale>/<name>.txt) of each, and index.html, which shows them. It returns the path of
// index.html.
func Write(dir string, locales []string) (string, error) {
	bundle, err := Load()
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	var emails []indexEmail
	for _, locale := range locales {
		rendered, err := Render(bundle, locale)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Join(dir, locale), 0o750); err != nil {
			return "", err
		}
		for _, r := range rendered {
			base := locale + "/" + r.Entry.Name()
			files := map[string]string{
				base + ".html":      r.Message.HTML,
				base + ".dark.html": Dark(r.Message.HTML),
				base + ".txt":       r.Message.Text,
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil { //nolint:gosec // G306: a preview for the developer to open
					return "", err
				}
			}
			emails = append(emails, indexEmail{
				Locale: locale, Name: r.Entry.Name(), Dir: i18n.Direction(locale), Subject: r.Message.Subject,
				Headers: r.Headers(), Text: r.Message.Text, HTML: base + ".html", Dark: base + ".dark.html",
			})
		}
	}
	index := filepath.Join(dir, "index.html")
	f, err := os.Create(index) //nolint:gosec // G304: the preview's own directory
	if err != nil {
		return "", err
	}
	if err := indexTemplate.Execute(f, struct {
		Locales []string
		Emails  []indexEmail
	}{locales, emails}); err != nil {
		_ = f.Close()
		return "", err
	}
	return index, f.Close()
}
