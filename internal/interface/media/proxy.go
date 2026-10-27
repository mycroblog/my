// SPDX-License-Identifier: AGPL-3.0-only

package media

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
)

// RemoteMedia opens remote media files for the proxy. content.MediaProxy implements it.
type RemoteMedia interface {
	Open(ctx context.Context, mediaID, variant, sig string) (*content.ProxiedMedia, error)
}

// ProxyOptions configure a ProxyHandler.
type ProxyOptions struct {
	Media RemoteMedia
	// Problem writes error responses; nil writes a plain-text body.
	Problem ProblemWriter
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
	// Errors receives the errors behind a 500 answer (ADR-0103 §2); nil reports nothing.
	Errors kernel.ErrorReporter
}

// ProxyHandler serves the media proxy: GET and HEAD of
// content.ProxyPathPrefix + "{media_id}/{variant}?sig=..." relay a remote media file from the
// instance's origin (docs/architecture/federation.md §9). A missing or wrong signature, an
// unknown file, a rejected domain, a refused destination, and content outside the allowed
// formats or over the size limit are all 404; a remote server that cannot be reached is 502.
type ProxyHandler struct {
	o ProxyOptions
}

// NewProxy returns a ProxyHandler on o.
func NewProxy(o ProxyOptions) *ProxyHandler {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Errors == nil {
		o.Errors = kernel.NopErrorReporter{}
	}
	if o.Problem == nil {
		o.Problem = func(w http.ResponseWriter, _ *http.Request, status int) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, http.StatusText(status), status)
		}
	}
	return &ProxyHandler{o: o}
}

// Register registers the handler on r. A GET pattern also matches HEAD; it is more specific
// than the pattern of the public blobs, so it wins over it.
func (h *ProxyHandler) Register(r Router) {
	r.Handle("GET "+content.ProxyPathPrefix+"{media_id}/{variant}", h)
}

// ServeHTTP implements http.Handler.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m, err := h.o.Media.Open(r.Context(), r.PathValue("media_id"), r.PathValue("variant"), r.URL.Query().Get("sig"))
	switch {
	case errors.Is(err, content.ErrNotFound):
		h.o.Logger.DebugContext(r.Context(), "media proxy refused", slog.Any("error", err))
		h.o.Problem(w, r, http.StatusNotFound)
		return
	case errors.Is(err, content.ErrProxyUpstream):
		h.o.Logger.InfoContext(r.Context(), "media proxy fetch failed", slog.Any("error", err))
		h.o.Problem(w, r, http.StatusBadGateway)
		return
	case err != nil:
		h.o.Logger.ErrorContext(r.Context(), "media proxy", slog.Any("error", err))
		h.o.Errors.Report(r.Context(), kernel.NewErrorEvent(err, "")) // the reporter names the route
		h.o.Problem(w, r, http.StatusInternalServerError)
		return
	}
	defer func() { _ = m.Body.Close() }()

	hd := w.Header()
	// The type detected from the content, never the remote server's (ADR-0019 §8).
	hd.Set("Content-Type", m.ContentType)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hd.Set("Cross-Origin-Resource-Policy", "cross-origin")
	hd.Set("Content-Disposition", "inline")
	hd.Set("Cache-Control", "public, max-age="+strconv.FormatInt(int64(m.MaxAge.Seconds()), 10))
	if m.Length >= 0 {
		hd.Set("Content-Length", strconv.FormatInt(m.Length, 10))
	}
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, m.Body); err != nil {
		// The status is sent: abort the response, so that neither the client nor a CDN takes
		// a truncated file for the whole one.
		h.o.Logger.InfoContext(r.Context(), "media proxy relay aborted", slog.Any("error", err))
		panic(http.ErrAbortHandler)
	}
}
