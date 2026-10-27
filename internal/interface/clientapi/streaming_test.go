// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// openStream opens GET /streaming with token on a real HTTP server, so that the response
// streams, and returns it with a reader of its lines. Canceling ctx disconnects.
func openStream(ctx context.Context, t *testing.T, srv *httptest.Server, token string) (*http.Request, *http.Response, *bufio.Reader) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+clientapi.BasePath+"/streaming", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /streaming: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /streaming = %d, want 200", res.StatusCode)
	}
	return req, res, bufio.NewReader(res.Body)
}

// readLine returns the next non-empty line of a stream, failing t after a second.
func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			line, err := r.ReadString('\n')
			if line = strings.TrimSuffix(line, "\n"); line != "" || err != nil {
				ch <- result{line, err}
				return
			}
		}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("read the stream: %v", res.err)
		}
		return res.line
	case <-time.After(time.Second):
		t.Fatal("no line on the stream within a second")
		return ""
	}
}

// waitEnd fails t unless the stream ends within a second.
func waitEnd(t *testing.T, r *bufio.Reader) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, r)
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the stream did not end within a second")
	}
}

// waitReleased fails t unless h holds no subscription within a second.
func waitReleased(t *testing.T, h *stream.Hub) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for h.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d subscriptions still open", h.Len())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStreamingHeartbeatAndTokenCheck(t *testing.T) {
	hub := stream.NewHub(stream.HubOptions{})
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Streams, o.StreamHeartbeat = hub, 20*time.Millisecond
	})
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)

	req, res, r := openStream(t.Context(), t, srv, "read")
	for k, want := range map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-store"} {
		if got := res.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if line := readLine(t, r); line != ": connected" {
		t.Errorf("first line = %q, want the connected comment", line)
	}
	if line := readLine(t, r); line != ": heartbeat" {
		t.Errorf("line = %q, want a heartbeat", line)
	}
	if hub.Len() != 1 {
		t.Errorf("open subscriptions = %d, want 1", hub.Len())
	}

	// A heartbeat finds the token revoked and ends the stream.
	e.auth.mu.Lock()
	e.auth.errs["read"] = appidentity.ErrInvalidAccessToken
	e.auth.mu.Unlock()
	waitEnd(t, r)
	waitReleased(t, hub)
	e.spec.ValidateResponse(t, req, res.StatusCode, res.Header, nil)
}

func TestStreamingReleasesOnDisconnectAndShutdown(t *testing.T) {
	hub := stream.NewHub(stream.HubOptions{})
	m := testkit.NewMeter()
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Streams, o.Meter = hub, m })
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	user := map[string]string{"my.stream.kind": "user"}

	ctx, cancel := context.WithCancel(t.Context())
	_, _, r := openStream(ctx, t, srv, "read")
	readLine(t, r)
	if got := m.Sum("my.stream.connections", user); got != 1 {
		t.Errorf("my.stream.connections = %v with a stream open, want 1", got)
	}
	cancel()
	waitReleased(t, hub)
	if got := m.Sum("my.stream.connections", user); got != 0 {
		t.Errorf("my.stream.connections = %v after the stream ended, want 0", got)
	}
	if err := m.Err(); err != nil {
		t.Error(err)
	}

	_, _, r = openStream(t.Context(), t, srv, "read")
	readLine(t, r)
	hub.Close()
	waitEnd(t, r)
	waitReleased(t, hub)

	// A closed broker refuses new streams.
	res := e.get(t, "/streaming", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusServiceUnavailable || res.Header.Get("Retry-After") == "" {
		t.Errorf("after shutdown = %d (Retry-After %q), want 503 with Retry-After", res.StatusCode, res.Header.Get("Retry-After"))
	}
}

func TestStreamingBrokerUnavailable(t *testing.T) {
	broker := stream.NewMemoryBroker()
	broker.SetUnreachable(true)
	relay := stream.NewRelay(broker.Bus(), stream.RelayOptions{ReadyTimeout: 20 * time.Millisecond})
	t.Cleanup(func() { _ = relay.Close(context.Background()) })
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Streams = relay })

	res := e.get(t, "/streaming", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}
	if got := res.Header.Get("Retry-After"); got == "" || got == "0" {
		t.Errorf("Retry-After = %q, want seconds", got)
	}
	if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeBlank || res.Header.Get("Content-Language") != "ja" {
		t.Errorf("problem = %+v (%s), want about:blank in alice's locale", p, res.Header.Get("Content-Language"))
	}
	if got := relay.Len(); got != 0 {
		t.Errorf("open subscriptions = %d, want 0", got)
	}
	// The recovery of the broker is Relay's (stream.TestRelayUnavailable, on a fake clock).
}

func TestStreamingLimitsStreamsPerAccount(t *testing.T) {
	hub := stream.NewHub(stream.HubOptions{MaxPerAccount: 1})
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Streams = hub })
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)

	_, _, r := openStream(t.Context(), t, srv, "read")
	readLine(t, r)
	res := e.get(t, "/streaming", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second stream = %d, want 429", res.StatusCode)
	}
	p := decodeProblem(t, res)
	if p.Type != clientapi.ProblemTypeTooManyStreams || p.Detail == nil || res.Header.Get("Content-Language") != "ja" {
		t.Errorf("problem = %+v (%s), want too-many-streams in alice's locale", p, res.Header.Get("Content-Language"))
	}
	// Another account has its own streams.
	if _, _, r := openStream(t.Context(), t, srv, "bob"); readLine(t, r) != ": connected" {
		t.Error("bob's stream did not open")
	}
}

func TestStreamingRemembersRecentEntries(t *testing.T) {
	r := clientapi.NewRecentIDs(3)
	for _, id := range []string{"a", "b", "c", "a"} {
		r.Add(id)
	}
	for _, id := range []string{"a", "b", "c"} {
		if !r.Has(id) {
			t.Errorf("%s forgotten within the capacity", id)
		}
	}
	r.Add("d") // forgets the oldest, a
	r.Add("e") // and b
	for id, want := range map[string]bool{"a": false, "b": false, "c": true, "d": true, "e": true} {
		if got := r.Has(id); got != want {
			t.Errorf("Has(%s) = %v, want %v", id, got, want)
		}
	}
}

func TestStreamingWithoutBroker(t *testing.T) {
	e := newAuthEnv(t, nil)
	res := e.get(t, "/streaming", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", res.StatusCode)
	}
}

// TestStreamingNotificationBurst checks that a stream sends every notification of a burst
// larger than a page of the list, oldest first, and each once.
func TestStreamingNotificationBurst(t *testing.T) {
	hub := stream.NewHub(stream.HubOptions{})
	e, f, _ := newNotificationEnvWithViews(t, func(o *clientapi.Options) { o.Streams = hub })
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)

	_, _, r := openStream(t.Context(), t, srv, "read")
	if line := readLine(t, r); line != ": connected" {
		t.Fatalf("first line = %q, want the connected comment", line)
	}
	// IDs after the time the stream opened, growing with i.
	burstID := func(i int) string { return fmt.Sprintf("ffff0000-0000-7000-8000-%012d", i) }
	add := func(i int) {
		f.ns = append(f.ns, notification.Notification{
			ID: burstID(i), RecipientAccountID: aliceAccountID, Type: notification.TypeFollow, SourceAccountID: bobAccountID,
			CreatedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC),
		})
	}
	expect := func(i int) {
		t.Helper()
		if line := readLine(t, r); line != "event: notification" {
			t.Fatalf("line = %q, want notification %d", line, i)
		}
		var n clientapi.Notification
		if err := json.Unmarshal([]byte(strings.TrimPrefix(readLine(t, r), "data: ")), &n); err != nil {
			t.Fatal(err)
		}
		if n.Id != burstID(i) {
			t.Fatalf("notification %d = %s, want %s", i, n.Id, burstID(i))
		}
	}
	burst := appnotification.MaxPageLimit + 1
	for i := range burst {
		add(i)
	}
	hub.Publish(stream.Event{Kind: stream.KindNotifications, AccountID: aliceAccountID})
	for i := range burst {
		expect(i)
	}
	// The next signal sends only the notification after the burst.
	add(burst)
	hub.Publish(stream.Event{Kind: stream.KindNotifications, AccountID: aliceAccountID})
	expect(burst)
}
