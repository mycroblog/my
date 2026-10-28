// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpModeratorNotifications registers the moderator notifications (FR-712): the projectors
// that create them from new reports and pending registrations, and, unless mail = none, the
// job that sends them by email to the moderators who get them by email. push enqueues their Web
// Push messages; nil pushes none. It needs the identity and notification modules, and returns
// the ports of the moderator notifications for the others that go to the staff.
func (a *App) setUpModeratorNotifications(cfg *config.Config, repo notification.Repository, staff notification.Staff, push *notification.Pusher) notification.ModeratorDeps {
	deps := notification.ModeratorDeps{Repository: repo, Staff: staff, IDs: a.IDs, Push: push}
	if sendsEmail(cfg) {
		deps.Outbox = a.mailOutbox()
		notification.RegisterModeratorJobs(a.Handlers, notification.MailDeps{
			Staff: staff, Settings: a.Notifications, Mailboxes: notificationMailboxes{a}, Mailer: a.Mailer, Rejections: a.Identity,
			Messages: a.Messages, Host: cfg.Host, WebOrigin: cfg.WebOrigin, MailSite: a.mailSite,
		})
	}
	notification.RegisterModeratorProjectors(a.Projectors, deps)
	return deps
}

// mailOutbox returns the outbox that enqueues the jobs of the notification emails in the
// transactions of their notifications.
func (a *App) mailOutbox() queue.Outbox {
	if a.Outbox != nil {
		return a.Outbox
	}
	// The inline queue has no transactional enqueue.
	return queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
}

// sendsEmail tells whether the instance sends email: mail is set and is not none.
func sendsEmail(cfg *config.Config) bool { return cfg.Mail != "none" && cfg.Mail != "" }

// moderatorStaff implements notification.Staff on the identity and account modules: the roles
// of a local account's user.
type moderatorStaff struct{ a *App }

// PermissionHolders implements notification.Staff.
func (s moderatorStaff) PermissionHolders(ctx context.Context, tx uow.Tx, p domainnotification.Permission) ([]notification.StaffMember, error) {
	holders, err := s.a.Identity.PermissionHolders(ctx, tx, domainidentity.Permission(p))
	if err != nil {
		return nil, err
	}
	out := make([]notification.StaffMember, len(holders))
	for i, h := range holders {
		out[i] = notification.StaffMember{AccountID: h.AccountID}
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

// notificationMailboxes implements notification.Mailboxes on the identity and account modules:
// the verified primary address of a local account's user, and the user's unsubscribe links.
type notificationMailboxes struct{ a *App }

// Mailbox implements notification.Mailboxes.
func (m notificationMailboxes) Mailbox(ctx context.Context, accountID string) (notification.Mailbox, bool, error) {
	userID, err := moderatorStaff(m).userOf(ctx, accountID)
	if err != nil || userID == "" {
		return notification.Mailbox{}, false, err
	}
	box, ok, err := m.a.Identity.NotificationMailbox(ctx, userID)
	return notification.Mailbox{EmailAddressID: box.EmailAddressID, Address: box.Address, Locale: box.Locale}, ok, err
}

// UnsubscribeLink implements notification.Mailboxes.
func (m notificationMailboxes) UnsubscribeLink(ctx context.Context, accountID string, g domainnotification.Group) (string, error) {
	userID, err := moderatorStaff(m).userOf(ctx, accountID)
	if err != nil {
		return "", err
	}
	if userID == "" {
		return "", fmt.Errorf("platform: account %s has no user to unsubscribe", accountID)
	}
	return m.a.Identity.IssueUnsubscribeLink(ctx, userID, domainidentity.UnsubscribeCategory(g))
}

var _ notification.Mailboxes = notificationMailboxes{}

// emailUnsubscriptions implements identity.EmailUnsubscriptions on the notification settings:
// the category of an unsubscribe link is a group of the settings, and the link turns its email
// off (ADR-0109 §4).
type emailUnsubscriptions struct{ a *App }

// Unsubscribable implements identity.EmailUnsubscriptions.
func (u emailUnsubscriptions) Unsubscribable(category domainidentity.UnsubscribeCategory) bool {
	return notification.Unsubscribable(domainnotification.Group(category))
}

// Unsubscribe implements identity.EmailUnsubscriptions.
func (u emailUnsubscriptions) Unsubscribe(ctx context.Context, userID string, category domainidentity.UnsubscribeCategory) error {
	la, err := u.a.Accounts.LocalAccountOfUser(ctx, userID)
	if errors.Is(err, account.ErrNotFound) {
		return fmt.Errorf("platform: user %s has no local account: %w", userID, appidentity.ErrNotFound)
	}
	if err != nil {
		return err
	}
	return u.a.Notifications.TurnOffEmail(ctx, la.AccountID, domainnotification.Group(category))
}

var _ appidentity.EmailUnsubscriptions = emailUnsubscriptions{}
