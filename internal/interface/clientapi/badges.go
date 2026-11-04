// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Role badges on profiles (ADR-0070, FR-913). Account.badges is filled by the account's own
// endpoints (newProfile) only, in one read; every other Account has none, so that timelines and
// lists of accounts read nothing more. The viewer chooses which badges to show through
// /preferences/badges, off by default.

// RoleBadges reads the badges that users show on their profiles and changes the viewer's
// choice. *appidentity.Service implements it.
type RoleBadges interface {
	ProfileBadges(ctx context.Context, userID string) ([]identity.ProfileBadge, error)
	BadgeChoices(ctx context.Context, userID string) ([]identity.BadgeChoice, error)
	// SetBadgeDisplayed fails with appidentity.ErrNotFound for a role that the user does not
	// hold or that has no badge.
	SetBadgeDisplayed(ctx context.Context, userID, roleID string, displayed bool) (identity.BadgeChoice, error)
}

// profileBadges presents the badges that the user shows on their profile; without
// Options.Badges there are none.
func (s *Server) profileBadges(ctx context.Context, userID string) ([]AccountBadge, error) {
	out := []AccountBadge{}
	if s.o.Badges == nil {
		return out, nil
	}
	badges, err := s.o.Badges.ProfileBadges(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the badges of %s: %w", userID, err)
	}
	for _, b := range badges {
		out = append(out, AccountBadge{RoleId: b.RoleID, Label: b.Label, Color: BadgeColor(b.Color)})
	}
	return out, nil
}

func newBadgeChoice(c identity.BadgeChoice) BadgeChoice {
	return BadgeChoice{RoleId: c.RoleID, Label: c.Label, Color: BadgeColor(c.Color), Displayed: c.Displayed}
}

// newRoleBadge presents the badge of a role, or nil for none.
func newRoleBadge(b *identity.RoleBadge) *RoleBadge {
	if b == nil {
		return nil
	}
	return &RoleBadge{Label: b.Label, Color: BadgeColor(b.Color)}
}

// ListBadgeChoices serves GET /api/v1/preferences/badges.
func (s *Server) ListBadgeChoices(ctx context.Context, _ ListBadgeChoicesRequestObject) (ListBadgeChoicesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	list := BadgeChoiceList{Items: []BadgeChoice{}}
	if s.o.Badges != nil {
		choices, err := s.o.Badges.BadgeChoices(ctx, v.UserID)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the badge choices of %s: %w", v.UserID, err)
		}
		for _, c := range choices {
			list.Items = append(list.Items, newBadgeChoice(c))
		}
	}
	cache := cacheNoStore
	return ListBadgeChoices200JSONResponse{Body: list, Headers: ListBadgeChoices200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdateBadgeChoice serves PUT /api/v1/preferences/badges/{role_id}.
func (s *Server) UpdateBadgeChoice(ctx context.Context, req UpdateBadgeChoiceRequestObject) (UpdateBadgeChoiceResponseObject, error) {
	v, err := requireViewer(ctx)
	switch {
	case err != nil:
		return nil, err
	case req.Body == nil:
		return UpdateBadgeChoicedefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	case s.o.Badges == nil:
		return UpdateBadgeChoicedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	c, err := s.o.Badges.SetBadgeDisplayed(ctx, v.UserID, req.RoleId, req.Body.Displayed)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return UpdateBadgeChoicedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: the badge choice of %s for %s: %w", v.UserID, req.RoleId, err)
	}
	cache := cacheNoStore
	return UpdateBadgeChoice200JSONResponse{Body: newBadgeChoice(c), Headers: UpdateBadgeChoice200ResponseHeaders{CacheControl: &cache}}, nil
}
