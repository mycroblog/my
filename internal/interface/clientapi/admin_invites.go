// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The invites (FR-001, manage_invites): the admin operations that create, list, and revoke the
// codes that let people sign up while the registration mode is open, approval, or invite
// (ADR-0108 §4): the staff's invites and those that members created (invites.go). The CLI's
// `my invite` runs the same use cases.

// Invites runs the use cases of the invites. *appidentity.Service implements it.
type Invites interface {
	ListInvites(ctx context.Context, before string, limit int) ([]appidentity.InviteStatus, error)
	CreateInvite(ctx context.Context, in appidentity.CreateInviteInput) (identity.Invite, error)
	RevokeInviteByID(ctx context.Context, id, revokedByUserID string) (appidentity.InviteStatus, error)
}

// maxInviteExpiry bounds CreateInviteRequest.expires_in: ten years.
const maxInviteExpiry = 10 * 365 * 24 * time.Hour

func (s *Server) newAdminInvite(st appidentity.InviteStatus) AdminInvite {
	out := AdminInvite{
		Id: st.ID, Code: st.Code, Url: s.inviteURL(st.Code), Uses: st.Uses, State: AdminInviteState(st.State),
		CreatorKind: newInviteCreatorKind(st.CreatorKind), CreatedAt: st.CreatedAt.UTC(),
	}
	if st.MaxUses > 0 {
		out.MaxUses = &st.MaxUses
	}
	if st.ExpiresAt != nil {
		t := st.ExpiresAt.UTC()
		out.ExpiresAt = &t
	}
	return out
}

// newInviteCreatorKind presents how an invite was created; staff unless a member created it.
func newInviteCreatorKind(k identity.InviteCreatorKind) InviteCreatorKind {
	if k == identity.InviteByMember {
		return InviteCreatorKindMember
	}
	return InviteCreatorKindStaff
}

// withInviteCreators presents invites to the staff with the accounts of their creators
// (ADR-0108 §5), read once for all of them.
func (s *Server) withInviteCreators(ctx context.Context, list []appidentity.InviteStatus) ([]AdminInvite, error) {
	if s.o.UserAccounts == nil || s.o.Accounts == nil {
		out := make([]AdminInvite, len(list))
		for i, st := range list {
			out[i] = s.newAdminInvite(st)
		}
		return out, nil
	}
	ids := make([]string, len(list))
	for i, st := range list {
		ids[i] = st.CreatedByUserID
	}
	accounts, err := s.userAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]AdminInvite, len(list))
	for i, st := range list {
		out[i] = s.newAdminInvite(st)
		if u := st.CreatedByUserID; u != "" {
			if a, ok := accounts[u]; ok {
				out[i].CreatedBy = &a
			} else {
				out[i].CreatedByDeleted = true
			}
		}
	}
	return out, nil
}

func (s *Server) adminInviteResponse(ctx context.Context, st appidentity.InviteStatus) (AdminInviteJSONResponse, error) {
	presented, err := s.withInviteCreators(ctx, []appidentity.InviteStatus{st})
	if err != nil {
		return AdminInviteJSONResponse{}, err
	}
	cache := cacheNoStore
	return AdminInviteJSONResponse{Body: presented[0], Headers: AdminInviteResponseHeaders{CacheControl: &cache}}, nil
}

// ListInvites serves GET /api/v1/admin/invites, newest first. The cursor is the ID of the last
// invite of the previous page.
func (s *Server) ListInvites(ctx context.Context, req ListInvitesRequestObject) (ListInvitesResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageInvites); err != nil {
		return nil, err
	} else if p != nil {
		return ListInvitesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListInvitesdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	list, err := s.o.Invites.ListInvites(ctx, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list invites: %w", err)
	}
	list, last := trim(list, q.limit)
	items, err := s.withInviteCreators(ctx, list)
	if err != nil {
		return nil, err
	}
	page := AdminInvitePage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(list[last].ID)
	}
	cache := cacheNoStore
	return ListInvites200JSONResponse{Body: page, Headers: ListInvites200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateInvite serves POST /api/v1/admin/invites.
func (s *Server) CreateInvite(ctx context.Context, req CreateInviteRequestObject) (CreateInviteResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageInvites); err != nil {
		return nil, err
	} else if p != nil {
		return CreateInvitedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(name string) CreateInviteResponseObject {
		return CreateInvitedefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(name)))
	}
	in := appidentity.CreateInviteInput{CreatedByUserID: v.UserID}
	if b := req.Body; b != nil {
		if b.MaxUses != nil {
			if *b.MaxUses < 1 || *b.MaxUses > appidentity.MaxInviteUses {
				return unprocessable("max_uses"), nil
			}
			in.MaxUses = *b.MaxUses
		}
		if b.ExpiresIn != nil {
			if *b.ExpiresIn < 1 || *b.ExpiresIn > int(maxInviteExpiry/time.Second) {
				return unprocessable("expires_in"), nil
			}
			in.ExpiresIn = time.Duration(*b.ExpiresIn) * time.Second
		}
	}
	inv, err := s.o.Invites.CreateInvite(ctx, in)
	if errors.Is(err, appidentity.ErrRegistrationClosed) {
		return CreateInvitedefaultApplicationProblemPlusJSONResponse(*s.inviteProblem(ctx, err)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create invite: %w", err)
	}
	resp, err := s.adminInviteResponse(ctx, appidentity.InviteStatus{Invite: inv, State: appidentity.InviteUsable})
	if err != nil {
		return nil, err
	}
	return CreateInvite200JSONResponse{resp}, nil
}

// RevokeInvite serves POST /api/v1/admin/invites/{id}/revoke.
func (s *Server) RevokeInvite(ctx context.Context, req RevokeInviteRequestObject) (RevokeInviteResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageInvites); err != nil {
		return nil, err
	} else if p != nil {
		return RevokeInvitedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.o.Invites.RevokeInviteByID(ctx, req.Id, v.UserID)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return RevokeInvitedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: revoke invite %s: %w", req.Id, err)
	}
	resp, err := s.adminInviteResponse(ctx, st)
	if err != nil {
		return nil, err
	}
	return RevokeInvite200JSONResponse{resp}, nil
}
