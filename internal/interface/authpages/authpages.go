// SPDX-License-Identifier: AGPL-3.0-only

// Package authpages serves the sign-in, sign-out, registration, email verification, password
// reset (password_reset.go), email unsubscribe (mail_unsubscribe.go), and OAuth authorization
// (consent) screens (ADR-0090 §16). Each screen is a document of the web client's auth entry
// (ADR-0028 §4): a full page load of a shell (page.go) that carries the screen's data in a JSON
// data block, under a Content-Security-Policy of its own, and that the entry's React renders.
// The forms post here, and every answer is again a document of the entry or a redirect. The
// screens' own wording is the entry's; this package words what it decides: the outcome of a form,
// and what it reads of the server (the consent's scopes, a moderation action). With a CAPTCHA
// (Options.HumanCheck), the sign-in, registration, and password reset screens also load the
// service's widget (humancheck.go), and with passkeys the sign-in screen asks the browser for one
// (passkey.go).
//
// A sign-in or sign-up starts a browser session carried by the __Host-my_session cookie
// (Secure, HttpOnly, SameSite=Lax, Path=/) with a new token every time (ADR-0019 §4). Every POST
// but the one-click unsubscribe passes net/http.CrossOriginProtection and must carry the
// anti-forgery token of the __Host-my_csrf cookie in its csrf_token field, for browsers that
// send neither Sec-Fetch-Site nor Origin (ADR-0019 §4). Failed sign-ins answer with one message whether or not the account
// exists. A completed sign-in goes on to where it was asked from, or to the web client's own
// sign-in (PathAppSignIn), which trades the session for the client's tokens.
//
// A signed-in user whose account cannot be used yet (a registration waiting for approval or
// rejected, an email address waiting for confirmation, or moderation actions that keep the
// account out of the app, identity.Access) sees a screen that says so, and lists those actions
// with a form to appeal each, instead of continuing to where they were going.
//
// The authorization endpoint (RFC 6749 §3.1) asks a signed-in user to consent to a third-party
// client and answers the client by a 303 redirect to its registered redirect URI with the code
// or the error, the state, and the issuer (RFC 9207). First-party clients skip the consent.
// Without a session, it sends the browser to the sign-in form and back; a user whose account
// cannot be used yet gets the status screen and no code.
package authpages

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// Paths of the screens.
const (
	PathSignIn = appidentity.PathSignIn
	// PathSignInTwoFactor is the second step of a sign-in, which asks a user with two-factor
	// authentication for a code (twofactor.go).
	PathSignInTwoFactor = "/auth/sign_in/two_factor"
	// PathSignInPasskey takes the sign-in screen's passkey form (passkey.go).
	PathSignInPasskey = "/auth/sign_in/passkey" //nolint:gosec // G101: a path, not a credential
	PathSignOut       = "/auth/sign_out"
	PathSignUp        = "/auth/sign_up"
	// PathAppSignIn is the web client's own sign-in (ADR-0090 §16): it asks the authorization
	// endpoint for a code with PKCE, which a browser with a session gets at once, and trades it for
	// the client's tokens. A sign-in that was not asked from anywhere goes on there.
	PathAppSignIn = "/login"
	// PathWelcome starts the web client's sign-in, which comes back to its welcome step after
	// sign-up: a new user who can use the account goes there.
	PathWelcome = PathAppSignIn + "?return_to=%2Fwelcome"
	// PathVerifyEmail is the page that verification emails link to.
	PathVerifyEmail = appidentity.PathVerifyEmail
	// PathResendVerification sends a signed-in user a new verification email.
	PathResendVerification = "/auth/verify_email/resend"
	// PathAppeal takes the status screen's appeals of the moderation actions that keep the user
	// out of the app (moderation.go).
	PathAppeal = "/auth/appeal"
	// PathReactivate brings back the deactivated account of the signed-in user (ADR-0089 §14).
	PathReactivate = "/auth/reactivate"
	// PathDeleteDeactivated deletes the deactivated account of the signed-in user.
	PathDeleteDeactivated = "/auth/deactivated/delete"
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
	// LoginProvidersOnly reports whether registration and sign-in are limited to the login
	// providers right now, read on every request.
	LoginProvidersOnly(ctx context.Context) (bool, error)
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

// Accounts names the local account of a signed-in user, which the consent screen shows as the
// account that grants the application (authorize.go).
type Accounts interface {
	SignedInAccount(ctx context.Context, userID string) (SignedInAccount, error)
}

// SignedInAccount is the local account of a signed-in user.
type SignedInAccount struct {
	Username string
	// DisplayName is the account's display name, "" when it has none.
	DisplayName string
}

// Options configure a Handler.
type Options struct {
	Identity Identity
	// OAuth serves the authorization endpoint; nil leaves it out.
	OAuth OAuth
	// Accounts names the account that the consent screen grants; nil names none.
	Accounts Accounts
	// AccountDomain is the domain of local handles (@alice@{AccountDomain}).
	AccountDomain string
	// Deactivation reactivates and deletes a deactivated account from the status screen; nil leaves
	// them out.
	Deactivation Deactivation
	// Issuer is the authorization server's identifier, sent as the iss parameter of the
	// authorization response (RFC 9207), e.g. https://social.example.
	Issuer string
	// FormActionOrigins are the origins besides this site that a sign-in may be redirected to,
	// such as the web client's origin for its callback.
	FormActionOrigins []string
	// Locales resolves the locale of the screens (ADR-0009 §2).
	Locales *i18n.Resolver
	// Entry is the web client's auth entry, which every screen loads (web.Handler.AuthEntry);
	// without its script a screen is a blank document.
	Entry Entry
	// MediaOrigin is the origin of the media files' URLs when they are served from a CDN
	// (media_origin), from which the server's banner may come; empty when they are on this site.
	MediaOrigin string
	// InstanceName returns the name of the instance: its display name, or its domain when it has
	// none. The screens show it beside the server's logo and in their title; nil leaves it out.
	InstanceName func(ctx context.Context) string
	// InstanceDescription returns the instance's one-paragraph summary as plain text, and Banner
	// the URL of its banner image, which the screens show in the server's introduction; nil or ""
	// shows none.
	InstanceDescription func(ctx context.Context) string
	Banner              func(ctx context.Context) string
	// ThemeStylesheet returns the URL of the instance theme's stylesheet, which the screens load
	// after the entry's (ADR-0059), or "" when the theme changes nothing; nil loads none.
	ThemeStylesheet func(ctx context.Context) string
	// Logos returns the paths of the instance's own logo and dark logo (ADR-0059), which the
	// screens show in place of the built-in wordmark; "" for one that is unset. nil shows the
	// built-in wordmark.
	Logos func(ctx context.Context) (logo, logoDark string)
	// StatusPageURL is the address of the instance's status page, which the footer of every screen
	// links to (ADR-0088 §20); empty links to none. It is a setting, not read from the database, so
	// the error screens of a database outage link to it too.
	StatusPageURL string
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
	// Errors receives the errors behind a 500 answer (ADR-0103 §2); nil reports nothing.
	Errors kernel.ErrorReporter
	// Clock tells the time the status screen counts the days left of an action from, the clock
	// that dated the action; nil is the system clock.
	Clock kernel.Clock
	// Moderation lists the actions that keep a signed-in user out of the app on the status
	// screen; nil lists none. UserLocales words the status screen in the user's saved locale; nil
	// words it in the request's.
	Moderation  Moderation
	UserLocales UserLocales
	// Documents tells which of the terms of service and the privacy policy are published, for the links to
	// them (documents.go); nil links to none.
	Documents Documents
	// HumanCheck is the CAPTCHA widget of the sign-in and registration forms (humancheck.go);
	// nil, as with the captcha setting none, renders none.
	HumanCheck *kernel.HumanCheckWidget
	// Passkeys signs in with passkeys (passkey.go); nil leaves them out of the sign-in screen.
	Passkeys Passkeys
	// PasswordReset serves the screens that reset a forgotten password (password_reset.go), which
	// the sign-in screen links to; nil leaves them out.
	PasswordReset PasswordReset
	// MailUnsubscribe serves the unsubscribe links of notification emails (mail_unsubscribe.go);
	// nil leaves them out.
	MailUnsubscribe MailUnsubscribe
	// ExternalLogins signs in and signs up with the external login providers that
	// administrators set up (external.go); nil leaves them out.
	ExternalLogins ExternalLogins
}

// Handler serves the screens.
type Handler struct {
	o   Options
	cop *http.CrossOriginProtection
}

// New returns a Handler on o.
func New(o Options) *Handler {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Errors == nil {
		o.Errors = kernel.NopErrorReporter{}
	}
	if o.Clock == nil {
		o.Clock = systemClock{}
	}
	if o.Entry.Script == "" {
		o.Logger.Warn("the web client's auth entry is not built into this binary; the sign-in screens are blank")
	}
	h := &Handler{o: o, cop: http.NewCrossOriginProtection()}
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
	if h.o.Deactivation != nil {
		r.Handle("POST "+PathReactivate, h.cop.Handler(http.HandlerFunc(h.reactivate)))
		r.Handle("POST "+PathDeleteDeactivated, h.cop.Handler(http.HandlerFunc(h.deleteDeactivated)))
	}
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
	if h.o.ExternalLogins != nil {
		// The provider's redirect back is a GET from its site, which the state checks (external.go).
		r.Handle("GET "+pathExternalStart, http.HandlerFunc(h.externalStart))
		r.Handle("GET "+pathExternalCallback, http.HandlerFunc(h.externalCallback))
		r.Handle("POST "+PathExternalSignUp, h.cop.Handler(http.HandlerFunc(h.externalSignUp)))
	}
	if h.o.OAuth != nil {
		r.Handle("GET "+PathAuthorize, http.HandlerFunc(h.authorizePage))
		r.Handle("POST "+PathAuthorize, h.cop.Handler(http.HandlerFunc(h.authorize)))
	}
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int, message func(*i18n.Localizer) string) {
	p := h.newPage(r, screenError)
	p.Error = message(p.L)
	h.render(w, r, status, p)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.o.Logger.ErrorContext(r.Context(), "auth page failed", slog.Any("error", err))
	h.o.Errors.Report(r.Context(), kernel.NewErrorEventSkip(err, "", 1)) // the reporter names the route; the stack starts with the caller
	h.renderError(w, r, http.StatusInternalServerError, (*i18n.Localizer).ServerErrorsInternal)
}

// crossOrigin answers a POST that CrossOriginProtection rejected.
func (h *Handler) crossOrigin(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthCrossOrigin)
}

// signInPage shows the sign-in form; when the browser has a session, it shows the signed-in
// screen, or the status screen of an account that cannot be used yet.
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
	p := h.newPage(r, screenSignedIn)
	p.CSRFToken = h.csrfToken(w, r)
	p.ReturnTo = returnTo
	if p.ReturnTo == "" {
		p.ReturnTo = PathAppSignIn
	}
	h.render(w, r, http.StatusOK, p)
}

func (h *Handler) renderSignIn(w http.ResponseWriter, r *http.Request, status int, returnTo, email string, message func(*i18n.Localizer) string) {
	h.renderSignInPage(w, r, status, returnTo, email, message, nil)
}

// renderSignInPage renders the sign-in form with the alert message about the password form or
// passkeyMessage about the passkey form.
func (h *Handler) renderSignInPage(w http.ResponseWriter, r *http.Request, status int, returnTo, email string, message, passkeyMessage func(*i18n.Localizer) string) {
	p := h.newPage(r, screenSignIn)
	p.CSRFToken = h.csrfToken(w, r)
	p.ReturnTo, p.Email = returnTo, email
	if message != nil {
		p.Error = message(p.L)
	}
	limited, err := h.o.Identity.LoginProvidersOnly(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	p.LoginProvidersOnly = limited
	if limited {
		// The screen shows no form that the server would refuse, only what signs in.
		p.Email = ""
		if p.Error == "" && passkeyMessage != nil {
			p.Error = passkeyMessage(p.L)
		}
	} else {
		p.Passkey = h.passkeyView(r)
		if p.Passkey != nil && passkeyMessage != nil {
			p.Passkey.Error = passkeyMessage(p.L)
		}
		p.CanResetPassword = h.passwordResetEnabled()
		p.HumanCheck = h.humanCheck(w, r, p.L, kernel.HumanCheckActionSignIn)
	}
	mode, err := h.o.Identity.RegistrationMode(r.Context())
	if err != nil {
		h.o.Logger.WarnContext(r.Context(), "read the registration mode", slog.Any("error", err))
	}
	p.CanSignUp = err == nil && mode != identity.RegistrationClosed
	p.External = h.externalButtons(r, returnTo, false)
	h.render(w, r, status, p)
}

// refuseLoginProvidersOnly answers a sign-in with a password or a passkey that the limit to the
// login providers refuses: the sign-in screen with the same message whatever the form holds, so
// that it tells nothing about any account.
func (h *Handler) refuseLoginProvidersOnly(w http.ResponseWriter, r *http.Request, returnTo string) {
	h.renderSignIn(w, r, http.StatusForbidden, returnTo, "", (*i18n.Localizer).ServerAuthSignInLoginProvidersOnly)
}

// loginProvidersOnly reports whether the limit to the login providers applies; it answers the
// request when the read fails, and then returns false and false.
func (h *Handler) loginProvidersOnly(w http.ResponseWriter, r *http.Request) (limited, ok bool) {
	limited, err := h.o.Identity.LoginProvidersOnly(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return false, false
	}
	return limited, true
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
	// The limit comes before anything about the form, so that the answer does not depend on it.
	limited, ok := h.loginProvidersOnly(w, r)
	if !ok {
		return
	}
	if limited {
		h.refuseLoginProvidersOnly(w, r, returnTo)
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
	var rateLimited *kernel.RateLimitedError
	var secondFactor *appidentity.SecondFactorRequiredError
	switch {
	case errors.Is(err, appidentity.ErrLoginProvidersOnly):
		h.refuseLoginProvidersOnly(w, r, returnTo)
		return
	case errors.As(err, &rateLimited):
		h.renderSignIn(w, r, http.StatusTooManyRequests, returnTo, email, rateLimitedMessage(w, rateLimited))
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

// signedIn gives the browser the session of a completed sign-in and sends it on to returnTo, else
// to the web client's own sign-in, or to the screen that says why the account cannot be used yet.
func (h *Handler) signedIn(w http.ResponseWriter, r *http.Request, signedIn appidentity.SignedIn, returnTo string) {
	setSessionCookie(w, signedIn)
	access, err := h.o.Identity.UserAccess(r.Context(), signedIn.Session.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	switch {
	case !access.Allowed():
		returnTo = PathSignIn // the screen that says why the account cannot be used yet
	case returnTo == "":
		returnTo = PathAppSignIn
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
