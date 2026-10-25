// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strings"

	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	mediahttp "github.com/mycroblog/my/internal/interface/media"
	"github.com/mycroblog/my/internal/interface/web"
)

// noIndexPrefixes are the paths whose responses are not pages for search engines: the client
// API, the sign-in and OAuth screens, the email unsubscribe links, ActivityPub documents,
// /.well-known/, and NodeInfo (ADR-0057). Their responses carry X-Robots-Tag: noindex.
var noIndexPrefixes = []string{"/api/", "/oauth/", "/auth/", "/mail/", "/ap/", "/.well-known/", "/nodeinfo/"}

// noArchivePaths are the API paths whose responses, problems included, carry X-Robots-Tag:
// noindex, noarchive whatever the instance states: the supporter list (ADR-0071 §4).
var noArchivePaths = []string{"/api/v1/instance/supporters"}

// noIndexTypes are the media types of documents that are not pages, wherever they are served:
// ActivityPub documents negotiated at a page's URL, and problem documents.
var noIndexTypes = map[string]bool{
	"application/activity+json": true, "application/ld+json": true, "application/json": true,
	"application/problem+json": true, "application/jrd+json": true, "application/xrd+xml": true,
}

// crawlerHeaders states the instance's crawler policy on the responses that the web client's
// handler does not (ADR-0057): noindex on the responses that are not pages, and the instance's
// policy on the media files the server serves. The pages state theirs themselves
// (internal/interface/web).
func (a *App) crawlerHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, mediahttp.PathPrefix) && a.Instance != nil:
			s, err := a.Instance.Settings(r.Context())
			if err != nil {
				// Without the settings, the defaults apply, which deny.
				a.Logger.WarnContext(r.Context(), "read the instance settings for the media headers", slog.Any("error", err))
				s = domaininstance.Settings{}
			}
			web.SetMediaHeaders(w.Header(), s)
		case slices.Contains(noArchivePaths, p):
			w.Header().Set("X-Robots-Tag", "noindex, noarchive")
		case hasAnyPrefix(p, noIndexPrefixes):
			w.Header().Set("X-Robots-Tag", web.HeaderNoIndex)
		default:
			w = &noIndexByType{ResponseWriter: w}
		}
		next.ServeHTTP(w, r)
	})
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// noIndexByType adds X-Robots-Tag: noindex to a response of one of noIndexTypes that has none.
type noIndexByType struct {
	http.ResponseWriter
	checked bool
}

func (w *noIndexByType) check() {
	if w.checked {
		return
	}
	w.checked = true
	h := w.Header()
	if h.Get("X-Robots-Tag") != "" {
		return
	}
	if mediaType, _, err := mime.ParseMediaType(h.Get("Content-Type")); err == nil && noIndexTypes[mediaType] {
		h.Set("X-Robots-Tag", web.HeaderNoIndex)
	}
}

func (w *noIndexByType) WriteHeader(status int) {
	w.check()
	w.ResponseWriter.WriteHeader(status)
}

func (w *noIndexByType) Write(p []byte) (int, error) {
	w.check()
	return w.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *noIndexByType) Unwrap() http.ResponseWriter { return w.ResponseWriter }
