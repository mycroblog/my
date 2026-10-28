// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// The bulk import of the sign-up block list (manage_accounts): a moderator sends a list, such as
// a public list of disposable email domains, and learns which lines were skipped and why.

// ImportSignupBlocks serves POST /api/v1/admin/signup_blocks/import.
func (s *Server) ImportSignupBlocks(ctx context.Context, req ImportSignupBlocksRequestObject) (ImportSignupBlocksResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return ImportSignupBlocksdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (ImportSignupBlocksResponseObject, error) {
		return ImportSignupBlocksdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	b := req.Body
	if b == nil {
		return ImportSignupBlocksdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "text")), nil
	}
	in := appmoderation.SignupBlockImportInput{
		Kind: moderation.SignupBlockKind(b.Kind), Text: b.Text, Severity: moderation.SignupBlockSeverity(b.Severity),
		Comment: deref(b.Comment), CreatedByUserID: v.UserID,
	}
	switch {
	case !in.Kind.Valid():
		return unprocessable(l.ServerErrorsInvalidValue("kind"))
	case !in.Severity.Valid():
		return unprocessable(l.ServerErrorsInvalidValue("severity"))
	}
	if b.ExpiresIn != nil {
		if !moderation.ValidDurationSeconds(int64(*b.ExpiresIn)) {
			return unprocessable(l.ServerErrorsInvalidValue("expires_in"))
		}
		in.Duration = time.Duration(*b.ExpiresIn) * time.Second
	}
	res, err := s.o.SignupBlocks.ImportSignupBlocks(ctx, in)
	var textErr *moderation.TextError
	switch {
	case errors.Is(err, appmoderation.ErrTooManySignupBlocks):
		return unprocessable(l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: moderation.MaxSignupBlockImport, Name: "text"}))
	case errors.As(err, &textErr) && textErr.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: textErr.Max, Name: textErr.Field}))
	case errors.As(err, &textErr):
		return unprocessable(l.ServerErrorsInvalidValue(textErr.Field))
	case err != nil:
		return nil, fmt.Errorf("clientapi: import signup blocks: %w", err)
	}
	out := SignupBlockImportResult{Added: res.Added, Skipped: make([]SignupBlockImportSkipped, len(res.Skipped))}
	for i, sk := range res.Skipped {
		out.Skipped[i] = SignupBlockImportSkipped{Line: sk.Line, Value: sk.Value, Reason: SignupBlockImportSkippedReason(sk.Reason)}
	}
	cache := cacheNoStore
	return ImportSignupBlocks200JSONResponse{Body: out, Headers: ImportSignupBlocks200ResponseHeaders{CacheControl: &cache}}, nil
}
