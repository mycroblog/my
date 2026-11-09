// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"maps"

	appaccount "github.com/mycroblog/my/internal/app/account"
)

// AccountSuggester suggests accounts to follow (FR-308). *appaccount.Suggestions implements it.
type AccountSuggester interface {
	// Suggest returns up to limit accounts to suggest to viewerID, featured accounts first.
	Suggest(ctx context.Context, viewerID string, limit int) ([]appaccount.Suggestion, error)
}

// ListAccountSuggestions serves GET /api/v1/accounts/suggestions: the featured accounts, then
// the recently active local accounts, that the viewer does not follow (FR-308).
func (s *Server) ListAccountSuggestions(ctx context.Context, req ListAccountSuggestionsRequestObject) (ListAccountSuggestionsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	limit := appaccount.DefaultSuggestionLimit
	if l := req.Params.Limit; l != nil {
		if *l < 1 || *l > appaccount.MaxSuggestionLimit {
			return ListAccountSuggestionsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "limit")), nil
		}
		limit = *l
	}
	cache := cacheNoStore
	out := []AccountSuggestion{}
	if s.o.AccountSuggestions == nil {
		return ListAccountSuggestions200JSONResponse{Body: AccountSuggestionList{Items: out}, Headers: ListAccountSuggestions200ResponseHeaders{CacheControl: &cache}}, nil
	}
	suggestions, err := s.o.AccountSuggestions.Suggest(ctx, v.AccountID, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(suggestions))
	sources := make(map[string]appaccount.SuggestionSource, len(suggestions))
	found := make(map[string]appaccount.Actor, len(suggestions))
	var unread []string
	for i, sg := range suggestions {
		ids[i] = sg.AccountID
		sources[sg.AccountID] = sg.Source
		if sg.Actor.Account.ID != "" {
			found[sg.AccountID] = sg.Actor
		} else {
			unread = append(unread, sg.AccountID)
		}
	}
	if len(unread) > 0 {
		read, err := s.o.Accounts.FindByIDs(ctx, unread)
		if err != nil {
			return nil, err
		}
		maps.Copy(found, read)
	}
	// Suggest read the accounts (a Suggestions without them leaves them to read); only moderation and the blocks of the viewer are left to check.
	shown, err := s.presentableOf(ctx, &v, found, ids, true)
	if err != nil {
		return nil, err
	}
	var actors []appaccount.Actor
	for _, id := range ids {
		if a, ok := shown[id]; ok {
			actors = append(actors, a)
		}
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		out = append(out, AccountSuggestion{Source: AccountSuggestionSource(sources[a.Id]), Account: a})
	}
	return ListAccountSuggestions200JSONResponse{Body: AccountSuggestionList{Items: out}, Headers: ListAccountSuggestions200ResponseHeaders{CacheControl: &cache}}, nil
}
