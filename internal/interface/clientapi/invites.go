// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The invites that members create of their own (ADR-0108): GET and POST /api/v1/invites and
// POST /api/v1/invites/{id}/revoke, under the caps of the instance settings.

// OwnInvites runs the use cases of the invites that members create. *appidentity.Service
// implements it.
type OwnInvites interface {
	ListOwnInvites(ctx context.Context, userID, before string, limit int) (appidentity.OwnInvites, error)
	CreateOwnInvite(ctx context.Context, in appidentity.CreateOwnInviteInput) (appidentity.InviteStatus, error)
	RevokeOwnInvite(ctx context.Context, userID, inviteID string) (appidentity.InviteStatus, error)
}

// Problem types of the invites.
const (
	// ProblemTypeInvitesNotAllowed is the problem of a user who may not create invites now.
	ProblemTypeInvitesNotAllowed = "/api/v1/problems/invites-not-allowed"
	// ProblemTypeTooManyInvites is the problem of a member who holds as many usable invites as
	// member_invite_max_active allows.
	ProblemTypeTooManyInvites = "/api/v1/problems/too-many-invites"
	// ProblemTypeRegistrationClosed is the problem of a staff invite while the registration
	// mode is closed.
	ProblemTypeRegistrationClosed = "/api/v1/problems/registration-closed"
)

// inviteURL returns the registration page with the invite's code filled in.
func (s *Server) inviteURL(code string) string {
	return s.origin() + s.o.SignUpPath + "?invite=" + url.QueryEscape(code)
}

func (s *Server) newOwnInvite(st appidentity.InviteStatus) OwnInvite {
	out := OwnInvite{
		Id: st.ID, Code: st.Code, Url: s.inviteURL(st.Code), MaxUses: st.MaxUses, Uses: st.Uses,
		State: AdminInviteState(st.State), CreatedAt: st.CreatedAt.UTC(),
	}
	if st.ExpiresAt != nil {
		out.ExpiresAt = st.ExpiresAt.UTC()
	}
	return out
}

func (s *Server) ownInviteResponse(st appidentity.InviteStatus) OwnInviteJSONResponse {
	cache := cacheNoStore
	return OwnInviteJSONResponse{Body: s.newOwnInvite(st), Headers: OwnInviteResponseHeaders{CacheControl: &cache}}
}

// inviteProblem returns the problem of an error of the invite use cases, or nil for an
// unexpected error.
func (s *Server) inviteProblem(ctx context.Context, err error) *problem {
	l := s.Localizer(ctx)
	var p problem
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		p = s.notFoundResponse(ctx)
	case errors.Is(err, identity.ErrInviteUsesOverCap):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("max_uses"))
	case errors.Is(err, identity.ErrInviteExpiryOverCap):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("expires_in"))
	case errors.Is(err, appidentity.ErrInvitesNotAllowed):
		p = s.problem(ctx, http.StatusForbidden, ProblemTypeInvitesNotAllowed, l.ServerErrorsInvitesNotAllowed(), "")
	case errors.Is(err, appidentity.ErrTooManyInvites):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeTooManyInvites, l.ServerErrorsInvitesTooMany(), "")
	case errors.Is(err, appidentity.ErrRegistrationClosed):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeRegistrationClosed, l.ServerErrorsInvitesRegistrationClosed(), "")
	default:
		return nil
	}
	return &p
}

// ListOwnInvites serves GET /api/v1/invites, newest first. The cursor is the ID of the last
// invite of the previous page.
func (s *Server) ListOwnInvites(ctx context.Context, req ListOwnInvitesRequestObject) (ListOwnInvitesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnInvites == nil {
		return ListOwnInvitesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListOwnInvitesdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	own, err := s.o.OwnInvites.ListOwnInvites(ctx, v.UserID, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the invites of %s: %w", v.UserID, err)
	}
	list, last := trim(own.Invites, q.limit)
	caps := own.Caps
	page := OwnInvitePage{
		Items: make([]OwnInvite, 0, len(list)), CanCreate: own.MayCreate, Usable: own.Usable,
		Limits: MemberInviteLimits{
			MaxActive: caps.MaxActive, MaxUses: caps.MaxUses, MaxExpiresIn: int(caps.MaxExpiresIn / time.Second),
			DefaultMaxUses: caps.DefaultUses(), DefaultExpiresIn: int(caps.DefaultExpiresIn() / time.Second),
		},
	}
	for _, st := range list {
		page.Items = append(page.Items, s.newOwnInvite(st))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(list[last].ID)
	}
	cache := cacheNoStore
	return ListOwnInvites200JSONResponse{Body: page, Headers: ListOwnInvites200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateOwnInvite serves POST /api/v1/invites.
func (s *Server) CreateOwnInvite(ctx context.Context, req CreateOwnInviteRequestObject) (CreateOwnInviteResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnInvites == nil {
		return CreateOwnInvitedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	in := appidentity.CreateOwnInviteInput{UserID: v.UserID}
	if b := req.Body; b != nil {
		if b.MaxUses != nil {
			if *b.MaxUses < 1 {
				return CreateOwnInvitedefaultApplicationProblemPlusJSONResponse(*s.inviteProblem(ctx, identity.ErrInviteUsesOverCap)), nil
			}
			in.MaxUses = *b.MaxUses
		}
		if b.ExpiresIn != nil {
			if *b.ExpiresIn < 1 {
				return CreateOwnInvitedefaultApplicationProblemPlusJSONResponse(*s.inviteProblem(ctx, identity.ErrInviteExpiryOverCap)), nil
			}
			in.ExpiresIn = time.Duration(*b.ExpiresIn) * time.Second
		}
	}
	st, err := s.o.OwnInvites.CreateOwnInvite(ctx, in)
	if err != nil {
		if p := s.inviteProblem(ctx, err); p != nil {
			return CreateOwnInvitedefaultApplicationProblemPlusJSONResponse(*p), nil
		}
		return nil, fmt.Errorf("clientapi: create the invite of %s: %w", v.UserID, err)
	}
	return CreateOwnInvite200JSONResponse{s.ownInviteResponse(st)}, nil
}

// RevokeOwnInvite serves POST /api/v1/invites/{id}/revoke.
func (s *Server) RevokeOwnInvite(ctx context.Context, req RevokeOwnInviteRequestObject) (RevokeOwnInviteResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnInvites == nil {
		return RevokeOwnInvitedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	st, err := s.o.OwnInvites.RevokeOwnInvite(ctx, v.UserID, req.Id)
	if err != nil {
		if p := s.inviteProblem(ctx, err); p != nil {
			return RevokeOwnInvitedefaultApplicationProblemPlusJSONResponse(*p), nil
		}
		return nil, fmt.Errorf("clientapi: revoke invite %s: %w", req.Id, err)
	}
	return RevokeOwnInvite200JSONResponse{s.ownInviteResponse(st)}, nil
}
