// SPDX-License-Identifier: AGPL-3.0-only

package payments

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

type stubWebhooks struct {
	res supporters.WebhookResult
	err error
	got supporters.WebhookRequest
}

func (s *stubWebhooks) ReceiveWebhook(_ context.Context, req supporters.WebhookRequest) (supporters.WebhookResult, error) {
	s.got = req
	return s.res, s.err
}

func serve(t *testing.T, w Webhooks, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	New(Options{Webhooks: w, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).Register(mux)
	req := httptest.NewRequest(http.MethodPost, PathPrefix+"stripe", strings.NewReader(body))
	req.Header.Set("Fake-Signature", "t=1,v1=00")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestStatuses(t *testing.T) {
	for _, tc := range []struct {
		res    supporters.WebhookResult
		err    error
		status int
	}{
		{res: supporters.WebhookResult{Outcome: supporters.WebhookRecorded}, status: http.StatusOK},
		{res: supporters.WebhookResult{Outcome: supporters.WebhookDuplicate}, status: http.StatusOK},
		{res: supporters.WebhookResult{Outcome: supporters.WebhookIgnored}, status: http.StatusOK},
		{res: supporters.WebhookResult{Outcome: supporters.WebhookRejected, ErrorKind: supporters.ErrorInvalidSignature}, status: http.StatusBadRequest},
		{res: supporters.WebhookResult{Outcome: supporters.WebhookUnknownProvider}, status: http.StatusNotFound},
		{res: supporters.WebhookResult{Outcome: supporters.WebhookRateLimited, RetryAfter: 1500 * time.Millisecond}, status: http.StatusTooManyRequests},
		{err: errors.New("database is down"), status: http.StatusServiceUnavailable},
	} {
		stub := &stubWebhooks{res: tc.res, err: tc.err}
		rec := serve(t, stub, `{"id":"evt_1"}`)
		if rec.Code != tc.status {
			t.Errorf("%s / %v: status %d, want %d", tc.res.Outcome, tc.err, rec.Code, tc.status)
		}
		if tc.status == http.StatusTooManyRequests && rec.Header().Get("Retry-After") != "2" {
			t.Errorf("Retry-After = %q, want 2", rec.Header().Get("Retry-After"))
		}
		if stub.got.Provider != "stripe" || string(stub.got.Body) != `{"id":"evt_1"}` || stub.got.Header.Get("Fake-Signature") == "" {
			t.Errorf("the module got %+v, want the provider, the raw body, and the headers", stub.got)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	stub := &stubWebhooks{res: supporters.WebhookResult{Outcome: supporters.WebhookRecorded}}
	if rec := serve(t, stub, strings.Repeat("x", supporters.WebhookMaxBodyBytes+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a body over the limit: status %d, want 413", rec.Code)
	}
	if stub.got.Provider != "" {
		t.Error("a body over the limit reached the module")
	}
	if rec := serve(t, stub, strings.Repeat("x", supporters.WebhookMaxBodyBytes)); rec.Code != http.StatusOK {
		t.Errorf("a body at the limit: status %d, want 200", rec.Code)
	}
}

type stubPages struct {
	res      supporters.PagesResult
	err      error
	provider string
}

func (s *stubPages) Pages(_ context.Context, provider, _ string) (supporters.PagesResult, error) {
	s.provider = provider
	return s.res, s.err
}

func servePage(t *testing.T, p Pages, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	o := Options{Webhooks: &stubWebhooks{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if p != nil {
		o.Pages = p
	}
	New(o).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestPages(t *testing.T) {
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Page", r.Method+" "+r.URL.Path)
	})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		stub := &stubPages{res: supporters.PagesResult{Handler: page}}
		rec := servePage(t, stub, method, supporters.PagesPath("payjp")+"checkout")
		if rec.Code != http.StatusOK || rec.Header().Get("X-Page") != method+" /payments/pages/payjp/checkout" || stub.provider != "payjp" {
			t.Errorf("%s: %d %q for %q, want the page's handler with the whole path", method, rec.Code, rec.Header().Get("X-Page"), stub.provider)
		}
	}
	for _, tc := range []struct {
		name   string
		pages  Pages
		status int
	}{
		{name: "no pages", status: http.StatusNotFound},
		{name: "another provider", pages: &stubPages{}, status: http.StatusNotFound},
		{name: "rate limited", pages: &stubPages{res: supporters.PagesResult{RateLimited: true, RetryAfter: time.Second}}, status: http.StatusTooManyRequests},
		{name: "limiter down", pages: &stubPages{err: errors.New("down")}, status: http.StatusServiceUnavailable},
	} {
		if rec := servePage(t, tc.pages, http.MethodGet, supporters.PagesPath("payjp")+"portal"); rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.status)
		}
	}
	if rec := servePage(t, &stubPages{res: supporters.PagesResult{Handler: page}}, http.MethodDelete, supporters.PagesPath("payjp")+"checkout"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: status %d, want 405", rec.Code)
	}
}
