// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// The instance texts (FR-705): the admin operations that read and write them (manage_settings),
// one value each, shown as it is in every locale, and the links of GET /instance to the web client's pages of the documents among
// them (the terms of service and the privacy policy, #489).

// InstanceTexts runs the use cases of the instance texts. *appinstance.Service implements it.
type InstanceTexts interface {
	// Texts returns every current, non-empty text, by key.
	Texts(ctx context.Context) ([]instance.Text, error)
	// SetText records a new revision of a text, audited.
	SetText(ctx context.Context, in appinstance.SetTextInput) (instance.TextRevision, error)
}

// documentURLs sets the links of out to the web client's pages of the documents that the instance has
// published, which the web client draws (Options.TermsPath, Options.PrivacyPolicyPath, Options.CommerceDisclosurePath).
func (s *Server) documentURLs(ctx context.Context, out *Instance) error {
	for _, d := range []struct {
		key  instance.TextKey
		path string
		set  func(string)
	}{
		{instance.TextTerms, s.o.TermsPath, func(u string) { out.TermsUrl = &u }},
		{instance.TextPrivacyPolicy, s.o.PrivacyPolicyPath, func(u string) { out.PrivacyPolicyUrl = &u }},
		{instance.TextCommerceDisclosure, s.o.CommerceDisclosurePath, func(u string) { out.CommerceDisclosureUrl = &u }},
	} {
		if d.path == "" {
			continue
		}
		_, ok, err := s.o.Instance.Text(ctx, d.key)
		if err != nil {
			return fmt.Errorf("clientapi: instance text %s: %w", d.key, err)
		}
		if ok {
			d.set(s.origin() + d.path)
		}
	}
	return nil
}

func newAdminInstanceText(key instance.TextKey, body string, revisedAt time.Time) AdminInstanceText {
	return AdminInstanceText{Key: InstanceTextKey(key), Body: body, RevisedAt: revisedAt.UTC()}
}

// ListInstanceTexts serves GET /api/v1/admin/instance_texts.
func (s *Server) ListInstanceTexts(ctx context.Context, _ ListInstanceTextsRequestObject) (ListInstanceTextsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil {
		return nil, err
	} else if p != nil {
		return ListInstanceTextsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	texts, err := s.o.InstanceTexts.Texts(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list instance texts: %w", err)
	}
	list := AdminInstanceTextList{Items: make([]AdminInstanceText, 0, len(texts))}
	for _, t := range texts {
		list.Items = append(list.Items, newAdminInstanceText(t.Key, t.Body, t.RevisedAt))
	}
	cache := cacheNoStore
	return ListInstanceTexts200JSONResponse{Body: list, Headers: ListInstanceTexts200ResponseHeaders{CacheControl: &cache}}, nil
}

// SetInstanceText serves PUT /api/v1/admin/instance_texts/{key}.
func (s *Server) SetInstanceText(ctx context.Context, req SetInstanceTextRequestObject) (SetInstanceTextResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil {
		return nil, err
	} else if p != nil {
		return SetInstanceTextdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) SetInstanceTextResponseObject {
		return SetInstanceTextdefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail))
	}
	def, err := instance.LookupText(instance.TextKey(req.Key))
	if err != nil {
		return SetInstanceTextdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return SetInstanceTextdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	if utf8.RuneCountInString(req.Body.Body) > def.MaxLength {
		return unprocessable(l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: def.MaxLength, Name: "body"})), nil
	}
	rev, err := s.o.InstanceTexts.SetText(ctx, appinstance.SetTextInput{
		Key: def.Key, Body: req.Body.Body, RevisedByUserID: v.UserID,
	})
	switch {
	case errors.Is(err, instance.ErrInvalidValue):
		return unprocessable(l.ServerErrorsInvalidValue("body")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: set instance text %s: %w", req.Key, err)
	}
	cache := cacheNoStore
	return SetInstanceText200JSONResponse{
		Body:    newAdminInstanceText(rev.Key, rev.Body, rev.RevisedAt),
		Headers: SetInstanceText200ResponseHeaders{CacheControl: &cache},
	}, nil
}
