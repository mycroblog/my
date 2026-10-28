// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/instance"
)

// overviewSources reads the overview of the instance (GET /instance/overview) from the account,
// content, and identity modules.
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

// OperatorAccountIDs maps the users who show the badge of a staff role to their accounts,
// leaving out a user who has none.
func (o overviewSources) OperatorAccountIDs(ctx context.Context) ([]string, error) {
	users, err := o.a.Identity.PublicStaffUserIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(users))
	for _, userID := range users {
		la, err := o.a.Accounts.LocalAccountOfUser(ctx, userID)
		switch {
		case errors.Is(err, account.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		}
		out = append(out, la.AccountID)
	}
	return out, nil
}
