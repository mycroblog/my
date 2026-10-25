// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpFederation registers the projectors that record outbound activities, after those of the
// modules whose events they follow (federation.md §7), and returns the outbox they enqueue the
// fan-out with.
func (a *App) setUpFederation(cfg *config.Config, repo federation.Repository) queue.Outbox {
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	federation.RegisterOutboundProjectors(a.Projectors, federation.RecorderDeps{
		Repository: repo, Outbox: outbox, IDs: a.IDs, Host: cfg.Host, Accounts: a.Accounts,
	})
	federation.RegisterBlockProjectors(a.Projectors, federation.RecorderDeps{
		Repository: repo, Outbox: outbox, IDs: a.IDs, Host: cfg.Host, Accounts: a.Accounts,
	})
	federation.RegisterSuspensionProjectors(a.Projectors, federation.RecorderDeps{
		Repository: repo, Outbox: outbox, IDs: a.IDs, Host: cfg.Host, Accounts: a.Accounts,
	})
	// Forwarded reports (FR-709) are Flags of the instance actor.
	federation.RegisterReportProjectors(a.Projectors, federation.RecorderDeps{
		Repository: repo, Outbox: outbox, IDs: a.IDs, Host: cfg.Host,
	}, a.Accounts)
	// The approvals of quotes (ADR-0069): the QuoteRequests, the answers, and the revocations.
	a.QuoteHandlers = federation.NewQuoteHandlers(federation.QuoteHandlersDeps{
		UnitOfWork: a.UnitOfWork,
		Recorder:   federation.RecorderDeps{Repository: repo, Outbox: outbox, IDs: a.IDs, Host: cfg.Host},
		Approvals:  a.Interactions, Quotes: a.Content, Statuses: a.Content, Accounts: a.Accounts,
		Fetcher: remoteDocuments{a}, // reads a.RemoteActors, built by setUpRemoteActors
		Clock:   a.Clock, Logger: a.Logger,
	})
	a.QuoteHandlers.RegisterProjectors(a.Projectors)
	a.setUpClosureDeletes(cfg, repo, outbox)
	return outbox
}

// remoteDocuments fetches remote documents through a.RemoteActors, which setUpRemoteActors
// builds after the projectors that read it are registered.
type remoteDocuments struct{ a *App }

func (r remoteDocuments) Fetch(ctx context.Context, uri string) (*federation.Fetched, error) {
	return r.a.RemoteActors.Fetch(ctx, uri)
}

// setUpPolls registers the end of polls and the Updates of local polls (FR-112): the jobs that
// close a poll at its end, enqueued in the transaction that posts it or votes in it, and the
// jobs that send the counts of a local poll after its votes and at its end.
func (a *App) setUpPolls(cfg *config.Config, statuses content.Repository, repo federation.Repository, outbox queue.Outbox) {
	closer := content.NewPollCloser(content.PollCloserDeps{
		UnitOfWork: a.UnitOfWork, Repository: statuses, Outbox: outbox, Clock: a.Clock,
	})
	closer.RegisterProjectors(a.Projectors)
	closer.RegisterJobs(a.Handlers)
	federation.NewPollUpdates(federation.PollUpdatesDeps{
		UnitOfWork: a.UnitOfWork, Statuses: a.Content, Clock: a.Clock,
		Recorder: federation.RecorderDeps{Repository: repo, Outbox: outbox, IDs: a.IDs, Host: cfg.Host},
	}).RegisterJobs(a.Handlers)
}

// setUpFanOut registers the fan-out of outbound activities (federation.md §7.2). It comes after
// the status reader, which resolves the media, mentions, and hashtags of the Notes it builds.
func (a *App) setUpFanOut(cfg *config.Config, repo federation.Repository, outbox queue.Outbox) {
	a.FanOut = federation.NewFanOut(federation.FanOutDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Outbox: outbox,
		Statuses: federation.ContentStatuses{Service: a.Content, Interactions: a.Interactions},
		Accounts: a.Accounts, Graph: a.Graph, Parts: a.Statuses, Follows: a.Graph, Images: a.Media.Images, Moves: a.Accounts,
		EmojiTags: a.Media.EmojiTags, Quotes: a.Content,
		Blocks:  a.Graph,
		Reports: a.Moderation,
		Relays:  a.Relays,
		Domains: a.ModerationPolicy,
		Clock:   a.Clock, IDs: a.IDs, Host: cfg.Host, AccountDomain: cfg.AccountDomain, Logger: a.Logger,
		Suspensions: a.ModerationPolicy,
		// The federation mode decides what goes out when each batch runs (ADR-0076), and the
		// service closure before it (ADR-0088 §2).
		Modes: federationModes{a}, Closure: closureSettings{a},
	})
	a.FanOut.RegisterJobs(a.Handlers)
}

// newObjects builds the use case behind the ActivityPub objects and collections of local
// statuses (federation.md §10). Signed GETs are verified with the HTTP signature adapter and
// the keys that RemoteActors resolves, so setUpRemoteActors runs first.
func (a *App) newObjects(cfg *config.Config) *federation.Objects {
	return federation.NewObjects(federation.ObjectsDeps{
		Statuses: a.Content, Accounts: a.Accounts, Graph: a.Graph, Timeline: a.Timeline, Reader: a.Statuses, Images: a.Media.Images,
		Quotes:     federation.ContentQuotes{Service: a.Content, Interactions: a.Interactions},
		Signatures: federation.NewSignatureVerifier(httpsig.New(), a.RemoteActors, a.KV, a.Clock),
		Domains:    a.ModerationPolicy,
		// Authorized fetch (FR-408) is the instance setting, read per request.
		Settings: a.Instance,
		Host:     cfg.Host, AccountDomain: cfg.AccountDomain, Logger: a.Logger,
	})
}
