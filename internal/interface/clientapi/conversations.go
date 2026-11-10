// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
)

// Conversations reads and keeps the viewer's direct messages by conversation (FR-212,
// *apptimeline.Service). The methods that take an ID answer a merged conversation with the one
// it was merged into, and fail with apptimeline.ErrNotFound for a conversation that the viewer
// has no direct post in, which the handlers answer as one that does not exist (404).
type Conversations interface {
	Conversations(ctx context.Context, viewer appcontent.Viewer, q apptimeline.Query) (apptimeline.ConversationPage, error)
	Conversation(ctx context.Context, viewer appcontent.Viewer, id string) (apptimeline.Conversation, error)
	ReadConversation(ctx context.Context, viewer appcontent.Viewer, id string) (apptimeline.Conversation, error)
	HideConversation(ctx context.Context, viewer appcontent.Viewer, id string) error
	// StreamConversation is Conversation as the list shows it, false for one it leaves out.
	StreamConversation(ctx context.Context, viewer appcontent.Viewer, id string) (apptimeline.Conversation, bool, error)
}

// ListConversations serves GET /api/v1/conversations.
func (s *Server) ListConversations(ctx context.Context, req ListConversationsRequestObject) (ListConversationsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Conversations == nil {
		return ListConversationsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if p := s.plainPageProblem(ctx, req.Params.Cursor, req.Params.Limit); p != nil {
		return ListConversationsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q, err := s.timelineQuery(ctx, &v, req.Params.Cursor, req.Params.Limit, false)
	if err != nil {
		return nil, err
	}
	viewer := localContentViewer(v)
	page, err := s.o.Conversations.Conversations(ctx, viewer, q)
	if err != nil {
		return nil, err
	}
	items, err := s.presentConversations(ctx, &v, viewer, page.Conversations)
	if err != nil {
		return nil, err
	}
	body := ConversationPage{Items: items}
	if page.Next != "" {
		body.NextCursor = &page.Next
	}
	cache := cacheNoStore
	return ListConversations200JSONResponse{Body: body, Headers: ListConversations200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetConversation serves GET /api/v1/conversations/{id}.
func (s *Server) GetConversation(ctx context.Context, req GetConversationRequestObject) (GetConversationResponseObject, error) {
	res, p, err := s.conversationResponse(ctx, req.Id, func(viewer appcontent.Viewer) (apptimeline.Conversation, error) {
		return s.o.Conversations.Conversation(ctx, viewer, req.Id)
	})
	if err != nil || p != nil {
		if p != nil {
			return GetConversationdefaultApplicationProblemPlusJSONResponse(*p), nil
		}
		return nil, err
	}
	return GetConversation200JSONResponse{res}, nil
}

// MarkConversationRead serves POST /api/v1/conversations/{id}/read.
func (s *Server) MarkConversationRead(ctx context.Context, req MarkConversationReadRequestObject) (MarkConversationReadResponseObject, error) {
	res, p, err := s.conversationResponse(ctx, req.Id, func(viewer appcontent.Viewer) (apptimeline.Conversation, error) {
		return s.o.Conversations.ReadConversation(ctx, viewer, req.Id)
	})
	if err != nil || p != nil {
		if p != nil {
			return MarkConversationReaddefaultApplicationProblemPlusJSONResponse(*p), nil
		}
		return nil, err
	}
	return MarkConversationRead200JSONResponse{res}, nil
}

// HideConversation serves POST /api/v1/conversations/{id}/hide.
func (s *Server) HideConversation(ctx context.Context, req HideConversationRequestObject) (HideConversationResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Conversations == nil || !kernel.ValidID(req.Id) {
		return HideConversationdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	err = s.o.Conversations.HideConversation(ctx, localContentViewer(v), req.Id)
	if errors.Is(err, apptimeline.ErrNotFound) {
		return HideConversationdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, err
	}
	return HideConversation204Response{}, nil
}

// conversationResponse answers an operation on the conversation id with the conversation that
// get returns, or the problem of a conversation that does not exist for the viewer.
func (s *Server) conversationResponse(ctx context.Context, id string, get func(appcontent.Viewer) (apptimeline.Conversation, error)) (ConversationJSONResponse, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return ConversationJSONResponse{}, nil, err
	}
	if s.o.Conversations == nil || !kernel.ValidID(id) {
		p := s.notFoundResponse(ctx)
		return ConversationJSONResponse{}, &p, nil
	}
	viewer := localContentViewer(v)
	c, err := get(viewer)
	if errors.Is(err, apptimeline.ErrNotFound) {
		p := s.notFoundResponse(ctx)
		return ConversationJSONResponse{}, &p, nil
	}
	if err != nil {
		return ConversationJSONResponse{}, nil, err
	}
	items, err := s.presentConversations(ctx, &v, viewer, []apptimeline.Conversation{c})
	if err != nil {
		return ConversationJSONResponse{}, nil, err
	}
	if len(items) == 0 {
		p := s.notFoundResponse(ctx) // its last post went since it was read
		return ConversationJSONResponse{}, &p, nil
	}
	cache := cacheNoStore
	return ConversationJSONResponse{Body: items[0], Headers: ConversationResponseHeaders{CacheControl: &cache}}, nil, nil
}

// presentConversations presents the conversations in order: their last statuses together, and
// the accounts of all of them together, so that the queries do not grow with the conversations.
// A conversation whose last status is gone or hidden since it was read is left out.
func (s *Server) presentConversations(ctx context.Context, v *Viewer, viewer appcontent.Viewer, cs []apptimeline.Conversation) ([]Conversation, error) {
	if len(cs) == 0 {
		return []Conversation{}, nil
	}
	entries := make([]apptimeline.Entry, len(cs))
	var accountIDs []string
	for i, c := range cs {
		entries[i] = c.LastEntry
		for _, id := range c.AccountIDs {
			if !slices.Contains(accountIDs, id) {
				accountIDs = append(accountIDs, id)
			}
		}
	}
	statuses, err := s.statuses(ctx, viewer, noFilter, entryItems(entries))
	if err != nil {
		return nil, err
	}
	byStatus := make(map[string]Status, len(statuses))
	for _, st := range statuses {
		byStatus[st.Id] = st
	}
	var actors []appaccount.Actor
	if len(accountIDs) > 0 {
		if actors, err = s.presentableActors(ctx, v, accountIDs, true); err != nil {
			return nil, err
		}
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	byAccount := make(map[string]Account, len(accounts))
	for _, a := range accounts {
		byAccount[a.Id] = a
	}
	out := make([]Conversation, 0, len(cs))
	for _, c := range cs {
		st, ok := byStatus[c.LastEntry.StatusID]
		if !ok {
			continue
		}
		item := Conversation{Id: c.ID, LastStatus: st, Unread: c.Unread, Accounts: []Account{}}
		for _, id := range c.AccountIDs {
			if a, ok := byAccount[id]; ok {
				item.Accounts = append(item.Accounts, a)
			}
		}
		out = append(out, item)
	}
	return out, nil
}
