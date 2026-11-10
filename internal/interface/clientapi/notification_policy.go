// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/notification"
)

// NotificationPolicies reads and changes the viewer's notification policy, and runs the requests
// of the notifications it filtered (ADR-0158). *appnotification.Service implements it.
type NotificationPolicies interface {
	Policy(ctx context.Context, accountID string) (notification.Policy, error)
	// UpdatePolicy fails with notification.ErrInvalidPolicy for an unknown condition or action.
	UpdatePolicy(ctx context.Context, accountID string, choices []notification.PolicyChoice) (notification.Policy, error)
	Requests(ctx context.Context, accountID, before string, limit int) ([]notification.Request, error)
	// Request, RequestNotifications, AcceptRequest, and DismissRequest fail with
	// appnotification.ErrNotFound when the source has no request.
	Request(ctx context.Context, accountID, sourceID string) (notification.Request, error)
	RequestSummary(ctx context.Context, accountID string) (notification.RequestSummary, error)
	RequestNotifications(ctx context.Context, accountID, sourceID, before string, limit int) ([]notification.Notification, error)
	AcceptRequest(ctx context.Context, accountID, sourceID string) error
	DismissRequest(ctx context.Context, accountID, sourceID string) error
}

func newNotificationPolicy(p notification.Policy) NotificationPolicy {
	action := func(c notification.Condition) NotificationPolicyAction { return NotificationPolicyAction(p.Action(c)) }
	return NotificationPolicy{
		NotFollowing:              action(notification.ConditionNotFollowing),
		NotFollower:               action(notification.ConditionNotFollower),
		NewAccount:                action(notification.ConditionNewAccount),
		UnsolicitedPrivateMention: action(notification.ConditionUnsolicitedPrivateMention),
		LimitedAccount:            action(notification.ConditionLimitedAccount),
	}
}

// GetNotificationPolicy serves GET /api/v1/preferences/notification_policy.
func (s *Server) GetNotificationPolicy(ctx context.Context, _ GetNotificationPolicyRequestObject) (GetNotificationPolicyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPolicies == nil {
		body, headers := s.notFound(ctx)
		return GetNotificationPolicydefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	p, err := s.o.NotificationPolicies.Policy(ctx, v.AccountID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the notification policy of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return GetNotificationPolicy200JSONResponse{Body: newNotificationPolicy(p), Headers: GetNotificationPolicy200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdateNotificationPolicy serves PATCH /api/v1/preferences/notification_policy.
func (s *Server) UpdateNotificationPolicy(ctx context.Context, req UpdateNotificationPolicyRequestObject) (UpdateNotificationPolicyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPolicies == nil {
		body, headers := s.notFound(ctx)
		return UpdateNotificationPolicydefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	var choices []notification.PolicyChoice
	if b := req.Body; b != nil {
		for _, c := range []struct {
			condition notification.Condition
			action    *NotificationPolicyAction
		}{
			{notification.ConditionNotFollowing, b.NotFollowing},
			{notification.ConditionNotFollower, b.NotFollower},
			{notification.ConditionNewAccount, b.NewAccount},
			{notification.ConditionUnsolicitedPrivateMention, b.UnsolicitedPrivateMention},
			{notification.ConditionLimitedAccount, b.LimitedAccount},
		} {
			if c.action != nil {
				choices = append(choices, notification.PolicyChoice{Condition: c.condition, Action: notification.Action(*c.action)})
			}
		}
	}
	p, err := s.o.NotificationPolicies.UpdatePolicy(ctx, v.AccountID, choices)
	if errors.Is(err, notification.ErrInvalidPolicy) {
		l := s.Localizer(ctx)
		body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("action"))
		return UpdateNotificationPolicydefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusUnprocessableEntity}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update the notification policy of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return UpdateNotificationPolicy200JSONResponse{Body: newNotificationPolicy(p), Headers: UpdateNotificationPolicy200ResponseHeaders{CacheControl: &cache}}, nil
}

// ListNotificationRequests serves GET /api/v1/notifications/requests: a page of the viewer's
// requests, the newest notification first. The cursor is the newest notification of the last
// request of the previous page.
func (s *Server) ListNotificationRequests(ctx context.Context, req ListNotificationRequestsRequestObject) (ListNotificationRequestsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPolicies == nil {
		body, headers := s.notFound(ctx)
		return ListNotificationRequestsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	limit, ok := pageLimit(req.Params.Limit)
	if !ok || !validCursor(req.Params.Cursor) {
		name := "limit"
		if ok {
			name = "cursor"
		}
		body, headers := s.invalidParameter(ctx, name)
		return ListNotificationRequestsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusBadRequest}, nil
	}
	var before string
	if req.Params.Cursor != nil {
		before = *req.Params.Cursor
	}
	// One more tells whether a next page exists.
	reqs, err := s.o.NotificationPolicies.Requests(ctx, v.AccountID, before, limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the notification requests of %s: %w", v.AccountID, err)
	}
	page := NotificationRequestPage{Items: []NotificationRequest{}}
	if len(reqs) > limit {
		reqs = reqs[:limit]
		next := reqs[limit-1].LastNotificationID
		page.NextCursor = &next
	}
	items, err := s.newNotificationRequests(ctx, v, reqs)
	if err != nil {
		return nil, err
	}
	page.Items = append(page.Items, items...)
	cache := cacheNoStore
	return ListNotificationRequests200JSONResponse{Body: page, Headers: ListNotificationRequests200ResponseHeaders{CacheControl: &cache}}, nil
}

// newNotificationRequests presents reqs in order as the NotificationRequest schema, each with its
// source account and newest notification; a request whose source account is gone is left out.
func (s *Server) newNotificationRequests(ctx context.Context, v Viewer, reqs []notification.Request) ([]NotificationRequest, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	ids := make([]string, len(reqs))
	lasts := make([]notification.Notification, len(reqs))
	for i, r := range reqs {
		ids[i], lasts[i] = r.SourceAccountID, r.Last
	}
	// The source accounts, read together, as the notifications present theirs.
	actors, err := s.o.Accounts.FindByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the accounts of the notification requests of %s: %w", v.AccountID, err)
	}
	found := make([]appaccount.Actor, 0, len(actors))
	for _, id := range ids {
		if a, ok := actors[id]; ok {
			found = append(found, a)
		}
	}
	parts, err := s.accountParts(ctx, found...)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Account, len(found))
	for id, a := range actors {
		byID[id] = s.newAccount(a, parts)
	}
	notified, err := s.newNotifications(ctx, v, lasts)
	if err != nil {
		return nil, err
	}
	out := make([]NotificationRequest, 0, len(reqs))
	for i, r := range reqs {
		a, ok := byID[r.SourceAccountID]
		if !ok {
			continue
		}
		item := NotificationRequest{Id: r.SourceAccountID, Account: a, NotificationsCount: r.NotificationsCount}
		if i < len(notified) && notified[i].Id == r.LastNotificationID {
			last := notified[i]
			item.LastNotification = &last
		}
		out = append(out, item)
	}
	return out, nil
}

// GetNotificationRequestSummary serves GET /api/v1/notifications/requests/summary.
func (s *Server) GetNotificationRequestSummary(ctx context.Context, _ GetNotificationRequestSummaryRequestObject) (GetNotificationRequestSummaryResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPolicies == nil {
		body, headers := s.notFound(ctx)
		return GetNotificationRequestSummarydefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	sum, err := s.o.NotificationPolicies.RequestSummary(ctx, v.AccountID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: count the notification requests of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return GetNotificationRequestSummary200JSONResponse{
		Body:    NotificationRequestSummary{AccountsCount: sum.Accounts, NotificationsCount: sum.Notifications},
		Headers: GetNotificationRequestSummary200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// GetNotificationRequest serves GET /api/v1/notifications/requests/{id}.
func (s *Server) GetNotificationRequest(ctx context.Context, req GetNotificationRequestRequestObject) (GetNotificationRequestResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	notFound := func() (GetNotificationRequestResponseObject, error) {
		body, headers := s.notFound(ctx)
		return GetNotificationRequestdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if s.o.NotificationPolicies == nil {
		return notFound()
	}
	r, err := s.o.NotificationPolicies.Request(ctx, v.AccountID, knownID(req.Id))
	if errors.Is(err, appnotification.ErrNotFound) {
		return notFound()
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the notification request of %s from %s: %w", v.AccountID, req.Id, err)
	}
	items, err := s.newNotificationRequests(ctx, v, []notification.Request{r})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return notFound()
	}
	cache := cacheNoStore
	return GetNotificationRequest200JSONResponse{Body: items[0], Headers: GetNotificationRequest200ResponseHeaders{CacheControl: &cache}}, nil
}

// ListNotificationRequestNotifications serves GET /api/v1/notifications/requests/{id}/notifications:
// a page of the notifications that wait in the request, newest first.
func (s *Server) ListNotificationRequestNotifications(ctx context.Context, req ListNotificationRequestNotificationsRequestObject) (ListNotificationRequestNotificationsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	notFound := func() (ListNotificationRequestNotificationsResponseObject, error) {
		body, headers := s.notFound(ctx)
		return ListNotificationRequestNotificationsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if s.o.NotificationPolicies == nil {
		return notFound()
	}
	limit, ok := pageLimit(req.Params.Limit)
	if !ok || !validCursor(req.Params.Cursor) {
		name := "limit"
		if ok {
			name = "cursor"
		}
		body, headers := s.invalidParameter(ctx, name)
		return ListNotificationRequestNotificationsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusBadRequest}, nil
	}
	var before string
	if req.Params.Cursor != nil {
		before = *req.Params.Cursor
	}
	ns, err := s.o.NotificationPolicies.RequestNotifications(ctx, v.AccountID, knownID(req.Id), before, limit+1)
	if errors.Is(err, appnotification.ErrNotFound) {
		return notFound()
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the notifications of the request of %s from %s: %w", v.AccountID, req.Id, err)
	}
	page := NotificationPage{Items: []Notification{}}
	if len(ns) > limit {
		ns = ns[:limit]
		next := ns[limit-1].ID
		page.NextCursor = &next
	}
	items, err := s.newNotifications(ctx, v, ns)
	if err != nil {
		return nil, err
	}
	page.Items = append(page.Items, items...)
	cache := cacheNoStore
	return ListNotificationRequestNotifications200JSONResponse{Body: page, Headers: ListNotificationRequestNotifications200ResponseHeaders{CacheControl: &cache}}, nil
}

// AcceptNotificationRequest serves POST /api/v1/notifications/requests/{id}/accept.
func (s *Server) AcceptNotificationRequest(ctx context.Context, req AcceptNotificationRequestRequestObject) (AcceptNotificationRequestResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPolicies != nil {
		err = s.o.NotificationPolicies.AcceptRequest(ctx, v.AccountID, knownID(req.Id))
	}
	if s.o.NotificationPolicies == nil || errors.Is(err, appnotification.ErrNotFound) {
		body, headers := s.notFound(ctx)
		return AcceptNotificationRequestdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: accept the notification request of %s from %s: %w", v.AccountID, req.Id, err)
	}
	return AcceptNotificationRequest204Response{}, nil
}

// DismissNotificationRequest serves POST /api/v1/notifications/requests/{id}/dismiss.
func (s *Server) DismissNotificationRequest(ctx context.Context, req DismissNotificationRequestRequestObject) (DismissNotificationRequestResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPolicies != nil {
		err = s.o.NotificationPolicies.DismissRequest(ctx, v.AccountID, knownID(req.Id))
	}
	if s.o.NotificationPolicies == nil || errors.Is(err, appnotification.ErrNotFound) {
		body, headers := s.notFound(ctx)
		return DismissNotificationRequestdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: dismiss the notification request of %s from %s: %w", v.AccountID, req.Id, err)
	}
	return DismissNotificationRequest204Response{}, nil
}
