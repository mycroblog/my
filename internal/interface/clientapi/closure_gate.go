// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"net/http"

	"github.com/mycroblog/my/internal/domain/instance"
)

// The client API of a closed server (FR-809, ADR-0088 §2): the users sign in (outside the client
// API) and take their data away, and nothing else runs. guard asks closureRefuses about every
// operation, the streaming endpoint included, so that no handler checks the closure itself.

// ProblemTypeServiceClosed is the problem type of an operation that a closed server refuses
// (403).
const ProblemTypeServiceClosed = "/api/v1/problems/service-closed"

// ProblemTypeServiceClosing is the problem type of a registration or a supporter subscription
// that the announcement of the service closure stops (409).
const ProblemTypeServiceClosing = "/api/v1/problems/service-closing"

// serviceClosingProblem is the 409 service-closing.
func (s *Server) serviceClosingProblem(ctx context.Context) problem {
	l := s.Localizer(ctx)
	return s.problem(ctx, http.StatusConflict, ProblemTypeServiceClosing, l.ServerErrorsServiceClosing(), l.ServerErrorsServiceClosingDetail())
}

// closureOperations are the operations, by operationId, that the users need to take their data
// away from a closed server (instance.ClosureOwnData): their own account and its settings to
// read, the exports, their sessions, and the instance; and the administrators' operations that
// follow the closure's deliveries (its state and the queue, whose dead jobs they may retry,
// ADR-0088 §3). Every other operation is instance.ClosureClientAPI, and the streaming endpoint
// instance.ClosureStream.
var closureOperations = map[string]bool{
	"verifyAccountCredentials": true, "getPreferences": true, "getPasswordStatus": true,
	"getTwoFactor": true, "listPasskeys": true,
	"listDataExports": true, "requestDataExport": true, "downloadDataExport": true, "exportCsv": true,
	"listAuthorizedApps": true, "revokeAuthorizedApp": true,
	"getInstance": true, "getServerHealth": true, "getInstanceDocument": true,
	"getAdminServiceClosure": true, "getAdminServiceClosureCheck": true, "closeService": true,
	"getAdminQueue": true, "listAdminJobs": true, "listAdminJobAttempts": true, "retryAdminJob": true,
}

// closureOperation returns the operation of the service closure that operationID is.
func closureOperation(operationID string) instance.ClosureOperation {
	switch {
	case operationID == "streamEvents":
		return instance.ClosureStream
	case closureOperations[operationID]:
		return instance.ClosureOwnData
	}
	return instance.ClosureClientAPI
}

// closureRefuses answers the 403 service-closed and reports true when the state of the service
// closure, read per request, does not run operationID.
func (s *Server) closureRefuses(w http.ResponseWriter, r *http.Request, operationID string) bool {
	if s.o.Closures == nil {
		return false
	}
	c, err := s.o.Closures.Closure(r.Context())
	if err != nil {
		s.responseError(w, r, err)
		return true
	}
	if instance.ClosureAllows(c.CurrentState(), closureOperation(operationID)) {
		return false
	}
	l := s.localizer(r)
	s.writeProblem(w, r, http.StatusForbidden, ProblemTypeServiceClosed, l.ServerErrorsServiceClosed(), l.ServerErrorsServiceClosedDetail())
	return true
}
