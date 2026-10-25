// SPDX-License-Identifier: MIT

package integration_test

import (
	"bufio"
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
		pr, pw := io.Pipe()
		go c.readEvents(pr)
		resp.Body = &teeBody{r: io.TeeReader(resp.Body, pw), c: resp.Body, w: pw}
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

// readEvents checks the data of each event of a stream against the schema of its event.
func (c *specCheck) readEvents(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var name, data string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if schema, ok := streamSchemas[name]; ok {
				c.spec.ValidateSchema(c.t, schema, []byte(data))
				c.count(c.events, name)
			} else if name != "" {
				c.t.Errorf("unknown event %q of the stream", name)
			}
			name, data = "", ""
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data += strings.TrimPrefix(line, "data: ")
		}
	}
	_, _ = io.Copy(io.Discard, r)
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

// teeBody passes a stream to the client and to readEvents.
type teeBody struct {
	r io.Reader
	c io.Closer
	w *io.PipeWriter
}

func (b *teeBody) Read(p []byte) (int, error) { return b.r.Read(p) }

func (b *teeBody) Close() error {
	_ = b.w.Close()
	return b.c.Close()
}

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
