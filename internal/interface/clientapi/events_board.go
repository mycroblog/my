// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

// The board of an event (ADR-0161 §17; tag events, scopes read:events and write:events): the list
// of its posts and a new post. Its posts are statuses of the content module, which the readers of
// the board read, favourite, react to, and vote in by their IDs through the operations of
// statuses, and whose authors edit and delete them there; the content module decides who may.

// EventBoards lists and takes the posts of the boards of the events. The composition root
// implements it on *appcontent.Reader and *appcontent.Service.
type EventBoards interface {
	// Board returns a page of the posts of the board of an event as viewer sees them.
	Board(ctx context.Context, viewer appcontent.Viewer, gatheringID, before string, limit int) (appcontent.BoardPage, error)
	// PostOnBoard posts a status on the board of an event.
	PostOnBoard(ctx context.Context, in appcontent.BoardPostInput) (appcontent.Post, error)
}

// boardViews presents the statuses of a viewer's notifications, admitting the posts of the boards
// that the viewer reads. *appcontent.Reader implements it.
type boardViews interface {
	ViewsWithBoards(ctx context.Context, viewer appcontent.Viewer, ids []string) ([]appcontent.StatusView, error)
}

// The problem types of the boards of the events.
const (
	ProblemTypeEventBoardNotReader = "/api/v1/problems/event-board-not-reader"
	ProblemTypeEventBoardNotWriter = "/api/v1/problems/event-board-not-writer"
	ProblemTypeEventBoardClosed    = "/api/v1/problems/event-board-closed"
)

// boardProblem maps the errors of the boards; it reports false for any other error.
func (s *Server) boardProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appcontent.ErrNotBoardReader):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeEventBoardNotReader, l.ServerErrorsEventBoardNotReader(), ""), true
	case errors.Is(err, appcontent.ErrNotBoardWriter):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeEventBoardNotWriter, l.ServerErrorsEventBoardNotWriter(), ""), true
	case errors.Is(err, appcontent.ErrBoardClosed):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventBoardClosed, l.ServerErrorsEventBoardClosed(), ""), true
	}
	return problem{}, false
}

// eventBoards returns the boards and the viewer, or the 404 of a server without events.
func (s *Server) eventBoards(ctx context.Context) (EventBoards, Viewer, *problem, error) {
	_, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return nil, Viewer{}, p, err
	}
	if s.o.EventBoards == nil {
		p := s.notFoundResponse(ctx)
		return nil, Viewer{}, &p, nil
	}
	return s.o.EventBoards, v, nil, nil
}

// ListEventBoard serves GET /api/v1/events/{id}/board.
func (s *Server) ListEventBoard(ctx context.Context, req ListEventBoardRequestObject) (ListEventBoardResponseObject, error) {
	boards, v, p, err := s.eventBoards(ctx)
	if p != nil || err != nil {
		return respond[ListEventBoarddefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListEventBoarddefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	page, err := boards.Board(ctx, localContentViewer(v), req.Id, q.before, q.limit)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return ListEventBoarddefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	items, err := s.presentStatuses(ctx, page.Statuses)
	if err != nil {
		return nil, err
	}
	out := EventBoardPage{Items: items}
	if page.Next != "" {
		out.NextCursor = encodeCursor(page.Next)
	}
	return ListEventBoard200JSONResponse{Body: out, Headers: ListEventBoard200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// PostToEventBoard serves POST /api/v1/events/{id}/board.
func (s *Server) PostToEventBoard(ctx context.Context, req PostToEventBoardRequestObject) (PostToEventBoardResponseObject, error) {
	boards, v, p, err := s.eventBoards(ctx)
	if p != nil || err != nil {
		return respond[PostToEventBoarddefaultApplicationProblemPlusJSONResponse](p, err)
	}
	if p, h, limited := s.LimitProblem(ctx, PostLimit, v.AccountID); limited {
		return PostToEventBoarddefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	if p, err := s.refuseMoved(ctx, v.AccountID); err != nil {
		return nil, err
	} else if p != nil {
		return PostToEventBoarddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	body := req.Body
	post, err := boards.PostOnBoard(ctx, appcontent.BoardPostInput{
		TextInput: appcontent.TextInput{
			Text: body.Text, SpoilerText: deref(body.SpoilerText), Sensitive: deref(body.Sensitive),
			Language: deref(body.Language), Media: mediaInput(body.Media),
		},
		AuthorAccountID: v.AccountID, GatheringID: req.Id, InReplyToID: knownID(deref(body.InReplyToId)),
		Poll: pollInput(body.Poll), ClientID: v.ClientID, IdempotencyKey: deref(req.Params.IdempotencyKey),
	})
	if err != nil {
		if r, ok, err := s.rejectedProblem(ctx, err); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return PostToEventBoard422ApplicationProblemPlusJSONResponse{r}, nil
		}
		if p, ok := s.statusProblem(ctx, err); ok {
			return PostToEventBoarddefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	view, err := s.o.StatusReader.View(ctx, localContentViewer(v), post.Status.ID)
	var out Status
	if err == nil {
		out, err = s.presentStatus(ctx, view)
	}
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return PostToEventBoarddefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return PostToEventBoard201JSONResponse{Body: out, Headers: PostToEventBoard201ResponseHeaders{CacheControl: eventCache()}}, nil
}
