// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"log/slog"
	"net/http"
	"net/url"

	appfederation "github.com/mycroblog/my/internal/app/federation"
)

// access is who asks for an ActivityPub document, and whether authorized fetch (FR-408) is on.
type access struct {
	requester appfederation.Requester
	// authorizedFetch makes an anonymous requester refused and every answer that depends on
	// the signature private (ADR-0018).
	authorizedFetch bool
}

// refused reports whether the request must be answered with 401: authorized fetch is on and
// the request carries no signature that verifies.
func (a access) refused() bool { return a.authorizedFetch && a.requester.Anonymous() }

// cache returns the Cache-Control of an answer that anyone would get alike when public holds.
// Under authorized fetch every answer depends on the signature, so none is public.
func (a access) cache(public bool) string {
	if public && !a.authorizedFetch {
		return cachePublic
	}
	return cachePrivate
}

// authorize decides the access of an ActivityPub GET; ok is false after it wrote a 500. The
// signature is verified only when its answer depends on it: always for objects
// (requesterMatters), and for actors and collections only under authorized fetch, since
// without it they are the same for everyone.
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, requesterMatters bool) (access, bool) {
	ctx := r.Context()
	af, err := h.o.Objects.AuthorizedFetch(ctx)
	if err != nil {
		h.o.Logger.ErrorContext(ctx, "activitypub authorized fetch setting failed", slog.Any("error", err))
		h.report(ctx, err)
		h.o.Problem(w, r, http.StatusInternalServerError)
		return access{}, false
	}
	a := access{authorizedFetch: af}
	if !af && !requesterMatters {
		return a, true
	}
	origin := &url.URL{Scheme: "https", Host: h.o.Host}
	if a.requester, err = h.o.Objects.Authenticate(ctx, appfederation.NewSignedRequest(origin, r, nil)); err != nil {
		h.o.Logger.ErrorContext(ctx, "activitypub authenticate failed", slog.Any("error", err))
		h.report(ctx, err)
		h.o.Problem(w, r, http.StatusInternalServerError)
		return access{}, false
	}
	return a, true
}

// unauthorized answers a request that authorized fetch refuses. Accept-Signature names the
// RFC 9421 signature that would be accepted, which a double-knocking client follows.
func (h *Handler) unauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Accept-Signature", appfederation.AcceptSignatureGET)
	h.o.Problem(w, r, http.StatusUnauthorized)
}
