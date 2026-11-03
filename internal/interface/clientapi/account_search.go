// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"

	appaccount "github.com/mycroblog/my/internal/app/account"
)

// AccountSearch is the prefix search of accounts (ADR-0092 §1). *appaccount.PrefixSearch
// implements it.
type AccountSearch interface {
	// Search returns the IDs of up to limit accounts whose search term starts with q, best match
	// first, failing with appaccount.ErrInvalidSearchQuery for a q that is empty or too long.
	Search(ctx context.Context, viewerID, q string, limit int) ([]string, error)
}

// cacheSearch is the Cache-Control of the prefix searches: the results depend on the viewer, and
// the client may reuse them while the user types the same prefix again (ADR-0092).
const cacheSearch = "private, max-age=30"

// SearchAccounts serves GET /api/v1/accounts/search: the accounts whose username,
// username@domain, or display name word starts with q (FR-119, FR-306).
func (s *Server) SearchAccounts(ctx context.Context, req SearchAccountsRequestObject) (SearchAccountsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	limit := appaccount.DefaultSearchLimit
	if l := req.Params.Limit; l != nil {
		if *l < 1 || *l > appaccount.MaxSearchLimit {
			return SearchAccountsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "limit")), nil
		}
		limit = *l
	}
	if _, err := appaccount.NormalizeSearchQuery(req.Params.Q); err != nil {
		return SearchAccountsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "q")), nil
	}
	var ids []string
	if s.o.AccountSearch != nil {
		ids, err = s.o.AccountSearch.Search(ctx, v.AccountID, req.Params.Q, limit)
		if errors.Is(err, appaccount.ErrInvalidSearchQuery) {
			return SearchAccountsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "q")), nil
		}
		if err != nil {
			return nil, err
		}
	}
	accounts, err := s.presentAccounts(ctx, &v, ids, true)
	if err != nil {
		return nil, err
	}
	if accounts == nil {
		accounts = []Account{}
	}
	cache := cacheSearch
	return SearchAccounts200JSONResponse{Body: AccountList{Items: accounts}, Headers: SearchAccounts200ResponseHeaders{CacheControl: &cache}}, nil
}
