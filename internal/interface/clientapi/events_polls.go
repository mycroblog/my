// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The date polls of the events (ADR-0161 §7, §16): the candidate dates, the answers to them, and
// the confirmation of one.

// The problem types of the date polls.
const (
	ProblemTypeEventUnscheduled   = "/api/v1/problems/event-unscheduled"
	ProblemTypeEventScheduled     = "/api/v1/problems/event-scheduled"
	ProblemTypeEventLastCandidate = "/api/v1/problems/event-last-candidate"
)

// EventPolls runs the date polls of the events: *appgathering.Service implements it.
type EventPolls interface {
	Candidates(ctx context.Context, accountID, id string) ([]appgathering.CandidateTally, error)
	AddCandidate(ctx context.Context, accountID, id string, sc gathering.Schedule) (appgathering.CandidateTally, error)
	WithdrawCandidate(ctx context.Context, accountID, id, candidateID string) error
	Vote(ctx context.Context, accountID, id, candidateID string, vote gathering.Vote) (appgathering.CandidateTally, error)
	Ballots(ctx context.Context, accountID, id, candidateID string, vote gathering.Vote, afterAccountID string, limit int) ([]appgathering.Ballot, bool, error)
	Confirm(ctx context.Context, accountID, id, candidateID string) (appgathering.View, error)
}

// eventPollProblem maps the errors of the date polls; it reports false for any other error.
func (s *Server) eventPollProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appgathering.ErrInvalidVote):
		return s.unprocessable(ctx, "vote"), true
	case errors.Is(err, appgathering.ErrUnscheduled):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventUnscheduled, l.ServerErrorsEventUnscheduled(), ""), true
	case errors.Is(err, appgathering.ErrScheduled):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventScheduled, l.ServerErrorsEventScheduled(), ""), true
	case errors.Is(err, appgathering.ErrLastCandidate):
		return s.problem(ctx, http.StatusConflict, ProblemTypeEventLastCandidate, l.ServerErrorsEventLastCandidate(), ""), true
	}
	return problem{}, false
}

// newEventCandidate presents a candidate as the viewer sees it.
func newEventCandidate(c appgathering.CandidateTally) EventCandidate {
	out := EventCandidate{
		Id: c.ID, StartsAt: c.Schedule.StartsAt.UTC(), EndsAt: c.Schedule.EndsAt.UTC(),
		YesCount: c.Yes, MaybeCount: c.Maybe, NoCount: c.No, Confirmed: c.Confirmed,
	}
	if c.ViewerVote != "" {
		v := EventVote(c.ViewerVote)
		out.Vote = &v
	}
	return out
}

// eventCandidates presents the candidates of a request, nil for none.
func eventCandidates(in *[]EventCandidateCreate) []gathering.Schedule {
	if in == nil {
		return nil
	}
	out := make([]gathering.Schedule, len(*in))
	for i, c := range *in {
		out[i] = gathering.Schedule{StartsAt: c.StartsAt, EndsAt: c.EndsAt}
	}
	return out
}

// ListEventCandidates serves GET /api/v1/events/{id}/candidates.
func (s *Server) ListEventCandidates(ctx context.Context, req ListEventCandidatesRequestObject) (ListEventCandidatesResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[ListEventCandidatesdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	cs, err := events.Candidates(ctx, v.AccountID, req.Id)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[ListEventCandidatesdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out := EventCandidateList{Items: make([]EventCandidate, 0, len(cs))}
	for _, c := range cs {
		out.Items = append(out.Items, newEventCandidate(c))
	}
	return ListEventCandidates200JSONResponse{Body: out, Headers: ListEventCandidates200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// AddEventCandidate serves POST /api/v1/events/{id}/candidates.
func (s *Server) AddEventCandidate(ctx context.Context, req AddEventCandidateRequestObject) (AddEventCandidateResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[AddEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	c, err := events.AddCandidate(ctx, v.AccountID, req.Id, gathering.Schedule{StartsAt: req.Body.StartsAt, EndsAt: req.Body.EndsAt})
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[AddEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return AddEventCandidate201JSONResponse{Body: newEventCandidate(c), Headers: AddEventCandidate201ResponseHeaders{CacheControl: eventCache()}}, nil
}

// WithdrawEventCandidate serves DELETE /api/v1/events/{id}/candidates/{candidate_id}.
func (s *Server) WithdrawEventCandidate(ctx context.Context, req WithdrawEventCandidateRequestObject) (WithdrawEventCandidateResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[WithdrawEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	if err := events.WithdrawCandidate(ctx, v.AccountID, req.Id, req.CandidateId); err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[WithdrawEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return WithdrawEventCandidate204Response{}, nil
}

// VoteEventCandidate serves PUT /api/v1/events/{id}/candidates/{candidate_id}/vote.
func (s *Server) VoteEventCandidate(ctx context.Context, req VoteEventCandidateRequestObject) (VoteEventCandidateResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[VoteEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	c, err := events.Vote(ctx, v.AccountID, req.Id, req.CandidateId, gathering.Vote(req.Body.Vote))
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[VoteEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return VoteEventCandidate200JSONResponse{Body: newEventCandidate(c), Headers: VoteEventCandidate200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// ListEventCandidateVotes serves GET /api/v1/events/{id}/candidates/{candidate_id}/votes.
func (s *Server) ListEventCandidateVotes(ctx context.Context, req ListEventCandidateVotesRequestObject) (ListEventCandidateVotesResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[ListEventCandidateVotesdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListEventCandidateVotesdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	var vote gathering.Vote
	if req.Params.Vote != nil {
		vote = gathering.Vote(*req.Params.Vote)
	}
	bs, more, err := events.Ballots(ctx, v.AccountID, req.Id, req.CandidateId, vote, q.before, q.limit)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[ListEventCandidateVotesdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	ids := make([]string, len(bs))
	for i, b := range bs {
		ids[i] = b.AccountID
	}
	accounts, err := s.accountsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := EventBallotPage{Items: make([]EventBallot, 0, len(bs))}
	for _, b := range bs {
		if a, ok := accounts[b.AccountID]; ok {
			out.Items = append(out.Items, EventBallot{Account: a, Vote: EventVote(b.Vote)})
		}
	}
	if more {
		out.NextCursor = encodeCursor(bs[len(bs)-1].AccountID)
	}
	return ListEventCandidateVotes200JSONResponse{Body: out, Headers: ListEventCandidateVotes200ResponseHeaders{CacheControl: eventCache()}}, nil
}

// ConfirmEventCandidate serves POST /api/v1/events/{id}/candidates/{candidate_id}/confirm.
func (s *Server) ConfirmEventCandidate(ctx context.Context, req ConfirmEventCandidateRequestObject) (ConfirmEventCandidateResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[ConfirmEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	view, err := events.Confirm(ctx, v.AccountID, req.Id, req.CandidateId)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[ConfirmEventCandidatedefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newEvent(ctx, view)
	if err != nil {
		return nil, err
	}
	return ConfirmEventCandidate200JSONResponse{Body: out, Headers: ConfirmEventCandidate200ResponseHeaders{CacheControl: eventCache()}}, nil
}
