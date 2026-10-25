// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

// Accounts is what the handlers need from the account module; *account.Service implements it.
type Accounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	FindByHandle(ctx context.Context, username, domain string) (appaccount.Actor, error)
	InstanceActor(ctx context.Context) (appaccount.Actor, error)
	CountLocalUsers(ctx context.Context) (int, error)
}

// Objects is what the handlers need from the federation module to serve local statuses and the
// collections made of them; *appfederation.Objects implements it.
type Objects interface {
	AuthorizedFetch(ctx context.Context) (bool, error)
	Authenticate(ctx context.Context, r *appfederation.SignedRequest) (appfederation.Requester, error)
	Status(ctx context.Context, req appfederation.Requester, statusID string) (appfederation.Object, error)
	StatusActivity(ctx context.Context, req appfederation.Requester, statusID string) (appfederation.Object, error)
	QuoteAuthorization(ctx context.Context, id string) (appfederation.Object, error)
	PageStatusURI(ctx context.Context, statusID string) (string, error)
	OutboxTotal(ctx context.Context, accountID string) (int64, error)
	Outbox(ctx context.Context, accountID, before string) (appfederation.OutboxPage, error)
	Featured(ctx context.Context, accountID string) ([]federation.NoteDocument, error)
}

// Graph is what the followers and following collections need from the graph module;
// *appgraph.Service implements it.
type Graph interface {
	Counters(ctx context.Context, accountID string) (graph.Counters, error)
	Followers(ctx context.Context, accountID string, p appgraph.Page) ([]graph.Edge, error)
	Following(ctx context.Context, accountID string, p appgraph.Page) ([]graph.Edge, error)
}

// Suspensions tells the local accounts that moderation suspends (suspend, ban), whose actor is
// served emptied and marked suspended (federation.md §11); *moderation.FederationPolicy
// implements it.
type Suspensions interface {
	AccountFederationSuspended(ctx context.Context, accountID string) (bool, error)
}

// ProblemWriter writes the error response of an HTTP status, the same one the rest of the
// server uses for that status.
type ProblemWriter func(w http.ResponseWriter, r *http.Request, status int)

// Options configure a Handler.
type Options struct {
	// Host is the serving domain of every ActivityPub id (ADR-0018).
	Host string
	// AccountDomain is the domain of handles; empty means Host.
	AccountDomain string
	// SoftwareVersion is the server version that NodeInfo reports.
	SoftwareVersion string
	// SourceURL is where users get the server's source code, an https URL or a mailto: address (NodeInfo
	// metadata.sourceCode, and software.repository when it is an https URL).
	SourceURL string
	// InstanceName returns the instance's display name in its default locale, which NodeInfo
	// reports as metadata.nodeName; nil or "" reports Host.
	InstanceName func(ctx context.Context) string
	// Metadata returns what NodeInfo's metadata says about the instance besides its name, in its
	// default locale; nil says nothing more.
	Metadata func(ctx context.Context) (NodeMetadata, error)
	// OpenRegistrations reports whether people may sign up on their own (NodeInfo
	// openRegistrations); nil reports false.
	OpenRegistrations func(ctx context.Context) (bool, error)
	// LocalPosts returns the number of local posts (NodeInfo usage.localPosts); nil reports 0.
	LocalPosts func(ctx context.Context) (int64, error)

	Accounts Accounts
	Objects  Objects
	Graph    Graph
	// Suspensions tells the suspended local actors; nil suspends none.
	Suspensions Suspensions
	// Images reads the avatars and headers of local actors; nil serves none.
	Images appfederation.ProfileImages
	// EmojiTags reads the custom emoji of local actors' profiles and serves the local custom
	// emoji at their ids (ADR-0051 §3); nil serves none.
	EmojiTags appfederation.EmojiTagReader
	// Aliases reads the aliases of local actors (alsoKnownAs, ADR-0089); nil serves none.
	Aliases appfederation.AliasReader
	// Moves tells the actor that a local actor moved to, its movedTo (ADR-0089); nil serves none.
	Moves appfederation.MoveReader
	// Inbox receives the activities POSTed to the inboxes; nil serves no inbox.
	Inbox Inbox
	// Modes reads the federation mode (ADR-0076) per request; nil federates both ways.
	Modes appfederation.ModeSettings
	// HTML serves the web client's page for profile pages (/@alice) and other single-segment
	// paths when the request does not ask for ActivityPub; nil answers 404.
	HTML http.Handler
	// Problem writes error responses; nil writes a plain-text body.
	Problem ProblemWriter
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
}

// Handler serves the federation endpoints.
type Handler struct {
	o Options
}

// New returns a Handler on o.
func New(o Options) *Handler {
	if o.AccountDomain == "" {
		o.AccountDomain = o.Host
	}
	if o.Problem == nil {
		o.Problem = func(w http.ResponseWriter, _ *http.Request, status int) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, http.StatusText(status), status)
		}
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Handler{o: o}
}

// Router registers handlers for http.ServeMux patterns.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Paths that the handlers serve, besides the ADR-0018 URIs under /ap/ and /@.
const (
	PathWebFinger     = "/.well-known/webfinger"
	PathHostMeta      = "/.well-known/host-meta"
	PathHostMetaJSON  = "/.well-known/host-meta.json"
	PathNodeInfoIndex = "/.well-known/nodeinfo"
	PathNodeInfo      = "/nodeinfo/2.1"
)

// Register registers every endpoint on r. The federation mode (ADR-0076) gates the GETs by their
// flow: the discovery documents and the actors identify (404 when the mode is off), and the
// statuses, their activities, the collections, the custom emoji, and the approvals of quotes
// publish (404 when the mode does not send the local posts out; never 410, so that nothing tells
// what exists).
func (h *Handler) Register(r Router) {
	identify := func(next http.HandlerFunc) http.HandlerFunc { return h.gate(federation.FlowIdentify, next) }
	publish := func(next http.HandlerFunc) http.HandlerFunc { return h.gate(federation.FlowPublish, next) }
	r.Handle("GET "+PathWebFinger, h.wellKnown(identify(h.webFinger)))
	r.Handle("GET "+PathHostMeta, h.wellKnown(identify(h.hostMeta)))
	r.Handle("GET "+PathHostMetaJSON, h.wellKnown(identify(h.hostMetaJSON)))
	r.Handle("GET "+PathNodeInfoIndex, h.wellKnown(identify(h.nodeInfoIndex)))
	r.Handle("GET "+PathNodeInfo, identify(h.nodeInfo))
	r.Handle("GET "+actorsPrefix+"{id}", identify(h.actor))
	r.Handle("GET "+actorsPrefix+"{id}/{collection}", publish(h.collection))
	if h.o.Inbox != nil {
		r.Handle("POST "+sharedInboxPath, http.HandlerFunc(h.sharedInbox))
		r.Handle("POST "+actorsPrefix+"{id}/"+collInbox, http.HandlerFunc(h.actorInbox))
	}
	r.Handle("GET "+objectsPrefix+"{id}", publish(h.object))
	r.Handle("GET "+objectsPrefix+"{id}/"+activitySegment, publish(h.object))
	r.Handle("GET "+emojisPrefix+"{id}", publish(h.emoji))
	r.Handle("GET "+quoteAuthorizationsPrefix+"{id}", publish(h.quoteAuthorization))
	// A wildcard cannot share a path segment with "@", so every single-segment GET that no
	// more specific pattern serves comes here; profilePage passes those that are not profile
	// pages to the HTML handler.
	r.Handle("GET /{name}", http.HandlerFunc(h.profilePage))
}

// Response headers.
const (
	cachePublic = "public, max-age=60"
	// cachePrivate marks a document that only its requester may see (ADR-0018).
	cachePrivate = "private, no-store"

	typeActivityJSON = "application/activity+json"
	typeJRD          = "application/jrd+json; charset=utf-8"
	typeXRD          = "application/xrd+xml; charset=utf-8"
	typeJSON         = "application/json; charset=utf-8"
	typeNodeInfo     = `application/json; profile="` + nodeInfoSchema + `#"`
)

// mode returns the federation mode for the request. A failure to read it is answered with 500,
// and ok is false.
func (h *Handler) mode(w http.ResponseWriter, r *http.Request) (mode federation.Mode, ok bool) {
	if h.o.Modes == nil {
		return federation.DefaultMode, true
	}
	mode, err := h.o.Modes.FederationMode(r.Context())
	if err != nil {
		h.o.Logger.ErrorContext(r.Context(), "activitypub: read the federation mode failed", slog.Any("error", err))
		h.o.Problem(w, r, http.StatusInternalServerError)
		return "", false
	}
	return mode, true
}

// gate serves next only when the federation mode lets flow through, and otherwise the 404 of a
// missing resource (ADR-0076).
func (h *Handler) gate(flow federation.Flow, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode, ok := h.mode(w, r)
		if !ok {
			return
		}
		if !federation.Allows(mode, flow) {
			h.notFound(w, r)
			return
		}
		next(w, r)
	}
}

// notFound answers with the one 404 response that missing and hidden resources share.
func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.o.Problem(w, r, http.StatusNotFound)
}

// fail answers a lookup error: 404 for ErrNotFound, otherwise 500 after logging it.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, appaccount.ErrNotFound) || errors.Is(err, appfederation.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	h.o.Logger.ErrorContext(r.Context(), "activitypub lookup failed", slog.Any("error", err))
	h.o.Problem(w, r, http.StatusInternalServerError)
}

// visible reports whether an actor may be presented: only active local actors are.
func visible(a appaccount.Actor) bool {
	return a.Account.IsLocal() && a.Lifecycle.Active()
}

// findVisible looks up an actor and hides the ones that may not be presented as missing.
func findVisible(a appaccount.Actor, err error) (appaccount.Actor, error) {
	if err != nil {
		return appaccount.Actor{}, err
	}
	if !visible(a) {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return a, nil
}

// writeJSON writes v as a cacheable public document of the content type ct.
func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, ct string, v any) {
	h.writeJSONCache(w, r, ct, cachePublic, v)
}

// writeJSONCache writes v as a document of the content type ct with the Cache-Control
// cacheControl.
func (h *Handler) writeJSONCache(w http.ResponseWriter, r *http.Request, ct, cacheControl string, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		h.o.Logger.ErrorContext(r.Context(), "activitypub encode failed", slog.Any("error", err))
		h.o.Problem(w, r, http.StatusInternalServerError)
		return
	}
	writeBody(w, ct, cacheControl, http.StatusOK, body)
}

// writeBody writes a document with its status. Federation documents are fetched by other
// servers' backends and by browsers on other origins, so they are cross-origin resources
// (ADR-0019 §9).
func writeBody(w http.ResponseWriter, ct, cacheControl string, status int, body []byte) {
	hd := w.Header()
	hd.Set("Content-Type", ct)
	hd.Set("Cache-Control", cacheControl)
	hd.Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// redirect answers with a redirect to location that may be cached like the document it
// points at.
func redirect(w http.ResponseWriter, r *http.Request, location string, status int) {
	w.Header().Set("Cache-Control", cachePublic)
	http.Redirect(w, r, location, status)
}

// wellKnown wraps a /.well-known endpoint: every response allows any origin to read it
// (RFC 7033 §5), and requests to a separate account domain are redirected to the host
// (ADR-0018).
func (h *Handler) wellKnown(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if h.o.AccountDomain != h.o.Host && strings.EqualFold(r.Host, h.o.AccountDomain) {
			// The target is always on the configured host; only the path and query are the request's.
			target := url.URL{Scheme: "https", Host: h.o.Host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
			http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
			return
		}
		next(w, r)
	})
}
