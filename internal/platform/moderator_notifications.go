// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpModeratorNotifications registers the moderator notifications (FR-712): the projectors
// that create them from new reports and pending registrations, and, unless mail = none, the
// job that sends them by email to the moderators who want that. push enqueues their Web Push
// messages; nil pushes none. It needs the identity and notification modules.
func (a *App) setUpModeratorNotifications(cfg *config.Config, repo notification.Repository, staff notification.Staff, push *notification.Pusher) {
	deps := notification.ModeratorDeps{Repository: repo, Staff: staff, IDs: a.IDs, Push: push}
	if sendsEmail(cfg) {
		deps.Outbox = a.Outbox
		if deps.Outbox == nil {
			// The inline queue has no transactional enqueue.
			deps.Outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
		}
		notification.RegisterModeratorJobs(a.Handlers, notification.MailDeps{
			Staff: staff, Mailer: a.Mailer, Messages: a.Messages, Host: cfg.Host, WebOrigin: cfg.WebOrigin,
			MailSite: a.mailSite,
		})
	}
	notification.RegisterModeratorProjectors(a.Projectors, deps)
}

// sendsEmail tells whether the instance sends email: mail is set and is not none.
func sendsEmail(cfg *config.Config) bool { return cfg.Mail != "none" && cfg.Mail != "" }

// moderatorStaff implements notification.Staff on the identity and account modules: the roles
// of a local account's user and their preference for email.
type moderatorStaff struct{ a *App }

// PermissionHolders implements notification.Staff.
func (s moderatorStaff) PermissionHolders(ctx context.Context, tx uow.Tx, p domainnotification.Permission) ([]notification.StaffMember, error) {
	holders, err := s.a.Identity.PermissionHolders(ctx, tx, domainidentity.Permission(p))
	if err != nil {
		return nil, err
	}
	out := make([]notification.StaffMember, len(holders))
	for i, h := range holders {
		out[i] = notification.StaffMember{AccountID: h.AccountID, Email: h.EmailModeratorNotifications}
	}
	return out, nil
}

// GrantedPermissions implements notification.Staff.
func (s moderatorStaff) GrantedPermissions(ctx context.Context, accountID string) ([]domainnotification.Permission, error) {
	userID, err := s.userOf(ctx, accountID)
	if err != nil || userID == "" {
		return nil, err
	}
	perms, err := s.a.Identity.Permissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domainnotification.Permission, len(perms))
	for i, p := range perms {
		out[i] = domainnotification.Permission(p)
	}
	return out, nil
}

// Mailbox implements notification.Staff.
func (s moderatorStaff) Mailbox(ctx context.Context, accountID string) (notification.Mailbox, bool, error) {
	userID, err := s.userOf(ctx, accountID)
	if err != nil || userID == "" {
		return notification.Mailbox{}, false, err
	}
	address, locale, ok, err := s.a.Identity.ModeratorMailbox(ctx, userID)
	return notification.Mailbox{Address: address, Locale: locale}, ok, err
}

// userOf returns the user of a local account; "" for any other account.
func (s moderatorStaff) userOf(ctx context.Context, accountID string) (string, error) {
	actor, err := s.a.Accounts.FindByID(ctx, accountID)
	switch {
	case errors.Is(err, account.ErrNotFound):
		return "", nil
	case err != nil:
		return "", err
	case actor.Local == nil:
		return "", nil
	}
	return actor.Local.UserID, nil
}

var _ notification.Staff = moderatorStaff{}
