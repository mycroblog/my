// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// The viewer's appeals of the actions on their own account (FR-707, ADR-0038). The moderation
// module checks the appeal window and that an action is appealed once.

// Problem types of the appeals.
const (
	// ProblemTypeAlreadyAppealed is the problem of a second appeal of one action.
	ProblemTypeAlreadyAppealed = "/api/v1/problems/already-appealed"
	// ProblemTypeAppealWindowClosed is the problem of an appeal after the appeal window.
	ProblemTypeAppealWindowClosed = "/api/v1/problems/appeal-window-closed"
	// ProblemTypeNotAppealable is the problem of an appeal, or the overturning of one, of a ban
	// whose account is deleted.
	ProblemTypeNotAppealable = "/api/v1/problems/not-appealable"
	// ProblemTypeAppealDecided is the problem of a decision on an appeal decided otherwise.
	ProblemTypeAppealDecided = "/api/v1/problems/appeal-decided"
)

// OwnAppeals runs the use cases of the viewer's appeals. *appmoderation.Service implements it.
type OwnAppeals interface {
	FileAppeal(ctx context.Context, accountID, actionID, text string) (appmoderation.TargetActionView, error)
	OwnAppealAction(ctx context.Context, accountID, appealID string) (appmoderation.TargetActionView, error)
}

// AppealModerationAction serves POST /api/v1/moderation/actions/{id}/appeal.
func (s *Server) AppealModerationAction(ctx context.Context, req AppealModerationActionRequestObject) (AppealModerationActionResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnAppeals == nil {
		return AppealModerationActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return AppealModerationActiondefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "text")), nil
	}
	l := s.Localizer(ctx)
	refuse := func(typ, title, detail string) (AppealModerationActionResponseObject, error) {
		return AppealModerationActiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, typ, title, detail)), nil
	}
	a, err := s.o.OwnAppeals.FileAppeal(ctx, v.AccountID, req.Id, req.Body.Text)
	var textErr *moderation.TextError
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		return AppealModerationActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appmoderation.ErrAlreadyAppealed):
		return refuse(ProblemTypeAlreadyAppealed, l.ServerErrorsAppealAlreadyAppealed(), "")
	case errors.Is(err, appmoderation.ErrAppealWindowClosed):
		return refuse(ProblemTypeAppealWindowClosed, l.ServerErrorsAppealWindowClosed(), "")
	case errors.Is(err, appmoderation.ErrNotAppealable):
		return refuse(ProblemTypeNotAppealable, l.ServerErrorsAppealNotAppealable(), "")
	case errors.As(err, &textErr) && textErr.Max > 0:
		return refuse(ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: textErr.Max, Name: textErr.Field}))
	case errors.As(err, &textErr):
		return refuse(ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(textErr.Field))
	case err != nil:
		return nil, fmt.Errorf("clientapi: appeal action %s of account %s: %w", req.Id, v.AccountID, err)
	}
	cache := cacheNoStore
	return AppealModerationAction200JSONResponse{
		Body: newModerationAction(l, a), Headers: AppealModerationAction200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// appealActions returns, by appeal ID, the actions on accountID whose appeals the appeal_decided
// notifications ns tell of, with the appeals; an appeal that no longer exists is left out. nil
// OwnAppeals reads none.
func (s *Server) appealActions(ctx context.Context, accountID string, ns []notification.Notification) (map[string]ModerationAction, error) {
	out := map[string]ModerationAction{}
	if s.o.OwnAppeals == nil {
		return out, nil
	}
	l := s.Localizer(ctx)
	for _, n := range ns {
		if _, ok := out[n.AppealID]; ok || n.Type != notification.TypeAppealDecided || n.AppealID == "" {
			continue
		}
		v, err := s.o.OwnAppeals.OwnAppealAction(ctx, accountID, n.AppealID)
		switch {
		case errors.Is(err, appmoderation.ErrNotFound):
			continue
		case err != nil:
			return nil, fmt.Errorf("clientapi: the action of appeal %s of a notice to %s: %w", n.AppealID, accountID, err)
		}
		out[n.AppealID] = newModerationAction(l, v)
	}
	return out, nil
}
