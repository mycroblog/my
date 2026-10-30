// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// OverviewReader reads the counts of the instance, kept briefly. *appinstance.Overview
// implements it.
type OverviewReader interface {
	Counts(ctx context.Context) (appinstance.Counts, error)
}

// GetInstanceOverview serves GET /api/v1/instance/overview: the counts that the landing page
// shows about the instance, while the instance setting public_counts shows them. The answer is
// the same for every viewer.
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
	cache := cachePublic
	return GetInstanceOverview200JSONResponse{Body: out, Headers: GetInstanceOverview200ResponseHeaders{CacheControl: &cache}}, nil
}
