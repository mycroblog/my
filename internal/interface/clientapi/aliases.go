// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
)

// The aliases of the viewer (ADR-0089, FR-009): the accounts the viewer moves from, which the
// viewer's actor names in alsoKnownAs.

const (
	// ProblemTypeInvalidAlias is the problem of an alias that leads to no account's actor or to
	// the viewer.
	ProblemTypeInvalidAlias = "/api/v1/problems/invalid-alias"
	// ProblemTypeTooManyAliases is the problem of an alias beyond account.MaxAliases.
	ProblemTypeTooManyAliases = "/api/v1/problems/too-many-aliases"
)

// AccountAliases reads and changes the aliases of local accounts; *appaccount.Service
// implements it.
type AccountAliases interface {
	Aliases(ctx context.Context, accountID string) ([]account.Alias, error)
	// AddAlias fails with account.ErrSelfAlias for the account itself, appaccount.ErrNotFound
	// for an alias that is not a usable actor, and account.ErrTooManyAliases beyond the limit.
	AddAlias(ctx context.Context, accountID string, alias appaccount.Actor) (account.Alias, bool, error)
	RemoveAlias(ctx context.Context, accountID, uri string) (bool, error)
}

// ListAccountAliases serves GET /api/v1/accounts/aliases.
func (s *Server) ListAccountAliases(ctx context.Context, _ ListAccountAliasesRequestObject) (ListAccountAliasesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Aliases == nil {
		return ListAccountAliasesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	list, err := s.aliasList(ctx, v.AccountID)
	if err != nil {
		return nil, err
	}
	return ListAccountAliases200JSONResponse{list}, nil
}

// AddAccountAlias serves POST /api/v1/accounts/aliases.
func (s *Server) AddAccountAlias(ctx context.Context, req AddAccountAliasRequestObject) (AddAccountAliasResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Aliases == nil {
		return AddAccountAliasdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	l := s.Localizer(ctx)
	invalid := AddAccountAliasdefaultApplicationProblemPlusJSONResponse(
		s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidAlias, l.ServerErrorsAliasesInvalid(), ""))
	if req.Body == nil {
		return invalid, nil
	}
	acct := strings.TrimSpace(req.Body.Acct)
	alias, found, err := s.lookup(ctx, acct)
	if err != nil {
		return nil, err
	}
	if !found && s.o.Resolver != nil && s.resolvable(acct) {
		// A mode that takes nothing in finds only the known accounts (ADR-0076).
		if ok, err := s.federationAllows(ctx, remoteAccountFlows...); err != nil || !ok {
			if err != nil {
				return nil, err
			}
			return invalid, nil
		}
		if p, h, limited := s.LimitProblem(ctx, ResolveLimit, v.AccountID); limited {
			return AddAccountAliasdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
		}
		if alias, found, err = s.resolve(ctx, acct); err != nil {
			return nil, err
		}
	}
	if !found {
		return invalid, nil
	}
	_, _, err = s.o.Aliases.AddAlias(ctx, v.AccountID, alias)
	switch {
	case errors.Is(err, account.ErrSelfAlias), errors.Is(err, appaccount.ErrNotFound):
		return invalid, nil
	case errors.Is(err, account.ErrTooManyAliases):
		return AddAccountAliasdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTooManyAliases, l.ServerErrorsAliasesTooMany(), "")), nil
	case err != nil:
		return nil, err
	}
	list, err := s.aliasList(ctx, v.AccountID)
	if err != nil {
		return nil, err
	}
	return AddAccountAlias200JSONResponse{list}, nil
}

// RemoveAccountAlias serves POST /api/v1/accounts/aliases/remove.
func (s *Server) RemoveAccountAlias(ctx context.Context, req RemoveAccountAliasRequestObject) (RemoveAccountAliasResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Aliases == nil {
		return RemoveAccountAliasdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body != nil {
		if _, err := s.o.Aliases.RemoveAlias(ctx, v.AccountID, req.Body.Uri); err != nil {
			return nil, err
		}
	}
	list, err := s.aliasList(ctx, v.AccountID)
	if err != nil {
		return nil, err
	}
	return RemoveAccountAlias200JSONResponse{list}, nil
}

// aliasList presents the aliases of the account accountID, each with the handle of its account
// when the instance knows it.
func (s *Server) aliasList(ctx context.Context, accountID string) (AccountAliasListJSONResponse, error) {
	aliases, err := s.o.Aliases.Aliases(ctx, accountID)
	if err != nil {
		return AccountAliasListJSONResponse{}, err
	}
	list := AccountAliasList{Aliases: make([]AccountAlias, len(aliases))}
	for i, al := range aliases {
		list.Aliases[i] = AccountAlias{Uri: al.URI, CreatedAt: al.AddedAt}
		a, err := s.o.Accounts.FindByURI(ctx, al.URI)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
			continue
		case err != nil:
			return AccountAliasListJSONResponse{}, err
		}
		if h := a.Handle; h != nil {
			acct := h.Username
			if h.Domain != account.LocalDomain {
				acct += "@" + h.Domain
			}
			list.Aliases[i].Acct = &acct
		}
	}
	cache := cacheNoStore
	return AccountAliasListJSONResponse{Body: list, Headers: AccountAliasListResponseHeaders{CacheControl: &cache}}, nil
}
