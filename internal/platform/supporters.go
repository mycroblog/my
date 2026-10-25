// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/supporters"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpSupporters builds the supporter program (ADR-0072): the service of the plans and the
// subscriptions on the payment provider of payments_provider (none keeps the program from being
// enabled), its projectors on the accounts' deletions and suspensions, and its jobs. It comes
// after the identity, account, moderation, and instance services, and before the erasure, which
// erases the subscribers' listings and customers. It returns the provider for setUpPayments.
func (a *App) setUpSupporters(ctx context.Context, cfg *config.Config, repo supporters.Repository, opts AppOptions) (supporters.PaymentProvider, error) {
	prov, err := newPaymentProvider(ctx, cfg, opts, a.Logger)
	if err != nil {
		return nil, err
	}
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Supporters = supporters.NewService(supporters.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Roles: a.Identity, Users: supporterUsers{a}, Accounts: a.Accounts,
		Provider: prov, SendsEmail: cfg.Mail != "none",
		CommerceDisclosure: func(ctx context.Context) (bool, error) {
			return a.Instance.HasDocument(ctx, domaininstance.TextCommerceDisclosure)
		},
		Income: financeIncome{a},
		// The announcement of the service closure stops new subscriptions (ADR-0088 §2).
		Closure: a.Instance,
		Outbox:  outbox, Clock: a.Clock, IDs: a.IDs, Logger: a.Logger,
	})
	a.Supporters.RegisterProjectors(a.Projectors)
	a.Supporters.RegisterJobs(a.Handlers, a.Scheduler)
	return prov, nil
}

// supporterSync returns what applies the payment provider's facts to the subscriptions:
// AppOptions.SupporterSync, or the supporters core.
func (a *App) supporterSync(opts AppOptions) supporters.Syncer {
	if opts.SupporterSync != nil {
		return opts.SupporterSync
	}
	return a.Supporters
}

// setUpSupporterNotifications registers the supporter notifications (ADR-0072 §16): the
// projectors that create them from the subscriptions' transitions and the plans' retirements,
// and, unless mail = none, the job that sends them by email. push enqueues their Web Push
// messages; nil pushes none.
func (a *App) setUpSupporterNotifications(cfg *config.Config, repo notification.Repository, push *notification.Pusher) {
	deps := notification.SupporterDeps{Repository: repo, IDs: a.IDs, Push: push}
	if sendsEmail(cfg) {
		deps.Outbox = a.Outbox
		if deps.Outbox == nil {
			// The inline queue has no transactional enqueue.
			deps.Outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
		}
		notification.RegisterSupporterJobs(a.Handlers, notification.SupporterMailDeps{
			Mailboxes: supporterUsers{a}, Mailer: a.Mailer, Rejections: a.Identity, Messages: a.Messages, Host: cfg.Host, WebOrigin: cfg.WebOrigin,
			MailSite: a.mailSite,
		})
	}
	notification.RegisterSupporterProjectors(a.Projectors, deps)
}

// supporterUsers reads the subscribers for the supporters module from the account, identity, and
// moderation modules (ADR-0072 §6), and their mailboxes for the supporter notifications.
type supporterUsers struct{ a *App }

// SupporterMailbox implements notification.SupporterMailboxes: the primary address and the
// locale of the user of a local account, unless the suppression of the address stops
// notifications (ADR-0086 §5).
func (u supporterUsers) SupporterMailbox(ctx context.Context, accountID string) (notification.Mailbox, bool, error) {
	userID, err := moderatorStaff(u).userOf(ctx, accountID)
	if err != nil || userID == "" {
		return notification.Mailbox{}, false, err
	}
	user, err := u.SupporterUser(ctx, userID)
	if errors.Is(err, supporters.ErrNotFound) {
		return notification.Mailbox{}, false, nil
	}
	if err != nil || !user.EmailVerified {
		return notification.Mailbox{}, false, err
	}
	addressID, ok, err := u.a.Identity.NotificationAddress(ctx, userID)
	if err != nil || !ok {
		return notification.Mailbox{}, false, err
	}
	return notification.Mailbox{EmailAddressID: addressID, Address: user.Email, Locale: user.Locale}, true, nil
}

// AccountIDs implements supporters.Users.
func (u supporterUsers) AccountIDs(ctx context.Context, userIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(userIDs))
	for _, id := range userIDs {
		la, err := u.a.Accounts.LocalAccountOfUser(ctx, id)
		if errors.Is(err, account.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = la.AccountID
	}
	return out, nil
}

// SupporterUser implements supporters.Users.
func (u supporterUsers) SupporterUser(ctx context.Context, userID string) (supporters.User, error) {
	la, err := u.a.Accounts.LocalAccountOfUser(ctx, userID)
	if errors.Is(err, account.ErrNotFound) {
		return supporters.User{}, fmt.Errorf("%w: user %s", supporters.ErrNotFound, userID)
	}
	if err != nil {
		return supporters.User{}, err
	}
	actor, err := u.a.Accounts.FindByID(ctx, la.AccountID)
	if err != nil {
		return supporters.User{}, err
	}
	out := supporters.User{ID: userID, AccountID: la.AccountID, Handle: "@" + la.Username}
	switch actor.Lifecycle.State {
	case domainaccount.StateDeleted, domainaccount.StateDeletionScheduled:
		out.Closed = true
	}
	if out.Suspended, err = u.a.ModerationPolicy.AccountFederationSuspended(ctx, la.AccountID); err != nil {
		return supporters.User{}, err
	}
	// A primary address is a verified one (user_primary_emails, data-model §4.1).
	if out.Email, err = u.a.Identity.PrimaryEmailAddress(ctx, userID); err != nil {
		return supporters.User{}, err
	}
	out.EmailVerified = out.Email != ""
	prefs, err := u.a.Identity.Preferences(ctx, userID)
	switch {
	case err == nil:
		out.Locale = prefs.Locale
	case !errors.Is(err, identity.ErrNotFound):
		return supporters.User{}, err
	}
	return out, nil
}
