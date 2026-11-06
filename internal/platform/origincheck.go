// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
)

// OriginRefused counts the requests that the origin check refused (origin_secret): a CDN that
// lost its rule shows as this counter rising while the other requests stop.
var OriginRefused = kernel.Instrument{
	Name: "my.http.origin_refused", Unit: "{request}", Description: "Requests refused because they did not come through the CDN.",
}

// internalPathPrefix is the prefix of the endpoints that the platform's queue and scheduler call
// on the server's own URL, past any CDN. They authenticate by themselves (queuepush).
const internalPathPrefix = "/_internal/"

// originRefusedKey marks the context of a request that the origin check refused.
type originRefusedKey struct{}

func originRefused(ctx context.Context) bool {
	refused, _ := ctx.Value(originRefusedKey{}).(bool)
	return refused
}

// originCheck refuses the requests that did not come through the CDN, before every other
// middleware, so that a caller who is not the CDN but connects from one of its addresses (the
// CDN's serverless platform, say) chooses nothing that trusted_proxies would believe.
//
// A request passes when it carries origin_secret_header exactly once with the value of
// origin_secret or origin_secret_previous, or when it is one that never comes through the CDN
// (originExempt). The header is removed in both cases, so that no handler, log, trace, or
// forwarded request sees it. A request that does not carry the secret proved nothing, so it also
// loses the headers that the server believes of a trusted peer: its client IP is the peer, its
// trace starts anew, and its request ID is the server's.
//
// A refused request costs what a 404 costs in the access log, with no route like any request
// refused before routing, and adds to OriginRefused. It starts no span and reads no database. Its
// answer is the plain 403 problem, the same for a wrong and a missing value, and it names nothing.
//
// With no origin_secret, next is returned as it is.
func (a *App) originCheck(next http.Handler) http.Handler {
	if a.Config.OriginSecret == "" {
		return next
	}
	name := http.CanonicalHeaderKey(a.Config.OriginSecretHeader)
	accepted := [][sha256.Size]byte{sha256.Sum256([]byte(a.Config.OriginSecret))}
	if a.Config.OriginSecretPrevious != "" {
		accepted = append(accepted, sha256.Sum256([]byte(a.Config.OriginSecretPrevious)))
	}
	refused := a.requestIDs(a.accessLog(securityHeaders(http.HandlerFunc(a.refuseOrigin), false)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proved := originProved(r.Header[name], accepted)
		r.Header.Del(name)
		if proved {
			next.ServeHTTP(w, r)
			return
		}
		for _, h := range [...]string{headerXForwardedFor, headerForwarded, headerTraceParent, HeaderRequestID} {
			r.Header.Del(h)
		}
		if originExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		refused.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), originRefusedKey{}, true)))
	})
}

// originProved compares the values of the header with every accepted value, in constant time:
// comparing the hashes keeps the length to itself too. More than one value is no proof, since a
// CDN sets the header and a client may have added another.
func originProved(values []string, accepted [][sha256.Size]byte) bool {
	if len(values) != 1 {
		return false
	}
	sum := sha256.Sum256([]byte(values[0]))
	match := 0
	for i := range accepted {
		match |= subtle.ConstantTimeCompare(sum[:], accepted[i][:])
	}
	return match == 1
}

// originExempt tells whether r stays reachable without the secret: a health check, for the
// platform's probes, or a call of the platform's queue or scheduler. Each is exempt by its method
// too, so that nothing else behind these paths answers: the web client answers the GET requests
// that no route serves, /_internal/ ones included. A path that the router would rewrite is not
// exempt, so that /_internal/../ leads nowhere.
func originExempt(r *http.Request) bool {
	p := r.URL.Path
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return isHealthCheck(p)
	case http.MethodPost:
		return strings.HasPrefix(p, internalPathPrefix) && path.Clean(p) == p
	}
	return false
}

// refuseOrigin answers a refused request. Its locale comes from Accept-Language alone: the
// default locale of the instance is in the database, which a refused request must not reach.
func (a *App) refuseOrigin(w http.ResponseWriter, r *http.Request) {
	if a.originRefused != nil {
		a.originRefused.Add(r.Context(), 1, kernel.Labels{})
	}
	l := (&i18n.Resolver{Bundle: a.Messages}).RequestLocalizer(r)
	a.writeProblem(w, r, l, http.StatusForbidden, l.ServerErrorsForbidden())
}

// logOriginCheck says at the start that the process refuses what did not come through the CDN,
// since a wrong setup refuses everyone while the health checks stay fine.
func (a *App) logOriginCheck(ctx context.Context) {
	if a.Config.OriginSecret == "" {
		return
	}
	a.Logger.InfoContext(ctx, "the origin check is on: requests without the CDN's secret header get 403, except the health checks and the calls of the platform's queue",
		slog.Bool("second_value", a.Config.OriginSecretPrevious != ""))
}
