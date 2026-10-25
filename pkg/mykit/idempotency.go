// SPDX-License-Identifier: MIT

package mykit

import (
	"net/http"
	"strings"
)

// idempotencyKeyHeader is the header that makes a request safe to send again (draft-ietf-httpapi-idempotency-key-header).
const idempotencyKeyHeader = "Idempotency-Key"

// idempotencyKeyOperations are the operations whose spec takes an Idempotency-Key, as the method and the path
// relative to the API's base path, with * for a path parameter. When the caller sends none, the client adds a
// new one, so that the request can be retried. TestIdempotencyKeyOperationsMatchGenerated keeps the list equal
// to the operations of api.gen.go that send the header.
var idempotencyKeyOperations = []struct{ method, path string }{
	{http.MethodPost, "/statuses"},
}

// acceptsIdempotencyKey reports whether the request to path (under basePath) is one of idempotencyKeyOperations.
func acceptsIdempotencyKey(method, path, basePath string) bool {
	rel, ok := strings.CutPrefix(path, basePath)
	if !ok {
		return false
	}
	for _, op := range idempotencyKeyOperations {
		if op.method == method && pathMatches(op.path, rel) {
			return true
		}
	}
	return false
}

// pathMatches reports whether path matches pattern, segment by segment, * matching one non-empty segment.
func pathMatches(pattern, path string) bool {
	ps, ss := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(ps) != len(ss) {
		return false
	}
	for i, p := range ps {
		if p == "*" {
			if ss[i] == "" {
				return false
			}
		} else if p != ss[i] {
			return false
		}
	}
	return true
}
