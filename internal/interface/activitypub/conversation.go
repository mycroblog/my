// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"net/http"
	"net/url"

	"github.com/mycroblog/my/internal/app/kernel"
)

// paramMinID is the cursor of a page of a conversation after the first, as Mastodon names it for
// collections in the oldest-first order.
const paramMinID = "min_id"

// conversationCollection is the collection of a conversation (FEP-7888) or one of its pages. It
// has no totalItems, since what the requester may see differs, nor attributedTo (ADR-0154 §9).
type conversationCollection struct {
	Context      string    `json:"@context"`
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	First        string    `json:"first,omitempty"`
	Next         string    `json:"next,omitempty"`
	PartOf       string    `json:"partOf,omitempty"`
	OrderedItems *[]string `json:"orderedItems,omitempty"`
}

// conversation serves GET /ap/contexts/{id}: the statuses of the thread of a conversation of this
// server that the requester may see, oldest first, as an OrderedCollection whose first page is
// ?page=true and whose next pages are ?min_id={cursor}&page=true (ADR-0154 §9). The signature
// decides the requester, as for an object; the answer to an unsigned request is the same for
// everyone and is cached publicly. A merged conversation is redirected to the one it was merged
// into with 308; a missing or remote one gets 404.
func (h *Handler) conversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	q := r.URL.Query()
	after := q.Get(paramMinID)
	if !kernel.ValidID(id) || (after != "" && !kernel.ValidID(after)) {
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
	paged := q.Get(paramPage) != ""
	c, err := h.o.Objects.Conversation(r.Context(), acc.requester, id, paged, after)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if c.RedirectTo != "" {
		// The same page of the conversation it was merged into, from the parameters read above.
		target := c.RedirectTo
		if paged {
			target = conversationPageURI(target, after)
		}
		redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}
	out := conversationCollection{Context: contextActivityStreams, ID: c.URI, Type: "OrderedCollection", First: conversationPageURI(c.URI, "")}
	if paged {
		items := c.Items
		out = conversationCollection{
			Context: contextActivityStreams, ID: conversationPageURI(c.URI, after), Type: "OrderedCollectionPage",
			PartOf: c.URI, OrderedItems: &items,
		}
		if c.Next != "" {
			out.Next = conversationPageURI(c.URI, c.Next)
		}
	}
	h.writeJSONCache(w, r, typeActivityJSON, acc.cache(acc.requester.Anonymous()), out)
}

// conversationPageURI returns the IRI of the page of the conversation id that starts after the
// cursor after ("" for the first page).
func conversationPageURI(id, after string) string {
	q := url.Values{paramPage: {"true"}}
	if after != "" {
		q.Set(paramMinID, after)
	}
	return id + "?" + q.Encode()
}
