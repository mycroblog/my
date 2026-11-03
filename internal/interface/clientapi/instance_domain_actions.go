// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mycroblog/my/internal/domain/instance"
)

// ListInstanceDomainActions serves GET /api/v1/instance/domain_actions: the limits and
// suspensions of remote domains in effect, newest first (FR-718). The instance setting
// domain_actions_visibility decides who may read it: off is a 404, signed_in a 401 for anonymous
// requests. domain_actions_rationale_visibility decides in the same way who gets the public
// comments. The cursor is the ID of the last action of the previous page.
func (s *Server) ListInstanceDomainActions(ctx context.Context, req ListInstanceDomainActionsRequestObject) (ListInstanceDomainActionsResponseObject, error) {
	var settings instance.Settings
	if s.o.Instance != nil {
		var err error
		if settings, err = s.o.Instance.Settings(ctx); err != nil {
			return nil, fmt.Errorf("clientapi: instance settings: %w", err)
		}
	}
	_, loggedIn := ViewerFrom(ctx)
	visibility := settings.DomainActionsVisibility()
	if !visibility.Allows(loggedIn) {
		if visibility != instance.AudienceSignedIn {
			return ListInstanceDomainActionsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
		}
		body, headers := s.NewProblem(ctx, http.StatusUnauthorized, ProblemTypeUnauthenticated, s.Localizer(ctx).ServerErrorsUnauthenticated(), "")
		challenge := "Bearer"
		headers.WWWAuthenticate = &challenge
		return ListInstanceDomainActionsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusUnauthorized}, nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListInstanceDomainActionsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	page := InstanceDomainActionPage{Items: []InstanceDomainAction{}}
	if s.o.Moderation != nil {
		as, err := s.o.Moderation.PublicDomainActions(ctx, q.before, q.limit+1)
		if err != nil {
			return nil, fmt.Errorf("clientapi: list public domain actions: %w", err)
		}
		as, last := trim(as, q.limit)
		comments := settings.DomainActionsRationaleVisibility().Allows(loggedIn)
		for _, a := range as {
			item := InstanceDomainAction{Domain: a.Domain, Digest: a.Digest, Kind: InstanceDomainActionKind(a.Kind)}
			if comments && a.Comment != "" {
				item.Comment = &a.Comment
			}
			page.Items = append(page.Items, item)
		}
		if last >= 0 {
			page.NextCursor = encodeCursor(as[last].ID)
		}
	}
	cache := cachePublic
	if loggedIn {
		cache = cachePrivate
	}
	return ListInstanceDomainActions200JSONResponse{Body: page, Headers: ListInstanceDomainActions200ResponseHeaders{CacheControl: &cache}}, nil
}
