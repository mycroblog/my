// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/graph"
)

// Graph is the part of the graph module's public API that the client API uses: follows,
// follow requests, blocks, and mutes of local accounts (FR-301〜303). *appgraph.Service
// implements it.
type Graph interface {
	Relationship(ctx context.Context, accountID, targetID string) (appgraph.Relationship, error)
	IsBlocking(ctx context.Context, blockerID, blockedID string) (bool, error)
	Counters(ctx context.Context, accountID string) (graph.Counters, error)

	Follow(ctx context.Context, followerID, followeeID string) (appgraph.Relationship, error)
	FollowWith(ctx context.Context, followerID, followeeID string, opts appgraph.FollowOptions) (appgraph.Relationship, error)
	UpdatePreferences(ctx context.Context, followerID, followeeID string, in appgraph.PreferencesInput) (appgraph.Relationship, error)
	Unfollow(ctx context.Context, followerID, followeeID string) (appgraph.Relationship, error)
	RemoveFollower(ctx context.Context, followeeID, followerID string) (appgraph.Relationship, error)
	AcceptFollowRequest(ctx context.Context, followeeID, followerID string) (appgraph.Relationship, error)
	RejectFollowRequest(ctx context.Context, followeeID, followerID string) (appgraph.Relationship, error)
	Block(ctx context.Context, blockerID, blockedID string) (appgraph.Relationship, error)
	Unblock(ctx context.Context, blockerID, blockedID string) (appgraph.Relationship, error)
	Mute(ctx context.Context, accountID, targetID string, in appgraph.MuteInput) (appgraph.Relationship, error)
	Unmute(ctx context.Context, accountID, targetID string) (appgraph.Relationship, error)

	Followers(ctx context.Context, accountID string, p appgraph.Page) ([]graph.Edge, error)
	Following(ctx context.Context, accountID string, p appgraph.Page) ([]graph.Edge, error)
	FollowRequests(ctx context.Context, accountID string, p appgraph.Page) ([]graph.Edge, error)
	FollowRequestCount(ctx context.Context, accountID string) (int64, error)
	Blocks(ctx context.Context, accountID string, p appgraph.Page) ([]graph.BlockEdge, error)
	Mutes(ctx context.Context, accountID string, p appgraph.MutePage) ([]graph.Mute, error)

	MuteDomain(ctx context.Context, accountID, domain string) (graph.DomainMute, error)
	UnmuteDomain(ctx context.Context, accountID, domain string) error
	DomainMutes(ctx context.Context, accountID string, p appgraph.DomainMutePage) ([]graph.DomainMute, error)
}

// ProblemTypeBlocking is the problem of following an account that the viewer blocks.
const ProblemTypeBlocking = "/api/v1/problems/blocking"

// maxMuteSeconds is the longest mute duration of the spec (MuteAccountRequest.duration).
const maxMuteSeconds = 365 * 24 * 60 * 60

// newRelationship presents a relationship as the Relationship schema. It leaves out
// BlockedBy: an account that blocks the viewer is not found at all.
func newRelationship(r appgraph.Relationship) Relationship {
	return Relationship{
		Id:                  r.TargetAccountID,
		Following:           r.Following == graph.StateActive,
		Requested:           r.Following == graph.StatePending,
		FollowedBy:          r.FollowedBy == graph.StateActive,
		RequestedBy:         r.FollowedBy == graph.StatePending,
		ShowBoosts:          r.ShowBoosts,
		Notify:              r.Notify,
		Blocking:            r.Blocking,
		Muting:              r.Muting,
		MutingNotifications: r.MutingNotifications,
	}
}

func relationshipResponse(r appgraph.Relationship) RelationshipJSONResponse {
	cache := cacheNoStore
	return RelationshipJSONResponse{Body: newRelationship(r), Headers: RelationshipResponseHeaders{CacheControl: &cache}}
}

// graphProblem returns the problem that a graph use case's error means, and false for an
// unexpected error.
func (s *Server) graphProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, graph.ErrSelfFollow), errors.Is(err, graph.ErrSelfBlock), errors.Is(err, graph.ErrSelfMute):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsSelfRelationship()), true
	case errors.Is(err, appgraph.ErrRemoteAccount):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsRemoteAccount()), true
	case errors.Is(err, appgraph.ErrInteractionRestricted):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeBlank, l.ServerErrorsForbidden(), l.ServerErrorsInteractionRestricted()), true
	case errors.Is(err, appgraph.ErrBlocked):
		// The target does not block the viewer (relate checked it), so the viewer blocks the target.
		return s.problem(ctx, http.StatusConflict, ProblemTypeBlocking, l.ServerErrorsBlocking(), ""), true
	case errors.Is(err, appgraph.ErrAccountUnavailable), errors.Is(err, appgraph.ErrNotFound):
		return s.notFoundResponse(ctx), true
	}
	return problem{}, false
}

// relate runs a graph use case of the viewer on the account with targetID, after checking that
// the viewer may see the account. It returns either the resulting relationship or the problem
// to answer with.
func (s *Server) relate(ctx context.Context, targetID string, change func(v Viewer) (appgraph.Relationship, error)) (*RelationshipJSONResponse, *problem, error) {
	return s.relateGuarded(ctx, targetID, nil, change)
}

// relateGuarded is relate with guard, which runs on the visible account before change and
// returns the problem that refuses the change, or nil; a nil guard refuses nothing.
func (s *Server) relateGuarded(ctx context.Context, targetID string, guard func(ctx context.Context, v Viewer, a appaccount.Actor) (*problem, error), change func(v Viewer) (appgraph.Relationship, error)) (*RelationshipJSONResponse, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	a, ok, err := s.visibleAccount(ctx, &v, targetID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		p := s.notFoundResponse(ctx)
		return nil, &p, nil
	}
	if guard != nil {
		if p, err := guard(ctx, v, a); err != nil || p != nil {
			return nil, p, err
		}
	}
	r, err := change(v)
	if err != nil {
		if p, ok := s.graphProblem(ctx, err); ok {
			return nil, &p, nil
		}
		return nil, nil, err
	}
	res := relationshipResponse(r)
	return &res, nil, nil
}

// GetRelationships serves GET /api/v1/accounts/relationships.
func (s *Server) GetRelationships(ctx context.Context, req GetRelationshipsRequestObject) (GetRelationshipsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Params.Id) > 40 {
		return GetRelationshipsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "id")), nil
	}
	out := []Relationship{}
	var seen []string
	for _, id := range req.Params.Id {
		id = knownID(id)
		if slices.Contains(seen, id) {
			continue
		}
		seen = append(seen, id)
		_, ok, err := s.visibleAccount(ctx, &v, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		r, err := s.o.Graph.Relationship(ctx, v.AccountID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, newRelationship(r))
	}
	cache := cacheNoStore
	return GetRelationships200JSONResponse{Body: out, Headers: GetRelationships200ResponseHeaders{CacheControl: &cache}}, nil
}

// FollowAccount serves POST /api/v1/accounts/{id}/follow.
func (s *Server) FollowAccount(ctx context.Context, req FollowAccountRequestObject) (FollowAccountResponseObject, error) {
	res, p, err := s.relateGuarded(ctx, req.Id, s.followable, func(v Viewer) (appgraph.Relationship, error) {
		var opts appgraph.FollowOptions
		if req.Body != nil {
			opts = appgraph.FollowOptions{ShowBoosts: req.Body.ShowBoosts, Notify: req.Body.Notify}
		}
		return s.o.Graph.FollowWith(ctx, v.AccountID, req.Id, opts)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return FollowAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return FollowAccount200JSONResponse{*res}, nil
}

// followable refuses every follow of an account that moved (ADR-0089), and then the follows that
// followableRemotely refuses.
func (s *Server) followable(ctx context.Context, v Viewer, a appaccount.Actor) (*problem, error) {
	if p, err := s.refuseMoved(ctx, v.AccountID); err != nil || p != nil {
		return p, err
	}
	return s.followableRemotely(ctx, v, a)
}

// followableRemotely refuses a new follow of a remote account while the federation mode does
// not follow remote accounts (ADR-0076); a follow or request that exists already may still change
// its preferences, since nothing goes out for that.
func (s *Server) followableRemotely(ctx context.Context, v Viewer, a appaccount.Actor) (*problem, error) {
	if a.Account.IsLocal() {
		return nil, nil
	}
	if ok, err := s.federationAllows(ctx, remoteAccountFlows...); err != nil || ok {
		return nil, err
	}
	r, err := s.o.Graph.Relationship(ctx, v.AccountID, a.Account.ID)
	if err != nil {
		return nil, err
	}
	if r.Following == graph.StateActive || r.Following == graph.StatePending {
		return nil, nil
	}
	p := s.federationRestricted(ctx)
	return &p, nil
}

// UnfollowAccount serves POST /api/v1/accounts/{id}/unfollow.
func (s *Server) UnfollowAccount(ctx context.Context, req UnfollowAccountRequestObject) (UnfollowAccountResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.Unfollow(ctx, v.AccountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UnfollowAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnfollowAccount200JSONResponse{*res}, nil
}

// RemoveAccountFromFollowers serves POST /api/v1/accounts/{id}/remove_from_followers.
func (s *Server) RemoveAccountFromFollowers(ctx context.Context, req RemoveAccountFromFollowersRequestObject) (RemoveAccountFromFollowersResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.RemoveFollower(ctx, v.AccountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return RemoveAccountFromFollowersdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return RemoveAccountFromFollowers200JSONResponse{*res}, nil
}

// AcceptFollowRequest serves POST /api/v1/follow_requests/{id}/authorize.
func (s *Server) AcceptFollowRequest(ctx context.Context, req AcceptFollowRequestRequestObject) (AcceptFollowRequestResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.AcceptFollowRequest(ctx, v.AccountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return AcceptFollowRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return AcceptFollowRequest200JSONResponse{*res}, nil
}

// RejectFollowRequest serves POST /api/v1/follow_requests/{id}/reject.
func (s *Server) RejectFollowRequest(ctx context.Context, req RejectFollowRequestRequestObject) (RejectFollowRequestResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.RejectFollowRequest(ctx, v.AccountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return RejectFollowRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return RejectFollowRequest200JSONResponse{*res}, nil
}

// BlockAccount serves POST /api/v1/accounts/{id}/block.
func (s *Server) BlockAccount(ctx context.Context, req BlockAccountRequestObject) (BlockAccountResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.Block(ctx, v.AccountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return BlockAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return BlockAccount200JSONResponse{*res}, nil
}

// UnblockAccount serves POST /api/v1/accounts/{id}/unblock.
func (s *Server) UnblockAccount(ctx context.Context, req UnblockAccountRequestObject) (UnblockAccountResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.Unblock(ctx, v.AccountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UnblockAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnblockAccount200JSONResponse{*res}, nil
}

// MuteAccount serves POST /api/v1/accounts/{id}/mute.
func (s *Server) MuteAccount(ctx context.Context, req MuteAccountRequestObject) (MuteAccountResponseObject, error) {
	in := appgraph.MuteInput{HideNotifications: true}
	if b := req.Body; b != nil {
		if b.Notifications != nil {
			in.HideNotifications = *b.Notifications
		}
		if b.Duration != nil {
			if *b.Duration < 0 || *b.Duration > maxMuteSeconds {
				return MuteAccountdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "duration")), nil
			}
			in.Duration = time.Duration(*b.Duration) * time.Second
		}
	}
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.Mute(ctx, v.AccountID, req.Id, in)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return MuteAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return MuteAccount200JSONResponse{*res}, nil
}

// UnmuteAccount serves POST /api/v1/accounts/{id}/unmute.
func (s *Server) UnmuteAccount(ctx context.Context, req UnmuteAccountRequestObject) (UnmuteAccountResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.o.Graph.Unmute(ctx, v.AccountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UnmuteAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnmuteAccount200JSONResponse{*res}, nil
}
