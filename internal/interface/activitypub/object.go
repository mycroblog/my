// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"net/http"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
)

// object serves GET /ap/objects/{id} (the Note of a local status) and GET
// /ap/objects/{id}/activity (its Create). The request's HTTP signature decides the requester,
// and the federation module's visibility decision answers: a status the requester may not see
// gets the same 404 as a missing one, and a deleted one that it could see gets 410 with a
// Tombstone (ADR-0018). A request that prefers HTML is redirected to the status page with 303
// when an anonymous visitor may see the status.
func (h *Handler) object(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Accept")
	id := r.PathValue("id")
	if !kernel.ValidID(id) {
		h.notFound(w, r)
		return
	}
	activity := strings.HasSuffix(r.URL.Path, "/"+activitySegment)
	html := parseAccept(r.Header.Get("Accept")).prefersHTML()

	// A browser carries no signature: it is anonymous, and authorized fetch does not apply to
	// the redirect to the status page.
	var acc access
	if !html {
		var ok bool
		if acc, ok = h.authorize(w, r, true); !ok {
			return
		}
		if acc.refused() {
			h.unauthorized(w, r)
			return
		}
	}
	get := h.o.Objects.Status
	if activity {
		get = h.o.Objects.StatusActivity
	}
	obj, err := get(r.Context(), acc.requester, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if html {
		if obj.Gone || obj.URL == "" {
			h.notFound(w, r)
			return
		}
		redirect(w, r, obj.URL, http.StatusSeeOther)
		return
	}
	status := http.StatusOK
	if obj.Gone {
		status = http.StatusGone
	}
	writeBody(w, typeActivityJSON, acc.cache(obj.Public), status, obj.Document)
}

// quoteAuthorization serves GET /ap/quote_authorizations/{id}, the approval of a quote of a local
// status (FEP-044f, ADR-0069 §1): to anyone, as the quoting post shows it; a revoked one gets 410
// with a Tombstone. Authorized fetch still asks for a signature.
func (h *Handler) quoteAuthorization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !kernel.ValidID(id) {
		h.notFound(w, r)
		return
	}
	acc, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	if acc.refused() {
		h.unauthorized(w, r)
		return
	}
	obj, err := h.o.Objects.QuoteAuthorization(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if obj.Gone {
		status = http.StatusGone
	}
	writeBody(w, typeActivityJSON, acc.cache(obj.Public), status, obj.Document)
}

// activity serves GET /ap/activities/{id}: the Announce of a public or unlisted boost (ADR-0144
// §5). Every other activity, and a request that prefers HTML, gets the same 404 as a missing one.
func (h *Handler) activity(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Accept")
	id := r.PathValue("id")
	if !kernel.ValidID(id) || parseAccept(r.Header.Get("Accept")).prefersHTML() {
		h.notFound(w, r)
		return
	}
	acc, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	if acc.refused() {
		h.unauthorized(w, r)
		return
	}
	obj, err := h.o.Objects.Announce(r.Context(), acc.requester, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeBody(w, typeActivityJSON, acc.cache(obj.Public), http.StatusOK, obj.Document)
}
