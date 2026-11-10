// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// The admin operations of the subscriptions to domain block lists (ADR-0159 §4–§6,
// manage_domains). The moderation module fetches the lists, records the fetches, and issues and
// lifts the subscriptions' domain actions, in the automatic mode or as drafts that a moderator
// accepts.

// DomainListSubscriptions runs the moderators' use cases of the subscriptions to domain block
// lists. *appmoderation.Service implements it.
type DomainListSubscriptions interface {
	DomainListSubscriptions(ctx context.Context, before string, limit int) ([]moderation.DomainListSubscriptionRecord, error)
	DomainListSubscription(ctx context.Context, id string) (moderation.DomainListSubscriptionRecord, error)
	CreateDomainListSubscription(ctx context.Context, in appmoderation.DomainListSubscriptionInput) (moderation.DomainListSubscriptionRecord, error)
	UpdateDomainListSubscription(ctx context.Context, id string, settings moderation.DomainListSettings, byUserID string) (moderation.DomainListSubscriptionRecord, error)
	DeleteDomainListSubscription(ctx context.Context, id string, liftActions bool, byUserID string) error
	QueueDomainListFetch(ctx context.Context, id, byUserID string) error
	DomainListFetches(ctx context.Context, id, before string, limit int) ([]moderation.DomainListFetch, error)
	DomainListDrafts(ctx context.Context, id string) ([]moderation.DomainListChange, error)
	AcceptDomainListDrafts(ctx context.Context, id string, keys []string, byUserID string) (appmoderation.DomainListDraftResult, error)
	RejectDomainListDrafts(ctx context.Context, id string, keys []string, byUserID string) (appmoderation.DomainListDraftResult, error)
}

func newAdminDomainListFetch(f moderation.DomainListFetch) AdminDomainListFetch {
	out := AdminDomainListFetch{Id: f.ID, Outcome: AdminDomainListFetchOutcome(f.Outcome), EntryCount: f.EntryCount, FetchedAt: f.FetchedAt.UTC()}
	if f.HTTPStatus != 0 {
		out.HttpStatus = &f.HTTPStatus
	}
	if f.ErrorCode != "" {
		code := AdminDomainListFetchError(f.ErrorCode)
		out.ErrorCode = &code
	}
	return out
}

func newAdminDomainListSubscription(rec moderation.DomainListSubscriptionRecord) AdminDomainListSubscription {
	st := rec.Revision.Settings
	out := AdminDomainListSubscription{
		Id: rec.Subscription.ID, Url: rec.Subscription.URL, Format: AdminDomainListFormat(rec.Subscription.Format), Title: st.Title,
		Mode: AdminDomainListMode(st.Mode), SuspendAs: AdminDomainListSuspendAs(st.SuspendAs), SilenceAs: AdminDomainListSilenceAs(st.SilenceAs),
		ApplyRejectMedia: st.ApplyRejectMedia, ApplyRejectReports: st.ApplyRejectReports, PublicCommentFromList: st.PublicCommentFromList,
		ConsecutiveFailures: rec.Head.ConsecutiveFailures, CreatedAt: rec.Subscription.CreatedAt.UTC(), UpdatedAt: rec.Revision.RevisedAt.UTC(),
	}
	if rec.LastFetch != nil {
		f := newAdminDomainListFetch(*rec.LastFetch)
		out.LastFetch = &f
	}
	return out
}

func newAdminDomainListDraft(c moderation.DomainListChange) AdminDomainListDraft {
	out := AdminDomainListDraft{
		Key: c.Key(), Change: AdminDomainListChange(c.Change), Domain: c.Domain, Kind: AdminDomainActionKind(c.Kind), Held: AdminDomainListHold(c.Held),
	}
	if c.Change == moderation.DomainListLift {
		out.ActionId = &c.ActionID
		return out
	}
	severity, obfuscate := string(c.Entry.Severity), c.Entry.Obfuscate
	out.Severity, out.Obfuscate = &severity, &obfuscate
	if c.Entry.PublicComment != "" {
		out.PublicComment = &c.Entry.PublicComment
	}
	return out
}

func newAdminDomainListDrafts(cs []moderation.DomainListChange) []AdminDomainListDraft {
	out := make([]AdminDomainListDraft, len(cs))
	for i, c := range cs {
		out[i] = newAdminDomainListDraft(c)
	}
	return out
}

// domainListSubscriptionResponse answers an operation on one subscription.
func domainListSubscriptionResponse(rec moderation.DomainListSubscriptionRecord) AdminDomainListSubscriptionJSONResponse {
	cache := cacheNoStore
	return AdminDomainListSubscriptionJSONResponse{
		Body: newAdminDomainListSubscription(rec), Headers: AdminDomainListSubscriptionResponseHeaders{CacheControl: &cache},
	}
}

// domainListDraftResultResponse answers the acceptance or the rejection of drafts.
func domainListDraftResultResponse(res appmoderation.DomainListDraftResult) AdminDomainListDraftResultJSONResponse {
	cache := cacheNoStore
	missing := res.Missing
	if missing == nil {
		missing = []string{}
	}
	return AdminDomainListDraftResultJSONResponse{
		Body:    AdminDomainListDraftResult{Done: newAdminDomainListDrafts(res.Done), Missing: missing},
		Headers: AdminDomainListDraftResultResponseHeaders{CacheControl: &cache},
	}
}

// domainListUnprocessable answers a subscription that the moderation module refused as
// invalid with a 422 that names the field.
func (s *Server) domainListUnprocessable(ctx context.Context, err error) problem {
	l := s.Localizer(ctx)
	var textErr *moderation.TextError
	if errors.As(err, &textErr) && textErr.Max > 0 {
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(),
			l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: textErr.Max, Name: textErr.Field}))
	}
	field := "settings"
	if fieldErr := (*moderation.DomainListFieldError)(nil); errors.As(err, &fieldErr) {
		field = fieldErr.Field
	}
	return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(field))
}

// ListDomainListSubscriptions serves GET /api/v1/admin/domain_list_subscriptions, newest first.
// The cursor is the ID of the last subscription of the previous page.
func (s *Server) ListDomainListSubscriptions(ctx context.Context, req ListDomainListSubscriptionsRequestObject) (ListDomainListSubscriptionsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListDomainListSubscriptionsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListDomainListSubscriptionsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	recs, err := s.o.DomainListSubscriptions.DomainListSubscriptions(ctx, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list domain list subscriptions: %w", err)
	}
	recs, last := trim(recs, q.limit)
	page := AdminDomainListSubscriptionPage{Items: make([]AdminDomainListSubscription, 0, len(recs))}
	for _, r := range recs {
		page.Items = append(page.Items, newAdminDomainListSubscription(r))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(recs[last].Subscription.ID)
	}
	cache := cacheNoStore
	return ListDomainListSubscriptions200JSONResponse{Body: page, Headers: ListDomainListSubscriptions200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateDomainListSubscription serves POST /api/v1/admin/domain_list_subscriptions.
func (s *Server) CreateDomainListSubscription(ctx context.Context, req CreateDomainListSubscriptionRequestObject) (CreateDomainListSubscriptionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return CreateDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	if b == nil {
		return CreateDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "url")), nil
	}
	settings := moderation.DefaultDomainListSettings(b.Title)
	applyDomainListSettings(&settings, b.Mode, b.SuspendAs, b.SilenceAs, b.ApplyRejectMedia, b.ApplyRejectReports, b.PublicCommentFromList)
	rec, err := s.o.DomainListSubscriptions.CreateDomainListSubscription(ctx, appmoderation.DomainListSubscriptionInput{
		URL: b.Url, Format: moderation.DomainListFormat(b.Format), Settings: settings, CreatedByUserID: v.UserID,
	})
	if errors.Is(err, moderation.ErrInvalidDomainList) {
		return CreateDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.domainListUnprocessable(ctx, err)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create a domain list subscription: %w", err)
	}
	return CreateDomainListSubscription201JSONResponse{domainListSubscriptionResponse(rec)}, nil
}

// applyDomainListSettings sets the settings that a request names.
func applyDomainListSettings(st *moderation.DomainListSettings, mode *AdminDomainListMode, suspendAs *AdminDomainListSuspendAs,
	silenceAs *AdminDomainListSilenceAs, rejectMedia, rejectReports, publicComment *bool,
) {
	if mode != nil {
		st.Mode = moderation.DomainListMode(*mode)
	}
	if suspendAs != nil {
		st.SuspendAs = moderation.DomainListSeverityMapping(*suspendAs)
	}
	if silenceAs != nil {
		st.SilenceAs = moderation.DomainListSeverityMapping(*silenceAs)
	}
	if rejectMedia != nil {
		st.ApplyRejectMedia = *rejectMedia
	}
	if rejectReports != nil {
		st.ApplyRejectReports = *rejectReports
	}
	if publicComment != nil {
		st.PublicCommentFromList = *publicComment
	}
}

// GetDomainListSubscription serves GET /api/v1/admin/domain_list_subscriptions/{id}.
func (s *Server) GetDomainListSubscription(ctx context.Context, req GetDomainListSubscriptionRequestObject) (GetDomainListSubscriptionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return GetDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	rec, err := s.o.DomainListSubscriptions.DomainListSubscription(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return GetDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: domain list subscription %s: %w", req.Id, err)
	}
	return GetDomainListSubscription200JSONResponse{domainListSubscriptionResponse(rec)}, nil
}

// UpdateDomainListSubscription serves PATCH /api/v1/admin/domain_list_subscriptions/{id}.
func (s *Server) UpdateDomainListSubscription(ctx context.Context, req UpdateDomainListSubscriptionRequestObject) (UpdateDomainListSubscriptionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return UpdateDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	rec, err := s.o.DomainListSubscriptions.DomainListSubscription(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return UpdateDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: domain list subscription %s: %w", req.Id, err)
	}
	settings := rec.Revision.Settings
	if b := req.Body; b != nil {
		if b.Title != nil {
			settings.Title = *b.Title
		}
		applyDomainListSettings(&settings, b.Mode, b.SuspendAs, b.SilenceAs, b.ApplyRejectMedia, b.ApplyRejectReports, b.PublicCommentFromList)
	}
	rec, err = s.o.DomainListSubscriptions.UpdateDomainListSubscription(ctx, req.Id, settings, v.UserID)
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		return UpdateDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, moderation.ErrInvalidDomainList):
		return UpdateDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.domainListUnprocessable(ctx, err)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: update domain list subscription %s: %w", req.Id, err)
	}
	return UpdateDomainListSubscription200JSONResponse{domainListSubscriptionResponse(rec)}, nil
}

// DeleteDomainListSubscription serves DELETE /api/v1/admin/domain_list_subscriptions/{id}.
func (s *Server) DeleteDomainListSubscription(ctx context.Context, req DeleteDomainListSubscriptionRequestObject) (DeleteDomainListSubscriptionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return DeleteDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	lift := req.Params.LiftActions != nil && *req.Params.LiftActions
	err = s.o.DomainListSubscriptions.DeleteDomainListSubscription(ctx, req.Id, lift, v.UserID)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return DeleteDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: delete domain list subscription %s: %w", req.Id, err)
	}
	return DeleteDomainListSubscription204Response{}, nil
}

// FetchDomainListSubscription serves POST /api/v1/admin/domain_list_subscriptions/{id}/fetch.
func (s *Server) FetchDomainListSubscription(ctx context.Context, req FetchDomainListSubscriptionRequestObject) (FetchDomainListSubscriptionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return FetchDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.DomainListSubscriptions.QueueDomainListFetch(ctx, req.Id, v.UserID)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return FetchDomainListSubscriptiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: queue the fetch of domain list subscription %s: %w", req.Id, err)
	}
	return FetchDomainListSubscription202Response{}, nil
}

// ListDomainListFetches serves GET /api/v1/admin/domain_list_subscriptions/{id}/fetches, newest
// first. The cursor is the ID of the last fetch of the previous page.
func (s *Server) ListDomainListFetches(ctx context.Context, req ListDomainListFetchesRequestObject) (ListDomainListFetchesResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListDomainListFetchesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListDomainListFetchesdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	fetches, err := s.o.DomainListSubscriptions.DomainListFetches(ctx, req.Id, q.before, q.limit+1)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return ListDomainListFetchesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the fetches of domain list subscription %s: %w", req.Id, err)
	}
	fetches, last := trim(fetches, q.limit)
	page := AdminDomainListFetchPage{Items: make([]AdminDomainListFetch, 0, len(fetches))}
	for _, f := range fetches {
		page.Items = append(page.Items, newAdminDomainListFetch(f))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(fetches[last].ID)
	}
	cache := cacheNoStore
	return ListDomainListFetches200JSONResponse{Body: page, Headers: ListDomainListFetches200ResponseHeaders{CacheControl: &cache}}, nil
}

// ListDomainListDrafts serves GET /api/v1/admin/domain_list_subscriptions/{id}/drafts.
func (s *Server) ListDomainListDrafts(ctx context.Context, req ListDomainListDraftsRequestObject) (ListDomainListDraftsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListDomainListDraftsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	drafts, err := s.o.DomainListSubscriptions.DomainListDrafts(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return ListDomainListDraftsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the drafts of domain list subscription %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	return ListDomainListDrafts200JSONResponse{
		Body: AdminDomainListDraftList{Items: newAdminDomainListDrafts(drafts)}, Headers: ListDomainListDrafts200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// AcceptDomainListDrafts serves POST /api/v1/admin/domain_list_subscriptions/{id}/drafts/accept.
func (s *Server) AcceptDomainListDrafts(ctx context.Context, req AcceptDomainListDraftsRequestObject) (AcceptDomainListDraftsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return AcceptDomainListDraftsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	var keys []string
	if req.Body != nil && req.Body.Keys != nil {
		keys = *req.Body.Keys
	}
	res, err := s.o.DomainListSubscriptions.AcceptDomainListDrafts(ctx, req.Id, keys, v.UserID)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return AcceptDomainListDraftsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: accept the drafts of domain list subscription %s: %w", req.Id, err)
	}
	return AcceptDomainListDrafts200JSONResponse{domainListDraftResultResponse(res)}, nil
}

// RejectDomainListDrafts serves POST /api/v1/admin/domain_list_subscriptions/{id}/drafts/reject.
func (s *Server) RejectDomainListDrafts(ctx context.Context, req RejectDomainListDraftsRequestObject) (RejectDomainListDraftsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return RejectDomainListDraftsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	var keys []string
	if req.Body != nil && req.Body.Keys != nil {
		keys = *req.Body.Keys
	}
	res, err := s.o.DomainListSubscriptions.RejectDomainListDrafts(ctx, req.Id, keys, v.UserID)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return RejectDomainListDraftsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: reject the drafts of domain list subscription %s: %w", req.Id, err)
	}
	return RejectDomainListDrafts200JSONResponse{domainListDraftResultResponse(res)}, nil
}
