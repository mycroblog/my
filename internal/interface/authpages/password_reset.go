// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// Resetting a forgotten password (FR-002). The sign-in page links to a form that asks for a
// reset link by email (PathForgotPassword), with the CAPTCHA widget when one is set; its answer
// reads the same whether or not an account uses the address. On a server that sends no email,
// the form is replaced by a note to ask the administrators, who issue links with the CLI. The
// link opens a form for the new password (PathResetPassword); like the email verification link,
// the GET only shows the form, and the POST uses the token, so that mail scanners that fetch
// links do not use it up.

// Paths of the password reset pages.
const (
	PathForgotPassword = appidentity.PathForgotPassword
	PathResetPassword  = appidentity.PathResetPassword
)

// PasswordReset is what the password reset pages need; *identity.Service implements it.
type PasswordReset interface {
	PasswordResetByEmail() bool
	RequestPasswordReset(ctx context.Context, in appidentity.PasswordResetRequest) error
	CheckPasswordResetToken(ctx context.Context, token string) error
	ResetPassword(ctx context.Context, token, password string) (string, error)
}

// forgotPasswordView is the page that asks for a reset link.
type forgotPasswordView struct {
	// Unavailable replaces the form with a note when the server sends no email.
	Unavailable bool
	// Sent replaces the form with the notice after a request.
	Sent bool
}

// newPasswordView is the field of the new password on the page that a reset link opens.
type newPasswordView struct {
	Hint     string
	Error    string
	Min, Max int
}

func (h *Handler) passwordResetEnabled() bool { return h.o.PasswordReset != nil }

// forgotPasswordPage shows the form that asks for a reset link.
func (h *Handler) forgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, (*i18n.Localizer).ServerAuthForgotPasswordTitle)
	h.renderForgotPassword(w, r, http.StatusOK, p)
}

func (h *Handler) renderForgotPassword(w http.ResponseWriter, r *http.Request, status int, p page) {
	if p.ForgotPassword == nil {
		p.ForgotPassword = &forgotPasswordView{}
	}
	p.ForgotPassword.Unavailable = !h.o.PasswordReset.PasswordResetByEmail()
	if !p.ForgotPassword.Unavailable && !p.ForgotPassword.Sent {
		p.CSRFToken = h.csrfToken(w, r)
		p.HumanCheck = h.humanCheck(w, r, p.L, kernel.HumanCheckActionPasswordReset)
	}
	h.render(w, r, status, pageForgotPassword, p)
}

// forgotPassword handles the form that asks for a reset link.
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthForgotPasswordTitle)
	p.Email = strings.TrimSpace(r.PostForm.Get("email"))
	if !h.validCSRF(r) {
		p.Error = p.L.ServerAuthFormExpired()
		h.renderForgotPassword(w, r, http.StatusForbidden, p)
		return
	}
	err := h.o.PasswordReset.RequestPasswordReset(r.Context(), appidentity.PasswordResetRequest{
		Email: p.Email, IP: kernel.RequestClientIP(r), HumanCheckToken: h.humanCheckToken(r),
	})
	var limited *kernel.RateLimitedError
	status := http.StatusUnprocessableEntity
	switch {
	case err == nil:
		minutes := int(appidentity.PasswordResetTTL / time.Minute)
		p.Notice = p.L.ServerAuthForgotPasswordSent(i18n.ServerAuthForgotPasswordSentArgs{Email: p.Email, Minutes: minutes})
		p.ForgotPassword = &forgotPasswordView{Sent: true}
		h.renderForgotPassword(w, r, http.StatusOK, p)
		return
	case errors.Is(err, appidentity.ErrPasswordResetByEmailUnavailable):
		status = http.StatusNotFound
	case errors.Is(err, identity.ErrInvalidEmail):
		p.Error = p.L.ServerAuthForgotPasswordInvalidEmail()
	case errors.As(err, &limited):
		status = http.StatusTooManyRequests
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
		p.Error = p.L.ServerAuthForgotPasswordRateLimited(int(math.Ceil(limited.RetryAfter.Minutes())))
	case errors.Is(err, kernel.ErrHumanCheckFailed):
		p.Error = p.L.ServerAuthHumanCheckFailed()
	case errors.Is(err, kernel.ErrHumanCheckUnavailable):
		// A request sends email, so it fails closed as a sign-up does (ADR-0042 §4).
		status = http.StatusServiceUnavailable
		p.Error = p.L.ServerAuthHumanCheckUnavailable()
	default:
		h.internalError(w, r, err)
		return
	}
	h.renderForgotPassword(w, r, status, p)
}

// resetPasswordPage shows the form for the new password of a reset link, or says that the link
// does not work.
func (h *Handler) resetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > maxTokenLength {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthResetPasswordInvalid)
		return
	}
	switch err := h.o.PasswordReset.CheckPasswordResetToken(r.Context(), token); {
	case errors.Is(err, appidentity.ErrInvalidToken):
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthResetPasswordInvalid)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthResetPasswordTitle)
	p.Token = token
	h.renderResetPassword(w, r, http.StatusOK, p)
}

func (h *Handler) renderResetPassword(w http.ResponseWriter, r *http.Request, status int, p page) {
	p.CSRFToken = h.csrfToken(w, r)
	if p.NewPassword == nil {
		p.NewPassword = &newPasswordView{}
	}
	p.NewPassword.Hint = p.L.ServerAuthSignUpPasswordHint(i18n.ServerAuthSignUpPasswordHintArgs{Max: identity.MaxPasswordLength, Min: identity.MinPasswordLength})
	p.NewPassword.Min, p.NewPassword.Max = identity.MinPasswordLength, identity.MaxPasswordLength
	h.render(w, r, status, pageResetPassword, p)
}

// resetPassword sets the new password of a reset link.
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	token := r.PostForm.Get("token")
	if len(token) > maxTokenLength {
		token = ""
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthResetPasswordTitle)
	p.Token = token
	if !h.validCSRF(r) {
		p.Error = p.L.ServerAuthFormExpired()
		h.renderResetPassword(w, r, http.StatusForbidden, p)
		return
	}
	_, err := h.o.PasswordReset.ResetPassword(r.Context(), token, r.PostForm.Get("password"))
	switch {
	case errors.Is(err, appidentity.ErrInvalidToken):
		h.renderError(w, r, http.StatusUnprocessableEntity, (*i18n.Localizer).ServerAuthResetPasswordInvalid)
		return
	case errors.Is(err, identity.ErrInvalidPassword):
		p.NewPassword = &newPasswordView{
			Error: p.L.ServerAuthSignUpPasswordInvalid(i18n.ServerAuthSignUpPasswordInvalidArgs{Max: float64(identity.MaxPasswordLength), Min: float64(identity.MinPasswordLength)}),
		}
		h.renderResetPassword(w, r, http.StatusUnprocessableEntity, p)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	// The new password ended every session, this browser's too.
	if cookieValue(r, CookieSession) != "" {
		http.SetCookie(w, &http.Cookie{
			Name: CookieSession, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	}
	h.render(w, r, http.StatusOK, pagePasswordReset, h.newPage(r, (*i18n.Localizer).ServerAuthResetPasswordDoneTitle))
}
