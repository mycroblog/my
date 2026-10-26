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
	got []appvoice.MediaWebhook
	res appvoice.MediaWebhookResult
	err error
}

func (w *webhooks) ReceiveMediaEvents(_ context.Context, req appvoice.MediaWebhook) (appvoice.MediaWebhookResult, error) {
	w.got = append(w.got, req)
	return w.res, w.err
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
