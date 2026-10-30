// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"net/http"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Ids that come from a client are checked before they reach the application layer, which
// passes them to the database as they are. Only the 36 lowercase characters of a UUIDv7 name a
// resource (kernel.ValidID, ADR-0078 §5): any other form means the same as an ID that
// names nothing, and answers like one, so that a malformed ID shows no more than a missing one
// (ADR-0018).
//
//   - A path ID is checked for every operation by pathIDsValid, in the guard.
//   - A cursor that holds an ID is checked by parsePage and validCursor; a cursor that
//     holds something else (a domain, a time and an ID) is parsed by its own decoder.
//   - A query or body ID is checked where a handler reads it, and answers as the handler
//     does for an ID that names nothing.

// pathIDParams are the names of the path parameters of the spec that carry an ID (the Id
// schema); the other path parameters are domains, keys and kinds.
var pathIDParams = [...]string{"id", "user_id", "role_id", "delivery_id", "quote_id", "account_id"}

// pathIDsValid reports whether every ID in the path of r is canonical.
func pathIDsValid(r *http.Request) bool {
	for _, name := range pathIDParams {
		if v := r.PathValue(name); v != "" && !kernel.ValidID(v) {
			return false
		}
	}
	return true
}

// validCursor reports whether cursor is absent or the canonical ID that a cursor of the plain
// kind carries.
func validCursor(cursor *Cursor) bool { return cursor == nil || kernel.ValidID(*cursor) }

// unknownID is a canonical ID that no resource has: the UUIDv7 of the Unix epoch's first
// millisecond, which this server never generates.
const unknownID = "00000000-0000-7000-8000-000000000000"

// knownID returns id when it is empty (absent) or canonical, and unknownID otherwise. A handler
// passes a query or body ID through it, so that a malformed ID takes the same way as an ID
// that names nothing, whatever the response of that field is, and never reaches the
// adapter.
func knownID(id string) string {
	if id == "" || kernel.ValidID(id) {
		return id
	}
	return unknownID
}

// knownIDs is knownID for each of ids; it returns ids itself when all are canonical.
func knownIDs(ids []string) []string {
	var out []string
	for i, id := range ids {
		if k := knownID(id); k != id {
			if out == nil {
				out = append([]string(nil), ids...)
			}
			out[i] = k
		}
	}
	if out == nil {
		return ids
	}
	return out
}

// knownIDPtr is knownID for an optional ID.
func knownIDPtr(id *string) *string {
	if id == nil {
		return nil
	}
	k := knownID(*id)
	return &k
}

// plainPageProblem returns the 400 problem of a page request of the plain kind, whose cursor
// is the ID of the last item of the previous page (not base64), when its limit or cursor is
// invalid, or nil.
func (s *Server) plainPageProblem(ctx context.Context, cursor *Cursor, limit *Limit) *problem {
	name := ""
	switch {
	case !limitValid(limit):
		name = "limit"
	case !validCursor(cursor):
		name = "cursor"
	default:
		return nil
	}
	p := s.invalidParameterResponse(ctx, name)
	return &p
}

func limitValid(limit *Limit) bool {
	_, ok := pageLimit(limit)
	return ok
}
