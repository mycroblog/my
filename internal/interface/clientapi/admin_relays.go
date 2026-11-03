// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The relays the instance joins (ADR-0061, FR-724): the admin operations on /admin/relays
// (manage_settings, ADR-0058 server administration).

// ProblemTypeInvalidRelay is the problem type of a relay operation that breaks a rule (422),
// whose reason says which.
const ProblemTypeInvalidRelay = "/api/v1/problems/invalid-relay"

// Relays runs the use cases of the relays. *appfederation.Relays implements it.
type Relays interface {
	List(ctx context.Context) ([]appfederation.RelayView, error)
	Add(ctx context.Context, in appfederation.RelayInput, byUserID string) (appfederation.RelayView, error)
	Follow(ctx context.Context, id, byUserID string) (appfederation.RelayView, error)
	Unfollow(ctx context.Context, id, byUserID string) (appfederation.RelayView, error)
	Remove(ctx context.Context, id, byUserID string) error
}

// requireRelays checks the manage_settings permission and that the use cases are configured; it
// returns the problem to answer otherwise, and the viewer.
func (s *Server) requireRelays(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.Relays == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

// newAdminRelay presents a relay to the administrators.
func newAdminRelay(v appfederation.RelayView) AdminRelay {
	r, st := v.Relay, v.Status
	out := AdminRelay{
		Id: r.ID, Protocol: RelayProtocol(r.Protocol), Url: string(r.URL), InboxUrl: string(r.InboxURL), Receive: r.Receive,
		State: RelayState(st.State), StateChangedAt: st.Since.UTC(), TakenLastDay: int(v.Taken), DroppedLastDay: int(v.Dropped),
		DomainSuspended: v.Suspended, CreatedAt: r.CreatedAt.UTC(),
	}
	if st.State == "" {
		out.State, out.StateChangedAt = RelayStatePending, r.CreatedAt.UTC()
	}
	if st.RelayActorURI != "" {
		a := string(st.RelayActorURI)
		out.ActorUri = &a
	}
	return out
}

func adminRelayResponse(v appfederation.RelayView) AdminRelayJSONResponse {
	cache := cacheNoStore
	return AdminRelayJSONResponse{Body: newAdminRelay(v), Headers: AdminRelayResponseHeaders{CacheControl: &cache}}
}

// relayProblem returns the 422 of a relay operation that breaks a rule, and false for another
// error.
func (s *Server) relayProblem(ctx context.Context, err error) (RelayProblemApplicationProblemPlusJSONResponse, bool) {
	var reason RelayProblemReason
	switch {
	case errors.Is(err, federation.ErrInvalidRelay):
		reason = RelayProblemReasonUrl
	case errors.Is(err, appfederation.ErrRelayExists):
		reason = RelayProblemReasonExists
	case errors.Is(err, appfederation.ErrRelayDomainSuspended):
		reason = RelayProblemReasonDomainSuspended
	case errors.Is(err, appfederation.ErrRelayNotActor):
		reason = RelayProblemReasonNotActor
	case errors.Is(err, federation.ErrRelayNotJoinable), errors.Is(err, federation.ErrRelayNotJoined),
		errors.Is(err, appfederation.ErrRelayNotRemovable):
		reason = RelayProblemReasonState
	default:
		return RelayProblemApplicationProblemPlusJSONResponse{}, false
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidRelay,
		l.ServerErrorsRelayInvalid(), l.ServerErrorsRelayInvalidDetail(string(reason)))
	return RelayProblemApplicationProblemPlusJSONResponse{
		Body: RelayProblem{
			Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
		},
		Headers: RelayProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}, true
}

func relayNotFound(err error) bool { return errors.Is(err, appfederation.ErrNotFound) }

// ListAdminRelays serves GET /api/v1/admin/relays.
func (s *Server) ListAdminRelays(ctx context.Context, _ ListAdminRelaysRequestObject) (ListAdminRelaysResponseObject, error) {
	_, p, err := s.requireRelays(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return ListAdminRelaysdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	list, err := s.o.Relays.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list relays: %w", err)
	}
	out := AdminRelayList{Items: make([]AdminRelay, 0, len(list))}
	for _, v := range list {
		out.Items = append(out.Items, newAdminRelay(v))
	}
	cache := cacheNoStore
	return ListAdminRelays200JSONResponse{Body: out, Headers: ListAdminRelays200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateRelay serves POST /api/v1/admin/relays.
func (s *Server) CreateRelay(ctx context.Context, req CreateRelayRequestObject) (CreateRelayResponseObject, error) {
	v, p, err := s.requireRelays(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return CreateRelaydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return CreateRelaydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	in := appfederation.RelayInput{URL: req.Body.Url, Receive: true}
	if pr := req.Body.Protocol; pr != nil {
		in.Protocol = federation.RelayProtocol(*pr)
		if !in.Protocol.Valid() {
			return CreateRelaydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "protocol")), nil
		}
	}
	if r := req.Body.Receive; r != nil {
		in.Receive = *r
	}
	rv, err := s.o.Relays.Add(ctx, in, v.UserID)
	if rp, ok := s.relayProblem(ctx, err); ok {
		return CreateRelay422ApplicationProblemPlusJSONResponse{rp}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: add relay: %w", err)
	}
	return CreateRelay201JSONResponse{adminRelayResponse(rv)}, nil
}

// DeleteRelay serves DELETE /api/v1/admin/relays/{id}.
func (s *Server) DeleteRelay(ctx context.Context, req DeleteRelayRequestObject) (DeleteRelayResponseObject, error) {
	v, p, err := s.requireRelays(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return DeleteRelaydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.Relays.Remove(ctx, req.Id, v.UserID)
	if rp, ok := s.relayProblem(ctx, err); ok {
		return DeleteRelay422ApplicationProblemPlusJSONResponse{rp}, nil
	}
	switch {
	case relayNotFound(err):
		return DeleteRelaydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: remove relay %s: %w", req.Id, err)
	}
	return DeleteRelay204Response{}, nil
}

// FollowRelay serves POST /api/v1/admin/relays/{id}/follow.
func (s *Server) FollowRelay(ctx context.Context, req FollowRelayRequestObject) (FollowRelayResponseObject, error) {
	v, p, err := s.requireRelays(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return FollowRelaydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	rv, err := s.o.Relays.Follow(ctx, req.Id, v.UserID)
	if rp, ok := s.relayProblem(ctx, err); ok {
		return FollowRelay422ApplicationProblemPlusJSONResponse{rp}, nil
	}
	switch {
	case relayNotFound(err):
		return FollowRelaydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: follow relay %s: %w", req.Id, err)
	}
	return FollowRelay200JSONResponse{adminRelayResponse(rv)}, nil
}

// UnfollowRelay serves POST /api/v1/admin/relays/{id}/unfollow.
func (s *Server) UnfollowRelay(ctx context.Context, req UnfollowRelayRequestObject) (UnfollowRelayResponseObject, error) {
	v, p, err := s.requireRelays(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return UnfollowRelaydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	rv, err := s.o.Relays.Unfollow(ctx, req.Id, v.UserID)
	if rp, ok := s.relayProblem(ctx, err); ok {
		return UnfollowRelay422ApplicationProblemPlusJSONResponse{rp}, nil
	}
	switch {
	case relayNotFound(err):
		return UnfollowRelaydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: unfollow relay %s: %w", req.Id, err)
	}
	return UnfollowRelay200JSONResponse{adminRelayResponse(rv)}, nil
}
