// SPDX-License-Identifier: AGPL-3.0-only

package voice_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/interface/voice"
)

type webhooks struct {
	got     []appvoice.MediaWebhook
	res     appvoice.MediaWebhookResult
	connect appvoice.MediaConnectResult
	err     error
}

func (w *webhooks) ReceiveMediaEvents(_ context.Context, req appvoice.MediaWebhook) (appvoice.MediaWebhookResult, error) {
	w.got = append(w.got, req)
	return w.res, w.err
}

func (w *webhooks) AuthorizeMediaConnect(_ context.Context, req appvoice.MediaWebhook) (appvoice.MediaConnectResult, error) {
	w.got = append(w.got, req)
	return w.connect, w.err
}

func TestSoraEventsAnswerJSON(t *testing.T) {
	wh := &webhooks{res: appvoice.MediaWebhookResult{Outcome: appvoice.MediaWebhookAccepted}}
	mux := http.NewServeMux()
	voice.New(voice.Options{Webhooks: wh}).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/voice/webhooks/sora/events", strings.NewReader(`{"type":"session.created"}`)))
	// The session webhook of Sora destroys the session unless the answer is a JSON object.
	if rec.Code != http.StatusOK || rec.Body.String() != "{}" || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("sora events = %d %q %q, want 200 {}", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if len(wh.got) != 1 || wh.got[0].Transport != "sora" {
		t.Errorf("delivery = %+v", wh.got)
	}
}

func TestConnect(t *testing.T) {
	allowed := []byte(`{"allowed":true}`)
	for name, c := range map[string]struct {
		res    appvoice.MediaConnectResult
		err    error
		body   string
		status int
		want   string
	}{
		"answered": {res: appvoice.MediaConnectResult{Outcome: appvoice.MediaWebhookAccepted, Status: http.StatusOK, Body: allowed}, body: "{}", status: http.StatusOK, want: string(allowed)},
		"unauthenticated": {
			res: appvoice.MediaConnectResult{Outcome: appvoice.MediaWebhookRejected, Status: http.StatusUnauthorized, Reason: errors.New("bad")}, body: "{}", status: http.StatusUnauthorized,
		},
		"not found":    {res: appvoice.MediaConnectResult{Outcome: appvoice.MediaWebhookNotFound}, body: "{}", status: http.StatusNotFound},
		"rate limited": {res: appvoice.MediaConnectResult{Outcome: appvoice.MediaWebhookRateLimited, RetryAfter: time.Second}, body: "{}", status: http.StatusTooManyRequests},
		"failed":       {err: errors.New("db down"), body: "{}", status: http.StatusServiceUnavailable},
		"too large":    {body: strings.Repeat("x", appvoice.MediaWebhookMaxBodyBytes+1), status: http.StatusRequestEntityTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			wh := &webhooks{connect: c.res, err: c.err}
			mux := http.NewServeMux()
			voice.New(voice.Options{Webhooks: wh}).Register(mux)
			req := httptest.NewRequest(http.MethodPost, "/voice/webhooks/sora/auth", strings.NewReader(c.body))
			req.SetBasicAuth("sora", "hunter2")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			if c.want != "" && (rec.Body.String() != c.want || rec.Header().Get("Content-Type") != "application/json") {
				t.Errorf("body = %q (%s), want %q", rec.Body.String(), rec.Header().Get("Content-Type"), c.want)
			}
			if c.status == http.StatusRequestEntityTooLarge {
				if len(wh.got) != 0 {
					t.Error("a body over the limit reached the voice module")
				}
				return
			}
			if len(wh.got) != 1 || wh.got[0].Transport != "sora" || wh.got[0].Header.Get("Authorization") == "" {
				t.Errorf("request = %+v", wh.got)
			}
		})
	}
}

func TestEvents(t *testing.T) {
	for name, c := range map[string]struct {
		res    appvoice.MediaWebhookResult
		err    error
		body   string
		status int
	}{
		"accepted":     {res: appvoice.MediaWebhookResult{Outcome: appvoice.MediaWebhookAccepted}, body: "{}", status: http.StatusNoContent},
		"rejected":     {res: appvoice.MediaWebhookResult{Outcome: appvoice.MediaWebhookRejected, Reason: errors.New("bad")}, body: "{}", status: http.StatusBadRequest},
		"not found":    {res: appvoice.MediaWebhookResult{Outcome: appvoice.MediaWebhookNotFound}, body: "{}", status: http.StatusNotFound},
		"rate limited": {res: appvoice.MediaWebhookResult{Outcome: appvoice.MediaWebhookRateLimited, RetryAfter: 1500 * time.Millisecond}, body: "{}", status: http.StatusTooManyRequests},
		"unqueued":     {err: errors.New("db down"), body: "{}", status: http.StatusServiceUnavailable},
		"too large":    {body: strings.Repeat("x", appvoice.MediaWebhookMaxBodyBytes+1), status: http.StatusRequestEntityTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			wh := &webhooks{res: c.res, err: c.err}
			mux := http.NewServeMux()
			voice.New(voice.Options{Webhooks: wh}).Register(mux)
			req := httptest.NewRequest(http.MethodPost, "/voice/webhooks/livekit", strings.NewReader(c.body))
			req.Header.Set("Authorization", "token")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			if c.status == http.StatusTooManyRequests && rec.Header().Get("Retry-After") != "2" {
				t.Errorf("Retry-After = %q, want 2", rec.Header().Get("Retry-After"))
			}
			if c.status == http.StatusRequestEntityTooLarge {
				if len(wh.got) != 0 {
					t.Error("a body over the limit reached the voice module")
				}
				return
			}
			if len(wh.got) != 1 || wh.got[0].Transport != "livekit" || string(wh.got[0].Body) != c.body || wh.got[0].Header.Get("Authorization") != "token" {
				t.Errorf("delivery = %+v", wh.got)
			}
		})
	}
	mux := http.NewServeMux()
	voice.New(voice.Options{Webhooks: &webhooks{}}).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/voice/webhooks/livekit", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d, want 405", rec.Code)
	}
}
