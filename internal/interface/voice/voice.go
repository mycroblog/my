// SPDX-License-Identifier: AGPL-3.0-only

// Package voice serves the endpoints that the media servers of the voice chat call (ADR-0102
// §1, §4), one per method: POST /voice/webhooks/livekit receives the events of a LiveKit
// server. They are not part of the client API: no user calls them. The body is read up to
// appvoice.MediaWebhookMaxBodyBytes before anything else, and the voice module verifies the
// delivery, applies the rate limit, and queues its events; an endpoint of another method than
// the voice chat's answers 404.
package voice

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appvoice "github.com/mycroblog/my/internal/app/voice"
)

// PathPrefix is the path of the webhook endpoints, followed by the method and, for a method
// with several, the endpoint's name.
const PathPrefix = "/voice/webhooks/"

// EventRoutes are the endpoints of the events of each method, by path below PathPrefix.
var EventRoutes = map[string]string{
	"livekit": "livekit",
}

// Webhooks is what the endpoints need from the voice module; *appvoice.Service implements it.
type Webhooks interface {
	ReceiveMediaEvents(ctx context.Context, req appvoice.MediaWebhook) (appvoice.MediaWebhookResult, error)
}

// ProblemWriter writes the error response of an HTTP status, the same one the rest of the
// server uses for that status.
type ProblemWriter func(w http.ResponseWriter, r *http.Request, status int)

// Options configure a Handler.
type Options struct {
	Webhooks Webhooks
	// Problem writes error responses; nil writes a plain-text body.
	Problem ProblemWriter
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
}

// Handler serves the webhook endpoints.
type Handler struct{ o Options }

// New returns a Handler on o.
func New(o Options) *Handler {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Problem == nil {
		o.Problem = func(w http.ResponseWriter, _ *http.Request, status int) {
			http.Error(w, http.StatusText(status), status)
		}
	}
	return &Handler{o: o}
}

// Router registers handlers for http.ServeMux patterns.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Register registers the endpoints on r.
func (h *Handler) Register(r Router) {
	for path, transport := range EventRoutes {
		r.Handle("POST "+PathPrefix+path, h.Events(transport))
	}
}

// Events returns the endpoint of the events of transport: 204 for a delivery whose events are
// queued or ignored; 400 for a wrong signature or body; 404 when the voice chat does not run on
// transport; 413 beyond the body's limit; 429 beyond the rate limit; and 503 when the events
// could not be queued, for the media server to send them again.
func (h *Handler) Events(transport string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, appvoice.MediaWebhookMaxBodyBytes))
		if err != nil {
			var tooLarge *http.MaxBytesError
			status := http.StatusBadRequest
			if errors.As(err, &tooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			h.o.Problem(w, r, status)
			return
		}
		res, err := h.o.Webhooks.ReceiveMediaEvents(r.Context(), appvoice.MediaWebhook{
			Transport: transport, Header: r.Header, Body: body, ClientIP: kernel.RequestClientIP(r),
		})
		if err != nil {
			h.o.Logger.ErrorContext(r.Context(), "voice webhook not queued", slog.String("transport", transport), slog.Any("error", err))
			h.o.Problem(w, r, http.StatusServiceUnavailable)
			return
		}
		switch res.Outcome {
		case appvoice.MediaWebhookAccepted:
			w.WriteHeader(http.StatusNoContent)
		case appvoice.MediaWebhookRejected:
			// The reason, never the body or the token.
			h.o.Logger.WarnContext(r.Context(), "voice webhook refused", slog.String("transport", transport), slog.Any("error", res.Reason))
			h.o.Problem(w, r, http.StatusBadRequest)
		case appvoice.MediaWebhookRateLimited:
			w.Header().Set("Retry-After", strconv.Itoa(int((res.RetryAfter+time.Second-1)/time.Second)))
			h.o.Problem(w, r, http.StatusTooManyRequests)
		default:
			h.o.Problem(w, r, http.StatusNotFound)
		}
	})
}
