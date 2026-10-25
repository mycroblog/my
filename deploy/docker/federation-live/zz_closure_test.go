// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	// closureDeleteTimeout bounds the wait for a peer to take the closure's Delete (Actor): the
	// fan-out pages, the delivery, and the peer's deletion job.
	closureDeleteTimeout = 3 * time.Minute
	// closureCompletionTimeout bounds the wait for delivery_completed: the completion check runs
	// every 10 minutes (instance.closure_completion) once every delivery has ended.
	closureCompletionTimeout = 15 * time.Minute
	// closureQuiet is how long a peer must send this server nothing, after it took the Delete and
	// its user posted again, for its deliveries to count as stopped: it covers the first retry of
	// a delivery that a peer would retry after a 410 (Mastodon's Sidekiq backoff starts at 15 s
	// to 45 s).
	closureQuiet = 75 * time.Second
)

// closureBefore is what the peer shows of alice before the closure.
type closureBefore struct {
	// alice is alice's account on the peer.
	alice account
	// post is alice's post as the peer stored it.
	post status
	// posted is when the peer user posted right after the closure.
	posted time.Time
}

// TestClosure closes this server for good (FR-809, ADR-0088) and checks what every peer does with
// it. The closure cannot be undone and ends the federation of the whole server, so this scenario
// runs last: its file sorts after the others.
//
//   - Before: alice and the peer user follow each other, and the peer stores a post of alice.
//   - The closure: an announcement whose closing date has passed, then the closure through the
//     admin API. Right away the peer user posts, so that the peer delivers to this server's
//     inbox, which answers 410, while the Deletes go out.
//   - This server, closed: the inbox answers 410 before any signature check, WebFinger 410, and
//     alice's actor the minimal suspended document with its key, which the peers need to verify
//     the Delete.
//   - Each peer takes the Delete (Actor) of alice: alice is gone there, and so is her post. The
//     peer user posts again, and the peer sends this server nothing more: no retry of the
//     deliveries that got a 410, and no delivery to the deleted followers. Every POST the peer
//     made after the closure got 410; Misskey marks this server goneSuspended once one of its
//     deliveries got a 410.
//   - Completion: delivery_completed once every Delete has been delivered, and from then on the
//     actor answers 410 with a Tombstone.
func TestClosure(t *testing.T) {
	e := stack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()

	self, err := e.local.resolve(ctx, localHandle)
	if err != nil {
		t.Fatal(err)
	}
	var actor struct {
		Inbox     string `json:"inbox"`
		Endpoints struct {
			SharedInbox string `json:"sharedInbox"`
		} `json:"endpoints"`
	}
	if code, body, err := e.getActivity(ctx, self.URI); err != nil || code != http.StatusOK || json.Unmarshal([]byte(body), &actor) != nil {
		t.Fatalf("GET %s = %d %v: %s", self.URI, code, err, body)
	}

	before := map[string]*closureBefore{}
	for _, p := range e.peers {
		t.Run(p.name+"/before", func(t *testing.T) {
			_, alice := mutual(t, e, p)
			mine, err := e.local.post(ctx, unique("before the closure, to "+p.name), "public")
			if err != nil {
				t.Fatal(err)
			}
			before[p.name] = &closureBefore{alice: alice, post: peerSees(t, p, mine.URI)}
		})
	}
	if t.Failed() {
		return
	}

	// The closing date has passed, so the closure may follow the announcement at once.
	if err := e.local.call(ctx, http.MethodPut, "/admin/closure/announcement",
		map[string]any{"closes_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)}, nil); err != nil {
		t.Fatalf("announcing the closure: %v", err)
	}
	closedAt := time.Now()
	var closure struct {
		State string `json:"state"`
	}
	if err := e.local.call(ctx, http.MethodPost, "/admin/closure/close", map[string]any{"confirm_domain": localHost}, &closure); err != nil || closure.State != "closed" {
		t.Fatalf("closing %s = %+v %v", localHost, closure, err)
	}
	for _, p := range e.peers {
		if _, err := p.post(ctx, unique("right after the closure, from "+p.name), "public"); err != nil {
			t.Errorf("posting as %s after the closure: %v", p.handle(), err)
		}
		before[p.name].posted = time.Now()
	}

	t.Run("closed", func(t *testing.T) {
		for _, inbox := range []string{actor.Inbox, actor.Endpoints.SharedInbox} {
			if inbox == "" {
				continue
			}
			if code, body, err := e.postUnsigned(ctx, inbox); err != nil || code != http.StatusGone {
				t.Errorf("unsigned POST %s = %d %v: %s, want 410", inbox, code, err, body)
			}
		}
		webFinger := "https://" + localHost + "/.well-known/webfinger?" + url.Values{"resource": {"acct:" + localHandle}}.Encode()
		if code, body, err := e.getActivity(ctx, webFinger); err != nil || code != http.StatusGone {
			t.Errorf("GET %s = %d %v: %s, want 410", webFinger, code, err, body)
		}
		code, body, err := e.getActivity(ctx, self.URI)
		var doc struct {
			Suspended bool `json:"suspended"`
			PublicKey struct {
				PublicKeyPem string `json:"publicKeyPem"`
			} `json:"publicKey"`
		}
		if err != nil || code != http.StatusOK || json.Unmarshal([]byte(body), &doc) != nil || !doc.Suspended || doc.PublicKey.PublicKeyPem == "" {
			t.Errorf("GET %s while the Deletes go out = %d %v: %s, want the suspended document with its key", self.URI, code, err, body)
		}
	})

	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			b := before[p.name]
			eventually(t, closureDeleteTimeout, p.host+" deletes "+localHandle, func() (bool, string) {
				gone, state, err := p.gone(ctx, b.alice.ID)
				return err == nil && gone, fmt.Sprintf("%s %v", state, err)
			})
			eventually(t, closureDeleteTimeout, p.host+" deletes "+b.post.URI, func() (bool, string) {
				s, err := p.status(ctx, b.post.ID)
				return isNotFound(err), fmt.Sprintf("%+v %v", s, err)
			})
			// The first deliveries of the post right after the closure have landed by now.
			if wait := time.Until(b.posted.Add(10 * time.Second)); wait > 0 {
				time.Sleep(wait)
			}
			again := time.Now()
			if _, err := p.post(ctx, unique("after the Delete, from "+p.name), "public"); err != nil {
				t.Fatal(err)
			}
			time.Sleep(closureQuiet)

			posts, err := peerRequests(ctx, p, http.MethodPost, closedAt)
			if err != nil {
				t.Fatalf("reading the access log of the proxy: %v", err)
			}
			gone := 0
			for _, en := range posts {
				t.Logf("%s POST %s at +%s = %d", p.host, en.Request.URI, en.time().Sub(closedAt).Round(time.Second), en.Status)
				if en.Status != http.StatusGone {
					t.Errorf("%s POST %s after the closure = %d, want 410", p.host, en.Request.URI, en.Status)
				} else {
					gone++
				}
				if en.time().After(again) {
					t.Errorf("%s still delivers to %s after it deleted %s: POST %s at +%s", p.host, localHost, localHandle,
						en.Request.URI, en.time().Sub(again).Round(time.Second))
				}
			}
			t.Logf("%s made %d POSTs to %s after the closure, %d answered 410", p.host, len(posts), localHost, gone)
			if p.name == "misskey" {
				e.checkGoneSuspended(ctx, t, gone > 0)
			}
		})
	}

	t.Run("delivery completed", func(t *testing.T) {
		var got struct {
			State    string          `json:"state"`
			Progress json.RawMessage `json:"progress"`
		}
		eventually(t, closureCompletionTimeout, "delivery_completed", func() (bool, string) {
			err := e.local.call(ctx, http.MethodGet, "/admin/closure", nil, &got)
			return err == nil && got.State == "delivery_completed", fmt.Sprintf("%s %s %v", got.State, got.Progress, err)
		})
		t.Logf("the closure's progress: %s", got.Progress)
		code, body, err := e.getActivity(ctx, self.URI)
		if err != nil || code != http.StatusGone || !strings.Contains(body, "Tombstone") {
			t.Errorf("GET %s after delivery_completed = %d %v: %s, want 410 with a Tombstone", self.URI, code, err, body)
		}
	})
}

// postUnsigned POSTs an empty activity to an inbox of this server without a signature, and returns
// the status and the body.
func (e *env) postUnsigned(ctx context.Context, inbox string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inbox, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/activity+json")
	resp, err := e.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), err
}

// checkGoneSuspended checks that Misskey marks this server goneSuspended after one of its
// deliveries got a 410 (DeliverProcessorService), and logs how it shows the server otherwise.
func (e *env) checkGoneSuspended(ctx context.Context, t *testing.T, delivered bool) {
	t.Helper()
	admin := &misskeyAPI{host: misskeyHost, token: os.Getenv("FEDERATION_LIVE_MISSKEY_ADMIN_TOKEN"), http: e.http}
	var instance struct {
		SuspensionState string `json:"suspensionState"`
		IsSuspended     bool   `json:"isSuspended"`
		IsNotResponding bool   `json:"isNotResponding"`
		FollowersCount  int    `json:"followersCount"`
		FollowingCount  int    `json:"followingCount"`
	}
	read := func() (bool, string) {
		err := admin.call(ctx, "federation/show-instance", map[string]any{"host": localHost}, &instance)
		return err == nil && instance.SuspensionState == "goneSuspended", fmt.Sprintf("%+v %v", instance, err)
	}
	if !delivered {
		_, state := read()
		t.Logf("%s made no delivery to %s after the closure, so goneSuspended cannot be checked; it shows the server as %s", misskeyHost, localHost, state)
		return
	}
	eventually(t, deliveryTimeout, misskeyHost+" marks "+localHost+" goneSuspended", read)
	t.Logf("%s shows %s as %+v", misskeyHost, localHost, instance)
}
