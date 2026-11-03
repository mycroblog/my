// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Creating, changing, and deleting roles (FR-720, manage_roles). The use cases keep the built-in
// roles, check the rank and the permissions of the viewer, and audit every change.

// requireRoleManager checks the manage_roles permission and returns the viewer, or the problem
// to answer.
func (s *Server) requireRoleManager(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRoles); err != nil || p != nil {
		return Viewer{}, p, err
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

// roleDefinitionProblem maps the errors of CreateRole, UpdateRole, and DeleteRole: a 422 for a
// change that breaks a rule (rp), or 404, 403, and the 409 of the roles of the supporter plans
// (p). It reports false for any other error.
func (s *Server) roleDefinitionProblem(ctx context.Context, err error) (*RoleProblemApplicationProblemPlusJSONResponse, *problem, bool) {
	var reason RoleProblemReason
	var missing *appidentity.MissingPermissionsError
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		p := s.notFoundResponse(ctx)
		return nil, &p, true
	case errors.Is(err, appidentity.ErrRoleOutranksActor):
		p := s.problem(ctx, http.StatusForbidden, ProblemTypeBlank, s.Localizer(ctx).ServerErrorsForbidden(), "")
		return nil, &p, true
	case errors.Is(err, appidentity.ErrInvalidRoleKey):
		reason = RoleProblemReasonKey
	case errors.Is(err, appidentity.ErrRoleKeyTaken):
		reason = RoleProblemReasonKeyTaken
	case errors.Is(err, appidentity.ErrInvalidRolePosition):
		reason = RoleProblemReasonPosition
	case errors.Is(err, appidentity.ErrInvalidRolePermissions):
		reason = RoleProblemReasonPermissions
	case errors.Is(err, appidentity.ErrBuiltinRole):
		reason = RoleProblemReasonBuiltin
	case errors.Is(err, appidentity.ErrRoleHeld):
		reason = RoleProblemReasonHeld
	case errors.As(err, &missing):
		reason = RoleProblemReasonMissingPermission
	case errors.Is(err, appidentity.ErrInvalidBadgeLabel):
		reason = RoleProblemReasonBadgeLabel
	case errors.Is(err, appidentity.ErrInvalidBadgeColor):
		reason = RoleProblemReasonBadgeColor
	default:
		if p, ok := s.managedRoleProblem(ctx, err); ok {
			return nil, &p, true
		}
		return nil, nil, false
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidRole,
		l.ServerErrorsRoleInvalid(), l.ServerErrorsRoleInvalidDetail(string(reason)))
	out := RoleProblem{
		Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
	}
	if missing != nil {
		perms := make([]Permission, len(missing.Missing))
		for i, p := range missing.Missing {
			perms[i] = Permission(p)
		}
		out.MissingPermissions = &perms
	}
	return &RoleProblemApplicationProblemPlusJSONResponse{
		Body: out, Headers: RoleProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}, nil, true
}

func adminRoleResponse(r appidentity.RoleWithPermissions) AdminRoleJSONResponse {
	cache := cacheNoStore
	return AdminRoleJSONResponse{Body: newAdminRole(r), Headers: AdminRoleResponseHeaders{CacheControl: &cache}}
}

func permissionsFrom(in []Permission) []identity.Permission {
	out := make([]identity.Permission, len(in))
	for i, p := range in {
		out[i] = identity.Permission(p)
	}
	return out
}

// CreateRole serves POST /api/v1/admin/roles.
func (s *Server) CreateRole(ctx context.Context, req CreateRoleRequestObject) (CreateRoleResponseObject, error) {
	v, p, err := s.requireRoleManager(ctx)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return CreateRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	case req.Body == nil:
		return CreateRoledefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	r, err := s.o.Roles.CreateRole(ctx, appidentity.CreateRoleInput{
		Key: req.Body.Key, Position: req.Body.Position, Permissions: permissionsFrom(req.Body.Permissions), ByUserID: v.UserID,
	})
	if rp, p, ok := s.roleDefinitionProblem(ctx, err); ok {
		if rp != nil {
			return CreateRole422ApplicationProblemPlusJSONResponse{*rp}, nil
		}
		return CreateRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create role %s: %w", req.Body.Key, err)
	}
	return CreateRole201JSONResponse{adminRoleResponse(r)}, nil
}

// UpdateRole serves PATCH /api/v1/admin/roles/{role}.
func (s *Server) UpdateRole(ctx context.Context, req UpdateRoleRequestObject) (UpdateRoleResponseObject, error) {
	v, p, err := s.requireRoleManager(ctx)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UpdateRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	case req.Body == nil:
		return UpdateRoledefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	remove := req.Body.RemoveBadge != nil && *req.Body.RemoveBadge
	if remove && req.Body.Badge != nil {
		return UpdateRoledefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "remove_badge")), nil
	}
	in := appidentity.UpdateRoleInput{Key: req.Role, Position: req.Body.Position, ByUserID: v.UserID, SetBadge: remove}
	if req.Body.Permissions != nil {
		in.Permissions, in.SetPermissions = permissionsFrom(*req.Body.Permissions), true
	}
	if b := req.Body.Badge; b != nil {
		in.Badge, in.SetBadge = &appidentity.BadgeInput{Label: b.Label, Color: identity.BadgeColor(b.Color)}, true
	}
	r, err := s.o.Roles.UpdateRole(ctx, in)
	if rp, p, ok := s.roleDefinitionProblem(ctx, err); ok {
		if rp != nil {
			return UpdateRole422ApplicationProblemPlusJSONResponse{*rp}, nil
		}
		return UpdateRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update role %s: %w", req.Role, err)
	}
	return UpdateRole200JSONResponse{adminRoleResponse(r)}, nil
}

// DeleteRole serves DELETE /api/v1/admin/roles/{role}.
func (s *Server) DeleteRole(ctx context.Context, req DeleteRoleRequestObject) (DeleteRoleResponseObject, error) {
	v, p, err := s.requireRoleManager(ctx)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return DeleteRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.Roles.DeleteRole(ctx, req.Role, v.UserID)
	if rp, p, ok := s.roleDefinitionProblem(ctx, err); ok {
		if rp != nil {
			return DeleteRole422ApplicationProblemPlusJSONResponse{*rp}, nil
		}
		return DeleteRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: delete role %s: %w", req.Role, err)
	}
	return DeleteRole204Response{}, nil
}
