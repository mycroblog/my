// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// Signing in and signing up with an external login provider that an administrator set up
// (FR-010). The sign-in page links to the start of each enabled provider, and the registration
// page to those that let new users sign up. The start sends the browser to the provider with
// the sealed state of the sign-in in the __Host-my_external cookie (Secure, HttpOnly,
// SameSite=Lax, so that the provider's redirect back carries it); the callback takes the
// provider's answer with the cookie, removes the cookie, and signs the linked user in as the
// password form does, with the code step of two-factor authentication. An account that no user
// links gets the registration form without a password, which carries the sealed sign-up in a
// hidden field.
//
// The same start and callback link an account at a provider to a signed-in user and confirm an
// operation of one: the web client's start URL carries the sealed intent, which the start takes
// only in a browser signed in as the intent's user (sending a browser without a session to the
// sign-in page first). The callback of an intent always goes back to the intent's return target
// with the outcome in the fragment, which the web client reads (and which no server sees):
// external_confirmation=<token> for a confirmation, linked=<provider ID> for a link, and
// external_error=<reason> for a failure, with the provider and the operation of a confirmation.

// Paths of the external sign-in.
const (
	// PathExternalSignUp takes the registration form of an account at a provider.
	PathExternalSignUp = "/auth/external/sign_up"
	// pathExternalStart and pathExternalCallback are the patterns of a provider's start and
	// callback (appidentity.ExternalLoginPathPrefix).
	pathExternalStart    = appidentity.ExternalLoginPathPrefix + "{id}/start"
	pathExternalCallback = appidentity.ExternalLoginPathPrefix + "{id}/callback"
)

// CookieExternal carries the sealed state of a sign-in with a provider, from its start to its
// callback.
const CookieExternal = "__Host-my_external"

// ExternalLogins is what the pages need to sign in and sign up with external login providers;
// *identity.Service implements it.
type ExternalLogins interface {
	ExternalLoginProviders(ctx context.Context) ([]appidentity.ExternalLoginProvider, error)
	BeginExternalLogin(ctx context.Context, providerID, returnTo string) (appidentity.ExternalLoginStart, error)
	BeginExternalIntent(ctx context.Context, providerID, intent, sessionUserID string) (appidentity.ExternalLoginStart, error)
	FinishExternalLogin(ctx context.Context, in appidentity.ExternalLoginInput) (appidentity.ExternalLogin, error)
	ExternalSignUp(ctx context.Context, ticket string) (appidentity.ExternalSignUpView, error)
	SignUpExternal(ctx context.Context, in appidentity.ExternalSignUpInput) (appidentity.SignedUp, error)
}

// externalButton is a link to a provider's start.
type externalButton struct {
	// Label is the button's text: an OpenID Connect provider's display name as its
	// administrator wrote it, never translated; the translated text for a github provider.
	Label string
	URL   string
}

// externalButtons returns the links to the enabled providers, those that let new users sign
// up when signUp; none without providers. A failure only leaves them out.
func (h *Handler) externalButtons(r *http.Request, returnTo string, signUp bool) []externalButton {
	if h.o.ExternalLogins == nil {
		return nil
	}
	providers, err := h.o.ExternalLogins.ExternalLoginProviders(r.Context())
	if err != nil {
		h.o.Logger.WarnContext(r.Context(), "list the external login providers", slog.Any("error", err))
		return nil
	}
	var out []externalButton
	for _, p := range providers {
		if signUp && !p.AllowSignup {
			continue
		}
		label := p.DisplayName
		if p.Kind == identity.LoginProviderGitHub {
			label = h.o.Locales.RequestLocalizer(r).ServerAuthExternalGithubButton()
		}
		out = append(out, externalButton{
			Label: label,
			URL:   withReturnTo(appidentity.ExternalLoginPathPrefix+p.ID+"/start", returnTo),
		})
	}
	return out
}

// externalStart sends the browser to the provider.
func (h *Handler) externalStart(w http.ResponseWriter, r *http.Request) {
	if intent := r.URL.Query().Get(appidentity.ExternalIntentParam); intent != "" {
		h.externalIntentStart(w, r, intent)
		return
	}
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	start, err := h.o.ExternalLogins.BeginExternalLogin(r.Context(), r.PathValue("id"), returnTo)
	switch {
	case errors.Is(err, appidentity.ErrLoginProviderUnavailable):
		h.renderError(w, r, http.StatusNotFound, (*i18n.Localizer).ServerAuthExternalUnavailable)
		return
	case errors.Is(err, appidentity.ErrExternalLoginFailed):
		h.o.Logger.WarnContext(r.Context(), "start an external sign-in", slog.Any("error", err))
		h.renderSignIn(w, r, http.StatusBadGateway, returnTo, "", (*i18n.Localizer).ServerAuthExternalFailed)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	redirectToProvider(w, r, start)
}

func redirectToProvider(w http.ResponseWriter, r *http.Request, start appidentity.ExternalLoginStart) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieExternal, Value: start.Cookie, Path: "/", MaxAge: int(time.Until(start.ExpiresAt).Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, start.AuthorizationURL, http.StatusSeeOther)
}

// externalIntentStart sends the browser of the intent's user to the provider to link an account
// or confirm an operation. A browser without a session signs in first and comes back.
func (h *Handler) externalIntentStart(w http.ResponseWriter, r *http.Request, intent string) {
	sessionUserID := ""
	switch session, err := h.o.Identity.Authenticate(r.Context(), cookieValue(r, CookieSession)); {
	case err == nil:
		sessionUserID = session.UserID
	case errors.Is(err, appidentity.ErrInvalidSession):
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, withReturnTo(PathSignIn, safeReturnTo(r.URL.RequestURI())), http.StatusSeeOther)
		return
	default:
		h.internalError(w, r, err)
		return
	}
	start, err := h.o.ExternalLogins.BeginExternalIntent(r.Context(), r.PathValue("id"), intent, sessionUserID)
	switch {
	case errors.Is(err, appidentity.ErrLoginProviderUnavailable):
		h.renderError(w, r, http.StatusNotFound, (*i18n.Localizer).ServerAuthExternalUnavailable)
	case errors.Is(err, appidentity.ErrExternalLoginExpired):
		h.renderError(w, r, http.StatusUnprocessableEntity, (*i18n.Localizer).ServerAuthExternalExpired)
	case errors.Is(err, appidentity.ErrExternalSessionMismatch):
		h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthExternalOtherUser)
	case errors.Is(err, appidentity.ErrExternalLoginFailed):
		h.o.Logger.WarnContext(r.Context(), "start an external link or confirmation", slog.Any("error", err))
		h.renderError(w, r, http.StatusBadGateway, (*i18n.Localizer).ServerAuthExternalFailed)
	case err != nil:
		h.internalError(w, r, err)
	default:
		redirectToProvider(w, r, start)
	}
}

// Reasons of the external_error fragment of an intent's return.
const (
	externalErrorRateLimited   = "rate_limited"
	externalErrorUnavailable   = "unavailable"
	externalErrorExpired       = "expired"
	externalErrorDenied        = "denied"
	externalErrorMismatch      = "mismatch"
	externalErrorLinked        = "linked_elsewhere"
	externalErrorProviderTaken = "already_linked"
	externalErrorFailed        = "failed"
)

// pathIntentReturn is where an intent without a return target goes back to.
const pathIntentReturn = "/settings/linked-accounts"

// externalIntentDone sends the browser back to the return target of a link or a confirmation
// with its outcome in the fragment.
func (h *Handler) externalIntentDone(w http.ResponseWriter, r *http.Request, res appidentity.ExternalLogin, err error) {
	var limited *kernel.RateLimitedError
	fragment := url.Values{}
	switch {
	case errors.As(err, &limited):
		fragment.Set("external_error", externalErrorRateLimited)
	case errors.Is(err, appidentity.ErrLoginProviderUnavailable):
		fragment.Set("external_error", externalErrorUnavailable)
	case errors.Is(err, appidentity.ErrExternalLoginDenied):
		fragment.Set("external_error", externalErrorDenied)
	case errors.Is(err, appidentity.ErrExternalAccountMismatch):
		fragment.Set("external_error", externalErrorMismatch)
	case errors.Is(err, appidentity.ErrExternalIdentityLinked):
		fragment.Set("external_error", externalErrorLinked)
	case errors.Is(err, appidentity.ErrLoginProviderLinked):
		fragment.Set("external_error", externalErrorProviderTaken)
	case errors.Is(err, appidentity.ErrExternalLoginFailed), errors.Is(err, appidentity.ErrInvalidCredentials):
		h.o.Logger.InfoContext(r.Context(), "an external link or confirmation failed", slog.Any("error", err))
		fragment.Set("external_error", externalErrorFailed)
	case err != nil:
		h.internalError(w, r, err)
		return
	case res.Confirmation != "":
		fragment.Set("external_confirmation", res.Confirmation)
	case res.Linked:
		fragment.Set("linked", r.PathValue("id"))
	default:
		h.internalError(w, r, errors.New("authpages: an external link or confirmation ended without an outcome"))
		return
	}
	if res.Operation != "" {
		fragment.Set("operation", string(res.Operation))
	}
	fragment.Set("provider", r.PathValue("id"))
	returnTo := safeReturnTo(res.ReturnTo)
	if returnTo == "" {
		returnTo = pathIntentReturn
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, returnTo+"#"+fragment.Encode(), http.StatusSeeOther)
}

// externalCallback takes the provider's answer.
func (h *Handler) externalCallback(w http.ResponseWriter, r *http.Request) {
	cookie := cookieValue(r, CookieExternal)
	http.SetCookie(w, &http.Cookie{
		Name: CookieExternal, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	q := r.URL.Query()
	res, err := h.o.ExternalLogins.FinishExternalLogin(r.Context(), appidentity.ExternalLoginInput{
		ProviderID: r.PathValue("id"), Cookie: cookie,
		State: q.Get("state"), Code: q.Get("code"), Error: q.Get("error"),
		IP: kernel.RequestClientIP(r), UserAgent: r.UserAgent(), CurrentToken: cookieValue(r, CookieSession),
	})
	if res.Purpose != "" {
		h.externalIntentDone(w, r, res, err)
		return
	}
	returnTo := res.ReturnTo
	var limited *kernel.RateLimitedError
	switch {
	case errors.As(err, &limited):
		h.renderSignIn(w, r, http.StatusTooManyRequests, returnTo, "", rateLimitedMessage(w, limited))
	case errors.Is(err, appidentity.ErrLoginProviderUnavailable):
		h.renderError(w, r, http.StatusNotFound, (*i18n.Localizer).ServerAuthExternalUnavailable)
	case errors.Is(err, appidentity.ErrExternalLoginExpired):
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, "", (*i18n.Localizer).ServerAuthExternalExpired)
	case errors.Is(err, appidentity.ErrExternalLoginDenied):
		h.renderSignIn(w, r, http.StatusForbidden, returnTo, "", (*i18n.Localizer).ServerAuthExternalDenied)
	case errors.Is(err, appidentity.ErrExternalAccountNotLinked):
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, "", (*i18n.Localizer).ServerAuthExternalNotLinked)
	case errors.Is(err, appidentity.ErrExternalLoginFailed), errors.Is(err, appidentity.ErrInvalidCredentials):
		h.o.Logger.InfoContext(r.Context(), "an external sign-in failed", slog.Any("error", err))
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, "", (*i18n.Localizer).ServerAuthExternalFailed)
	case err != nil:
		h.internalError(w, r, err)
	case res.SecondFactorChallenge != "":
		h.askSecondFactor(w, r, res.SecondFactorChallenge, returnTo)
	case res.SignUp != nil:
		h.renderExternalSignUp(w, r, http.StatusOK, *res.SignUp, nil)
	case res.SignedIn != nil:
		h.signedIn(w, r, *res.SignedIn, returnTo)
	default:
		h.internalError(w, r, errors.New("authpages: an external sign-in ended without an outcome"))
	}
}

// renderExternalSignUp shows the registration form of an account at a provider; form is what
// the applicant sent, nil for the first view.
func (h *Handler) renderExternalSignUp(w http.ResponseWriter, r *http.Request, status int, v appidentity.ExternalSignUpView, p *page) {
	mode, err := h.o.Identity.RegistrationMode(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if p == nil {
		pg := h.newPage(r, (*i18n.Localizer).ServerAuthSignUpTitle)
		pg.SignUp = newSignUpForm(pg.L, mode)
		pg.SignUp.Username, pg.SignUp.Email = v.Username, v.Email
		p = &pg
	}
	f := p.SignUp
	f.External = &externalSignUpForm{Ticket: v.Ticket, Provider: v.ProviderName}
	p.Title = p.L.ServerAuthExternalSignUpTitle(v.ProviderName)
	if v.EmailVerified {
		f.External.EmailVerifiedHint = p.L.ServerAuthExternalSignUpEmailVerified(v.ProviderName)
	}
	h.renderSignUp(w, r, status, *p)
}

// externalSignUp handles the registration form of an account at a provider.
func (h *Handler) externalSignUp(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	form := r.PostForm
	ticket := form.Get("ticket")
	if !h.validCSRF(r) {
		h.renderSignIn(w, r, http.StatusForbidden, "", "", (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	v, err := h.o.ExternalLogins.ExternalSignUp(r.Context(), ticket)
	if h.externalSignUpGone(w, r, err) {
		return
	}
	mode, err := h.o.Identity.RegistrationMode(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthSignUpTitle)
	f := newSignUpForm(p.L, mode)
	p.SignUp = f
	f.Username = strings.TrimSpace(form.Get(fieldUsername))
	f.Email = strings.TrimSpace(form.Get(fieldEmail))
	f.InviteCode = strings.TrimSpace(form.Get(fieldInvite))
	f.Reason = form.Get(fieldReason)
	if f.Closed {
		h.renderExternalSignUp(w, r, http.StatusForbidden, v, &p)
		return
	}
	f.checkFields(p.L)
	if len(f.Errors) > 0 {
		p.Error = p.L.ServerAuthSignUpErrorSummary()
		h.renderExternalSignUp(w, r, http.StatusUnprocessableEntity, v, &p)
		return
	}
	locale, _ := h.o.Locales.AcceptedLocale(r.Header.Get("Accept-Language"))
	signedUp, err := h.o.ExternalLogins.SignUpExternal(r.Context(), appidentity.ExternalSignUpInput{
		Ticket: ticket, Username: f.Username, Email: f.Email, Locale: locale,
		InviteCode: f.InviteCode, Reason: f.Reason,
		IP: kernel.RequestClientIP(r), UserAgent: r.UserAgent(), CurrentToken: cookieValue(r, CookieSession),
		HumanCheckToken: h.humanCheckToken(r),
	})
	if h.externalSignUpGone(w, r, err) {
		return
	}
	if err != nil {
		h.signUpFailedWith(w, r, p, err, func(status int, p page) { h.renderExternalSignUp(w, r, status, v, &p) })
		return
	}
	setSessionCookie(w, signedUp.Session)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
}

// externalSignUpGone answers a sign-up whose ticket no longer works and reports whether it did:
// the sign-up starts again from the sign-in page.
func (h *Handler) externalSignUpGone(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, appidentity.ErrExternalLoginExpired), errors.Is(err, appidentity.ErrExternalIdentityLinked):
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, "", "", (*i18n.Localizer).ServerAuthExternalExpired)
	case errors.Is(err, appidentity.ErrLoginProviderUnavailable):
		h.renderError(w, r, http.StatusNotFound, (*i18n.Localizer).ServerAuthExternalUnavailable)
	default:
		return false
	}
	return true
}

// externalSignUpForm is what the registration form of an account at a provider adds.
type externalSignUpForm struct {
	// Ticket is the sealed sign-up.
	Ticket string
	// Provider is the provider's display name.
	Provider string
	// EmailVerifiedHint tells that the provider verified the address in the form.
	EmailVerifiedHint string
}

// checkFields sets the error of every field but the password that the identity module would
// reject.
func (f *signUpForm) checkFields(l *i18n.Localizer) {
	if appaccount.ValidateUsername(f.Username) != nil {
		f.Errors[fieldUsername] = l.ServerAuthSignUpUsernameInvalid(float64(f.UsernameMax))
	}
	if identity.ValidateEmail(f.Email) != nil {
		f.Errors[fieldEmail] = l.ServerAuthSignUpEmailInvalid()
	}
	if f.NeedInvite && identity.NormalizeInviteCode(f.InviteCode) == "" {
		f.Errors[fieldInvite] = l.ServerAuthSignUpInviteRequired()
	}
	if f.AskReason && identity.ValidateRegistrationReason(strings.TrimSpace(f.Reason)) != nil {
		f.Errors[fieldReason] = l.ServerAuthSignUpReasonInvalid(float64(f.ReasonMax))
	}
}
