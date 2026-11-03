// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"context"
	"errors"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// A user who deactivated their account signs in to the status page only (ADR-0145 §4): it tells
// the date of the deletion and offers to reactivate or delete the account. A reactivation leads on
// to the app unless moderation still keeps the user out.

// Deactivation is what the status page of a deactivated account does; *appidentity.Service
// implements it.
type Deactivation interface {
	ReactivateOwnAccount(ctx context.Context, userID string) error
	DeleteDeactivatedAccount(ctx context.Context, session identity.Session, password string) error
	HasPassword(ctx context.Context, userID string) (bool, error)
}

// deactivatedSession reads the form of a page of a deactivated account: the signed-in user whose
// account it is. It answers the request itself, and returns false, for anything else.
func (h *Handler) deactivatedSession(w http.ResponseWriter, r *http.Request) (identity.Session, appidentity.Access, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return identity.Session{}, appidentity.Access{}, false
	}
	if !h.validCSRF(r) {
		h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerAuthFormExpired)
		return identity.Session{}, appidentity.Access{}, false
	}
	session, err := h.o.Identity.Authenticate(r.Context(), cookieValue(r, CookieSession))
	if errors.Is(err, appidentity.ErrInvalidSession) {
		http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
		return identity.Session{}, appidentity.Access{}, false
	} else if err != nil {
		h.internalError(w, r, err)
		return identity.Session{}, appidentity.Access{}, false
	}
	access, err := h.o.Identity.UserAccess(r.Context(), session.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return identity.Session{}, appidentity.Access{}, false
	}
	if !access.Deactivated || h.o.Deactivation == nil {
		// Nothing to do here; the sign-in page says where the user stands.
		http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
		return identity.Session{}, appidentity.Access{}, false
	}
	return session, access, true
}

// reactivate handles the reactivation button of the status page.
func (h *Handler) reactivate(w http.ResponseWriter, r *http.Request) {
	session, access, ok := h.deactivatedSession(w, r)
	if !ok {
		return
	}
	err := h.o.Deactivation.ReactivateOwnAccount(r.Context(), session.UserID)
	switch {
	case errors.Is(err, appaccount.ErrDeactivationExpired):
		h.renderStatus(w, r, http.StatusConflict, session.UserID, access, nil, (*i18n.Localizer).ServerAuthStatusDeactivatedExpired)
		return
	case errors.Is(err, appaccount.ErrNotDeactivated):
		// A second press, or a ban that came in between; the page says where the user stands.
		http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	// The sign-in page goes on to the app, or shows what moderation still has in effect.
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
}

// deleteDeactivated handles the deletion form of the status page.
func (h *Handler) deleteDeactivated(w http.ResponseWriter, r *http.Request) {
	session, access, ok := h.deactivatedSession(w, r)
	if !ok {
		return
	}
	refuse := func(status int, alert func(*i18n.Localizer) string) {
		h.renderStatus(w, r, status, session.UserID, access, nil, alert)
	}
	err := h.o.Deactivation.DeleteDeactivatedAccount(r.Context(), session, r.PostForm.Get("password"))
	var limited *kernel.RateLimitedError
	switch {
	case errors.Is(err, appidentity.ErrWrongPassword):
		refuse(http.StatusUnprocessableEntity, (*i18n.Localizer).ServerAuthStatusDeactivatedWrongPassword)
		return
	case errors.Is(err, appidentity.ErrLastAdmin):
		refuse(http.StatusConflict, (*i18n.Localizer).ServerErrorsDeactivationLastAdministrator)
		return
	case errors.Is(err, appidentity.ErrSignInAgain):
		refuse(http.StatusUnprocessableEntity, (*i18n.Localizer).ServerAuthStatusDeactivatedSignInAgain)
		return
	case errors.As(err, &limited):
		refuse(http.StatusTooManyRequests, rateLimitedMessage(w, limited))
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: CookieSession, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	p := h.newPage(r, (*i18n.Localizer).ServerAuthStatusDeletedTitle)
	h.render(w, r, http.StatusOK, pageAccountDeleted, p)
}
