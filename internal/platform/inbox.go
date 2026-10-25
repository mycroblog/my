// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/adapter/httpsig"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpInbox builds the synchronous part of the inbox (docs/architecture/federation.md §3.1):
// signatures verified with the HTTP signature adapter and the keys that RemoteActors resolves,
// the IP and domain rate limits, and the record and job written once per activity. It needs
// RemoteActors and InstanceHealth.
func (a *App) setUpInbox(cfg *config.Config, repo federationRepository) {
	outbox := a.Outbox
	if outbox == nil {
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Inbox = federation.NewInbox(federation.InboxDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Outbox: outbox,
		Signatures: federation.NewSignatureVerifier(httpsig.New(), a.RemoteActors, a.KV, a.Clock),
		Accounts:   a.Accounts,
		Domains:    a.ModerationPolicy,
		Health:     a.InstanceHealth, RateLimiter: a.RateLimiter, Clock: a.Clock, Logger: a.Logger, Meter: a.Meter,
		// The activities that accepted relays sign carry the relay (ADR-0061).
		Relays: a.Relays,
		// The mode off refuses every request (ADR-0076).
		Modes: federationModes{a},
	})
	a.setUpInboxProcessor(cfg, repo, outbox)
}

// setUpInboxProcessor builds the asynchronous part of the inbox (docs/architecture/federation.md
// §3.2): the federation.inbox job, which resolves actors and fetches documents through
// RemoteActors and dispatches each activity to the handler of its type.
func (a *App) setUpInboxProcessor(cfg *config.Config, repo federationRepository, outbox queue.Outbox) {
	a.InboxProcessor = federation.NewInboxProcessor(federation.InboxProcessorDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Actors: a.RemoteActors, Fetcher: a.RemoteActors,
		Domains: a.ModerationPolicy, Accounts: a.ModerationPolicy,
		// What relays bring in passes their gate before it is fetched (ADR-0061).
		Relays: a.Relays,
		// The federation mode at the time of processing sorts the activities by flow (ADR-0076).
		Modes: federationModes{a}, Host: cfg.Host,
		Clock: a.Clock, Logger: a.Logger,
	})
	federation.RegisterModerationHandlers(a.InboxProcessor, federation.ModerationHandlerDeps{
		Blocks: a.Graph, Reports: a.Moderation, Policy: a.ModerationPolicy,
		Objects: federation.LocalObjectsOf{Accounts: a.Accounts, Content: a.Content}, Host: a.Config.Host,
	})
	a.RemoteActors.RegisterInboxHandlers(a.InboxProcessor)
	a.InboxProcessor.RegisterJobs(a.Handlers)
	a.registerStatusHandlers(repo)
	// The handlers of each activity type (federation.md §3.4).
	federation.RegisterFollowHandlers(a.InboxProcessor, federation.FollowHandlerDeps{
		Graph: a.Graph, Accounts: a.Accounts, Relays: a.Relays, Quotes: a.QuoteHandlers, Moves: a.Accounts,
		// The Follows that the federation mode refuses get a Reject (ADR-0076).
		Refusals: federation.NewFollowRefusals(federation.FollowRefusalsDeps{
			UnitOfWork: a.UnitOfWork, Outbound: repo, Outbox: outbox, Inboxes: a.Accounts,
			Clock: a.Clock, IDs: a.IDs, Host: cfg.Host,
		}),
	})
	a.QuoteHandlers.Register(a.InboxProcessor)
	// The Moves of remote accounts move their local followers (ADR-0089).
	federation.RegisterMoveHandler(a.InboxProcessor, federation.MoveHandlerDeps{
		Targets: a.RemoteActors, Accounts: a.Accounts,
		Domains: a.ModerationPolicy, Suspensions: a.ModerationPolicy, Limits: a.ModerationPolicy,
		Host: cfg.Host, Meter: a.Meter, Logger: a.Logger,
	})
}
