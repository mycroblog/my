// SPDX-License-Identifier: AGPL-3.0-only

// Package maintenance answers every request of the maintenance mode (ADR-0084): a 503 that says
// the server is under maintenance, in the form each surface understands, without touching any
// adapter. It also renders the maintenance page that `my maintenance export` writes out, and in
// the same template the static closure page and the front ends' configuration that
// `my admin closure export-html` writes out (ADR-0088 §6).
package maintenance

import (
	"encoding/json"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/i18n"
)

// ProblemType is the type of the client API's maintenance problem.
const ProblemType = "/api/v1/problems/maintenance"

// MaxRetryAfter bounds the Retry-After counted to Page.Until (ADR-0084 §3).
const MaxRetryAfter = 24 * time.Hour

// Paths that the maintenance mode answers by path (ADR-0084 §3). They repeat the paths of the
// packages of the normal server, which this package does not import.
const (
	PathLiveness       = "/healthz"
	PathReadiness      = "/readyz"
	PathInstanceHealth = "/api/v1/instance/health"
)

// oauthJSON are the OAuth endpoints that answer JSON; /oauth/authorize is a page.
var oauthJSON = map[string]bool{
	"/oauth/token": true, "/oauth/token/refresh": true, "/oauth/revoke": true, "/oauth/register": true,
	"/.well-known/oauth-authorization-server": true, "/.well-known/oauth-protected-resource": true,
}

// shortJSON are the paths of the machines that read no page: federation discovery and NodeInfo.
var shortJSON = map[string]bool{
	"/.well-known/webfinger": true, "/.well-known/host-meta": true, "/.well-known/host-meta.json": true,
	"/.well-known/nodeinfo": true, "/inbox": true,
}

// shortJSONPrefixes are the path prefixes of ActivityPub, NodeInfo, and the notifications of
// other services (the payment webhooks, the platform queues and schedulers that call
// /_internal/jobs and /_internal/tick), whose senders retry.
var shortJSONPrefixes = []string{"/ap/", "/nodeinfo/", "/payments/webhooks/", "/_internal/"}

// Options configure a Handler.
type Options struct {
	Page Page
	// RetryAfter is the Retry-After when Page.Until is zero or past (maintenance_retry_after).
	RetryAfter time.Duration
	// Locales picks the locale from Accept-Language, falling back to en: the instance's default
	// locale is in the database.
	Locales *i18n.Resolver
	// Now reads the clock; nil means time.Now.
	Now    func() time.Time
	Logger *slog.Logger
}

// Handler serves the maintenance mode.
type Handler struct{ o Options }

// New returns a Handler on o.
func New(o Options) *Handler {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Handler{o: o}
}

// ServeHTTP answers by the table of ADR-0084 §3, routing only as far as that table needs.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Retry-After", strconv.Itoa(h.retryAfter()))
	hd.Set("Cache-Control", "no-store")
	path := r.URL.Path
	switch {
	case (path == PathLiveness || path == PathReadiness) && safe(r.Method):
		writeJSON(w, http.StatusOK, `{"status":"maintenance"}`)
	case path == PathInstanceHealth:
		writeJSON(w, http.StatusServiceUnavailable, `{"status":"maintenance"}`)
	case path == "/api" || strings.HasPrefix(path, "/api/") || oauthJSON[path]:
		h.problem(w, r)
	case h.federation(r):
		writeJSON(w, http.StatusServiceUnavailable, `{"error":"maintenance"}`)
	case wantsJSON(r.Header.Get("Accept")):
		h.problem(w, r)
	default:
		h.page(w, r)
	}
}

// retryAfter is the seconds to Page.Until while it is ahead, at most MaxRetryAfter, and
// RetryAfter otherwise.
func (h *Handler) retryAfter() int {
	if u := h.o.Page.Until; !u.IsZero() {
		if d := u.Sub(h.o.Now()); d > 0 {
			return int(math.Ceil(min(d, MaxRetryAfter).Seconds()))
		}
	}
	return max(int(h.o.RetryAfter.Seconds()), 1)
}

// federation reports whether r comes from another server: ActivityPub by its path, Accept, or
// Content-Type, discovery, NodeInfo, and the notifications of other services.
func (h *Handler) federation(r *http.Request) bool {
	path := r.URL.Path
	if shortJSON[path] || strings.HasSuffix(path, "/inbox") {
		return true
	}
	for _, p := range shortJSONPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return activityPub(r.Header.Get("Content-Type")) || wantsActivityPub(r.Header.Get("Accept"))
}

func (h *Handler) problem(w http.ResponseWriter, r *http.Request) {
	l := h.o.Locales.RequestLocalizer(r)
	body := map[string]any{
		"type": ProblemType, "title": l.ServerMaintenanceTitle(), "status": http.StatusServiceUnavailable,
	}
	if m := h.o.Page.Message; m != "" {
		body["message"] = m
	}
	if u := h.o.Page.Until; !u.IsZero() {
		body["until"] = u.UTC().Format(time.RFC3339)
	}
	if s := h.o.Page.StatusPageURL; s != "" {
		body["status_page_url"] = s
	}
	b, _ := json.Marshal(body)
	w.Header().Set("Content-Language", l.Locale())
	w.Header().Add("Vary", "Accept, Accept-Language")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write(append(b, '\n'))
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	l := h.o.Locales.RequestLocalizer(r)
	html, csp, err := Render(h.o.Page, []*i18n.Localizer{l}, RenderOptions{})
	if err != nil {
		h.o.Logger.ErrorContext(r.Context(), "render maintenance page", slog.Any("error", err))
		writeJSON(w, http.StatusServiceUnavailable, `{"error":"maintenance"}`)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Language", l.Locale())
	hd.Add("Vary", "Accept, Accept-Language")
	hd.Set("Content-Security-Policy", csp)
	w.WriteHeader(http.StatusServiceUnavailable)
	if r.Method != http.MethodHead {
		_, _ = w.Write(html) //nolint:gosec // G705: html/template escaped every value; the locale is one of the bundle's
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}

func safe(method string) bool { return method == http.MethodGet || method == http.MethodHead }

// activityPub reports whether a Content-Type is ActivityStreams JSON.
func activityPub(contentType string) bool {
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/activity+json" ||
		mt == "application/ld+json" && strings.Contains(params["profile"], "https://www.w3.org/ns/activitystreams")
}

// wantsActivityPub reports whether an Accept header names ActivityStreams JSON.
func wantsActivityPub(accept string) bool {
	for part := range strings.SplitSeq(accept, ",") {
		if activityPub(strings.TrimSpace(part)) {
			return true
		}
	}
	return false
}

// wantsJSON reports whether an Accept header names JSON and not HTML, as API clients send.
func wantsJSON(accept string) bool {
	jsonQ, htmlQ := -1.0, -1.0
	for part := range strings.SplitSeq(accept, ",") {
		mt, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		q := 1.0
		if v, err := strconv.ParseFloat(params["q"], 64); err == nil {
			q = v
		}
		switch {
		case mt == "application/json" || mt == "application/problem+json" || strings.HasSuffix(mt, "+json"):
			jsonQ = max(jsonQ, q)
		case mt == "text/html" || mt == "application/xhtml+xml":
			htmlQ = max(htmlQ, q)
		}
	}
	return jsonQ > 0 && jsonQ > htmlQ
}
