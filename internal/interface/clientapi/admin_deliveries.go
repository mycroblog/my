// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The deliveries to a remote server and their attempts (tag admin-moderation). What is masked,
// and when the audit log records a read, is decided by the federation module (DeliveryLog); the
// handlers only present it.

// DeliveryLog lists the deliveries to a remote server and reveals the masked fields of one.
// *appfederation.DeliveryLog implements it.
type DeliveryLog interface {
	List(ctx context.Context, q appfederation.DeliveryQuery, byUserID string) ([]appfederation.DeliveryEntry, error)
	Reveal(ctx context.Context, domain, id, byUserID string) (appfederation.RevealedDelivery, error)
}

// ListInstanceDeliveries serves GET /api/v1/admin/instances/{domain}/deliveries, newest first.
// The cursor is the ID of the last delivery of the previous page.
func (s *Server) ListInstanceDeliveries(ctx context.Context, req ListInstanceDeliveriesRequestObject) (ListInstanceDeliveriesResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListInstanceDeliveriesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListInstanceDeliveriesdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	domain, p := s.instanceDomain(ctx, req.Domain)
	if p != nil {
		return ListInstanceDeliveriesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := appfederation.DeliveryQuery{Domain: domain, Before: pq.before, Limit: pq.limit + 1}
	if a := req.Params.AccountId; a != nil {
		if !kernel.ValidID(*a) {
			return ListInstanceDeliveriesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "account_id")), nil
		}
		q.ActorAccountID = *a
	}
	if u := req.Params.ActivityUri; u != nil {
		q.ActivityURI = federation.IRI(*u)
	}
	if s.o.DeliveryLog == nil {
		return ListInstanceDeliveriesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	entries, err := s.o.DeliveryLog.List(ctx, q, v.UserID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return ListInstanceDeliveriesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appfederation.ErrNotLocalAccount):
		return ListInstanceDeliveriesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "account_id")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: deliveries to %s: %w", domain, err)
	}
	entries, last := trim(entries, pq.limit)
	page := AdminDeliveryPage{Items: make([]AdminDelivery, 0, len(entries))}
	for _, e := range entries {
		page.Items = append(page.Items, newAdminDelivery(e))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(entries[last].ID)
	}
	cache := cacheNoStore
	return ListInstanceDeliveries200JSONResponse{Body: page, Headers: ListInstanceDeliveries200ResponseHeaders{CacheControl: &cache}}, nil
}

// RevealInstanceDelivery serves POST /api/v1/admin/instances/{domain}/deliveries/{id}/reveal.
func (s *Server) RevealInstanceDelivery(ctx context.Context, req RevealInstanceDeliveryRequestObject) (RevealInstanceDeliveryResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return RevealInstanceDeliverydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	domain, p := s.instanceDomain(ctx, req.Domain)
	if p != nil {
		return RevealInstanceDeliverydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if !kernel.ValidID(req.Id) {
		return RevealInstanceDeliverydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if s.o.DeliveryLog == nil {
		return RevealInstanceDeliverydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	r, err := s.o.DeliveryLog.Reveal(ctx, domain, req.Id, v.UserID)
	switch {
	case errors.Is(err, appfederation.ErrNotFound):
		return RevealInstanceDeliverydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: reveal delivery %s: %w", req.Id, err)
	}
	out := AdminDeliveryReveal{
		Id: r.ID, Sender: newAdminDeliverySender(r.Sender), ActivityUri: string(r.ActivityURI), InboxUrl: string(r.InboxURL),
	}
	if r.Mask != "" {
		m := AdminDeliveryMask(r.Mask)
		out.Mask = &m
	}
	cache := cacheNoStore
	return RevealInstanceDelivery200JSONResponse{Body: out, Headers: RevealInstanceDelivery200ResponseHeaders{CacheControl: &cache}}, nil
}

func newAdminDeliverySender(s appfederation.DeliverySender) AdminDeliverySender {
	out := AdminDeliverySender{AccountId: s.AccountID}
	if s.Username != "" {
		u := s.Username
		out.Username = &u
	}
	return out
}

func newAdminDelivery(e appfederation.DeliveryEntry) AdminDelivery {
	out := AdminDelivery{
		Id: e.ID, Domain: e.Domain, ActivityType: string(e.ActivityType), ActivityCreatedAt: e.ActivityAt.UTC(),
		CreatedAt: e.CreatedAt.UTC(), SharedInbox: e.SharedInbox, Attempts: make([]AdminDeliveryAttempt, 0, len(e.Attempts)),
	}
	if e.Mask != "" {
		m := AdminDeliveryMask(e.Mask)
		out.Mask = &m
	}
	if e.Sender != nil {
		sender := newAdminDeliverySender(*e.Sender)
		out.Sender = &sender
	}
	if e.ActivityURI != "" {
		u := string(e.ActivityURI)
		out.ActivityUri = &u
	}
	if e.InboxURL != "" {
		u := string(e.InboxURL)
		out.InboxUrl = &u
	}
	for _, a := range e.Attempts {
		out.Attempts = append(out.Attempts, newAdminDeliveryAttempt(a))
	}
	return out
}

func newAdminDeliveryAttempt(a federation.DeliveryAttempt) AdminDeliveryAttempt {
	out := AdminDeliveryAttempt{Id: a.ID, Outcome: AdminDeliveryAttemptOutcome(a.Outcome), AttemptedAt: a.AttemptedAt.UTC()}
	if a.Scheme != "" {
		scheme := AdminDeliveryAttemptSignatureScheme(a.Scheme)
		out.SignatureScheme = &scheme
	}
	if a.HTTPStatus != 0 {
		code := a.HTTPStatus
		out.StatusCode = &code
	}
	if a.Error != "" {
		msg := a.Error
		out.Error = &msg
	}
	return out
}
