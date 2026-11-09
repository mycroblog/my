// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/graph"
)

// Keyset pages of accounts (docs/architecture/client-api.md §2.2). A cursor is the base64url
// of the position of the page's last item: the follow ID of a follow list, the block ID of the
// block list, and the creation time and target of a mute. A page asks the graph module for one
// item more than its limit, so that the last page has no next_cursor.

// pageQuery is the parsed Cursor and Limit of a list request; problem is set when either is
// invalid.
type pageQuery struct {
	before  string
	limit   int
	problem *problem
}

func (s *Server) parsePage(ctx context.Context, cursor *Cursor, limit *Limit) pageQuery {
	return s.parsePageOf(ctx, cursor, limit, kernel.ValidID)
}

// parseOpaquePage is parsePage for a cursor that is not an ID (a domain, a time and an ID):
// the endpoint's own decoder checks its content.
func (s *Server) parseOpaquePage(ctx context.Context, cursor *Cursor, limit *Limit) pageQuery {
	return s.parsePageOf(ctx, cursor, limit, func(string) bool { return true })
}

// parsePageOf parses a page request whose cursor is valid when its decoded content passes
// valid; a cursor that does not is a broken cursor.
func (s *Server) parsePageOf(ctx context.Context, cursor *Cursor, limit *Limit, valid func(string) bool) pageQuery {
	n, ok := pageLimit(limit)
	if !ok {
		p := s.invalidParameterResponse(ctx, "limit")
		return pageQuery{problem: &p}
	}
	q := pageQuery{limit: n}
	if cursor != nil {
		b, err := base64.RawURLEncoding.DecodeString(*cursor)
		if err != nil || len(b) == 0 || !valid(string(b)) {
			p := s.invalidParameterResponse(ctx, "cursor")
			return pageQuery{problem: &p}
		}
		q.before = string(b)
	}
	return q
}

func encodeCursor(position string) *Cursor {
	c := base64.RawURLEncoding.EncodeToString([]byte(position))
	return &c
}

// trim cuts a page fetched with one item more than limit to limit items and returns the
// index of the page's last item when another page follows, or -1.
func trim[T any](items []T, limit int) ([]T, int) {
	if len(items) <= limit {
		return items, -1
	}
	return items[:limit], limit - 1
}

// presentAccounts presents the accounts with ids in order, leaving out those that are gone
// or not presentable (presentable with blockCheck).
func (s *Server) presentAccounts(ctx context.Context, viewer *Viewer, ids []string, blockCheck bool) ([]Account, error) {
	actors, err := s.presentableActors(ctx, viewer, ids, blockCheck)
	if err != nil {
		return nil, err
	}
	return s.newAccounts(ctx, actors)
}

// edgePage presents a page of follow edges as a page of accounts: the followers of the edges
// when followers is set, the followees otherwise.
func (s *Server) edgePage(ctx context.Context, viewer *Viewer, edges []graph.Edge, limit int, followers, blockCheck bool) (AccountPage, error) {
	edges, last := trim(edges, limit)
	ids := make([]string, len(edges))
	for i, e := range edges {
		ids[i] = e.FolloweeAccountID
		if followers {
			ids[i] = e.FollowerAccountID
		}
	}
	accounts, err := s.presentAccounts(ctx, viewer, ids, blockCheck)
	if err != nil {
		return AccountPage{}, err
	}
	page := AccountPage{Items: accounts}
	if last >= 0 {
		page.NextCursor = encodeCursor(edges[last].FollowID)
	}
	return page, nil
}

// followList serves the followers or following list of the account with id.
func (s *Server) followList(ctx context.Context, id string, cursor *Cursor, limit *Limit, followers bool) (*AccountPageJSONResponse, *problem, error) {
	v := viewerOf(ctx)
	q := s.parsePage(ctx, cursor, limit)
	if q.problem != nil {
		return nil, q.problem, nil
	}
	_, ok, err := s.visibleAccount(ctx, v, id)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		p := s.notFoundResponse(ctx)
		return nil, &p, nil
	}
	list := s.o.Graph.Following
	if followers {
		list = s.o.Graph.Followers
	}
	edges, err := list(ctx, id, appgraph.Page{Before: q.before, Limit: q.limit + 1})
	if err != nil {
		return nil, nil, err
	}
	page, err := s.edgePage(ctx, v, edges, q.limit, followers, true)
	if err != nil {
		return nil, nil, err
	}
	return &AccountPageJSONResponse{Body: page, Headers: AccountPageResponseHeaders{CacheControl: cacheFor(v)}}, nil, nil
}

// ListAccountFollowers serves GET /api/v1/accounts/{id}/followers.
func (s *Server) ListAccountFollowers(ctx context.Context, req ListAccountFollowersRequestObject) (ListAccountFollowersResponseObject, error) {
	res, p, err := s.followList(ctx, req.Id, req.Params.Cursor, req.Params.Limit, true)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return ListAccountFollowersdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return ListAccountFollowers200JSONResponse{*res}, nil
}

// ListAccountFollowing serves GET /api/v1/accounts/{id}/following.
func (s *Server) ListAccountFollowing(ctx context.Context, req ListAccountFollowingRequestObject) (ListAccountFollowingResponseObject, error) {
	res, p, err := s.followList(ctx, req.Id, req.Params.Cursor, req.Params.Limit, false)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return ListAccountFollowingdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return ListAccountFollowing200JSONResponse{*res}, nil
}

func ownPageResponse(page AccountPage) AccountPageJSONResponse {
	cache := cacheNoStore
	return AccountPageJSONResponse{Body: page, Headers: AccountPageResponseHeaders{CacheControl: &cache}}
}

// ListFollowRequests serves GET /api/v1/follow_requests.
func (s *Server) ListFollowRequests(ctx context.Context, req ListFollowRequestsRequestObject) (ListFollowRequestsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListFollowRequestsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	edges, err := s.o.Graph.FollowRequests(ctx, v.AccountID, appgraph.Page{Before: q.before, Limit: q.limit + 1})
	if err != nil {
		return nil, err
	}
	page, err := s.edgePage(ctx, &v, edges, q.limit, true, false)
	if err != nil {
		return nil, err
	}
	return ListFollowRequests200JSONResponse{ownPageResponse(page)}, nil
}

// ListBlocks serves GET /api/v1/blocks.
func (s *Server) ListBlocks(ctx context.Context, req ListBlocksRequestObject) (ListBlocksResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListBlocksdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	edges, err := s.o.Graph.Blocks(ctx, v.AccountID, appgraph.Page{Before: q.before, Limit: q.limit + 1})
	if err != nil {
		return nil, err
	}
	edges, last := trim(edges, q.limit)
	ids := make([]string, len(edges))
	for i, e := range edges {
		ids[i] = e.BlockedAccountID
	}
	// The viewer sees the accounts they block even when those block them back, to unblock them.
	accounts, err := s.presentAccounts(ctx, &v, ids, false)
	if err != nil {
		return nil, err
	}
	page := AccountPage{Items: accounts}
	if last >= 0 {
		page.NextCursor = encodeCursor(edges[last].BlockID)
	}
	return ListBlocks200JSONResponse{ownPageResponse(page)}, nil
}

// muteCursorSeparator separates the creation time and the target of a mute cursor.
const muteCursorSeparator = " "

func encodeMuteCursor(m graph.Mute) *Cursor {
	return encodeCursor(m.CreatedAt.UTC().Format(time.RFC3339Nano) + muteCursorSeparator + m.TargetAccountID)
}

func decodeMuteCursor(position string) (appgraph.MuteCursor, bool) {
	at, target, ok := strings.Cut(position, muteCursorSeparator)
	if !ok || !kernel.ValidID(target) {
		return appgraph.MuteCursor{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return appgraph.MuteCursor{}, false
	}
	return appgraph.MuteCursor{CreatedAt: t, TargetAccountID: target}, true
}

// ListMutes serves GET /api/v1/mutes.
func (s *Server) ListMutes(ctx context.Context, req ListMutesRequestObject) (ListMutesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parseOpaquePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListMutesdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	var before appgraph.MuteCursor
	if q.before != "" {
		var ok bool
		if before, ok = decodeMuteCursor(q.before); !ok {
			return ListMutesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
	}
	mutes, err := s.o.Graph.Mutes(ctx, v.AccountID, appgraph.MutePage{Before: before, Limit: q.limit + 1})
	if err != nil {
		return nil, err
	}
	mutes, last := trim(mutes, q.limit)
	page := MutePage{Items: []MutedAccount{}}
	var actors []appaccount.Actor
	var shown []graph.Mute
	targets := make([]string, len(mutes))
	for i, m := range mutes {
		targets[i] = m.TargetAccountID
	}
	presentable, err := s.presentableAmong(ctx, &v, targets, false)
	if err != nil {
		return nil, err
	}
	for _, m := range mutes {
		if a, ok := presentable[m.TargetAccountID]; ok {
			actors, shown = append(actors, a), append(shown, m)
		}
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	for i, m := range shown {
		page.Items = append(page.Items, MutedAccount{Account: accounts[i], HideNotifications: m.HideNotifications, ExpiresAt: m.ExpiresAt})
	}
	if last >= 0 {
		page.NextCursor = encodeMuteCursor(mutes[last])
	}
	cache := cacheNoStore
	return ListMutes200JSONResponse{Body: page, Headers: ListMutes200ResponseHeaders{CacheControl: &cache}}, nil
}
