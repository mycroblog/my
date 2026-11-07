// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mycroblog/my/internal/domain/instance"
)

// The client API of a one-person server (FR-811, ADR-0152 §4): the operations of what such a server
// does not offer answer a 404 of their own type, so that a client tells it from a missing resource.
// guard asks singleUserRefuses about every operation, so that no handler checks the mode itself;
// the streams of the public timelines check it again at each heartbeat (timelineDenied).

const (
	// ProblemTypeSingleUserMode is the 404 of an operation that a one-person server does not
	// offer (ADR-0152 §4); clients learn the mode from Instance.single_user.
	ProblemTypeSingleUserMode = "/api/v1/problems/single-user-mode"
	// ProblemTypeSingleUserRequiresOneAccount is the 422 of turning single_user on while the
	// server does not have exactly one local account (ADR-0152 §1).
	ProblemTypeSingleUserRequiresOneAccount = "/api/v1/problems/single-user-requires-one-account"
)

// singleUserOperations are the operations, by operationId, that a one-person server answers with
// the 404 single-user-mode: the local and federated timelines, the suggestions of accounts, the
// invites of users and of administrators, the review of registrations, and the sign-up blocks.
var singleUserOperations = map[string]bool{
	"getLocalTimeline": true, "getFederatedTimeline": true, "streamTimeline": true,
	"listAccountSuggestions": true,
	"listOwnInvites":         true, "createOwnInvite": true, "revokeOwnInvite": true,
	"listInvites": true, "createInvite": true, "revokeInvite": true,
	"listPendingRegistrations": true, "approveRegistration": true, "rejectRegistration": true,
	"listSignupBlocks": true, "createSignupBlock": true, "getSignupBlock": true, "liftSignupBlock": true,
	"importSignupBlocks": true,
}

// singleUserRefuses answers the 404 single-user-mode and reports true when operationID is one that
// a one-person server does not offer and the server, read per request, is one.
func (s *Server) singleUserRefuses(w http.ResponseWriter, r *http.Request, operationID string) bool {
	if !singleUserOperations[operationID] {
		return false
	}
	on, err := s.singleUser(r.Context())
	if err != nil {
		s.responseError(w, r, err)
		return true
	}
	if !on {
		return false
	}
	s.writeProblemOf(w, r, s.singleUserProblem(r.Context()))
	return true
}

// singleUser reports whether the server is a one-person server; without Options.Instance it is
// not.
func (s *Server) singleUser(ctx context.Context) (bool, error) {
	if s.o.Instance == nil {
		return false, nil
	}
	settings, err := s.o.Instance.Settings(ctx)
	if err != nil {
		return false, fmt.Errorf("clientapi: instance settings: %w", err)
	}
	return settings.SingleUser(), nil
}

// singleUserProblem is the 404 single-user-mode.
func (s *Server) singleUserProblem(ctx context.Context) problem {
	return s.problem(ctx, http.StatusNotFound, ProblemTypeSingleUserMode, s.Localizer(ctx).ServerErrorsSingleUserMode(), "")
}

// singleUserSettingsProblem is the 422 single-user-requires-one-account of a server with count
// local accounts.
func (s *Server) singleUserSettingsProblem(ctx context.Context, count int) problem {
	l := s.Localizer(ctx)
	return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeSingleUserRequiresOneAccount,
		l.ServerErrorsSingleUserRequiresOneAccount(), l.ServerErrorsSingleUserRequiresOneAccountDetail(count))
}

// timelineSingleUserProblem returns the 404 single-user-mode for a public timeline on a one-person
// server, and nil otherwise.
func (s *Server) timelineSingleUserProblem(ctx context.Context, settings instance.Settings) *problem {
	if !settings.SingleUser() {
		return nil
	}
	p := s.singleUserProblem(ctx)
	return &p
}
