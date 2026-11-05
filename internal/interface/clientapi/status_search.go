// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
)

const (
	// ProblemTypeSearchUnavailable is the 404 of a search on a server with search = none, whose
	// clients learn it from GET /instance too (ADR-0149 §9).
	ProblemTypeSearchUnavailable = "/api/v1/problems/search-unavailable"
	// ProblemTypeSearchFailed is the 503 of a search that the backend could not answer.
	ProblemTypeSearchFailed = "/api/v1/problems/search-failed"

	// cacheStatusSearch is the Cache-Control of the full-text search: a query is the user's own, and
	// the results must not outlive what the visibility policy says now.
	cacheStatusSearch = "private, no-store"

	// maxSearchQueryLength is the longest q in characters, as the spec says.
	maxSearchQueryLength = 200
	// maxSearchCursorLength is the longest cursor in bytes: the maxLength of the schema Cursor. The
	// cursor is the backend's, so it cannot be checked as an ID.
	maxSearchCursorLength = 256
)

// StatusSearchLimit bounds the full-text searches of one account, in addition to AccountLimit: each one
// costs the search engine and a read of every result (ADR-0149 §9). The value is provisional until
// a first deployment shows what a search costs.
var StatusSearchLimit = kernel.Limit{Name: "api.search", Max: 30, Window: 5 * time.Minute}

// SearchStatuses serves GET /api/v1/statuses/search, the full-text search of statuses (FR-309). The
// query reaches no log, trace, or error report: this handler logs nothing that holds it, and
// search.Service returns no error that does.
func (s *Server) SearchStatuses(ctx context.Context, req SearchStatusesRequestObject) (SearchStatusesResponseObject, error) {
	cv, v := timelineViewer(ctx)
	if v == nil {
		return nil, errNoViewer
	}
	if s.o.FullTextSearch == nil {
		return SearchStatusesdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusNotFound, ProblemTypeSearchUnavailable, s.Localizer(ctx).ServerErrorsSearchUnavailable(), "")), nil
	}
	if n := utf8.RuneCountInString(req.Params.Q); n < 1 || n > maxSearchQueryLength {
		return SearchStatusesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "q")), nil
	}
	limit, ok := pageLimit(req.Params.Limit)
	if !ok {
		return SearchStatusesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "limit")), nil
	}
	cursor := ""
	if c := req.Params.Cursor; c != nil {
		if len(*c) < 1 || len(*c) > maxSearchCursorLength {
			return SearchStatusesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
		cursor = *c
	}
	if p, h, limited := s.LimitProblem(ctx, StatusSearchLimit, v.AccountID); limited {
		return SearchStatusesdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	page, err := s.o.FullTextSearch.Search(ctx, cv, search.Query{Text: req.Params.Q, Cursor: cursor, Limit: limit})
	switch {
	case errors.Is(err, search.ErrInvalidCursor):
		return SearchStatusesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
	case errors.Is(err, search.ErrBackendFailed):
		s.o.Logger.ErrorContext(ctx, "full-text search failed", slog.Any("error", err))
		return SearchStatusesdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusServiceUnavailable, ProblemTypeSearchFailed, s.Localizer(ctx).ServerErrorsSearchFailed(), "")), nil
	case err != nil:
		return nil, err
	}
	items := make([]statusItem, 0, len(page.StatusIDs))
	for _, id := range page.StatusIDs {
		items = append(items, statusItem{StatusID: id})
	}
	res, err := s.statusPage(ctx, v, items, page.Next)
	if err != nil {
		return nil, err
	}
	cache := cacheStatusSearch
	res.Headers.CacheControl = &cache
	return SearchStatuses200JSONResponse{res}, nil
}
