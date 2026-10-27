// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
)

// The move of the viewer's own account to another (FR-009, ADR-0089): the move, confirmed by the
// current password or a passkey; its cancellation; and the operations that a moved account
// cannot do (posting, boosting, reacting, following, voting), which are refused with
// ProblemTypeAccountMoved.

// Problem types of the moves of accounts.
const (
	// ProblemTypeAccountMoved is the problem of an operation that a moved account cannot do.
	ProblemTypeAccountMoved = "/api/v1/problems/account-moved"
	// ProblemTypeAccountNotMoved is the problem of the cancellation of a move that is not in
	// force.
	ProblemTypeAccountNotMoved = "/api/v1/problems/account-not-moved"
	// ProblemTypeMoveAliasMissing is the problem of a move to an account whose aliases do not
	// name the viewer's.
	ProblemTypeMoveAliasMissing = "/api/v1/problems/move-alias-missing"
	// ProblemTypeMoveTargetMoved is the problem of a move to an account that moved itself.
	ProblemTypeMoveTargetMoved = "/api/v1/problems/move-target-moved"
	// ProblemTypeMoveTargetUnavailable is the problem of a move to an account that cannot be
	// moved to.
	ProblemTypeMoveTargetUnavailable = "/api/v1/problems/move-target-unavailable"
	// ProblemTypeMoveTooSoon is the problem of a move within the wait after the last one.
	ProblemTypeMoveTooSoon = "/api/v1/problems/move-too-soon"
)

// MoveAccountLimit bounds the attempts to move the own account, which check a password or a
// passkey and fetch the target.
var MoveAccountLimit = kernel.Limit{Name: "api.account.move", Max: 10, Window: time.Hour}

// LocalMoves moves the viewer's own account and cancels its move; *appfederation.LocalMoves
// implements it.
type LocalMoves interface {
	Move(ctx context.Context, in appfederation.LocalMoveRequest) (appaccount.Move, error)
	Cancel(ctx context.Context, accountID string) error
}

// MoveConfirmations checks the password or passkey that confirms a move; *appidentity.Service
// implements it.
type MoveConfirmations interface {
	ConfirmMove(ctx context.Context, userID string, c appidentity.Confirmation) error
}

// MoveOwnAccount serves POST /api/v1/accounts/move.
func (s *Server) MoveOwnAccount(ctx context.Context, req MoveOwnAccountRequestObject) (MoveOwnAccountResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.LocalMoves == nil || s.o.MoveConfirmations == nil {
		return MoveOwnAccountdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if p, h, limited := s.LimitProblem(ctx, MoveAccountLimit, v.AccountID); limited {
		return MoveOwnAccountdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	if req.Body == nil || req.Body.TargetAccountId == "" {
		return MoveOwnAccountdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "target_account_id")), nil
	}
	c, param, ok := confirmationOf(req.Body.Password, req.Body.Passkey, req.Body.ExternalConfirmation, "password")
	if !ok {
		return MoveOwnAccountdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, param)), nil
	}
	err = s.o.MoveConfirmations.ConfirmMove(ctx, v.UserID, c)
	if p := s.passkeyProblem(ctx, err); p != nil {
		return MoveOwnAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: confirm the move of user %s: %w", v.UserID, err)
	}
	_, err = s.o.LocalMoves.Move(ctx, appfederation.LocalMoveRequest{
		AccountID: v.AccountID, UserID: v.UserID, TargetAccountID: knownID(req.Body.TargetAccountId),
	})
	if p := s.moveProblem(ctx, err); p != nil {
		return MoveOwnAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: move the account %s: %w", v.AccountID, err)
	}
	return MoveOwnAccount204Response{}, nil
}

// moveProblem returns the problem of a refused move, nil for any other err.
func (s *Server) moveProblem(ctx context.Context, err error) *problem {
	l := s.Localizer(ctx)
	var p problem
	switch {
	case errors.Is(err, appfederation.ErrMoveAliasMissing):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeMoveAliasMissing, l.ServerErrorsMoveAliasMissing(), "")
	case errors.Is(err, appfederation.ErrMoveTargetMoved):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeMoveTargetMoved, l.ServerErrorsMoveTargetMoved(), "")
	case errors.Is(err, appfederation.ErrMoveTargetUnavailable):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeMoveTargetUnavailable, l.ServerErrorsMoveTargetUnavailable(), "")
	case errors.Is(err, appaccount.ErrMoveTooSoon), errors.Is(err, appaccount.ErrMovedBefore):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeMoveTooSoon, l.ServerErrorsMoveTooSoon(), "")
	case errors.Is(err, appfederation.ErrMoveSourceUnavailable):
		p = s.problem(ctx, http.StatusForbidden, ProblemTypeAccountUnavailable, l.ServerErrorsAccountUnavailable(), "")
	default:
		return nil
	}
	return &p
}

// CancelOwnAccountMove serves POST /api/v1/accounts/move/cancel.
func (s *Server) CancelOwnAccountMove(ctx context.Context, _ CancelOwnAccountMoveRequestObject) (CancelOwnAccountMoveResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.LocalMoves == nil {
		return CancelOwnAccountMovedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	err = s.o.LocalMoves.Cancel(ctx, v.AccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotMoved):
		return CancelOwnAccountMovedefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusConflict, ProblemTypeAccountNotMoved, s.Localizer(ctx).ServerErrorsAccountNotMoved(), "")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: cancel the move of %s: %w", v.AccountID, err)
	}
	return CancelOwnAccountMove204Response{}, nil
}

// refuseMoved returns the problem ProblemTypeAccountMoved when the account accountID moved, nil
// when it has not or the server has no Options.Moves (ADR-0089).
func (s *Server) refuseMoved(ctx context.Context, accountID string) (*problem, error) {
	if s.o.Moves == nil {
		return nil, nil
	}
	_, err := s.o.Moves.MoveState(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: the move of %s: %w", accountID, err)
	}
	p := s.problem(ctx, http.StatusForbidden, ProblemTypeAccountMoved, s.Localizer(ctx).ServerErrorsAccountMoved(), "")
	return &p, nil
}
