// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"

	apptimeline "github.com/mycroblog/my/internal/app/timeline"
)

// TagSearch is the prefix search of hashtags (ADR-0092 §1). *apptimeline.TagSearch implements
// it.
type TagSearch interface {
	// Search returns up to limit hashtags whose normalized name starts with q, best match first,
	// failing with apptimeline.ErrInvalidTagSearchQuery for a q that is empty or too long.
	Search(ctx context.Context, q string, limit int) ([]apptimeline.TagSuggestion, error)
}

// SearchTags serves GET /api/v1/tags/search: the hashtags that public, local, and unlisted
// posts carry and whose name starts with q (FR-119, FR-306).
func (s *Server) SearchTags(ctx context.Context, req SearchTagsRequestObject) (SearchTagsResponseObject, error) {
	if _, err := requireViewer(ctx); err != nil {
		return nil, err
	}
	limit := apptimeline.DefaultTagSearchLimit
	if l := req.Params.Limit; l != nil {
		if *l < 1 || *l > apptimeline.MaxTagSearchLimit {
			return SearchTagsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "limit")), nil
		}
		limit = *l
	}
	if _, err := apptimeline.NormalizeTagSearchQuery(req.Params.Q); err != nil {
		return SearchTagsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "q")), nil
	}
	tags := []TagSuggestion{}
	if s.o.TagSearch != nil {
		found, err := s.o.TagSearch.Search(ctx, req.Params.Q, limit)
		if errors.Is(err, apptimeline.ErrInvalidTagSearchQuery) {
			return SearchTagsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "q")), nil
		}
		if err != nil {
			return nil, err
		}
		for _, t := range found {
			tags = append(tags, TagSuggestion{Name: t.Name, StatusesCount: t.StatusesCount})
		}
	}
	cache := cacheSearch
	return SearchTags200JSONResponse{Body: TagSuggestionList{Items: tags}, Headers: SearchTags200ResponseHeaders{CacheControl: &cache}}, nil
}
