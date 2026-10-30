// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// The server-wide announcements (FR-726): the users' list with their reads, and the admin
// operations that write, publish, and withdraw them (manage_announcements, ADR-0058 server
// administration).

// Announcements runs the use cases of the announcements. *appinstance.Service implements it.
type Announcements interface {
	ShownAnnouncements(ctx context.Context, userID string) ([]appinstance.ReaderAnnouncement, error)
	ShownAnnouncement(ctx context.Context, userID, id string) (appinstance.ReaderAnnouncement, bool, error)
	PastAnnouncements(ctx context.Context, userID string, after appinstance.AnnouncementCursor, limit int) ([]appinstance.ReaderAnnouncement, bool, error)
	MarkAnnouncementRead(ctx context.Context, userID, id string) (appinstance.ReaderAnnouncement, error)

	Announcements(ctx context.Context) ([]instance.Announcement, error)
	Announcement(ctx context.Context, id string) (instance.Announcement, error)
	CreateAnnouncement(ctx context.Context, in appinstance.CreateAnnouncementInput) (instance.Announcement, error)
	UpdateAnnouncement(ctx context.Context, in appinstance.UpdateAnnouncementInput) (instance.Announcement, error)
	PublishAnnouncement(ctx context.Context, id, byUserID string) (instance.Announcement, error)
	WithdrawAnnouncement(ctx context.Context, id, byUserID string) (instance.Announcement, error)
	RenderAnnouncement(body string) string
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func newAnnouncement(a appinstance.ReaderAnnouncement) Announcement {
	return Announcement{
		Id: a.ID, Title: a.Revision.Title, Content: a.HTML, PublishedAt: a.ShowsFrom().UTC(), EndsAt: optionalTime(a.Revision.EndsAt),
		UpdatedAt: a.Revision.RevisedAt.UTC(), Read: a.Read, Ended: a.Ended,
	}
}

// ListAnnouncements serves GET /api/v1/announcements.
func (s *Server) ListAnnouncements(ctx context.Context, _ ListAnnouncementsRequestObject) (ListAnnouncementsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	list := AnnouncementList{Items: []Announcement{}}
	if s.o.Announcements != nil {
		shown, err := s.o.Announcements.ShownAnnouncements(ctx, v.UserID)
		if err != nil {
			return nil, fmt.Errorf("clientapi: list announcements: %w", err)
		}
		for _, a := range shown {
			list.Items = append(list.Items, newAnnouncement(a))
		}
	}
	cache := cacheNoStore
	return ListAnnouncements200JSONResponse{Body: list, Headers: ListAnnouncements200ResponseHeaders{CacheControl: &cache}}, nil
}

// ListAnnouncementHistory serves GET /api/v1/announcements/history. The cursor is the
// ShowsFrom and the ID of the last announcement of the previous page.
func (s *Server) ListAnnouncementHistory(ctx context.Context, req ListAnnouncementHistoryRequestObject) (ListAnnouncementHistoryResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parseOpaquePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListAnnouncementHistorydefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	var after appinstance.AnnouncementCursor
	if req.Params.Cursor != nil {
		at, id, ok := decodeTimeCursor(*req.Params.Cursor)
		if !ok || !kernel.ValidID(id) {
			return ListAnnouncementHistorydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
		after = appinstance.AnnouncementCursor{ShowsFrom: at, ID: id}
	}
	page := AnnouncementPage{Items: []Announcement{}}
	if s.o.Announcements != nil {
		list, more, err := s.o.Announcements.PastAnnouncements(ctx, v.UserID, after, q.limit)
		if err != nil {
			return nil, fmt.Errorf("clientapi: list the past announcements: %w", err)
		}
		for _, a := range list {
			page.Items = append(page.Items, newAnnouncement(a))
		}
		if more {
			last := list[len(list)-1]
			page.NextCursor = encodeTimeCursor(last.ShowsFrom(), last.ID)
		}
	}
	cache := cacheNoStore
	return ListAnnouncementHistory200JSONResponse{Body: page, Headers: ListAnnouncementHistory200ResponseHeaders{CacheControl: &cache}}, nil
}

// MarkAnnouncementRead serves POST /api/v1/announcements/{id}/read.
func (s *Server) MarkAnnouncementRead(ctx context.Context, req MarkAnnouncementReadRequestObject) (MarkAnnouncementReadResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Announcements == nil {
		return MarkAnnouncementReaddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	a, err := s.o.Announcements.MarkAnnouncementRead(ctx, v.UserID, req.Id)
	if errors.Is(err, appinstance.ErrNotFound) {
		return MarkAnnouncementReaddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: mark announcement %s read: %w", req.Id, err)
	}
	cache := cacheNoStore
	return MarkAnnouncementRead200JSONResponse{Body: newAnnouncement(a), Headers: MarkAnnouncementRead200ResponseHeaders{CacheControl: &cache}}, nil
}

func (s *Server) newAdminAnnouncement(a instance.Announcement) AdminAnnouncement {
	return AdminAnnouncement{
		Id: a.ID, Title: a.Revision.Title, Text: a.Revision.Body, Content: s.o.Announcements.RenderAnnouncement(a.Revision.Body),
		State: AdminAnnouncementState(a.State), StartsAt: optionalTime(a.Revision.StartsAt), EndsAt: optionalTime(a.Revision.EndsAt),
		PublishedAt: optionalTime(a.PublishedAt), CreatedAt: a.CreatedAt.UTC(), UpdatedAt: a.Revision.RevisedAt.UTC(),
	}
}

func (s *Server) adminAnnouncementResponse(a instance.Announcement) AdminAnnouncementJSONResponse {
	cache := cacheNoStore
	return AdminAnnouncementJSONResponse{Body: s.newAdminAnnouncement(a), Headers: AdminAnnouncementResponseHeaders{CacheControl: &cache}}
}

// requireAnnouncements checks the manage_announcements permission and that the use cases are
// configured; it returns the problem to answer, or nil to go on.
func (s *Server) requireAnnouncements(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAnnouncements); err != nil || p != nil {
		return p, err
	}
	if s.o.Announcements == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// announcementProblem maps the errors of the announcement use cases to problems: 404 for an
// unknown announcement, 409 for a withdrawn one, and 422 for bad input. It reports false for any
// other error.
func (s *Server) announcementProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appinstance.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, instance.ErrAnnouncementWithdrawn):
		return s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), ""), true
	case errors.Is(err, instance.ErrAnnouncementTitle):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("title")), true
	case errors.Is(err, instance.ErrAnnouncementWindow):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("ends_at")), true
	case errors.Is(err, instance.ErrAnnouncementTooLong):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(),
			l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: instance.MaxAnnouncementBodyLength, Name: "text"})), true
	case errors.Is(err, instance.ErrInvalidValue):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("text")), true
	}
	return problem{}, false
}

// ListAdminAnnouncements serves GET /api/v1/admin/announcements.
func (s *Server) ListAdminAnnouncements(ctx context.Context, _ ListAdminAnnouncementsRequestObject) (ListAdminAnnouncementsResponseObject, error) {
	if p, err := s.requireAnnouncements(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListAdminAnnouncementsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	all, err := s.o.Announcements.Announcements(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list announcements: %w", err)
	}
	list := AdminAnnouncementList{Items: make([]AdminAnnouncement, 0, len(all))}
	for _, a := range all {
		list.Items = append(list.Items, s.newAdminAnnouncement(a))
	}
	cache := cacheNoStore
	return ListAdminAnnouncements200JSONResponse{Body: list, Headers: ListAdminAnnouncements200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateAdminAnnouncement serves POST /api/v1/admin/announcements.
func (s *Server) CreateAdminAnnouncement(ctx context.Context, req CreateAdminAnnouncementRequestObject) (CreateAdminAnnouncementResponseObject, error) {
	if p, err := s.requireAnnouncements(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return CreateAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return CreateAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "text")), nil
	}
	a, err := s.o.Announcements.CreateAnnouncement(ctx, appinstance.CreateAnnouncementInput{
		AnnouncementInput: appinstance.AnnouncementInput{
			Title: req.Body.Title, Body: req.Body.Text, StartsAt: timeOrZero(req.Body.StartsAt), EndsAt: timeOrZero(req.Body.EndsAt),
		},
		Publish: req.Body.Publish != nil && *req.Body.Publish, CreatedByUserID: v.UserID,
	})
	if p, ok := s.announcementProblem(ctx, err); ok {
		return CreateAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create an announcement: %w", err)
	}
	return CreateAdminAnnouncement200JSONResponse{s.adminAnnouncementResponse(a)}, nil
}

// GetAdminAnnouncement serves GET /api/v1/admin/announcements/{id}.
func (s *Server) GetAdminAnnouncement(ctx context.Context, req GetAdminAnnouncementRequestObject) (GetAdminAnnouncementResponseObject, error) {
	if p, err := s.requireAnnouncements(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return GetAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	a, err := s.o.Announcements.Announcement(ctx, req.Id)
	if p, ok := s.announcementProblem(ctx, err); ok {
		return GetAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: announcement %s: %w", req.Id, err)
	}
	return GetAdminAnnouncement200JSONResponse{s.adminAnnouncementResponse(a)}, nil
}

// UpdateAdminAnnouncement serves PUT /api/v1/admin/announcements/{id}.
func (s *Server) UpdateAdminAnnouncement(ctx context.Context, req UpdateAdminAnnouncementRequestObject) (UpdateAdminAnnouncementResponseObject, error) {
	if p, err := s.requireAnnouncements(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return UpdateAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return UpdateAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "text")), nil
	}
	a, err := s.o.Announcements.UpdateAnnouncement(ctx, appinstance.UpdateAnnouncementInput{
		AnnouncementID: req.Id,
		AnnouncementInput: appinstance.AnnouncementInput{
			Title: req.Body.Title, Body: req.Body.Text, StartsAt: timeOrZero(req.Body.StartsAt), EndsAt: timeOrZero(req.Body.EndsAt),
		},
		RevisedByUserID: v.UserID,
	})
	if p, ok := s.announcementProblem(ctx, err); ok {
		return UpdateAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: revise announcement %s: %w", req.Id, err)
	}
	return UpdateAdminAnnouncement200JSONResponse{s.adminAnnouncementResponse(a)}, nil
}

// PublishAdminAnnouncement serves POST /api/v1/admin/announcements/{id}/publish.
func (s *Server) PublishAdminAnnouncement(ctx context.Context, req PublishAdminAnnouncementRequestObject) (PublishAdminAnnouncementResponseObject, error) {
	if p, err := s.requireAnnouncements(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return PublishAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	a, err := s.o.Announcements.PublishAnnouncement(ctx, req.Id, v.UserID)
	if p, ok := s.announcementProblem(ctx, err); ok {
		return PublishAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: publish announcement %s: %w", req.Id, err)
	}
	return PublishAdminAnnouncement200JSONResponse{s.adminAnnouncementResponse(a)}, nil
}

// WithdrawAdminAnnouncement serves POST /api/v1/admin/announcements/{id}/withdraw.
func (s *Server) WithdrawAdminAnnouncement(ctx context.Context, req WithdrawAdminAnnouncementRequestObject) (WithdrawAdminAnnouncementResponseObject, error) {
	if p, err := s.requireAnnouncements(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return WithdrawAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	a, err := s.o.Announcements.WithdrawAnnouncement(ctx, req.Id, v.UserID)
	if p, ok := s.announcementProblem(ctx, err); ok {
		return WithdrawAdminAnnouncementdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: withdraw announcement %s: %w", req.Id, err)
	}
	return WithdrawAdminAnnouncement200JSONResponse{s.adminAnnouncementResponse(a)}, nil
}
