// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"net/http"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"

	"github.com/mycroblog/my/internal/domain/content"
)

// Pages returns the handler of the GET paths that no route serves: the web client's pages.
// Every answer depends on Accept, since the same URL may be an ActivityPub document. A request
// that asks for HTML gets the page (Options.HTML). One that asks for ActivityPub gets 404, except
// at a post page, which redirects to the post's id (ADR-0018).
func (h *Handler) Pages() http.Handler {
	return http.HandlerFunc(h.page)
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	if !slices.Contains(w.Header().Values("Vary"), "Accept") {
		w.Header().Add("Vary", "Accept")
	}
	if !AsksForActivityPub(r) {
		h.html(w, r)
		return
	}
	if id, ok := statusPageID(r.URL.Path); ok {
		h.statusPage(w, r, id)
		return
	}
	h.notFound(w, r)
}

// statusPage redirects a request of a post page that asks for ActivityPub to the post's id with
// 302, so that a server resolving the page's URL (FR-305) gets the Note. The page is anonymous:
// a post that an anonymous visitor may not see gets the same 404 as a missing one, as its HTML
// page does (ADR-0017). The username in the path does not matter; the id is the post's.
func (h *Handler) statusPage(w http.ResponseWriter, r *http.Request, id string) {
	if !kernel.ValidID(id) {
		h.notFound(w, r)
		return
	}
	uri, err := h.o.Objects.PageStatusURI(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", cachePublic)
	http.Redirect(w, r, uri, http.StatusFound) //nolint:gosec // G710: the stored id of a local status, under this server's host; the request only picks which

}

// statusPageID returns the status ID of a post page path, /@{name}/status/{id}.
func statusPageID(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, "/@")
	if !ok {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != content.StatusPageSegment {
		return "", false
	}
	return parts[2], true
}
