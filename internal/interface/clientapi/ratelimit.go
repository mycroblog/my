// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Rate limits of the client API (ADR-0019 §5): requests without a valid access token share a
// limit per client IP (IPv6 by /64), authenticated requests one per account.
var (
	AnonymousLimit = kernel.Limit{Name: "api.ip", Max: 300, Window: 5 * time.Minute}
	AccountLimit   = kernel.Limit{Name: "api.account", Max: 1500, Window: 5 * time.Minute}
	// PostLimit bounds the statuses one account posts (createStatus).
	PostLimit = kernel.Limit{Name: "api.post", Max: 300, Window: 3 * time.Hour}
	// ResolveLimit bounds the remote accounts one account fetches with lookupAccount's resolve
	// (FR-305): each fetch is WebFinger and actor requests to another server.
	ResolveLimit = kernel.Limit{Name: "api.resolve", Max: 30, Window: 5 * time.Minute}
	// OAuthAppRegisterLimit bounds the OAuth apps one user registers (ADR-0090 §7); the 25 apps
	// a user may keep are a separate cap.
	OAuthAppRegisterLimit = kernel.Limit{Name: "oauth.app.register", Max: 5, Window: time.Hour}
)

// Names of the rate-limit policies in the RateLimit and RateLimit-Policy fields.
const (
	policyIP      = "ip"
	policyAccount = "account"
	policyClient  = "client"
)

// limit takes one request of key from lim, the policy called name, and sets the
// RateLimit-Policy and RateLimit fields (draft-ietf-httpapi-ratelimit-headers): the policy's
// quota q and window w in seconds, and the remaining quota r and the seconds t until the
// window ends (or, over the limit, until a request fits again). Over the limit it answers 429 with
// Retry-After and returns false. Without a RateLimiter, or when the store fails, it lets the
// request through: the limit protects the server and must not take the API down with it.
func (s *Server) limit(w http.ResponseWriter, r *http.Request, name string, lim kernel.Limit, key string) bool {
	if s.o.RateLimiter == nil {
		return true
	}
	d, err := s.o.RateLimiter.Take(r.Context(), lim, key)
	if err != nil {
		s.o.Logger.WarnContext(r.Context(), "client api rate limit unavailable", slog.String("limit", lim.Name), slog.Any("error", err))
		return true
	}
	h := w.Header()
	h.Set("RateLimit-Policy", fmt.Sprintf("%q;q=%d;w=%d", name, lim.Max, seconds(lim.Window)))
	if d.Allowed {
		h.Set("RateLimit", fmt.Sprintf("%q;r=%d;t=%d", name, d.Remaining, seconds(d.Reset)))
		return true
	}
	retry := seconds(d.RetryAfter)
	h.Set("RateLimit", fmt.Sprintf("%q;r=0;t=%d", name, retry))
	h.Set("Retry-After", strconv.Itoa(retry))
	l := s.localizer(r)
	s.writeProblem(w, r, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(retry))
	return false
}

// seconds rounds d up to whole seconds, at least 1.
func seconds(d time.Duration) int {
	return max(int((d+time.Second-1)/time.Second), 1)
}

// clientKey returns the rate-limit key of the request's client IP.
func (s *Server) clientKey(r *http.Request) string {
	return kernel.ClientKey(kernel.RequestClientIP(r))
}

// LimitProblem takes one event of key from an operation's own limit, such as the posting or
// upload limits of ADR-0019 §5, for handlers to call after the API-wide limits passed. Over
// the limit it returns limited and the 429 problem with its Retry-After, which the handler
// returns as its default response:
//
//	if p, h, limited := s.LimitProblem(ctx, PostLimit, v.AccountID); limited {
//		return CreateStatusdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
//	}
//
// Like the API-wide limits, it lets the event through without a RateLimiter or when the store
// fails.
func (s *Server) LimitProblem(ctx context.Context, lim kernel.Limit, key string) (Problem, ProblemResponseHeaders, bool) {
	if s.o.RateLimiter == nil {
		return Problem{}, ProblemResponseHeaders{}, false
	}
	d, err := s.o.RateLimiter.Take(ctx, lim, key)
	if err != nil {
		s.o.Logger.WarnContext(ctx, "client api rate limit unavailable", slog.String("limit", lim.Name), slog.Any("error", err))
		return Problem{}, ProblemResponseHeaders{}, false
	}
	if d.Allowed {
		return Problem{}, ProblemResponseHeaders{}, false
	}
	retry := seconds(d.RetryAfter)
	l := s.Localizer(ctx)
	p, h := s.NewProblem(ctx, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(retry))
	h.RetryAfter = &retry
	return p, h, true
}
