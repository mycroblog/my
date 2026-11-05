// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/domain/identity"
)

// TranslateReportedStatus serves POST /api/v1/admin/reports/{id}/statuses/{status_id}/translation
// for the moderators (ADR-0148 §13). The service reads only a status that the report points at.
func (s *Server) TranslateReportedStatus(ctx context.Context, req TranslateReportedStatusRequestObject) (TranslateReportedStatusResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return TranslateReportedStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return TranslateReportedStatusdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	out, err := translation.Translation{}, translation.ErrNoProvider
	if s.o.Translations != nil {
		out, err = s.o.Translations.TranslateReportedStatus(ctx, req.Id, req.StatusId, req.Body.TargetLanguage)
	}
	if err != nil {
		if p, ok := s.reportTranslationProblem(ctx, err); ok {
			return TranslateReportedStatusdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: translate status %s of report %s: %w", req.StatusId, req.Id, err)
	}
	cache := cacheNoStore
	return TranslateReportedStatus200JSONResponse{Body: newStatusTranslation(out), Headers: TranslateReportedStatus200ResponseHeaders{CacheControl: &cache}}, nil
}

// TranslateReportComment serves POST /api/v1/admin/reports/{id}/comment/translation for the
// moderators (ADR-0148 §13).
func (s *Server) TranslateReportComment(ctx context.Context, req TranslateReportCommentRequestObject) (TranslateReportCommentResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return TranslateReportCommentdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return TranslateReportCommentdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	out, err := translation.CommentTranslation{}, translation.ErrNoProvider
	if s.o.Translations != nil {
		out, err = s.o.Translations.TranslateReportComment(ctx, req.Id, req.Body.TargetLanguage)
	}
	if err != nil {
		if p, ok := s.reportTranslationProblem(ctx, err); ok {
			return TranslateReportCommentdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: translate the comment of report %s: %w", req.Id, err)
	}
	res := ReportCommentTranslation{Language: out.TargetLanguage, SourceLanguage: out.SourceLanguage, Comment: out.Text}
	if p := out.Provider; p != nil {
		res.Provider = new(newTranslationProvider(*p))
	}
	cache := cacheNoStore
	return TranslateReportComment200JSONResponse{Body: res, Headers: TranslateReportComment200ResponseHeaders{CacheControl: &cache}}, nil
}

// reportTranslationProblem is translationProblem for a report: an unknown report, a status that it
// does not point at, and one that is gone are a 404.
func (s *Server) reportTranslationProblem(ctx context.Context, err error) (problem, bool) {
	if errors.Is(err, appmoderation.ErrNotFound) {
		return s.notFoundResponse(ctx), true
	}
	return s.translationProblem(ctx, err)
}
