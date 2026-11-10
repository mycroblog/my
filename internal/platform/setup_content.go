// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/markdown"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpContent registers the content module's projectors and builds the statuses and their
// interactions. The media use cases, the custom emoji, and the remote actors it reads are built
// later; the dependencies below read them per call.
func (a *App) setUpContent(cfg *config.Config, statuses content.Repository, quotes content.QuoteRepository,
	interactions content.InteractionRepository, medias content.MediaRepository,
) {
	content.RegisterProjectors(a.Projectors, statuses)
	content.RegisterApprovalProjectors(a.Projectors, quotes, a.IDs, a.Clock)
	a.Content = content.NewService(content.Deps{
		UnitOfWork: a.UnitOfWork, Repository: statuses, Accounts: a.Accounts, Relationships: a.Graph,
		Restrictions:   a.AccountPolicy.Content(),
		Media:          medias,
		RemoteMentions: remoteMentions{a}, // reads a.RemoteActors, built by setUpRemoteActors
		Sanitizer:      newHTMLSanitizer(),
		RemoteArticles: newRemoteArticleSanitizer(),
		RemoteMedia:    remoteMediaRecorder{a}, // reads a.Media, built by setMedia
		Quotes:         quotes,
		// Reads a.Interactions, built below.
		ReconsiderRequests: func(ctx context.Context, statusID string) error {
			return a.Interactions.ReconsiderRequests(ctx, statusID)
		},
		DefaultInteractionPolicy: a.defaultInteractionPolicy,
		MaxStatusLength:          a.Instance.MaxStatusLength,
		RequireMediaDescription:  a.Instance.RequireMediaDescription,
		// A one-person server posts nothing new as local (ADR-0152 §5).
		SingleUser:      a.Instance.SingleUser,
		DefaultLanguage: a.defaultPostLanguage,
		// Reads a.CustomEmojis, built by setMedia.
		LocalEmojis: func(ctx context.Context, texts ...string) ([]string, error) {
			return a.CustomEmojis.LocalEmojiIDs(ctx, texts...)
		},
		// The decision hook local.status (ADR-0147), built by setUpHooks before this; a flag files
		// its report on the moderation module, built by setUpModeration before this.
		StatusReview: content.StatusReview{
			Hooks: a.Hooks,
			MaxAccountAge: func(ctx context.Context) (time.Duration, error) {
				settings, err := a.Instance.Settings(ctx)
				if err != nil {
					return 0, err
				}
				return settings.HookLocalStatusAccountAge(), nil
			},
			Reports: hookReports{a.Moderation}, InstanceActor: a.emojiImageOwner,
		},
		MediaReview: a.mediaReview(),
		// Articles (ADR-0160): their Markdown, the author's images, which read a.Media, built by
		// setMedia, and the instance settings of who writes them and how long they are.
		ArticleRenderer: markdown.NewArticleRenderer(),
		ArticleImage: func(ctx context.Context, accountID, mediaID string) (content.ArticleImageSource, bool, error) {
			return a.Media.ArticleImage(ctx, accountID, mediaID)
		},
		ArticleLimits:    a.articleLimits,
		ArticleLimitsTx:  a.articleLimitsTx,
		MayWriteArticles: a.mayWriteArticles,
		Clock:            a.Clock, IDs: a.IDs, Host: cfg.Host, AccountDomain: cfg.AccountDomain,
	})
	a.Hooks.RegisterOverrideEffect(hook.MediaCheck, a.Content.UndoMediaReview)
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
		Quotes: quotes, DomainMutes: a.Graph, RemoteArticles: newRemoteArticleSanitizer(),
	})
}

// setUpPreviews builds the link previews of profiles and statuses and the Atom feeds (FR-213).
// They read statuses and their images, so they come after the media use cases, and the feeds
// list their posts from the timelines.
func (a *App) setUpPreviews(statuses content.Repository) {
	a.Previews = content.NewPreviews(content.PreviewDeps{
		Repository: statuses, Accounts: a.Accounts, Restrictions: a.AccountPolicy.Content(), Media: a.Media,
		DefaultLanguage: a.defaultPostLanguage, Moved: a.moved,
	})
	page := func(p timeline.Page, err error) ([]string, string, error) {
		if err != nil {
			return nil, "", err
		}
		ids := make([]string, 0, len(p.Entries))
		for _, e := range p.Entries {
			ids = append(ids, e.StatusID)
		}
		return ids, p.Next, nil
	}
	a.Feeds = content.NewFeeds(content.FeedDeps{
		Previews: a.Previews, Repository: statuses,
		ProfilePosts: func(ctx context.Context, accountID, before string, limit int) ([]string, string, error) {
			return page(a.Timeline.ProfileFeed(ctx, accountID, timeline.Query{Before: before, Limit: limit}))
		},
		TagPosts: func(ctx context.Context, tagID, before string, limit int) ([]string, string, error) {
			return page(a.Timeline.HashtagFeed(ctx, tagID, timeline.Query{Before: before, Limit: limit}))
		},
	})
}

// moved reports whether an account moved to another one (ADR-0089).
func (a *App) moved(ctx context.Context, accountID string) (bool, error) {
	_, err := a.Accounts.MoveState(ctx, accountID)
	if errors.Is(err, account.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// articleLimits returns the instance settings that bound an article (ADR-0160 §5).
func (a *App) articleLimits(ctx context.Context) (content.ArticleLimits, error) {
	s, err := a.Instance.Settings(ctx)
	if err != nil {
		return content.ArticleLimits{}, err
	}
	return content.ArticleLimits{MaxBytes: s.MaxArticleBytes(), MaxImages: s.MaxArticleImages()}, nil
}

// articleLimitsTx is articleLimits inside tx.
func (a *App) articleLimitsTx(ctx context.Context, tx uow.Tx) (content.ArticleLimits, error) {
	s, err := a.Instance.SettingsTx(ctx, tx)
	if err != nil {
		return content.ArticleLimits{}, err
	}
	return content.ArticleLimits{MaxBytes: s.MaxArticleBytes(), MaxImages: s.MaxArticleImages()}, nil
}

// mayWriteArticles reports whether a local user may write new articles (ADR-0160 §3): everyone
// while article_authoring is everyone, the holders of create_articles otherwise.
func (a *App) mayWriteArticles(ctx context.Context, userID string) (bool, error) {
	s, err := a.Instance.Settings(ctx)
	if err != nil {
		return false, err
	}
	if s.ArticleAuthoring() == instance.ArticleAuthoringEveryone {
		return true, nil
	}
	return a.Identity.HasPermission(ctx, userID, domainidentity.PermCreateArticles)
}

// defaultInteractionPolicy returns who may reply to, boost, and quote the new statuses of a user
// (ADR-0155 §5): the defaults of their preferences.
func (a *App) defaultInteractionPolicy(ctx context.Context, userID string) (content.InteractionSettings, error) {
	p, err := a.Identity.Preferences(ctx, userID)
	if errors.Is(err, identity.ErrNotFound) {
		return content.DefaultInteractionSettings, nil
	}
	if err != nil {
		return content.InteractionSettings{}, err
	}
	setting := func(s domainidentity.InteractionSetting) content.InteractionSetting {
		return content.InteractionSetting{Automatic: content.InteractionAudience(s.Automatic), Manual: content.InteractionAudience(s.Manual)}
	}
	d := p.DefaultInteractionPolicy
	return content.InteractionSettings{Reply: setting(d.Reply), Announce: setting(d.Announce), Quote: setting(d.Quote)}, nil
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
