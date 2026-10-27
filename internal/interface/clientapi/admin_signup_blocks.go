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
)

// The admin operations of the sign-up block list (FR-719, manage_accounts). The moderation module
// records the blocks and audits every change; the registration page screens sign-ups against
// the blocks in effect.

// SignupBlocks runs the moderators' use cases of the sign-up block list.
// *appmoderation.Service implements it.
type SignupBlocks interface {
	SignupBlocks(ctx context.Context, f appmoderation.SignupBlockFilter, limit int) ([]appmoderation.SignupBlockView, error)
	SignupBlock(ctx context.Context, id string) (appmoderation.SignupBlockView, error)
	AddSignupBlock(ctx context.Context, in appmoderation.SignupBlockInput) (moderation.SignupBlock, error)
	LiftSignupBlock(ctx context.Context, blockID, liftedByUserID string) error
	ImportSignupBlocks(ctx context.Context, in appmoderation.SignupBlockImportInput) (appmoderation.SignupBlockImportResult, error)
}

func newAdminSignupBlock(v appmoderation.SignupBlockView) AdminSignupBlock {
	b := v.Block
	out := AdminSignupBlock{
		Id: b.ID, Kind: AdminSignupBlockKind(b.Kind), Value: b.Value, Severity: AdminSignupBlockSeverity(b.Severity),
		State: AdminSignupBlockState(v.State), CreatedAt: b.CreatedAt.UTC(),
	}
	if b.Comment != "" {
		out.Comment = &b.Comment
	}
	if b.ModerationActionID != "" {
		out.ModerationActionId = &b.ModerationActionID
	}
	if b.ExpiresAt != nil {
		t := b.ExpiresAt.UTC()
		out.ExpiresAt = &t
	}
	if v.Lift != nil {
		t := v.Lift.LiftedAt.UTC()
		out.LiftedAt = &t
	}
	return out
}

// signupBlockResponse answers an operation on one sign-up block.
func signupBlockResponse(v appmoderation.SignupBlockView) AdminSignupBlockJSONResponse {
	cache := cacheNoStore
	return AdminSignupBlockJSONResponse{Body: newAdminSignupBlock(v), Headers: AdminSignupBlockResponseHeaders{CacheControl: &cache}}
}

// ListSignupBlocks serves GET /api/v1/admin/signup_blocks, newest first. The cursor is the ID of
// the last block of the previous page.
func (s *Server) ListSignupBlocks(ctx context.Context, req ListSignupBlocksRequestObject) (ListSignupBlocksResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return ListSignupBlocksdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListSignupBlocksdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	f := appmoderation.SignupBlockFilter{Before: q.before, Active: req.Params.Active != nil && *req.Params.Active}
	if req.Params.Kind != nil {
		f.Kind = moderation.SignupBlockKind(*req.Params.Kind)
	}
	vs, err := s.o.SignupBlocks.SignupBlocks(ctx, f, q.limit+1)
	if errors.Is(err, appmoderation.ErrInvalidQuery) {
		return ListSignupBlocksdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "kind")), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: list signup blocks: %w", err)
	}
	vs, last := trim(vs, q.limit)
	page := AdminSignupBlockPage{Items: make([]AdminSignupBlock, 0, len(vs))}
	for _, v := range vs {
		page.Items = append(page.Items, newAdminSignupBlock(v))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(vs[last].Block.ID)
	}
	cache := cacheNoStore
	return ListSignupBlocks200JSONResponse{Body: page, Headers: ListSignupBlocks200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateSignupBlock serves POST /api/v1/admin/signup_blocks.
func (s *Server) CreateSignupBlock(ctx context.Context, req CreateSignupBlockRequestObject) (CreateSignupBlockResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return CreateSignupBlockdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (CreateSignupBlockResponseObject, error) {
		return CreateSignupBlockdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	b := req.Body
	if b == nil {
		return CreateSignupBlockdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "kind")), nil
	}
	in := appmoderation.SignupBlockInput{
		Kind: moderation.SignupBlockKind(b.Kind), Value: b.Value, Severity: moderation.SignupBlockSeverity(b.Severity),
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
	block, err := s.o.SignupBlocks.AddSignupBlock(ctx, in)
	var textErr *moderation.TextError
	switch {
	case errors.As(err, &textErr) && textErr.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(textErr.Max, textErr.Field))
	case errors.As(err, &textErr):
		return unprocessable(l.ServerErrorsInvalidValue(textErr.Field))
	case errors.Is(err, appmoderation.ErrInvalidSignupBlock):
		return unprocessable(l.ServerErrorsInvalidValue("value"))
	case err != nil:
		return nil, fmt.Errorf("clientapi: add a signup block: %w", err)
	}
	view, err := s.o.SignupBlocks.SignupBlock(ctx, block.ID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: signup block %s: %w", block.ID, err)
	}
	return CreateSignupBlock200JSONResponse{signupBlockResponse(view)}, nil
}

// GetSignupBlock serves GET /api/v1/admin/signup_blocks/{id}.
func (s *Server) GetSignupBlock(ctx context.Context, req GetSignupBlockRequestObject) (GetSignupBlockResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return GetSignupBlockdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	view, err := s.o.SignupBlocks.SignupBlock(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return GetSignupBlockdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: signup block %s: %w", req.Id, err)
	}
	return GetSignupBlock200JSONResponse{signupBlockResponse(view)}, nil
}

// LiftSignupBlock serves POST /api/v1/admin/signup_blocks/{id}/lift.
func (s *Server) LiftSignupBlock(ctx context.Context, req LiftSignupBlockRequestObject) (LiftSignupBlockResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return LiftSignupBlockdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.SignupBlocks.LiftSignupBlock(ctx, req.Id, v.UserID)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return LiftSignupBlockdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: lift signup block %s: %w", req.Id, err)
	}
	view, err := s.o.SignupBlocks.SignupBlock(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: signup block %s: %w", req.Id, err)
	}
	return LiftSignupBlock200JSONResponse{signupBlockResponse(view)}, nil
}
