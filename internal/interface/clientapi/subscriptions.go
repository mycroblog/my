// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/notification"
)

// PostSubscriptions runs the viewer's subscriptions to the new posts of accounts (ADR-0123).
// *appnotification.Subscriptions implements it.
type PostSubscriptions interface {
	Subscribe(ctx context.Context, accountID, targetID string) error
	SubscribeWithFollow(ctx context.Context, accountID, targetID string) error
	Unsubscribe(ctx context.Context, accountID, targetID string) error
	Subscribed(ctx context.Context, accountID, targetID string) (bool, error)
	List(ctx context.Context, accountID string, p appnotification.SubscriptionPage) ([]notification.Subscription, error)
}

// subscribed reports whether the viewer accountID subscribes to targetID; false while the
// server keeps no subscriptions.
func (s *Server) subscribed(ctx context.Context, accountID, targetID string) (bool, error) {
	if s.o.Subscriptions == nil {
		return false, nil
	}
	return s.o.Subscriptions.Subscribed(ctx, accountID, targetID)
}

// subscriptionProblem returns the problem that a subscription use case's error means, and false
// for an unexpected error.
func (s *Server) subscriptionProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	var detail string
	switch {
	case errors.Is(err, notification.ErrSelfSubscription):
		detail = l.ServerErrorsSelfRelationship()
	case errors.Is(err, appnotification.ErrRemoteNotFollowed):
		detail = l.ServerErrorsSubscriptionRequiresFollow()
	case errors.Is(err, appnotification.ErrSubscriptionBlocked):
		detail = l.ServerErrorsSubscriptionBlocked()
	case errors.Is(err, notification.ErrSubscriptionLimit):
		detail = l.ServerErrorsSubscriptionLimit()
	default:
		return problem{}, false
	}
	return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail), true
}

// changeSubscription subscribes the viewer to targetID (on) or ends the subscription, and
// returns the relationship then.
func (s *Server) changeSubscription(ctx context.Context, v Viewer, targetID string, on bool) (appgraph.Relationship, error) {
	if s.o.Subscriptions == nil {
		return appgraph.Relationship{}, errSubscriptionsOff
	}
	var err error
	if on {
		err = s.o.Subscriptions.Subscribe(ctx, v.AccountID, targetID)
	} else {
		err = s.o.Subscriptions.Unsubscribe(ctx, v.AccountID, targetID)
	}
	if err != nil {
		return appgraph.Relationship{}, err
	}
	return s.o.Graph.Relationship(ctx, v.AccountID, targetID)
}

// errSubscriptionsOff answers the subscription operations 404 while the server keeps no
// subscriptions.
var errSubscriptionsOff = errors.New("clientapi: no subscriptions")

// SubscribeAccount serves POST /api/v1/accounts/{id}/subscribe.
func (s *Server) SubscribeAccount(ctx context.Context, req SubscribeAccountRequestObject) (SubscribeAccountResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.changeSubscription(ctx, v, req.Id, true)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return SubscribeAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return SubscribeAccount200JSONResponse{*res}, nil
}

// UnsubscribeAccount serves POST /api/v1/accounts/{id}/unsubscribe.
func (s *Server) UnsubscribeAccount(ctx context.Context, req UnsubscribeAccountRequestObject) (UnsubscribeAccountResponseObject, error) {
	res, p, err := s.relate(ctx, req.Id, func(v Viewer) (appgraph.Relationship, error) {
		return s.changeSubscription(ctx, v, req.Id, false)
	})
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UnsubscribeAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnsubscribeAccount200JSONResponse{*res}, nil
}

// subscriptionCursorSeparator separates the creation time and the target of a subscription
// cursor.
const subscriptionCursorSeparator = " "

func encodeSubscriptionCursor(sub notification.Subscription) *Cursor {
	return encodeCursor(sub.CreatedAt.UTC().Format(time.RFC3339Nano) + subscriptionCursorSeparator + sub.TargetAccountID)
}

func decodeSubscriptionCursor(position string) (appnotification.SubscriptionCursor, bool) {
	at, target, ok := strings.Cut(position, subscriptionCursorSeparator)
	if !ok || !kernel.ValidID(target) {
		return appnotification.SubscriptionCursor{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return appnotification.SubscriptionCursor{}, false
	}
	return appnotification.SubscriptionCursor{CreatedAt: t, TargetAccountID: target}, true
}

// ListSubscriptions serves GET /api/v1/subscriptions.
func (s *Server) ListSubscriptions(ctx context.Context, req ListSubscriptionsRequestObject) (ListSubscriptionsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Subscriptions == nil {
		return ListSubscriptionsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	q := s.parseOpaquePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListSubscriptionsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	var before appnotification.SubscriptionCursor
	if q.before != "" {
		var ok bool
		if before, ok = decodeSubscriptionCursor(q.before); !ok {
			return ListSubscriptionsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
	}
	subs, err := s.o.Subscriptions.List(ctx, v.AccountID, appnotification.SubscriptionPage{Before: before, Limit: q.limit + 1})
	if err != nil {
		return nil, err
	}
	subs, last := trim(subs, q.limit)
	ids := make([]string, len(subs))
	for i, sub := range subs {
		ids[i] = sub.TargetAccountID
	}
	accounts, err := s.presentAccounts(ctx, &v, ids, true)
	if err != nil {
		return nil, err
	}
	page := AccountPage{Accounts: accounts}
	if last >= 0 {
		page.NextCursor = encodeSubscriptionCursor(subs[last])
	}
	return ListSubscriptions200JSONResponse{ownPageResponse(page)}, nil
}
