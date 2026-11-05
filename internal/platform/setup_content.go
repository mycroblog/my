// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpContent registers the content module's projectors and builds the statuses and their
// interactions. The media use cases, the custom emoji, and the remote actors it reads are built
// later; the dependencies below read them per call.
func (a *App) setUpContent(cfg *config.Config, statuses content.Repository, quotes content.QuoteRepository,
	interactions content.InteractionRepository, medias content.MediaRepository,
) {
	content.RegisterProjectors(a.Projectors, statuses)
	content.RegisterQuoteProjectors(a.Projectors, quotes)
	a.Content = content.NewService(content.Deps{
		UnitOfWork: a.UnitOfWork, Repository: statuses, Accounts: a.Accounts, Relationships: a.Graph,
		Restrictions:            a.AccountPolicy.Content(),
		Media:                   medias,
		RemoteMentions:          remoteMentions{a}, // reads a.RemoteActors, built by setUpRemoteActors
		Sanitizer:               newHTMLSanitizer(),
		RemoteMedia:             remoteMediaRecorder{a}, // reads a.Media, built by setMedia
		Quotes:                  quotes,
		DefaultQuotePolicy:      a.defaultQuotePolicy,
		MaxStatusLength:         a.Instance.MaxStatusLength,
		RequireMediaDescription: a.Instance.RequireMediaDescription,
		DefaultLanguage:         a.defaultPostLanguage,
		// Reads a.CustomEmojis, built by setMedia.
		LocalEmojis: func(ctx context.Context, texts ...string) ([]string, error) {
			return a.CustomEmojis.LocalEmojiIDs(ctx, texts...)
		},
		// The decision hook local.status (ADR-0147), built by setUpHooks before this.
		StatusReview: content.StatusReview{
			Hooks: a.Hooks,
			MaxAccountAge: func(ctx context.Context) (time.Duration, error) {
				settings, err := a.Instance.Settings(ctx)
				if err != nil {
					return 0, err
				}
				return settings.HookLocalStatusAccountAge(), nil
			},
		},
		Clock: a.Clock, IDs: a.IDs, Host: cfg.Host, AccountDomain: cfg.AccountDomain,
	})
	// After RegisterProjectors, which creates the counters that these projectors update.
	content.RegisterInteractionProjectors(a.Projectors, interactions)
	a.Interactions = content.NewInteractions(content.InteractionDeps{
		UnitOfWork: a.UnitOfWork, Statuses: statuses, Interactions: interactions,
		Accounts: a.Accounts, Relationships: a.Graph, Restrictions: a.AccountPolicy.Content(),
		RecordRemoteEmojis: a.Content.RecordRemoteEmojis,
		Quotes:             quotes,
		// Reads a.Media, built by setMedia.
		EmojiTags: func(ctx context.Context, ids []string) ([]content.EmojiTag, error) {
			return a.Media.EmojiTags(ctx, ids)
		},
		Clock: a.Clock, IDs: a.IDs, Host: cfg.Host,
	})
}

// setUpStatusReader builds what presents statuses to a viewer. It comes after the media use
// cases and the link cards, which it reads.
func (a *App) setUpStatusReader(statuses content.Repository, interactions content.InteractionRepository, quotes content.QuoteRepository) {
	a.Statuses = content.NewReader(content.ReaderDeps{
		Statuses: statuses, Interactions: interactions, Accounts: a.Accounts, Relationships: a.Graph,
		Restrictions: a.AccountPolicy.Content(), Media: a.Media, Clock: a.Clock, LinkCards: a.LinkCards,
		Quotes: quotes, DomainMutes: a.Graph,
	})
}

// setUpPreviews builds the link previews of profiles and statuses. They read statuses and their
// images, so they come after the media use cases.
func (a *App) setUpPreviews(statuses content.Repository) {
	a.Previews = content.NewPreviews(content.PreviewDeps{
		Repository: statuses, Accounts: a.Accounts, Restrictions: a.AccountPolicy.Content(), Media: a.Media,
		DefaultLanguage: a.defaultPostLanguage,
	})
}

// defaultQuotePolicy returns who may quote the new statuses of a user (ADR-0069 §1): the
// default of their preferences.
func (a *App) defaultQuotePolicy(ctx context.Context, userID string) (content.QuotePolicy, error) {
	p, err := a.Identity.Preferences(ctx, userID)
	if errors.Is(err, identity.ErrNotFound) {
		return content.QuotePolicyPublic, nil
	}
	if err != nil {
		return "", err
	}
	return content.QuotePolicy(p.DefaultQuotePolicy), nil
}

// defaultPostLanguage returns the default post language of a user's preferences, or "" for a
// user without preferences.
func (a *App) defaultPostLanguage(ctx context.Context, userID string) (string, error) {
	p, err := a.Identity.Preferences(ctx, userID)
	if errors.Is(err, identity.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return p.DefaultPostLanguage, nil
}
