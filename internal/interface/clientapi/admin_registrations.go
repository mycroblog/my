// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The admin operations of the registration review (FR-721, manage_accounts): the sign-ups that
// wait for a moderator in approval mode, and the moderator's decision on one. The decision is
// the use case of `my registration approve|reject`, which records it in registration_decisions
// and the audit log (data-model §4.1, ADR-0037).

// Registrations runs the moderators' use cases on registrations. *appidentity.Service
// implements it.
type Registrations interface {
	ListRegistrationsAfter(ctx context.Context, state identity.RegistrationState, after appidentity.RegistrationPosition, limit int) ([]appidentity.RegistrationEntry, error)
	Registration(ctx context.Context, userID string) (appidentity.RegistrationEntry, error)
	DecideRegistration(ctx context.Context, in appidentity.DecideInput) (identity.RegistrationDecision, error)
}

// registrationCursorSeparator separates the time and the user of a registration cursor.
const registrationCursorSeparator = " "

func encodeRegistrationCursor(p appidentity.RegistrationPosition) *Cursor {
	return encodeCursor(p.Since.UTC().Format(time.RFC3339Nano) + registrationCursorSeparator + p.UserID)
}

func decodeRegistrationCursor(position string) (appidentity.RegistrationPosition, bool) {
	at, user, ok := strings.Cut(position, registrationCursorSeparator)
	if !ok || !kernel.ValidID(user) {
		return appidentity.RegistrationPosition{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return appidentity.RegistrationPosition{}, false
	}
	return appidentity.RegistrationPosition{Since: t, UserID: user}, true
}

func newAdminRegistration(e appidentity.RegistrationEntry) AdminRegistration {
	a := e.Application
	out := AdminRegistration{
		UserId: a.UserID, State: RegistrationState(e.State.State), Since: e.State.Since.UTC(),
		Email: e.Email, EmailVerified: e.EmailVerified, SubmittedAt: a.SubmittedAt.UTC(),
	}
	if e.AccountID != "" {
		out.AccountId = &e.AccountID
	}
	if e.Username != "" {
		out.Username = &e.Username
	}
	if a.Reason != "" {
		out.Reason = &a.Reason
	}
	if a.InviteID != "" {
		out.InviteId = &a.InviteID
	}
	return out
}

// ListPendingRegistrations serves GET /api/v1/admin/registrations, longest waiting first. The
// cursor is the position of the last registration of the previous page.
func (s *Server) ListPendingRegistrations(ctx context.Context, req ListPendingRegistrationsRequestObject) (ListPendingRegistrationsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return ListPendingRegistrationsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parseOpaquePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListPendingRegistrationsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	var after appidentity.RegistrationPosition
	if q.before != "" {
		var ok bool
		if after, ok = decodeRegistrationCursor(q.before); !ok {
			return ListPendingRegistrationsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
	}
	entries, err := s.o.Registrations.ListRegistrationsAfter(ctx, identity.RegistrationPending, after, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the pending registrations: %w", err)
	}
	entries, last := trim(entries, q.limit)
	page := AdminRegistrationPage{Items: make([]AdminRegistration, 0, len(entries))}
	for _, e := range entries {
		page.Items = append(page.Items, newAdminRegistration(e))
	}
	if last >= 0 {
		page.NextCursor = encodeRegistrationCursor(entries[last].Position())
	}
	cache := cacheNoStore
	return ListPendingRegistrations200JSONResponse{Body: page, Headers: ListPendingRegistrations200ResponseHeaders{CacheControl: &cache}}, nil
}

// ApproveRegistration serves POST /api/v1/admin/registrations/{user_id}/approve.
func (s *Server) ApproveRegistration(ctx context.Context, req ApproveRegistrationRequestObject) (ApproveRegistrationResponseObject, error) {
	res, p, err := s.decideRegistration(ctx, req.UserId, identity.DecisionApproved, req.Body)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return ApproveRegistrationdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return ApproveRegistration200JSONResponse{res}, nil
}

// RejectRegistration serves POST /api/v1/admin/registrations/{user_id}/reject.
func (s *Server) RejectRegistration(ctx context.Context, req RejectRegistrationRequestObject) (RejectRegistrationResponseObject, error) {
	res, p, err := s.decideRegistration(ctx, req.UserId, identity.DecisionRejected, req.Body)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return RejectRegistrationdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return RejectRegistration200JSONResponse{res}, nil
}

// decideRegistration decides on the registration of a user and answers with the registration
// as it is then. A registration that is already decided stays as it is and is answered alike.
func (s *Server) decideRegistration(ctx context.Context, userID string, d identity.Decision, body *RegistrationDecisionRequest) (AdminRegistrationJSONResponse, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil || p != nil {
		return AdminRegistrationJSONResponse{}, p, err
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return AdminRegistrationJSONResponse{}, nil, err
	}
	in := appidentity.DecideInput{UserID: userID, Decision: d, DecidedByUserID: v.UserID}
	if body != nil && body.Note != nil {
		in.Note = *body.Note
	}
	_, err = s.o.Registrations.DecideRegistration(ctx, in)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		p := s.notFoundResponse(ctx)
		return AdminRegistrationJSONResponse{}, &p, nil
	case errors.Is(err, identity.ErrInvalidNote):
		l := s.Localizer(ctx)
		detail := l.ServerErrorsInvalidValue("note")
		if utf8.RuneCountInString(in.Note) > identity.MaxDecisionNoteLength {
			detail = l.ServerErrorsTooLong(identity.MaxDecisionNoteLength, "note")
		}
		p := s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)
		return AdminRegistrationJSONResponse{}, &p, nil
	case errors.Is(err, appidentity.ErrServiceClosing):
		p := s.serviceClosingProblem(ctx)
		return AdminRegistrationJSONResponse{}, &p, nil
	case err != nil && !errors.Is(err, appidentity.ErrNotPending):
		return AdminRegistrationJSONResponse{}, nil, fmt.Errorf("clientapi: decide the registration of %s: %w", userID, err)
	}
	e, err := s.o.Registrations.Registration(ctx, userID)
	if err != nil {
		return AdminRegistrationJSONResponse{}, nil, fmt.Errorf("clientapi: the registration of %s: %w", userID, err)
	}
	cache := cacheNoStore
	return AdminRegistrationJSONResponse{Body: newAdminRegistration(e), Headers: AdminRegistrationResponseHeaders{CacheControl: &cache}}, nil, nil
}
