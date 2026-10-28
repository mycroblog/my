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

// UserPreferences reads and changes the viewer's preferences (FR-007, FR-208).
// *appidentity.Service implements it.
type UserPreferences interface {
	Preferences(ctx context.Context, userID string) (appidentity.UserPreferences, error)
	// UpdatePreferences fails with a *appidentity.PreferenceError for invalid input.
	UpdatePreferences(ctx context.Context, userID string, in appidentity.PreferencesUpdate) (appidentity.UserPreferences, error)
	// PrimaryEmailSuppression returns the suppression of the user's primary address (ADR-0086
	// §5) and whether it is suppressed.
	PrimaryEmailSuppression(ctx context.Context, userID string) (identity.EmailSuppression, bool, error)
}

// newEmailSuppression presents a suppression; nil for an address that is not suppressed.
func newEmailSuppression(sup identity.EmailSuppression, ok bool) *EmailSuppression {
	if !ok {
		return nil
	}
	return &EmailSuppression{Kind: EmailSuppressionKind(sup.Kind), Since: sup.Since.UTC()}
}

// preferencesOf presents the preferences p of userID with the suppression of their address.
func (s *Server) preferencesOf(ctx context.Context, userID string, p appidentity.UserPreferences) (Preferences, error) {
	out := newPreferences(p)
	sup, ok, err := s.o.Preferences.PrimaryEmailSuppression(ctx, userID)
	if err != nil {
		return Preferences{}, fmt.Errorf("clientapi: the email suppression of %s: %w", userID, err)
	}
	out.EmailSuppression = newEmailSuppression(sup, ok)
	return out, nil
}

func newPreferences(p appidentity.UserPreferences) Preferences {
	langs := p.TimelineLanguages
	if langs == nil {
		langs = []string{}
	}
	return Preferences{
		Locale: p.Locale, TimeZone: p.TimeZone, TimeZoneChosen: p.TimeZoneChosen, DefaultVisibility: Visibility(p.DefaultVisibility),
		DefaultQuotePolicy:  QuotePolicy(p.DefaultQuotePolicy),
		DefaultPostLanguage: p.DefaultPostLanguage, RequireMediaDescription: p.RequireMediaDescription,
		TimelineAutoInsert: p.TimelineAutoInsert, TimelineLanguages: langs,
	}
}

// GetPreferences serves GET /api/v1/preferences.
func (s *Server) GetPreferences(ctx context.Context, _ GetPreferencesRequestObject) (GetPreferencesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.o.Preferences.Preferences(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the preferences of %s: %w", v.UserID, err)
	}
	body, err := s.preferencesOf(ctx, v.UserID, p)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return GetPreferences200JSONResponse{Body: body, Headers: GetPreferences200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdatePreferences serves PATCH /api/v1/preferences.
func (s *Server) UpdatePreferences(ctx context.Context, req UpdatePreferencesRequestObject) (UpdatePreferencesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	var in appidentity.PreferencesUpdate
	if b := req.Body; b != nil {
		in = appidentity.PreferencesUpdate{
			Locale: b.Locale, TimeZone: b.TimeZone, DefaultPostLanguage: b.DefaultPostLanguage,
			RequireMediaDescription: b.RequireMediaDescription, TimelineLanguages: b.TimelineLanguages,
			TimelineAutoInsert: b.TimelineAutoInsert,
		}
		if b.DefaultVisibility != nil {
			vis := identity.Visibility(*b.DefaultVisibility)
			in.DefaultVisibility = &vis
		}
		if b.DefaultQuotePolicy != nil {
			policy := identity.QuotePolicy(*b.DefaultQuotePolicy)
			in.DefaultQuotePolicy = &policy
		}
	}
	p, err := s.o.Preferences.UpdatePreferences(ctx, v.UserID, in)
	var invalid *appidentity.PreferenceError
	if errors.As(err, &invalid) {
		l := s.Localizer(ctx)
		detail := l.ServerErrorsInvalidValue(invalid.Preference)
		if errors.Is(invalid.Err, identity.ErrTooManyLanguages) {
			detail = l.ServerErrorsTooMany(identity.MaxTimelineLanguageCount, invalid.Preference)
		}
		body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)
		return UpdatePreferencesdefaultApplicationProblemPlusJSONResponse{
			Body: body, Headers: headers, StatusCode: http.StatusUnprocessableEntity,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update the preferences of %s: %w", v.UserID, err)
	}
	body, err := s.preferencesOf(ctx, v.UserID, p)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return UpdatePreferences200JSONResponse{Body: body, Headers: UpdatePreferences200ResponseHeaders{CacheControl: &cache}}, nil
}
