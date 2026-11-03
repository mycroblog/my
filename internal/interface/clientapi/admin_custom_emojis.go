// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The custom emoji (ADR-0051 §4): the list of the picker (GET /custom_emojis) and the admin
// operations on /admin/custom_emojis (manage_custom_emojis, ADR-0058 server administration).

// ProblemTypeInvalidCustomEmoji is the problem type of a custom emoji that breaks a rule (422),
// whose reason says which.
const ProblemTypeInvalidCustomEmoji = "/api/v1/problems/invalid-custom-emoji"

// CustomEmojis runs the use cases of the custom emoji that the administrators manage.
// *appcontent.CustomEmojiService implements it.
type CustomEmojis interface {
	Picker(ctx context.Context) ([]appcontent.PickerCustomEmoji, error)
	List(ctx context.Context, q appcontent.CustomEmojiQuery, limit int) ([]appcontent.AdminCustomEmoji, error)
	Get(ctx context.Context, id string) (appcontent.AdminCustomEmoji, error)
	Create(ctx context.Context, in appcontent.CreateCustomEmojiInput) (appcontent.AdminCustomEmoji, error)
	Update(ctx context.Context, id string, u appcontent.CustomEmojiUpdate) (appcontent.AdminCustomEmoji, error)
	ReplaceImage(ctx context.Context, id, ownerAccountID, mediaID, byUserID string) (appcontent.AdminCustomEmoji, error)
	Copy(ctx context.Context, in appcontent.CopyCustomEmojiInput) (appcontent.AdminCustomEmoji, error)
	Delete(ctx context.Context, id, byUserID string) error
}

// ListCustomEmojis serves GET /api/v1/custom_emojis.
func (s *Server) ListCustomEmojis(ctx context.Context, _ ListCustomEmojisRequestObject) (ListCustomEmojisResponseObject, error) {
	out := PickerCustomEmojiList{Items: []PickerCustomEmoji{}}
	if s.o.CustomEmojis != nil {
		emojis, err := s.o.CustomEmojis.Picker(ctx)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the custom emoji of the picker: %w", err)
		}
		for _, e := range emojis {
			item := PickerCustomEmoji{Shortcode: e.Shortcode, Url: e.URL, StaticUrl: e.StaticURL, Aliases: e.Aliases}
			if item.Aliases == nil {
				item.Aliases = []string{}
			}
			if e.Category != "" {
				item.Category = &e.Category
			}
			out.Items = append(out.Items, item)
		}
	}
	cache := cachePublic
	if _, ok := ViewerFrom(ctx); ok {
		cache = cachePrivate
	}
	return ListCustomEmojis200JSONResponse{Body: out, Headers: ListCustomEmojis200ResponseHeaders{CacheControl: &cache}}, nil
}

// newAdminCustomEmoji presents a custom emoji to the administrators, without its creator
// (withCreators adds it).
func (s *Server) newAdminCustomEmoji(e appcontent.AdminCustomEmoji) AdminCustomEmoji {
	out := AdminCustomEmoji{
		Id: e.Emoji.ID, Shortcode: e.Emoji.Shortcode, Local: e.Emoji.Local(), Enabled: e.Settings.Enabled,
		VisibleInPicker: e.Settings.VisibleInPicker || !e.Emoji.Local(), Aliases: e.Aliases,
		CreatedAt: e.Emoji.CreatedAt.UTC(), UpdatedAt: e.Image.RevisedAt.UTC(),
	}
	if out.Aliases == nil {
		out.Aliases = []string{}
	}
	uri := e.Emoji.URI
	if e.Emoji.Local() {
		uri = content.CustomEmojiURI(s.o.Host, e.Emoji.ID)
	} else {
		out.Domain = &e.Emoji.Domain
	}
	if uri != "" {
		out.Uri = &uri
	}
	if e.Settings.Category != "" {
		out.Category = &e.Settings.Category
	}
	if v := e.View; v != nil {
		out.Url, out.StaticUrl = &v.URL, &v.StaticURL
	}
	return out
}

// withCreators presents emojis to the administrators with the accounts of their creators
// (ADR-0070 §7), read once for all of them.
func (s *Server) withCreators(ctx context.Context, emojis []appcontent.AdminCustomEmoji) ([]AdminCustomEmoji, error) {
	if s.o.UserAccounts == nil || s.o.Accounts == nil {
		out := make([]AdminCustomEmoji, len(emojis))
		for i, e := range emojis {
			out[i] = s.newAdminCustomEmoji(e)
		}
		return out, nil
	}
	return s.withCreatorsWith(ctx, s.reads(), emojis)
}

// withCreatorsWith is withCreators through reads.
func (s *Server) withCreatorsWith(ctx context.Context, reads presentReads, emojis []appcontent.AdminCustomEmoji) ([]AdminCustomEmoji, error) {
	ids := make([]string, len(emojis))
	for i, e := range emojis {
		ids[i] = e.Emoji.CreatedByUserID
	}
	accounts, err := s.userAccountsWith(ctx, reads, ids)
	if err != nil {
		return nil, err
	}
	out := make([]AdminCustomEmoji, len(emojis))
	for i, e := range emojis {
		out[i] = s.newAdminCustomEmoji(e)
		if u := e.Emoji.CreatedByUserID; u != "" {
			if a, ok := accounts[u]; ok {
				out[i].CreatedBy = &a
			} else {
				out[i].CreatedByDeleted = true
			}
		}
	}
	return out, nil
}

// withCreator presents one emoji as withCreators does.
func (s *Server) withCreator(ctx context.Context, e appcontent.AdminCustomEmoji) (AdminCustomEmoji, error) {
	out, err := s.withCreators(ctx, []appcontent.AdminCustomEmoji{e})
	if err != nil {
		return AdminCustomEmoji{}, err
	}
	return out[0], nil
}

// requireCustomEmojis checks the manage_custom_emojis permission and that the use cases are
// configured; it returns the problem to answer otherwise.
func (s *Server) requireCustomEmojis(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageCustomEmojis); err != nil || p != nil {
		return p, err
	}
	if s.o.CustomEmojis == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// customEmojiProblem returns the 422 of a custom emoji that breaks a rule, and false for
// another error.
func (s *Server) customEmojiProblem(ctx context.Context, err error) (CustomEmojiProblemApplicationProblemPlusJSONResponse, bool) {
	var ce *content.CustomEmojiError
	if !errors.As(err, &ce) {
		return CustomEmojiProblemApplicationProblemPlusJSONResponse{}, false
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidCustomEmoji,
		l.ServerErrorsInvalidCustomEmoji(), l.ServerErrorsInvalidCustomEmojiDetail(string(ce.Reason)))
	reason := CustomEmojiProblemReason(ce.Reason)
	return CustomEmojiProblemApplicationProblemPlusJSONResponse{
		Body: CustomEmojiProblem{
			Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
		},
		Headers: CustomEmojiProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}, true
}

// customEmojiNotFound reports whether err is an unknown custom emoji.
func customEmojiNotFound(err error) bool { return errors.Is(err, appcontent.ErrNotFound) }

func adminCustomEmojiResponse(body AdminCustomEmoji) AdminCustomEmojiJSONResponse {
	cache := cacheNoStore
	return AdminCustomEmojiJSONResponse{Body: body, Headers: AdminCustomEmojiResponseHeaders{CacheControl: &cache}}
}

// ListAdminCustomEmojis serves GET /api/v1/admin/custom_emojis.
func (s *Server) ListAdminCustomEmojis(ctx context.Context, req ListAdminCustomEmojisRequestObject) (ListAdminCustomEmojisResponseObject, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListAdminCustomEmojisdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListAdminCustomEmojisdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	q := appcontent.CustomEmojiQuery{Before: pq.before}
	if l := req.Params.Local; l != nil {
		q.Locality = appcontent.RemoteEmoji
		if *l {
			q.Locality = appcontent.LocalEmoji
		}
	}
	if d := req.Params.Domain; d != nil {
		q.Domain = *d
	}
	if c := req.Params.Shortcode; c != nil {
		q.Shortcode = *c
	}
	emojis, err := s.o.CustomEmojis.List(ctx, q, pq.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list custom emoji: %w", err)
	}
	emojis, last := trim(emojis, pq.limit)
	items, err := s.withCreators(ctx, emojis)
	if err != nil {
		return nil, err
	}
	page := AdminCustomEmojiPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(emojis[last].Emoji.ID)
	}
	cache := cacheNoStore
	return ListAdminCustomEmojis200JSONResponse{Body: page, Headers: ListAdminCustomEmojis200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetAdminCustomEmoji serves GET /api/v1/admin/custom_emojis/{id}.
func (s *Server) GetAdminCustomEmoji(ctx context.Context, req GetAdminCustomEmojiRequestObject) (GetAdminCustomEmojiResponseObject, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return GetAdminCustomEmojidefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	e, err := s.o.CustomEmojis.Get(ctx, req.Id)
	switch {
	case customEmojiNotFound(err):
		return GetAdminCustomEmojidefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: custom emoji %s: %w", req.Id, err)
	}
	out, err := s.withCreator(ctx, e)
	if err != nil {
		return nil, err
	}
	return GetAdminCustomEmoji200JSONResponse{adminCustomEmojiResponse(out)}, nil
}

// CreateCustomEmoji serves POST /api/v1/admin/custom_emojis.
func (s *Server) CreateCustomEmoji(ctx context.Context, req CreateCustomEmojiRequestObject) (CreateCustomEmojiResponseObject, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return CreateCustomEmojidefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return CreateCustomEmojidefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	in := appcontent.CreateCustomEmojiInput{
		Shortcode: req.Body.Shortcode, MediaID: knownID(req.Body.MediaId), OwnerAccountID: v.AccountID, VisibleInPicker: true, ByUserID: v.UserID,
	}
	if c := req.Body.Category; c != nil {
		in.Category = *c
	}
	if a := req.Body.Aliases; a != nil {
		in.Aliases = *a
	}
	if p := req.Body.VisibleInPicker; p != nil {
		in.VisibleInPicker = *p
	}
	e, err := s.o.CustomEmojis.Create(ctx, in)
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return CreateCustomEmoji422ApplicationProblemPlusJSONResponse{p}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create custom emoji: %w", err)
	}
	out, err := s.withCreator(ctx, e)
	if err != nil {
		return nil, err
	}
	return CreateCustomEmoji201JSONResponse{adminCustomEmojiResponse(out)}, nil
}

// UpdateCustomEmoji serves PATCH /api/v1/admin/custom_emojis/{id}.
func (s *Server) UpdateCustomEmoji(ctx context.Context, req UpdateCustomEmojiRequestObject) (UpdateCustomEmojiResponseObject, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return UpdateCustomEmojidefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return UpdateCustomEmojidefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	e, err := s.o.CustomEmojis.Update(ctx, req.Id, appcontent.CustomEmojiUpdate{
		Enabled: req.Body.Enabled, VisibleInPicker: req.Body.VisibleInPicker, Category: req.Body.Category, Aliases: req.Body.Aliases,
		ByUserID: v.UserID,
	})
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return UpdateCustomEmoji422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return UpdateCustomEmojidefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: update custom emoji %s: %w", req.Id, err)
	}
	out, err := s.withCreator(ctx, e)
	if err != nil {
		return nil, err
	}
	return UpdateCustomEmoji200JSONResponse{adminCustomEmojiResponse(out)}, nil
}

// ReplaceCustomEmojiImage serves PUT /api/v1/admin/custom_emojis/{id}/image.
func (s *Server) ReplaceCustomEmojiImage(ctx context.Context, req ReplaceCustomEmojiImageRequestObject) (ReplaceCustomEmojiImageResponseObject, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ReplaceCustomEmojiImagedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return ReplaceCustomEmojiImagedefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	e, err := s.o.CustomEmojis.ReplaceImage(ctx, req.Id, v.AccountID, knownID(req.Body.MediaId), v.UserID)
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return ReplaceCustomEmojiImage422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return ReplaceCustomEmojiImagedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: replace the image of custom emoji %s: %w", req.Id, err)
	}
	out, err := s.withCreator(ctx, e)
	if err != nil {
		return nil, err
	}
	return ReplaceCustomEmojiImage200JSONResponse{adminCustomEmojiResponse(out)}, nil
}

// CopyCustomEmoji serves POST /api/v1/admin/custom_emojis/{id}/copy.
func (s *Server) CopyCustomEmoji(ctx context.Context, req CopyCustomEmojiRequestObject) (CopyCustomEmojiResponseObject, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return CopyCustomEmojidefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	in := appcontent.CopyCustomEmojiInput{ID: req.Id, OwnerAccountID: v.AccountID, ByUserID: v.UserID}
	if req.Body != nil && req.Body.Shortcode != nil {
		in.Shortcode = *req.Body.Shortcode
	}
	e, err := s.o.CustomEmojis.Copy(ctx, in)
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return CopyCustomEmoji422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return CopyCustomEmojidefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: copy custom emoji %s: %w", req.Id, err)
	}
	out, err := s.withCreator(ctx, e)
	if err != nil {
		return nil, err
	}
	return CopyCustomEmoji201JSONResponse{adminCustomEmojiResponse(out)}, nil
}

// DeleteCustomEmoji serves DELETE /api/v1/admin/custom_emojis/{id}.
func (s *Server) DeleteCustomEmoji(ctx context.Context, req DeleteCustomEmojiRequestObject) (DeleteCustomEmojiResponseObject, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return DeleteCustomEmojidefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.CustomEmojis.Delete(ctx, req.Id, v.UserID)
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return DeleteCustomEmoji422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return DeleteCustomEmojidefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: delete custom emoji %s: %w", req.Id, err)
	}
	return DeleteCustomEmoji204Response{}, nil
}
