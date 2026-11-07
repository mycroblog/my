// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance settings (FR-705, manage_settings): the admin operations that read every setting
// with what it accepts and change several at once, each change audited (FR-704). The CLI's
// `my instance set` changes the same settings through the same use case.

// InstanceSettings reads and changes the instance settings. *appinstance.Service implements it.
type InstanceSettings interface {
	Settings(ctx context.Context) (instance.Settings, error)
	SetSettings(ctx context.Context, ins []appinstance.SetSettingInput) ([]instance.SettingRevision, error)
}

// SignInLimit reads the limit of registration and sign-in to the login providers.
// *appidentity.Service implements it.
type SignInLimit interface {
	LoginProvidersOnly(ctx context.Context) (bool, error)
	RegistrationMode(ctx context.Context) (identity.RegistrationMode, error)
	LoginProvidersOnlyImpact(ctx context.Context, viewerUserID, withProviderID string) (appidentity.LoginProvidersOnlyImpact, error)
}

// GetLoginProvidersOnlyImpact serves GET /api/v1/admin/settings/login_providers_only/impact, for
// those who change the setting and those who change the providers under it.
func (s *Server) GetLoginProvidersOnlyImpact(ctx context.Context, req GetLoginProvidersOnlyImpactRequestObject) (GetLoginProvidersOnlyImpactResponseObject, error) {
	p, err := s.requirePermission(ctx, identity.PermManageLoginProviders)
	if err != nil {
		return nil, err
	}
	if p != nil {
		if p, err = s.requirePermission(ctx, identity.PermManageSettings); err != nil {
			return nil, err
		}
		if p != nil {
			return GetLoginProvidersOnlyImpactdefaultApplicationProblemPlusJSONResponse(*p), nil
		}
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.SignInLimit == nil {
		return GetLoginProvidersOnlyImpactdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	providerID := ""
	if req.Params.ProviderId != nil {
		providerID = knownID(*req.Params.ProviderId)
	}
	impact, err := s.o.SignInLimit.LoginProvidersOnlyImpact(ctx, v.UserID, providerID)
	if errors.Is(err, appidentity.ErrNotFound) {
		return GetLoginProvidersOnlyImpactdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: count who the limit to login providers affects: %w", err)
	}
	cache := cacheNoStore
	return GetLoginProvidersOnlyImpact200JSONResponse{
		Body: LoginProvidersOnlyImpact{
			SettingOn: impact.SettingOn, EnabledProviders: impact.EnabledProviders,
			UsersWithoutProvider: impact.UsersWithoutProvider, IncludesViewer: impact.IncludesViewer,
			StartsLimit: impact.StartsLimit,
		},
		Headers: GetLoginProvidersOnlyImpact200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// adminSettingList returns every setting of the current settings, in the order of the
// definitions.
func (s *Server) adminSettingList(ctx context.Context) (AdminSettingListJSONResponse, error) {
	current, err := s.o.Settings.Settings(ctx)
	if err != nil {
		return AdminSettingListJSONResponse{}, fmt.Errorf("clientapi: instance settings: %w", err)
	}
	defs := instance.Definitions()
	list := AdminSettingList{Items: make([]AdminSetting, 0, len(defs))}
	for _, d := range defs {
		value, stored := current.Value(d.Key)
		item := AdminSetting{Key: InstanceSettingKey(d.Key), Kind: InstanceSettingKind(d.Kind), Value: value, Default: d.Default, Stored: stored}
		switch d.Kind {
		case instance.KindEnum:
			item.Choices = &d.Choices
		case instance.KindInt:
			item.Minimum, item.Maximum = &d.Min, &d.Max
		}
		list.Items = append(list.Items, item)
	}
	cache := cacheNoStore
	return AdminSettingListJSONResponse{Body: list, Headers: AdminSettingListResponseHeaders{CacheControl: &cache}}, nil
}

// ListInstanceSettings serves GET /api/v1/admin/settings.
func (s *Server) ListInstanceSettings(ctx context.Context, _ ListInstanceSettingsRequestObject) (ListInstanceSettingsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil {
		return nil, err
	} else if p != nil {
		return ListInstanceSettingsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	list, err := s.adminSettingList(ctx)
	if err != nil {
		return nil, err
	}
	return ListInstanceSettings200JSONResponse{list}, nil
}

// UpdateInstanceSettings serves PATCH /api/v1/admin/settings: every value or none, in the
// order of the definitions.
func (s *Server) UpdateInstanceSettings(ctx context.Context, req UpdateInstanceSettingsRequestObject) (UpdateInstanceSettingsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil {
		return nil, err
	} else if p != nil {
		return UpdateInstanceSettingsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(name string) UpdateInstanceSettingsResponseObject {
		return UpdateInstanceSettingsdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(name)))
	}
	if req.Body == nil || len(req.Body.Values) == 0 {
		return unprocessable("values"), nil
	}
	keys := make([]string, 0, len(req.Body.Values))
	for key := range req.Body.Values {
		if _, err := instance.Lookup(instance.SettingKey(key)); err != nil {
			return unprocessable("values." + key), nil
		}
		keys = append(keys, key)
	}
	order := func(key string) int {
		return slices.IndexFunc(instance.Definitions(), func(d instance.Definition) bool { return string(d.Key) == key })
	}
	slices.SortFunc(keys, func(a, b string) int { return order(a) - order(b) })
	ins := make([]appinstance.SetSettingInput, len(keys))
	for i, key := range keys {
		ins[i] = appinstance.SetSettingInput{Key: instance.SettingKey(key), Value: req.Body.Values[key], RevisedByUserID: v.UserID}
	}
	_, err = s.o.Settings.SetSettings(ctx, ins)
	var se *appinstance.SettingError
	var accounts *appinstance.SingleUserAccountsError
	switch {
	case errors.As(err, &accounts):
		return UpdateInstanceSettingsdefaultApplicationProblemPlusJSONResponse(s.singleUserSettingsProblem(ctx, accounts.Count)), nil
	case errors.As(err, &se):
		return unprocessable("values." + string(se.Key)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: set instance settings: %w", err)
	}
	list, err := s.adminSettingList(ctx)
	if err != nil {
		return nil, err
	}
	return UpdateInstanceSettings200JSONResponse{list}, nil
}
