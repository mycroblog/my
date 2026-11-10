// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// The moderation of the events (ADR-0161 §14): the target of a report of an event (reports.go),
// a moderator cancelling an event (tag admin-moderation, manage_reports), and the records of a
// reported event in the report's detail.

// EventModeration runs the moderation of the events: *appgathering.Service implements it.
type EventModeration interface {
	ReportTarget(ctx context.Context, accountID, id string) (appgathering.ReportTarget, error)
	CancelByModerator(ctx context.Context, in appgathering.ModeratorCancelInput) error
	EventRecords(ctx context.Context, id string) (appgathering.EventRecords, error)
}

// CancelEventAsModerator serves POST /api/v1/admin/events/{id}/cancel.
func (s *Server) CancelEventAsModerator(ctx context.Context, req CancelEventAsModeratorRequestObject) (CancelEventAsModeratorResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return CancelEventAsModeratordefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.EventModeration == nil {
		return CancelEventAsModeratordefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	in := appgathering.ModeratorCancelInput{GatheringID: req.Id, ByUserID: v.UserID, ByAccountID: v.AccountID}
	if req.Body != nil {
		in.Note = deref(req.Body.Reason)
	}
	err = s.o.EventModeration.CancelByModerator(ctx, in)
	var se *gathering.SettingsError
	switch {
	case errors.As(err, &se):
		l := s.Localizer(ctx)
		return CancelEventAsModeratordefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(),
				l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: gathering.MaxNoteLength, Name: "reason"}))), nil
	case err != nil:
		if p, ok := s.eventProblem(ctx, err); ok {
			return CancelEventAsModeratordefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: cancel event %s as a moderator: %w", req.Id, err)
	}
	return CancelEventAsModerator204Response{}, nil
}

// reportEvent presents the event of a report with its revisions for the moderators, nil for a
// report of no event or of an event that the server does not have.
func (s *Server) reportEvent(ctx context.Context, r moderation.Report) (*AdminReportEvent, error) {
	if r.Gathering == nil || s.o.EventModeration == nil {
		return nil, nil
	}
	rec, err := s.o.EventModeration.EventRecords(ctx, r.Gathering.GatheringID)
	if errors.Is(err, appgathering.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the event of report %s: %w", r.ID, err)
	}
	out := &AdminReportEvent{
		Id: rec.ID, OrganizerAccountId: rec.OrganizerAccountID, CreatedAt: rec.CreatedAt.UTC(), State: EventState(rec.Head.State),
		ReportedRevisionId: r.Gathering.RevisionID, Revisions: make([]AdminEventRevision, 0, len(rec.Revisions)),
	}
	// An event whose date poll confirmed no candidate has no date (ADR-0161 §7).
	if rec.Scheduled() {
		starts, ends := rec.Confirmation.Schedule.StartsAt.UTC(), rec.Confirmation.Schedule.EndsAt.UTC()
		out.StartsAt, out.EndsAt = &starts, &ends
	}
	if c := rec.Cancellation; c != nil {
		out.Cancellation = &AdminEventCancellation{Reason: EventCancelReason(c.Reason), CancelledAt: c.CancelledAt.UTC()}
		if c.CancelledByAccountID != "" {
			by := c.CancelledByAccountID
			out.Cancellation.CancelledByAccountId = &by
		}
		if c.Note != "" {
			note := c.Note
			out.Cancellation.Note = &note
		}
	}
	for _, rv := range rec.Revisions {
		st := rv.Settings
		places := make([]EventPlace, 0, len(st.Places))
		for _, p := range st.Places {
			places = append(places, newEventPlace(p))
		}
		out.Revisions = append(out.Revisions, AdminEventRevision{
			Id: rv.ID, Title: st.Title, Description: st.Description, Visibility: EventVisibility(st.Visibility),
			TimeZone: st.TimeZone, PlaceDisclosure: EventPlaceDisclosure(st.PlaceDisclosure), Capacity: st.Capacity,
			Places: places, RevisedByAccountId: rv.RevisedByAccountID, RevisedAt: rv.RevisedAt.UTC(),
		})
	}
	return out, nil
}
