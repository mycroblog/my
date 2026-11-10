// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The reminders of the events and the events of the notifications (ADR-0161 §11, §12): how long
// before the start of an event that the user answered going or maybe it is reminded, under
// /preferences/event_reminders, and the event that an event_* notification carries.

// EventReminderSettings reads and changes the reminders of an account. *appgathering.Service
// implements it, next to Events.
type EventReminderSettings interface {
	Reminders(ctx context.Context, accountID string) (appgathering.Reminders, error)
	// SetReminders sets the reminders; nil restores the defaults.
	SetReminders(ctx context.Context, accountID string, minutes []int) (appgathering.Reminders, error)
}

// eventReminderSettings returns the reminders of the events, nil on a server without them.
func (s *Server) eventReminderSettings() EventReminderSettings {
	r, _ := s.o.Events.(EventReminderSettings)
	return r
}

func newEventReminders(r appgathering.Reminders) EventReminders {
	minutes := append([]int{}, r.Minutes...)
	return EventReminders{MinutesBefore: minutes, Defaults: r.Defaults}
}

// GetEventReminders implements getEventReminders.
func (s *Server) GetEventReminders(ctx context.Context, _ GetEventRemindersRequestObject) (GetEventRemindersResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	settings := s.eventReminderSettings()
	if settings == nil {
		body, headers := s.notFound(ctx)
		return GetEventRemindersdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	r, err := settings.Reminders(ctx, v.AccountID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the event reminders of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return GetEventReminders200JSONResponse{Body: newEventReminders(r), Headers: GetEventReminders200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdateEventReminders implements updateEventReminders.
func (s *Server) UpdateEventReminders(ctx context.Context, req UpdateEventRemindersRequestObject) (UpdateEventRemindersResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	settings := s.eventReminderSettings()
	if settings == nil {
		body, headers := s.notFound(ctx)
		return UpdateEventRemindersdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	var minutes []int
	if req.Body == nil || req.Body.Defaults == nil || !*req.Body.Defaults {
		minutes = []int{}
		if req.Body != nil && req.Body.MinutesBefore != nil {
			minutes = append(minutes, *req.Body.MinutesBefore...)
		}
	}
	r, err := settings.SetReminders(ctx, v.AccountID, minutes)
	if errors.Is(err, gathering.ErrInvalidReminders) {
		l := s.Localizer(ctx)
		body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("minutes_before"))
		return UpdateEventRemindersdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusUnprocessableEntity}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update the event reminders of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return UpdateEventReminders200JSONResponse{Body: newEventReminders(r), Headers: UpdateEventReminders200ResponseHeaders{CacheControl: &cache}}, nil
}

// notifiedEvent returns the event id of a notification as accountID sees it now, read once per
// list in seen; nil when the account no longer sees it or the server has no events.
func (s *Server) notifiedEvent(ctx context.Context, accountID, id string, seen map[string]*Event) (*Event, error) {
	if e, ok := seen[id]; ok {
		return e, nil
	}
	seen[id] = nil
	if s.o.Events == nil {
		return nil, nil
	}
	view, err := s.o.Events.Get(ctx, accountID, id)
	if errors.Is(err, appgathering.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	seen[id] = &e
	return &e, nil
}
