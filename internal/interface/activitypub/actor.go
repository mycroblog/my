// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// instanceActorUsername is the preferredUsername and WebFinger username of the instance actor:
// the account domain, as Mastodon names its instance actor. Local usernames cannot contain
// dots, so it never collides with a user unless the domain has no dot.
func (h *Handler) instanceActorUsername() string { return h.o.AccountDomain }

// actorDocument builds the document of a visible local actor.
func (h *Handler) actorDocument(ctx context.Context, a appaccount.Actor) (federation.ActorDocument, error) {
	images, err := appfederation.LocalActorImages(ctx, h.o.Images, a)
	if err != nil {
		return federation.ActorDocument{}, err
	}
	if images.Emojis, err = appfederation.LocalActorEmojis(ctx, h.o.EmojiTags, h.o.Host, a); err != nil {
		return federation.ActorDocument{}, err
	}
	if images.MovedTo, err = appfederation.LocalActorMovedTo(ctx, h.o.Moves, a); err != nil {
		return federation.ActorDocument{}, err
	}
	return appfederation.LocalActorDocument(h.o.Host, a, h.instanceActorUsername(), images), nil
}

// keyDocument is the minimal actor document that authorized fetch serves to a request without
// a verified signature: what a server needs to verify the actor's signatures and to address it,
// with the properties Mastodon keeps in the same case (id, type, preferredUsername, inbox,
// publicKey, endpoints), and the movedTo of an actor that moved, which Misskey reads when the
// actor's Move arrives (ADR-0089) and which a federation mode that sends the Move must serve.
type keyDocument struct {
	Context           []any                         `json:"@context"`
	ID                federation.IRI                `json:"id"`
	Type              federation.ActorType          `json:"type"`
	PreferredUsername string                        `json:"preferredUsername"`
	Inbox             federation.IRI                `json:"inbox"`
	PublicKey         *federation.PublicKeyDocument `json:"publicKey,omitempty"`
	Endpoints         map[string]federation.IRI     `json:"endpoints"`
	MovedTo           federation.IRI                `json:"movedTo,omitempty"`
}

// keyDocumentOf returns the minimal document of a full actor document.
func keyDocumentOf(doc federation.ActorDocument) keyDocument {
	return keyDocument{
		Context: doc.Context, ID: doc.ID, Type: doc.Type, PreferredUsername: doc.PreferredUsername,
		Inbox: doc.Inbox, PublicKey: doc.PublicKey, Endpoints: doc.Endpoints, MovedTo: doc.MovedTo,
	}
}

// actor serves GET /ap/actors/{id}: the actor document, or for a request that prefers HTML
// a 303 redirect to the profile page (ADR-0018). Under authorized fetch (FR-408), a request
// without a verified signature gets the minimal document that verifying the actor's signatures
// needs, so that any server can still fetch the key; the instance actor's document is minimal
// already and is served whole to everyone, so that servers that also use authorized fetch can
// fetch each other's instance actors to verify the instance actors' signed fetches.
//
// A local actor that moderation suspends (suspend, ban) is served, not hidden, until it is
// deleted: its document is emptied and marked suspended, with its key, so that other servers
// show the suspension and can verify the actor's Delete (federation.md §11, ADR-0036).
//
// The federation mode decides the rest (ADR-0076): Register serves the actors only when it
// identifies (404 when it is off), and when it does not publish (inbound) a user's document is
// the minimal one for everyone, as authorized fetch serves it to an unsigned request, so that
// remote servers verify the signatures of its Follows without seeing its profile.
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Accept")
	a, suspended, err := h.lookupActor(r, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	doc, err := h.actorDocument(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if suspended {
		doc = doc.Suspend()
	}
	if doc.URL != "" && parseAccept(r.Header.Get("Accept")).prefersHTML() {
		redirect(w, r, doc.URL, http.StatusSeeOther)
		return
	}
	if a.Account.ActorType == account.ActorApplication {
		h.writeJSON(w, r, typeActivityJSON, doc)
		return
	}
	mode, ok := h.mode(w, r)
	if !ok {
		return
	}
	if !federation.Allows(mode, federation.FlowPublish) {
		h.writeJSON(w, r, typeActivityJSON, keyDocumentOf(doc))
		return
	}
	acc, ok := h.authorize(w, r, false)
	if !ok {
		return
	}
	if acc.refused() {
		h.writeJSONCache(w, r, typeActivityJSON, acc.cache(false), keyDocumentOf(doc))
		return
	}
	h.writeJSONCache(w, r, typeActivityJSON, acc.cache(true), doc)
}

// lookupID finds the visible local actor with an id from a URI.
func (h *Handler) lookupID(r *http.Request, id string) (appaccount.Actor, error) {
	if !kernel.ValidID(id) {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return findVisible(h.o.Accounts.FindByID(r.Context(), id))
}

// lookupActor finds the local actor with an id from a URI that GET of the actor presents: a
// visible one, or a suspended one that is not deleted, whatever its lifecycle (a ban schedules
// its deletion). suspended tells the latter.
func (h *Handler) lookupActor(r *http.Request, id string) (a appaccount.Actor, suspended bool, err error) {
	if !kernel.ValidID(id) {
		return appaccount.Actor{}, false, appaccount.ErrNotFound
	}
	a, err = h.o.Accounts.FindByID(r.Context(), id)
	if err != nil {
		return appaccount.Actor{}, false, err
	}
	if h.o.Suspensions != nil && a.Account.IsLocal() && !a.IsInstanceActor() && !a.Deleted() {
		if suspended, err = h.o.Suspensions.AccountFederationSuspended(r.Context(), a.Account.ID); err != nil {
			return appaccount.Actor{}, false, err
		}
	}
	if !suspended && !visible(a) {
		return appaccount.Actor{}, false, appaccount.ErrNotFound
	}
	return a, suspended, nil
}

// lookupUsername finds the visible local actor with a username: a user, or the instance actor
// under instanceActorUsername.
func (h *Handler) lookupUsername(r *http.Request, username string) (appaccount.Actor, error) {
	a, err := h.o.Accounts.FindByHandle(r.Context(), username, account.LocalDomain)
	if errors.Is(err, appaccount.ErrNotFound) && strings.EqualFold(username, h.instanceActorUsername()) {
		a, err = h.o.Accounts.InstanceActor(r.Context())
	}
	return findVisible(a, err)
}

// profilePage serves GET /@{username}. An ActivityPub request is redirected to the actor id
// with 302 (ADR-0018), so that pasting a profile URL into a remote search finds the actor;
// any other request gets the HTML of the web client. Single-segment paths without "@" are
// the web client's.
func (h *Handler) profilePage(w http.ResponseWriter, r *http.Request) {
	if _, ok := cutAt(r.PathValue("name")); !ok {
		h.page(w, r)
		return
	}
	w.Header().Add("Vary", "Accept")
	if !AsksForActivityPub(r) {
		h.html(w, r)
		return
	}
	h.gate(federation.FlowIdentify, h.profileRedirect)(w, r)
}

// profileRedirect redirects an ActivityPub request for the profile page /@{username} to the
// actor id.
func (h *Handler) profileRedirect(w http.ResponseWriter, r *http.Request) {
	username, _ := cutAt(r.PathValue("name"))
	if account.ValidateUsername(username) != nil {
		h.notFound(w, r)
		return
	}
	a, err := h.lookupUsername(r, username)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if a.Local == nil { // the instance actor has no profile page
		h.notFound(w, r)
		return
	}
	redirect(w, r, a.Account.URI, http.StatusFound)
}

// html serves the page of the web client, or 404 without one.
func (h *Handler) html(w http.ResponseWriter, r *http.Request) {
	if h.o.HTML == nil {
		h.notFound(w, r)
		return
	}
	h.o.HTML.ServeHTTP(w, r)
}

// cutAt returns the username of a profile page path segment "@username".
func cutAt(segment string) (string, bool) {
	s, ok := strings.CutPrefix(segment, "@")
	return s, ok && s != ""
}
