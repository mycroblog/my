// SPDX-License-Identifier: AGPL-3.0-only

package queuepush_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/interface/queuepush"
)

const base = "https://my.example"

type fakeReceiver struct {
	ids []string
	err error
}

func (f *fakeReceiver) Receive(_ context.Context, id string) error {
	f.ids = append(f.ids, id)
	return f.err
}

func (f *fakeReceiver) Lease() time.Duration { return 5 * time.Minute }

type fakeTrigger struct {
	ticks int
	err   error
}

func (f *fakeTrigger) Tick(context.Context) error {
	f.ticks++
	return f.err
}

// fakeAuth accepts the token "good" for any endpoint, and "good:<endpoint>" for that one only.
type fakeAuth struct {
	endpoints []string
	err       error
}

func (f *fakeAuth) Authenticate(_ context.Context, token, endpoint string) error {
	f.endpoints = append(f.endpoints, endpoint)
	if f.err != nil {
		return f.err
	}
	if token == "good" || token == "good:"+endpoint {
		return nil
	}
	return fmt.Errorf("%w: bad token", queue.ErrPushUnauthenticated)
}

func setup(t *testing.T) (*http.ServeMux, *fakeReceiver, *fakeTrigger, *fakeAuth) {
	t.Helper()
	r, tr, a := &fakeReceiver{}, &fakeTrigger{}, &fakeAuth{}
	mux := http.NewServeMux()
	queuepush.New(queuepush.Options{Receiver: r, Trigger: tr, Auth: a, BaseURL: base + "/"}).Register(mux)
	return mux, r, tr, a
}

func do(mux http.Handler, path, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestJobs(t *testing.T) {
	t.Parallel()
	mux, r, _, a := setup(t)
	if rec := do(mux, queuepush.PathJobs, "Bearer good", "job-1\n"); rec.Code != http.StatusNoContent {
		t.Errorf("signal = %d, want 204", rec.Code)
	}
	if len(r.ids) != 1 || r.ids[0] != "job-1" {
		t.Errorf("received = %q, want [job-1]", r.ids)
	}
	if len(a.endpoints) != 1 || a.endpoints[0] != base+queuepush.PathJobs {
		t.Errorf("authenticated for %q, want the URL of the endpoint", a.endpoints)
	}
	if rec := do(mux, queuepush.PathJobs, "bearer good:"+base+queuepush.PathJobs, "job-2"); rec.Code != http.StatusNoContent {
		t.Errorf("signal with a lower-case scheme = %d, want 204", rec.Code)
	}
	if rec := do(mux, queuepush.PathJobs, "Bearer good", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("empty body = %d, want 400", rec.Code)
	}
	if rec := do(mux, queuepush.PathJobs, "Bearer good", strings.Repeat("x", 1000)); rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body = %d, want 400", rec.Code)
	}
	r.err = errors.New("database down")
	if rec := do(mux, queuepush.PathJobs, "Bearer good", "job-3"); rec.Code < 500 {
		t.Errorf("unrecorded result = %d, want 5xx", rec.Code)
	}
}

func TestRefusals(t *testing.T) {
	t.Parallel()
	for _, path := range []string{queuepush.PathJobs, queuepush.PathTick} {
		mux, r, tr, _ := setup(t)
		for name, auth := range map[string]string{
			"no token":        "",
			"basic":           "Basic Z29vZDo=",
			"empty bearer":    "Bearer ",
			"bad token":       "Bearer bad",
			"other endpoint":  "Bearer good:" + base + "/_internal/other",
			"bearer no space": "Bearergood",
		} {
			rec := do(mux, path, auth, "job-1")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s = %d, want 401", path, name, rec.Code)
			}
			if !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Bearer") {
				t.Errorf("%s %s WWW-Authenticate = %q", path, name, rec.Header().Get("WWW-Authenticate"))
			}
		}
		if len(r.ids) != 0 || tr.ticks != 0 {
			t.Errorf("%s ran %q and %d ticks on refused requests", path, r.ids, tr.ticks)
		}
	}
}

func TestAuthUnavailable(t *testing.T) {
	t.Parallel()
	mux, r, _, a := setup(t)
	a.err = errors.New("JWKS out of reach")
	if rec := do(mux, queuepush.PathJobs, "Bearer good", "job-1"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("signal while the keys are out of reach = %d, want 503", rec.Code)
	}
	if len(r.ids) != 0 {
		t.Errorf("received %q", r.ids)
	}
}

func TestTick(t *testing.T) {
	t.Parallel()
	mux, _, tr, a := setup(t)
	if rec := do(mux, queuepush.PathTick, "Bearer good", ""); rec.Code != http.StatusNoContent {
		t.Errorf("tick = %d, want 204", rec.Code)
	}
	if tr.ticks != 1 || a.endpoints[0] != base+queuepush.PathTick {
		t.Errorf("ticks = %d for %q", tr.ticks, a.endpoints)
	}
	tr.err = errors.New("sweep failed")
	if rec := do(mux, queuepush.PathTick, "Bearer good", ""); rec.Code < 500 {
		t.Errorf("failed tick = %d, want 5xx", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, queuepush.PathTick, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET tick = %d, want 405", rec.Code)
	}
}
