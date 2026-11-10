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

// ProblemTypeTooManyFeaturedTags is the problem of more hashtags than an account features
// (ADR-0006 §17).
const ProblemTypeTooManyFeaturedTags = "/api/v1/problems/too-many-featured-tags"

// FeaturedTags keeps the hashtags that accounts feature on their profiles
// (*apptimeline.Service, ADR-0006 §17).
type FeaturedTags interface {
	// SetFeaturedTags fails with appcontent.ErrInvalidHashtag for a name that is no hashtag and
	// with timeline.ErrTooManyFeaturedTags for too many names.
	SetFeaturedTags(ctx context.Context, accountID string, names []string) ([]apptimeline.FeaturedTag, error)
	FeaturedTags(ctx context.Context, accountID string) ([]apptimeline.FeaturedTag, error)
}

// featuredTagsResponse returns the featured hashtags tags with the Cache-Control cache.
func (s *Server) featuredTagsResponse(tags []apptimeline.FeaturedTag, cache *string) FeaturedHashtagCollectionJSONResponse {
	items := make([]FeaturedHashtag, 0, len(tags))
	for _, t := range tags {
		item := FeaturedHashtag{Name: t.Tag.Name, Url: s.tagURL(t.Tag), StatusesCount: t.StatusesCount}
		if !t.LastStatusAt.IsZero() {
			at := t.LastStatusAt
			item.LastStatusAt = &at
		}
		items = append(items, item)
	}
	return FeaturedHashtagCollectionJSONResponse{
		Body: FeaturedHashtagCollection{Items: items}, Headers: FeaturedHashtagCollectionResponseHeaders{CacheControl: cache},
	}
}

// ListFeaturedTags serves GET /api/v1/featured_tags: the viewer's own featured hashtags.
func (s *Server) ListFeaturedTags(ctx context.Context, _ ListFeaturedTagsRequestObject) (ListFeaturedTagsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	var tags []apptimeline.FeaturedTag
	if s.o.FeaturedTags != nil {
		if tags, err = s.o.FeaturedTags.FeaturedTags(ctx, v.AccountID); err != nil {
			return nil, err
		}
	}
	cache := cacheNoStore
	return ListFeaturedTags200JSONResponse{s.featuredTagsResponse(tags, &cache)}, nil
}

// SetFeaturedTags serves PUT /api/v1/featured_tags.
func (s *Server) SetFeaturedTags(ctx context.Context, req SetFeaturedTagsRequestObject) (SetFeaturedTagsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.FeaturedTags == nil {
		return SetFeaturedTagsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return SetFeaturedTagsdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "names")), nil
	}
	tags, err := s.o.FeaturedTags.SetFeaturedTags(ctx, v.AccountID, req.Body.Names)
	switch {
	case errors.Is(err, appcontent.ErrInvalidHashtag):
		return SetFeaturedTagsdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "names")), nil
	case errors.Is(err, timeline.ErrTooManyFeaturedTags):
		return SetFeaturedTagsdefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusUnprocessableEntity,
			ProblemTypeTooManyFeaturedTags, s.Localizer(ctx).ServerErrorsFeaturedTagsTooMany(), "")), nil
	case err != nil:
		return nil, err
	}
	cache := cacheNoStore
	return SetFeaturedTags200JSONResponse{s.featuredTagsResponse(tags, &cache)}, nil
}

// ListAccountFeaturedTags serves GET /api/v1/accounts/{id}/featured_tags: the hashtags that an
// account the viewer may see features.
func (s *Server) ListAccountFeaturedTags(ctx context.Context, req ListAccountFeaturedTagsRequestObject) (ListAccountFeaturedTagsResponseObject, error) {
	v := viewerOf(ctx)
	a, ok, err := s.visibleAccount(ctx, v, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok || a.IsInstanceActor() {
		return ListAccountFeaturedTagsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	var tags []apptimeline.FeaturedTag
	if s.o.FeaturedTags != nil {
		if tags, err = s.o.FeaturedTags.FeaturedTags(ctx, a.Account.ID); err != nil {
			return nil, err
		}
	}
	return ListAccountFeaturedTags200JSONResponse{s.featuredTagsResponse(tags, cacheFor(v))}, nil
}
