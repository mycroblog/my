// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// OverviewReader reads the counts and the operators of the instance, each kept briefly.
// *appinstance.Overview implements it.
type OverviewReader interface {
	Counts(ctx context.Context) (appinstance.Counts, error)
	OperatorAccountIDs(ctx context.Context) ([]string, error)
}

// GetInstanceOverview serves GET /api/v1/instance/overview: what the landing page shows about
// the instance, as far as the instance settings public_counts and public_operators show it.
// The answer is the same for every viewer: the operators are presented as an anonymous visitor
// sees them, with the badges they show on their profile.
func (s *Server) GetInstanceOverview(ctx context.Context, _ GetInstanceOverviewRequestObject) (GetInstanceOverviewResponseObject, error) {
	var settings instance.Settings
	if s.o.Instance != nil {
		var err error
		if settings, err = s.o.Instance.Settings(ctx); err != nil {
			return nil, fmt.Errorf("clientapi: instance settings: %w", err)
		}
	}
	var out InstanceOverview
	if s.o.Overview != nil && settings.PublicCounts() {
		c, err := s.o.Overview.Counts(ctx)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the counts of the instance: %w", err)
		}
		out.Counts = &InstanceCounts{Members: c.Members, DailyPosts: c.DailyPosts, StartedAt: c.StartedAt, CountedAt: c.CountedAt}
	}
	if s.o.Overview != nil && settings.PublicOperators() {
		operators, err := s.operators(ctx)
		if err != nil {
			return nil, err
		}
		out.Operators = &operators
	}
	cache := cachePublic
	return GetInstanceOverview200JSONResponse{Body: out, Headers: GetInstanceOverview200ResponseHeaders{CacheControl: &cache}}, nil
}

// operators presents the accounts of the operators that an anonymous visitor may see, in order,
// each with the badges it shows on its profile.
func (s *Server) operators(ctx context.Context) ([]Account, error) {
	ids, err := s.o.Overview.OperatorAccountIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the operators of the instance: %w", err)
	}
	var actors []appaccount.Actor
	for _, id := range ids {
		a, ok, err := s.presentableByID(ctx, nil, id, false)
		if err != nil {
			return nil, fmt.Errorf("clientapi: operator %s: %w", id, err)
		}
		if ok && a.Local != nil {
			actors = append(actors, a)
		}
	}
	out, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	for i, a := range actors {
		if out[i].Badges, err = s.profileBadges(ctx, a.Local.UserID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
