// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// CORS of the client API (ADR-0019 §4). The API authenticates only with the Authorization
// header, never with cookies, so it allows the configured web client origin without
// Access-Control-Allow-Credentials.
const (
	corsAllowMethods = "GET, POST, PUT, PATCH, DELETE"
	// Accept-Language is listed because browsers treat some of its values as not safelisted.
	corsAllowHeaders  = "Authorization, Content-Type, Accept-Language, Idempotency-Key"
	corsExposeHeaders = "Content-Language, X-Request-Id, WWW-Authenticate, Retry-After, RateLimit, RateLimit-Policy, " + clientapi.HeaderWebClientEpoch
	corsMaxAge        = "600"
)

// clientAPIEpoch adds the web client's epoch (ADR-0141 §4) to the responses of the client API,
// including its errors: a client too old to work may get nothing else.
func clientAPIEpoch(next http.Handler) http.Handler {
	epoch := strconv.Itoa(clientapi.WebClientEpoch)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == clientapi.BasePath || strings.HasPrefix(r.URL.Path, clientapi.BasePath+"/") {
			w.Header().Set(clientapi.HeaderWebClientEpoch, epoch)
		}
		next.ServeHTTP(w, r)
	})
}

// clientAPICORS answers the CORS preflight requests of the client API and adds the CORS
// headers to its responses, including the problem documents of the fallback 404 and 405.
// Requests outside the client API pass through untouched.
func clientAPICORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != clientapi.BasePath && !strings.HasPrefix(r.URL.Path, clientapi.BasePath+"/") {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Add("Vary", "Origin")
		allowed := r.Header.Get("Origin") == origin
		if allowed {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", corsExposeHeaders)
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			if allowed {
				h.Set("Access-Control-Allow-Methods", corsAllowMethods)
				h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
				h.Set("Access-Control-Max-Age", corsMaxAge)
			}
			// Without the allow headers the browser rejects the request itself.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
