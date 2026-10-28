// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"context"
	"errors"
	"net/http"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// Unsubscribing from notification email (ADR-0086 §4). The List-Unsubscribe header and the
// signature of every notification email link to PathMailUnsubscribe with a token of that email.
// The POST is the one-click unsubscribe of RFC 8058: mail providers send it from their servers,
// with the body List-Unsubscribe=One-Click and neither a session nor the anti-forgery token, so
// it skips CrossOriginProtection and the CSRF check; the token alone authorizes it, and using it
// again changes nothing more. The GET only shows a button that sends the same POST, so that mail
// scanners that fetch links change nothing.

// PathMailUnsubscribe is the endpoint of the unsubscribe links.
const PathMailUnsubscribe = appidentity.PathMailUnsubscribe

// MailUnsubscribe is what the unsubscribe endpoint needs; *identity.Service implements it.
type MailUnsubscribe interface {
	CheckUnsubscribeToken(ctx context.Context, token string) (identity.UnsubscribeCategory, error)
	Unsubscribe(ctx context.Context, token string) (identity.UnsubscribeCategory, error)
}

// unsubscribeView is the page of an unsubscribe link.
type unsubscribeView struct {
	// Prompt asks to stop the emails of the link's category; Done says that they stopped.
	Prompt string
	Done   string
}

// unsubscribeGroup names the category of an unsubscribe link: the row of the notification
// settings whose email it turns off; "" for a category that no email carries.
func unsubscribeGroup(l *i18n.Localizer, category identity.UnsubscribeCategory) string {
	return appnotification.GroupLabel(l, notification.Group(category))
}

// mailUnsubscribePage asks to stop the emails of an unsubscribe link, without changing anything.
func (h *Handler) mailUnsubscribePage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > maxTokenLength {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthMailUnsubscribeInvalid)
		return
	}
	category, err := h.o.MailUnsubscribe.CheckUnsubscribeToken(r.Context(), token)
	switch {
	case errors.Is(err, appidentity.ErrInvalidToken) || err == nil && !appnotification.Unsubscribable(notification.Group(category)):
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthMailUnsubscribeInvalid)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthMailUnsubscribeTitle)
	p.Token = token
	p.Unsubscribe = &unsubscribeView{Prompt: p.L.ServerAuthMailUnsubscribePrompt(unsubscribeGroup(p.L, category))}
	h.render(w, r, http.StatusOK, pageMailUnsubscribe, p)
}

// mailUnsubscribe stops the emails of an unsubscribe link: the one-click POST of RFC 8058, and
// the button of mailUnsubscribePage. The token is in the query, as in the link, or in the form.
func (h *Handler) mailUnsubscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerErrorsBadRequest)
		return
	}
	token := r.Form.Get("token")
	if token == "" || len(token) > maxTokenLength {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthMailUnsubscribeInvalid)
		return
	}
	category, err := h.o.MailUnsubscribe.Unsubscribe(r.Context(), token)
	switch {
	case errors.Is(err, appidentity.ErrInvalidToken) || err == nil && !appnotification.Unsubscribable(notification.Group(category)):
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerAuthMailUnsubscribeInvalid)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	p := h.newPage(r, (*i18n.Localizer).ServerAuthMailUnsubscribeDoneTitle)
	p.Unsubscribe = &unsubscribeView{Done: p.L.ServerAuthMailUnsubscribeDone(unsubscribeGroup(p.L, category))}
	h.render(w, r, http.StatusOK, pageMailUnsubscribed, p)
}
