// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The service closure (FR-808, FR-809, ADR-0088 §1): the admin operations that read, announce,
// and withdraw it, check the server before the closure, and close it (manage_service_closure),
// and Instance.closure of GET /instance.

// ProblemTypeServiceClosureState is the problem type of an operation that the state of the
// service closure does not allow (409).
const ProblemTypeServiceClosureState = "/api/v1/problems/service-closure-state"

// ServiceClosures runs the use cases of the service closure. *appinstance.Service implements it.
type ServiceClosures interface {
	// Closure returns the current state of the service closure, the zero value for none.
	Closure(ctx context.Context) (instance.Closure, error)
	// AnnounceClosure records an announcement or a change of its dates, audited.
	AnnounceClosure(ctx context.Context, in appinstance.AnnounceClosureInput) (instance.Closure, error)
	// WithdrawClosure withdraws the announcement in effect, audited.
	WithdrawClosure(ctx context.Context, byUserID string) (instance.Closure, error)
	// CheckClosure counts what the closure would end.
	CheckClosure(ctx context.Context) (appinstance.ClosureCheck, error)
	// CloseService closes the server for good, audited.
	CloseService(ctx context.Context, byUserID string) (instance.Closure, error)
	// ClosureProgress returns how far the closure's Deletes have gone, and false while the
	// server is not closed.
	ClosureProgress(ctx context.Context) (appinstance.ClosureProgress, bool, error)
}

// adminServiceClosureProgress returns AdminServiceClosure.progress.
func adminServiceClosureProgress(p appinstance.ClosureProgress) *AdminServiceClosureProgress {
	d := p.Deliveries
	return &AdminServiceClosureProgress{
		Accounts: p.Accounts, FanoutsPending: p.FanOuts,
		Deliveries: AdminServiceClosureDeliveries{
			Pending: d["pending"], Succeeded: d["succeeded"], PermanentFailure: d["permanent_failure"], Skipped: d["skipped"], Dead: d["dead"],
		},
	}
}

func adminServiceClosure(c instance.Closure) AdminServiceClosure {
	out := AdminServiceClosure{State: ServiceClosureState(c.CurrentState()), ShortNotice: c.ShortNotice()}
	if c.CurrentState() == instance.ClosureNone {
		return out
	}
	out.ClosesAt, out.ExportUntil = utcPtr(c.Schedule.ClosesAt), utcPtr(c.Schedule.ExportUntil)
	out.AnnouncedAt, out.ChangedAt = utcPtr(c.AnnouncedAt), utcPtr(c.ChangedAt)
	if c.Schedule.Message != "" {
		out.Message = &c.Schedule.Message
	}
	return out
}

// instanceClosure returns Instance.closure, or nil when no closure is announced.
func (s *Server) instanceClosure(ctx context.Context) (*InstanceClosure, error) {
	if s.o.Closures == nil {
		return nil, nil
	}
	c, err := s.o.Closures.Closure(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: service closure: %w", err)
	}
	if c.CurrentState() == instance.ClosureNone {
		return nil, nil
	}
	out := &InstanceClosure{
		State: ServiceClosureState(c.State), ClosesAt: c.Schedule.ClosesAt.UTC(), ExportUntil: c.Schedule.ExportUntil.UTC(),
	}
	if c.Schedule.Message != "" {
		out.Message = &c.Schedule.Message
	}
	return out, nil
}

// closureProblem returns the problem of a refused change of the service closure, and false for
// an error that is not one.
func (s *Server) closureProblem(ctx context.Context, err error) (problem, bool) {
	switch {
	case errors.Is(err, instance.ErrClosureMessage):
		return s.invalidParameterResponse(ctx, "message"), true
	case errors.Is(err, instance.ErrClosureSchedule):
		return s.invalidParameterResponse(ctx, "export_until"), true
	case errors.Is(err, instance.ErrClosureTransition):
		return s.problem(ctx, http.StatusConflict, ProblemTypeServiceClosureState, s.Localizer(ctx).ServerErrorsServiceClosureState(), ""), true
	}
	return problem{}, false
}

func closureResponse(c instance.Closure) AdminServiceClosureJSONResponse {
	cache := cacheNoStore
	return AdminServiceClosureJSONResponse{Body: adminServiceClosure(c), Headers: AdminServiceClosureResponseHeaders{CacheControl: &cache}}
}

// GetAdminServiceClosure serves GET /api/v1/admin/closure.
func (s *Server) GetAdminServiceClosure(ctx context.Context, _ GetAdminServiceClosureRequestObject) (GetAdminServiceClosureResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageServiceClosure); err != nil {
		return nil, err
	} else if p != nil {
		return GetAdminServiceClosuredefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if s.o.Closures == nil {
		return GetAdminServiceClosuredefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	c, err := s.o.Closures.Closure(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: admin service closure: %w", err)
	}
	res := closureResponse(c)
	progress, ok, err := s.o.Closures.ClosureProgress(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: admin service closure: %w", err)
	}
	if ok {
		res.Body.Progress = adminServiceClosureProgress(progress)
	}
	return GetAdminServiceClosure200JSONResponse{res}, nil
}

// AnnounceServiceClosure serves PUT /api/v1/admin/closure/announcement.
func (s *Server) AnnounceServiceClosure(ctx context.Context, req AnnounceServiceClosureRequestObject) (AnnounceServiceClosureResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageServiceClosure); err != nil {
		return nil, err
	} else if p != nil {
		return AnnounceServiceClosuredefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if s.o.Closures == nil {
		return AnnounceServiceClosuredefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return AnnounceServiceClosuredefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	in := appinstance.AnnounceClosureInput{ClosesAt: req.Body.ClosesAt, ByUserID: v.UserID}
	if req.Body.ExportUntil != nil {
		in.ExportUntil = *req.Body.ExportUntil
	}
	if req.Body.Message != nil {
		in.Message = *req.Body.Message
	}
	c, err := s.o.Closures.AnnounceClosure(ctx, in)
	if p, ok := s.closureProblem(ctx, err); ok {
		return AnnounceServiceClosuredefaultApplicationProblemPlusJSONResponse(p), nil
	} else if err != nil {
		return nil, fmt.Errorf("clientapi: announce the service closure: %w", err)
	}
	return AnnounceServiceClosure200JSONResponse{closureResponse(c)}, nil
}

// WithdrawServiceClosure serves DELETE /api/v1/admin/closure/announcement.
func (s *Server) WithdrawServiceClosure(ctx context.Context, _ WithdrawServiceClosureRequestObject) (WithdrawServiceClosureResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageServiceClosure); err != nil {
		return nil, err
	} else if p != nil {
		return WithdrawServiceClosuredefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if s.o.Closures == nil {
		return WithdrawServiceClosuredefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.o.Closures.WithdrawClosure(ctx, v.UserID)
	if p, ok := s.closureProblem(ctx, err); ok {
		return WithdrawServiceClosuredefaultApplicationProblemPlusJSONResponse(p), nil
	} else if err != nil {
		return nil, fmt.Errorf("clientapi: withdraw the service closure: %w", err)
	}
	return WithdrawServiceClosure200JSONResponse{closureResponse(c)}, nil
}

// GetAdminServiceClosureCheck serves GET /api/v1/admin/closure/check.
func (s *Server) GetAdminServiceClosureCheck(ctx context.Context, _ GetAdminServiceClosureCheckRequestObject) (GetAdminServiceClosureCheckResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageServiceClosure); err != nil {
		return nil, err
	} else if p != nil {
		return GetAdminServiceClosureCheckdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if s.o.Closures == nil {
		return GetAdminServiceClosureCheckdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	c, err := s.o.Closures.CheckClosure(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: check the service closure: %w", err)
	}
	cache := cacheNoStore
	return GetAdminServiceClosureCheck200JSONResponse{AdminServiceClosureCheckJSONResponse{
		Body: AdminServiceClosureCheck{
			LocalAccounts: c.LocalAccounts, DeliveryInboxes: c.DeliveryInboxes, EstimatedDeletes: c.EstimatedDeletes,
			LiveSupporterSubscriptions: c.LiveSupporterSubscriptions, PendingMigrations: c.PendingMigrations,
			RetentionHolds: c.RetentionHolds,
		},
		Headers: AdminServiceClosureCheckResponseHeaders{CacheControl: &cache},
	}}, nil
}

// CloseService serves POST /api/v1/admin/closure/close: confirm_domain must be the server's
// domain (Options.Host), compared without regard to case.
func (s *Server) CloseService(ctx context.Context, req CloseServiceRequestObject) (CloseServiceResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageServiceClosure); err != nil {
		return nil, err
	} else if p != nil {
		return CloseServicedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if s.o.Closures == nil {
		return CloseServicedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || !strings.EqualFold(strings.TrimSpace(req.Body.ConfirmDomain), s.o.Host) {
		return CloseServicedefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "confirm_domain")), nil
	}
	c, err := s.o.Closures.CloseService(ctx, v.UserID)
	if p, ok := s.closureProblem(ctx, err); ok {
		return CloseServicedefaultApplicationProblemPlusJSONResponse(p), nil
	} else if err != nil {
		return nil, fmt.Errorf("clientapi: close the service: %w", err)
	}
	return CloseService200JSONResponse{closureResponse(c)}, nil
}
