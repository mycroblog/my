// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"maps"
	"net/http"
	"slices"

	"github.com/mycroblog/my/internal/domain/identity"
)

// RequestErrorHandler and ResponseErrorHandler expose the error handlers that Register gives
// the generated code, so that tests reach them without an operation that fails.
func RequestErrorHandler(s *Server) func(http.ResponseWriter, *http.Request, error) {
	return s.requestError
}

func ResponseErrorHandler(s *Server) func(http.ResponseWriter, *http.Request, error) {
	return s.responseError
}

// GuardedPatterns returns the sorted patterns of security.gen.go.
func GuardedPatterns() []string { return slices.Sorted(maps.Keys(operationSecurity)) }

// WithOperationAccess registers an operation's security under pattern for the test, so that
// Guard accepts it. The entry goes when the test ends.
func WithOperationAccess(t interface{ Cleanup(func()) }, pattern, operation string, anonymous bool, deprecated, sunset int64) {
	operationSecurity[pattern] = operationAccess{Operation: operation, Anonymous: anonymous, Deprecated: deprecated, Sunset: sunset}
	t.Cleanup(func() { delete(operationSecurity, pattern) })
}

// RecentIDs exposes the memory of the home timeline entries a stream sent.
type RecentIDs = recentIDs

// NewRecentIDs exposes newRecentIDs.
func NewRecentIDs(capacity int) *RecentIDs { return newRecentIDs(capacity) }

func (r *RecentIDs) Has(id string) bool { return r.has(id) }
func (r *RecentIDs) Add(id string)      { r.add(id) }

// Guard exposes Server.guard.
func Guard(s *Server, pattern string, h http.Handler) http.Handler { return s.guard(pattern, h) }

// OperationPermission returns the operationId and the x-permission of the operation of pattern.
func OperationPermission(pattern string) (operation string, permission identity.Permission) {
	a := operationSecurity[pattern]
	return a.Operation, a.Permission
}
