// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"

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
		return ListAccountSuggestions200JSONResponse{Body: out, Headers: ListAccountSuggestions200ResponseHeaders{CacheControl: &cache}}, nil
	}
	suggestions, err := s.o.AccountSuggestions.Suggest(ctx, v.AccountID, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(suggestions))
	sources := make(map[string]appaccount.SuggestionSource, len(suggestions))
	for i, sg := range suggestions {
		ids[i] = sg.AccountID
		sources[sg.AccountID] = sg.Source
	}
	accounts, err := s.presentAccounts(ctx, &v, ids, true)
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		out = append(out, AccountSuggestion{Source: AccountSuggestionSource(sources[a.Id]), Account: a})
	}
	return ListAccountSuggestions200JSONResponse{Body: out, Headers: ListAccountSuggestions200ResponseHeaders{CacheControl: &cache}}, nil
}
