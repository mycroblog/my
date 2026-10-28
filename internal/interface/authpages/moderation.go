// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// A user whom moderation keeps out of the app (disable_login, suspend, ban) still signs in to
// these screens, a banned one until the ban deletes the account (identity.Service.SignIn): the
// status page lists the actions in effect that keep them out, so that they can see why without
// an access token (FR-715, NFR-MOD-6), and takes their appeal of each action, once and within
// its appeal window (FR-707, ADR-0038), since they cannot reach the client API.

// Moderation lists the actions on a user's own account as the user sees them, with no private
// note, report, or issuer, and files the user's appeals of them. *appmoderation.Service
// implements it.
type Moderation interface {
	OwnActions(ctx context.Context, accountID string, f appmoderation.OwnActionFilter, limit int) ([]appmoderation.TargetActionView, error)
	FileAppeal(ctx context.Context, accountID, actionID, text string) (appmoderation.TargetActionView, error)
}

// UserLocales reads the UI locale that a user saved. *appidentity.Service implements it.
type UserLocales interface {
	UserLocale(ctx context.Context, userID string) (string, error)
}

// maxStatusActions bounds the actions that the status page lists.
const maxStatusActions = 40

// statusAction is one action in effect on the status page.
type statusAction struct {
	ID             string
	Kind, Category string
	// Reason is the moderators' explanation; empty when there is none.
	Reason string
	// Ends says when the action ends; EndsAt is that time in RFC 3339, empty for an action that
	// lasts until it is lifted.
	Ends, EndsAt string

	// Appeal is the user's appeal of the action; nil for none.
	Appeal *statusAppeal
	// CanAppeal shows the appeal form, which AppealHint describes; AppealMax is the longest
	// appeal in characters.
	CanAppeal  bool
	AppealHint string
	AppealMax  int
	// NoAppeal says why the action cannot be appealed; empty when it can be or is appealed.
	NoAppeal string
	// AppealText is the text of a refused appeal of the action, shown again in its form, and
	// AppealError why the text was refused.
	AppealText, AppealError string
}

// statusAppeal is the user's appeal of an action on the status page.
type statusAppeal struct {
	// State says where the appeal stands.
	State string
	Text  string
	// Answer is the moderators' explanation of their decision; empty for none.
	Answer string
}

// appealDraft is an appeal that the appeal form sent and that was refused for its text: empty,
// too long, or not valid text.
type appealDraft struct {
	ActionID, Text string
}

// userLocalizer returns the localizer of a page for a signed-in user: their saved locale, then
// the request's (ADR-0009 §2).
func (h *Handler) userLocalizer(r *http.Request, userID string) *i18n.Localizer {
	var saved string
	if h.o.UserLocales != nil {
		var err error
		if saved, err = h.o.UserLocales.UserLocale(r.Context(), userID); err != nil {
			h.o.Logger.WarnContext(r.Context(), "read the user's locale", slog.String("user", userID), slog.Any("error", err))
		}
	}
	return h.o.Locales.UserLocalizer(r, saved)
}

// restrictionActions returns the actions in effect on the account of access that keep it out
// of the app, for the status page of a user whom moderation keeps out; none when moderation
// does not. The other actions in effect are in the app, once the user can use it again. draft,
// when not nil, goes back into the appeal form of its action.
func (h *Handler) restrictionActions(ctx context.Context, l *i18n.Localizer, access appidentity.Access, draft *appealDraft) ([]statusAction, error) {
	if !access.LoginRestricted || access.AccountID == "" || h.o.Moderation == nil {
		return nil, nil
	}
	vs, err := h.o.Moderation.OwnActions(ctx, access.AccountID, appmoderation.OwnActionFilter{Active: true}, maxStatusActions)
	if err != nil {
		return nil, err
	}
	now := h.o.Clock.Now()
	out := make([]statusAction, 0, len(vs))
	for _, v := range vs {
		if !slices.Contains(v.Kind.Restrictions(), moderation.RestrictLogin) {
			continue
		}
		a := statusAction{
			ID:       v.ID,
			Kind:     l.ServerModerationActionKind(string(v.Kind)),
			Category: l.ServerAuthStatusActionReason(l.ServerModerationActionCategory(string(v.ReasonCategory))),
			Reason:   v.PublicReason,
			Ends:     l.ServerAuthStatusActionUntilLifted(),
		}
		switch {
		case v.Kind == moderation.ActionBan && access.DeletionDueAt != nil:
			// A ban never ends; it deletes the account.
			due := *access.DeletionDueAt
			a.Ends, a.EndsAt = l.ServerAuthStatusActionDeletesIn(daysUntil(now, due)), due.UTC().Format(time.RFC3339)
		case v.ExpiresAt != nil:
			a.Ends, a.EndsAt = l.ServerAuthStatusActionEndsIn(daysUntil(now, *v.ExpiresAt)), v.ExpiresAt.UTC().Format(time.RFC3339)
		}
		describeAppeal(&a, l, v, now)
		if draft != nil && draft.ActionID == v.ID && a.CanAppeal {
			a.AppealText, a.AppealError = draft.Text, l.ServerAuthStatusAppealTextInvalid(float64(moderation.MaxAppealText))
		}
		out = append(out, a)
	}
	return out, nil
}

// describeAppeal fills in what the status page says of appealing v: the appeal, the form, or
// why there is no form.
func describeAppeal(a *statusAction, l *i18n.Localizer, v appmoderation.TargetActionView, now time.Time) {
	switch {
	case v.Appeal != nil:
		a.Appeal = &statusAppeal{State: l.ServerAuthStatusAppealState(string(v.Appeal.State)), Text: v.Appeal.Text, Answer: v.Appeal.DecisionReason}
	case v.Appealable && v.AppealDeadline != nil:
		a.CanAppeal, a.AppealMax = true, moderation.MaxAppealText
		a.AppealHint = l.ServerAuthStatusAppealHint(i18n.ServerAuthStatusAppealHintArgs{Days: daysUntil(now, *v.AppealDeadline), Max: float64(moderation.MaxAppealText)})
	case v.AppealDeadline != nil && now.After(*v.AppealDeadline):
		a.NoAppeal = l.ServerErrorsAppealWindowClosed()
	default:
		a.NoAppeal = l.ServerErrorsAppealNotAppealable()
	}
}

// systemClock is the clock of Options without one.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// daysUntil returns the days from now to t, rounded up and at least 1.
func daysUntil(now, t time.Time) int {
	return max(1, int(math.Ceil(t.Sub(now).Hours()/24)))
}

// appeal handles the appeal form of the status page: the signed-in user appeals an action on
// their own account. A refused appeal shows the status page again with the reason.
func (h *Handler) appeal(w http.ResponseWriter, r *http.Request) {
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
	if access.AccountID == "" || h.o.Moderation == nil {
		// Nothing to appeal; the sign-in page says where the user stands.
		http.Redirect(w, r, PathSignIn, http.StatusSeeOther)
		return
	}
	actionID := r.PostForm.Get("action_id")
	// Browsers send the line breaks of a textarea as CRLF (HTML §4.10.21.8).
	text := strings.ReplaceAll(r.PostForm.Get("text"), "\r\n", "\n")
	refuse := func(status int, alert func(*i18n.Localizer) string) {
		h.renderStatusPage(w, r, status, session.UserID, access, nil, alert, nil)
	}
	_, err = h.o.Moderation.FileAppeal(r.Context(), access.AccountID, actionID, text)
	var textErr *moderation.TextError
	switch {
	case err == nil:
		h.renderStatusPage(w, r, http.StatusOK, session.UserID, access, (*i18n.Localizer).ServerAuthStatusAppealSent, nil, nil)
	case errors.Is(err, appmoderation.ErrNotFound):
		refuse(http.StatusNotFound, (*i18n.Localizer).ServerErrorsNotFound)
	case errors.Is(err, appmoderation.ErrAlreadyAppealed):
		refuse(http.StatusUnprocessableEntity, (*i18n.Localizer).ServerErrorsAppealAlreadyAppealed)
	case errors.Is(err, appmoderation.ErrAppealWindowClosed):
		refuse(http.StatusUnprocessableEntity, (*i18n.Localizer).ServerErrorsAppealWindowClosed)
	case errors.Is(err, appmoderation.ErrNotAppealable):
		refuse(http.StatusUnprocessableEntity, (*i18n.Localizer).ServerErrorsAppealNotAppealable)
	case errors.As(err, &textErr):
		draft := &appealDraft{ActionID: actionID, Text: text}
		h.renderStatusPage(w, r, http.StatusUnprocessableEntity, session.UserID, access, nil, (*i18n.Localizer).ServerAuthStatusAppealNotSent, draft)
	default:
		h.internalError(w, r, err)
	}
}
