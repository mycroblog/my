// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appfederation "github.com/mycroblog/my/internal/app/federation"
)

// StatusResolver finds the posts that a URL names (lookupStatus): Stored among the stored ones,
// and Resolve by fetching an unknown one from its server. Both fail with an error wrapping
// appfederation.ErrUnresolvable when the URL leads to no post. *appfederation.Resolver
// implements it.
type StatusResolver interface {
	Stored(ctx context.Context, uri string) (appfederation.Resolved, bool, error)
	Resolve(ctx context.Context, query string) (appfederation.Resolved, error)
}

// LookupStatus serves GET /api/v1/statuses/lookup: the status with an ActivityPub id, the stored
// one or the one fetched from its server, which the page of the Create intents of WebFinger
// (FEP-3b86) answers. It is presented as getStatus presents it, with the same 404 for a status
// the viewer may not see.
func (s *Server) LookupStatus(ctx context.Context, req LookupStatusRequestObject) (LookupStatusResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.StatusResolver == nil {
		return LookupStatusdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	res, found, err := s.o.StatusResolver.Stored(ctx, req.Params.Uri)
	switch {
	case errors.Is(err, appfederation.ErrUnresolvable):
		return LookupStatusdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, err
	}
	if !found {
		// A mode that takes nothing in finds only the known statuses (ADR-0006).
		if ok, err := s.federationAllows(ctx, remoteAccountFlows...); err != nil {
			return nil, err
		} else if !ok {
			return LookupStatusdefaultApplicationProblemPlusJSONResponse(s.federationRestricted(ctx)), nil
		}
		if p, h, limited := s.LimitProblem(ctx, ResolveLimit, v.AccountID); limited {
			return LookupStatusdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
		}
		res, err = s.o.StatusResolver.Resolve(ctx, req.Params.Uri)
		switch {
		case errors.Is(err, appfederation.ErrUnresolvable):
			return LookupStatusdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
		case err != nil:
			return nil, err
		}
	}
	if res.Status == nil {
		// The URL names an account, not a post.
		return LookupStatusdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	got, err := s.GetStatus(ctx, GetStatusRequestObject{Id: res.Status.ID})
	if err != nil {
		return nil, err
	}
	switch r := got.(type) {
	case GetStatus200JSONResponse:
		return LookupStatus200JSONResponse{Body: r.Body, Headers: LookupStatus200ResponseHeaders(r.Headers)}, nil
	case GetStatusdefaultApplicationProblemPlusJSONResponse:
		return LookupStatusdefaultApplicationProblemPlusJSONResponse(r), nil
	}
	return nil, errors.New("clientapi: getStatus answered an unexpected response")
}
