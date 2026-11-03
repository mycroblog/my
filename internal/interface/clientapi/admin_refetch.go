// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The refetch of remote posts from their servers (FR-407, federation.md §6.4): the admin
// operations that queue it (manage_domains). Nothing schedules a refetch on its own.

// Refetches queues the refetch of remote posts. *appfederation.Refetches implements it.
type Refetches interface {
	RefetchStatus(ctx context.Context, statusID, byUserID string) error
	RefetchAccount(ctx context.Context, accountID, byUserID string) error
	RefetchDomain(ctx context.Context, domain, byUserID string) (string, error)
}

// requireRefetches checks the manage_domains permission and that the use cases are configured;
// it returns the problem to answer otherwise, and the viewer.
func (s *Server) requireRefetches(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.Refetches == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

// refetchProblem returns the problem that err of a refetch means to the client, and false for
// an unexpected error.
func (s *Server) refetchProblem(ctx context.Context, err error, param string) (problem, bool) {
	switch {
	case errors.Is(err, appfederation.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, appfederation.ErrInvalidHost):
		return s.invalidParameterResponse(ctx, param), true
	case errors.Is(err, appfederation.ErrNotRemote):
		l := s.Localizer(ctx)
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsRefetchNotRemote()), true
	}
	return problem{}, false
}

// RefetchStatus serves POST /api/v1/admin/statuses/{id}/refetch.
func (s *Server) RefetchStatus(ctx context.Context, req RefetchStatusRequestObject) (RefetchStatusResponseObject, error) {
	v, p, err := s.requireRefetches(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RefetchStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err := s.o.Refetches.RefetchStatus(ctx, req.Id, v.UserID); err != nil {
		if p, ok := s.refetchProblem(ctx, err, "id"); ok {
			return RefetchStatusdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: refetch status %s: %w", req.Id, err)
	}
	return RefetchStatus202Response{}, nil
}

// RefetchAccountStatuses serves POST /api/v1/admin/accounts/{id}/refetch_statuses.
func (s *Server) RefetchAccountStatuses(ctx context.Context, req RefetchAccountStatusesRequestObject) (RefetchAccountStatusesResponseObject, error) {
	v, p, err := s.requireRefetches(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RefetchAccountStatusesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err := s.o.Refetches.RefetchAccount(ctx, req.Id, v.UserID); err != nil {
		if p, ok := s.refetchProblem(ctx, err, "id"); ok {
			return RefetchAccountStatusesdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: refetch the statuses of account %s: %w", req.Id, err)
	}
	return RefetchAccountStatuses202Response{}, nil
}

// RefetchInstanceStatuses serves POST /api/v1/admin/instances/{domain}/refetch_statuses.
func (s *Server) RefetchInstanceStatuses(ctx context.Context, req RefetchInstanceStatusesRequestObject) (RefetchInstanceStatusesResponseObject, error) {
	v, p, err := s.requireRefetches(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RefetchInstanceStatusesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if _, err := s.o.Refetches.RefetchDomain(ctx, req.Domain, v.UserID); err != nil {
		if p, ok := s.refetchProblem(ctx, err, "domain"); ok {
			return RefetchInstanceStatusesdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: refetch the statuses of %s: %w", req.Domain, err)
	}
	return RefetchInstanceStatuses202Response{}, nil
}
