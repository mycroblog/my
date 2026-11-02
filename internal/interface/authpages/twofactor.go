// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// The second step of a sign-in for a user with two-factor authentication (FR-005). The sign-in
// form's right password leaves the challenge of the sign-in in the __Host-my_2fa cookie
// (HttpOnly, five minutes) and redirects to the code form, whose right code starts the session.
// The CAPTCHA belongs to the password step; wrong codes count against the rate limits of the
// user and of the client's address.

// askSecondFactor keeps the challenge of a sign-in whose password was right and sends the
// browser to the code form.
func (h *Handler) askSecondFactor(w http.ResponseWriter, r *http.Request, challenge, returnTo string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieTwoFactor,
		Value:    challenge,
		Path:     "/",
		MaxAge:   int(appidentity.SecondFactorChallengeTTL.Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, withReturnTo(PathSignInTwoFactor, returnTo), http.StatusSeeOther)
}

// withReturnTo adds return_to to path when it is set.
func withReturnTo(path, returnTo string) string {
	if returnTo == "" {
		return path
	}
	return path + "?" + url.Values{"return_to": {returnTo}}.Encode()
}

func clearTwoFactorCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieTwoFactor, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// twoFactorPage shows the code form, or sends a browser without a pending sign-in back to the
// sign-in form.
func (h *Handler) twoFactorPage(w http.ResponseWriter, r *http.Request) {
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	if cookieValue(r, CookieTwoFactor) == "" {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, withReturnTo(PathSignIn, returnTo), http.StatusSeeOther)
		return
	}
	h.renderTwoFactor(w, r, http.StatusOK, returnTo, nil)
}

func (h *Handler) renderTwoFactor(w http.ResponseWriter, r *http.Request, status int, returnTo string, message func(*i18n.Localizer) string) {
	p := h.newPage(r, (*i18n.Localizer).ServerAuthTwoFactorTitle)
	p.CSRFToken = h.csrfToken(w, r)
	p.ReturnTo = returnTo
	if message != nil {
		p.Error = message(p.L)
	}
	h.render(w, r, status, pageTwoFactor, p)
}

// twoFactor handles the code form.
func (h *Handler) twoFactor(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	returnTo := safeReturnTo(r.PostForm.Get("return_to"))
	if !h.validCSRF(r) {
		h.renderTwoFactor(w, r, http.StatusForbidden, returnTo, (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	code := strings.TrimSpace(r.PostForm.Get("code"))
	if code == "" {
		h.renderTwoFactor(w, r, http.StatusUnprocessableEntity, returnTo, (*i18n.Localizer).ServerAuthTwoFactorMissing)
		return
	}
	signedIn, err := h.o.Identity.CompleteSignIn(r.Context(), appidentity.SecondFactorInput{
		Challenge:    cookieValue(r, CookieTwoFactor),
		Code:         code,
		IP:           kernel.RequestClientIP(r),
		UserAgent:    r.UserAgent(),
		CurrentToken: cookieValue(r, CookieSession),
	})
	var limited *kernel.RateLimitedError
	switch {
	case errors.Is(err, appidentity.ErrLoginProvidersOnly):
		clearTwoFactorCookie(w)
		h.refuseLoginProvidersOnly(w, r, returnTo)
		return
	case errors.As(err, &limited):
		h.renderTwoFactor(w, r, http.StatusTooManyRequests, returnTo, rateLimitedMessage(w, limited))
		return
	case errors.Is(err, appidentity.ErrInvalidSecondFactor):
		h.renderTwoFactor(w, r, http.StatusUnprocessableEntity, returnTo, (*i18n.Localizer).ServerAuthTwoFactorInvalid)
		return
	case errors.Is(err, appidentity.ErrSecondFactorExpired):
		clearTwoFactorCookie(w)
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, "", (*i18n.Localizer).ServerAuthTwoFactorExpired)
		return
	case errors.Is(err, appidentity.ErrInvalidCredentials):
		clearTwoFactorCookie(w)
		h.renderSignIn(w, r, http.StatusUnprocessableEntity, returnTo, "", (*i18n.Localizer).ServerAuthSignInFailed)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	clearTwoFactorCookie(w)
	h.signedIn(w, r, signedIn, returnTo)
}
