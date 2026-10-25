// SPDX-License-Identifier: AGPL-3.0-only

package maintenance

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/i18n"
)

// Page is what the maintenance page shows (ADR-0084 §4). The maintenance mode and
// `my maintenance export` fill it from the configuration (and the export from the database),
// and both render it with Render, so that the two give the same words and look.
type Page struct {
	// ServerName names the server: maintenance_server_name, or the host name.
	ServerName string
	// Message is the administrators' notice in plain text, the same in every locale; "" for none.
	Message string
	// Until is when the maintenance is expected to end; zero for unknown.
	Until time.Time
	// StatusPageURL is the instance's status page (ADR-0085 §6); "" for none.
	StatusPageURL string
	// Theme holds the values of the tokens the page's CSS uses (TokenNames).
	Theme Theme
}

// Theme holds the values of design tokens (ADR-0059) by name, for the light and the dark color
// scheme. A token that a color scheme lacks keeps the other scheme's value.
type Theme struct {
	Light, Dark map[string]string
}

// TokenNames are the tokens of the theme (app/ui/src/theme.css) that the page's CSS reads; the
// callers fill Theme with their defaults and the instance's overrides.
var TokenNames = []string{
	"color-surface", "color-surface-raised", "color-fg", "color-fg-muted", "color-accent", "color-border", "font-sans",
}

//go:embed templates/page.html templates/closure
var templateFS embed.FS

var page = template.Must(template.ParseFS(templateFS, "templates/page.html"))

// document is the data of templates/page.html: one section per locale.
// The maintenance page and the closure page (ClosurePage) fill the same template, so that they
// share their markup and CSS.
type document struct {
	// Kind prefixes the ids of the headings: "maintenance" or "closure".
	Kind, Lang, Dir, Title string
	// MetaCSP is the Content-Security-Policy of the exported pages, which have no headers.
	MetaCSP  string
	CSS      template.CSS
	Sections []section
}

// section is one locale's part of the page, its words already formatted.
type section struct {
	Lang, Dir   string
	ServerName  string
	Heading     string
	Description string
	// NoticeHeading heads Message, the administrators' plain text; Message "" leaves both out.
	NoticeHeading, Message string
	// DateLabel names the page's one time; Date is the time in RFC 3339 and DateText the same
	// time as ISO 8601 in UTC: messages formatted in Go take no date arguments (ADR-0009), so the
	// time is not localized. Date "" leaves it out.
	DateLabel, Date, DateText string
	// StatusPageURL links to the instance's status page with StatusPageText; "" for none.
	StatusPageURL, StatusPageText string
}

// withDate sets the section's time, unless t is zero.
func (s section) withDate(label string, t time.Time) section {
	if !t.IsZero() {
		u := t.UTC()
		s.DateLabel, s.Date, s.DateText = label, u.Format(time.RFC3339), u.Format("2006-01-02 15:04 UTC")
	}
	return s
}

// RenderOptions configure Render.
type RenderOptions struct {
	// MetaCSP puts the Content-Security-Policy in a <meta> element, for pages served without
	// this server's headers (my maintenance export).
	MetaCSP bool
}

// Render writes the maintenance page in the locales of ls, one section each in that order; the
// first sets the document's language and title. It returns the page and its
// Content-Security-Policy, which admits the inline CSS by its hash (ADR-0084 §4).
func Render(p Page, ls []*i18n.Localizer, o RenderOptions) (html []byte, csp string, err error) {
	if len(ls) == 0 {
		return nil, "", fmt.Errorf("maintenance: no locale")
	}
	sections := make([]section, 0, len(ls))
	for _, l := range ls {
		s := section{
			Lang: l.Locale(), Dir: l.Direction(), ServerName: p.ServerName,
			Heading: l.ServerMaintenanceTitle(), Description: l.ServerMaintenanceDescription(p.ServerName),
			NoticeHeading: l.ServerMaintenanceNotice(), Message: p.Message,
			StatusPageURL: p.StatusPageURL, StatusPageText: l.ServerMaintenanceStatusPage(),
		}
		sections = append(sections, s.withDate(l.ServerMaintenanceUntil(), p.Until))
	}
	return render(document{Kind: "maintenance", Title: ls[0].ServerMaintenanceDocumentTitle(p.ServerName), Sections: sections}, p.Theme, o)
}

// render fills d's language, CSS, and CSP from its first section and t, and executes the template.
func render(d document, t Theme, o RenderOptions) (html []byte, csp string, err error) {
	css, err := stylesheet(t)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256([]byte(css))
	policy := "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; img-src data:; base-uri 'none'; form-action 'none'"
	d.Lang, d.Dir = d.Sections[0].Lang, d.Sections[0].Dir
	d.CSS = template.CSS(css) //nolint:gosec // G203: built by stylesheet from checked token values
	if o.MetaCSP {
		d.MetaCSP = policy
	}
	var buf bytes.Buffer
	if err := page.Execute(&buf, d); err != nil {
		return nil, "", fmt.Errorf("maintenance: render: %w", err)
	}
	return buf.Bytes(), policy + "; frame-ancestors 'none'", nil
}

// stylesheet is the page's only CSS. It uses logical properties only (ADR-0009 §5) and the
// theme's tokens as custom properties, switching to the dark values by prefers-color-scheme.
func stylesheet(t Theme) (string, error) {
	light, err := declarations(t.Light, t.Dark)
	if err != nil {
		return "", err
	}
	dark, err := declarations(t.Dark, t.Light)
	if err != nil {
		return "", err
	}
	return ":root{" + light + "}@media (prefers-color-scheme: dark){:root{" + dark + "}}" + baseCSS, nil
}

// declarations writes the custom properties of TokenNames from values, falling back to other.
func declarations(values, other map[string]string) (string, error) {
	var b strings.Builder
	for _, name := range TokenNames {
		v, ok := values[name]
		if !ok {
			v = other[name]
		}
		if v == "" {
			continue
		}
		if !safeValue(v) {
			return "", fmt.Errorf("maintenance: token %s: unsafe value %q", name, v)
		}
		b.WriteString("--" + name + ":" + v + ";")
	}
	return b.String(), nil
}

// safeValue admits the characters of colors, lengths, and font families, and nothing that can
// end a declaration, a block, or the <style> element, or load a resource.
func safeValue(v string) bool {
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(" #%.,()\"'-+_", r):
		default:
			return false
		}
	}
	return !strings.Contains(strings.ToLower(v), "url(")
}

const baseCSS = `html{color-scheme:light dark;background:var(--color-surface,Canvas);color:var(--color-fg,CanvasText);` +
	`font-family:var(--font-sans,system-ui,sans-serif);line-height:1.7}` +
	`body{margin:0}` +
	`main{box-sizing:border-box;max-inline-size:40rem;margin-inline:auto;padding-block:3rem;padding-inline:1rem;display:grid;gap:1.5rem}` +
	`section{background:var(--color-surface-raised,Canvas);border:1px solid var(--color-border,GrayText);border-radius:1rem;` +
	`padding-block:1.5rem;padding-inline:1.5rem}` +
	`h1{font-size:1.5rem;line-height:1.3;margin-block:0 .75rem}` +
	`h2{font-size:1rem;margin-block:1.25rem .25rem}` +
	`p,dl{margin-block:0 .75rem}` +
	`.server{color:var(--color-fg-muted,GrayText);font-size:.875rem;font-weight:600;margin-block-end:.25rem}` +
	`.message{white-space:pre-wrap;overflow-wrap:anywhere}` +
	`dt{font-weight:600}dd{margin-inline-start:0}` +
	`a{color:var(--color-accent,LinkText)}`
