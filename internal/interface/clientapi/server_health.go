// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"net/http"
	"time"
)

// HealthCheck reports whether the server can serve requests, for GET /instance/health
// (ADR-0085 §1). The platform implements it with the readiness check behind a cache, so that
// the endpoint reaches the database at most once a minute per process.
type HealthCheck interface {
	// Check returns whether the last check passed and when it ran.
	Check(ctx context.Context) (healthy bool, checkedAt time.Time)
}

// serverHealthPattern is the http.ServeMux pattern of GET /instance/health.
const serverHealthPattern = "GET " + BasePath + "/instance/health"

// wrapOperation wraps the handler of the operation of pattern in the guard, except GET
// /instance/health: uptime monitors call it constantly, and its answer must not depend on the
// database. The guard looks up access tokens and takes from the rate limits, which may both
// read the database (the key-value store may be the database), whereas the cache of
// the HealthCheck bounds the checks instead of a rate limit (ADR-0085 §1). The operation takes no
// access token and says nothing that depends on the viewer.
func (s *Server) wrapOperation(pattern string, h http.Handler) http.Handler {
	if pattern == serverHealthPattern {
		return h
	}
	return s.guard(pattern, h)
}

// GetServerHealth serves GET /api/v1/instance/health: 200 with status ok, or 503 with status
// unavailable, and when the check ran. The body holds only these fixed words, never the reason
// of a failure (ADR-0019), which the platform logs instead. The maintenance mode (ADR-0084)
// answers this path itself, without this server.
func (s *Server) GetServerHealth(ctx context.Context, _ GetServerHealthRequestObject) (GetServerHealthResponseObject, error) {
	healthy, checkedAt := false, time.Now()
	if s.o.ServerHealth != nil {
		healthy, checkedAt = s.o.ServerHealth.Check(ctx)
	}
	checkedAt = checkedAt.UTC().Truncate(time.Second)
	noStore := cacheNoStore
	if healthy {
		return GetServerHealth200JSONResponse{
			Body:    ServerHealth{Status: ServerHealthStatusOk, CheckedAt: &checkedAt},
			Headers: GetServerHealth200ResponseHeaders{CacheControl: &noStore},
		}, nil
	}
	return GetServerHealth503JSONResponse{
		Body:    ServerHealth{Status: ServerHealthStatusUnavailable, CheckedAt: &checkedAt},
		Headers: GetServerHealth503ResponseHeaders{CacheControl: &noStore},
	}, nil
}
