// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/mycroblog/my/internal/app/kernel"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

const contextActivityStreams = "https://www.w3.org/ns/activitystreams"

// followPageSize is the number of actors on one page of the followers and following collections.
const followPageSize = 40

// Query parameters of collection pages, as Mastodon names them: page selects a page instead of
// the collection, max_id is the cursor of the page after the first.
const (
	paramPage  = "page"
	paramMaxID = "max_id"
)

// orderedCollection is an ActivityStreams collection or collection page. The outbox, followers,
// and following collections hold the total and link to their first page, whose pages link to
// the next one; the featured and featured-tags collections hold their few items inline.
type orderedCollection struct {
	Context      any    `json:"@context"`
	ID           string `json:"id"`
	Type         string `json:"type"`
	TotalItems   int64  `json:"totalItems"`
	First        string `json:"first,omitempty"`
	Next         string `json:"next,omitempty"`
	PartOf       string `json:"partOf,omitempty"`
	OrderedItems *[]any `json:"orderedItems,omitempty"`
	Items        *[]any `json:"items,omitempty"`
}

// collection serves GET /ap/actors/{id}/{collection} for the collections an actor document
// names, except the inbox. Every collection lists only what anyone may see, so the answer is
// the same for every requester; under authorized fetch (FR-408) a request without a verified
// signature gets 401 instead.
func (h *Handler) collection(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("collection")
	switch name {
	case collOutbox, collFollowers, collFollowing, collFeatured, collFeaturedTag:
	default:
		h.notFound(w, r)
		return
	}
	acc, ok := h.authorize(w, r, false)
	if !ok {
		return
	}
	if acc.refused() {
		h.unauthorized(w, r)
		return
	}
	a, err := h.lookupID(r, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	isInstanceActor := a.Local == nil
	if isInstanceActor && (name == collFeatured || name == collFeaturedTag) {
		h.notFound(w, r) // its document does not name them
		return
	}
	id := a.Account.URI + "/" + name
	q := r.URL.Query()
	before := q.Get(paramMaxID)
	if before != "" && !kernel.ValidID(before) {
		h.notFound(w, r)
		return
	}
	paged := q.Get(paramPage) != ""

	var c orderedCollection
	switch name {
	case collOutbox:
		c, err = h.outbox(r, a, id, paged, before)
	case collFollowers, collFollowing:
		c, err = h.follows(r, a, name, id, paged, before)
	case collFeatured:
		c, err = h.featured(r, a, id)
	case collFeaturedTag:
		// Mastodon publishes featured tags as an unordered Collection of Hashtag items.
		c = orderedCollection{ID: id, Type: "Collection", Items: &[]any{}}
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	c.Context = contextActivityStreams
	// Embedded documents have no context of their own, so a collection that embeds Notes or
	// activities defines their terms. Followers and following hold ids alone.
	if name == collFeatured || (name == collOutbox && paged) {
		c.Context = federation.LDContext()
	}
	h.writeJSONCache(w, r, typeActivityJSON, acc.cache(true), c)
}

// pageURI returns the IRI of the page of collection id that starts after the cursor before (""
// for the first page).
func pageURI(id, before string) string {
	q := url.Values{paramPage: {"true"}}
	if before != "" {
		q.Set(paramMaxID, before)
	}
	return id + "?" + q.Encode()
}

// page returns the collection id, or with paged its page after before: the items and the
// cursor of the next page ("" on the last one).
func page(id string, total int64, paged bool, before string, items []any, next string) orderedCollection {
	if !paged {
		return orderedCollection{ID: id, Type: "OrderedCollection", TotalItems: total, First: pageURI(id, "")}
	}
	c := orderedCollection{
		ID: pageURI(id, before), Type: "OrderedCollectionPage", TotalItems: total, PartOf: id, OrderedItems: &items,
	}
	if next != "" {
		c.Next = pageURI(id, next)
	}
	return c
}

// outbox returns the outbox of a: the Create activities of its public and unlisted statuses.
func (h *Handler) outbox(r *http.Request, a appaccount.Actor, id string, paged bool, before string) (orderedCollection, error) {
	total, err := h.o.Objects.OutboxTotal(r.Context(), a.Account.ID)
	if err != nil || !paged {
		return page(id, total, false, "", nil, ""), err
	}
	p, err := h.o.Objects.Outbox(r.Context(), a.Account.ID, before)
	if err != nil {
		return orderedCollection{}, err
	}
	items := make([]any, 0, len(p.Items))
	for _, it := range p.Items {
		items = append(items, it)
	}
	return page(id, total, true, before, items, p.Next), nil
}

// follows returns the followers or following collection of a: the ids of the active accounts
// that it actively follows or that actively follow it, newest follow first.
func (h *Handler) follows(r *http.Request, a appaccount.Actor, name, id string, paged bool, before string) (orderedCollection, error) {
	ctx := r.Context()
	counters, err := h.o.Graph.Counters(ctx, a.Account.ID)
	if err != nil {
		return orderedCollection{}, err
	}
	total := counters.FollowingCount
	if name == collFollowers {
		total = counters.FollowersCount
	}
	if !paged {
		return page(id, total, false, "", nil, ""), nil
	}
	list, other := h.o.Graph.Following, func(e graph.Edge) string { return e.FolloweeAccountID }
	if name == collFollowers {
		list, other = h.o.Graph.Followers, func(e graph.Edge) string { return e.FollowerAccountID }
	}
	edges, err := list(ctx, a.Account.ID, appgraph.Page{Before: before, Limit: followPageSize})
	if err != nil {
		return orderedCollection{}, err
	}
	ids := make([]string, len(edges))
	for i, e := range edges {
		ids[i] = other(e)
	}
	actors, err := h.findAccounts(ctx, ids)
	if err != nil {
		return orderedCollection{}, err
	}
	items := []any{}
	for _, id := range ids {
		if actor, ok := actors[id]; ok && actor.Lifecycle.Active() {
			items = append(items, actor.Account.URI)
		}
	}
	next := ""
	if len(edges) == followPageSize {
		next = edges[len(edges)-1].FollowID
	}
	return page(id, total, true, before, items, next), nil
}

// featured returns the featured collection of a: its pinned public and unlisted statuses.
// Mastodon reads pinned posts from orderedItems without paging.
func (h *Handler) featured(r *http.Request, a appaccount.Actor, id string) (orderedCollection, error) {
	notes, err := h.o.Objects.Featured(r.Context(), a.Account.ID)
	if err != nil {
		return orderedCollection{}, err
	}
	items := make([]any, 0, len(notes))
	for _, n := range notes {
		items = append(items, n)
	}
	return orderedCollection{ID: id, Type: "OrderedCollection", TotalItems: int64(len(items)), OrderedItems: &items}, nil
}

// accountBatch is an Accounts that reads many accounts at once, as *appaccount.Service does.
type accountBatch interface {
	FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error)
}

// findAccounts returns the accounts of ids that exist, by ID: together when Accounts can, one by
// one otherwise.
func (h *Handler) findAccounts(ctx context.Context, ids []string) (map[string]appaccount.Actor, error) {
	if b, ok := h.o.Accounts.(accountBatch); ok {
		return b.FindByIDs(ctx, ids)
	}
	out := make(map[string]appaccount.Actor, len(ids))
	for _, id := range ids {
		a, err := h.o.Accounts.FindByID(ctx, id)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
		case err != nil:
			return nil, err
		default:
			out[id] = a
		}
	}
	return out, nil
}
