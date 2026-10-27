// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The roles of users (FR-720, manage_roles): listing the roles, and granting and revoking them
// on local accounts. The use cases check the rank of the viewer and keep the last admin, and
// audit every change.

// ProblemTypeInvalidRole is the problem type of a role change that breaks a rule (422), whose
// reason says which.
const ProblemTypeInvalidRole = "/api/v1/problems/invalid-role"

// Roles runs the use cases of the roles. *appidentity.Service implements it.
type Roles interface {
	Roles(ctx context.Context) ([]appidentity.RoleWithPermissions, error)
	UserRoles(ctx context.Context, userID string) ([]identity.Role, error)
	GrantRole(ctx context.Context, in appidentity.RoleChangeInput) (bool, error)
	RevokeRole(ctx context.Context, in appidentity.RoleChangeInput) (bool, error)
	CreateRole(ctx context.Context, in appidentity.CreateRoleInput) (appidentity.RoleWithPermissions, error)
	UpdateRole(ctx context.Context, in appidentity.UpdateRoleInput) (appidentity.RoleWithPermissions, error)
	DeleteRole(ctx context.Context, key, byUserID string) error
}

// newAdminRole presents a role with its permissions.
func newAdminRole(r appidentity.RoleWithPermissions) AdminRole {
	item := AdminRole{
		Id: r.ID, Key: r.Key, Position: r.Position, Permissions: make([]Permission, len(r.Permissions)), Badge: newRoleBadge(r.Badge),
	}
	for i, p := range r.Permissions {
		item.Permissions[i] = Permission(p)
	}
	return item
}

// roleList presents the user's current roles with their permissions, highest first.
func (s *Server) roleList(ctx context.Context, userID string) (AdminRoleListJSONResponse, error) {
	all, err := s.o.Roles.Roles(ctx)
	if err != nil {
		return AdminRoleListJSONResponse{}, fmt.Errorf("clientapi: list roles: %w", err)
	}
	keep := func(string) bool { return true }
	if userID != "" {
		held, err := s.o.Roles.UserRoles(ctx, userID)
		if err != nil {
			return AdminRoleListJSONResponse{}, fmt.Errorf("clientapi: roles of %s: %w", userID, err)
		}
		ids := make(map[string]bool, len(held))
		for _, r := range held {
			ids[r.ID] = true
		}
		keep = func(id string) bool { return ids[id] }
	}
	list := AdminRoleList{Items: []AdminRole{}}
	for _, r := range all {
		if !keep(r.ID) {
			continue
		}
		list.Items = append(list.Items, newAdminRole(r))
	}
	cache := cacheNoStore
	return AdminRoleListJSONResponse{Body: list, Headers: AdminRoleListResponseHeaders{CacheControl: &cache}}, nil
}

// localUser returns the user of the local account with ID accountID, and false for an unknown
// account or one that is not local.
func (s *Server) localUser(ctx context.Context, accountID string) (string, bool, error) {
	a, err := s.o.Accounts.FindByID(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("clientapi: account %s: %w", accountID, err)
	case a.Local == nil:
		return "", false, nil
	}
	return a.Local.UserID, true, nil
}

// roleProblem maps the errors of GrantRole and RevokeRole to problems: 404 for an unknown user
// or role, 403 for a role that outranks the viewer or has a permission that the viewer lacks,
// and 409 for the last admin and for the role of a supporter plan. It reports false for any
// other error.
func (s *Server) roleProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	var missing *appidentity.MissingPermissionsError
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, appidentity.ErrRoleOutranksActor), errors.As(err, &missing):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeBlank, l.ServerErrorsForbidden(), ""), true
	case errors.Is(err, appidentity.ErrLastAdmin):
		return s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), ""), true
	}
	return s.managedRoleProblem(ctx, err)
}

// ListRoles serves GET /api/v1/admin/roles.
func (s *Server) ListRoles(ctx context.Context, _ ListRolesRequestObject) (ListRolesResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRoles); err != nil {
		return nil, err
	} else if p != nil {
		return ListRolesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	res, err := s.roleList(ctx, "")
	if err != nil {
		return nil, err
	}
	return ListRoles200JSONResponse{res}, nil
}

// ListAccountRoles serves GET /api/v1/admin/accounts/{id}/roles.
func (s *Server) ListAccountRoles(ctx context.Context, req ListAccountRolesRequestObject) (ListAccountRolesResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRoles); err != nil {
		return nil, err
	} else if p != nil {
		return ListAccountRolesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	userID, ok, err := s.localUser(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListAccountRolesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	res, err := s.roleList(ctx, userID)
	if err != nil {
		return nil, err
	}
	return ListAccountRoles200JSONResponse{res}, nil
}

// GrantAccountRole serves PUT /api/v1/admin/accounts/{id}/roles/{role}.
func (s *Server) GrantAccountRole(ctx context.Context, req GrantAccountRoleRequestObject) (GrantAccountRoleResponseObject, error) {
	res, p, err := s.changeRole(ctx, req.Id, req.Role, s.o.Roles.GrantRole)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return GrantAccountRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return GrantAccountRole200JSONResponse{res}, nil
}

// RevokeAccountRole serves DELETE /api/v1/admin/accounts/{id}/roles/{role}.
func (s *Server) RevokeAccountRole(ctx context.Context, req RevokeAccountRoleRequestObject) (RevokeAccountRoleResponseObject, error) {
	res, p, err := s.changeRole(ctx, req.Id, req.Role, s.o.Roles.RevokeRole)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return RevokeAccountRoledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return RevokeAccountRole200JSONResponse{res}, nil
}

// changeRole grants or revokes (change) the role with key on the user of a local account, as the
// viewer, and presents the user's roles afterwards.
func (s *Server) changeRole(ctx context.Context, accountID, key string,
	change func(context.Context, appidentity.RoleChangeInput) (bool, error)) (AdminRoleListJSONResponse, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRoles); err != nil || p != nil {
		return AdminRoleListJSONResponse{}, p, err
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return AdminRoleListJSONResponse{}, nil, err
	}
	userID, ok, err := s.localUser(ctx, accountID)
	if err != nil {
		return AdminRoleListJSONResponse{}, nil, err
	}
	if !ok {
		p := s.notFoundResponse(ctx)
		return AdminRoleListJSONResponse{}, &p, nil
	}
	_, err = change(ctx, appidentity.RoleChangeInput{UserID: userID, RoleKey: key, ByUserID: v.UserID})
	if p, ok := s.roleProblem(ctx, err); ok {
		return AdminRoleListJSONResponse{}, &p, nil
	}
	if err != nil {
		return AdminRoleListJSONResponse{}, nil, fmt.Errorf("clientapi: change role %s of %s: %w", key, accountID, err)
	}
	res, err := s.roleList(ctx, userID)
	return res, nil, err
}

// managedRoleProblem maps the refusals of the roles of the supporter plans (ADR-0072 §5): a 409
// for a direct change of a plan's role, and a 409 for a change that would put a staff role at or
// below one. It reports false for any other error.
func (s *Server) managedRoleProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appidentity.ErrRoleManagedByPlan):
		return s.problem(ctx, http.StatusConflict, ProblemTypeRoleManagedByPlan, l.ServerErrorsRoleManagedByPlan(),
			l.ServerErrorsRoleManagedByPlanDetail()), true
	case errors.Is(err, appidentity.ErrBreaksManagedRoles):
		return s.problem(ctx, http.StatusConflict, ProblemTypeBreaksSupporterRoles, l.ServerErrorsRoleBreaksSupporterRoles(), ""), true
	}
	return problem{}, false
}
