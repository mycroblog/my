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

// Problem types of lists (ADR-0122 §5, §2).
const (
	ProblemTypeTooManyLists       = "/api/v1/problems/too-many-lists"
	ProblemTypeTooManyListMembers = "/api/v1/problems/too-many-list-members"
	ProblemTypeNotFollowed        = "/api/v1/problems/not-followed"
)

// Lists keeps the viewer's lists of the accounts it follows and reads their timelines
// (*apptimeline.Service, ADR-0122). Every method fails with apptimeline.ErrNotFound for a list
// that the owner does not own, which the handlers answer as a list that does not exist (404).
type Lists interface {
	CreateList(ctx context.Context, owner, name string) (apptimeline.List, error)
	RenameList(ctx context.Context, owner, id, name string) (apptimeline.List, error)
	DeleteList(ctx context.Context, owner, id string) error
	List(ctx context.Context, owner, id string) (apptimeline.List, error)
	Lists(ctx context.Context, owner string) ([]apptimeline.List, error)
	ListsWithMember(ctx context.Context, owner, member string) ([]apptimeline.List, error)
	AddListMember(ctx context.Context, owner, id, member string) error
	RemoveListMember(ctx context.Context, owner, id, member string) error
	ListMembers(ctx context.Context, owner, id, after string, limit int) ([]string, error)
	ListTimeline(ctx context.Context, viewer appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Page, error)
}

// EntryLists tells the streams which lists of the viewer list a home timeline entry.
// *apptimeline.Service implements it.
type EntryLists interface {
	EntryLists(ctx context.Context, viewer appcontent.Viewer, e apptimeline.Entry) ([]string, error)
}

func newList(l apptimeline.List) List {
	return List{Id: l.ID, Name: l.Name, CreatedAt: l.CreatedAt, MembersCount: l.MembersCount}
}

func listCollection(ls []apptimeline.List) ListCollectionJSONResponse {
	items := make([]List, len(ls))
	for i, l := range ls {
		items[i] = newList(l)
	}
	cache := cacheNoStore
	return ListCollectionJSONResponse{Body: ListCollection{Items: items}, Headers: ListCollectionResponseHeaders{CacheControl: &cache}}
}

func listResponse(l apptimeline.List) ListJSONResponse {
	cache := cacheNoStore
	return ListJSONResponse{Body: newList(l), Headers: ListResponseHeaders{CacheControl: &cache}}
}

// listProblem returns the problem that answers err of the lists, false for an error that is
// none of theirs.
func (s *Server) listProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, apptimeline.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, timeline.ErrInvalidListName):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("name")), true
	case errors.Is(err, timeline.ErrTooManyLists):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTooManyLists, l.ServerErrorsListsTooMany(), ""), true
	case errors.Is(err, timeline.ErrTooManyListMembers):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTooManyListMembers, l.ServerErrorsListMembersTooMany(), ""), true
	case errors.Is(err, timeline.ErrNotFollowed):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeNotFollowed, l.ServerErrorsListMembersNotFollowed(), ""), true
	}
	return problem{}, false
}

// listsViewer returns the viewer of a request of the lists, or the 404 that answers it when the
// server keeps no lists.
func (s *Server) listsViewer(ctx context.Context) (Viewer, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return Viewer{}, nil, err
	}
	if s.o.Lists == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	return v, nil, nil
}

// ListLists serves GET /api/v1/lists.
func (s *Server) ListLists(ctx context.Context, _ ListListsRequestObject) (ListListsResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return ListListsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	ls, err := s.o.Lists.Lists(ctx, v.AccountID)
	if err != nil {
		return nil, err
	}
	return ListLists200JSONResponse{listCollection(ls)}, nil
}

// CreateList serves POST /api/v1/lists.
func (s *Server) CreateList(ctx context.Context, req CreateListRequestObject) (CreateListResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return CreateListdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	name := ""
	if req.Body != nil {
		name = req.Body.Name
	}
	l, err := s.o.Lists.CreateList(ctx, v.AccountID, name)
	if err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return CreateListdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return CreateList200JSONResponse{listResponse(l)}, nil
}

// GetList serves GET /api/v1/lists/{id}.
func (s *Server) GetList(ctx context.Context, req GetListRequestObject) (GetListResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return GetListdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	l, err := s.o.Lists.List(ctx, v.AccountID, req.Id)
	if err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return GetListdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return GetList200JSONResponse{listResponse(l)}, nil
}

// UpdateList serves PATCH /api/v1/lists/{id}.
func (s *Server) UpdateList(ctx context.Context, req UpdateListRequestObject) (UpdateListResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return UpdateListdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	name := ""
	if req.Body != nil {
		name = req.Body.Name
	}
	l, err := s.o.Lists.RenameList(ctx, v.AccountID, req.Id, name)
	if err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return UpdateListdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return UpdateList200JSONResponse{listResponse(l)}, nil
}

// DeleteList serves DELETE /api/v1/lists/{id}.
func (s *Server) DeleteList(ctx context.Context, req DeleteListRequestObject) (DeleteListResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return DeleteListdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err := s.o.Lists.DeleteList(ctx, v.AccountID, req.Id); err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return DeleteListdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return DeleteList204Response{}, nil
}

// ListListAccounts serves GET /api/v1/lists/{id}/accounts. A page is ordered by member ID, and
// its cursor is the base64url of the last member's ID.
func (s *Server) ListListAccounts(ctx context.Context, req ListListAccountsRequestObject) (ListListAccountsResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return ListListAccountsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListListAccountsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	ids, err := s.o.Lists.ListMembers(ctx, v.AccountID, req.Id, q.before, q.limit+1)
	if err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return ListListAccountsdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	ids, last := trim(ids, q.limit)
	accounts, err := s.presentAccounts(ctx, &v, ids, false)
	if err != nil {
		return nil, err
	}
	page := AccountPage{Accounts: accounts}
	if last >= 0 {
		page.NextCursor = encodeCursor(ids[last])
	}
	return ListListAccounts200JSONResponse{ownPageResponse(page)}, nil
}

// AddListAccount serves POST /api/v1/lists/{id}/accounts/{account_id}.
func (s *Server) AddListAccount(ctx context.Context, req AddListAccountRequestObject) (AddListAccountResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return AddListAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err := s.o.Lists.AddListMember(ctx, v.AccountID, req.Id, req.AccountId); err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return AddListAccountdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return AddListAccount204Response{}, nil
}

// RemoveListAccount serves DELETE /api/v1/lists/{id}/accounts/{account_id}.
func (s *Server) RemoveListAccount(ctx context.Context, req RemoveListAccountRequestObject) (RemoveListAccountResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return RemoveListAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err := s.o.Lists.RemoveListMember(ctx, v.AccountID, req.Id, req.AccountId); err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return RemoveListAccountdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return RemoveListAccount204Response{}, nil
}

// ListAccountLists serves GET /api/v1/accounts/{id}/lists: the viewer's lists that have the
// account. An account in none of them, or unknown, has an empty collection.
func (s *Server) ListAccountLists(ctx context.Context, req ListAccountListsRequestObject) (ListAccountListsResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return ListAccountListsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	ls, err := s.o.Lists.ListsWithMember(ctx, v.AccountID, req.Id)
	if err != nil {
		return nil, err
	}
	return ListAccountLists200JSONResponse{listCollection(ls)}, nil
}

// GetListTimeline serves GET /api/v1/timelines/list/{id}.
func (s *Server) GetListTimeline(ctx context.Context, req GetListTimelineRequestObject) (GetListTimelineResponseObject, error) {
	v, p, err := s.listsViewer(ctx)
	if err != nil {
		return nil, err
	} else if p != nil {
		return GetListTimelinedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if p := s.plainPageProblem(ctx, req.Params.Cursor, req.Params.Limit); p != nil {
		return GetListTimelinedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q, err := s.timelineQuery(ctx, &v, req.Params.Cursor, req.Params.Limit, true)
	if err != nil {
		return nil, err
	}
	page, err := s.o.Lists.ListTimeline(ctx, contentViewer(ctx), req.Id, q)
	if err != nil {
		if p, ok := s.listProblem(ctx, err); ok {
			return GetListTimelinedefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	res, err := s.statusPage(ctx, &v, entryItems(page.Entries), page.Next)
	return GetListTimeline200JSONResponse{res}, err
}
