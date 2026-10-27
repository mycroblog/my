// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/federation"
)

// redirectNet is a kernel.HTTPClient that sends https://{host}/... to the handler of host and
// follows redirects as net/http does (a 301, 302, or 303 answer to a POST becomes a GET),
// honoring NoRedirects and CheckRedirect as the port describes. It records every request.
type redirectNet struct {
	mu       sync.Mutex
	handlers map[string]http.HandlerFunc
	requests []string // "METHOD https://host/path"
}

func newRedirectNet() *redirectNet {
	return &redirectNet{handlers: map[string]http.HandlerFunc{}}
}

func (n *redirectNet) handle(host string, h http.HandlerFunc) { n.handlers[host] = h }

func (n *redirectNet) seen() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.requests...)
}

func (n *redirectNet) RoundTrip(r *http.Request) (*http.Response, error) {
	n.mu.Lock()
	n.requests = append(n.requests, r.Method+" "+r.URL.String())
	h := n.handlers[r.URL.Hostname()]
	n.mu.Unlock()
	if h == nil {
		return nil, fmt.Errorf("%w: %s", kernel.ErrForbiddenDestination, r.URL.Host)
	}
	w := httptest.NewRecorder()
	h(w, r)
	resp := w.Result()
	resp.Request = r
	return resp, nil
}

func (n *redirectNet) Do(ctx context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	c := &http.Client{Transport: n, CheckRedirect: func(next *http.Request, _ []*http.Request) error {
		if req.NoRedirects {
			return http.ErrUseLastResponse
		}
		if req.CheckRedirect != nil {
			return req.CheckRedirect(next.Context(), next.URL.String())
		}
		return nil
	}}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	r, err := http.NewRequestWithContext(ctx, method, req.URL, strings.NewReader(string(req.Body)))
	if err != nil {
		return nil, err
	}
	if req.Header != nil {
		r.Header = req.Header.Clone()
	}
	resp, err := c.Do(r)
	if err != nil {
		return nil, err
	}
	return &kernel.HTTPResponse{StatusCode: resp.StatusCode, Header: resp.Header, URL: resp.Request.URL.String(), Body: resp.Body}, nil
}

// nopSigner signs nothing: these tests are about where the requests go.
type nopSigner struct{}

func (nopSigner) Sign(*kernel.HTTPRequest, SigningKey, SignOptions) error { return nil }
func (nopSigner) ParseAcceptSignature(string) ([]RequestedSignature, error) {
	return nil, nil
}

// emptyKV stores nothing.
type emptyKV struct{}

func (emptyKV) Get(context.Context, string, string) ([]byte, error) {
	return nil, kernel.ErrKeyNotFound
}
func (emptyKV) Set(context.Context, string, string, []byte, time.Duration) error {
	return nil
}
func (emptyKV) Delete(context.Context, string, string) error { return nil }
func (emptyKV) CompareAndSwap(context.Context, string, string, []byte, []byte, time.Duration) (bool, error) {
	return false, nil
}
func (emptyKV) IncrBy(context.Context, string, string, int64, time.Duration) (int64, error) {
	return 0, nil
}
func (emptyKV) DeleteExpired(context.Context, int) (int64, error) { return 0, nil }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

// suspendedDomains suspends the listed domains.
type suspendedDomains []string

func (s suspendedDomains) FederationSuspended(_ context.Context, domain string) (bool, error) {
	for _, d := range s {
		if d == domain {
			return true, nil
		}
	}
	return false, nil
}

func testSigningKey(t *testing.T) SigningKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return SigningKey{KeyID: "https://social.example/actor#main-key", PrivateKey: priv}
}

func testFetcher(t *testing.T, n *redirectNet, suspended ...string) *Fetcher {
	key := testSigningKey(t)
	return NewFetcher(FetcherDeps{
		Client:    NewSignedHTTPClient(n, nopSigner{}, emptyKV{}, fixedClock{}),
		KV:        emptyKV{},
		Clock:     fixedClock{},
		Domains:   suspendedDomains(suspended),
		Instances: availableInstances{},
		Key:       func(context.Context) (SigningKey, error) { return key, nil },
		Host:      "social.example",
	})
}

// A delivery whose inbox answers with a redirect is not delivered: the POST is not followed
// (as a GET, or at all), and the redirect is a failure.
func TestDeliverDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			n := newRedirectNet()
			n.handle("remote.example", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/inbox" {
					http.Redirect(w, r, "https://remote.example/landing", status)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
			d := &Deliverer{d: DeliverDeps{Client: NewSignedHTTPClient(n, nopSigner{}, emptyKV{}, fixedClock{}), Clock: fixedClock{}}}
			a, _ := d.send(t.Context(), federation.Delivery{InboxURL: "https://remote.example/inbox", Domain: "remote.example"}, []byte(`{}`), testSigningKey(t))
			if a.Outcome == federation.DeliverySucceeded {
				t.Errorf("outcome = %s after a %d, want a failure", a.Outcome, status)
			}
			if a.HTTPStatus != status {
				t.Errorf("HTTPStatus = %d, want %d", a.HTTPStatus, status)
			}
			if got := n.seen(); len(got) != 1 || got[0] != "POST https://remote.example/inbox" {
				t.Errorf("requests = %q, want the POST to the inbox only", got)
			}
		})
	}
}

// A fetch that redirects to a suspended server fails as refused without reaching it.
func TestFetchRefusesARedirectToASuspendedServer(t *testing.T) {
	n := newRedirectNet()
	n.handle("remote.example", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://blocked.example/note", http.StatusFound)
	})
	n.handle("blocked.example", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/activity+json")
		_, _ = io.WriteString(w, `{"id":"https://blocked.example/note","type":"Note"}`)
	})
	_, err := testFetcher(t, n, "blocked.example").Fetch(t.Context(), "https://remote.example/note")
	if !errors.Is(err, ErrFetchRefused) {
		t.Errorf("Fetch() = %v, want ErrFetchRefused", err)
	}
	for _, r := range n.seen() {
		if strings.Contains(r, "blocked.example") {
			t.Errorf("request %q reached the suspended server", r)
		}
	}
}

// The handle check does not ask a suspended server, or this instance, to confirm a handle that
// another server's WebFinger names, and the handle is not confirmed.
func TestCheckHandleRefusesASuspendedAccountDomain(t *testing.T) {
	for _, domain := range []string{"blocked.example", "social.example"} {
		t.Run(domain, func(t *testing.T) {
			n := newRedirectNet()
			n.handle("remote.example", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"subject":"acct:alice@`+domain+`","links":[{"rel":"self","type":"application/activity+json","href":"https://remote.example/users/alice"}]}`)
			})
			n.handle(domain, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"subject":"acct:alice@`+domain+`","links":[{"rel":"self","type":"application/activity+json","href":"https://remote.example/users/alice"}]}`)
			})
			r := &RemoteActors{d: RemoteActorsDeps{Fetcher: testFetcher(t, n, "blocked.example"), HTTP: n}}
			got := r.checkHandle(t.Context(), &federation.Actor{ID: "https://remote.example/users/alice", PreferredUsername: "alice"})
			if got.Outcome == appaccount.HandleVerified {
				t.Errorf("checkHandle() = %+v, want no verified handle", got)
			}
			for _, req := range n.seen() {
				if strings.Contains(req, domain) {
					t.Errorf("request %q reached %s", req, domain)
				}
			}
		})
	}
}

// WebFinger that redirects to a suspended server does not confirm a handle.
func TestWebFingerRefusesARedirectToASuspendedServer(t *testing.T) {
	n := newRedirectNet()
	n.handle("remote.example", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://blocked.example/.well-known/webfinger?resource=acct%3Aalice%40remote.example", http.StatusFound)
	})
	n.handle("blocked.example", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"subject":"acct:alice@remote.example","links":[{"rel":"self","type":"application/activity+json","href":"https://remote.example/users/alice"}]}`)
	})
	r := &RemoteActors{d: RemoteActorsDeps{Fetcher: testFetcher(t, n, "blocked.example"), HTTP: n}}
	got := r.checkHandle(t.Context(), &federation.Actor{ID: "https://remote.example/users/alice", PreferredUsername: "alice"})
	if got.Outcome != appaccount.HandleMismatch {
		t.Errorf("checkHandle() = %+v, want a mismatch", got)
	}
	for _, req := range n.seen() {
		if u, err := url.Parse(strings.TrimPrefix(req, "GET ")); err == nil && u.Hostname() == "blocked.example" {
			t.Errorf("request %q reached the suspended server", req)
		}
	}
}
