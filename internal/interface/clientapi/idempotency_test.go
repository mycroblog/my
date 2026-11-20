// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// idempotencyEnv is a Server whose posting counts the posts it makes, with the answers to the
// Idempotency-Keys kept in a store on clock.
type idempotencyEnv struct {
	*authEnv
	clock   *testkit.Clock
	created atomic.Int64
	// fail, when set, is the error of the next posts.
	fail error
	// started and hold, when set, report that a post began and hold it until hold is closed.
	started, hold chan struct{}
}

func newIdempotencyEnv(t *testing.T) *idempotencyEnv {
	t.Helper()
	e := &idempotencyEnv{clock: testkit.NewClock(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))}
	f := &fakeStatuses{
		create: func(appcontent.CreateInput) (appcontent.Post, error) {
			if e.hold != nil {
				close(e.started)
				<-e.hold
			}
			if e.fail != nil {
				return appcontent.Post{}, e.fail
			}
			e.created.Add(1)
			return appcontent.Post{Status: content.Status{ID: sampleStatusID}}, nil
		},
		replay: func(string, string) (appcontent.Post, error) { return appcontent.Post{}, errNotFound },
		view:   func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
	}
	e.authEnv = newAuthEnv(t, func(o *clientapi.Options) {
		o.Statuses, o.StatusReader, o.Interactions = f, f, f
		o.Idempotency = testkit.NewKeyValueStore(e.clock)
	})
	e.auth.tokens["bob-write"] = bearer("write", bobAccountID)
	return e
}

// post posts body as the user of token with the Idempotency-Key key ("" for none).
func (e *idempotencyEnv) post(t *testing.T, token, key, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/statuses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	return e.spec.Serve(t, e.h, req)
}

const (
	postBody  = `{"text":"hi","visibility":"public"}`
	otherBody = `{"text":"another","visibility":"public"}`
)

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A repeat of a request with its Idempotency-Key gets the first answer without being carried
// out again.
func TestIdempotencyKeyRepeatGetsTheFirstAnswer(t *testing.T) {
	e := newIdempotencyEnv(t)
	first := e.post(t, "write", "k1", postBody)
	firstBody := readBody(t, first)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first post = %d", first.StatusCode)
	}
	again := e.post(t, "write", "k1", postBody)
	if body := readBody(t, again); again.StatusCode != http.StatusCreated || body != firstBody ||
		again.Header.Get("Content-Type") != first.Header.Get("Content-Type") || again.Header.Get("Cache-Control") != first.Header.Get("Cache-Control") {
		t.Errorf("repeat = %d %q (%v), want the first answer %q (%v)", again.StatusCode, body, again.Header, firstBody, first.Header)
	}
	if n := e.created.Load(); n != 1 {
		t.Errorf("posted %d times, want once", n)
	}
	// Without a key, every request is carried out.
	e.post(t, "write", "", postBody)
	e.post(t, "write", "", postBody)
	if n := e.created.Load(); n != 3 {
		t.Errorf("posted %d times, want 3", n)
	}
}

// The same key with another request is refused, and carries out nothing.
func TestIdempotencyKeyWithAnotherRequestIsRefused(t *testing.T) {
	e := newIdempotencyEnv(t)
	if res := e.post(t, "write", "k1", postBody); res.StatusCode != http.StatusCreated {
		t.Fatalf("first post = %d", res.StatusCode)
	}
	res := e.post(t, "write", "k1", otherBody)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeIdempotencyKeyReused {
		t.Errorf("another body with the key = %d %q, want 422 %s", res.StatusCode, p.Type, clientapi.ProblemTypeIdempotencyKeyReused)
	}
	if n := e.created.Load(); n != 1 {
		t.Errorf("posted %d times, want once", n)
	}
}

// Keys are each user's own: another user's request with the same key is carried out.
func TestIdempotencyKeysArePerAccount(t *testing.T) {
	e := newIdempotencyEnv(t)
	if res := e.post(t, "write", "shared", postBody); res.StatusCode != http.StatusCreated {
		t.Fatalf("alice's post = %d", res.StatusCode)
	}
	if res := e.post(t, "bob-write", "shared", otherBody); res.StatusCode != http.StatusCreated {
		t.Errorf("bob's post with alice's key = %d, want 201", res.StatusCode)
	}
	if n := e.created.Load(); n != 2 {
		t.Errorf("posted %d times, want twice", n)
	}
}

// After IdempotencyWindow the key is free: a request with it is carried out again.
func TestIdempotencyKeyExpiresAfterTheWindow(t *testing.T) {
	e := newIdempotencyEnv(t)
	e.post(t, "write", "k1", postBody)
	e.clock.Advance(clientapi.IdempotencyWindow - time.Second)
	e.post(t, "write", "k1", postBody)
	if n := e.created.Load(); n != 1 {
		t.Fatalf("posted %d times within the window, want once", n)
	}
	e.clock.Advance(time.Second)
	if res := e.post(t, "write", "k1", otherBody); res.StatusCode != http.StatusCreated {
		t.Errorf("post after the window = %d, want 201", res.StatusCode)
	}
	if n := e.created.Load(); n != 2 {
		t.Errorf("posted %d times, want twice", n)
	}
}

// An answer of 5xx is not kept: the repeat is carried out.
func TestIdempotencyKeyKeepsNoServerError(t *testing.T) {
	e := newIdempotencyEnv(t)
	e.fail = errors.New("boom")
	if res := e.post(t, "write", "k1", postBody); res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("failing post = %d, want 500", res.StatusCode)
	}
	e.fail = nil
	if res := e.post(t, "write", "k1", postBody); res.StatusCode != http.StatusCreated {
		t.Errorf("repeat after a 500 = %d, want 201", res.StatusCode)
	}
	if n := e.created.Load(); n != 1 {
		t.Errorf("posted %d times, want once", n)
	}
}

// A repeat while the first request is in progress is a 409.
func TestIdempotencyKeyInUseWhileTheFirstRuns(t *testing.T) {
	e := newIdempotencyEnv(t)
	e.started, e.hold = make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/statuses", strings.NewReader(postBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer write")
		req.Header.Set("Idempotency-Key", "k1")
		e.h.ServeHTTP(httptest.NewRecorder(), req)
	})
	<-e.started
	res := e.post(t, "write", "k1", postBody)
	close(e.hold)
	wg.Wait()
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeIdempotencyKeyInUse {
		t.Errorf("repeat in progress = %d %q, want 409 %s", res.StatusCode, p.Type, clientapi.ProblemTypeIdempotencyKeyInUse)
	}
	e.hold = nil
	if res := e.post(t, "write", "k1", postBody); res.StatusCode != http.StatusCreated {
		t.Errorf("repeat after the first ended = %d, want its 201", res.StatusCode)
	}
	if n := e.created.Load(); n != 1 {
		t.Errorf("posted %d times, want once", n)
	}
}

// A key that is not valid text of at most 256 characters is a 422 about the header.
func TestIdempotencyKeyMustBeValid(t *testing.T) {
	e := newIdempotencyEnv(t)
	res := e.post(t, "write", strings.Repeat("k", content.MaxIdempotencyKeyLength+1), postBody)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Field == nil || *p.Field != "Idempotency-Key" {
		t.Errorf("long key = %d %+v, want a 422 about Idempotency-Key", res.StatusCode, p)
	}
	if n := e.created.Load(); n != 0 {
		t.Errorf("posted %d times, want never", n)
	}
}
