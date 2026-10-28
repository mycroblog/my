// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// The viewer's follows, blocks, mutes, and bookmarks as Mastodon's CSV files: exports, and
// imports with a preview (FR-008).

// ProblemTypeImportInProgress is the problem of an import while another one is in progress.
const ProblemTypeImportInProgress = "/api/v1/problems/import-in-progress"

var (
	// CSVExportLimit bounds the CSV exports, each of which reads everything of a kind.
	CSVExportLimit = kernel.Limit{Name: "api.csv_export", Max: 30, Window: time.Hour}
	// ImportLimit bounds the imports, each of which may resolve many remote accounts.
	ImportLimit = kernel.Limit{Name: "api.import", Max: 10, Window: 24 * time.Hour}
)

// Imports writes the viewer's CSV exports and runs the CSV imports; *appidentity.Imports
// implements it.
type Imports interface {
	WriteExport(ctx context.Context, accountID string, kind identity.ImportKind, w io.Writer) error
	PreviewImport(ctx context.Context, in appidentity.ImportInput) (appidentity.ImportPreview, error)
	StartImport(ctx context.Context, in appidentity.ImportInput) (identity.DataImportProgress, error)
	ListImports(ctx context.Context, userID string) ([]identity.DataImportProgress, error)
}

// ExportCsv serves GET /api/v1/csv_exports/{kind}.
func (s *Server) ExportCsv(ctx context.Context, req ExportCsvRequestObject) (ExportCsvResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Imports == nil {
		return ExportCsvdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	kind := identity.ImportKind(req.Kind)
	if !kind.Valid() {
		return ExportCsvdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "kind")), nil
	}
	if p, h, limited := s.LimitProblem(ctx, CSVExportLimit, v.AccountID); limited {
		return ExportCsvdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	var b bytes.Buffer
	if err := s.o.Imports.WriteExport(ctx, v.AccountID, kind, &b); err != nil {
		return nil, fmt.Errorf("clientapi: export the %s of account %s: %w", kind, v.AccountID, err)
	}
	cache := cacheNoStore
	disposition := `attachment; filename="` + kind.FileName() + `"`
	return ExportCsv200TextcsvResponse{
		Body: &b, ContentLength: int64(b.Len()),
		Headers: ExportCsv200ResponseHeaders{CacheControl: &cache, ContentDisposition: &disposition},
	}, nil
}

// importInput checks the body of an import or its preview; it returns the problem to answer
// with when the body is not one.
func (s *Server) importInput(ctx context.Context, v Viewer, b *DataImportRequest) (appidentity.ImportInput, *problem) {
	if b == nil {
		p := s.invalidParameterResponse(ctx, "text")
		return appidentity.ImportInput{}, &p
	}
	in := appidentity.ImportInput{
		UserID: v.UserID, AccountID: v.AccountID, Kind: identity.ImportKind(b.Kind), Mode: identity.ImportMode(b.Mode), Text: b.Text,
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (appidentity.ImportInput, *problem) {
		p := s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)
		return appidentity.ImportInput{}, &p
	}
	switch {
	case !in.Kind.Valid():
		return unprocessable(l.ServerErrorsInvalidValue("kind"))
	case !in.Mode.Valid():
		return unprocessable(l.ServerErrorsInvalidValue("mode"))
	}
	return in, nil
}

// importProblem returns the problem of an error of PreviewImport or StartImport that the user
// can act on, or nil.
func (s *Server) importProblem(ctx context.Context, err error) *problem {
	l := s.Localizer(ctx)
	var p problem
	var parseErr *csv.ParseError
	switch {
	case errors.Is(err, identity.ErrTooManyImportEntries):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(),
			l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: identity.MaxImportEntries, Name: "text"}))
	case errors.As(err, &parseErr):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsImportInvalidCsv())
	case errors.Is(err, appidentity.ErrImportInProgress):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeImportInProgress, l.ServerErrorsImportInProgress(), "")
	default:
		return nil
	}
	return &p
}

// PreviewDataImport serves POST /api/v1/imports/preview.
func (s *Server) PreviewDataImport(ctx context.Context, req PreviewDataImportRequestObject) (PreviewDataImportResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Imports == nil {
		return PreviewDataImportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	in, p := s.importInput(ctx, v, req.Body)
	if p != nil {
		return PreviewDataImportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	preview, err := s.o.Imports.PreviewImport(ctx, in)
	if p := s.importProblem(ctx, err); p != nil {
		return PreviewDataImportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: preview an import of account %s: %w", v.AccountID, err)
	}
	out := DataImportPreview{
		Entries: len(preview.Entries), Present: preview.Present, Removals: len(preview.Removals),
		Skipped: make([]DataImportSkipped, len(preview.Skipped)),
	}
	for i, sk := range preview.Skipped {
		out.Skipped[i] = DataImportSkipped{Line: sk.Line, Value: sk.Value, Reason: DataImportSkippedReason(sk.Reason)}
	}
	cache := cacheNoStore
	return PreviewDataImport200JSONResponse{Body: out, Headers: PreviewDataImport200ResponseHeaders{CacheControl: &cache}}, nil
}

// StartDataImport serves POST /api/v1/imports.
func (s *Server) StartDataImport(ctx context.Context, req StartDataImportRequestObject) (StartDataImportResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Imports == nil {
		return StartDataImportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	in, p := s.importInput(ctx, v, req.Body)
	if p != nil {
		return StartDataImportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if p, h, limited := s.LimitProblem(ctx, ImportLimit, v.AccountID); limited {
		return StartDataImportdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	started, err := s.o.Imports.StartImport(ctx, in)
	if p := s.importProblem(ctx, err); p != nil {
		return StartDataImportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: start an import of account %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return StartDataImport202JSONResponse{Body: newDataImport(started), Headers: StartDataImport202ResponseHeaders{CacheControl: &cache}}, nil
}

// ListDataImports serves GET /api/v1/imports.
func (s *Server) ListDataImports(ctx context.Context, _ ListDataImportsRequestObject) (ListDataImportsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Imports == nil {
		return ListDataImportsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	is, err := s.o.Imports.ListImports(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the imports of user %s: %w", v.UserID, err)
	}
	items := make([]DataImport, len(is))
	for i, imp := range is {
		items[i] = newDataImport(imp)
	}
	cache := cacheNoStore
	return ListDataImports200JSONResponse{Body: DataImportList{Items: items}, Headers: ListDataImports200ResponseHeaders{CacheControl: &cache}}, nil
}

func newDataImport(p identity.DataImportProgress) DataImport {
	d := DataImport{
		Id: p.ID, Kind: DataImportKind(p.Kind), Mode: DataImportMode(p.Mode), CreatedAt: p.CreatedAt,
		Total: p.Total, Processed: p.Processed, Failed: p.Failed, State: DataImportStateInProgress,
	}
	if p.Finished() {
		d.State = DataImportStateFinished
	}
	return d
}
