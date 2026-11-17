// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance theme (ADR-0059 §15, FR-605): the admin operations that read the draft and the
// token set in effect, save the draft, preview it, publish it, and restore the set published
// before (manage_settings), and Instance.theme of GET /instance.

// ProblemTypeInvalidTheme is the problem type of a token set that breaks a rule (422), whose
// theme_errors list every reason.
const ProblemTypeInvalidTheme = "/api/v1/problems/invalid-theme"

// Themes runs the use cases of the instance theme. *appinstance.Service implements it.
type Themes interface {
	// Theme returns the publication in effect, and false when there is none or it overrides
	// nothing.
	Theme(ctx context.Context) (instance.ThemeRevision, bool, error)
	// ThemeDraft returns the draft, and false when no set was saved.
	ThemeDraft(ctx context.Context) (instance.ThemeSet, bool, error)
	// SaveThemeDraft records a set as the draft; it fails with an *instance.ThemeError.
	SaveThemeDraft(ctx context.Context, in appinstance.SaveThemeDraftInput) (instance.ThemeSet, error)
	// PublishTheme publishes the draft, audited; it fails with appinstance.ErrNotFound without a
	// draft and with an *instance.ThemeError.
	PublishTheme(ctx context.Context, publishedByUserID string) (instance.ThemeRevision, error)
	// RestoreTheme publishes the set published before, audited; it fails with
	// appinstance.ErrNotFound without one and with an *instance.ThemeError.
	RestoreTheme(ctx context.Context, publishedByUserID string) (instance.ThemeRevision, error)
}

func themeTokenValues(tokens []instance.ThemeTokenValue) []ThemeTokenValue {
	out := make([]ThemeTokenValue, len(tokens))
	for i, v := range tokens {
		out[i] = ThemeTokenValue{Token: v.Token}
		if t, ok := instance.LookupThemeToken(v.Token); ok && t.Kind == instance.ThemeTokenColor {
			out[i].Light, out[i].Dark = &v.Light, &v.Dark
		} else {
			out[i].Value = &v.Value
		}
	}
	return out
}

// themeSet returns a set as the admin API shows it, with the overrides that no longer apply.
func themeSet(set instance.ThemeSet) ThemeSet {
	out := ThemeSet{Id: set.ID, Tokens: themeTokenValues(set.Tokens), StaleTokens: make([]StaleThemeToken, len(set.Stale)), SavedAt: set.SavedAt.UTC()}
	opt := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	for i, o := range set.Stale {
		out.StaleTokens[i] = StaleThemeToken{
			Token: o.Token, Light: opt(o.Light), Dark: opt(o.Dark), Value: opt(o.Value), Reason: StaleThemeTokenReason(o.Reason),
		}
	}
	return out
}

// themeStylesheetURL is the URL of a revision's stylesheet, or "" without Options.ThemeStylesheetPath.
func (s *Server) themeStylesheetURL(revisionID string) string {
	if s.o.ThemeStylesheetPath == nil {
		return ""
	}
	return s.origin() + s.o.ThemeStylesheetPath(revisionID)
}

// adminTheme reads the draft and the set in effect.
func (s *Server) adminTheme(ctx context.Context) (AdminTheme, error) {
	var out AdminTheme
	draft, ok, err := s.o.Themes.ThemeDraft(ctx)
	if err != nil {
		return AdminTheme{}, fmt.Errorf("clientapi: theme draft: %w", err)
	}
	if ok {
		d := themeSet(draft)
		out.Draft = &d
	}
	rev, inEffect, err := s.o.Themes.Theme(ctx)
	if err != nil {
		return AdminTheme{}, fmt.Errorf("clientapi: theme: %w", err)
	}
	if rev.ID != "" {
		p := AdminThemeRevision{Revision: rev.ID, Set: themeSet(rev.Set), PublishedAt: rev.PublishedAt.UTC()}
		if u := s.themeStylesheetURL(rev.ID); inEffect && u != "" {
			p.StylesheetUrl = &u
		}
		out.Published = &p
	}
	return out, nil
}

// adminThemeResponse is the 200 of every theme operation: the theme as it now is.
func (s *Server) adminThemeResponse(ctx context.Context) (AdminThemeJSONResponse, error) {
	body, err := s.adminTheme(ctx)
	cache := cacheNoStore
	return AdminThemeJSONResponse{Body: body, Headers: AdminThemeResponseHeaders{CacheControl: &cache}}, err
}

// instanceTheme returns Instance.theme, or nil when the instance has no theme.
func (s *Server) instanceTheme(ctx context.Context) (*InstanceTheme, error) {
	if s.o.Themes == nil {
		return nil, nil
	}
	rev, ok, err := s.o.Themes.Theme(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: instance theme: %w", err)
	}
	u := s.themeStylesheetURL(rev.ID)
	if !ok || u == "" {
		return nil, nil
	}
	return &InstanceTheme{Revision: rev.ID, StylesheetUrl: u, Tokens: themeTokenValues(rev.Set.Tokens)}, nil
}

// themeOperation checks the permission and that the server has a theme: a problem, or nil to go on.
func (s *Server) themeOperation(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil || p != nil {
		return p, err
	}
	if s.o.Themes == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// GetAdminTheme serves GET /api/v1/admin/theme.
func (s *Server) GetAdminTheme(ctx context.Context, _ GetAdminThemeRequestObject) (GetAdminThemeResponseObject, error) {
	if p, err := s.themeOperation(ctx); err != nil || p != nil {
		return problemOr[GetAdminThemedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	res, err := s.adminThemeResponse(ctx)
	if err != nil {
		return nil, err
	}
	return GetAdminTheme200JSONResponse{res}, nil
}

// SetAdminThemeDraft serves PUT /api/v1/admin/theme/draft.
func (s *Server) SetAdminThemeDraft(ctx context.Context, req SetAdminThemeDraftRequestObject) (SetAdminThemeDraftResponseObject, error) {
	if p, err := s.themeOperation(ctx); err != nil || p != nil {
		return problemOr[SetAdminThemeDraftdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return SetAdminThemeDraftdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	tokens := make([]instance.ThemeTokenValue, len(req.Body.Tokens))
	for i, t := range req.Body.Tokens {
		tokens[i] = instance.ThemeTokenValue{Token: t.Token, Light: deref(t.Light), Dark: deref(t.Dark), Value: deref(t.Value)}
	}
	_, err = s.o.Themes.SaveThemeDraft(ctx, appinstance.SaveThemeDraftInput{Tokens: tokens, SavedByUserID: v.UserID})
	var te *instance.ThemeError
	switch {
	case errors.As(err, &te):
		return SetAdminThemeDraft422ApplicationProblemPlusJSONResponse{s.themeProblem(ctx, te)}, nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: save theme draft: %w", err)
	}
	res, err := s.adminThemeResponse(ctx)
	if err != nil {
		return nil, err
	}
	return SetAdminThemeDraft200JSONResponse{res}, nil
}

// GetAdminThemeDraftStylesheet serves GET /api/v1/admin/theme/draft/stylesheet: the preview of the
// draft, only for administrators and never cached.
func (s *Server) GetAdminThemeDraftStylesheet(ctx context.Context, _ GetAdminThemeDraftStylesheetRequestObject) (GetAdminThemeDraftStylesheetResponseObject, error) {
	if p, err := s.themeOperation(ctx); err != nil || p != nil {
		return problemOr[GetAdminThemeDraftStylesheetdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	draft, ok, err := s.o.Themes.ThemeDraft(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: theme draft: %w", err)
	}
	if !ok {
		return GetAdminThemeDraftStylesheetdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	body := draft.Stylesheet()
	cache := cacheNoStore
	return GetAdminThemeDraftStylesheet200TextcssResponse{
		Body: bytes.NewReader(body), ContentLength: int64(len(body)),
		Headers: GetAdminThemeDraftStylesheet200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// PublishAdminTheme serves POST /api/v1/admin/theme/publish.
func (s *Server) PublishAdminTheme(ctx context.Context, _ PublishAdminThemeRequestObject) (PublishAdminThemeResponseObject, error) {
	if p, err := s.themeOperation(ctx); err != nil || p != nil {
		return problemOr[PublishAdminThemedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	_, err = s.o.Themes.PublishTheme(ctx, v.UserID)
	var te *instance.ThemeError
	switch {
	case errors.As(err, &te):
		return PublishAdminTheme422ApplicationProblemPlusJSONResponse{s.themeProblem(ctx, te)}, nil
	case errors.Is(err, appinstance.ErrNotFound):
		return PublishAdminThemedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: publish theme: %w", err)
	}
	res, err := s.adminThemeResponse(ctx)
	if err != nil {
		return nil, err
	}
	return PublishAdminTheme200JSONResponse{res}, nil
}

// RestoreAdminTheme serves POST /api/v1/admin/theme/restore.
func (s *Server) RestoreAdminTheme(ctx context.Context, _ RestoreAdminThemeRequestObject) (RestoreAdminThemeResponseObject, error) {
	if p, err := s.themeOperation(ctx); err != nil || p != nil {
		return problemOr[RestoreAdminThemedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	_, err = s.o.Themes.RestoreTheme(ctx, v.UserID)
	var te *instance.ThemeError
	switch {
	case errors.As(err, &te):
		return RestoreAdminTheme422ApplicationProblemPlusJSONResponse{s.themeProblem(ctx, te)}, nil
	case errors.Is(err, appinstance.ErrNotFound):
		return RestoreAdminThemedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: restore theme: %w", err)
	}
	res, err := s.adminThemeResponse(ctx)
	if err != nil {
		return nil, err
	}
	return RestoreAdminTheme200JSONResponse{res}, nil
}

// problemOr returns the problem p as the response type R, or err.
func problemOr[R ~struct {
	Body       Problem
	Headers    ProblemResponseHeaders
	StatusCode int
}](p *problem, err error) (R, error) {
	if err != nil {
		return R{}, err
	}
	return R(*p), nil
}

func (s *Server) themeProblem(ctx context.Context, te *instance.ThemeError) ThemeProblemApplicationProblemPlusJSONResponse {
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidTheme, l.ServerErrorsInvalidTheme(), "")
	out := ThemeProblem{Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId}
	errs := make([]ThemeError, len(te.Problems))
	num := func(f float64) *float32 { v := float32(f); return &v }
	for i, p := range te.Problems {
		e := ThemeError{Code: ThemeErrorCode(p.Code)}
		if p.Token != "" {
			e.Token = &p.Token
		}
		if p.Scheme == instance.SchemeLight || p.Scheme == instance.SchemeDark {
			scheme := ThemeErrorScheme(p.Scheme)
			e.Scheme = &scheme
		}
		switch p.Code {
		case instance.ThemeLowContrast:
			e.Background, e.Ratio, e.Minimum = &p.Background, num(p.Ratio), num(p.Minimum)
		case instance.ThemeOutOfRange:
			e.Min, e.Max = num(p.Min), num(p.Max)
		case instance.ThemeTooManyOverrides:
			e.Max = num(p.Max)
		}
		errs[i] = e
	}
	out.ThemeErrors = &errs
	return ThemeProblemApplicationProblemPlusJSONResponse{
		Body:    out,
		Headers: ThemeProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}
}
