// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// ProblemTypeTooManyFollowedTags is the problem of a hashtag beyond the most a user follows
// (ADR-0013 §27).
const ProblemTypeTooManyFollowedTags = "/api/v1/problems/too-many-followed-tags"

// FollowedTags keeps the hashtags that the viewer follows, whose posts the home timeline lists
// (*apptimeline.Service, ADR-0013 §24 to §28). The methods that take a name fail with
// appcontent.ErrInvalidHashtag for one that is no hashtag.
type FollowedTags interface {
	FollowTag(ctx context.Context, accountID, name string) (appcontent.Tag, error)
	UnfollowTag(ctx context.Context, accountID, name string) (appcontent.Tag, error)
	TagFollowing(ctx context.Context, accountID, name string) (appcontent.Tag, bool, error)
	FollowedTags(ctx context.Context, accountID string) ([]apptimeline.FollowedTag, error)
}

// tagURL is the hashtag's page on this server.
func (s *Server) tagURL(t appcontent.Tag) string {
	return s.origin() + "/tags/" + url.PathEscape(t.NameNormalized)
}

func (s *Server) hashtagResponse(t appcontent.Tag, following bool, cache string) HashtagJSONResponse {
	return HashtagJSONResponse{
		Body:    Hashtag{Name: t.Name, Url: s.tagURL(t), Following: following},
		Headers: HashtagResponseHeaders{CacheControl: &cache},
	}
}

// followedTagProblem returns the problem that answers err of the followed hashtags, false for
// an error that is none of theirs.
func (s *Server) followedTagProblem(ctx context.Context, err error) (problem, bool) {
	switch {
	case errors.Is(err, appcontent.ErrInvalidHashtag):
		return s.unprocessableValue(ctx, "hashtag"), true
	case errors.Is(err, timeline.ErrTooManyFollowedTags):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTooManyFollowedTags, s.Localizer(ctx).ServerErrorsFollowedTagsTooMany(), ""), true
	}
	return problem{}, false
}

// GetTag serves GET /api/v1/tags/{hashtag}: the hashtag, stored or not, and whether the viewer
// follows it (never for a visitor).
func (s *Server) GetTag(ctx context.Context, req GetTagRequestObject) (GetTagResponseObject, error) {
	if s.o.FollowedTags == nil {
		return GetTagdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	accountID, cache := "", cachePublic
	if v, ok := ViewerFrom(ctx); ok {
		accountID, cache = v.AccountID, cacheNoStore
	}
	tag, following, err := s.o.FollowedTags.TagFollowing(ctx, accountID, req.Hashtag)
	if err != nil {
		if p, ok := s.followedTagProblem(ctx, err); ok {
			return GetTagdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return GetTag200JSONResponse{s.hashtagResponse(tag, following, cache)}, nil
}

// FollowTag serves POST /api/v1/tags/{hashtag}/follow.
func (s *Server) FollowTag(ctx context.Context, req FollowTagRequestObject) (FollowTagResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.FollowedTags == nil {
		return FollowTagdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	tag, err := s.o.FollowedTags.FollowTag(ctx, v.AccountID, req.Hashtag)
	if err != nil {
		if p, ok := s.followedTagProblem(ctx, err); ok {
			return FollowTagdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return FollowTag200JSONResponse{s.hashtagResponse(tag, true, cacheNoStore)}, nil
}

// UnfollowTag serves DELETE /api/v1/tags/{hashtag}/follow.
func (s *Server) UnfollowTag(ctx context.Context, req UnfollowTagRequestObject) (UnfollowTagResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.FollowedTags == nil {
		return UnfollowTagdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	tag, err := s.o.FollowedTags.UnfollowTag(ctx, v.AccountID, req.Hashtag)
	if err != nil {
		if p, ok := s.followedTagProblem(ctx, err); ok {
			return UnfollowTagdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return UnfollowTag200JSONResponse{s.hashtagResponse(tag, false, cacheNoStore)}, nil
}

// ListFollowedTags serves GET /api/v1/followed_tags.
func (s *Server) ListFollowedTags(ctx context.Context, _ ListFollowedTagsRequestObject) (ListFollowedTagsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	items := []FollowedHashtag{}
	if s.o.FollowedTags != nil {
		tags, err := s.o.FollowedTags.FollowedTags(ctx, v.AccountID)
		if err != nil {
			return nil, err
		}
		for _, t := range tags {
			items = append(items, FollowedHashtag{Name: t.Tag.Name, Url: s.tagURL(t.Tag), FollowedAt: t.FollowedAt})
		}
	}
	cache := cacheNoStore
	return ListFollowedTags200JSONResponse{FollowedHashtagCollectionJSONResponse{
		Body: FollowedHashtagCollection{Items: items}, Headers: FollowedHashtagCollectionResponseHeaders{CacheControl: &cache},
	}}, nil
}
