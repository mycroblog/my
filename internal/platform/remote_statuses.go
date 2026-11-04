// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	domaincontent "github.com/mycroblog/my/internal/domain/content"
)

// remoteMediaRecorder records the attachments and custom emoji images of remote statuses for the
// content module. It reads a.Media per call, since the content module is built before setMedia.
type remoteMediaRecorder struct{ a *App }

func (r remoteMediaRecorder) RecordRemoteMedia(ctx context.Context, tx uow.Tx, in content.RemoteMediaInput) (domaincontent.RemoteMedia, error) {
	return r.a.Media.RecordRemoteMedia(ctx, tx, in)
}

// registerStatusHandlers registers the inbox handlers of remote posts (Create, Update, and
// Delete, docs/architecture/federation.md §3.4) on the inbox processor: the posts related to
// local accounts are ingested through the content module (ADR-0006). The boosts and favourites
// (Announce, Like, EmojiReact, and their Undo) go through the same posts. It also registers the
// fetches of remote posts, threads, outboxes, and featured collections (federation.fetch_object,
// federation.fetch_outbox, and federation.fetch_featured, §6.2, §6.3), builds the Resolver, and
// the refetches of remote posts that operators request (federation.refetch_status, §6.4).
func (a *App) registerStatusHandlers(repo federation.InboxRepository) {
	outbox := a.Outbox
	if outbox == nil {
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.RemoteStatuses = federation.NewStatusHandlers(federation.StatusHandlersDeps{
		UnitOfWork: a.UnitOfWork, Statuses: a.Content, Accounts: a.Accounts, Actors: a.RemoteActors,
		Fetcher: a.RemoteActors, Followers: a.Graph, Tombstones: repo, Outbox: outbox, Jobs: a.Jobs, KV: a.KV,
		Profiles: a.Accounts, Relationships: a.Graph, Restrictions: a.ModerationPolicy, Pins: a.Interactions, Votes: a.Interactions,
		// The posts that only relays bring in are kept for relay_status_retention_days (ADR-0006).
		Relayed: a.Content, RelayPolicy: a.ModerationPolicy, Quotes: a.Content, Approvals: a.Interactions,
		Clock: a.Clock, Host: a.Config.Host, Logger: a.Logger,
	})
	a.RemoteStatuses.Register(a.InboxProcessor)
	a.RemoteStatuses.RegisterJobs(a.Handlers)
	federation.RegisterInteractionHandlers(a.InboxProcessor, federation.InteractionHandlersDeps{
		Interactions: a.Interactions, Statuses: a.RemoteStatuses,
	})
	a.Resolver = federation.NewResolver(a.RemoteActors, a.RemoteStatuses)
	// The refetch of remote posts that operators request; nothing schedules it (FR-407, §6.4).
	a.Refetches = federation.NewRefetches(federation.RefetchesDeps{
		UnitOfWork: a.UnitOfWork, Statuses: a.Content, Accounts: a.Accounts, Outbox: outbox, Jobs: a.Jobs,
		Clock: a.Clock, IDs: a.IDs, Host: a.Config.Host,
	})
	a.Refetches.RegisterJobs(a.Handlers, a.RemoteStatuses)
}
