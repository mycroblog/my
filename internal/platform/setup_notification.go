// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpNotification builds Web Push, registers the notification module's projectors, and builds
// its service and the notifications of the moderators, the service closure, and the supporters. It comes after the
// graph and content projectors, whose events these projectors turn into notifications.
func (a *App) setUpNotification(cfg *config.Config, repo notification.Repository, pushes notification.PushRepository) error {
	pusher, err := a.setUpPush(cfg, pushes)
	if err != nil {
		return err
	}
	deps := notification.ProjectorDeps{
		Repository: repo, Accounts: a.Accounts, Relationships: a.Graph, Reach: a.AccountPolicy, IDs: a.IDs, Push: pusher,
	}
	notification.RegisterProjectors(a.Projectors, deps)
	a.voiceNotifier = notification.NewVoiceNotifier(deps)
	a.Notifications = notification.NewService(notification.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, Staff: moderatorStaff{a},
	})
	a.Notifications.RegisterErrorGroupJobs(a.Handlers, a.Scheduler)
	moderators := a.setUpModeratorNotifications(cfg, repo, moderatorStaff{a}, pusher)
	a.setUpClosureNotifications(repo, moderators, pusher)
	a.setUpSupporterNotifications(cfg, repo, pusher)
	return nil
}
