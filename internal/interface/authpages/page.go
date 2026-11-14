// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// Every screen is a document of the web client's auth entry (ADR-0028 §4, ADR-0090 §16): a shell
// that loads the entry's script and stylesheets, with the screen's data (page) in a JSON data
// block that the entry renders. The shell is the same for every screen; html/template writes it,
// escaping every value for its context and encoding the data as JSON that cannot end the script
// element. Nothing of a screen is HTML of this package.

// Entry is what the shell loads to start the auth entry: web.Handler.AuthEntry.
type Entry struct {
	// Script is the entry module; "" leaves the screens blank.
	Script string
	// Stylesheets are the CSS of the entry and of its static imports, and Preloads those imports.
	Stylesheets, Preloads []string
	// ViteClient is the client script of a Vite dev server, and DevOrigin that server's origin,
	// which the screens' policy then allows; both empty in production.
	ViteClient, DevOrigin string
}

// The screens, the screen field of the data that tells the entry which one to render.
const (
	screenSignIn           = "sign_in"
	screenTwoFactor        = "two_factor"
	screenSignedIn         = "signed_in"
	screenError            = "error"
	screenSignUp           = "sign_up"
	screenStatus           = "status"
	screenVerifyEmail      = "verify_email"
	screenVerified         = "verified"
	screenAccountDeleted   = "account_deleted"
	screenAuthorize        = "authorize"
	screenForgotPassword   = "forgot_password"
	screenResetPassword    = "reset_password"
	screenPasswordReset    = "password_reset"
	screenMailUnsubscribe  = "mail_unsubscribe"
	screenMailUnsubscribed = "mail_unsubscribed"
)

// page is the data of a screen, the JSON that the entry reads. The entry words the screen
// itself; the strings here are what this package decides and words in the screen's locale: the
// outcome of a form (Error, Notice, a field's error) and what it reads of the server.
type page struct {
	// L words the strings in the screen's locale, which Lang names and Dir writes.
	L         *i18n.Localizer `json:"-"`
	Lang, Dir string          `json:"-"`
	// formAction lists origins the screen's forms may be redirected to besides
	// Options.FormActionOrigins.
	formAction []string

	Screen string `json:"screen"`
	// Server is the server the screen belongs to; render fills it.
	Server server `json:"server"`
	// Documents are the links to the published documents, in the footer of every screen and on
	// the registration form; render fills them.
	Documents []documentLink `json:"documents,omitempty"`
	// StatusPageURL is the instance's status page, in the footer of every screen; render fills it.
	StatusPageURL string `json:"statusPageUrl,omitempty"`

	CSRFToken string `json:"csrfToken,omitempty"`
	ReturnTo  string `json:"returnTo,omitempty"`
	Email     string `json:"email,omitempty"`
	// Error is an alert about the whole form; Notice reports success.
	Error  string `json:"error,omitempty"`
	Notice string `json:"notice,omitempty"`

	// CanSignUp shows the link to the registration screen, and CanResetPassword the link to the
	// screen that asks for a password reset link.
	CanSignUp        bool `json:"canSignUp,omitempty"`
	CanResetPassword bool `json:"canResetPassword,omitempty"`
	// LoginProvidersOnly leaves the password and passkey forms out of the sign-in screen and says
	// that only the login providers sign in.
	LoginProvidersOnly bool `json:"loginProvidersOnly,omitempty"`
	// SignUp is the registration form.
	SignUp *signUpForm `json:"signUp,omitempty"`
	// Status is the state of an account that cannot be used yet.
	Status *statusView `json:"status,omitempty"`
	// Token is the email verification token of the confirmation form, the password reset token
	// of the form of a new password, or the token of an unsubscribe link.
	Token string `json:"token,omitempty"`
	// ForgotPassword is the screen that asks for a password reset link, and NewPassword the field
	// of the screen that sets the new password (password_reset.go).
	ForgotPassword *forgotPasswordView `json:"forgotPassword,omitempty"`
	NewPassword    *newPasswordView    `json:"newPassword,omitempty"`
	// Unsubscribe is the screen of an unsubscribe link (mail_unsubscribe.go).
	Unsubscribe *unsubscribeView `json:"unsubscribe,omitempty"`
	// Consent is the authorization request on the consent screen.
	Consent *consent `json:"consent,omitempty"`
	// HumanCheck is the CAPTCHA widget of the form; the screen's Content-Security-Policy admits
	// the service when it is set.
	HumanCheck *humanCheckView `json:"humanCheck,omitempty"`
	// Passkey is the passkey sign-in of the sign-in screen.
	Passkey *passkeyView `json:"passkey,omitempty"`
	// External are the links to the external login providers of the sign-in and registration
	// screens (external.go).
	External []externalButton `json:"external,omitempty"`
}

// server is what a screen shows of the server it belongs to: its name, its own logo (or the
// built-in wordmark, when Logo is ""), and its introduction.
type server struct {
	Name        string `json:"name"`
	Logo        string `json:"logo,omitempty"`
	LogoDark    string `json:"logoDark,omitempty"`
	Description string `json:"description,omitempty"`
	Banner      string `json:"banner,omitempty"`
}

// newPage returns the data of screen in the request's locale.
func (h *Handler) newPage(r *http.Request, screen string) page {
	l := h.o.Locales.RequestLocalizer(r)
	return page{L: l, Lang: l.Locale(), Dir: l.Direction(), Screen: screen}
}

// shellData is what the shell writes.
type shellData struct {
	Lang, Dir, Title string
	Entry            Entry
	ThemeStylesheet  string
	// Page is a struct, not a string: html/template encodes it as JSON in the data block and
	// escapes what could end the script element.
	Page page
}

// shellTemplate is the document of every screen. The data block sits in the body between the
// comments that keep Cloudflare's Email Address Obfuscation from rewriting the addresses it holds
// ("we sent the email to alice@example.com") into "[email protected]"; html/template drops the
// comments of a template, so they come from functions.
var shellTemplate = template.Must(template.New("auth").Funcs(template.FuncMap{
	"emailOff": func() template.HTML { return "<!--email_off-->" },
	"emailOn":  func() template.HTML { return "<!--/email_off-->" },
}).Parse(`<!doctype html>
<html lang="{{.Lang}}" dir="{{.Dir}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="color-scheme" content="light dark">
<title>{{.Title}}</title>
{{- range .Entry.Stylesheets}}
<link rel="stylesheet" href="{{.}}">
{{- end}}
{{- with .ThemeStylesheet}}
<link rel="stylesheet" href="{{.}}">
{{- end}}
{{- range .Entry.Preloads}}
<link rel="modulepreload" href="{{.}}">
{{- end}}
{{- with .Entry.ViteClient}}
<script type="module" src="{{.}}"></script>
{{- end}}
{{- with .Entry.Script}}
<script type="module" src="{{.}}"></script>
{{- end}}
</head>
<body>
{{emailOff}}<script type="application/json" id="auth">{{.Page}}</script>{{emailOn}}
<div id="root"></div>
</body>
</html>
`))

// render answers with the document of the screen p.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, p page) {
	ctx := r.Context()
	p.Server = h.server(ctx)
	p.Documents = h.documentLinks(r, p.L)
	p.StatusPageURL = h.o.StatusPageURL
	d := shellData{Lang: p.Lang, Dir: p.Dir, Title: p.Server.Name, Entry: h.o.Entry, Page: p}
	if h.o.ThemeStylesheet != nil {
		d.ThemeStylesheet = h.o.ThemeStylesheet(ctx)
	}
	var body bytes.Buffer
	if err := shellTemplate.Execute(&body, d); err != nil {
		h.o.Logger.ErrorContext(ctx, "render auth page", slog.String("screen", p.Screen), slog.Any("error", err))
		h.o.Errors.Report(ctx, kernel.NewErrorEventSkip(err, "", 1))
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Language", p.Lang)
	var widget kernel.HumanCheckCSP
	if p.HumanCheck != nil {
		widget = h.humanCheckCSP()
	}
	hd.Set("Content-Security-Policy", contentSecurityPolicy(cspSources{
		formAction: append(slices.Clone(h.o.FormActionOrigins), p.formAction...),
		widget:     widget,
		media:      h.o.MediaOrigin,
		dev:        h.o.Entry.DevOrigin,
	}))
	hd.Set("Cache-Control", "no-store")
	hd.Add("Vary", "Accept-Language, Cookie")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

// server reads what the screens show of the server.
func (h *Handler) server(ctx context.Context) server {
	var s server
	if h.o.InstanceName != nil {
		s.Name = h.o.InstanceName(ctx)
	}
	if h.o.Logos != nil {
		s.Logo, s.LogoDark = h.o.Logos(ctx)
	}
	if h.o.InstanceDescription != nil {
		s.Description = strings.TrimSpace(h.o.InstanceDescription(ctx))
	}
	if h.o.Banner != nil {
		s.Banner = h.o.Banner(ctx)
	}
	return s
}

// cspSources are what a screen's policy allows besides this site.
type cspSources struct {
	// formAction lists the origins a form may end up at besides this site: browsers check
	// form-action on the redirects that follow a form.
	formAction []string
	// widget are the CAPTCHA service's sources, on a screen that shows its widget.
	widget kernel.HumanCheckCSP
	// media is the origin of the media files (the server's banner), "" for this site.
	media string
	// dev is the origin of a Vite dev server, whose scripts, styles, images, and hot module
	// replacement the screen loads in development, "" in production.
	dev string
}

// contentSecurityPolicy is the policy of a screen (ADR-0028 §4): the entry's scripts and
// stylesheets from this site, its images from this site and the media origin, and nothing else
// but the CAPTCHA service on a screen with its widget. It has no nonce: the data block is not
// executed, and the entry sets no inline style.
func contentSecurityPolicy(s cspSources) string {
	own := func(more ...string) []string {
		out := []string{"'self'"}
		for _, m := range more {
			if m != "" && !slices.Contains(out, m) {
				out = append(out, m)
			}
		}
		return out
	}
	script := own(append([]string{s.dev}, s.widget.ScriptSrc...)...)
	style := own(append([]string{s.dev}, s.widget.StyleSrc...)...)
	var connect []string
	if s.dev != "" {
		// The dev server's client injects <style> elements and talks to it over a WebSocket.
		style = slices.Insert(style, 1, "'unsafe-inline'")
		connect = []string{"'self'", s.dev, websocketOrigin(s.dev)}
	}
	connect = append(connect, s.widget.ConnectSrc...)
	directives := []string{
		"default-src 'none'",
		"script-src " + strings.Join(script, " "),
		"style-src " + strings.Join(style, " "),
		"img-src " + strings.Join(own(s.media, s.dev), " "),
		"font-src " + strings.Join(own(s.dev), " "),
	}
	if len(connect) > 0 {
		directives = append(directives, "connect-src "+strings.Join(connect, " "))
	}
	if len(s.widget.FrameSrc) > 0 {
		directives = append(directives, "frame-src "+strings.Join(s.widget.FrameSrc, " "))
	}
	directives = append(directives,
		"form-action "+strings.Join(own(s.formAction...), " "),
		"frame-ancestors 'none'", "base-uri 'none'", "object-src 'none'")
	return strings.Join(directives, "; ")
}

// websocketOrigin is the ws: or wss: origin of an http: or https: origin.
func websocketOrigin(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	if u.Scheme == "https" {
		return "wss://" + u.Host
	}
	return "ws://" + u.Host
}
