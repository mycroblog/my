// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/instance"
)

// retentionRepositories are the persistence of the purges of federated data; the database
// adapters' repositories implement them.
type retentionRepositories struct {
	statuses content.RetentionRepository
	purges   federation.PurgeRepository
	profiles account.ProfilePruneRepository
	emojis   content.EmojiPurgeRepository
	// autoDeletes is the persistence of the users' automatic deletion of their old posts.
	autoDeletes content.AutoDeleteRepository
}

// setUpRetention registers the purges of federated data and their schedules (ADR-0006, FR-803):
// the remote statuses past remote_status_retention_days (or remote_status_max_retention_days), the ones
// that only relays brought in past relay_status_retention_days (ADR-0006), the inbox
// and delivery records, the past profiles of unrelated remote accounts, and the remote custom
// emoji that nothing refers to any more (ADR-0051 §1). The statuses that the moderation records
// need stay (ADR-0058). It also sets up the users' automatic deletion of their own old posts
// (FR-118) and its schedule. It comes after the content, graph, and notification services, which
// tell what local data refers to.
func (a *App) setUpRetention(repos retentionRepositories, statuses content.Repository) {
	content.NewRetention(content.RetentionDeps{
		UnitOfWork: a.UnitOfWork, Repository: statuses, Retention: repos.statuses, Notifications: a.Notifications,
		Evidence: a.Moderation,
		Jobs:     a.Jobs, Clock: a.Clock, Periods: a.remoteStatusPeriods,
	}).RegisterJobs(a.Handlers, a.Scheduler)
	// The posts that only relays brought in (ADR-0006).
	content.NewRelayedRetention(content.RelayedRetentionDeps{
		UnitOfWork: a.UnitOfWork, Repository: statuses, Retention: repos.statuses, Notifications: a.Notifications,
		Evidence: a.Moderation, Follows: followedAuthors{a},
		Jobs: a.Jobs, Clock: a.Clock, Period: a.relayStatusRetention,
	}).RegisterJobs(a.Handlers, a.Scheduler)
	federation.NewPurger(federation.PurgeDeps{
		UnitOfWork: a.UnitOfWork, Repository: repos.purges, Clock: a.Clock,
	}).RegisterJobs(a.Handlers, a.Scheduler)
	account.NewProfilePruner(account.PruneDeps{
		UnitOfWork: a.UnitOfWork, Repository: repos.profiles, Relations: remoteRelations{a}, Clock: a.Clock, Jobs: a.Jobs,
	}).RegisterJobs(a.Handlers, a.Scheduler)
	// The profiles of the account module use emoji, and their avatars and headers may share a
	// remote media file with an emoji image.
	content.NewEmojiPurge(content.EmojiPurgeDeps{
		UnitOfWork: a.UnitOfWork, Repository: repos.emojis, Referrers: []content.EmojiReferrer{a.Accounts},
		MediaReferrers: []content.MediaReferrer{a.Accounts}, Jobs: a.Jobs,
	}).RegisterJobs(a.Handlers, a.Scheduler)
	// The local accounts' automatic deletion of their own old posts (FR-118), which deletes them
	// as their authors would and so sends a Delete of each.
	a.AutoDelete = content.NewAutoDelete(content.AutoDeleteDeps{
		UnitOfWork: a.UnitOfWork, Repository: statuses, AutoDeletes: repos.autoDeletes, Accounts: a.Accounts,
		Jobs: a.Jobs, Clock: a.Clock,
	})
	a.AutoDelete.RegisterJobs(a.Handlers, a.Scheduler)
}

// remoteRelations tells the account module whether something local relates to a remote account:
// a follow either way (the graph module) or a stored status of it (the content module), which
// both import the account module.
type remoteRelations struct{ a *App }

func (r remoteRelations) RelatedToLocal(ctx context.Context, accountID string) (bool, error) {
	c, err := r.a.Graph.Counters(ctx, accountID)
	if err != nil || c.FollowersCount > 0 || c.FollowingCount > 0 {
		return err == nil, err
	}
	n, err := r.a.Content.PostCount(ctx, accountID)
	return n > 0, err
}

// remoteStatusPeriods reads the periods of the purge of remote statuses.
func (a *App) remoteStatusPeriods(ctx context.Context) (content.RetentionPeriods, error) {
	s, err := a.Instance.Settings(ctx)
	if err != nil {
		return content.RetentionPeriods{}, err
	}
	return content.RetentionPeriods{
		Period:    s.Retention(instance.SettingRemoteStatusRetentionDays),
		MaxPeriod: s.Retention(instance.SettingRemoteStatusMaxRetentionDays),
	}, nil
}

// relayStatusRetention reads how long the posts that only relays brought in are kept.
func (a *App) relayStatusRetention(ctx context.Context) (time.Duration, error) {
	s, err := a.Instance.Settings(ctx)
	if err != nil {
		return 0, err
	}
	return s.Retention(instance.SettingRelayStatusRetentionDays), nil
}
