// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The decisions of the decision hooks (ADR-0147 §9): the admin operations under
// /admin/hook_decisions, which need manage_reports.

// ProblemTypeHookDecisionNotOverridable is the problem of cancelling a decision that had no effect,
// that is already cancelled, or that goes through its appeal.
const ProblemTypeHookDecisionNotOverridable = "/api/v1/problems/hook-decision-not-overridable"

// hookSummaryPeriod is the period that the counts of the summary cover.
const hookSummaryPeriod = 7 * 24 * time.Hour

// HookDecisions runs the use cases of the decision hooks. *apphook.Service implements it.
type HookDecisions interface {
	List(ctx context.Context, q apphook.ListQuery) ([]hook.Record, error)
	Statuses(ctx context.Context) ([]apphook.Status, error)
	// Summary counts the decisions of the last period and returns when the period starts.
	Summary(ctx context.Context, period time.Duration) (time.Time, []apphook.SummaryRow, error)
	Override(ctx context.Context, id, byUserID string) (hook.Record, error)
	InUse(ctx context.Context) ([]hook.Name, error)
}

// requireHookDecisions checks manage_reports; a server built without HookDecisions answers 404.
func (s *Server) requireHookDecisions(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil || p != nil {
		return p, err
	}
	if s.o.HookDecisions == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

func newHookDecision(r hook.Record) AdminHookDecision {
	d := r.Decision
	out := AdminHookDecision{
		Id: d.ID, Hook: HookName(d.Hook), SubjectKey: d.SubjectKey, Mode: HookDecisionMode(d.Mode), Outcome: HookOutcome(d.Outcome),
		LatencyMs: d.Latency.Milliseconds(), DecidedAt: d.DecidedAt, Overridable: r.Overridable(),
	}
	if d.AccountID != "" {
		out.AccountId = &d.AccountID
	}
	if d.Reason != hook.ReasonNone {
		reason := HookReason(d.Reason)
		out.Reason = &reason
	}
	if d.Note != "" {
		out.Note = &d.Note
	}
	if r.Override != nil {
		out.OverriddenAt = &r.Override.At
	}
	if (d.Hook == hook.InboundStatus || d.Hook == hook.InboundFollow) && d.Outcome == hook.Drop {
		until := d.DecidedAt.Add(appfederation.ReceivedDocumentRetention)
		out.RestorableUntil = &until
	}
	return out
}

// ListHookDecisions serves GET /api/v1/admin/hook_decisions.
func (s *Server) ListHookDecisions(ctx context.Context, req ListHookDecisionsRequestObject) (ListHookDecisionsResponseObject, error) {
	if p, err := s.requireHookDecisions(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListHookDecisionsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	// The cursor is a decision's ID, which goes to the database as one (ADR-0003 §9).
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListHookDecisionsdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	q := apphook.ListQuery{Before: pq.before, Limit: pq.limit + 1}
	if h := req.Params.Hook; h != nil {
		if !h.Valid() {
			return ListHookDecisionsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "hook")), nil
		}
		q.Hook = hook.Name(*h)
	}
	if m := req.Params.Mode; m != nil {
		if !m.Valid() {
			return ListHookDecisionsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "mode")), nil
		}
		q.Mode = hook.Mode(*m)
	}
	if o := req.Params.Outcome; o != nil {
		if !o.Valid() {
			return ListHookDecisionsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "outcome")), nil
		}
		q.Outcome = hook.Outcome(*o)
	}
	records, err := s.o.HookDecisions.List(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list hook decisions: %w", err)
	}
	records, last := trim(records, pq.limit)
	page := AdminHookDecisionPage{Items: make([]AdminHookDecision, len(records))}
	for i, r := range records {
		page.Items[i] = newHookDecision(r)
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(records[last].Decision.ID)
	}
	cache := cacheNoStore
	return ListHookDecisions200JSONResponse{Body: page, Headers: ListHookDecisions200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetHookSummary serves GET /api/v1/admin/hook_decisions/summary.
func (s *Server) GetHookSummary(ctx context.Context, _ GetHookSummaryRequestObject) (GetHookSummaryResponseObject, error) {
	if p, err := s.requireHookDecisions(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return GetHookSummarydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	statuses, err := s.o.HookDecisions.Statuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the hooks: %w", err)
	}
	since, rows, err := s.o.HookDecisions.Summary(ctx, hookSummaryPeriod)
	if err != nil {
		return nil, fmt.Errorf("clientapi: summarize the hooks: %w", err)
	}
	out := AdminHookSummary{Since: since.UTC(), Hooks: make([]AdminHook, len(statuses))}
	at := make(map[hook.Name]int, len(statuses))
	for i, st := range statuses {
		out.Hooks[i] = AdminHook{
			Hook: HookName(st.Name), Configured: st.Configured, Mode: HookMode(st.Mode), Outcomes: []AdminHookOutcome{},
		}
		if st.OnFailure != "" {
			onFailure := HookOnFailure(st.OnFailure)
			out.Hooks[i].OnFailure = &onFailure
		}
		at[st.Name] = i
	}
	for _, r := range rows {
		i, ok := at[r.Hook]
		if !ok || r.Decisions <= 0 {
			continue
		}
		out.Hooks[i].Outcomes = append(out.Hooks[i].Outcomes, AdminHookOutcome{
			Mode: HookDecisionMode(r.Mode), Outcome: HookOutcome(r.Outcome), Count: r.Decisions,
			AverageLatencyMs: r.TotalLatency.Milliseconds() / r.Decisions, MaxLatencyMs: r.MaxLatency.Milliseconds(),
		})
	}
	cache := cacheNoStore
	return GetHookSummary200JSONResponse{Body: out, Headers: GetHookSummary200ResponseHeaders{CacheControl: &cache}}, nil
}

// OverrideHookDecision serves POST /api/v1/admin/hook_decisions/{id}/override.
func (s *Server) OverrideHookDecision(ctx context.Context, req OverrideHookDecisionRequestObject) (OverrideHookDecisionResponseObject, error) {
	if p, err := s.requireHookDecisions(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return OverrideHookDecisiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if !kernel.ValidID(req.Id) {
		return OverrideHookDecisiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	record, err := s.o.HookDecisions.Override(ctx, req.Id, v.UserID)
	switch {
	case errors.Is(err, apphook.ErrNotFound):
		return OverrideHookDecisiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, apphook.ErrNotOverridable), errors.Is(err, apphook.ErrAlreadyOverridden):
		return OverrideHookDecisiondefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusConflict, ProblemTypeHookDecisionNotOverridable,
			s.Localizer(ctx).ServerErrorsHookDecisionNotOverridable(), "")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: override hook decision %s: %w", req.Id, err)
	}
	return OverrideHookDecision200JSONResponse(newHookDecision(record)), nil
}
