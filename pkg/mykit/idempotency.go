// SPDX-License-Identifier: MIT

package mykit

import (
	"net/http"
	"strings"
)

// idempotencyKeyHeader is the header that makes a request safe to send again (draft-ietf-httpapi-idempotency-key-header).
const idempotencyKeyHeader = "Idempotency-Key"

// problemTypeIdempotencyKeyInUse is the 409 of a request whose key's first request is still being carried out.
const problemTypeIdempotencyKeyInUse = "/api/v1/problems/idempotency-key-in-use"

// writesWithoutIdempotencyKey are the writes whose spec takes no Idempotency-Key, since their answer carries a
// secret or a payment provider's URL that the server does not keep to send again, or each call must reach the
// audit log; every other write takes one. They are the method and the path
// relative to the API's base path, with * for a path parameter. When the caller sends no key to a write that takes
// one, the client adds a new one, so that the request can be retried. TestIdempotencyKeyOperationsMatchGenerated
// keeps the list equal to the writes of api.gen.go that send no such header.
var writesWithoutIdempotencyKey = []struct{ method, path string }{
	{http.MethodPost, "/two_factor/totp/setup"},
	{http.MethodPost, "/two_factor/totp"},
	{http.MethodPost, "/two_factor/recovery_codes"},
	{http.MethodPost, "/passkeys/registration"},
	{http.MethodPost, "/passkeys/confirmation"},
	{http.MethodPost, "/linked_identities"},
	{http.MethodPost, "/linked_identities/confirmation"},
	{http.MethodPost, "/oauth_apps"},
	{http.MethodPost, "/oauth_apps/*/rotate_secret"},
	{http.MethodPost, "/oauth_apps/*/tokens"},
	{http.MethodPost, "/voice/rooms"},
	{http.MethodPost, "/voice/rooms/*/participation"},
	{http.MethodPost, "/voice/rooms/*/grant"},
	{http.MethodPost, "/admin/webhooks"},
	{http.MethodPost, "/admin/webhooks/*/rotate_secret"},
	{http.MethodPost, "/admin/accounts/*/password_reset_link"},
	{http.MethodPost, "/admin/accounts/*/reveal"},
	{http.MethodPost, "/admin/instances/*/deliveries/*/reveal"},
	{http.MethodPost, "/supporter/checkout"},
	{http.MethodPost, "/supporter/tips"},
	{http.MethodPost, "/supporter/portal"},
}

// isWrite reports whether method is one of a write: POST, PUT, PATCH, or DELETE.
func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// acceptsIdempotencyKey reports whether the request to path (under basePath) is a write that takes an
// Idempotency-Key: any write but writesWithoutIdempotencyKey.
func acceptsIdempotencyKey(method, path, basePath string) bool {
	rel, ok := strings.CutPrefix(path, basePath)
	if !ok || !strings.HasPrefix(rel, "/") || !isWrite(method) {
		return false
	}
	for _, op := range writesWithoutIdempotencyKey {
		if op.method == method && pathMatches(op.path, rel) {
			return false
		}
	}
	return true
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
