// SPDX-License-Identifier: AGPL-3.0-only

// Package queuepush serves the internal endpoints that a platform's queue and scheduler call
// (ADR-0120 §2, §4, §5): POST /_internal/jobs, whose body is the ID of a job of the database
// queue for the push receiver to claim and run, and POST /_internal/tick, the external trigger of
// the periodic jobs and of the sweep queue.wake. No user calls them, and they are not part of the
// client API.
//
// Every request must carry a bearer token that the PushAuthenticator accepts for the endpoint's
// URL. A request answers 2xx once its result is recorded, so that the sender drops it, and 5xx
// when it could not be, for the sender to deliver it again: the application, not the sender,
// retries a job that failed.
package queuepush

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// The paths of the endpoints.
const (
	PathJobs = "/_internal/jobs"
	PathTick = "/_internal/tick"
)

// maxBodyBytes bounds the body of a signal, a job ID.
const maxBodyBytes = 256

// writeMargin is how long past the lease of a run the response may still be written.
const writeMargin = time.Minute

// Receiver runs the job of a signal; *queue.PushReceiver implements it.
type Receiver interface {
	Receive(ctx context.Context, id string) error
	// Lease bounds the run of one signal.
	Lease() time.Duration
}

// Ticker runs the periodic work; *queue.Trigger implements it.
type Ticker interface {
	Tick(ctx context.Context) error
}

// ProblemWriter writes the error response of an HTTP status, the same one the rest of the
// server uses for that status.
type ProblemWriter func(w http.ResponseWriter, r *http.Request, status int)

// Options configure a Handler. Receiver, Trigger, Auth, and BaseURL are required.
type Options struct {
	Receiver Receiver
	Trigger  Ticker
	Auth     queue.PushAuthenticator
	// BaseURL is the absolute URL of the server that the senders were told to call, without a
	// trailing slash: the endpoint of a request is BaseURL followed by its path.
	BaseURL string
	// Problem writes error responses; nil writes a plain-text body.
	Problem ProblemWriter
	// Logger logs the refused requests and the failures; nil means slog.Default().
	Logger *slog.Logger
}

// Handler serves the endpoints.
type Handler struct{ o Options }

// New returns a Handler on o. It panics when a required option is missing.
func New(o Options) *Handler {
	if o.Receiver == nil || o.Trigger == nil || o.Auth == nil || o.BaseURL == "" {
		panic("queuepush: Options.Receiver, Trigger, Auth, and BaseURL are required")
	}
	o.BaseURL = strings.TrimSuffix(o.BaseURL, "/")
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
	r.Handle("POST "+PathJobs, http.HandlerFunc(h.serveJobs))
	r.Handle("POST "+PathTick, http.HandlerFunc(h.serveTick))
}

// serveJobs runs the job of a signal: 204 once its result is recorded (also when the job is
// not due, held back, leased, or gone), 400 for a body that is no job ID, 401 without a valid
// token, and 5xx when the token could not be checked or the result not recorded.
func (h *Handler) serveJobs(w http.ResponseWriter, r *http.Request) {
	if !h.authenticate(w, r, PathJobs) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	id := strings.TrimSpace(string(body))
	if err != nil || id == "" {
		h.o.Problem(w, r, http.StatusBadRequest)
		return
	}
	// A run lasts up to the lease, past the server's write timeout.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(h.o.Receiver.Lease() + writeMargin))
	if err := h.o.Receiver.Receive(r.Context(), id); err != nil {
		h.o.Logger.ErrorContext(r.Context(), "job signal not handled", slog.String("job.id", id), slog.Any("error", err))
		h.o.Problem(w, r, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// serveTick runs the periodic work: 204 once done, 401 without a valid token, and 5xx when the
// token could not be checked or the sweep failed.
func (h *Handler) serveTick(w http.ResponseWriter, r *http.Request) {
	if !h.authenticate(w, r, PathTick) {
		return
	}
	_, _ = io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, maxBodyBytes))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(h.o.Receiver.Lease() + writeMargin))
	if err := h.o.Trigger.Tick(r.Context()); err != nil {
		h.o.Logger.ErrorContext(r.Context(), "tick not handled", slog.Any("error", err))
		h.o.Problem(w, r, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// authenticate checks the bearer token of r for the endpoint path and writes the refusal when it
// fails.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request, path string) bool {
	w.Header().Set("Cache-Control", "no-store")
	token, ok := bearer(r.Header.Get("Authorization"))
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		h.o.Problem(w, r, http.StatusUnauthorized)
		return false
	}
	err := h.o.Auth.Authenticate(r.Context(), token, h.o.BaseURL+path)
	switch {
	case err == nil:
		return true
	case errors.Is(err, queue.ErrPushUnauthenticated):
		h.o.Logger.WarnContext(r.Context(), "internal endpoint refused a request", slog.String("url.path", path), slog.Any("error", err))
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		h.o.Problem(w, r, http.StatusUnauthorized)
	default:
		h.o.Logger.ErrorContext(r.Context(), "internal endpoint could not check a token", slog.String("url.path", path), slog.Any("error", err))
		h.o.Problem(w, r, http.StatusServiceUnavailable)
	}
	return false
}

// bearer returns the token of an Authorization header of the Bearer scheme (RFC 6750 §2.1).
func bearer(v string) (string, bool) {
	scheme, token, ok := strings.Cut(v, " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}
