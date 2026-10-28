// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// The admin operations of the appeals (FR-707, manage_accounts). Overturning an appeal lifts its
// action in the moderation module, in the transaction of the decision.

// AppealModeration runs the moderators' use cases on the appeals. *appmoderation.Service
// implements it.
type AppealModeration interface {
	Appeals(ctx context.Context, f appmoderation.AppealFilter, limit int) ([]appmoderation.AppealView, error)
	Appeal(ctx context.Context, id string) (appmoderation.AppealView, error)
	DecideAppeal(ctx context.Context, appealID string, decision moderation.AppealDecisionKind, publicReason, byUserID string) (appmoderation.AppealView, error)
	CountPendingAppeals(ctx context.Context) (int64, error)
}

// newAdminAppeals presents appeals with their actions, with the accounts of their moderators
// read once each.
func (s *Server) newAdminAppeals(ctx context.Context, vs []appmoderation.AppealView) ([]AdminAppeal, error) {
	return s.newAdminAppealsWith(ctx, s.reads(), vs)
}

// newAdminAppealsWith is newAdminAppeals through reads.
func (s *Server) newAdminAppealsWith(ctx context.Context, reads presentReads, vs []appmoderation.AppealView) ([]AdminAppeal, error) {
	actions := make([]appmoderation.AccountActionView, len(vs))
	for i, v := range vs {
		actions[i] = v.Action
	}
	presented, err := s.newAdminAccountActionsWith(ctx, reads, actions)
	if err != nil {
		return nil, err
	}
	moderators := map[string]string{}
	out := make([]AdminAppeal, 0, len(vs))
	for i, v := range vs {
		a := v.Appeal
		item := AdminAppeal{
			Id: a.ID, AccountId: a.AccountID, Text: a.Text, State: ModerationAppealState(v.State()),
			SubmittedAt: a.SubmittedAt.UTC(), Action: presented[i],
		}
		if d := v.Decision; d != nil {
			t := d.DecidedAt.UTC()
			item.DecidedAt = &t
			if d.PublicReason != "" {
				item.DecisionReason = &d.PublicReason
			}
			id, err := moderatorAccount(ctx, reads, moderators, d.DecidedByUserID)
			if err != nil {
				return nil, err
			}
			if id != "" {
				item.DecidedByAccountId = &id
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// moderatorAccount returns the local account of the moderator userID, "" for none, through
// seen, which keeps the accounts already read.
func moderatorAccount(ctx context.Context, reads presentReads, seen map[string]string, userID string) (string, error) {
	if id, ok := seen[userID]; ok || userID == "" {
		return id, nil
	}
	la, err := reads.LocalAccountOfUser(ctx, userID)
	if err != nil && !errors.Is(err, appaccount.ErrNotFound) {
		return "", fmt.Errorf("clientapi: the account of user %s: %w", userID, err)
	}
	seen[userID] = la.AccountID
	return la.AccountID, nil
}

// appealResponse presents one appeal.
func (s *Server) appealResponse(ctx context.Context, v appmoderation.AppealView) (AdminAppealJSONResponse, error) {
	items, err := s.newAdminAppeals(ctx, []appmoderation.AppealView{v})
	if err != nil {
		return AdminAppealJSONResponse{}, err
	}
	cache := cacheNoStore
	return AdminAppealJSONResponse{Body: items[0], Headers: AdminAppealResponseHeaders{CacheControl: &cache}}, nil
}

// ListAppeals serves GET /api/v1/admin/appeals, newest first. The cursor is the ID of the last
// appeal of the previous page.
func (s *Server) ListAppeals(ctx context.Context, req ListAppealsRequestObject) (ListAppealsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return ListAppealsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListAppealsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	f := appmoderation.AppealFilter{Before: q.before}
	if req.Params.State != nil {
		f.State = moderation.AppealState(*req.Params.State)
	}
	vs, err := s.o.AppealModeration.Appeals(ctx, f, q.limit+1)
	if errors.Is(err, appmoderation.ErrInvalidQuery) {
		return ListAppealsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the appeals: %w", err)
	}
	vs, last := trim(vs, q.limit)
	items, err := s.newAdminAppeals(ctx, vs)
	if err != nil {
		return nil, err
	}
	page := AdminAppealPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(vs[last].Appeal.ID)
	}
	cache := cacheNoStore
	return ListAppeals200JSONResponse{Body: page, Headers: ListAppeals200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetAppeal serves GET /api/v1/admin/appeals/{id}.
func (s *Server) GetAppeal(ctx context.Context, req GetAppealRequestObject) (GetAppealResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return GetAppealdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := s.o.AppealModeration.Appeal(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return GetAppealdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: appeal %s: %w", req.Id, err)
	}
	res, err := s.appealResponse(ctx, v)
	if err != nil {
		return nil, err
	}
	return GetAppeal200JSONResponse{res}, nil
}

// OverturnAppeal serves POST /api/v1/admin/appeals/{id}/overturn.
func (s *Server) OverturnAppeal(ctx context.Context, req OverturnAppealRequestObject) (OverturnAppealResponseObject, error) {
	res, p, err := s.decideAppeal(ctx, req.Id, moderation.AppealDecisionOverturned, req.Body)
	if err != nil || p != nil {
		if p != nil {
			return OverturnAppealdefaultApplicationProblemPlusJSONResponse(*p), nil
		}
		return nil, err
	}
	return OverturnAppeal200JSONResponse{res}, nil
}

// UpholdAppeal serves POST /api/v1/admin/appeals/{id}/uphold.
func (s *Server) UpholdAppeal(ctx context.Context, req UpholdAppealRequestObject) (UpholdAppealResponseObject, error) {
	res, p, err := s.decideAppeal(ctx, req.Id, moderation.AppealDecisionUpheld, req.Body)
	if err != nil || p != nil {
		if p != nil {
			return UpholdAppealdefaultApplicationProblemPlusJSONResponse(*p), nil
		}
		return nil, err
	}
	return UpholdAppeal200JSONResponse{res}, nil
}

// decideAppeal records the viewer's decision on an appeal and presents it, or returns the
// problem that refuses it.
func (s *Server) decideAppeal(ctx context.Context, id string, decision moderation.AppealDecisionKind, body *DecideAppealRequest) (AdminAppealJSONResponse, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil || p != nil {
		return AdminAppealJSONResponse{}, p, err
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return AdminAppealJSONResponse{}, nil, err
	}
	var reason string
	if body != nil {
		reason = deref(body.PublicReason)
	}
	l := s.Localizer(ctx)
	refuse := func(status int, typ, title, detail string) (AdminAppealJSONResponse, *problem, error) {
		p := s.problem(ctx, status, typ, title, detail)
		return AdminAppealJSONResponse{}, &p, nil
	}
	a, err := s.o.AppealModeration.DecideAppeal(ctx, id, decision, reason, v.UserID)
	var textErr *moderation.TextError
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		p := s.notFoundResponse(ctx)
		return AdminAppealJSONResponse{}, &p, nil
	case errors.Is(err, appmoderation.ErrNotAppealable):
		return refuse(http.StatusUnprocessableEntity, ProblemTypeNotAppealable, l.ServerErrorsAppealNotAppealable(), "")
	case errors.Is(err, appmoderation.ErrAppealDecided):
		return refuse(http.StatusConflict, ProblemTypeAppealDecided, l.ServerErrorsAppealDecided(), "")
	case errors.As(err, &textErr) && textErr.Max > 0:
		return refuse(http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: textErr.Max, Name: textErr.Field}))
	case errors.As(err, &textErr):
		return refuse(http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(textErr.Field))
	case err != nil:
		return AdminAppealJSONResponse{}, nil, fmt.Errorf("clientapi: decide appeal %s: %w", id, err)
	}
	res, err := s.appealResponse(ctx, a)
	return res, nil, err
}
