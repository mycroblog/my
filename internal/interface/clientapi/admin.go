// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The admin operations (tag admin): the health of remote servers and the queue dashboard
// (admin_jobs.go). Their
// scopes (admin:read, admin:write) are checked before the handler runs, like every operation's;
// the handler then checks that the viewer's roles grant the operation's permission
// (requirePermission).

// ProblemTypeMissingPermission is the problem type of an admin operation that the viewer's roles
// do not allow; api/openapi.yaml documents it.
const ProblemTypeMissingPermission = "/api/v1/problems/missing-permission"

// Permissions reads the permissions of users' roles. *appidentity.Service implements it.
type Permissions interface {
	HasPermission(ctx context.Context, userID string, p identity.Permission) (bool, error)
	// Permissions lists the permissions of the user's roles.
	Permissions(ctx context.Context, userID string) ([]identity.Permission, error)
}

// permissions presents the permissions of the user's roles for CredentialAccount; without
// Options.Permissions there are none.
func (s *Server) permissions(ctx context.Context, userID string) ([]Permission, error) {
	out := []Permission{}
	if s.o.Permissions == nil {
		return out, nil
	}
	perms, err := s.o.Permissions.Permissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: permissions of %s: %w", userID, err)
	}
	for _, p := range perms {
		out = append(out, Permission(p))
	}
	return out, nil
}

// InstanceHealth reads the health of remote servers and clears it (federation.md §7.3).
// *appfederation.Health implements it.
type InstanceHealth interface {
	Find(ctx context.Context, domain string) (federation.InstanceHealth, error)
	List(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.InstanceHealth, error)
	Events(ctx context.Context, domain, before string, limit int) ([]federation.AvailabilityEvent, error)
	MarkAvailable(ctx context.Context, domain, byUserID string) (federation.InstanceHealth, error)
}

// requirePermission returns the 403 problem of a viewer whose roles do not grant p, or nil when
// they do.
func (s *Server) requirePermission(ctx context.Context, p identity.Permission) (*problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	ok := false
	if s.o.Permissions != nil {
		ok, err = s.o.Permissions.HasPermission(ctx, v.UserID, p)
		if err != nil {
			return nil, fmt.Errorf("clientapi: permissions of %s: %w", v.UserID, err)
		}
	}
	if ok {
		return nil, nil
	}
	l := s.Localizer(ctx)
	pr := s.problem(ctx, http.StatusForbidden, ProblemTypeMissingPermission, l.ServerErrorsForbidden(), l.ServerErrorsMissingPermissionDetail(string(p)))
	return &pr, nil
}

// normalizeDomain returns the form in which the federation module keys a server's domain.
func normalizeDomain(d string) string { return strings.ToLower(d) }

func newAdminInstanceHealth(h federation.InstanceHealth) AdminInstanceHealth {
	out := AdminInstanceHealth{Domain: h.Domain, State: AdminInstanceState(h.State), FailureDays: h.FailureDays}
	if !h.Since.IsZero() {
		since := h.Since.UTC()
		out.Since = &since
	}
	if day, err := time.Parse("20060102", h.LastFailureDay); err == nil {
		out.LastFailureDay = &openapi_types.Date{Time: day}
	}
	return out
}

// ListInstanceHealth serves GET /api/v1/admin/instances: the servers with failures or marked
// unavailable, in domain order. The cursor is the domain of the last server of the previous
// page.
func (s *Server) ListInstanceHealth(ctx context.Context, req ListInstanceHealthRequestObject) (ListInstanceHealthResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListInstanceHealthdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parseOpaquePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListInstanceHealthdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	var state federation.InstanceState
	if req.Params.State != nil {
		state = federation.InstanceState(*req.Params.State)
	}
	hs, err := s.o.InstanceHealth.List(ctx, state, q.before, q.limit+1)
	if errors.Is(err, appfederation.ErrInvalidInstanceState) {
		return ListInstanceHealthdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: list instance health: %w", err)
	}
	hs, last := trim(hs, q.limit)
	page := AdminInstanceHealthPage{Items: make([]AdminInstanceHealth, 0, len(hs))}
	for _, h := range hs {
		page.Items = append(page.Items, newAdminInstanceHealth(h))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(hs[last].Domain)
	}
	cache := cacheNoStore
	return ListInstanceHealth200JSONResponse{Body: page, Headers: ListInstanceHealth200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetInstanceHealth serves GET /api/v1/admin/instances/{domain}.
func (s *Server) GetInstanceHealth(ctx context.Context, req GetInstanceHealthRequestObject) (GetInstanceHealthResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return GetInstanceHealthdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	h, err := s.o.InstanceHealth.Find(ctx, normalizeDomain(req.Domain))
	if err != nil {
		return nil, fmt.Errorf("clientapi: health of %s: %w", req.Domain, err)
	}
	cache := cacheNoStore
	return GetInstanceHealth200JSONResponse{AdminInstanceHealthJSONResponse{
		Body: newAdminInstanceHealth(h), Headers: AdminInstanceHealthResponseHeaders{CacheControl: &cache},
	}}, nil
}

// ListInstanceAvailabilityEvents serves GET /api/v1/admin/instances/{domain}/availability_events,
// newest first. The cursor is the ID of the last event of the previous page.
func (s *Server) ListInstanceAvailabilityEvents(ctx context.Context, req ListInstanceAvailabilityEventsRequestObject) (ListInstanceAvailabilityEventsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListInstanceAvailabilityEventsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListInstanceAvailabilityEventsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	evs, err := s.o.InstanceHealth.Events(ctx, normalizeDomain(req.Domain), q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: availability events of %s: %w", req.Domain, err)
	}
	evs, last := trim(evs, q.limit)
	page := AdminAvailabilityEventPage{Items: make([]AdminAvailabilityEvent, 0, len(evs))}
	for _, e := range evs {
		page.Items = append(page.Items, AdminAvailabilityEvent{
			Id: e.ID, Kind: AdminAvailabilityEventKind(e.Kind), Reason: AdminAvailabilityEventReason(e.Reason),
			OccurredAt: e.OccurredAt.UTC(),
		})
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(evs[last].ID)
	}
	cache := cacheNoStore
	return ListInstanceAvailabilityEvents200JSONResponse{Body: page, Headers: ListInstanceAvailabilityEvents200ResponseHeaders{CacheControl: &cache}}, nil
}

// MarkInstanceAvailable serves POST /api/v1/admin/instances/{domain}/mark_available.
func (s *Server) MarkInstanceAvailable(ctx context.Context, req MarkInstanceAvailableRequestObject) (MarkInstanceAvailableResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return MarkInstanceAvailabledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	h, err := s.o.InstanceHealth.MarkAvailable(ctx, normalizeDomain(req.Domain), v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: mark %s available: %w", req.Domain, err)
	}
	cache := cacheNoStore
	return MarkInstanceAvailable200JSONResponse{AdminInstanceHealthJSONResponse{
		Body: newAdminInstanceHealth(h), Headers: AdminInstanceHealthResponseHeaders{CacheControl: &cache},
	}}, nil
}
