// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// The domain block list in Mastodon's CSV (FR-706, manage_domains): the export of the domain
// actions in effect, and the import of a CSV that Mastodon exported, previewed first so that the
// moderator chooses the rows to import.

// domainBlockCSVName is the file name that the export suggests, Mastodon's.
const domainBlockCSVName = "domain_blocks.csv"

// ExportDomainBlocks serves GET /api/v1/admin/domain_actions/export.
func (s *Server) ExportDomainBlocks(ctx context.Context, _ ExportDomainBlocksRequestObject) (ExportDomainBlocksResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ExportDomainBlocksdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	var buf bytes.Buffer
	if err := s.o.Moderation.ExportDomainBlocks(ctx, &buf); err != nil {
		return nil, fmt.Errorf("clientapi: export domain blocks: %w", err)
	}
	cache := cacheNoStore
	disposition := `attachment; filename="` + domainBlockCSVName + `"`
	return ExportDomainBlocks200TextcsvResponse{
		Body: &buf, ContentLength: int64(buf.Len()),
		Headers: ExportDomainBlocks200ResponseHeaders{CacheControl: &cache, ContentDisposition: &disposition},
	}, nil
}

// PreviewDomainBlockImport serves POST /api/v1/admin/domain_actions/import/preview.
func (s *Server) PreviewDomainBlockImport(ctx context.Context, req PreviewDomainBlockImportRequestObject) (PreviewDomainBlockImportResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return PreviewDomainBlockImportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return PreviewDomainBlockImportdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "csv")), nil
	}
	l := s.Localizer(ctx)
	rows, err := s.o.Moderation.PreviewDomainBlocks(ctx, req.Body.Csv)
	switch {
	case errors.Is(err, moderation.ErrTooManyDomainBlocks):
		return PreviewDomainBlockImportdefaultApplicationProblemPlusJSONResponse(s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank,
			l.ServerErrorsUnprocessable(), l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: moderation.MaxDomainBlockImport, Name: "csv"}))), nil
	case errors.Is(err, moderation.ErrMalformedDomainBlockCSV):
		return PreviewDomainBlockImportdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "csv")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: preview domain blocks: %w", err)
	}
	out := DomainBlockImportPreview{Rows: make([]DomainBlockImportRow, len(rows))}
	for i, r := range rows {
		issue := make([]AdminDomainActionKind, len(r.Issue))
		for j, k := range r.Issue {
			issue[j] = AdminDomainActionKind(k)
		}
		out.Rows[i] = DomainBlockImportRow{Line: r.Line, WrittenDomain: r.Written, Status: DomainBlockImportStatus(r.Status), Issue: issue}
		if r.Status != appmoderation.DomainBlockInvalid {
			b := newDomainBlock(r.Block)
			out.Rows[i].Block = &b
		}
	}
	cache := cacheNoStore
	return PreviewDomainBlockImport200JSONResponse{Body: out, Headers: PreviewDomainBlockImport200ResponseHeaders{CacheControl: &cache}}, nil
}

// ImportDomainBlocks serves POST /api/v1/admin/domain_actions/import.
func (s *Server) ImportDomainBlocks(ctx context.Context, req ImportDomainBlocksRequestObject) (ImportDomainBlocksResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ImportDomainBlocksdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return ImportDomainBlocksdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "blocks")), nil
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (ImportDomainBlocksResponseObject, error) {
		return ImportDomainBlocksdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	in := appmoderation.DomainBlockImportInput{
		Blocks: make([]moderation.DomainBlock, len(req.Body.Blocks)), PrivateNote: deref(req.Body.PrivateNote), IssuedByUserID: v.UserID,
	}
	for i, b := range req.Body.Blocks {
		in.Blocks[i] = moderation.DomainBlock{
			Domain: b.Domain, Severity: moderation.DomainBlockSeverity(b.Severity), RejectMedia: b.RejectMedia,
			RejectReports: b.RejectReports, PublicComment: deref(b.PublicComment), Obfuscate: b.Obfuscate,
		}
	}
	res, err := s.o.Moderation.ImportDomainBlocks(ctx, in)
	var textErr *moderation.TextError
	switch {
	case errors.Is(err, moderation.ErrTooManyDomainBlocks):
		return unprocessable(l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: moderation.MaxDomainBlockImport, Name: "blocks"}))
	case errors.As(err, &textErr) && textErr.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: textErr.Max, Name: textErr.Field}))
	case errors.As(err, &textErr):
		return unprocessable(l.ServerErrorsInvalidValue(textErr.Field))
	case err != nil:
		return nil, fmt.Errorf("clientapi: import domain blocks: %w", err)
	}
	out := DomainBlockImportResult{Issued: make([]AdminDomainAction, len(res.Issued)), Skipped: make([]DomainBlockImportSkipped, len(res.Skipped))}
	for i, a := range res.Issued {
		out.Issued[i] = newAdminDomainAction(appmoderation.DomainActionView{
			DomainActionRecord: moderation.DomainActionRecord{Action: a}, State: moderation.ActionActive,
		})
	}
	for i, sk := range res.Skipped {
		out.Skipped[i] = DomainBlockImportSkipped{Domain: sk.Domain, Reason: DomainBlockImportStatus(sk.Reason)}
	}
	cache := cacheNoStore
	return ImportDomainBlocks200JSONResponse{Body: out, Headers: ImportDomainBlocks200ResponseHeaders{CacheControl: &cache}}, nil
}

func newDomainBlock(b moderation.DomainBlock) DomainBlock {
	out := DomainBlock{
		Domain: b.Domain, Severity: DomainBlockSeverity(b.Severity), RejectMedia: b.RejectMedia,
		RejectReports: b.RejectReports, Obfuscate: b.Obfuscate,
	}
	if b.PublicComment != "" {
		out.PublicComment = &b.PublicComment
	}
	return out
}
