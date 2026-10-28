// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The error groups (ADR-0103 §8): the admin operations under /admin/error_groups, which need
// manage_errors, and the data of the webhook events error_group.*.

// ErrorGroups runs the use cases of the error groups. *appdiagnostics.Service implements it.
type ErrorGroups interface {
	Summaries(ctx context.Context, q appdiagnostics.SummaryQuery) ([]appdiagnostics.Summary, error)
	Summary(ctx context.Context, id string) (appdiagnostics.Summary, error)
	SummaryTx(ctx context.Context, tx uow.Tx, id string) (appdiagnostics.Summary, error)
	Sample(ctx context.Context, groupID string) (appdiagnostics.Sample, error)
	HourCounts(ctx context.Context, groupID string) ([]appdiagnostics.HourCount, error)
	Events(ctx context.Context, groupID string) ([]appdiagnostics.Event, error)
	Resolve(ctx context.Context, groupID string, by appdiagnostics.Actor) (appdiagnostics.Head, error)
	Mute(ctx context.Context, groupID string, by appdiagnostics.Actor) (appdiagnostics.Head, error)
	Unmute(ctx context.Context, groupID string, by appdiagnostics.Actor) (appdiagnostics.Head, error)
}

// requireErrorGroups checks manage_errors; a server built without ErrorGroups answers 404.
func (s *Server) requireErrorGroups(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageErrors); err != nil || p != nil {
		return p, err
	}
	if s.o.ErrorGroups == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

func optText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// newErrorGroup presents a summary as ErrorGroup.
func newErrorGroup(sm appdiagnostics.Summary) ErrorGroup {
	g, h := sm.Group, sm.Head
	var days *[]int64
	if sm.DayCounts != nil {
		days = &sm.DayCounts
	}
	return ErrorGroup{
		Id: g.ID, Fingerprint: g.Fingerprint, State: ErrorGroupState(h.State), StateSince: h.Since,
		ResolvedVersion: optText(h.ResolvedVersion), Source: ErrorGroupSource(g.Source), Kind: ErrorGroupKind(g.Kind),
		Where: g.Where, Type: g.ErrorType, Message: g.Message, Count24h: sm.CountDay, Count7d: sm.CountWeek, DailyCounts: days,
		FirstSeenAt: g.FirstSeenAt, LastSeenAt: sm.LastSeenAt, FirstVersion: g.FirstVersion, LastVersion: sm.LastVersion,
	}
}

// errorGroupCursorSeparator separates the time and the ID of a cursor of the error groups.
const errorGroupCursorSeparator = " "

func encodeErrorGroupCursor(sm appdiagnostics.Summary) *Cursor {
	return encodeCursor(sm.LastSeenAt.UTC().Format(time.RFC3339Nano) + errorGroupCursorSeparator + sm.Group.ID)
}

func decodeErrorGroupCursor(position string) (time.Time, string, bool) {
	at, id, ok := strings.Cut(position, errorGroupCursorSeparator)
	if !ok || !kernel.ValidID(id) {
		return time.Time{}, "", false
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	return t, id, err == nil
}

// ListErrorGroups serves GET /api/v1/admin/error_groups.
func (s *Server) ListErrorGroups(ctx context.Context, req ListErrorGroupsRequestObject) (ListErrorGroupsResponseObject, error) {
	if p, err := s.requireErrorGroups(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListErrorGroupsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	pq := s.parseOpaquePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListErrorGroupsdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	q := appdiagnostics.SummaryQuery{Limit: pq.limit + 1}
	if pq.before != "" {
		var ok bool
		if q.BeforeSeen, q.BeforeID, ok = decodeErrorGroupCursor(pq.before); !ok {
			return ListErrorGroupsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
	}
	if st := req.Params.State; st != nil {
		switch q.State = diagnostics.State(*st); q.State {
		case diagnostics.StateOpen, diagnostics.StateResolved, diagnostics.StateMuted:
		default:
			return ListErrorGroupsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
		}
	}
	if src := req.Params.Source; src != nil {
		switch q.Source = string(*src); kernel.ErrorSource(q.Source) {
		case kernel.ErrorSourceServer, kernel.ErrorSourceWeb:
		default:
			return ListErrorGroupsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "source")), nil
		}
	}
	groups, err := s.o.ErrorGroups.Summaries(ctx, q)
	if err != nil {
		return nil, err
	}
	groups, last := trim(groups, pq.limit)
	page := ErrorGroupPage{Items: make([]ErrorGroup, len(groups))}
	for i, g := range groups {
		page.Items[i] = newErrorGroup(g)
	}
	if last >= 0 {
		page.NextCursor = encodeErrorGroupCursor(groups[last])
	}
	cache := cacheNoStore
	return ListErrorGroups200JSONResponse{Body: page, Headers: ListErrorGroups200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetErrorGroup serves GET /api/v1/admin/error_groups/{id}.
func (s *Server) GetErrorGroup(ctx context.Context, req GetErrorGroupRequestObject) (GetErrorGroupResponseObject, error) {
	if p, err := s.requireErrorGroups(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return GetErrorGroupdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	sm, err := s.o.ErrorGroups.Summary(ctx, req.Id)
	switch {
	case errors.Is(err, appdiagnostics.ErrNotFound):
		return GetErrorGroupdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: error group %s: %w", req.Id, err)
	}
	detail, err := s.errorGroupDetail(ctx, sm)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return GetErrorGroup200JSONResponse{Body: detail, Headers: GetErrorGroup200ResponseHeaders{CacheControl: &cache}}, nil
}

// errorGroupDetail presents a summary with its sample, hourly counts, and events.
func (s *Server) errorGroupDetail(ctx context.Context, sm appdiagnostics.Summary) (ErrorGroupDetail, error) {
	g := newErrorGroup(sm)
	d := ErrorGroupDetail{
		Id: g.Id, Fingerprint: g.Fingerprint, State: g.State, StateSince: g.StateSince, ResolvedVersion: g.ResolvedVersion,
		Source: g.Source, Kind: g.Kind, Where: g.Where, Type: g.Type, Message: g.Message, Count24h: g.Count24h, Count7d: g.Count7d,
		FirstSeenAt: g.FirstSeenAt, LastSeenAt: g.LastSeenAt, FirstVersion: g.FirstVersion, LastVersion: g.LastVersion,
		HourlyCounts: []ErrorGroupHourCount{}, Events: []ErrorGroupEvent{},
	}
	sample, err := s.o.ErrorGroups.Sample(ctx, sm.Group.ID)
	switch {
	case err == nil:
		d.Sample = &ErrorGroupSample{
			Type: sample.ErrorType, Message: sample.Message, Stack: sample.Stack, RequestId: optText(sample.RequestID),
			TraceId: optText(sample.TraceID), Version: sample.Version, Role: sample.Role, OccurredAt: sample.OccurredAt,
		}
	case !errors.Is(err, appdiagnostics.ErrNotFound):
		return ErrorGroupDetail{}, fmt.Errorf("clientapi: the sample of error group %s: %w", sm.Group.ID, err)
	}
	counts, err := s.o.ErrorGroups.HourCounts(ctx, sm.Group.ID)
	if err != nil {
		return ErrorGroupDetail{}, fmt.Errorf("clientapi: the counts of error group %s: %w", sm.Group.ID, err)
	}
	for _, c := range counts {
		d.HourlyCounts = append(d.HourlyCounts, ErrorGroupHourCount{Hour: c.Hour, Count: c.Occurrences, LastVersion: c.LastVersion})
	}
	events, err := s.o.ErrorGroups.Events(ctx, sm.Group.ID)
	if err != nil {
		return ErrorGroupDetail{}, fmt.Errorf("clientapi: the events of error group %s: %w", sm.Group.ID, err)
	}
	for _, e := range events {
		d.Events = append(d.Events, ErrorGroupEvent{
			Kind: ErrorGroupEventKind(e.Kind), Actor: ErrorGroupEventActor(e.Actor.Kind), Version: optText(e.Version), OccurredAt: e.OccurredAt,
		})
	}
	return d, nil
}

// changeErrorGroup runs a resolve, a mute, or an unmute by the viewer and presents the group:
// the problem when the group is unknown or the viewer may not.
func (s *Server) changeErrorGroup(ctx context.Context, id string, pick func(ErrorGroups) errorGroupChange) (*ErrorGroup, *problem, error) {
	if p, err := s.requireErrorGroups(ctx); err != nil || p != nil {
		return nil, p, err
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	_, err = pick(s.o.ErrorGroups)(ctx, id, appdiagnostics.Actor{Kind: diagnostics.ActorUser, UserID: v.UserID})
	var sm appdiagnostics.Summary
	if err == nil {
		sm, err = s.o.ErrorGroups.Summary(ctx, id)
	}
	switch {
	case errors.Is(err, appdiagnostics.ErrNotFound):
		p := s.notFoundResponse(ctx)
		return nil, &p, nil
	case err != nil:
		return nil, nil, fmt.Errorf("clientapi: change error group %s: %w", id, err)
	}
	g := newErrorGroup(sm)
	return &g, nil, nil
}

func errorGroupResponse(g ErrorGroup) ErrorGroupJSONResponse {
	cache := cacheNoStore
	return ErrorGroupJSONResponse{Body: g, Headers: ErrorGroupResponseHeaders{CacheControl: &cache}}
}

// ResolveErrorGroup serves POST /api/v1/admin/error_groups/{id}/resolve.
func (s *Server) ResolveErrorGroup(ctx context.Context, req ResolveErrorGroupRequestObject) (ResolveErrorGroupResponseObject, error) {
	g, p, err := s.changeErrorGroup(ctx, req.Id, func(e ErrorGroups) errorGroupChange { return e.Resolve })
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return ResolveErrorGroupdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return ResolveErrorGroup200JSONResponse{errorGroupResponse(*g)}, nil
}

// MuteErrorGroup serves POST /api/v1/admin/error_groups/{id}/mute.
func (s *Server) MuteErrorGroup(ctx context.Context, req MuteErrorGroupRequestObject) (MuteErrorGroupResponseObject, error) {
	g, p, err := s.changeErrorGroup(ctx, req.Id, func(e ErrorGroups) errorGroupChange { return e.Mute })
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return MuteErrorGroupdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return MuteErrorGroup200JSONResponse{errorGroupResponse(*g)}, nil
}

// UnmuteErrorGroup serves POST /api/v1/admin/error_groups/{id}/unmute.
func (s *Server) UnmuteErrorGroup(ctx context.Context, req UnmuteErrorGroupRequestObject) (UnmuteErrorGroupResponseObject, error) {
	g, p, err := s.changeErrorGroup(ctx, req.Id, func(e ErrorGroups) errorGroupChange { return e.Unmute })
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UnmuteErrorGroupdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnmuteErrorGroup200JSONResponse{errorGroupResponse(*g)}, nil
}

// errorGroupChange is a resolve, a mute, or an unmute.
type errorGroupChange func(context.Context, string, appdiagnostics.Actor) (appdiagnostics.Head, error)

// WebhookErrorGroup presents an error group as ErrorGroup, read in tx.
func (s *Server) WebhookErrorGroup(ctx context.Context, tx uow.Tx, id string) (any, error) {
	if s.o.ErrorGroups == nil {
		return nil, fmt.Errorf("clientapi: error group %s: %w", id, appdiagnostics.ErrNotFound)
	}
	sm, err := s.o.ErrorGroups.SummaryTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: error group %s: %w", id, err)
	}
	return newErrorGroup(sm), nil
}
