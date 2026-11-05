// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/translation"
)

// The problem types of translateStatus.
const (
	// ProblemTypeTranslationUnavailable is the problem of a post that no provider may receive.
	ProblemTypeTranslationUnavailable = "/api/v1/problems/translation-unavailable"
	// ProblemTypeTranslationUnsupportedLanguage is the problem of a language that no provider translates.
	ProblemTypeTranslationUnsupportedLanguage = "/api/v1/problems/translation-unsupported-language"
	// ProblemTypeTranslationLimit is the problem of a request over a monthly limit.
	ProblemTypeTranslationLimit = "/api/v1/problems/translation-limit"
	// ProblemTypeTranslationFailed is the problem of a request that no provider answered usefully.
	ProblemTypeTranslationFailed = "/api/v1/problems/translation-failed"
)

// Translations translates statuses. *translation.Service implements it.
type Translations interface {
	Translate(ctx context.Context, viewer appcontent.Viewer, statusID, target string) (translation.Translation, error)
}

// TranslateStatus serves POST /api/v1/statuses/{id}/translation. The answer depends on the viewer,
// who may see the post, so no cache stores it; the view check is the service's first step.
func (s *Server) TranslateStatus(ctx context.Context, req TranslateStatusRequestObject) (TranslateStatusResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return TranslateStatusdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	out, err := translation.Translation{}, translation.ErrNoProvider
	if s.o.Translations != nil {
		out, err = s.o.Translations.Translate(ctx, localContentViewer(v), req.Id, req.Body.TargetLanguage)
	}
	if err != nil {
		if p, ok := s.translationProblem(ctx, err); ok {
			return TranslateStatusdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: translate status %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	return TranslateStatus200JSONResponse{Body: newStatusTranslation(out), Headers: TranslateStatus200ResponseHeaders{CacheControl: &cache}}, nil
}

// translationProblem returns the problem that err of the service means to the client, or false for
// an unexpected error, which the handler returns as a 500.
func (s *Server) translationProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return s.statusProblem(ctx, err)
	case errors.Is(err, translation.ErrInvalidLanguage):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("target_language")), true
	case errors.Is(err, translation.ErrNoProvider):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTranslationUnavailable, l.ServerErrorsTranslationUnavailable(), ""), true
	case errors.Is(err, translation.ErrUnsupportedLanguage):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTranslationUnsupportedLanguage, l.ServerErrorsTranslationUnsupportedLanguage(), ""), true
	case errors.Is(err, translation.ErrLimitReached):
		return s.problem(ctx, http.StatusTooManyRequests, ProblemTypeTranslationLimit, l.ServerErrorsTranslationLimit(), ""), true
	case errors.Is(err, translation.ErrFailed):
		return s.problem(ctx, http.StatusServiceUnavailable, ProblemTypeTranslationFailed, l.ServerErrorsTranslationFailed(), ""), true
	}
	return problem{}, false
}

func newStatusTranslation(t translation.Translation) StatusTranslation {
	out := StatusTranslation{
		Language: t.TargetLanguage, SourceLanguage: t.SourceLanguage, Content: t.ContentHTML, SpoilerText: t.SpoilerText,
		MediaAttachments: make([]StatusTranslationAttachment, 0, len(t.Attachments)),
		PollOptions:      make([]string, 0, len(t.PollOptions)),
	}
	for _, a := range t.Attachments {
		out.MediaAttachments = append(out.MediaAttachments, StatusTranslationAttachment{Id: a.MediaID, Description: a.Description})
	}
	out.PollOptions = append(out.PollOptions, t.PollOptions...)
	if p := t.Provider; p != nil {
		out.Provider = &TranslationProvider{Id: p.ID, Name: p.Name}
		if p.URL != "" {
			out.Provider.Url = &p.URL
		}
		if p.LogoURL != "" {
			out.Provider.LogoUrl = &p.LogoURL
		}
	}
	return out
}
