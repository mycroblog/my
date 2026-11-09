// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/htmlsanitizer"
	"github.com/mycroblog/my/internal/adapter/httpsig"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
	domaincontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/platform/config"
)

// federationRepository is what the database adapters provide to the federation module: the
// outbound side, the remote servers seen, and the received activities.
type federationRepository interface {
	federation.Repository
	federation.InstanceRepository
	federation.InboxRepository
	federation.RelayRepository
}

// newHTMLSanitizer returns the sanitizer of remote HTML (ADR-0019 §2).
func newHTMLSanitizer() kernel.HTMLSanitizer { return htmlsanitizer.New() }

// recordRemoteImage records the avatar or header (prop) of a remote actor as a remote media file
// of the actor, which the media proxy serves (federation.md §9).
func (a *App) recordRemoteImage(ctx context.Context, tx uow.Tx, accountID, url string, prop domainaccount.ProfileProperty) (string, error) {
	purpose := domaincontent.PurposeAvatar
	if prop == domainaccount.PropertyHeader {
		purpose = domaincontent.PurposeHeader
	}
	m, err := a.Media.RecordRemoteMedia(ctx, tx, content.RemoteMediaInput{
		OwnerAccountID: accountID, RemoteURL: url, DeclaredMediaType: domaincontent.MediaTypeImage, Purpose: purpose,
	})
	return m.MediaID, err
}

// recordRemoteEmojis records the custom emoji of a remote profile for the account module
// (ADR-0051 §3) through the content module.
func (a *App) recordRemoteEmojis(ctx context.Context, tx uow.Tx, accountID, domain string, emojis []account.RemoteEmojiInput) ([]string, error) {
	in := make([]content.RemoteEmojiInput, 0, len(emojis))
	for _, e := range emojis {
		in = append(in, content.RemoteEmojiInput{URI: e.URI, Shortcode: e.Shortcode, ImageURL: e.ImageURL})
	}
	return a.Content.RecordRemoteEmojis(ctx, tx, accountID, domain, in)
}

// setUpRemoteActors builds the fetching of remote actors (federation.md §6): GETs signed with
// the instance actor's key, WebFinger checks of handles, key rotation, the public key resolver
// of inbound signatures, and the refresh and NodeInfo jobs. It needs the HTTPClient.
func (a *App) setUpRemoteActors(cfg *config.Config, repo federationRepository) {
	outbox := a.Outbox
	if outbox == nil {
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	fetcher := federation.NewFetcher(federation.FetcherDeps{
		Client: federation.NewSignedHTTPClient(a.HTTPClient, httpsig.New(), a.KV, a.Clock),
		KV:     a.KV, Clock: a.Clock,
		Domains: a.ModerationPolicy, Instances: repo,
		Key: federation.InstanceKey(a.Accounts), Host: cfg.Host,
		// The federation mode decides what is fetched (ADR-0006).
		Modes: federationModes{a},
	})
	a.RemoteActors = federation.NewRemoteActors(federation.RemoteActorsDeps{
		UnitOfWork: a.UnitOfWork, Accounts: a.Accounts, Instances: repo, Fetcher: fetcher, HTTP: a.HTTPClient,
		Jobs: a.Jobs, Outbox: outbox, Followers: a.Graph, Clock: a.Clock, IDs: a.IDs, Logger: a.Logger,
		Review: federation.ActorReview{Gate: a.Hooks, Reports: a.Moderation},
	})
	a.RemoteActors.RegisterJobs(a.Handlers, a.Scheduler)
	a.Hooks.RegisterOverrideEffect(apphook.InboundActor, federation.RefetchDroppedProfile(outbox))
}

// remoteMentions resolves the remote handles of local posts for the content module: handles
// through RemoteActors, profile pages through the account module. It reads a.RemoteActors per
// call, since the content module is built before setUpRemoteActors.
type remoteMentions struct{ a *App }

func (m remoteMentions) ResolveHandle(ctx context.Context, username, domain string) (account.Actor, error) {
	return m.a.RemoteActors.ResolveHandle(ctx, username, domain)
}

func (m remoteMentions) RemoteProfileURL(ctx context.Context, accountID string) (string, error) {
	return m.a.Accounts.RemoteProfileURL(ctx, accountID)
}

// RemoteProfileURLs reads the profile pages of many remote accounts at once.
func (m remoteMentions) RemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	return m.a.Accounts.RemoteProfileURLs(ctx, accountIDs)
}
