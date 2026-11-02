// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// Signing in with a passkey (FR-004). The sign-in page carries a second form, hidden, with the
// options and the sealed challenge of a sign-in in its attributes; the web build's script
// (Options.PasskeyScript) shows it in a browser with passkeys, asks the browser for a passkey
// when it is sent, and sends the browser's answer in its credential field. The rest is a form
// like the password's: the anti-forgery token, CrossOriginProtection, the session cookie of a
// sign-in, and the code step for a user with two-factor authentication. Failures count against
// the client's sign-in rate limit; the CAPTCHA belongs to the password, which a passkey does
// not guess.

// Passkeys is what the sign-in page needs to sign in with passkeys; *identity.Service
// implements it.
type Passkeys interface {
	BeginPasskeySignIn(ctx context.Context) (appidentity.PasskeySignIn, error)
	SignInWithPasskey(ctx context.Context, in appidentity.PasskeySignInInput) (appidentity.SignedIn, error)
}

// passkeyView is the passkey form of the sign-in page.
type passkeyView struct {
	// Options are the request options of the sign-in as JSON, and Challenge its sealed state.
	Options   string
	Challenge string
	// Script is the URL of the script that runs the form.
	Script string
	// Error is an alert about the passkey form.
	Error string
}

func (h *Handler) passkeysEnabled() bool { return h.o.Passkeys != nil && h.o.PasskeyScript != "" }

// passkeyView starts a passkey sign-in for the sign-in page, or returns nil without passkeys.
// A failure only leaves the passkey out of the page.
func (h *Handler) passkeyView(r *http.Request) *passkeyView {
	if !h.passkeysEnabled() {
		return nil
	}
	begun, err := h.o.Passkeys.BeginPasskeySignIn(r.Context())
	if err != nil {
		h.o.Logger.WarnContext(r.Context(), "start a passkey sign-in", slog.Any("error", err))
		return nil
	}
	return &passkeyView{Options: string(begun.Options), Challenge: begun.Challenge, Script: h.o.PasskeyScript}
}

// maxCredentialBytes bounds the browser's answer to a sign-in: base64url of the client data,
// the authenticator data, a signature, and a user handle are a few kilobytes at most.
const maxCredentialBytes = 16 << 10

// signInWithPasskey handles the passkey form of the sign-in page.
func (h *Handler) signInWithPasskey(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	returnTo := safeReturnTo(r.PostForm.Get("return_to"))
	if !h.validCSRF(r) {
		h.renderSignInPage(w, r, http.StatusForbidden, returnTo, "", nil, (*i18n.Localizer).ServerAuthFormExpired)
		return
	}
	// The limit comes before anything about the form, so that the answer does not depend on it.
	switch limited, ok := h.loginProvidersOnly(w, r); {
	case !ok:
		return
	case limited:
		h.refuseLoginProvidersOnly(w, r, returnTo)
		return
	}
	credential := r.PostForm.Get("credential")
	if credential == "" || len(credential) > maxCredentialBytes {
		h.renderSignInPage(w, r, http.StatusUnprocessableEntity, returnTo, "", nil, (*i18n.Localizer).ServerAuthSignInPasskeyCancelled)
		return
	}
	signedIn, err := h.o.Passkeys.SignInWithPasskey(r.Context(), appidentity.PasskeySignInInput{
		Challenge:    r.PostForm.Get("challenge"),
		Response:     []byte(credential),
		IP:           kernel.RequestClientIP(r),
		UserAgent:    r.UserAgent(),
		CurrentToken: cookieValue(r, CookieSession),
	})
	var limited *kernel.RateLimitedError
	var secondFactor *appidentity.SecondFactorRequiredError
	switch {
	case errors.Is(err, appidentity.ErrLoginProvidersOnly):
		h.refuseLoginProvidersOnly(w, r, returnTo)
		return
	case errors.As(err, &limited):
		h.renderSignInPage(w, r, http.StatusTooManyRequests, returnTo, "", nil, rateLimitedMessage(w, limited))
		return
	case errors.As(err, &secondFactor):
		h.askSecondFactor(w, r, secondFactor.Challenge, returnTo)
		return
	case errors.Is(err, appidentity.ErrPasskeyCeremonyExpired):
		h.renderSignInPage(w, r, http.StatusUnprocessableEntity, returnTo, "", nil, (*i18n.Localizer).ServerAuthSignInPasskeyExpired)
		return
	case errors.Is(err, appidentity.ErrInvalidPasskey), errors.Is(err, appidentity.ErrInvalidCredentials):
		h.renderSignInPage(w, r, http.StatusUnprocessableEntity, returnTo, "", nil, (*i18n.Localizer).ServerAuthSignInPasskeyFailed)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	h.signedIn(w, r, signedIn, returnTo)
}
