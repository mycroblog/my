// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/adapter/webpush"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpPush builds the Web Push of the notifications (FR-206) when vapid_private_key is set:
// the sender on the SSRF-guarded HTTP client, the use cases of the client API, and the job
// that sends the messages. It returns the Pusher that the notification projectors enqueue the
// jobs with, nil when Web Push is off; the client API still answers the subscription
// operations then, and Instance.push_server_key is omitted.
func (a *App) setUpPush(cfg *config.Config, repo notification.PushRepository) (*notification.Pusher, error) {
	deps := notification.PushDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs, Audience: pushAudience{a},
		Messages: a.Messages, Host: cfg.Host, InstanceName: a.instanceName,
		// The closure of the service stops Web Push (ADR-0088 §2).
		Stopped: func(ctx context.Context) (bool, error) { return a.Instance.PushStopped(ctx) },
	}
	if cfg.VAPIDPrivateKey == "" {
		a.Push = notification.NewPushService(deps)
		return nil, nil
	}
	key, err := webpush.ParsePrivateKey(cfg.VAPIDPrivateKey)
	if err != nil {
		return nil, err
	}
	// The contact of RFC 8292 §2.1: the instance's own pages.
	sender, err := webpush.New(webpush.Options{PrivateKey: key, Subject: cfg.ServerOrigin(), HTTP: a.HTTPClient, Clock: a.Clock})
	if err != nil {
		return nil, err
	}
	deps.Sender = sender
	a.Push = notification.NewPushService(deps)
	a.Push.RegisterPushJobs(a.Handlers)
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	return &notification.Pusher{Repository: repo, Outbox: outbox}, nil
}

// pushAudience implements notification.PushAudience on the identity and account modules.
type pushAudience struct{ a *App }

// GrantActive implements notification.PushAudience.
func (p pushAudience) GrantActive(ctx context.Context, tokenFamilyID string) (bool, error) {
	return p.a.Identity.TokenFamilyActive(ctx, tokenFamilyID)
}

// Locale implements notification.PushAudience.
func (p pushAudience) Locale(ctx context.Context, accountID string) (string, error) {
	userID, err := moderatorStaff(p).userOf(ctx, accountID)
	if err != nil || userID == "" {
		return "", err
	}
	return p.a.Identity.UserLocale(ctx, userID)
}

// Account implements notification.PushAudience.
func (p pushAudience) Account(ctx context.Context, accountID string) (notification.PushAccount, error) {
	actor, err := p.a.Accounts.FindByID(ctx, accountID)
	if errors.Is(err, account.ErrNotFound) {
		return notification.PushAccount{}, notification.ErrNotFound
	}
	if err != nil {
		return notification.PushAccount{}, err
	}
	var out notification.PushAccount
	switch {
	case actor.Local != nil:
		out.Acct = actor.Local.Username
	case actor.Handle != nil:
		out.Acct = actor.Handle.Username
		if actor.Handle.Domain != domainaccount.LocalDomain {
			out.Acct += "@" + actor.Handle.Domain
		}
	}
	if actor.Profile != nil {
		out.DisplayName = actor.Profile.DisplayName
	}
	return out, nil
}

var _ notification.PushAudience = pushAudience{}
