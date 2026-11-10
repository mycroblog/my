// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// Drafts runs the use cases of the viewer's drafts and scheduled posts (ADR-0156 §9).
// *appcontent.DraftService implements it.
type Drafts interface {
	Create(ctx context.Context, accountID string, in appcontent.DraftInput) (appcontent.DraftView, error)
	Get(ctx context.Context, accountID, id string) (appcontent.DraftView, error)
	List(ctx context.Context, accountID, beforeID string, scheduled *bool, limit int) ([]appcontent.DraftView, error)
	Update(ctx context.Context, accountID, id string, ifMatch time.Time, in appcontent.DraftInput) (appcontent.DraftView, error)
	Delete(ctx context.Context, accountID, id string) error
	Schedule(ctx context.Context, accountID, id string, publishAt time.Time) (appcontent.DraftView, error)
	Unschedule(ctx context.Context, accountID, id string) (appcontent.DraftView, error)
	Publish(ctx context.Context, accountID, id, clientID string) (appcontent.Post, error)
}

// ListDrafts serves GET /api/v1/drafts: a page of the viewer's drafts, newest first. The cursor
// is the ID of the last draft of the previous page.
func (s *Server) ListDrafts(ctx context.Context, req ListDraftsRequestObject) (ListDraftsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Drafts == nil {
		return ListDraftsdefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusNotFound, ProblemTypeBlank, s.Localizer(ctx).ServerErrorsNotFound(), "")), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListDraftsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	views, err := s.o.Drafts.List(ctx, v.AccountID, q.before, req.Params.Scheduled, q.limit+1)
	if err != nil {
		return nil, err
	}
	views, last := trim(views, q.limit)
	out := DraftPage{Items: make([]Draft, 0, len(views))}
	for _, view := range views {
		out.Items = append(out.Items, presentDraft(view))
	}
	if last >= 0 {
		out.NextCursor = encodeCursor(views[last].Draft.ID)
	}
	return ListDrafts200JSONResponse(out), nil
}

// CreateDraft serves POST /api/v1/drafts.
func (s *Server) CreateDraft(ctx context.Context, req CreateDraftRequestObject) (CreateDraftResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Drafts == nil {
		return CreateDraftdefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusNotFound, ProblemTypeBlank, s.Localizer(ctx).ServerErrorsNotFound(), "")), nil
	}
	view, err := s.o.Drafts.Create(ctx, v.AccountID, draftInput(req.Body))
	if err != nil {
		if p, ok := s.draftProblem(ctx, err); ok {
			return CreateDraftdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return CreateDraft201JSONResponse{draftResponse(view)}, nil
}

// GetDraft serves GET /api/v1/drafts/{id}.
func (s *Server) GetDraft(ctx context.Context, req GetDraftRequestObject) (GetDraftResponseObject, error) {
	view, p, err := s.draftOp(ctx, func(accountID string) (appcontent.DraftView, error) {
		return s.o.Drafts.Get(ctx, accountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return GetDraftdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return GetDraft200JSONResponse{draftResponse(view)}, nil
}

// UpdateDraft serves PUT /api/v1/drafts/{id}, which takes the ETag of the draft the client read
// in If-Match (ADR-0156 §9).
func (s *Server) UpdateDraft(ctx context.Context, req UpdateDraftRequestObject) (UpdateDraftResponseObject, error) {
	l := s.Localizer(ctx)
	if req.Params.IfMatch == nil {
		return UpdateDraftdefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusPreconditionRequired, ProblemTypeBlank,
			l.ServerErrorsPreconditionRequired(), l.ServerErrorsDraftIfMatchRequired())), nil
	}
	ifMatch, ok := parseDraftETag(*req.Params.IfMatch)
	view, p, err := s.draftOp(ctx, func(accountID string) (appcontent.DraftView, error) {
		if !ok {
			// A tag this server did not make matches no version: answered as a draft that changed,
			// once the draft is known to be the viewer's.
			if _, err := s.o.Drafts.Get(ctx, accountID, req.Id); err != nil {
				return appcontent.DraftView{}, err
			}
			return appcontent.DraftView{}, appcontent.ErrDraftChanged
		}
		return s.o.Drafts.Update(ctx, accountID, req.Id, ifMatch, draftInput(req.Body))
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UpdateDraftdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UpdateDraft200JSONResponse{draftResponse(view)}, nil
}

// DeleteDraft serves DELETE /api/v1/drafts/{id}.
func (s *Server) DeleteDraft(ctx context.Context, req DeleteDraftRequestObject) (DeleteDraftResponseObject, error) {
	_, p, err := s.draftOp(ctx, func(accountID string) (appcontent.DraftView, error) {
		return appcontent.DraftView{}, s.o.Drafts.Delete(ctx, accountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return DeleteDraftdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return DeleteDraft204Response{}, nil
}

// ScheduleDraft serves PUT /api/v1/drafts/{id}/schedule.
func (s *Server) ScheduleDraft(ctx context.Context, req ScheduleDraftRequestObject) (ScheduleDraftResponseObject, error) {
	view, p, err := s.draftOp(ctx, func(accountID string) (appcontent.DraftView, error) {
		return s.o.Drafts.Schedule(ctx, accountID, req.Id, req.Body.PublishAt)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return ScheduleDraftdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return ScheduleDraft200JSONResponse{draftResponse(view)}, nil
}

// UnscheduleDraft serves DELETE /api/v1/drafts/{id}/schedule.
func (s *Server) UnscheduleDraft(ctx context.Context, req UnscheduleDraftRequestObject) (UnscheduleDraftResponseObject, error) {
	view, p, err := s.draftOp(ctx, func(accountID string) (appcontent.DraftView, error) {
		return s.o.Drafts.Unschedule(ctx, accountID, req.Id)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UnscheduleDraftdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnscheduleDraft200JSONResponse{draftResponse(view)}, nil
}

// PublishDraft serves POST /api/v1/drafts/{id}/publish: the draft becomes a new post with the
// checks and answers of POST /api/v1/statuses.
func (s *Server) PublishDraft(ctx context.Context, req PublishDraftRequestObject) (PublishDraftResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Drafts == nil {
		return PublishDraftdefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusNotFound, ProblemTypeBlank, s.Localizer(ctx).ServerErrorsNotFound(), "")), nil
	}
	if p, h, limited := s.LimitProblem(ctx, PostLimit, v.AccountID); limited {
		return PublishDraftdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	if p, err := s.refuseMoved(ctx, v.AccountID); err != nil {
		return nil, err
	} else if p != nil {
		return PublishDraftdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if view, err := s.o.Drafts.Get(ctx, v.AccountID, req.Id); err == nil && view.Content.QuotedStatusID != "" {
		if p, err := s.quotableRemotely(ctx, view.Content.QuotedStatusID); err != nil {
			return nil, err
		} else if p != nil {
			return PublishDraftdefaultApplicationProblemPlusJSONResponse(*p), nil
		}
	}
	post, err := s.o.Drafts.Publish(ctx, v.AccountID, req.Id, v.ClientID)
	if err != nil {
		if r, ok, err := s.statusRejected(ctx, err); ok {
			if err != nil {
				return nil, err
			}
			return PublishDraft422ApplicationProblemPlusJSONResponse{r.(CreateStatus422ApplicationProblemPlusJSONResponse).StatusRejectedProblemApplicationProblemPlusJSONResponse}, nil
		}
		if p, ok := s.draftProblem(ctx, err); ok {
			return PublishDraftdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	created, err := s.createdStatus(ctx, post.Status.ID)
	if err != nil {
		return nil, err
	}
	switch r := created.(type) {
	case CreateStatus201JSONResponse:
		return PublishDraft201JSONResponse{Body: r.Body, Headers: PublishDraft201ResponseHeaders(r.Headers)}, nil
	case CreateStatusdefaultApplicationProblemPlusJSONResponse:
		return PublishDraftdefaultApplicationProblemPlusJSONResponse(r), nil
	}
	return nil, errors.New("clientapi: unexpected answer to a published draft")
}

// draftOp runs op for the viewer and returns its draft, or the problem of its failure.
func (s *Server) draftOp(ctx context.Context, op func(accountID string) (appcontent.DraftView, error)) (appcontent.DraftView, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return appcontent.DraftView{}, nil, err
	}
	if s.o.Drafts == nil {
		p := s.problem(ctx, http.StatusNotFound, ProblemTypeBlank, s.Localizer(ctx).ServerErrorsNotFound(), "")
		return appcontent.DraftView{}, &p, nil
	}
	view, err := op(v.AccountID)
	if err != nil {
		if p, ok := s.draftProblem(ctx, err); ok {
			return appcontent.DraftView{}, &p, nil
		}
		return appcontent.DraftView{}, nil, err
	}
	return view, nil, nil
}

// draftProblem returns the problem that err of a drafts use case means to the client: those of
// the drafts, else those of a new post (statusProblem).
func (s *Server) draftProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (problem, bool) {
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail), true
	}
	var limit *appcontent.DraftLimitError
	switch {
	case errors.As(err, &limit):
		switch limit.Limit {
		case appcontent.LimitDrafts:
			return unprocessable(l.ServerErrorsDraftMaxDrafts(limit.Max))
		case appcontent.LimitScheduledPosts:
			return unprocessable(l.ServerErrorsDraftMaxScheduledPosts(limit.Max))
		case appcontent.LimitScheduledPostsPerDay:
			return unprocessable(l.ServerErrorsDraftMaxScheduledPostsPerDay(limit.Max))
		case appcontent.LimitScheduleMinLead:
			return unprocessable(l.ServerErrorsDraftScheduleTooSoon(limit.Max))
		case appcontent.LimitScheduleMaxLead:
			return unprocessable(l.ServerErrorsDraftScheduleTooFar(limit.Max))
		}
		return unprocessable("")
	case errors.Is(err, appcontent.ErrDraftChanged):
		return s.problem(ctx, http.StatusPreconditionFailed, ProblemTypeBlank, l.ServerErrorsPreconditionFailed(), l.ServerErrorsDraftChanged()), true
	case errors.Is(err, appcontent.ErrDraftNotScheduled):
		return s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), l.ServerErrorsDraftNotScheduled()), true
	case errors.Is(err, content.ErrInvalidDraft):
		return unprocessable(l.ServerErrorsDraftInvalid())
	}
	return s.statusProblem(ctx, err)
}

// draftInput maps the content of a draft that a request saves.
func draftInput(body *DraftRequest) appcontent.DraftInput {
	if body == nil {
		return appcontent.DraftInput{}
	}
	in := appcontent.DraftInput{
		Text: deref(body.Text), SpoilerText: deref(body.SpoilerText), Sensitive: deref(body.Sensitive),
		Language: deref(body.Language), Visibility: content.Visibility(body.Visibility),
		InReplyToID: knownID(deref(body.InReplyToId)), QuotedStatusID: knownID(deref(body.QuotedStatusId)),
		InteractionPolicy: interactionPolicyChange(body.InteractionPolicy), Media: mediaInput(body.Media),
	}
	if p := body.Poll; p != nil {
		in.Poll = &content.DraftPoll{
			Options: p.Options, Duration: time.Duration(p.ExpiresIn) * time.Second,
			Multiple: deref(p.Multiple), HideTotals: deref(p.HideTotals),
		}
	}
	return in
}

// draftResponse returns the body and the ETag of a draft.
func draftResponse(v appcontent.DraftView) DraftJSONResponse {
	tag := draftETag(v.Content.UpdatedAt)
	return DraftJSONResponse{Body: presentDraft(v), Headers: DraftResponseHeaders{ETag: &tag}}
}

// draftETag is the ETag of a draft saved at updatedAt: the time of the save, to the millisecond
// that the databases keep, which every save advances.
func draftETag(updatedAt time.Time) string {
	return `"` + strconv.FormatInt(updatedAt.UnixMilli(), 10) + `"`
}

// parseDraftETag returns the save time of an If-Match that draftETag made.
func parseDraftETag(tag string) (time.Time, bool) {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
	if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return time.Time{}, false
	}
	ms, err := strconv.ParseInt(tag[1:len(tag)-1], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(ms).UTC(), true
}

// presentDraft presents a draft.
func presentDraft(v appcontent.DraftView) Draft {
	c := v.Content
	out := Draft{
		Id: v.Draft.ID, CreatedAt: v.Draft.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC(),
		Text: c.Text, SpoilerText: c.SpoilerText, Sensitive: c.Sensitive, Visibility: Visibility(c.Visibility),
		Media: make([]StatusMediaItem, 0, len(c.Attachments)),
	}
	if c.Language != "" {
		out.Language = &c.Language
	}
	if c.InReplyToID != "" {
		out.InReplyToId = &c.InReplyToID
	}
	if c.QuotedStatusID != "" {
		out.QuotedStatusId = &c.QuotedStatusID
	}
	if len(c.InteractionPolicy) > 0 {
		p := InteractionPolicyChange{}
		for k, set := range c.InteractionPolicy {
			rule := newInteractionRule(string(set.Automatic), string(set.Manual))
			switch k {
			case content.InteractionReply:
				p.Reply = &rule
			case content.InteractionAnnounce:
				p.Announce = &rule
			case content.InteractionQuote:
				p.Quote = &rule
			}
		}
		out.InteractionPolicy = &p
	}
	for _, a := range c.Attachments {
		item := StatusMediaItem{Id: a.MediaID}
		if a.Description != "" {
			d := a.Description
			item.Description = &d
		}
		out.Media = append(out.Media, item)
	}
	if p := c.Poll; p != nil {
		multiple, hide := p.Multiple, p.HideTotals
		out.Poll = &DraftPoll{Options: p.Options, ExpiresIn: int(p.Duration / time.Second), Multiple: &multiple, HideTotals: &hide}
		if out.Poll.Options == nil {
			out.Poll.Options = []string{}
		}
	}
	if v.Scheduled != nil {
		at := v.Scheduled.PublishAt.UTC()
		out.ScheduledAt = &at
	}
	if r := v.LastOutcome; r != nil {
		o := DraftScheduleOutcome{
			PublishAt: r.Schedule.PublishAt.UTC(), Outcome: DraftScheduleOutcomeOutcome(r.Outcome.Outcome), OccurredAt: r.Outcome.OccurredAt.UTC(),
		}
		if r.Outcome.FailureReason != "" {
			reason := DraftFailureReason(r.Outcome.FailureReason)
			o.Reason = &reason
		}
		out.LastSchedule = &o
	}
	return out
}
