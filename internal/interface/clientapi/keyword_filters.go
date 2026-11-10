// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// Problem types of keyword filters (ADR-0157 §9).
const (
	ProblemTypeTooManyKeywordFilters = "/api/v1/problems/too-many-keyword-filters"
	ProblemTypeTooManyFilterKeywords = "/api/v1/problems/too-many-filter-keywords"
)

// KeywordFilters keeps the viewer's keyword filters (*apptimeline.Service, ADR-0157). Every
// method fails with apptimeline.ErrNotFound for a filter that the viewer does not have, which the
// handlers answer as a filter that does not exist (404).
type KeywordFilters interface {
	KeywordFilters(ctx context.Context, accountID string) ([]apptimeline.OwnedKeywordFilter, error)
	KeywordFilter(ctx context.Context, accountID, id string) (apptimeline.OwnedKeywordFilter, error)
	CreateKeywordFilter(ctx context.Context, accountID string, in apptimeline.KeywordFilterInput) (apptimeline.OwnedKeywordFilter, error)
	UpdateKeywordFilter(ctx context.Context, accountID, id string, in apptimeline.KeywordFilterInput) (apptimeline.OwnedKeywordFilter, error)
	DeleteKeywordFilter(ctx context.Context, accountID, id string) error
}

func newKeywordFilter(f apptimeline.OwnedKeywordFilter) KeywordFilter {
	out := KeywordFilter{
		Id: f.ID, Title: f.Title, Action: KeywordFilterAction(f.Action), Expired: f.Expired, CreatedAt: f.CreatedAt,
		Contexts: make([]KeywordFilterContext, len(f.Contexts)), Keywords: make([]KeywordFilterKeyword, len(f.Words)),
	}
	for i, c := range f.Contexts {
		out.Contexts[i] = KeywordFilterContext(c)
	}
	for i, w := range f.Words {
		out.Keywords[i] = KeywordFilterKeyword{Keyword: w.Word, WholeWord: w.WholeWord}
	}
	if !f.ExpiresAt.IsZero() {
		at := f.ExpiresAt.UTC()
		out.ExpiresAt = &at
	}
	return out
}

func keywordFilterResponse(f apptimeline.OwnedKeywordFilter) KeywordFilterJSONResponse {
	cache := cacheNoStore
	return KeywordFilterJSONResponse{Body: newKeywordFilter(f), Headers: KeywordFilterResponseHeaders{CacheControl: &cache}}
}

// keywordFilterInput maps the body of a create or a change; a missing body is an empty filter,
// which fails validation.
func keywordFilterInput(body *KeywordFilterRequest) apptimeline.KeywordFilterInput {
	var in apptimeline.KeywordFilterInput
	if body == nil {
		return in
	}
	in.Title, in.Action = body.Title, timeline.FilterAction(body.Action)
	for _, c := range body.Contexts {
		in.Contexts = append(in.Contexts, timeline.FilterContext(c))
	}
	for _, k := range body.Keywords {
		in.Words = append(in.Words, apptimeline.KeywordInput{Word: k.Keyword, WholeWord: k.WholeWord})
	}
	if body.ExpiresAt != nil {
		in.ExpiresAt = body.ExpiresAt.UTC()
	}
	return in
}

// keywordFilterProblem returns the problem that answers err of the keyword filters, false for an
// error that is none of theirs.
func (s *Server) keywordFilterProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	invalid := func(field string) problem {
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(field))
	}
	switch {
	case errors.Is(err, apptimeline.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, timeline.ErrInvalidFilterTitle):
		return invalid("title"), true
	case errors.Is(err, timeline.ErrInvalidFilterAction):
		return invalid("action"), true
	case errors.Is(err, timeline.ErrInvalidFilterContext):
		return invalid("contexts"), true
	case errors.Is(err, timeline.ErrInvalidFilterWord), errors.Is(err, timeline.ErrNoFilterWords):
		return invalid("keywords"), true
	case errors.Is(err, timeline.ErrTooManyKeywordFilters):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTooManyKeywordFilters, l.ServerErrorsKeywordFiltersTooMany(), ""), true
	case errors.Is(err, timeline.ErrTooManyFilterWords):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTooManyFilterKeywords, l.ServerErrorsKeywordFiltersTooManyWords(), ""), true
	}
	return problem{}, false
}

// keywordFiltersViewer returns the viewer of a request of the keyword filters, or the 404 that
// answers it when the server keeps none.
func (s *Server) keywordFiltersViewer(ctx context.Context) (Viewer, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return Viewer{}, nil, err
	}
	if s.o.KeywordFilters == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	return v, nil, nil
}

// ListKeywordFilters serves GET /api/v1/filters.
func (s *Server) ListKeywordFilters(ctx context.Context, _ ListKeywordFiltersRequestObject) (ListKeywordFiltersResponseObject, error) {
	v, p, err := s.keywordFiltersViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return ListKeywordFiltersdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	fs, err := s.o.KeywordFilters.KeywordFilters(ctx, v.AccountID)
	if err != nil {
		return nil, err
	}
	items := make([]KeywordFilter, len(fs))
	for i, f := range fs {
		items[i] = newKeywordFilter(f)
	}
	cache := cacheNoStore
	return ListKeywordFilters200JSONResponse{KeywordFilterCollectionJSONResponse{
		Body: KeywordFilterCollection{Items: items}, Headers: KeywordFilterCollectionResponseHeaders{CacheControl: &cache},
	}}, nil
}

// CreateKeywordFilter serves POST /api/v1/filters.
func (s *Server) CreateKeywordFilter(ctx context.Context, req CreateKeywordFilterRequestObject) (CreateKeywordFilterResponseObject, error) {
	v, p, err := s.keywordFiltersViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return CreateKeywordFilterdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	f, err := s.o.KeywordFilters.CreateKeywordFilter(ctx, v.AccountID, keywordFilterInput(req.Body))
	if err != nil {
		if p, ok := s.keywordFilterProblem(ctx, err); ok {
			return CreateKeywordFilterdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return CreateKeywordFilter201JSONResponse{keywordFilterResponse(f)}, nil
}

// GetKeywordFilter serves GET /api/v1/filters/{id}.
func (s *Server) GetKeywordFilter(ctx context.Context, req GetKeywordFilterRequestObject) (GetKeywordFilterResponseObject, error) {
	v, p, err := s.keywordFiltersViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return GetKeywordFilterdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	f, err := s.o.KeywordFilters.KeywordFilter(ctx, v.AccountID, req.Id)
	if err != nil {
		if p, ok := s.keywordFilterProblem(ctx, err); ok {
			return GetKeywordFilterdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return GetKeywordFilter200JSONResponse{keywordFilterResponse(f)}, nil
}

// UpdateKeywordFilter serves PATCH /api/v1/filters/{id}.
func (s *Server) UpdateKeywordFilter(ctx context.Context, req UpdateKeywordFilterRequestObject) (UpdateKeywordFilterResponseObject, error) {
	v, p, err := s.keywordFiltersViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return UpdateKeywordFilterdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	f, err := s.o.KeywordFilters.UpdateKeywordFilter(ctx, v.AccountID, req.Id, keywordFilterInput(req.Body))
	if err != nil {
		if p, ok := s.keywordFilterProblem(ctx, err); ok {
			return UpdateKeywordFilterdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return UpdateKeywordFilter200JSONResponse{keywordFilterResponse(f)}, nil
}

// DeleteKeywordFilter serves DELETE /api/v1/filters/{id}.
func (s *Server) DeleteKeywordFilter(ctx context.Context, req DeleteKeywordFilterRequestObject) (DeleteKeywordFilterResponseObject, error) {
	v, p, err := s.keywordFiltersViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return DeleteKeywordFilterdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err := s.o.KeywordFilters.DeleteKeywordFilter(ctx, v.AccountID, req.Id); err != nil {
		if p, ok := s.keywordFilterProblem(ctx, err); ok {
			return DeleteKeywordFilterdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return DeleteKeywordFilter204Response{}, nil
}

// StatusFilters builds the keyword filters that judge the statuses a read shows to its viewers
// (*apptimeline.Service, ADR-0157 §7).
type StatusFilters interface {
	StatusFilter(ctx context.Context, viewer appcontent.Viewer, c timeline.FilterContext) (*apptimeline.StatusFilter, error)
	StatusFiltersOf(ctx context.Context, accountIDs []string, c timeline.FilterContext) (map[string]*apptimeline.StatusFilter, error)
}

// noFilter is the filter context of the reads that keyword filters do not apply to: the
// full-text search, bookmarks, favourites, the direct messages, the responses of commands, and
// what moderators read (ADR-0157 §6).
const noFilter timeline.FilterContext = ""

// statusFilter returns the keyword filter of viewer in c; nil, which judges nothing, for
// noFilter, a visitor, a viewer without filters there, or a server without keyword filters.
func (s *Server) statusFilter(ctx context.Context, viewer appcontent.Viewer, c timeline.FilterContext) (*apptimeline.StatusFilter, error) {
	if c == noFilter || s.o.StatusFilters == nil {
		return nil, nil
	}
	return s.o.StatusFilters.StatusFilter(ctx, viewer, c)
}

// newFilterResults presents the matches of a status that its Status folds behind.
func newFilterResults(ms []timeline.FilterMatch) []FilterResult {
	out := make([]FilterResult, len(ms))
	for i, m := range ms {
		out[i] = FilterResult{Filter: MatchedKeywordFilter{Id: m.Filter.ID, Title: m.Filter.Title}, KeywordMatches: m.Words}
	}
	return out
}

// applyJudgement marks st, the Status of a row, with what the keyword filters made of its status,
// and reports false when the row is left out: a match of a hide filter, unless opened, which
// folds the status as for a warn filter (ADR-0157 §6). The quoted status carries its own marks,
// and a quote never leaves out the row (§3).
func applyJudgement(st *Status, j apptimeline.Judgement, opened bool) bool {
	if j.Hide && !opened {
		return false
	}
	if len(j.Matches) > 0 {
		st.Filtered = newFilterResults(j.Matches)
	}
	quoted := st
	if st.Boost != nil {
		quoted = st.Boost
	}
	if len(j.Quote) > 0 && quoted.Quote != nil && quoted.Quote.QuotedStatus != nil {
		// Copies, since the rows of one status share its quote.
		q, qs := *quoted.Quote, *quoted.Quote.QuotedStatus
		qs.Filtered = newFilterResults(j.Quote)
		q.QuotedStatus = &qs
		quoted.Quote = &q
	}
	return true
}

// filteredStatuses presents views in order as viewer sees them in c, leaving out those that a
// hide filter of the viewer matches.
func (s *Server) filteredStatuses(ctx context.Context, viewer appcontent.Viewer, c timeline.FilterContext, views []appcontent.StatusView) ([]Status, error) {
	f, err := s.statusFilter(ctx, viewer, c)
	if err != nil {
		return nil, err
	}
	sts, err := s.presentStatuses(ctx, views)
	if err != nil {
		return nil, err
	}
	out := sts[:0]
	for i, v := range views {
		if applyJudgement(&sts[i], f.Judge(v, ""), false) {
			out = append(out, sts[i])
		}
	}
	return out, nil
}
