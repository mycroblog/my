// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/notification"
)

// WebPush runs the use cases of the Web Push subscription of the viewer's grant (FR-206).
// *appnotification.PushService implements it.
type WebPush interface {
	// ServerKey returns the VAPID public key; "" when the instance sends no Web Push.
	ServerKey() string
	Subscription(ctx context.Context, tokenFamilyID string) (notification.PushSubscription, error)
	Subscribe(ctx context.Context, in appnotification.SubscribeInput) (notification.PushSubscription, error)
	Unsubscribe(ctx context.Context, tokenFamilyID string) error
}

// pushServerKey returns the VAPID public key, "" without Options.Push.
func (s *Server) pushServerKey() string {
	if s.o.Push == nil {
		return ""
	}
	return s.o.Push.ServerKey()
}

// notFoundProblem is notFound as a problem of any operation.
func (s *Server) notFoundProblem(ctx context.Context) problem {
	body, headers := s.notFound(ctx)
	return problem{Body: body, Headers: headers, StatusCode: http.StatusNotFound}
}

func (s *Server) newPushSubscription(sub notification.PushSubscription) PushSubscription {
	alerts := make([]NotificationType, len(sub.Alerts))
	for i, t := range sub.Alerts {
		alerts[i] = NotificationType(t)
	}
	return PushSubscription{Id: sub.ID, Endpoint: sub.Endpoint, Alerts: alerts, ServerKey: s.pushServerKey()}
}

// GetPushSubscription serves GET /api/v1/push/subscription.
func (s *Server) GetPushSubscription(ctx context.Context, _ GetPushSubscriptionRequestObject) (GetPushSubscriptionResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Push == nil {
		return GetPushSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundProblem(ctx)), nil
	}
	sub, err := s.o.Push.Subscription(ctx, v.FamilyID)
	if errors.Is(err, appnotification.ErrNotFound) {
		return GetPushSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundProblem(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the push subscription of %s: %w", v.FamilyID, err)
	}
	cache := cacheNoStore
	return GetPushSubscription200JSONResponse{Body: s.newPushSubscription(sub), Headers: GetPushSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// PutPushSubscription serves PUT /api/v1/push/subscription.
func (s *Server) PutPushSubscription(ctx context.Context, req PutPushSubscriptionRequestObject) (PutPushSubscriptionResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Push == nil || req.Body == nil {
		return PutPushSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundProblem(ctx)), nil
	}
	b := req.Body
	in := appnotification.SubscribeInput{
		AccountID: v.AccountID, TokenFamilyID: v.FamilyID, Endpoint: b.Endpoint, KeyP256DH: b.Keys.P256dh, KeyAuth: b.Keys.Auth,
	}
	for _, t := range b.Alerts {
		// The moderator notifications stay with the first-party clients, as in the list.
		if _, moderator := notification.ModeratorPermission(notification.Type(t)); moderator && !v.FirstParty {
			continue
		}
		in.Alerts = append(in.Alerts, notification.Type(t))
	}
	sub, err := s.o.Push.Subscribe(ctx, in)
	property := ""
	switch {
	case errors.Is(err, appnotification.ErrPushUnavailable):
		return PutPushSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundProblem(ctx)), nil
	case errors.Is(err, notification.ErrInvalidPushEndpoint):
		property = "endpoint"
	case errors.Is(err, notification.ErrInvalidPushKey):
		property = "keys"
	case errors.Is(err, notification.ErrInvalidType):
		property = "alerts"
	case err != nil:
		return nil, fmt.Errorf("clientapi: register the push subscription of %s: %w", v.FamilyID, err)
	}
	if property != "" {
		l := s.Localizer(ctx)
		body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(property))
		return PutPushSubscriptiondefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusUnprocessableEntity}, nil
	}
	cache := cacheNoStore
	return PutPushSubscription200JSONResponse{Body: s.newPushSubscription(sub), Headers: PutPushSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// DeletePushSubscription serves DELETE /api/v1/push/subscription.
func (s *Server) DeletePushSubscription(ctx context.Context, _ DeletePushSubscriptionRequestObject) (DeletePushSubscriptionResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Push != nil {
		if err := s.o.Push.Unsubscribe(ctx, v.FamilyID); err != nil {
			return nil, fmt.Errorf("clientapi: delete the push subscription of %s: %w", v.FamilyID, err)
		}
	}
	return DeletePushSubscription204Response{}, nil
}
