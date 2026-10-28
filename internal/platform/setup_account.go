// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
	domaincontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpAccount registers the account module's projectors and builds its service. It comes first
// of the modules: the others read the accounts.
func (a *App) setUpAccount(cfg *config.Config, repo account.Repository) {
	account.RegisterProjectors(a.Projectors, repo)
	a.setUpAccountSearch(repo)
	a.Accounts = account.NewService(account.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs, Sealer: a.Sealer, Host: cfg.Host,
		// Profiles are rendered and their images checked by the content module, built below; the
		// account module does not import it, since the content module imports the account module.
		RenderProfileText: func(ctx context.Context, text string) (string, error) {
			return a.Content.RenderProfileText(ctx, text)
		},
		CheckProfileImage: func(ctx context.Context, accountID, mediaID string, prop domainaccount.ProfileProperty) error {
			purpose := domaincontent.PurposeAvatar
			if prop == domainaccount.PropertyHeader {
				purpose = domaincontent.PurposeHeader
			}
			return a.Media.CheckProfileImage(ctx, accountID, mediaID, purpose)
		},
		Sanitizer:          newHTMLSanitizer(),
		RecordRemoteImage:  a.recordRemoteImage,
		RecordRemoteEmojis: a.recordRemoteEmojis,
		// The local custom emoji of profiles, which the content module keeps (ADR-0051 §3).
		LocalEmojis: func(ctx context.Context, texts ...string) ([]string, error) {
			return a.CustomEmojis.LocalEmojiIDs(ctx, texts...)
		},
	})
	a.setUpAccountSuggestions()
}

// setUpAccountSuggestions builds the suggestions of accounts to follow (FR-308): the featured
// accounts of the instance settings, then the local accounts that posted recently. The instance,
// content, and graph modules, which the account module does not import, are built after it;
// the functions read them when they run.
func (a *App) setUpAccountSuggestions() {
	a.AccountSuggestions = account.NewSuggestions(account.SuggestionDeps{
		Accounts: a.Accounts, Clock: a.Clock,
		Featured: func(ctx context.Context) ([]string, error) {
			s, err := a.Instance.Settings(ctx)
			return s.FeaturedAccounts(), err
		},
		RecentPosters: func(ctx context.Context, since time.Time, limit int) ([]string, error) {
			return a.Content.ListRecentLocalPosters(ctx, since, limit)
		},
		Relations: a.searchRelations,
	})
}

// searchRelations returns the relations of a viewer with accounts that the search and the
// suggestions of accounts read from the graph module.
func (a *App) searchRelations(ctx context.Context, viewerID string, ids []string) (map[string]account.SearchRelation, error) {
	rels, err := a.Graph.CandidateRelations(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]account.SearchRelation, len(rels))
	for id, r := range rels {
		out[id] = account.SearchRelation(r)
	}
	return out, nil
}

// setUpAccountSearch registers the projector of the account search terms and builds the prefix
// search of accounts (ADR-0092 §1). The moderation and graph modules, which the account module
// does not import, are built after it; the functions read them when they run.
func (a *App) setUpAccountSearch(repo account.Repository) {
	a.SearchTerms = account.NewSearchTermProjector(repo, func(ctx context.Context, tx uow.Tx, accountID string) (bool, error) {
		return a.AccountPolicy.SuspendedTx(ctx, tx, accountID)
	})
	a.SearchTerms.Register(a.Projectors)
	a.Projectors.Register(moderation.EventAccountSuspensionChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return a.SearchTerms.Refresh(ctx, tx, e.(moderation.AccountSuspensionChanged).AccountID)
	})
	a.AccountSearch = account.NewPrefixSearch(account.SearchDeps{
		Repository: repo,
		Relations:  a.searchRelations,
		DomainSuspended: func(ctx context.Context, domain string) (bool, error) {
			return a.ModerationPolicy.FederationSuspended(ctx, domain)
		},
	})
}
