// SPDX-License-Identifier: MIT

package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/interface/clientapi/apitest"

	"github.com/mycroblog/my/pkg/mybot"
	"github.com/mycroblog/my/pkg/mybot/mybottest"
	"github.com/mycroblog/my/pkg/mykit"
)

// specCheck is a transport that checks every request of the bot and every answer of the server against
// api/openapi.yaml, the events of the stream against their schemas, and counts the operations it saw.
type specCheck struct {
	t    *testing.T
	spec *apitest.Spec

	mu     sync.Mutex
	seen   map[string]int // "METHOD /path pattern" -> answers
	events map[string]int // event name -> events
}

// streamSchemas are the schemas of the data of the stream's events (streamEvents in api/openapi.yaml).
var streamSchemas = map[string]string{"notification": "Notification", "update": "Status"}

func (c *specCheck) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		body = b
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	check := req.Clone(req.Context())
	check.Body = io.NopCloser(bytes.NewReader(body))
	c.spec.ValidateRequest(c.t, check)

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	check = req.Clone(req.Context())
	check.Body = io.NopCloser(bytes.NewReader(body))
	c.count(c.seen, req.Method+" "+operationPath(req.URL.Path))
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		// The body of a stream never ends: check the answer without it, and each event as it arrives.
		c.spec.ValidateResponse(c.t, check, resp.StatusCode, resp.Header, nil)
		resp.Body = &teeBody{body: resp.Body, ev: &eventReader{c: c}}
		return resp, nil
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	c.spec.ValidateResponse(c.t, check, resp.StatusCode, resp.Header, b)
	resp.Body = io.NopCloser(bytes.NewReader(b))
	return resp, nil
}

// eventReader checks the data of each event of a stream against the schema of its event. It is fed by
// the body that the bot reads, so an event is counted before the bytes that complete it reach the bot.
type eventReader struct {
	c          *specCheck
	buf        []byte
	name, data string
}

func (e *eventReader) write(p []byte) {
	e.buf = append(e.buf, p...)
	for {
		i := bytes.IndexByte(e.buf, '\n')
		if i < 0 {
			return
		}
		line := strings.TrimSuffix(string(e.buf[:i]), "\r")
		e.buf = e.buf[i+1:]
		e.line(line)
	}
}

func (e *eventReader) line(line string) {
	switch {
	case line == "":
		if schema, ok := streamSchemas[e.name]; ok {
			e.c.spec.ValidateSchema(e.c.t, schema, []byte(e.data))
			e.c.count(e.c.events, e.name)
		} else if e.name != "" {
			e.c.t.Errorf("unknown event %q of the stream", e.name)
		}
		e.name, e.data = "", ""
	case strings.HasPrefix(line, "event: "):
		e.name = strings.TrimPrefix(line, "event: ")
	case strings.HasPrefix(line, "data: "):
		e.data += strings.TrimPrefix(line, "data: ")
	}
}

func (c *specCheck) count(m map[string]int, k string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m[k]++
}

func (c *specCheck) counts() (seen, events map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen, events = map[string]int{}, map[string]int{}
	for k, v := range c.seen {
		seen[k] = v
	}
	for k, v := range c.events {
		events[k] = v
	}
	return seen, events
}

// teeBody passes a stream to the client after eventReader has checked and counted what it carries.
type teeBody struct {
	body io.ReadCloser
	ev   *eventReader
}

func (b *teeBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	b.ev.write(p[:n])
	return n, err
}

func (b *teeBody) Close() error { return b.body.Close() }

// operationPath turns the IDs of a path into the parameters of its operation.
func operationPath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		if len(s) == 36 && strings.Count(s, "-") == 4 {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

// TestFakeServerConformsToSpec runs a bot against the fake server of mybottest through every operation that it
// serves, and checks each request, answer, and event of the stream against api/openapi.yaml.
func TestFakeServerConformsToSpec(t *testing.T) {
	t.Parallel()
	check := &specCheck{t: t, spec: apitest.Load(t), seen: map[string]int{}, events: map[string]int{}}
	srv := mybottest.NewServer()
	t.Cleanup(srv.Close)
	srv.SetBot(false) // updateAccountCredentials

	cfg := srv.Config()
	cfg.HTTPClient = &http.Client{Transport: check}
	cfg.SetBotFlag = true
	bot, err := mybot.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	handled := make(chan string, 64)
	bot.OnMention(func(c *mybot.Context) error {
		defer func() { handled <- c.Notification.Id }()
		if _, err := c.Reply("echo: " + c.Text); err != nil {
			return err
		}
		if err := c.React("👍"); err != nil {
			return err
		}
		if err := c.Favourite(); err != nil {
			return err
		}
		return c.Follow()
	})
	statuses := make(chan string, 4)
	bot.OnStatus(func(c *mybot.Context) error { statuses <- c.Status.Id; return nil })

	alice := srv.Account("alice")
	carol := srv.Account("carol@remote.example")
	// Notifications from before the start, over a page of the catch-up (40): listNotifications follows next_cursor.
	srv.InjectMissed(srv.Mention(carol, "missed", mykit.VisibilityPrivate))
	for range 45 {
		srv.InjectMissed(srv.Notification(mykit.NotificationTypeFollow, carol, nil))
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()
	wait(t, ctx, handled)
	if err := srv.WaitStreams(ctx, 1); err != nil {
		t.Fatal(err)
	}
	srv.Inject(srv.Mention(alice, "hello", mykit.VisibilityPublic))
	wait(t, ctx, handled)
	// A mention while the stream is down comes by the catch-up after the reconnection.
	srv.Disconnect()
	srv.Inject(srv.Mention(alice, "again", mykit.VisibilityDirect))
	srv.Reconnect()
	wait(t, ctx, handled)
	st := srv.Status(alice, "a post of the home timeline", mykit.VisibilityPublic)
	if err := srv.WaitStreams(ctx, 1); err != nil {
		t.Fatal(err)
	}
	srv.InjectStatus(st)
	wait(t, ctx, statuses)

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}

	// A request without an access token: the problem of the default answer.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL()+"/api/v1/accounts/verify_credentials", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&http.Client{Transport: check}).Do(req); err != nil {
		t.Fatal(err)
	}

	seen, events := check.counts()
	for _, op := range []string{
		"GET /api/v1/accounts/verify_credentials",
		"PATCH /api/v1/accounts/update_credentials",
		"GET /api/v1/notifications",
		"GET /api/v1/streaming",
		"POST /api/v1/statuses",
		"PUT /api/v1/statuses/{id}/reaction",
		"POST /api/v1/statuses/{id}/favourite",
		"POST /api/v1/accounts/{id}/follow",
	} {
		if seen[op] == 0 {
			t.Errorf("the bot never called %s; seen %v", op, seen)
		}
	}
	if seen["GET /api/v1/notifications"] < 2 {
		t.Errorf("listNotifications answered %d times, want the pages of two catch-ups", seen["GET /api/v1/notifications"])
	}
	for _, e := range []string{"notification", "update"} {
		if events[e] == 0 {
			t.Errorf("no %s event checked; seen %v", e, events)
		}
	}
}

func wait[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-ctx.Done():
		t.Fatal("timed out")
		var zero T
		return zero
	}
}
