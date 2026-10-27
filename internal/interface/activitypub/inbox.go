// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
)

// Inbox is what the inbox endpoints need from the federation module; *appfederation.Inbox
// implements it.
type Inbox interface {
	Receive(ctx context.Context, in appfederation.InboxRequest) (appfederation.Received, error)
}

// as2Profile is the profile that an application/ld+json body must declare (ActivityPub §7).
const as2Profile = "https://www.w3.org/ns/activitystreams"

// sharedInbox serves POST /ap/inbox.
func (h *Handler) sharedInbox(w http.ResponseWriter, r *http.Request) {
	h.receive(w, r)
}

// actorInbox serves POST /ap/actors/{id}/inbox: the inbox of an active local actor, 404 for any
// other id. Every inbox records the activity the same way; who it is for is in its addressing.
func (h *Handler) actorInbox(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !kernel.ValidID(id) {
		h.notFound(w, r)
		return
	}
	if _, err := findVisible(h.o.Accounts.FindByID(r.Context(), id)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.receive(w, r)
}

// receive runs the synchronous part of the inbox (federation.md §3.1): the content type (415)
// and the body size (413) here, the rest in the federation module. It answers 202 Accepted once
// the activity is recorded, and the status of the refusing step otherwise.
func (h *Handler) receive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !activityContentType(r.Header.Get("Content-Type")) {
		h.o.Problem(w, r, http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, appfederation.InboxMaxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		h.o.Problem(w, r, status)
		return
	}
	origin := &url.URL{Scheme: "https", Host: h.o.Host}
	got, err := h.o.Inbox.Receive(r.Context(), appfederation.InboxRequest{
		Request: appfederation.NewSignedRequest(origin, r, body), ClientIP: kernel.RequestClientIP(r),
	})
	switch rej := got.Rejection; {
	case err != nil:
		h.o.Logger.ErrorContext(r.Context(), "inbox failed", slog.Any("error", err))
		h.report(r.Context(), err)
		h.o.Problem(w, r, http.StatusInternalServerError)
	case rej != nil:
		if rej.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int((rej.RetryAfter+time.Second-1)/time.Second)))
		}
		if rej.AcceptSignature != "" {
			w.Header().Set("Accept-Signature", rej.AcceptSignature)
		}
		// The reason, not the error, which quotes the request's signature headers (ADR-0019).
		h.o.Logger.DebugContext(r.Context(), "inbox refused a request", slog.String("path", r.URL.Path),
			slog.Int("status", rej.Status), slog.String("reason", rej.Reason()))
		h.o.Problem(w, r, rej.Status)
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}

// activityContentType reports whether a Content-Type names an ActivityPub body:
// application/activity+json, application/ld+json with the ActivityStreams profile, or
// application/json.
func activityContentType(v string) bool {
	mt, params, err := mime.ParseMediaType(v)
	if err != nil {
		return false
	}
	switch mt {
	case "application/activity+json", "application/json":
		return true
	case "application/ld+json":
		for _, p := range strings.Fields(params["profile"]) {
			if strings.TrimSuffix(p, "#") == as2Profile {
				return true
			}
		}
	}
	return false
}
