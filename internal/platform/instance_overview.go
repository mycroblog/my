// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/instance"
)

// overviewSources reads the overview of the instance (GET /instance/overview) from the account
// and content modules.
type overviewSources struct{ a *App }

var _ instance.OverviewSources = overviewSources{}

func (o overviewSources) CountMembers(ctx context.Context) (int, error) {
	return o.a.Accounts.CountLocalUsers(ctx)
}

// StartedAt is when the instance actor was made, the first thing that the setup of an
// instance makes. An instance that has not federated yet (federation_mode = off from the start)
// may have none, so it is made here, as federation would make it on first use.
func (o overviewSources) StartedAt(ctx context.Context) (time.Time, error) {
	actor, err := o.a.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return actor.CreatedAt, nil
}

func (o overviewSources) DailyPublicPosts(ctx context.Context, to time.Time, days int) ([]int, error) {
	return o.a.Content.DailyLocalPublicPosts(ctx, to, days)
}
