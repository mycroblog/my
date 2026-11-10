// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The events of the local users (ADR-0161 §16; tag events, scopes read:events and write:events):
// the list and the events, creating, changing, and cancelling them, the answers, the attendees
// and their approval, the invitations, and the co-organizers. The places that the viewer does
// not see never reach the response (appgathering.View).

// The problem types of the events.
const (
	ProblemTypeEventNotEligible         = "/api/v1/problems/event-not-eligible"
	ProblemTypeEventNotOrganizer        = "/api/v1/problems/event-not-organizer"
	ProblemTypeEventOrganizerOnly       = "/api/v1/problems/event-organizer-only"
	ProblemTypeEventClosed              = "/api/v1/problems/event-closed"
	ProblemTypeEventFull                = "/api/v1/problems/event-full"
	ProblemTypeEventVoiceUnavailable    = "/api/v1/problems/event-voice-unavailable"
	ProblemTypeEventAlreadyInvited      = "/api/v1/problems/event-already-invited"
	ProblemTypeEventInvalidInvitee      = "/api/v1/problems/event-invalid-invitee"
	ProblemTypeEventInvalidCoorganizer  = "/api/v1/problems/event-invalid-coorganizer"
	ProblemTypeEventTooManyCoorganizers = "/api/v1/problems/event-too-many-coorganizers"
	ProblemTypeEventNotAttending        = "/api/v1/problems/event-not-attending"
)

// Events runs the events: *appgathering.Service implements it.
type Events interface {
	EventPolls
	Create(ctx context.Context, accountID string, in appgathering.CreateInput) (appgathering.View, error)
	Get(ctx context.Context, accountID, id string) (appgathering.View, error)
	List(ctx context.Context, accountID string, q appgathering.ListQuery) ([]appgathering.View, *appgathering.ListKey, error)
	Update(ctx context.Context, accountID, id string, p appgathering.Patch) (appgathering.View, error)
	Cancel(ctx context.Context, accountID, id, note string) (appgathering.View, error)
	Answer(ctx context.Context, accountID, id string, answer gathering.Answer) (appgathering.View, error)
	Attendees(ctx context.Context, accountID, id, afterRSVPID string, limit int) ([]appgathering.Attendee, bool, error)
	Approve(ctx context.Context, byAccountID, id, accountID string) (appgathering.Attendee, error)
	RevokeApproval(ctx context.Context, byAccountID, id, accountID string) (appgathering.Attendee, error)
	Invite(ctx context.Context, byAccountID, id, accountID string) (appgathering.Invitation, error)
	RevokeInvitation(ctx context.Context, byAccountID, id, accountID string) error
	Invitations(ctx context.Context, byAccountID, id, afterID string, limit int) ([]appgathering.Invitation, bool, error)
	AssignCoorganizer(ctx context.Context, byAccountID, id, accountID string) (appgathering.View, error)
	RemoveCoorganizer(ctx context.Context, byAccountID, id, accountID string) (appgathering.View, error)
}

// events returns the events and the viewer, or the 404 of a server without them.
func (s *Server) events(ctx context.Context) (Events, Viewer, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, Viewer{}, nil, err
	}
	if s.o.Events == nil {
		p := s.notFoundResponse(ctx)
		return nil, Viewer{}, &p, nil
	}
	return s.o.Events, v, nil, nil
}

// eventProblem maps the errors of the event use cases; it reports false for any other error.
func (s *Server) eventProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	var se *gathering.SettingsError
	var limited *kernel.RateLimitedError
	switch {
	case errors.Is(err, appgathering.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.As(err, &se):
		return s.unprocessable(ctx, string(se.Field)), true
	case errors.Is(err, appgathering.ErrInvalidAnswer):
		return s.unprocessable(ctx, "answer"), true
	case errors.As(err, &limited):
		retry := seconds(limited.RetryAfter)
		p := s.problem(ctx, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(retry))
		p.Headers.RetryAfter = &retry
		return p, true
	case errors.Is(err, appgathering.ErrNotEligible):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeEventNotEligible, l.ServerErrorsEventNotEligible(), ""), true
	case errors.Is(err, appgathering.ErrNotOrganizer):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeEventNotOrganizer, l.ServerErrorsEventNotOrganizer(), ""), true
	case errors.Is(err, appgathering.ErrOrganizerOnly):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeEventOrganizerOnly, l.ServerErrorsEventOrganizerOnly(), ""), true
	case errors.Is(err, appgathering.ErrClosed):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventClosed, l.ServerErrorsEventClosed(), ""), true
	case errors.Is(err, appgathering.ErrFull):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventFull, l.ServerErrorsEventFull(), ""), true
	case errors.Is(err, appgathering.ErrVoiceUnavailable):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeEventVoiceUnavailable, l.ServerErrorsEventVoiceUnavailable(), ""), true
	case errors.Is(err, appgathering.ErrAlreadyInvited):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventAlreadyInvited, l.ServerErrorsEventAlreadyInvited(), ""), true
	case errors.Is(err, appgathering.ErrInvalidInvitee):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeEventInvalidInvitee, l.ServerErrorsEventInvalidInvitee(), ""), true
	case errors.Is(err, appgathering.ErrInvalidCoorganizer):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeEventInvalidCoorganizer, l.ServerErrorsEventInvalidCoorganizer(), ""), true
	case errors.Is(err, appgathering.ErrTooManyCoorganizers):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventTooManyCoorganizers, l.ServerErrorsEventTooManyCoorganizers(), ""), true
	case errors.Is(err, appgathering.ErrNotAttending):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeEventNotAttending, l.ServerErrorsEventNotAttending(), ""), true
	}
	return s.eventPollProblem(ctx, err)
}

// eventFail returns the problem of err, or err itself for an unexpected error.
func (s *Server) eventFail(ctx context.Context, err error) (*problem, error) {
	if p, ok := s.eventProblem(ctx, err); ok {
		return &p, nil
	}
	return nil, err
}

func eventCache() *string {
	c := cacheNoStore
	return &c
}

// accountsOf presents the accounts of ids, read together, by ID; an account that is gone is
// left out.
func (s *Server) accountsOf(ctx context.Context, ids []string) (map[string]Account, error) {
	var unique []string
	for _, id := range ids {
		if !slices.Contains(unique, id) {
			unique = append(unique, id)
		}
	}
	found, err := s.o.Accounts.FindByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	actors := make([]appaccount.Actor, 0, len(unique))
	for _, id := range unique {
		if a, ok := found[id]; ok {
			actors = append(actors, a)
		}
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Account, len(actors))
	for i, a := range actors {
		out[a.Account.ID] = accounts[i]
	}
	return out, nil
}

// newEvents presents views, reading the accounts of their organizers and co-organizers together.
func (s *Server) newEvents(ctx context.Context, views []appgathering.View) ([]Event, error) {
	var ids []string
	for _, v := range views {
		ids = append(ids, v.OrganizerAccountID)
		for _, c := range v.Coorganizers {
			ids = append(ids, c.AccountID)
		}
	}
	accounts, err := s.accountsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(views))
	for _, v := range views {
		st := v.Revision.Settings
		e := Event{
			Id: v.ID, Organizer: accounts[v.OrganizerAccountID], Coorganizers: make([]Account, 0, len(v.Coorganizers)),
			Title: st.Title, Description: st.Description, Visibility: EventVisibility(st.Visibility), TimeZone: st.TimeZone,
			PlaceDisclosure: EventPlaceDisclosure(st.PlaceDisclosure), Capacity: st.Capacity,
			Places: make([]EventPlace, 0, len(v.Places)), HiddenPlaces: make([]EventPlaceKind, 0, len(v.HiddenPlaces)),
			StartsAt: v.Head.Schedule.StartsAt.UTC(), EndsAt: v.Head.Schedule.EndsAt.UTC(), State: EventState(v.Head.State),
			GoingCount: v.Head.GoingCount, CreatedAt: v.CreatedAt.UTC(), RevisedAt: v.Revision.RevisedAt.UTC(), RevisionCount: v.Revisions,
			Viewer: EventViewer{
				Organizer: v.Viewer.Organizer, Coorganizer: v.Viewer.Coorganizer, Invited: v.Viewer.Invited, Approved: v.Viewer.Approved,
			},
		}
		for _, c := range v.Coorganizers {
			if a, ok := accounts[c.AccountID]; ok {
				e.Coorganizers = append(e.Coorganizers, a)
			}
		}
		for _, p := range v.Places {
			place := newEventPlace(p)
			if p.Kind == gathering.PlaceVoice && v.Room != nil {
				// The room of the current date, once it opened or failed to (ADR-0161 §10).
				if v.Room.RoomID != "" {
					place.RoomId = &v.Room.RoomID
				}
				if v.Room.Unavailable != "" {
					r := EventRoomUnavailableReason(v.Room.Unavailable)
					place.RoomUnavailable = &r
				}
			}
			e.Places = append(e.Places, place)
		}
		for _, k := range v.HiddenPlaces {
			e.HiddenPlaces = append(e.HiddenPlaces, EventPlaceKind(k))
		}
		if v.Viewer.Answer != "" {
			a := EventAnswer(v.Viewer.Answer)
			e.Viewer.Answer = &a
		}
		if c := v.Cancellation; c != nil {
			e.Cancellation = &EventCancellation{Reason: EventCancelReason(c.Reason), CancelledAt: c.CancelledAt.UTC()}
			if c.Note != "" {
				e.Cancellation.Note = &c.Note
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *Server) newEvent(ctx context.Context, v appgathering.View) (Event, error) {
	out, err := s.newEvents(ctx, []appgathering.View{v})
	if err != nil {
		return Event{}, err
	}
	return out[0], nil
}

// newEventPlace presents a place that the viewer sees, leaving out the fields its kind lacks.
func newEventPlace(p gathering.Place) EventPlace {
	out := EventPlace{Kind: EventPlaceKind(p.Kind)}
	opt := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	out.Name, out.Address, out.Url = opt(p.Name), opt(p.Address), opt(p.URL)
	if p.Speakers != "" {
		sp := VoiceRoomSpeakers(p.Speakers)
		out.Speakers = &sp
	}
	return out
}

// eventPlaces returns the places of a request.
func eventPlaces(in []EventPlace) []gathering.Place {
	out := make([]gathering.Place, len(in))
	for i, p := range in {
		out[i] = gathering.Place{Kind: gathering.PlaceKind(p.Kind), Name: deref(p.Name), Address: deref(p.Address), URL: deref(p.Url)}
		if p.Speakers != nil {
			out[i].Speakers = gathering.Speakers(*p.Speakers)
		}
	}
	return out
}

// encodeEventCursor writes the position of an event in the list as a cursor.
func encodeEventCursor(k appgathering.ListKey) *Cursor {
	return encodeCursor(strconv.FormatInt(k.StartsAt.UnixMilli(), 10) + ":" + k.GatheringID)
}

// decodeEventCursor reads the position that encodeEventCursor wrote.
func decodeEventCursor(s string) (appgathering.ListKey, bool) {
	ms, id, ok := strings.Cut(s, ":")
	if !ok || !kernel.ValidID(id) {
		return appgathering.ListKey{}, false
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil || n < 0 {
		return appgathering.ListKey{}, false
	}
	return appgathering.ListKey{StartsAt: time.UnixMilli(n).UTC(), GatheringID: id}, true
}

// ListEvents serves GET /api/v1/events.
func (s *Server) ListEvents(ctx context.Context, req ListEventsRequestObject) (ListEventsResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[ListEventsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePageOf(ctx, req.Params.Cursor, req.Params.Limit, func(c string) bool { _, ok := decodeEventCursor(c); return ok })
	if q.problem != nil {
		return ListEventsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	lq := appgathering.ListQuery{Limit: q.limit, OrganizerID: deref(req.Params.OrganizerId)}
	if q.before != "" {
		k, _ := decodeEventCursor(q.before)
		lq.After = &k
	}
	views, next, err := events.List(ctx, v.AccountID, lq)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[ListEventsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	items, err := s.newEvents(ctx, views)
	if err != nil {
		return nil, err
	}
	out := EventPage{Items: items}
	if next != nil {
		out.NextCursor = encodeEventCursor(*next)
	}
	return ListEvents200JSONResponse{Body: out, Headers: ListEvents200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// CreateEvent serves POST /api/v1/events.
func (s *Server) CreateEvent(ctx context.Context, req CreateEventRequestObject) (CreateEventResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[CreateEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	b := req.Body
	in := appgathering.CreateInput{
		Settings:   gathering.Settings{Title: b.Title, Description: deref(b.Description), TimeZone: deref(b.TimeZone), Capacity: deref(b.Capacity)},
		Schedule:   gathering.Schedule{StartsAt: deref(b.StartsAt), EndsAt: deref(b.EndsAt)},
		Candidates: eventCandidates(b.Candidates),
	}
	if b.Visibility != nil {
		in.Settings.Visibility = gathering.Visibility(*b.Visibility)
	}
	if b.PlaceDisclosure != nil {
		in.Settings.PlaceDisclosure = gathering.PlaceDisclosure(*b.PlaceDisclosure)
	}
	if b.Places != nil {
		in.Settings.Places = eventPlaces(*b.Places)
	}
	view, err := events.Create(ctx, v.AccountID, in)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[CreateEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return CreateEvent201JSONResponse{Body: out, Headers: CreateEvent201ResponseHeaders{CacheControl: eventCache()}}, nil
}

// GetEvent serves GET /api/v1/events/{id}.
func (s *Server) GetEvent(ctx context.Context, req GetEventRequestObject) (GetEventResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[GetEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	view, err := events.Get(ctx, v.AccountID, req.Id)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[GetEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return GetEvent200JSONResponse{Body: out, Headers: GetEvent200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// UpdateEvent serves PATCH /api/v1/events/{id}.
func (s *Server) UpdateEvent(ctx context.Context, req UpdateEventRequestObject) (UpdateEventResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[UpdateEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	b := req.Body
	patch := appgathering.Patch{Title: b.Title, Description: b.Description, TimeZone: b.TimeZone, Capacity: b.Capacity}
	if b.Visibility != nil {
		vis := gathering.Visibility(*b.Visibility)
		patch.Visibility = &vis
	}
	if b.PlaceDisclosure != nil {
		d := gathering.PlaceDisclosure(*b.PlaceDisclosure)
		patch.PlaceDisclosure = &d
	}
	if b.Places != nil {
		places := eventPlaces(*b.Places)
		patch.Places = &places
	}
	switch {
	case b.StartsAt != nil && b.EndsAt != nil:
		patch.Schedule = &gathering.Schedule{StartsAt: *b.StartsAt, EndsAt: *b.EndsAt}
	case b.StartsAt != nil:
		p := s.unprocessable(ctx, "ends_at")
		return UpdateEventdefaultApplicationProblemPlusJSONResponse(p), nil
	case b.EndsAt != nil:
		p := s.unprocessable(ctx, "starts_at")
		return UpdateEventdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	view, err := events.Update(ctx, v.AccountID, req.Id, patch)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[UpdateEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return UpdateEvent200JSONResponse{Body: out, Headers: UpdateEvent200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// CancelEvent serves POST /api/v1/events/{id}/cancel.
func (s *Server) CancelEvent(ctx context.Context, req CancelEventRequestObject) (CancelEventResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[CancelEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	var note string
	if req.Body != nil {
		note = deref(req.Body.Note)
	}
	view, err := events.Cancel(ctx, v.AccountID, req.Id, note)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[CancelEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return CancelEvent200JSONResponse{Body: out, Headers: CancelEvent200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// AnswerEvent serves PUT /api/v1/events/{id}/rsvp.
func (s *Server) AnswerEvent(ctx context.Context, req AnswerEventRequestObject) (AnswerEventResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[AnswerEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	view, err := events.Answer(ctx, v.AccountID, req.Id, gathering.Answer(req.Body.Answer))
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[AnswerEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return AnswerEvent200JSONResponse{Body: out, Headers: AnswerEvent200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// newEventAttendees presents attendees, leaving out the accounts that are gone.
func (s *Server) newEventAttendees(ctx context.Context, as []appgathering.Attendee) ([]EventAttendee, error) {
	ids := make([]string, len(as))
	for i, a := range as {
		ids[i] = a.AccountID
	}
	accounts, err := s.accountsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]EventAttendee, 0, len(as))
	for _, a := range as {
		if acct, ok := accounts[a.AccountID]; ok {
			out = append(out, EventAttendee{Account: acct, Answer: EventAnswer(a.Answer), Approved: a.Approved})
		}
	}
	return out, nil
}

// ListEventAttendees serves GET /api/v1/events/{id}/attendees.
func (s *Server) ListEventAttendees(ctx context.Context, req ListEventAttendeesRequestObject) (ListEventAttendeesResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[ListEventAttendeesdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListEventAttendeesdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	as, more, err := events.Attendees(ctx, v.AccountID, req.Id, q.before, q.limit)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[ListEventAttendeesdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	items, err := s.newEventAttendees(ctx, as)
	if err != nil {
		return nil, err
	}
	out := EventAttendeePage{Items: items}
	if more {
		out.NextCursor = encodeCursor(as[len(as)-1].RSVPID)
	}
	return ListEventAttendees200JSONResponse{Body: out, Headers: ListEventAttendees200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// eventAttendee presents one attendee.
func (s *Server) eventAttendee(ctx context.Context, a appgathering.Attendee) (EventAttendee, error) {
	items, err := s.newEventAttendees(ctx, []appgathering.Attendee{a})
	if err != nil {
		return EventAttendee{}, err
	}
	if len(items) == 0 {
		return EventAttendee{}, appgathering.ErrNotFound
	}
	return items[0], nil
}

// ApproveEventAttendee serves PUT /api/v1/events/{id}/attendees/{account_id}/approval.
func (s *Server) ApproveEventAttendee(ctx context.Context, req ApproveEventAttendeeRequestObject) (ApproveEventAttendeeResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[ApproveEventAttendeedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	a, err := events.Approve(ctx, v.AccountID, req.Id, req.AccountId)
	if err == nil {
		var out EventAttendee
		if out, err = s.eventAttendee(ctx, a); err == nil {
			return ApproveEventAttendee200JSONResponse{Body: out, Headers: ApproveEventAttendee200ResponseHeaders{CacheControl: eventCache()}}, nil
		}
	}
	p, err = s.eventFail(ctx, err)
	return respond[ApproveEventAttendeedefaultApplicationProblemPlusJSONResponse](p, err)
}

// RevokeEventAttendeeApproval serves DELETE /api/v1/events/{id}/attendees/{account_id}/approval.
func (s *Server) RevokeEventAttendeeApproval(ctx context.Context, req RevokeEventAttendeeApprovalRequestObject) (RevokeEventAttendeeApprovalResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[RevokeEventAttendeeApprovaldefaultApplicationProblemPlusJSONResponse](p, err)
	}
	a, err := events.RevokeApproval(ctx, v.AccountID, req.Id, req.AccountId)
	if err == nil {
		var out EventAttendee
		if out, err = s.eventAttendee(ctx, a); err == nil {
			return RevokeEventAttendeeApproval200JSONResponse{Body: out, Headers: RevokeEventAttendeeApproval200ResponseHeaders{CacheControl: eventCache()}}, nil
		}
	}
	p, err = s.eventFail(ctx, err)
	return respond[RevokeEventAttendeeApprovaldefaultApplicationProblemPlusJSONResponse](p, err)
}

// newEventInvitations presents invitations, leaving out the accounts that are gone.
func (s *Server) newEventInvitations(ctx context.Context, invs []appgathering.Invitation) ([]EventInvitation, error) {
	ids := make([]string, len(invs))
	for i, inv := range invs {
		ids[i] = inv.InviteeAccountID
	}
	accounts, err := s.accountsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]EventInvitation, 0, len(invs))
	for _, inv := range invs {
		if a, ok := accounts[inv.InviteeAccountID]; ok {
			out = append(out, EventInvitation{Id: inv.ID, Account: a, InvitedById: inv.InvitedByAccountID, InvitedAt: inv.InvitedAt.UTC()})
		}
	}
	return out, nil
}

// ListEventInvitations serves GET /api/v1/events/{id}/invitations.
func (s *Server) ListEventInvitations(ctx context.Context, req ListEventInvitationsRequestObject) (ListEventInvitationsResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[ListEventInvitationsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListEventInvitationsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	invs, more, err := events.Invitations(ctx, v.AccountID, req.Id, q.before, q.limit)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[ListEventInvitationsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	items, err := s.newEventInvitations(ctx, invs)
	if err != nil {
		return nil, err
	}
	out := EventInvitationPage{Items: items}
	if more {
		out.NextCursor = encodeCursor(invs[len(invs)-1].ID)
	}
	return ListEventInvitations200JSONResponse{Body: out, Headers: ListEventInvitations200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// InviteToEvent serves POST /api/v1/events/{id}/invitations.
func (s *Server) InviteToEvent(ctx context.Context, req InviteToEventRequestObject) (InviteToEventResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[InviteToEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	inv, err := events.Invite(ctx, v.AccountID, req.Id, knownID(req.Body.AccountId))
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[InviteToEventdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	items, err := s.newEventInvitations(ctx, []appgathering.Invitation{inv})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		p := s.notFoundResponse(ctx)
		return InviteToEventdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	return InviteToEvent201JSONResponse{Body: items[0], Headers: InviteToEvent201ResponseHeaders{CacheControl: eventCache()}}, nil
}

// RevokeEventInvitation serves DELETE /api/v1/events/{id}/invitations/{account_id}.
func (s *Server) RevokeEventInvitation(ctx context.Context, req RevokeEventInvitationRequestObject) (RevokeEventInvitationResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[RevokeEventInvitationdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	if err := events.RevokeInvitation(ctx, v.AccountID, req.Id, req.AccountId); err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[RevokeEventInvitationdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return RevokeEventInvitation204Response{}, nil
}

// AssignEventCoorganizer serves PUT /api/v1/events/{id}/coorganizers/{account_id}.
func (s *Server) AssignEventCoorganizer(ctx context.Context, req AssignEventCoorganizerRequestObject) (AssignEventCoorganizerResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[AssignEventCoorganizerdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	view, err := events.AssignCoorganizer(ctx, v.AccountID, req.Id, req.AccountId)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[AssignEventCoorganizerdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return AssignEventCoorganizer200JSONResponse{Body: out, Headers: AssignEventCoorganizer200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// RemoveEventCoorganizer serves DELETE /api/v1/events/{id}/coorganizers/{account_id}.
func (s *Server) RemoveEventCoorganizer(ctx context.Context, req RemoveEventCoorganizerRequestObject) (RemoveEventCoorganizerResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[RemoveEventCoorganizerdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	view, err := events.RemoveCoorganizer(ctx, v.AccountID, req.Id, req.AccountId)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[RemoveEventCoorganizerdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return RemoveEventCoorganizer200JSONResponse{Body: out, Headers: RemoveEventCoorganizer200ResponseHeaders{CacheControl: eventCache()}}, nil
}
