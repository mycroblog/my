// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform/config"
)

// Health check paths (FR-802).
const (
	// PathLiveness answers 200 while the process can serve HTTP at all; it checks nothing else,
	// so that a slow database never gets the process restarted.
	PathLiveness = "/healthz"
	// PathReadiness answers 200 when the database is reachable and its schema is applied
	// (every table of schema/<dialect> exists), and 503 with a problem document otherwise.
	PathReadiness = "/readyz"
	// PathInstanceHealth is the health endpoint of the client API for uptime monitors, status
	// pages, and the clients (ADR-0085 §1): the readiness check's answer, which reaches the
	// database at most once every healthCacheTTL per process (healthCache). The client API
	// serves it on the web role; the maintenance mode (ADR-0084) answers it itself.
	PathInstanceHealth = "/api/v1/instance/health"
)

// readinessTimeout bounds one readiness check, so that a hung database fails the probe
// instead of hanging it.
const readinessTimeout = 3 * time.Second

// Handler returns the HTTP handler of the App: the routes of its role behind the middleware
// that every response passes (request ID, access log, security headers, panic recovery, the
// statements to crawlers of the responses that are not pages, the CORS of the client API, and
// the client IP of the rate limits, clientIPs). GET requests that
// no route serves get the web client's HTML (the SPA routes) unless they ask for ActivityPub. Requests whose target or form holds text
// that no database can look up are a 400 before any route (rejectInvalidText).
// The worker role serves only the health checks, for the platform's probes.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PathLiveness, a.liveness)
	mux.HandleFunc("GET "+PathReadiness, a.readiness)
	var page http.Handler
	targets := map[string]routeTarget{}
	if a.Config.Role != config.RoleWorker {
		for _, r := range a.routes {
			mux.Handle(r.pattern, r.handler)
			targets[r.pattern] = routeTarget{route: routeOf(r.pattern), surface: r.surface}
		}
		page = a.page
	}
	h := a.problemFallback(mux, page, targets)
	h = a.rejectInvalidText(h)
	h = a.clientIPs(h)
	h = clientAPICORS(a.Config.WebOrigin, h)
	h = a.crawlerHeaders(h)
	h = a.recoverPanics(h)
	h = securityHeaders(h, a.Voice.Enabled())
	h = a.accessLog(h)
	return a.requestIDs(h)
}

func (a *App) liveness(w http.ResponseWriter, _ *http.Request) {
	writeStatusOK(w)
}

// readiness checks the App on every request, one check for the concurrent requests
// (readyCheck).
func (a *App) readiness(w http.ResponseWriter, r *http.Request) {
	if err := a.readyCheck(r.Context()); err != nil {
		// The default locale lives in the database that just failed, so it is not consulted.
		l := (&i18n.Resolver{Bundle: a.Messages}).RequestLocalizer(r)
		a.writeProblem(w, r, l, http.StatusServiceUnavailable, l.ServerHealthUnavailable())
		return
	}
	writeStatusOK(w)
}

func writeStatusOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
}

// problem is an RFC 9457 problem document (ADR-0016). Problems that mean no more than their
// HTTP status use the type about:blank; the title is localized from the request's
// Accept-Language and never reveals internal details.
type problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	RequestID string `json:"request_id,omitempty"`
}

func (a *App) localizer(r *http.Request) *i18n.Localizer { return a.Locales.RequestLocalizer(r) }

// instanceDefaultLocale returns the current default_locale setting for Locales, read per
// call so that a change applies without a restart; it returns "" (meaning en) when the
// setting cannot be read.
func (a *App) instanceDefaultLocale(ctx context.Context) string {
	locale, err := a.Instance.DefaultLocale(ctx)
	if err != nil {
		a.Logger.WarnContext(ctx, "cannot read the default locale", slog.String("error", kernel.ScrubErrorMessage(err.Error())))
		return ""
	}
	return locale
}

// viewerLocale returns the saved locale of the client API request's authenticated user for
// Locales, or "" for an anonymous request or when it cannot be read.
func (a *App) viewerLocale(ctx context.Context) string {
	v, ok := clientapi.ViewerFrom(ctx)
	if !ok {
		return ""
	}
	locale, err := a.Identity.UserLocale(ctx, v.UserID)
	if err != nil {
		a.Logger.WarnContext(ctx, "cannot read the user's locale", slog.String("error", kernel.ScrubErrorMessage(err.Error())))
		return ""
	}
	return locale
}

func (a *App) writeProblem(w http.ResponseWriter, r *http.Request, l *i18n.Localizer, status int, title string) {
	body, err := json.Marshal(problem{Type: "about:blank", Title: title, Status: status, RequestID: RequestID(r.Context())})
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/problem+json")
	h.Set("Content-Language", l.Locale())
	h.Set("Cache-Control", "no-store")
	h.Add("Vary", "Accept-Language")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// writeStatusProblem writes the problem document of an HTTP status, for handlers outside this
// package (activitypub.ProblemWriter), so that their errors look like every other error of
// the server.
func (a *App) writeStatusProblem(w http.ResponseWriter, r *http.Request, status int) {
	l := a.localizer(r)
	var title string
	switch status {
	case http.StatusBadRequest:
		title = l.ServerErrorsBadRequest()
	case http.StatusUnauthorized:
		title = l.ServerErrorsSignatureRequired()
	case http.StatusForbidden:
		title = l.ServerErrorsForbidden()
	case http.StatusNotFound:
		title = l.ServerErrorsNotFound()
	case http.StatusGone:
		title = l.ServerErrorsGone()
	case http.StatusRequestEntityTooLarge:
		title = l.ServerErrorsRequestTooLarge()
	case http.StatusUnsupportedMediaType:
		title = l.ServerErrorsUnsupportedMediaType()
	case http.StatusTooManyRequests:
		title = l.ServerErrorsRateLimited()
	case http.StatusBadGateway:
		title = l.ServerErrorsBadGateway()
	case http.StatusServiceUnavailable:
		title = l.ServerErrorsServiceUnavailable()
	default:
		status, title = http.StatusInternalServerError, l.ServerErrorsInternal()
	}
	a.writeProblem(w, r, l, status, title)
}

// problemFallback serves mux, replacing its plain-text 404 and 405 responses with problem
// documents, or the 404 of a GET request with page when there is one. It passes the route of the
// request to the access log (targets, by pattern, or other on the web for the requests that no
// route serves) and, with the HTTP metrics, counts the request as active under its surface; the
// health checks have no route and are not measured.
func (a *App) problemFallback(mux *http.ServeMux, page http.Handler, targets map[string]routeTarget) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		t, ok := targets[pattern]
		if !ok {
			t = routeTarget{route: routeOther, surface: surfaceWeb}
		}
		if pattern == "" || ok {
			if rr, found := r.Context().Value(requestRouteKey{}).(*requestRoute); found {
				rr.route, rr.surface = t.route, t.surface
			}
			if a.http != nil {
				defer a.http.begin(r, t)()
			}
		}
		if pattern != "" {
			mux.ServeHTTP(w, r) // sets the request's pattern and path values
			return
		}
		rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		l := a.localizer(r)
		switch rec.status {
		case http.StatusNotFound:
			if page != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				page.ServeHTTP(w, r)
				return
			}
			a.writeProblem(w, r, l, http.StatusNotFound, l.ServerErrorsNotFound())
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", rec.header.Get("Allow"))
			a.writeProblem(w, r, l, http.StatusMethodNotAllowed, l.ServerErrorsMethodNotAllowed(r.Method))
		default: // the mux's redirects to the canonical path
			for k, v := range rec.header {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.status)
			_, _ = w.Write(rec.body.Bytes())
		}
	})
}

// maxCheckedFormBytes bounds the form bodies that rejectInvalidText reads. A longer body passes
// unchecked: every form handler refuses more than 32 KiB (authpages and oauth maxFormBytes).
const maxCheckedFormBytes = 64 << 10

// rejectInvalidText answers a request with a 400 problem when its path, a query parameter, or
// a field of its application/x-www-form-urlencoded body is not valid UTF-8 or holds a NUL,
// before any handler looks it up: PostgreSQL fails on such a parameter where SQLite finds
// nothing, so that the dialects would answer differently (data-model §1.2). The client API
// checks its JSON bodies itself; the other headers and cookies that handlers look up (bearer
// and session tokens) are hashed first.
func (a *App) rejectInvalidText(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := validText(r.URL.Path) && validValues(r.URL.Query())
		if ok && isForm(r) {
			ok = checkForm(r)
		}
		if !ok {
			a.writeStatusProblem(w, r, http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validText reports whether s is valid UTF-8 without NUL (data-model §1.2).
func validText(s string) bool { return utf8.ValidString(s) && strings.IndexByte(s, 0) < 0 }

func validValues(values url.Values) bool {
	for k, vs := range values {
		if !validText(k) {
			return false
		}
		for _, v := range vs {
			if !validText(v) {
				return false
			}
		}
	}
	return true
}

func isForm(r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/x-www-form-urlencoded"
}

// checkForm reports whether the fields of r's form body are valid text, and puts the body back
// for the handler, which parses it with its own limit. A body that cannot be read or is longer
// than maxCheckedFormBytes passes, for the handler to refuse.
func checkForm(r *http.Request) bool {
	buf, err := io.ReadAll(io.LimitReader(r.Body, maxCheckedFormBytes+1))
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(buf), r.Body), r.Body}
	if err != nil || len(buf) > maxCheckedFormBytes {
		return true
	}
	values, _ := url.ParseQuery(string(buf)) // the handler answers a malformed form
	return validValues(values)
}

// bufferedResponse captures a response that problemFallback may replace.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }
func (b *bufferedResponse) WriteHeader(status int)      { b.status = status }

// recoverPanics turns a handler panic into a 500 problem and reports it as an error at the
// request's route (ADR-0103 §2). http.ErrAbortHandler keeps its meaning (abort the response
// silently).
func (a *App) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			kernel.RecoverPanic(r.Context(), a.ErrorReporter, p, "") // the reporter names the route
			if rw, ok := w.(*responseWriter); ok && rw.wroteHeader {
				return // too late for an error response
			}
			l := a.localizer(r)
			a.writeProblem(w, r, l, http.StatusInternalServerError, l.ServerErrorsInternal())
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets the headers of ADR-0019 §9 that every response carries. A handler may
// override them, for example Cross-Origin-Resource-Policy: cross-origin for ActivityPub JSON
// and public media. The Content-Security-Policy is the policy for responses that are not
// documents; the web client's responses set their own. microphone lets the documents of this
// origin use the microphone, for the voice chat (ADR-0019 §9 relaxes a feature to self when
// it is needed; the decider's answer 46 in #1345 does so only while the voice chat is on).
func securityHeaders(next http.Handler, microphone bool) http.Handler {
	policy := permissionsPolicy(microphone)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", policy)
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

// permissionsPolicy is the Permissions-Policy of every response: no camera, geolocation,
// payment, or USB, and the microphone only for this origin when microphone is set.
func permissionsPolicy(microphone bool) string {
	mic := "microphone=()"
	if microphone {
		mic = "microphone=(self)"
	}
	return "camera=(), " + mic + ", geolocation=(), payment=(), usb=()"
}

// HeaderRequestID carries the request ID in requests (from a trusted proxy) and responses.
const HeaderRequestID = "X-Request-Id"

// requestIDs gives every request an ID, kept in its context for logging and echoed in the
// response. An incoming X-Request-Id is kept when it is short and made of safe characters,
// so that IDs assigned by a load balancer can be followed; otherwise a new UUIDv7 is used.
func (a *App) requestIDs(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID(id) {
			id = a.IDs.NewID()
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
	})
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range []byte(id) {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':'
		if !ok {
			return false
		}
	}
	return true
}

// accessLog logs one line per request. It logs the path without the query string, which may
// carry tokens, and logs the health checks at debug level, since probes and uptime monitors
// call them constantly (isHealthCheck).
// The line carries the labels of http.server.request.duration under the same names and values,
// and its duration in seconds (ADR-0055 §5), so that a log-based metric counts the requests as
// the metric does: http.route and my.surface are those that problemFallback found, and are absent
// when the request reached no route (the probes of the platform, the requests refused before routing),
// which the metric does not measure either. With the HTTP metrics, it also records the request's
// http.server.request.duration in a unit of work of the Meter (ADR-0055 §3), which the line
// carries with metrics_exporter = emf (§5). The request is a span of the Tracer
// (HTTPServerSpan, ADR-0063 §1), which the line names by trace_id and span_id.
func (a *App) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		rr := &requestRoute{method: methodLabel(r.Method)}
		r, endSpan := a.startHTTPSpan(r)
		if endSpan != nil {
			defer func() { endSpan(rr, rw.status) }()
		}
		ctx := r.Context()
		recorded := false
		if a.http != nil {
			var end func()
			ctx, end = a.Meter.Begin(ctx)
			defer end()
			defer func() { // a handler that panics with http.ErrAbortHandler writes no log line
				if !recorded && rr.route != "" {
					a.http.record(ctx, r, rr, rw.status, time.Since(start))
				}
			}()
		}
		r = r.WithContext(context.WithValue(ctx, requestRouteKey{}, rr))
		next.ServeHTTP(rw, r)
		d := time.Since(start)
		if a.http != nil && rr.route != "" {
			a.http.record(ctx, r, rr, rw.status, d)
		}
		recorded = true
		level := slog.LevelInfo
		if isHealthCheck(r.URL.Path) {
			level = slog.LevelDebug
		}
		attrs := make([]slog.Attr, 0, 7)
		attrs = append(attrs, slog.String(labelMethod, methodLabel(r.Method)), slog.String(logPath, r.URL.Path))
		if rr.route != "" {
			attrs = append(attrs, slog.String(labelRoute, rr.route), slog.String(labelSurface, rr.surface))
		}
		attrs = append(attrs,
			slog.Int(labelStatus, rw.status),
			slog.Int64(logResponseBytes, rw.bytes),
			slog.Float64(logDuration, d.Seconds()),
		)
		kernel.LogUnit(r.Context(), a.Logger, level, "http request", a.Meter, attrs, HTTPRequestDuration)
	})
}

// isHealthCheck tells whether path is a health check: a probe of the platform or the endpoint
// of the uptime monitors, which the access log writes at debug level and no span traces.
func isHealthCheck(path string) bool {
	return path == PathLiveness || path == PathReadiness || path == PathInstanceHealth
}

// Keys of the access log that are not labels of the HTTP metrics, named after the
// OpenTelemetry semantic conventions where they have one (ADR-0055 §5).
const (
	logPath          = "url.path"
	logResponseBytes = "http.response.body.size"
	logDuration      = "duration_seconds"
)

// responseWriter records the status and size of a response.
type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = status, true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(p []byte) (int, error) {
	w.wroteHeader = true
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
