// SPDX-License-Identifier: AGPL-3.0-only

// Package authpages serves the server-rendered sign-in, sign-out, registration, email
// verification, password reset (password_reset.go), email unsubscribe (mail_unsubscribe.go), and
// OAuth authorization (consent) screens (ADR-0030): plain HTML forms from html/template that
// work without JavaScript, worded from the server message catalog and styled by one stylesheet
// that the web client's build provides. With
// a CAPTCHA (Options.HumanCheck), the sign-in, registration, and password reset forms also load
// the service's widget (humancheck.go). In a browser with passkeys, the web build's script adds
// a passkey button to the sign-in form (passkey.go); without JavaScript the page stays the
// password form.
//
// A sign-in or sign-up starts a browser session carried by the __Host-my_session cookie
// (Secure, HttpOnly, SameSite=Lax, Path=/) with a new token every time (ADR-0019 §4). Every POST
// but the one-click unsubscribe passes net/http.CrossOriginProtection and must carry the
// anti-forgery token of the __Host-my_csrf cookie in its csrf_token field, for browsers that
// send neither Sec-Fetch-Site nor Origin (ADR-0019 §4). Failed sign-ins answer with one message whether or not the account
// exists.
//
// A signed-in user whose account cannot be used yet (a registration waiting for approval or
// rejected, an email address waiting for confirmation, or moderation actions that keep the
// account out of the app, identity.Access) sees a page that says so, and lists those actions
// with a form to appeal each, instead of continuing to where they were going.
//
// The authorization endpoint (RFC 6749 §3.1) asks a signed-in user to consent to a third-party
// client and answers the client by a 303 redirect to its registered redirect URI with the code
// or the error, the state, and the issuer (RFC 9207). First-party clients skip the consent.
// Without a session, it sends the browser to the sign-in form and back; a user whose account
// cannot be used yet gets the status page and no code.
package authpages

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"errors"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// Paths of the screens.
const (
	PathSignIn = "/auth/sign_in"
	// PathSignInTwoFactor is the second step of a sign-in, which asks a user with two-factor
	// authentication for a code (twofactor.go).
	PathSignInTwoFactor = "/auth/sign_in/two_factor"
	// PathSignInPasskey takes the sign-in page's passkey form (passkey.go).
	PathSignInPasskey = "/auth/sign_in/passkey" //nolint:gosec // G101: a path, not a credential
	PathSignOut       = "/auth/sign_out"
	PathSignUp        = "/auth/sign_up"
	// PathVerifyEmail is the page that verification emails link to.
	PathVerifyEmail = appidentity.PathVerifyEmail
	// PathResendVerification sends a signed-in user a new verification email.
	PathResendVerification = "/auth/verify_email/resend"
	// PathAppeal takes the status page's appeals of the moderation actions that keep the user
	// out of the app (moderation.go).
	PathAppeal = "/auth/appeal"
	// PathAuthorize is the OAuth authorization endpoint.
	PathAuthorize = "/oauth/authorize"
)

// Cookie names. The __Host- prefix makes browsers accept the cookies only when Secure, without
// a Domain, and with Path=/, so that no other host can set or overwrite them.
const (
	CookieSession = "__Host-my_session"
	CookieCSRF    = "__Host-my_csrf"
	// CookieTwoFactor carries the challenge of a sign-in between its password and its code.
	CookieTwoFactor = "__Host-my_2fa"
)

// contentSecurityPolicy allows the pages nothing but their own stylesheet, images, and forms.
// Browsers check form-action on the redirects that follow a form, so it lists the origins a
// form may end up at: this site, Options.FormActionOrigins, and the consent page's redirect URI.
// A page with the CAPTCHA widget adds the service's sources in widget (humancheck.go); the
// others pass none. A page that loads the web build's script (scripts) allows this site's
// scripts.
func contentSecurityPolicy(formAction []string, widget kernel.HumanCheckCSP, scripts bool) string {
	var b strings.Builder
	b.WriteString("default-src 'none'")
	scriptSrc := slices.Clone(widget.ScriptSrc)
	if scripts {
		scriptSrc = append([]string{"'self'"}, scriptSrc...)
	}
	if len(scriptSrc) > 0 {
		b.WriteString("; script-src " + strings.Join(scriptSrc, " "))
	}
	if len(widget.FrameSrc) > 0 {
		b.WriteString("; frame-src " + strings.Join(widget.FrameSrc, " "))
	}
	if len(widget.ConnectSrc) > 0 {
		b.WriteString("; connect-src " + strings.Join(widget.ConnectSrc, " "))
	}
	b.WriteString("; style-src " + strings.Join(append([]string{"'self'"}, widget.StyleSrc...), " "))
	b.WriteString("; img-src 'self'; form-action " + strings.Join(append([]string{"'self'"}, formAction...), " "))
	b.WriteString("; frame-ancestors 'none'; base-uri 'none'")
	return b.String()
}

// maxFormBytes bounds a form body; the largest are the registration form, whose reason field
// holds up to identity.MaxRegistrationReasonLength characters, and the appeal form, whose text
// holds up to moderation.MaxAppealText.
const maxFormBytes = 32 << 10

// maxReturnTo bounds the return_to path.
const maxReturnTo = 2048

// Identity is what the screens need from the identity module; *identity.Service implements it.
type Identity interface {
	SignIn(ctx context.Context, in appidentity.SignInInput) (appidentity.SignedIn, error)
	CompleteSignIn(ctx context.Context, in appidentity.SecondFactorInput) (appidentity.SignedIn, error)
	Authenticate(ctx context.Context, token string) (identity.Session, error)
	SignOut(ctx context.Context, token string) error
	SignUp(ctx context.Context, in appidentity.SignUpInput) (appidentity.SignedUp, error)
	RegistrationMode(ctx context.Context) (identity.RegistrationMode, error)
	UserAccess(ctx context.Context, userID string) (appidentity.Access, error)
	VerifyEmail(ctx context.Context, token string) (string, error)
	ResendVerificationEmail(ctx context.Context, userID string) error
}

// OAuth is what the authorization endpoint needs from the identity module; *identity.Service
// implements it.
type OAuth interface {
	ValidateAuthorization(ctx context.Context, req appidentity.AuthorizationRequest) (appidentity.Authorization, error)
	IssueAuthorizationCode(ctx context.Context, a appidentity.Authorization, userID string) (string, error)
}

// Options configure a Handler.
type Options struct {
	Identity Identity
	// OAuth serves the authorization endpoint; nil leaves it out.
	OAuth OAuth
	// Issuer is the authorization server's identifier, sent as the iss parameter of the
	// authorization response (RFC 9207), e.g. https://social.example.
	Issuer string
	// FormActionOrigins are the origins besides this site that a sign-in may be redirected to,
	// such as the web client's origin for its callback.
	FormActionOrigins []string
	// Locales resolves the locale of the screens (ADR-0009 §2).
	Locales *i18n.Resolver
	// InstanceName returns the name of the instance: its display name, or
	// its domain when it has none. The pages show it above their card and in their title; nil
	// leaves it out.
	InstanceName func(ctx context.Context) string
	// Stylesheet is the URL of the pages' stylesheet, the web build's server pages entry
	// (web.Handler.Stylesheet); empty leaves the pages unstyled.
	Stylesheet string
	// ThemeStylesheet returns the URL of the instance theme's stylesheet, which the pages load
	// after Stylesheet (ADR-0059), or "" when the theme changes nothing; nil loads none.
	ThemeStylesheet func(ctx context.Context) string
	// StatusPageURL is the address of the instance's status page, which the footer of every page
	// links to (ADR-0085 §6); empty links to none. It is a setting, not read from the database, so
	// the error pages of a database outage link to it too.
	StatusPageURL string
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
	// Errors receives the errors behind a 500 answer (ADR-0103 §2); nil reports nothing.
	Errors kernel.ErrorReporter
	// Moderation lists the actions that keep a signed-in user out of the app on the status
	// page; nil lists none. UserLocales words the status page in the user's saved locale; nil
	// words it in the request's.
	Moderation  Moderation
	UserLocales UserLocales
	// Documents tells which of the terms of service and the privacy policy are published, for the links to
	// them (documents.go); nil links to none.
	Documents Documents
	// HumanCheck is the CAPTCHA widget of the sign-in and registration forms (humancheck.go);
	// nil, as with the captcha setting none, renders none.
	HumanCheck *kernel.HumanCheckWidget
	// Passkeys signs in with passkeys (passkey.go), and PasskeyScript is the URL of the web
	// build's script that runs the browser's side; the sign-in page offers passkeys only with
	// both.
	Passkeys      Passkeys
	PasskeyScript string
	// PasswordReset serves the pages that reset a forgotten password (password_reset.go), which
	// the sign-in page links to; nil leaves them out.
	PasswordReset PasswordReset
	// MailUnsubscribe serves the unsubscribe links of notification emails (mail_unsubscribe.go);
	// nil leaves them out.
	MailUnsubscribe MailUnsubscribe
}

// Handler serves the screens.
type Handler struct {
	o     Options
	pages map[string]*template.Template
	cop   *http.CrossOriginProtection
}

//go:embed templates/*.html
var templateFS embed.FS

// Page templates, each rendered inside templates/layout.html.
const (
	pageSignIn      = "sign_in"
	pageTwoFactor   = "two_factor"
	pageSignedIn    = "signed_in"
	pageError       = "error"
	pageSignUp      = "sign_up"
	pageStatus      = "status"
	pageVerifyEmail = "verify_email"
	pageVerified    = "verified"
	pageAuthorize   = "authorize"
	// The pages of password_reset.go.
	pageForgotPassword = "forgot_password"
	pageResetPassword  = "reset_password"
	pagePasswordReset  = "password_reset"
	// The pages of mail_unsubscribe.go.
	pageMailUnsubscribe  = "mail_unsubscribe"
	pageMailUnsubscribed = "mail_unsubscribed"
)

// New returns a Handler on o.
func New(o Options) *Handler {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Errors == nil {
		o.Errors = kernel.NopErrorReporter{}
	}
	h := &Handler{o: o, pages: map[string]*template.Template{}, cop: http.NewCrossOriginProtection()}
	for _, name := range []string{pageSignIn, pageTwoFactor, pageSignedIn, pageError, pageSignUp, pageStatus, pageVerifyEmail, pageVerified, pageAuthorize,
		pageForgotPassword, pageResetPassword, pagePasswordReset, pageMailUnsubscribe, pageMailUnsubscribed} {
		h.pages[name] = template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/human_check.html", "templates/"+name+".html"))
	}
	h.cop.SetDenyHandler(http.HandlerFunc(h.crossOrigin))
	return h
}

// Router registers handlers for http.ServeMux patterns.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Register registers the screens on r.
func (h *Handler) Register(r Router) {
	r.Handle("GET "+PathSignIn, http.HandlerFunc(h.signInPage))
	r.Handle("POST "+PathSignIn, h.cop.Handler(http.HandlerFunc(h.signIn)))
	r.Handle("GET "+PathSignInTwoFactor, http.HandlerFunc(h.twoFactorPage))
	r.Handle("POST "+PathSignInTwoFactor, h.cop.Handler(http.HandlerFunc(h.twoFactor)))
	if h.passkeysEnabled() {
		r.Handle("POST "+PathSignInPasskey, h.cop.Handler(http.HandlerFunc(h.signInWithPasskey)))
	}
	r.Handle("POST "+PathSignOut, h.cop.Handler(http.HandlerFunc(h.signOut)))
	r.Handle("GET "+PathSignUp, http.HandlerFunc(h.signUpPage))
	r.Handle("POST "+PathSignUp, h.cop.Handler(http.HandlerFunc(h.signUp)))
	r.Handle("GET "+PathVerifyEmail, http.HandlerFunc(h.verifyEmailPage))
	r.Handle("POST "+PathVerifyEmail, h.cop.Handler(http.HandlerFunc(h.verifyEmail)))
	r.Handle("POST "+PathResendVerification, h.cop.Handler(http.HandlerFunc(h.resendVerification)))
	r.Handle("POST "+PathAppeal, h.cop.Handler(http.HandlerFunc(h.appeal)))
	if h.passwordResetEnabled() {
		r.Handle("GET "+PathForgotPassword, http.HandlerFunc(h.forgotPasswordPage))
		r.Handle("POST "+PathForgotPassword, h.cop.Handler(http.HandlerFunc(h.forgotPassword)))
		r.Handle("GET "+PathResetPassword, http.HandlerFunc(h.resetPasswordPage))
		r.Handle("POST "+PathResetPassword, h.cop.Handler(http.HandlerFunc(h.resetPassword)))
	}
	if h.o.MailUnsubscribe != nil {
		// Mail providers send the one-click POST from their servers (mail_unsubscribe.go).
		r.Handle("GET "+PathMailUnsubscribe, http.HandlerFunc(h.mailUnsubscribePage))
		r.Handle("POST "+PathMailUnsubscribe, http.HandlerFunc(h.mailUnsubscribe))
	}
	if h.o.OAuth != nil {
		r.Handle("GET "+PathAuthorize, http.HandlerFunc(h.authorizePage))
		r.Handle("POST "+PathAuthorize, h.cop.Handler(http.HandlerFunc(h.authorize)))
	}
}

// page is the data of a template.
type page struct {
	L         *i18n.Localizer
	Lang, Dir string
	// Title is the page's title; DocumentTitle adds the instance's name to it.
	Title string
	// InstanceName names the instance in the page's locale; "" leaves it out.
	InstanceName string
	Stylesheet   string
	// ThemeStylesheet is the instance theme's stylesheet; render fills it.
	ThemeStylesheet string
	CSRFToken       string
	ReturnTo        string
	Email           string
	// Error is an alert about the whole form; Notice reports success.
	Error  string
	Notice string

	// CanSignUp shows the link to the registration page, and CanResetPassword the link to the
	// page that asks for a password reset link.
	CanSignUp        bool
	CanResetPassword bool
	// SignUp is the registration form.
	SignUp *signUpForm
	// Status is the state of an account that cannot be used yet.
	Status *statusView
	// Token is the email verification token of the confirmation form, or the password reset
	// token of the form of a new password.
	Token string
	// ForgotPassword is the page that asks for a password reset link, and NewPassword the field
	// of the page that sets the new password (password_reset.go).
	ForgotPassword *forgotPasswordView
	NewPassword    *newPasswordView
	// Unsubscribe is the page of an unsubscribe link (mail_unsubscribe.go).
	Unsubscribe *unsubscribeView
	// Consent is the authorization request on the consent page.
	Consent *consent
	// HumanCheck is the CAPTCHA widget of the form; the page's Content-Security-Policy admits
	// the service when it is set.
	HumanCheck *humanCheckView
	// Passkey is the passkey form of the sign-in page; the page's Content-Security-Policy admits
	// this site's scripts when it is set.
	Passkey *passkeyView
	// Documents are the links to the published documents, in the footer of every page and on
	// the registration form; render fills them.
	Documents []documentLink
	// StatusPageURL is the instance's status page, in the footer of every page; render fills it.
	StatusPageURL string
	// formAction lists origins the page's forms may be redirected to besides
	// Options.FormActionOrigins.
	formAction []string
}

// DocumentTitle is the <title> of the page: its title and the instance's name.
func (p page) DocumentTitle() string {
	if p.InstanceName == "" {
		return p.Title
	}
	return p.L.ServerAuthDocumentTitle(p.InstanceName, p.Title)
}

func (h *Handler) newPage(r *http.Request, title func(*i18n.Localizer) string) page {
	l := h.o.Locales.RequestLocalizer(r)
	return page{L: l, Lang: l.Locale(), Dir: l.Direction(), Title: title(l), Stylesheet: h.o.Stylesheet}
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, name string, p page) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Language", p.Lang)
	var widget kernel.HumanCheckCSP
	if p.HumanCheck != nil {
		widget = h.humanCheckCSP()
	}
	hd.Set("Content-Security-Policy", contentSecurityPolicy(append(slices.Clone(h.o.FormActionOrigins), p.formAction...), widget, p.Passkey != nil))
	hd.Set("Cache-Control", "no-store")
	hd.Add("Vary", "Accept-Language, Cookie")
	if h.o.InstanceName != nil {
		p.InstanceName = h.o.InstanceName(r.Context())
	}
	p.Documents = h.documentLinks(r, p.L)
	p.StatusPageURL = h.o.StatusPageURL
	if h.o.ThemeStylesheet != nil {
		p.ThemeStylesheet = h.o.ThemeStylesheet(r.Context())
	}
	w.WriteHeader(status)
	if err := h.pages[name].ExecuteTemplate(w, "layout", p); err != nil {
		h.o.Logger.ErrorContext(r.Context(), "render auth page", slog.String("page", name), slog.Any("error", err))
	}
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int, message func(*i18n.Localizer) string) {
	p := h.newPage(r, (*i18n.Localizer).ServerAuthErrorTitle)
	p.Error = message(p.L)
	h.render(w, r, status, pageError, p)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.o.Logger.ErrorContext(r.Context(), "auth page failed", slog.Any("error", err))
	h.o.Errors.Report(r.Context(), kernel.NewErrorEvent(err, "")) // the reporter names the route
	h.renderError(w, r, http.StatusInternalServerError, (*i18n.Localizer).ServerErrorsInternal)
}

// crossOrigin answers a POST that CrossOriginProtection rejected.
func (h *Handler) crossOrigin(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthCrossOrigin)
}

// signInPage shows the sign-in form; when the browser has a session, it shows the signed-in
// page, or the status page of an account that cannot be used yet.
func (h *Handler) signInPage(w http.ResponseWriter, r *http.Request) {
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	session, err := h.o.Identity.Authenticate(r.Context(), cookieValue(r, CookieSession))
	switch {
	case err == nil:
		access, err := h.o.Identity.UserAccess(r.Context(), session.UserID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if !access.Allowed() {
			h.renderStatus(w, r, http.StatusOK, session.UserID, access, nil, nil)
			return
		}
		h.renderSignedIn(w, r, returnTo)
	case errors.Is(err, appidentity.ErrInvalidSession):
		h.renderSignIn(w, r, http.StatusOK, returnTo, "", nil)
	default:
		h.internalError(w, r, err)
	}
}

func (h *Handler) renderSignedIn(w http.ResponseWriter, r *http.Request, returnTo string) {
	p := h.newPage(r, (*i18n.Localizer).ServerAuthSignedInTitle)
	p.CSRFToken = h.csrfToken(w, r)
	p.ReturnTo = returnTo
	if p.ReturnTo == "" {
		p.ReturnTo = "/"
	}
	h.render(w, r, http.StatusOK, pageSignedIn, p)
}

func (h *Handler) renderSignIn(w http.ResponseWriter, r *http.Request, status int, returnTo, email string, message func(*i18n.Localizer) string) {
	h.renderSignInPage(w, r, status, returnTo, email, message, nil)
}

// renderSignInPage renders the sign-in form with the alert message about the password form or
// passkeyMessage about the passkey form.
func (h *Handler) renderSignInPage(w http.ResponseWriter, r *http.Request, status int, returnTo, email string, message, passkeyMessage func(*i18n.Localizer) string) {
	p := h.newPage(r, (*i18n.Localizer).ServerAuthSignInTitle)
	p.CSRFToken = h.csrfToken(w, r)
	p.ReturnTo, p.Email = returnTo, email
	if message != nil {
		p.Error = message(p.L)
	}
	p.Passkey = h.passkeyView(r)
	if p.Passkey != nil && passkeyMessage != nil {
		p.Passkey.Error = passkeyMessage(p.L)
	}
	mode, err := h.o.Identity.RegistrationMode(r.Context())
	if err != nil {
		h.o.Logger.WarnContext(r.Context(), "read the registration mode", slog.Any("error", err))
	}
	p.CanSignUp = err == nil && mode != identity.RegistrationClosed
	p.CanResetPassword = h.passwordResetEnabled()
	p.HumanCheck = h.humanCheck(w, r, p.L, kernel.HumanCheckActionSignIn)
	h.render(w, r, status, pageSignIn, p)
}

// signIn handles the sign-in form.
func (h *Handler) signIn(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	returnTo := safeReturnTo(r.PostForm.Get("return_to"))
	email := strings.TrimSpace(r.PostForm.Get("email"))
	if !h.validCSRF(r) {
		h.renderSignIn(w, r, http.StatusForbidden, returnTo, email, (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	password := r.PostForm.Get("password")
	if email == "" || password == "" {
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, email, (*i18n.Localizer).ServerAuthSignInMissing)
		return
	}
	signedIn, err := h.o.Identity.SignIn(r.Context(), appidentity.SignInInput{
		Email:           email,
		Password:        password,
		IP:              kernel.RequestClientIP(r),
		UserAgent:       r.UserAgent(),
		CurrentToken:    cookieValue(r, CookieSession),
		HumanCheckToken: h.humanCheckToken(r),
	})
	var limited *kernel.RateLimitedError
	var secondFactor *appidentity.SecondFactorRequiredError
	switch {
	case errors.As(err, &limited):
		h.renderSignIn(w, r, http.StatusTooManyRequests, returnTo, email, rateLimitedMessage(w, limited))
		return
	case errors.As(err, &secondFactor):
		h.askSecondFactor(w, r, secondFactor.Challenge, returnTo)
		return
	case errors.Is(err, appidentity.ErrInvalidCredentials):
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, email, (*i18n.Localizer).ServerAuthSignInFailed)
		return
	case errors.Is(err, kernel.ErrHumanCheckFailed):
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, email, (*i18n.Localizer).ServerAuthHumanCheckFailed)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	h.signedIn(w, r, signedIn, returnTo)
}

// rateLimitedMessage sets the Retry-After of a refused sign-in and returns its message.
func rateLimitedMessage(w http.ResponseWriter, limited *kernel.RateLimitedError) func(*i18n.Localizer) string {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
	minutes := int(math.Ceil(limited.RetryAfter.Minutes()))
	return func(l *i18n.Localizer) string { return l.ServerAuthSignInRateLimited(minutes) }
}

// signedIn gives the browser the session of a completed sign-in and sends it on to returnTo,
// or to the page that says why the account cannot be used yet.
func (h *Handler) signedIn(w http.ResponseWriter, r *http.Request, signedIn appidentity.SignedIn, returnTo string) {
	setSessionCookie(w, signedIn)
	access, err := h.o.Identity.UserAccess(r.Context(), signedIn.Session.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	switch {
	case !access.Allowed():
		returnTo = PathSignIn // the page that says why the account cannot be used yet
	case returnTo == "":
		returnTo = "/"
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, returnTo, http.StatusSeeOther) //nolint:gosec // G710: safeReturnTo admits only paths on this site
}

// setSessionCookie gives the browser the session of s.
func setSessionCookie(w http.ResponseWriter, s appidentity.SignedIn) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieSession,
		Value:    s.Token,
		Path:     "/",
		Expires:  s.Session.ExpiresAt,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// signOut ends the browser's session and returns to the sign-in form.
func (h *Handler) signOut(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	if !h.validCSRF(r) {
		h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	if err := h.o.Identity.SignOut(r.Context(), cookieValue(r, CookieSession)); err != nil {
		h.internalError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: CookieSession, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
}

// csrfToken returns the browser's anti-forgery token, issuing a new one in the CSRF cookie when
// the browser has none (the double-submit pattern; the __Host- prefix keeps other hosts from
// planting the cookie).
func (h *Handler) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if t := cookieValue(r, CookieCSRF); validTokenSyntax(t) {
		return t
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	t := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name: CookieCSRF, Value: t, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	return t
}

// validCSRF reports whether the form's csrf_token matches the CSRF cookie.
func (h *Handler) validCSRF(r *http.Request) bool {
	cookie := cookieValue(r, CookieCSRF)
	form := r.PostForm.Get("csrf_token")
	return validTokenSyntax(cookie) && subtle.ConstantTimeCompare([]byte(cookie), []byte(form)) == 1
}

// validTokenSyntax accepts what csrfToken issues: 43 characters of unpadded base64url.
func validTokenSyntax(t string) bool {
	if len(t) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(t)
	return err == nil
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

// returnToBase stands for this site when safeReturnTo resolves a return_to value. Only the
// path and the query of the result are kept, so the name does not matter; a .invalid name
// (RFC 6761) can never be another site's host.
var returnToBase = &url.URL{Scheme: "https", Host: "return-to.invalid", Path: "/"}

// safeReturnTo returns the path and query on this site that s points to, for the sign-in form
// to go back to, and "" when s is not such a reference, so that the form cannot be turned into
// an open redirect (ADR-0019; OWASP Unvalidated Redirects and Forwards Cheat Sheet). Instead of
// matching prefixes of s, it parses s, resolves it against this site's origin, checks that the
// origin stays the same, and rebuilds the result from the normalized path and the query alone,
// dropping the fragment. It refuses what a browser could read differently from net/url:
// control characters and backslashes (which WHATWG URL parsers drop or read as "/"), anything
// but an absolute path, an empty path segment ("//", which a dot segment or a decoded "%2F"
// could turn into "//host"), and a percent-encoded dot segment.
func safeReturnTo(s string) string {
	if len(s) > maxReturnTo || !onSitePath(s) || strings.ContainsFunc(s, unsafeReturnToRune) {
		return ""
	}
	ref, err := url.Parse(s)
	if err != nil || ref.Scheme != "" || ref.Opaque != "" || ref.User != nil || ref.Host != "" || ref.OmitHost ||
		strings.Contains(ref.Path, "//") {
		return ""
	}
	u := returnToBase.ResolveReference(ref)
	if u.Scheme != returnToBase.Scheme || u.Host != returnToBase.Host || u.User != nil {
		return ""
	}
	path := u.EscapedPath()
	if !onSitePath(path) || strings.Contains(u.Path, "//") ||
		strings.ContainsFunc(u.Path, unsafeReturnToRune) || hasDotSegment(u.Path) {
		return ""
	}
	out := path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	// The rebuilt value must read back as a path on this site.
	if back, err := url.Parse(out); err != nil || back.Scheme != "" || back.Host != "" || back.User != nil {
		return ""
	}
	return out
}

// onSitePath reports whether p starts like an absolute path on this site: one "/", not "//"
// or "/\" (which browsers read as the start of another host).
func onSitePath(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "/\\")
}

// hasDotSegment reports whether the decoded path p still has a "." or ".." segment. Once
// ResolveReference has removed the literal ones, such a segment was percent-encoded
// ("%2E%2E"), which browsers resolve but net/url does not.
func hasDotSegment(p string) bool {
	for seg := range strings.SplitSeq(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// unsafeReturnToRune reports a character that a return_to value must not contain, raw or
// percent-decoded: a C0 or C1 control character, DEL, or a backslash.
func unsafeReturnToRune(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == '\\'
}
