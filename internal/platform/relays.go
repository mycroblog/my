// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpRelays builds the relays (ADR-0006): their administration, the Follow and Undo of the
// instance actor that join and leave them, and the gate of what they bring in. It needs
// RemoteActors, which fetches the actor of a LitePub relay, and comes before the inbox and the
// fan-out, which use it.
func (a *App) setUpRelays(cfg *config.Config, repo federationRepository) {
	outbox := a.Outbox
	if outbox == nil {
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Relays = federation.NewRelays(federation.RelaysDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Outbound: repo, Outbox: outbox,
		Instance: ensuredInstanceActor{a}, Fetcher: a.RemoteActors, Accounts: a.Accounts, Inboxes: a.Accounts,
		Domains: a.ModerationPolicy, Policy: a.ModerationPolicy,
		KV: a.KV, Clock: a.Clock, IDs: a.IDs, Host: cfg.Host, HourlyLimit: cfg.RelayIngestHourlyLimit,
		Logger: a.Logger, Meter: a.Meter,
	})
}

// followedAuthors tells the content module whether a local account follows a remote author, from
// the graph module's counters: a remote account's followers are local accounts.
type followedAuthors struct{ a *App }

func (f followedAuthors) AuthorFollowed(ctx context.Context, accountID string) (bool, error) {
	c, err := f.a.Graph.Counters(ctx, accountID)
	return err == nil && c.FollowersCount > 0, err
}

// ensuredInstanceActor returns the instance actor, creating it on first use: a relay may be
// joined before anything else needed the actor.
type ensuredInstanceActor struct{ a *App }

func (e ensuredInstanceActor) InstanceActor(ctx context.Context) (account.Actor, error) {
	if _, err := e.a.Accounts.EnsureInstanceActor(ctx); err != nil {
		return account.Actor{}, err
	}
	return e.a.Accounts.InstanceActor(ctx)
}
