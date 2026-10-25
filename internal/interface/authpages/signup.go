// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// Fields of the registration form.
const (
	fieldUsername = "username"
	fieldEmail    = "email"
	fieldPassword = "password"
	fieldInvite   = "invite_code"
	fieldReason   = "reason"
)

// signUpForm is the registration form: what the applicant typed (never the password), the
// fields the mode asks for, and the error of each field.
type signUpForm struct {
	Closed     bool
	NeedInvite bool
	AskReason  bool

	Username, Email, InviteCode, Reason string

	UsernameHint, PasswordHint, ReasonHint string
	// Errors maps a field name to its error message.
	Errors map[string]string

	UsernameMax, PasswordMin, PasswordMax, ReasonMax int
}

func newSignUpForm(l *i18n.Localizer, mode identity.RegistrationMode) *signUpForm {
	return &signUpForm{
		Closed:       mode == identity.RegistrationClosed,
		NeedInvite:   mode == identity.RegistrationInvite,
		AskReason:    mode == identity.RegistrationApproval,
		UsernameHint: l.ServerAuthSignUpUsernameHint(account.MaxUsernameLength),
		PasswordHint: l.ServerAuthSignUpPasswordHint(identity.MaxPasswordLength, identity.MinPasswordLength),
		ReasonHint:   l.ServerAuthSignUpReasonHint(identity.MaxRegistrationReasonLength),
		Errors:       map[string]string{},
		UsernameMax:  account.MaxUsernameLength,
		PasswordMin:  identity.MinPasswordLength,
		PasswordMax:  identity.MaxPasswordLength,
		ReasonMax:    identity.MaxRegistrationReasonLength,
	}
}

// signUpPage shows the registration form for the current mode. A browser that already has a
// session goes to the sign-in page, which shows that session.
func (h *Handler) signUpPage(w http.ResponseWriter, r *http.Request) {
	if _, err := h.o.Identity.Authenticate(r.Context(), cookieValue(r, CookieSession)); err == nil {
		http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
		return
	} else if !errors.Is(err, appidentity.ErrInvalidSession) {
		h.internalError(w, r, err)
		return
	}
	mode, err := h.o.Identity.RegistrationMode(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthSignUpTitle)
	p.SignUp = newSignUpForm(p.L, mode)
	p.SignUp.InviteCode = r.URL.Query().Get("invite")
	h.renderSignUp(w, r, http.StatusOK, p)
}

func (h *Handler) renderSignUp(w http.ResponseWriter, r *http.Request, status int, p page) {
	p.CSRFToken = h.csrfToken(w, r)
	if p.SignUp.Closed && status == http.StatusOK {
		status = http.StatusForbidden
	}
	if !p.SignUp.Closed {
		p.HumanCheck = h.humanCheck(w, r, p.L, kernel.HumanCheckActionSignUp)
	}
	h.render(w, r, status, pageSignUp, p)
}

// signUp handles the registration form.
func (h *Handler) signUp(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
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
	form := r.PostForm
	f.Username = strings.TrimSpace(form.Get(fieldUsername))
	f.Email = strings.TrimSpace(form.Get(fieldEmail))
	f.InviteCode = strings.TrimSpace(form.Get(fieldInvite))
	f.Reason = form.Get(fieldReason)
	password := form.Get(fieldPassword)
	if !h.validCSRF(r) {
		p.Error = p.L.ServerAuthFormExpired()
		h.renderSignUp(w, r, http.StatusForbidden, p)
		return
	}
	if f.Closed {
		h.renderSignUp(w, r, http.StatusForbidden, p)
		return
	}
	// Check every field first, so that the form shows all of its errors at once.
	f.check(p.L, password)
	if len(f.Errors) > 0 {
		p.Error = p.L.ServerAuthSignUpErrorSummary()
		h.renderSignUp(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	// The new account's locale is the browser's language that the page is shown in, never a
	// pseudo-locale; with none, the identity module gives it the instance's default locale.
	locale, _ := h.o.Locales.AcceptedLocale(r.Header.Get("Accept-Language"))
	signedUp, err := h.o.Identity.SignUp(r.Context(), appidentity.SignUpInput{
		Username: f.Username, Email: f.Email, Password: password, Locale: locale,
		InviteCode: f.InviteCode, Reason: f.Reason,
		IP: kernel.RequestClientIP(r), UserAgent: r.UserAgent(), CurrentToken: cookieValue(r, CookieSession),
		HumanCheckToken: h.humanCheckToken(r),
	})
	if err != nil {
		h.signUpFailed(w, r, p, err)
		return
	}
	setSessionCookie(w, signedUp.Session)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
}

// check sets the error of every field that the identity module would reject.
func (f *signUpForm) check(l *i18n.Localizer, password string) {
	if appaccount.ValidateUsername(f.Username) != nil {
		f.Errors[fieldUsername] = l.ServerAuthSignUpUsernameInvalid(float64(f.UsernameMax))
	}
	if identity.ValidateEmail(f.Email) != nil {
		f.Errors[fieldEmail] = l.ServerAuthSignUpEmailInvalid()
	}
	if identity.ValidatePassword(password) != nil {
		f.Errors[fieldPassword] = l.ServerAuthSignUpPasswordInvalid(float64(f.PasswordMax), float64(f.PasswordMin))
	}
	if f.NeedInvite && identity.NormalizeInviteCode(f.InviteCode) == "" {
		f.Errors[fieldInvite] = l.ServerAuthSignUpInviteRequired()
	}
	if f.AskReason && identity.ValidateRegistrationReason(strings.TrimSpace(f.Reason)) != nil {
		f.Errors[fieldReason] = l.ServerAuthSignUpReasonInvalid(float64(f.ReasonMax))
	}
}

// signUpFailed shows the form again with what SignUp rejected.
func (h *Handler) signUpFailed(w http.ResponseWriter, r *http.Request, p page, err error) {
	f, l := p.SignUp, p.L
	status := http.StatusUnprocessableEntity
	var limited *kernel.RateLimitedError
	switch {
	case errors.As(err, &limited):
		status = http.StatusTooManyRequests
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
		p.Error = l.ServerAuthSignUpRateLimited(int(math.Ceil(limited.RetryAfter.Minutes())))
	case errors.Is(err, appidentity.ErrRegistrationClosed):
		f.Closed = true
		status = http.StatusForbidden
	case errors.Is(err, kernel.ErrHumanCheckFailed):
		p.Error = l.ServerAuthHumanCheckFailed()
	case errors.Is(err, kernel.ErrHumanCheckUnavailable):
		// Registration fails closed while the service cannot check the token (ADR-0042 §4).
		status = http.StatusServiceUnavailable
		p.Error = l.ServerAuthHumanCheckUnavailable()
	case errors.Is(err, appidentity.ErrSignUpRefused):
		// The sign-up block list refused it; the page does not say which block or why (FR-719).
		status = http.StatusForbidden
		p.Error = l.ServerAuthSignUpRefused()
	case errors.Is(err, appaccount.ErrUsernameTaken):
		f.Errors[fieldUsername] = l.ServerAuthSignUpUsernameTaken()
	case errors.Is(err, appidentity.ErrEmailTaken):
		f.Errors[fieldEmail] = l.ServerAuthSignUpEmailTaken()
	case errors.Is(err, appidentity.ErrInviteRequired):
		f.Errors[fieldInvite] = l.ServerAuthSignUpInviteRequired()
	case errors.Is(err, appidentity.ErrInvalidInvite):
		f.Errors[fieldInvite] = l.ServerAuthSignUpInviteInvalid()
	case errors.Is(err, account.ErrInvalidUsername):
		f.Errors[fieldUsername] = l.ServerAuthSignUpUsernameInvalid(float64(f.UsernameMax))
	case errors.Is(err, identity.ErrInvalidEmail):
		f.Errors[fieldEmail] = l.ServerAuthSignUpEmailInvalid()
	case errors.Is(err, identity.ErrInvalidPassword):
		f.Errors[fieldPassword] = l.ServerAuthSignUpPasswordInvalid(float64(f.PasswordMax), float64(f.PasswordMin))
	case errors.Is(err, identity.ErrInvalidReason):
		f.Errors[fieldReason] = l.ServerAuthSignUpReasonInvalid(float64(f.ReasonMax))
	default:
		h.internalError(w, r, err)
		return
	}
	if len(f.Errors) > 0 {
		p.Error = l.ServerAuthSignUpErrorSummary()
	}
	h.renderSignUp(w, r, status, p)
}

// statusView is what the status page says about an account that cannot be used yet.
type statusView struct {
	Pending, Rejected bool
	// VerifyEmail asks the user to open the link sent to their address; empty when verified.
	VerifyEmail string
	// Restricted tells that moderation keeps the account out of the app, by Actions
	// (moderation.go).
	Restricted bool
	Actions    []statusAction
}

// renderStatus shows the status page of userID, in the user's saved locale. notice and alert
// are worded in l, the page's localizer, when given.
func (h *Handler) renderStatus(w http.ResponseWriter, r *http.Request, status int, userID string, access appidentity.Access, notice, alert func(l *i18n.Localizer) string) {
	h.renderStatusPage(w, r, status, userID, access, notice, alert, nil)
}

// renderStatusPage is renderStatus with an appeal to show again in its form (moderation.go);
// nil for none.
func (h *Handler) renderStatusPage(w http.ResponseWriter, r *http.Request, status int, userID string, access appidentity.Access, notice, alert func(l *i18n.Localizer) string, draft *appealDraft) {
	l := h.userLocalizer(r, userID)
	title := (*i18n.Localizer).ServerAuthStatusTitle
	if access.LoginRestricted {
		title = (*i18n.Localizer).ServerAuthStatusRestrictedTitle
	}
	actions, err := h.restrictionActions(r.Context(), l, access, draft)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	p := page{L: l, Lang: l.Locale(), Dir: l.Direction(), Title: title(l), Stylesheet: h.o.Stylesheet}
	p.CSRFToken = h.csrfToken(w, r)
	if notice != nil {
		p.Notice = notice(l)
	}
	if alert != nil {
		p.Error = alert(l)
	}
	v := &statusView{
		Pending:    access.Registration == identity.RegistrationPending,
		Rejected:   access.Registration == identity.RegistrationRejected,
		Restricted: access.LoginRestricted,
		Actions:    actions,
	}
	if access.AwaitingVerification() && !v.Rejected {
		v.VerifyEmail = l.ServerAuthStatusVerifyEmail(access.Email)
	}
	p.Status = v
	h.render(w, r, status, pageStatus, p)
}

// verifyEmailPage shows a button that confirms the email address of the link's token. The
// token is redeemed only by the POST, so that mail scanners that fetch links do not use it up.
func (h *Handler) verifyEmailPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > maxTokenLength {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthVerifyEmailInvalid)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthVerifyEmailTitle)
	p.CSRFToken = h.csrfToken(w, r)
	p.Token = token
	h.render(w, r, http.StatusOK, pageVerifyEmail, p)
}

// maxTokenLength bounds the token of a verification link; issued tokens are far shorter.
const maxTokenLength = 256

// verifyEmail redeems the token of the confirmation form.
func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	if !h.validCSRF(r) {
		h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	token := r.PostForm.Get("token")
	if len(token) > maxTokenLength {
		token = ""
	}
	_, err := h.o.Identity.VerifyEmail(r.Context(), token)
	switch {
	case errors.Is(err, appidentity.ErrInvalidToken):
		h.renderError(w, r, http.StatusUnprocessableEntity, (*i18n.Localizer).ServerAuthVerifyEmailInvalid)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthVerifyEmailDoneTitle)
	h.render(w, r, http.StatusOK, pageVerified, p)
}

// resendVerification sends the signed-in user a new verification email.
func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	if !h.validCSRF(r) {
		h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	session, err := h.o.Identity.Authenticate(r.Context(), cookieValue(r, CookieSession))
	if errors.Is(err, appidentity.ErrInvalidSession) {
		http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	access, err := h.o.Identity.UserAccess(r.Context(), session.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	var limited *kernel.RateLimitedError
	switch err := h.o.Identity.ResendVerificationEmail(r.Context(), session.UserID); {
	case err == nil:
		h.renderStatus(w, r, http.StatusOK, session.UserID, access, func(l *i18n.Localizer) string { return l.ServerAuthStatusResent(access.Email) }, nil)
	case errors.Is(err, appidentity.ErrAlreadyVerified):
		http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
		minutes := int(math.Ceil(limited.RetryAfter.Minutes()))
		h.renderStatus(w, r, http.StatusTooManyRequests, session.UserID, access, nil, func(l *i18n.Localizer) string { return l.ServerAuthStatusResendRateLimited(minutes) })
	default:
		h.o.Logger.ErrorContext(r.Context(), "send the verification email again", slog.String("user", session.UserID), slog.Any("error", err))
		h.renderStatus(w, r, http.StatusServiceUnavailable, session.UserID, access, nil, (*i18n.Localizer).ServerAuthStatusResendFailed)
	}
}
