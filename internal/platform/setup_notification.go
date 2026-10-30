// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpNotification builds Web Push, registers the notification module's projectors, and builds
// its service and the notifications of the moderators, the service closure, the announcements, and the supporters. It comes after the
// graph and content projectors, whose events these projectors turn into notifications. Unless
// mail = none, the notifications that their recipients get by email are emailed once they stay
// unread (ADR-0109 §4).
func (a *App) setUpNotification(cfg *config.Config, repo notification.Repository, pushes notification.PushRepository) error {
	a.Notifications = notification.NewService(notification.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, Staff: moderatorStaff{a}, Announcements: a.Instance,
		// The supporter row of the settings is offered while the program is on; the supporters
		// module is built after this one.
		SupportersEnabled: func(ctx context.Context) (bool, error) {
			if a.Supporters == nil {
				return false, nil
			}
			return a.Supporters.Enabled(ctx)
		},
	})
	a.Notifications.RegisterErrorGroupJobs(a.Handlers, a.Scheduler)
	pusher, err := a.setUpPush(cfg, pushes)
	if err != nil {
		return err
	}
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	deps := notification.ProjectorDeps{
		Repository: repo, Accounts: a.Accounts, Relationships: a.Graph, Reach: a.AccountPolicy, IDs: a.IDs, Push: pusher,
		PostJobs: outbox,
	}
	a.Subscriptions = notification.NewSubscriptions(notification.SubscriptionDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Accounts: a.Accounts, Relationships: a.Graph, Clock: a.Clock,
	})
	if sendsEmail(cfg) {
		deps.Email = &notification.EmailScheduler{Outbox: a.mailOutbox(), Clock: a.Clock}
		notification.RegisterEmailJobs(a.Handlers, notification.EmailDeps{
			Notifications: a.Notifications, Audience: pushAudience{a}, Mailboxes: notificationMailboxes{a},
			Mailer: a.Mailer, Rejections: a.Identity, Messages: a.Messages, Host: cfg.Host, InstanceName: a.instanceName,
			MailSite: a.mailSite, WebOrigin: cfg.WebOrigin,
		})
	}
	notification.RegisterProjectors(a.Projectors, deps)
	notification.RegisterPostNotificationJobs(a.Handlers, notification.PostNotificationDeps{
		UnitOfWork: a.UnitOfWork, Posts: subscribedPosts{a}, Projectors: deps,
	})
	a.voiceNotifier = notification.NewVoiceNotifier(deps)
	moderators := a.setUpModeratorNotifications(cfg, repo, moderatorStaff{a}, pusher)
	a.setUpClosureNotifications(repo, moderators, pusher)
	a.setUpAnnouncementNotifications(repo, pusher)
	a.setUpSupporterNotifications(cfg, repo, pusher)
	return nil
}

// subscribedPosts implements notification.Posts on the content module.
type subscribedPosts struct{ a *App }

// StatusVisible implements notification.Posts through the content module's visibility policy.
func (p subscribedPosts) StatusVisible(ctx context.Context, accountID, statusID string) (bool, error) {
	return pushAudience(p).StatusVisible(ctx, accountID, statusID)
}

// AuthorOf implements notification.Posts.
func (p subscribedPosts) AuthorOf(ctx context.Context, uri string) (string, error) {
	st, err := p.a.Content.FindStatusByURI(ctx, uri)
	switch {
	case errors.Is(err, content.ErrNotFound):
		return "", nil
	case err != nil:
		return "", err
	}
	return st.AuthorAccountID, nil
}

var _ notification.Posts = subscribedPosts{}
