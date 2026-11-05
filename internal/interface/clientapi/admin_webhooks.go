// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// The instance's webhooks (ADR-0060, FR-722): the admin operations on /admin/webhooks
// (manage_webhooks, ADR-0058 server administration).

// ProblemTypeInvalidWebhook is the problem type of a webhook that breaks a rule (422), whose
// reason says which.
const ProblemTypeInvalidWebhook = "/api/v1/problems/invalid-webhook"

// Webhooks runs the use cases of the webhooks. *appwebhook.Service implements it.
type Webhooks interface {
	List(ctx context.Context) ([]webhook.Endpoint, error)
	Get(ctx context.Context, id string) (webhook.Endpoint, error)
	Create(ctx context.Context, settings webhook.EndpointSettings, byUserID string) (appwebhook.Created, error)
	Update(ctx context.Context, id string, settings webhook.EndpointSettings, byUserID string) (webhook.Endpoint, error)
	Enable(ctx context.Context, id, byUserID string) (webhook.Endpoint, error)
	Disable(ctx context.Context, id, byUserID string) (webhook.Endpoint, error)
	Delete(ctx context.Context, id, byUserID string) error
	RotateSecret(ctx context.Context, id, byUserID string) (string, error)
	Test(ctx context.Context, id string) (webhook.DeliveryRecord, error)
	Deliveries(ctx context.Context, endpointID, before string, limit int) ([]webhook.DeliveryRecord, error)
	Redeliver(ctx context.Context, endpointID, deliveryID string) (webhook.DeliveryRecord, error)
}

// requireWebhooks checks the manage_webhooks permission and that the use cases are configured;
// it returns the problem to answer otherwise, and the viewer.
func (s *Server) requireWebhooks(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageWebhooks); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.Webhooks == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

// newAdminWebhook presents an endpoint to the administrators.
func newAdminWebhook(e webhook.Endpoint) AdminWebhook {
	out := AdminWebhook{
		Id: e.ID, Url: e.Settings.URL, Events: make([]WebhookEventType, 0, len(e.Settings.Events)),
		State: AdminWebhookState(e.State), StateReason: AdminWebhookStateReason(e.StateReason),
		StateChangedAt: e.StateChangedAt.UTC(), CreatedAt: e.CreatedAt.UTC(), RevisedAt: e.RevisedAt.UTC(),
	}
	if e.Settings.Description != "" {
		out.Description = &e.Settings.Description
	}
	for _, ev := range e.Settings.Events {
		out.Events = append(out.Events, WebhookEventType(ev))
	}
	if f := e.Failures; f != nil {
		out.Failure = &AdminWebhookFailure{ConsecutiveFailures: f.Consecutive, FirstFailedAt: f.FirstFailureAt.UTC()}
	}
	return out
}

// newAdminWebhookDelivery presents a delivery to the administrators.
func newAdminWebhookDelivery(d webhook.DeliveryRecord) AdminWebhookDelivery {
	out := AdminWebhookDelivery{
		Id: d.Delivery.ID, MessageId: d.Delivery.MessageID, EventType: WebhookMessageType(d.EventType),
		Status: AdminWebhookDeliveryStatus(d.Status()), Attempts: d.Attempts, CreatedAt: d.Delivery.CreatedAt.UTC(),
	}
	if a := d.Last; a != nil {
		out.LastAttempt = &AdminWebhookAttempt{
			Outcome: AdminWebhookAttemptOutcome(a.Outcome), DurationMs: int(a.Duration.Milliseconds()), AttemptedAt: a.AttemptedAt.UTC(),
		}
		if a.StatusCode != 0 {
			code := a.StatusCode
			out.LastAttempt.StatusCode = &code
		}
	}
	return out
}

func adminWebhookResponse(e webhook.Endpoint) AdminWebhookJSONResponse {
	cache := cacheNoStore
	return AdminWebhookJSONResponse{Body: newAdminWebhook(e), Headers: AdminWebhookResponseHeaders{CacheControl: &cache}}
}

func adminWebhookDeliveryResponse(d webhook.DeliveryRecord) AdminWebhookDeliveryJSONResponse {
	cache := cacheNoStore
	return AdminWebhookDeliveryJSONResponse{Body: newAdminWebhookDelivery(d), Headers: AdminWebhookDeliveryResponseHeaders{CacheControl: &cache}}
}

// webhookProblem returns the 422 of a webhook that breaks a rule, and false for another error.
func (s *Server) webhookProblem(ctx context.Context, err error) (WebhookProblemApplicationProblemPlusJSONResponse, bool) {
	var reason WebhookProblemReason
	var missing *appwebhook.MissingPermissionError
	switch {
	case errors.Is(err, webhook.ErrInvalidURL):
		reason = WebhookProblemReasonUrl
	case errors.Is(err, webhook.ErrInvalidDescription):
		reason = WebhookProblemReasonDescription
	case errors.Is(err, webhook.ErrInvalidEvents):
		reason = WebhookProblemReasonEvents
	case errors.As(err, &missing):
		reason = WebhookProblemReasonMissingPermission
	default:
		return WebhookProblemApplicationProblemPlusJSONResponse{}, false
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidWebhook,
		l.ServerErrorsWebhookInvalid(), l.ServerErrorsWebhookInvalidDetail(string(reason)))
	out := WebhookProblem{
		Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
	}
	if missing != nil {
		perms := make([]Permission, len(missing.Missing))
		for i, p := range missing.Missing {
			perms[i] = Permission(p)
		}
		out.MissingPermissions = &perms
	}
	return WebhookProblemApplicationProblemPlusJSONResponse{
		Body: out, Headers: WebhookProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}, true
}

func webhookNotFound(err error) bool { return errors.Is(err, appwebhook.ErrNotFound) }

func webhookEvents(in []WebhookEventType) []webhook.EventType {
	out := make([]webhook.EventType, len(in))
	for i, e := range in {
		out[i] = webhook.EventType(e)
	}
	return out
}

// ListAdminWebhooks serves GET /api/v1/admin/webhooks.
func (s *Server) ListAdminWebhooks(ctx context.Context, _ ListAdminWebhooksRequestObject) (ListAdminWebhooksResponseObject, error) {
	_, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return ListAdminWebhooksdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	list, err := s.o.Webhooks.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list webhooks: %w", err)
	}
	out := AdminWebhookList{Items: make([]AdminWebhook, 0, len(list))}
	for _, e := range list {
		out.Items = append(out.Items, newAdminWebhook(e))
	}
	cache := cacheNoStore
	return ListAdminWebhooks200JSONResponse{Body: out, Headers: ListAdminWebhooks200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateWebhook serves POST /api/v1/admin/webhooks.
func (s *Server) CreateWebhook(ctx context.Context, req CreateWebhookRequestObject) (CreateWebhookResponseObject, error) {
	v, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return CreateWebhookdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return CreateWebhookdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	settings := webhook.EndpointSettings{URL: req.Body.Url, Events: webhookEvents(req.Body.Events)}
	if d := req.Body.Description; d != nil {
		settings.Description = *d
	}
	c, err := s.o.Webhooks.Create(ctx, settings, v.UserID)
	if wp, ok := s.webhookProblem(ctx, err); ok {
		return CreateWebhook422ApplicationProblemPlusJSONResponse{wp}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create webhook: %w", err)
	}
	cache := cacheNoStore
	return CreateWebhook201JSONResponse{
		Body:    AdminWebhookCreated{Webhook: newAdminWebhook(c.Endpoint), Secret: c.Secret},
		Headers: CreateWebhook201ResponseHeaders{CacheControl: &cache},
	}, nil
}

// GetAdminWebhook serves GET /api/v1/admin/webhooks/{id}.
func (s *Server) GetAdminWebhook(ctx context.Context, req GetAdminWebhookRequestObject) (GetAdminWebhookResponseObject, error) {
	_, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetAdminWebhookdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	e, err := s.o.Webhooks.Get(ctx, req.Id)
	switch {
	case webhookNotFound(err):
		return GetAdminWebhookdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: webhook %s: %w", req.Id, err)
	}
	return GetAdminWebhook200JSONResponse{adminWebhookResponse(e)}, nil
}

// UpdateWebhook serves PATCH /api/v1/admin/webhooks/{id}.
func (s *Server) UpdateWebhook(ctx context.Context, req UpdateWebhookRequestObject) (UpdateWebhookResponseObject, error) {
	v, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return UpdateWebhookdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return UpdateWebhookdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	old, err := s.o.Webhooks.Get(ctx, req.Id)
	switch {
	case webhookNotFound(err):
		return UpdateWebhookdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: webhook %s: %w", req.Id, err)
	}
	settings := old.Settings
	if u := req.Body.Url; u != nil {
		settings.URL = *u
	}
	if d := req.Body.Description; d != nil {
		settings.Description = *d
	}
	if ev := req.Body.Events; ev != nil {
		settings.Events = webhookEvents(*ev)
	}
	e, err := s.o.Webhooks.Update(ctx, req.Id, settings, v.UserID)
	if wp, ok := s.webhookProblem(ctx, err); ok {
		return UpdateWebhook422ApplicationProblemPlusJSONResponse{wp}, nil
	}
	switch {
	case webhookNotFound(err):
		return UpdateWebhookdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: update webhook %s: %w", req.Id, err)
	}
	return UpdateWebhook200JSONResponse{adminWebhookResponse(e)}, nil
}

// DeleteWebhook serves DELETE /api/v1/admin/webhooks/{id}.
func (s *Server) DeleteWebhook(ctx context.Context, req DeleteWebhookRequestObject) (DeleteWebhookResponseObject, error) {
	v, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return DeleteWebhookdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.Webhooks.Delete(ctx, req.Id, v.UserID)
	switch {
	case webhookNotFound(err):
		return DeleteWebhookdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: delete webhook %s: %w", req.Id, err)
	}
	return DeleteWebhook204Response{}, nil
}

// EnableWebhook serves POST /api/v1/admin/webhooks/{id}/enable.
func (s *Server) EnableWebhook(ctx context.Context, req EnableWebhookRequestObject) (EnableWebhookResponseObject, error) {
	v, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return EnableWebhookdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	e, err := s.o.Webhooks.Enable(ctx, req.Id, v.UserID)
	switch {
	case webhookNotFound(err):
		return EnableWebhookdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: enable webhook %s: %w", req.Id, err)
	}
	return EnableWebhook200JSONResponse{adminWebhookResponse(e)}, nil
}

// DisableWebhook serves POST /api/v1/admin/webhooks/{id}/disable.
func (s *Server) DisableWebhook(ctx context.Context, req DisableWebhookRequestObject) (DisableWebhookResponseObject, error) {
	v, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return DisableWebhookdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	e, err := s.o.Webhooks.Disable(ctx, req.Id, v.UserID)
	switch {
	case webhookNotFound(err):
		return DisableWebhookdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: disable webhook %s: %w", req.Id, err)
	}
	return DisableWebhook200JSONResponse{adminWebhookResponse(e)}, nil
}

// RotateWebhookSecret serves POST /api/v1/admin/webhooks/{id}/rotate_secret.
func (s *Server) RotateWebhookSecret(ctx context.Context, req RotateWebhookSecretRequestObject) (RotateWebhookSecretResponseObject, error) {
	v, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RotateWebhookSecretdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	secret, err := s.o.Webhooks.RotateSecret(ctx, req.Id, v.UserID)
	switch {
	case webhookNotFound(err):
		return RotateWebhookSecretdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: rotate the secret of webhook %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	return RotateWebhookSecret200JSONResponse{
		Body: AdminWebhookSecret{Secret: secret}, Headers: RotateWebhookSecret200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// TestWebhook serves POST /api/v1/admin/webhooks/{id}/test.
func (s *Server) TestWebhook(ctx context.Context, req TestWebhookRequestObject) (TestWebhookResponseObject, error) {
	_, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return TestWebhookdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	d, err := s.o.Webhooks.Test(ctx, req.Id)
	switch {
	case webhookNotFound(err):
		return TestWebhookdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: test webhook %s: %w", req.Id, err)
	}
	return TestWebhook202JSONResponse{adminWebhookDeliveryResponse(d)}, nil
}

// ListWebhookDeliveries serves GET /api/v1/admin/webhooks/{id}/deliveries.
func (s *Server) ListWebhookDeliveries(ctx context.Context, req ListWebhookDeliveriesRequestObject) (ListWebhookDeliveriesResponseObject, error) {
	_, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return ListWebhookDeliveriesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListWebhookDeliveriesdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	ds, err := s.o.Webhooks.Deliveries(ctx, req.Id, pq.before, pq.limit+1)
	switch {
	case webhookNotFound(err):
		return ListWebhookDeliveriesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: the deliveries of webhook %s: %w", req.Id, err)
	}
	ds, last := trim(ds, pq.limit)
	page := AdminWebhookDeliveryPage{Items: make([]AdminWebhookDelivery, 0, len(ds))}
	for _, d := range ds {
		page.Items = append(page.Items, newAdminWebhookDelivery(d))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(ds[last].Delivery.ID)
	}
	cache := cacheNoStore
	return ListWebhookDeliveries200JSONResponse{Body: page, Headers: ListWebhookDeliveries200ResponseHeaders{CacheControl: &cache}}, nil
}

// RedeliverWebhookDelivery serves POST /api/v1/admin/webhooks/{id}/deliveries/{delivery_id}/redeliver.
func (s *Server) RedeliverWebhookDelivery(ctx context.Context, req RedeliverWebhookDeliveryRequestObject) (RedeliverWebhookDeliveryResponseObject, error) {
	_, p, err := s.requireWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RedeliverWebhookDeliverydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	d, err := s.o.Webhooks.Redeliver(ctx, req.Id, req.DeliveryId)
	switch {
	case webhookNotFound(err):
		return RedeliverWebhookDeliverydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: redeliver %s of webhook %s: %w", req.DeliveryId, req.Id, err)
	}
	return RedeliverWebhookDelivery202JSONResponse{adminWebhookDeliveryResponse(d)}, nil
}
