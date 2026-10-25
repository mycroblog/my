// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InstanceInfo reads the instance settings and texts. *appinstance.Service implements it.
type InstanceInfo interface {
	// Settings returns the current value of every setting, defaults included.
	Settings(ctx context.Context) (instance.Settings, error)
	// Text returns the text of key, the same in every locale, and false when there is none.
	Text(ctx context.Context, key instance.TextKey) (instance.Text, bool, error)
}

// InstanceDocuments reads the instance's documents, rendered. *appinstance.Service implements it.
type InstanceDocuments interface {
	// Document returns the document key rendered, and false when the instance has not
	// published it.
	Document(ctx context.Context, key instance.TextKey) (appinstance.Document, bool, error)
}

// InstanceImage is a processed image of the instance, as GET /instance presents it.
type InstanceImage struct {
	URL           string
	Width, Height int
	// Alt is the alt text, "" for none.
	Alt string
}

// cachePublic is the Cache-Control of anonymous public GETs, which shared caches may store
// (ADR-0016).
const cachePublic = "public, max-age=60"

// cachePrivate is the Cache-Control of public resources fetched with an access token, which
// depend on the viewer's locale.
const cachePrivate = "private, max-age=60"

// GetInstance serves GET /api/v1/instance. The administrators' texts and rules hold one value
// each, the same in every locale. Like every response, it varies with Accept-Language (localize)
// for anonymous requests and is private for authenticated ones.
func (s *Server) GetInstance(ctx context.Context, _ GetInstanceRequestObject) (GetInstanceResponseObject, error) {
	var settings instance.Settings
	out := Instance{Domain: s.o.Host, Name: s.o.Host, Version: s.o.SoftwareVersion, SourceUrl: s.o.SourceURL}
	if s.o.StatusPageURL != "" {
		out.StatusPageUrl = &s.o.StatusPageURL
	}
	if s.o.Instance != nil {
		var err error
		if settings, err = s.o.Instance.Settings(ctx); err != nil {
			return nil, fmt.Errorf("clientapi: instance settings: %w", err)
		}
		for _, t := range []struct {
			key instance.TextKey
			set func(string)
		}{
			{instance.TextName, func(v string) { out.Name = v }},
			{instance.TextShortDescription, func(v string) { out.ShortDescription = &v }},
			{instance.TextDescription, func(v string) { out.Description = &v }},
		} {
			text, ok, err := s.o.Instance.Text(ctx, t.key)
			if err != nil {
				return nil, fmt.Errorf("clientapi: instance text %s: %w", t.key, err)
			}
			if ok {
				t.set(text.Body)
			}
		}
		if err := s.documentURLs(ctx, &out); err != nil {
			return nil, err
		}
	}
	out.Registrations = InstanceRegistrations{Mode: InstanceRegistrationsMode(settings.RegistrationMode())}
	out.Limits = InstanceLimits{MaxPostCharacters: settings.MaxStatusLength(), RequireMediaDescription: settings.RequireMediaDescription()}
	out.Timelines = InstanceTimelines{Local: TimelineAccess(settings.LocalTimelineAccess()), Federated: TimelineAccess(settings.FederatedTimelineAccess())}
	// The direction the instance federates in, by which clients offer the operations on remote
	// accounts and posts (FR-413, ADR-0076).
	out.Federation = InstanceFederation{Mode: InstanceFederationMode(federationMode(settings))}
	out.SendsEmail = s.o.SendsEmail
	// What the instance states to search engines and AI crawlers (FR-725, ADR-0057).
	out.SearchIndexing, out.AiUsage = SearchIndexing(settings.SearchIndexing()), AIUsage(settings.AIUsage())
	// How much of the costs GET /instance/cost_totals publishes (FR-903, ADR-0067 §8).
	out.FinanceVisibility = FinanceVisibility(settings.FinanceVisibility())
	// How much of the income GET /instance/income_totals publishes (FR-917, ADR-0073 §7).
	out.IncomeVisibility = IncomeVisibility(settings.IncomeVisibility())
	// Who may read the supporter list of GET /instance/supporters (ADR-0071 §4).
	out.SupporterListVisibility = SupporterListVisibility(settings.SupporterListVisibility())
	// Whether the supporter program shows its plans (ADR-0072 §2).
	if s.o.Supporters != nil {
		enabled, err := s.o.Supporters.Enabled(ctx)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the supporter program: %w", err)
		}
		out.SupportersEnabled = enabled
	}
	if key := s.pushServerKey(); key != "" {
		out.PushServerKey = &key
	}
	rules, err := s.instanceRules(ctx)
	if err != nil {
		return nil, err
	}
	out.Rules = rules
	if out.Theme, err = s.instanceTheme(ctx); err != nil {
		return nil, err
	}
	if out.Contact, err = s.instanceContact(ctx, settings); err != nil {
		return nil, err
	}
	if out.Thumbnail, err = s.instanceThumbnail(ctx); err != nil {
		return nil, err
	}
	// The closure of the service while one is announced (FR-808, ADR-0088 §2).
	if out.Closure, err = s.instanceClosure(ctx); err != nil {
		return nil, err
	}

	cache := cachePublic
	if _, ok := ViewerFrom(ctx); ok {
		cache = cachePrivate
	}
	return GetInstance200JSONResponse{Body: out, Headers: GetInstance200ResponseHeaders{CacheControl: &cache}}, nil
}

// instanceContact presents the contact_account and contact_email settings, nil when the
// instance has neither. An account that is gone or that the viewer may not see is left out.
func (s *Server) instanceContact(ctx context.Context, settings instance.Settings) (*InstanceContact, error) {
	var out InstanceContact
	if email := settings.ContactEmail(); email != "" {
		out.Email = &email
	}
	if username := settings.ContactAccount(); username != "" && s.o.Accounts != nil {
		a, err := s.o.Accounts.FindByHandle(ctx, username, account.LocalDomain)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
		case err != nil:
			return nil, fmt.Errorf("clientapi: instance contact %s: %w", username, err)
		default:
			accounts, err := s.presentAccounts(ctx, viewerOf(ctx), []string{a.Account.ID}, false)
			if err != nil {
				return nil, fmt.Errorf("clientapi: instance contact %s: %w", username, err)
			}
			if len(accounts) == 1 {
				out.Account = &accounts[0]
			}
		}
	}
	if out.Email == nil && out.Account == nil {
		return nil, nil
	}
	return &out, nil
}

// instanceThumbnail presents the thumbnail setting, nil when there is none or it cannot be shown.
func (s *Server) instanceThumbnail(ctx context.Context) (*InstanceThumbnail, error) {
	if s.o.InstanceThumbnail == nil {
		return nil, nil
	}
	img, err := s.o.InstanceThumbnail(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: instance thumbnail: %w", err)
	}
	if img == nil {
		return nil, nil
	}
	out := &InstanceThumbnail{Url: img.URL, Width: img.Width, Height: img.Height}
	if img.Alt != "" {
		out.Alt = &img.Alt
	}
	return out, nil
}

// GetInstanceDocument serves GET /api/v1/instance/documents/{kind}: the sanitized HTML of a
// document that the instance has published. A document that is not published, and a kind that
// is not a document, is a 404. The text is the same in every locale.
func (s *Server) GetInstanceDocument(ctx context.Context, req GetInstanceDocumentRequestObject) (GetInstanceDocumentResponseObject, error) {
	key := instance.TextKey(req.Kind)
	switch key {
	case instance.TextTerms, instance.TextPrivacyPolicy, instance.TextCommerceDisclosure:
	default:
		return GetInstanceDocumentdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if s.o.Documents == nil {
		return GetInstanceDocumentdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	doc, ok, err := s.o.Documents.Document(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("clientapi: instance document %s: %w", key, err)
	}
	if !ok {
		return GetInstanceDocumentdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	cache := cachePublic
	if _, ok := ViewerFrom(ctx); ok {
		cache = cachePrivate
	}
	return GetInstanceDocument200JSONResponse{
		Body:    InstanceDocument{Kind: req.Kind, Html: doc.HTML, RevisedAt: doc.RevisedAt.UTC()},
		Headers: GetInstanceDocument200ResponseHeaders{CacheControl: &cache},
	}, nil
}
