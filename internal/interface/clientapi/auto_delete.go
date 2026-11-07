// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// AutoDeletePolicies reads and replaces the viewer's automatic deletion of their old posts
// (FR-118) under the server maximum (ADR-0151); *content.AutoDelete implements it.
type AutoDeletePolicies interface {
	AutoDeletePolicy(ctx context.Context, accountID string) (appcontent.AutoDeleteView, error)
	// UpdateAutoDeletePolicy fails with a *content.AutoDeletePolicyError for invalid input or
	// input over the server maximum.
	UpdateAutoDeletePolicy(ctx context.Context, accountID string, p content.AutoDeletePolicy) (appcontent.AutoDeleteView, error)
	// PreviewAutoDeletePolicy counts what p would delete now without saving it; it fails with a
	// *content.AutoDeletePolicyError for invalid input.
	PreviewAutoDeletePolicy(ctx context.Context, accountID string, p content.AutoDeletePolicy) (appcontent.AutoDeletePreview, error)
	// LimitNotices returns the notices of the server maximum among ids that went to the
	// account, by ID.
	LimitNotices(ctx context.Context, accountID string, ids []string) (map[string]content.AutoDeleteLimitNotice, error)
}

// autoDeletePolicyFrom is the policy of a request body.
func autoDeletePolicyFrom(b AutoDeletePolicy) content.AutoDeletePolicy {
	in := content.AutoDeletePolicy{
		Enabled: b.Enabled, MinAgeDays: b.MinAgeDays,
		KeepPinned: b.KeepPinned, KeepBookmarked: b.KeepBookmarked, KeepFavourited: b.KeepFavourited,
		KeepMedia: b.KeepMedia, KeepPolls: b.KeepPolls,
		KeepMinBoosts: b.KeepMinBoosts, KeepMinFavourites: b.KeepMinFavourites,
	}
	for _, vis := range b.KeepVisibilities {
		in.KeepVisibilities = append(in.KeepVisibilities, content.Visibility(vis))
	}
	return in
}

// autoDeletePolicyProblem is the 422 problem naming the invalid property of a policy, when err
// is a *content.AutoDeletePolicyError.
func (s *Server) autoDeletePolicyProblem(ctx context.Context, err error) (problem, bool) {
	var invalid *content.AutoDeletePolicyError
	if !errors.As(err, &invalid) {
		return problem{}, false
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(invalid.Property))
	return problem{Body: body, Headers: headers, StatusCode: http.StatusUnprocessableEntity}, true
}

func newAutoDeletePolicy(p content.AutoDeletePolicy) AutoDeletePolicy {
	kept := make([]Visibility, 0, len(p.KeepVisibilities))
	for _, v := range p.KeepVisibilities {
		kept = append(kept, Visibility(v))
	}
	return AutoDeletePolicy{
		Enabled: p.Enabled, MinAgeDays: p.MinAgeDays,
		KeepPinned: p.KeepPinned, KeepBookmarked: p.KeepBookmarked, KeepFavourited: p.KeepFavourited,
		KeepMedia: p.KeepMedia, KeepPolls: p.KeepPolls, KeepVisibilities: kept,
		KeepMinBoosts: p.KeepMinBoosts, KeepMinFavourites: p.KeepMinFavourites,
	}
}

// newAutoDeleteLimit is the public part of the server maximum.
func newAutoDeleteLimit(l content.AutoDeleteLimit) AutoDeleteLimit {
	return AutoDeleteLimit{
		MaxAgeDays: l.MaxAgeDays, AllowPinned: l.AllowPinned, KeptPins: l.KeptPins,
		AllowBookmarked: l.AllowBookmarked, AllowFavourited: l.AllowFavourited, AllowMedia: l.AllowMedia,
		AllowPolls: l.AllowPolls, AllowVisibilities: l.AllowVisibilities, AllowThresholds: l.AllowThresholds,
	}
}

// newAutoDeleteSettings is the saved policy of v with the server maximum when it applies.
func newAutoDeleteSettings(v appcontent.AutoDeleteView) AutoDeleteSettings {
	p := newAutoDeletePolicy(v.Policy)
	out := AutoDeleteSettings{
		Enabled: p.Enabled, MinAgeDays: p.MinAgeDays,
		KeepPinned: p.KeepPinned, KeepBookmarked: p.KeepBookmarked, KeepFavourited: p.KeepFavourited,
		KeepMedia: p.KeepMedia, KeepPolls: p.KeepPolls, KeepVisibilities: p.KeepVisibilities,
		KeepMinBoosts: p.KeepMinBoosts, KeepMinFavourites: p.KeepMinFavourites,
	}
	if v.Limit != nil {
		l := newAutoDeleteLimit(v.Limit.AutoDeleteLimit)
		sl := AutoDeleteServerLimit{
			MaxAgeDays: l.MaxAgeDays, AllowPinned: l.AllowPinned, KeptPins: l.KeptPins,
			AllowBookmarked: l.AllowBookmarked, AllowFavourited: l.AllowFavourited, AllowMedia: l.AllowMedia,
			AllowPolls: l.AllowPolls, AllowVisibilities: l.AllowVisibilities, AllowThresholds: l.AllowThresholds,
			Effective: newAutoDeletePolicy(v.Limit.Effective), Bound: v.Limit.Bound,
		}
		if !v.Limit.EnforceFrom.IsZero() {
			at := v.Limit.EnforceFrom.UTC()
			sl.EnforceFrom = &at
		}
		out.ServerLimit = &sl
	}
	return out
}

// GetAutoDeletePolicy serves GET /api/v1/preferences/auto_delete.
func (s *Server) GetAutoDeletePolicy(ctx context.Context, _ GetAutoDeletePolicyRequestObject) (GetAutoDeletePolicyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.AutoDelete == nil {
		body, headers := s.notFound(ctx)
		return GetAutoDeletePolicydefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	p, err := s.o.AutoDelete.AutoDeletePolicy(ctx, v.AccountID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the auto delete policy of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return GetAutoDeletePolicy200JSONResponse{Body: newAutoDeleteSettings(p), Headers: GetAutoDeletePolicy200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdateAutoDeletePolicy serves PUT /api/v1/preferences/auto_delete.
func (s *Server) UpdateAutoDeletePolicy(ctx context.Context, req UpdateAutoDeletePolicyRequestObject) (UpdateAutoDeletePolicyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.AutoDelete == nil {
		body, headers := s.notFound(ctx)
		return UpdateAutoDeletePolicydefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if req.Body == nil {
		return UpdateAutoDeletePolicydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	p, err := s.o.AutoDelete.UpdateAutoDeletePolicy(ctx, v.AccountID, autoDeletePolicyFrom(*req.Body))
	if pr, ok := s.autoDeletePolicyProblem(ctx, err); ok {
		return UpdateAutoDeletePolicydefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update the auto delete policy of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return UpdateAutoDeletePolicy200JSONResponse{Body: newAutoDeleteSettings(p), Headers: UpdateAutoDeletePolicy200ResponseHeaders{CacheControl: &cache}}, nil
}

// PreviewAutoDeletePolicy serves POST /api/v1/preferences/auto_delete/preview. The oldest posts
// that the policy would delete are presented as the viewer, their author, sees them.
func (s *Server) PreviewAutoDeletePolicy(ctx context.Context, req PreviewAutoDeletePolicyRequestObject) (PreviewAutoDeletePolicyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.AutoDelete == nil {
		body, headers := s.notFound(ctx)
		return PreviewAutoDeletePolicydefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if req.Body == nil {
		return PreviewAutoDeletePolicydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	preview, err := s.o.AutoDelete.PreviewAutoDeletePolicy(ctx, v.AccountID, autoDeletePolicyFrom(*req.Body))
	if pr, ok := s.autoDeletePolicyProblem(ctx, err); ok {
		return PreviewAutoDeletePolicydefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: preview the auto delete policy of %s: %w", v.AccountID, err)
	}
	items := make([]statusItem, 0, len(preview.Examples))
	for _, st := range preview.Examples {
		items = append(items, statusItem{StatusID: st.ID})
	}
	examples, err := s.statuses(ctx, contentViewer(ctx), items)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the auto delete examples of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return PreviewAutoDeletePolicy200JSONResponse{
		Body:    AutoDeletePreview{Count: preview.Count, More: preview.More, Examples: examples},
		Headers: PreviewAutoDeletePolicy200ResponseHeaders{CacheControl: &cache},
	}, nil
}
