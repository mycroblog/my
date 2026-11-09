// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// The viewer's own moderation actions (FR-715, NFR-MOD-6): the list of them and the action of a
// moderation_notice. The moderation module gives them without the private note, the report, or
// the issuer, so nothing here can leak them.

// OwnModeration runs the use cases of the actions on the viewer's own account.
// *appmoderation.Service implements it.
type OwnModeration interface {
	OwnActions(ctx context.Context, accountID string, f appmoderation.OwnActionFilter, limit int) ([]appmoderation.TargetActionView, error)
	OwnAction(ctx context.Context, accountID, actionID string) (appmoderation.TargetActionView, error)
}

// newModerationAction presents an action on the viewer's account, with its labels in the
// viewer's language.
func newModerationAction(l *i18n.Localizer, v appmoderation.TargetActionView) ModerationAction {
	out := ModerationAction{
		Id: v.ID, Kind: AdminAccountActionKind(v.Kind), KindLabel: l.ServerModerationActionKind(string(v.Kind)),
		ReasonCategory: AdminAccountActionCategory(v.ReasonCategory), ReasonCategoryLabel: l.ServerModerationActionCategory(string(v.ReasonCategory)),
		StatusIds: append([]string{}, v.StatusIDs...), State: ModerationActionState(v.State), IssuedAt: v.IssuedAt.UTC(),
	}
	if v.PublicReason != "" {
		out.PublicReason = &v.PublicReason
	}
	if v.ExpiresAt != nil {
		t := v.ExpiresAt.UTC()
		out.ExpiresAt = &t
	}
	if v.LiftedAt != nil {
		t := v.LiftedAt.UTC()
		out.LiftedAt = &t
	}
	out.Appealable = v.Appealable
	if v.AppealDeadline != nil {
		t := v.AppealDeadline.UTC()
		out.AppealDeadline = &t
	}
	if a := v.Appeal; a != nil {
		out.Appeal = &ModerationAppeal{
			Id: a.ID, Text: a.Text, State: ModerationAppealState(a.State), SubmittedAt: a.SubmittedAt.UTC(),
		}
		if a.DecidedAt != nil {
			t := a.DecidedAt.UTC()
			out.Appeal.DecidedAt = &t
		}
		if a.DecisionReason != "" {
			out.Appeal.DecisionReason = &a.DecisionReason
		}
	}
	return out
}

// ListOwnModerationActions serves GET /api/v1/moderation/actions: the actions on the viewer's
// account, newest first. The cursor is the ID of the last action of the previous page.
func (s *Server) ListOwnModerationActions(ctx context.Context, req ListOwnModerationActionsRequestObject) (ListOwnModerationActionsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListOwnModerationActionsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	f := appmoderation.OwnActionFilter{Before: q.before, Active: req.Params.Active != nil && *req.Params.Active}
	vs, err := s.o.OwnModeration.OwnActions(ctx, v.AccountID, f, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the actions on account %s: %w", v.AccountID, err)
	}
	vs, last := trim(vs, q.limit)
	l := s.Localizer(ctx)
	page := ModerationActionPage{Items: make([]ModerationAction, 0, len(vs))}
	for _, a := range vs {
		page.Items = append(page.Items, newModerationAction(l, a))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(vs[last].ID)
	}
	cache := cacheNoStore
	return ListOwnModerationActions200JSONResponse{Body: page, Headers: ListOwnModerationActions200ResponseHeaders{CacheControl: &cache}}, nil
}

// noticeActions returns the actions on accountID with the IDs that its moderation notices name,
// by ID; an action that no longer exists is left out. nil OwnModeration reads none.
func (s *Server) noticeActions(ctx context.Context, accountID string, actionIDs []string) (map[string]ModerationAction, error) {
	out := map[string]ModerationAction{}
	if s.o.OwnModeration == nil || len(actionIDs) == 0 {
		return out, nil
	}
	views, err := s.ownActionsByID(ctx, accountID, actionIDs)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the moderation actions of the notices to %s: %w", accountID, err)
	}
	l := s.Localizer(ctx)
	for id, v := range views {
		out[id] = newModerationAction(l, v)
	}
	return out, nil
}
