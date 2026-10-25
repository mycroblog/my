// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/adapter/markdown"
	"github.com/mycroblog/my/internal/app/instance"
)

// setUpInstance registers the instance module's projectors and builds its service, which holds
// the instance settings and texts.
func (a *App) setUpInstance(repo instance.Repository) {
	instance.RegisterProjectors(a.Projectors, repo)
	a.Instance = instance.NewService(instance.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs,
		// The preview_image setting refers to a media file, which setMedia sets up later.
		Media: instanceMedia{a},
		// The contact_account setting names a local account.
		Accounts: instanceAccounts{a},
		// The terms of service, the privacy policy (#489), and the announcements (FR-726) are Markdown.
		Documents: markdown.New(),
		// The checks before the closure of the service count over other modules.
		ClosureCounts: closureCounts{a},
		// The progress of the closure's Deletes is the federation module's, set up later.
		ClosureDeliveries: closureDeliveries{a},
	})
}

// instanceMedia checks the media files of the instance settings with the media use cases.
type instanceMedia struct{ a *App }

func (m instanceMedia) CheckImage(ctx context.Context, mediaID string) error {
	if m.a.Media == nil {
		return errors.New("media files are not set up")
	}
	return m.a.Media.CheckImage(ctx, mediaID)
}

// closureCounts counts what the closure of the service ends (ADR-0088 §1) with the account,
// supporters, moderation, and federation modules. The modules set up later are read at call time.
type closureCounts struct{ a *App }

func (c closureCounts) CountLocalUsers(ctx context.Context) (int, error) {
	return c.a.Accounts.CountLocalUsers(ctx)
}

func (c closureCounts) CountKnownSharedInboxes(ctx context.Context) (int, error) {
	return c.a.Accounts.CountKnownSharedInboxes(ctx)
}

func (c closureCounts) LiveSubscriptionCount(ctx context.Context) (int, error) {
	if c.a.Supporters == nil {
		return 0, nil
	}
	return c.a.Supporters.LiveSubscriptionCount(ctx)
}

// PendingMigrations counts the Moves of local accounts that the federation module still sends
// (ADR-0089).
func (c closureCounts) PendingMigrations(ctx context.Context) (int, error) {
	if c.a.ClosureDeletes == nil {
		return 0, nil
	}
	return c.a.ClosureDeletes.PendingMoves(ctx)
}

func (c closureCounts) HeldAccounts(ctx context.Context) (int, error) {
	ids, err := c.a.Moderation.HeldAccountIDs(ctx)
	return len(ids), err
}
