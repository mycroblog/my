// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The admin operations of the actions on accounts (FR-703, manage_accounts). The moderation
// module records them; issuing or lifting one changes at once what the account may do and who
// sees it (data-model §5.2).

// AccountModeration runs the moderators' use cases on accounts. *appmoderation.Service
// implements it.
type AccountModeration interface {
	AccountActions(ctx context.Context, f appmoderation.AccountActionFilter, limit int) ([]appmoderation.AccountActionView, error)
	AccountAction(ctx context.Context, id string) (appmoderation.AccountActionView, error)
	IssueAccountAction(ctx context.Context, in appmoderation.AccountActionInput) (moderation.Action, error)
	LiftAccountAction(ctx context.Context, actionID, liftedByUserID, reason string) error
}

// newAdminAccountActions presents actions on accounts, with the accounts of their moderators
// read once each.
func (s *Server) newAdminAccountActions(ctx context.Context, vs []appmoderation.AccountActionView) ([]AdminAccountAction, error) {
	return s.newAdminAccountActionsWith(ctx, s.reads(), vs)
}

// newAdminAccountActionsWith is newAdminAccountActions through reads.
func (s *Server) newAdminAccountActionsWith(ctx context.Context, reads presentReads, vs []appmoderation.AccountActionView) ([]AdminAccountAction, error) {
	moderators := map[string]string{}
	for _, v := range vs {
		u := v.Action.IssuedByUserID
		if _, ok := moderators[u]; ok || u == "" {
			continue
		}
		la, err := reads.LocalAccountOfUser(ctx, u)
		if err != nil && !errors.Is(err, appaccount.ErrNotFound) {
			return nil, fmt.Errorf("clientapi: the account of user %s: %w", u, err)
		}
		moderators[u] = la.AccountID
	}
	out := make([]AdminAccountAction, 0, len(vs))
	for _, v := range vs {
		a := v.Action
		item := AdminAccountAction{
			Id: a.ID, AccountId: a.TargetAccountID, Kind: AdminAccountActionKind(a.Kind),
			ReasonCategory: AdminAccountActionCategory(a.ReasonCategory), Issuer: AdminAccountActionIssuer(a.Issuer),
			NotifyTarget: a.NotifyTarget, State: AdminAccountActionState(v.State), IssuedAt: a.IssuedAt.UTC(),
		}
		if a.PublicReason != "" {
			item.PublicReason = &a.PublicReason
		}
		if a.PrivateNote != "" {
			item.PrivateNote = &a.PrivateNote
		}
		if a.ReportID != "" {
			item.ReportId = &a.ReportID
		}
		if id := moderators[a.IssuedByUserID]; id != "" {
			item.IssuedByAccountId = &id
		}
		if a.ExpiresAt != nil {
			t := a.ExpiresAt.UTC()
			item.ExpiresAt = &t
		}
		if len(a.StatusIDs) > 0 {
			ids := append([]string{}, a.StatusIDs...)
			item.StatusIds = &ids
		}
		if l := v.Lift; l != nil {
			t := l.LiftedAt.UTC()
			item.LiftedAt = &t
			if l.Reason != "" {
				item.LiftReason = &l.Reason
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// accountActionResponse answers an operation on one action on an account.
func (s *Server) accountActionResponse(ctx context.Context, id string) (AdminAccountActionJSONResponse, error) {
	v, err := s.o.AccountModeration.AccountAction(ctx, id)
	if err != nil {
		return AdminAccountActionJSONResponse{}, fmt.Errorf("clientapi: account action %s: %w", id, err)
	}
	items, err := s.newAdminAccountActions(ctx, []appmoderation.AccountActionView{v})
	if err != nil {
		return AdminAccountActionJSONResponse{}, err
	}
	cache := cacheNoStore
	return AdminAccountActionJSONResponse{Body: items[0], Headers: AdminAccountActionResponseHeaders{CacheControl: &cache}}, nil
}

// adminAccount returns the account an admin operation names, local or remote; ok is false when
// there is none.
func (s *Server) adminAccount(ctx context.Context, id string) (appaccount.Actor, bool, error) {
	a, err := s.o.Accounts.FindByID(ctx, id)
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, false, nil
	}
	if err != nil {
		return appaccount.Actor{}, false, fmt.Errorf("clientapi: account %s: %w", id, err)
	}
	return a, true, nil
}

// ListAccountActions serves GET /api/v1/admin/accounts/{id}/actions, newest first. The cursor
// is the ID of the last action of the previous page.
func (s *Server) ListAccountActions(ctx context.Context, req ListAccountActionsRequestObject) (ListAccountActionsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return ListAccountActionsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListAccountActionsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	if _, ok, err := s.adminAccount(ctx, req.Id); err != nil {
		return nil, err
	} else if !ok {
		return ListAccountActionsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	f := appmoderation.AccountActionFilter{AccountID: req.Id, Before: q.before, Active: req.Params.Active != nil && *req.Params.Active}
	vs, err := s.o.AccountModeration.AccountActions(ctx, f, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the actions on account %s: %w", req.Id, err)
	}
	vs, last := trim(vs, q.limit)
	items, err := s.newAdminAccountActions(ctx, vs)
	if err != nil {
		return nil, err
	}
	page := AdminAccountActionPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(vs[last].Action.ID)
	}
	cache := cacheNoStore
	return ListAccountActions200JSONResponse{Body: page, Headers: ListAccountActions200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateAccountAction serves POST /api/v1/admin/accounts/{id}/actions.
func (s *Server) CreateAccountAction(ctx context.Context, req CreateAccountActionRequestObject) (CreateAccountActionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return CreateAccountActiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (CreateAccountActionResponseObject, error) {
		return CreateAccountActiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	b := req.Body
	if b == nil {
		return CreateAccountActiondefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "kind")), nil
	}
	in := appmoderation.AccountActionInput{
		TargetAccountID: req.Id, Kind: moderation.ActionKind(b.Kind), ReasonCategory: moderation.CategoryOther,
		NotifyTarget: b.NotifyTarget == nil || *b.NotifyTarget, IssuedByUserID: v.UserID,
	}
	if !in.Kind.Valid() {
		return unprocessable(l.ServerErrorsInvalidValue("kind"))
	}
	if b.ReasonCategory != nil {
		if in.ReasonCategory = moderation.Category(*b.ReasonCategory); !in.ReasonCategory.Valid() {
			return unprocessable(l.ServerErrorsInvalidValue("reason_category"))
		}
	}
	in.PublicReason, in.PrivateNote, in.ReportID = deref(b.PublicReason), deref(b.PrivateNote), knownID(deref(b.ReportId))
	if b.ExpiresIn != nil {
		if !moderation.ValidDurationSeconds(int64(*b.ExpiresIn)) {
			return unprocessable(l.ServerErrorsInvalidValue("expires_in"))
		}
		in.Duration = time.Duration(*b.ExpiresIn) * time.Second
	}
	if b.StatusIds != nil {
		in.StatusIDs = knownIDs(*b.StatusIds)
	}
	in.BlockEmail = b.BlockEmail != nil && *b.BlockEmail
	target, ok, err := s.adminAccount(ctx, req.Id)
	switch {
	case err != nil:
		return nil, err
	case !ok:
		return CreateAccountActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case target.IsInstanceActor():
		return unprocessable(l.ServerErrorsInvalidValue("id"))
	}
	a, err := s.o.AccountModeration.IssueAccountAction(ctx, in)
	var textErr *moderation.TextError
	switch {
	case errors.Is(err, appmoderation.ErrAccountNotFound):
		return CreateAccountActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appmoderation.ErrInvalidReportReference):
		return unprocessable(l.ServerErrorsInvalidValue("report_id"))
	case errors.Is(err, appmoderation.ErrInvalidStatusReference), errors.Is(err, moderation.ErrInvalidActionStatuses):
		return unprocessable(l.ServerErrorsInvalidValue("status_ids"))
	case errors.Is(err, moderation.ErrActionNeverExpires):
		return unprocessable(l.ServerErrorsInvalidValue("expires_in"))
	case errors.Is(err, appmoderation.ErrCannotBlockEmail):
		return unprocessable(l.ServerErrorsInvalidValue("block_email"))
	case errors.As(err, &textErr) && textErr.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(textErr.Max, textErr.Field))
	case errors.As(err, &textErr):
		return unprocessable(l.ServerErrorsInvalidValue(textErr.Field))
	case errors.Is(err, appmoderation.ErrUnsupportedAction), errors.Is(err, moderation.ErrInvalidAction):
		return unprocessable(l.ServerErrorsInvalidValue("kind"))
	case err != nil:
		return nil, fmt.Errorf("clientapi: issue an action on account %s: %w", req.Id, err)
	}
	res, err := s.accountActionResponse(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	return CreateAccountAction200JSONResponse{res}, nil
}

// GetAccountAction serves GET /api/v1/admin/account_actions/{id}.
func (s *Server) GetAccountAction(ctx context.Context, req GetAccountActionRequestObject) (GetAccountActionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return GetAccountActiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	res, err := s.accountActionResponse(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return GetAccountActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, err
	}
	return GetAccountAction200JSONResponse{res}, nil
}

// LiftAccountAction serves POST /api/v1/admin/account_actions/{id}/lift.
func (s *Server) LiftAccountAction(ctx context.Context, req LiftAccountActionRequestObject) (LiftAccountActionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return LiftAccountActiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	var reason string
	if req.Body != nil {
		reason = deref(req.Body.Reason)
	}
	err = s.o.AccountModeration.LiftAccountAction(ctx, req.Id, v.UserID, reason)
	var textErr *moderation.TextError
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		return LiftAccountActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appmoderation.ErrNotLiftable):
		return LiftAccountActiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("id"))), nil
	case errors.As(err, &textErr):
		detail := l.ServerErrorsInvalidValue(textErr.Field)
		if textErr.Max > 0 {
			detail = l.ServerErrorsTooLong(textErr.Max, textErr.Field)
		}
		return LiftAccountActiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: lift account action %s: %w", req.Id, err)
	}
	res, err := s.accountActionResponse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return LiftAccountAction200JSONResponse{res}, nil
}
