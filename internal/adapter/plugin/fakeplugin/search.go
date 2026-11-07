// SPDX-License-Identifier: AGPL-3.0-only

package fakeplugin

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect/v2"

	searchv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/search/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/search/v1/searchv1connect"
	"github.com/mycroblog/my/internal/app/search"
)

type searchServer struct {
	searchv1connect.UnimplementedSearchServiceHandler

	o Options
}

// misbehave answers in place of the index when the options say so, and reports whether it did.
func (s *searchServer) misbehave(ctx context.Context) (bool, error) {
	switch {
	case s.o.Search == nil:
		return true, connect.NewError(connect.CodeUnimplemented, "this fake has no search index")
	case s.o.Block:
		<-ctx.Done()
		return true, ctx.Err()
	}
	return false, nil
}

// writeOutcome is the outcome of a write of the index: an error of the port is a failure of the engine.
func (s *searchServer) writeOutcome(err error) searchv1.Outcome {
	if s.o.Broken {
		return searchv1.Outcome_OUTCOME_UNSPECIFIED
	}
	if err != nil {
		return searchv1.Outcome_OUTCOME_FAILED
	}
	return searchv1.Outcome_OUTCOME_OK
}

var searchVisibilities = map[searchv1.Visibility]search.Visibility{
	searchv1.Visibility_VISIBILITY_PUBLIC: search.VisibilityPublic,
	searchv1.Visibility_VISIBILITY_LOCAL:  search.VisibilityLocal,
}

// Replace implements searchv1connect.SearchServiceHandler. A visibility the port does not know reaches the index as the
// empty one, which refuses it.
func (s *searchServer) Replace(ctx context.Context, req *searchv1.ReplaceRequest) (*searchv1.ReplaceResponse, error) {
	if ok, err := s.misbehave(ctx); ok {
		return nil, err
	}
	d := req.GetDocument()
	published, err := time.Parse(time.RFC3339Nano, d.GetPublishedAt())
	if err != nil {
		return &searchv1.ReplaceResponse{Outcome: searchv1.Outcome_OUTCOME_FAILED}, nil
	}
	err = s.o.Search.Replace(ctx, search.Document{
		StatusID: d.GetStatusId(), AuthorID: d.GetAuthorId(), Visibility: searchVisibilities[d.GetVisibility()],
		Language: d.GetLanguage(), PublishedAt: published, Text: d.GetText(),
	})
	return &searchv1.ReplaceResponse{Outcome: s.writeOutcome(err)}, nil
}

// Remove implements searchv1connect.SearchServiceHandler.
func (s *searchServer) Remove(ctx context.Context, req *searchv1.RemoveRequest) (*searchv1.RemoveResponse, error) {
	if ok, err := s.misbehave(ctx); ok {
		return nil, err
	}
	return &searchv1.RemoveResponse{Outcome: s.writeOutcome(s.o.Search.Remove(ctx, req.GetStatusId()))}, nil
}

// RemoveByAuthor implements searchv1connect.SearchServiceHandler.
func (s *searchServer) RemoveByAuthor(ctx context.Context, req *searchv1.RemoveByAuthorRequest) (*searchv1.RemoveByAuthorResponse, error) {
	if ok, err := s.misbehave(ctx); ok {
		return nil, err
	}
	return &searchv1.RemoveByAuthorResponse{Outcome: s.writeOutcome(s.o.Search.RemoveByAuthor(ctx, req.GetAuthorId()))}, nil
}

// RemoveByVisibility implements searchv1connect.SearchServiceHandler.
func (s *searchServer) RemoveByVisibility(ctx context.Context, req *searchv1.RemoveByVisibilityRequest) (*searchv1.RemoveByVisibilityResponse, error) {
	if ok, err := s.misbehave(ctx); ok {
		return nil, err
	}
	v, known := searchVisibilities[req.GetVisibility()]
	if !known {
		return &searchv1.RemoveByVisibilityResponse{Outcome: searchv1.Outcome_OUTCOME_FAILED}, nil
	}
	return &searchv1.RemoveByVisibilityResponse{Outcome: s.writeOutcome(s.o.Search.RemoveByVisibility(ctx, v))}, nil
}

// Clear implements searchv1connect.SearchServiceHandler.
func (s *searchServer) Clear(ctx context.Context, _ *searchv1.ClearRequest) (*searchv1.ClearResponse, error) {
	if ok, err := s.misbehave(ctx); ok {
		return nil, err
	}
	return &searchv1.ClearResponse{Outcome: s.writeOutcome(s.o.Search.Clear(ctx))}, nil
}

// Search implements searchv1connect.SearchServiceHandler. The page goes out as the index made it.
func (s *searchServer) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	if ok, err := s.misbehave(ctx); ok {
		return nil, err
	}
	if s.o.Broken {
		return &searchv1.SearchResponse{}, nil
	}
	page, err := s.o.Search.Search(ctx, search.Query{Text: req.GetText(), Cursor: req.GetCursor(), Limit: int(req.GetLimit())})
	switch {
	case errors.Is(err, search.ErrInvalidCursor):
		return &searchv1.SearchResponse{Outcome: searchv1.Outcome_OUTCOME_INVALID_CURSOR}, nil
	case err != nil:
		return &searchv1.SearchResponse{Outcome: searchv1.Outcome_OUTCOME_FAILED}, nil
	}
	return &searchv1.SearchResponse{Outcome: searchv1.Outcome_OUTCOME_OK, StatusIds: page.StatusIDs, NextCursor: page.Next}, nil
}
