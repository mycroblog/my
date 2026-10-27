// SPDX-License-Identifier: AGPL-3.0-only

package sentry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/errorreport/sentry"
	"github.com/mycroblog/my/internal/app/kernel"
)

var update = flag.Bool("update", false, "rewrite testdata/envelope.golden")

// stack is a debug.Stack() of a panic in a handler, as -trimpath builds print it, after the
// reporter's scrubbing replaced the argument lists.
const stack = `goroutine 42 [running]:
github.com/mycroblog/my/internal/app/status.(*Service).Get(...)
	github.com/mycroblog/my/internal/app/status/get.go:42 +0x32
github.com/mycroblog/my/internal/interface/clientapi.(*Server).getStatus(...)
	github.com/mycroblog/my/internal/interface/clientapi/statuses.go:120 +0x1a4
net/http.HandlerFunc.ServeHTTP(...)
	net/http/server.go:2294 +0x29
created by net/http.(*Server).Serve in goroutine 1
	net/http/server.go:3454 +0x485
`

// event is the ErrorEvent that the reporter hands to the exporter: filled and scrubbed.
func event() kernel.ErrorEvent {
	return kernel.ErrorEvent{
		Time:   time.Date(2026, 10, 1, 12, 0, 0, 123_000_000, time.UTC),
		Source: kernel.ErrorSourceServer, Kind: kernel.ErrorKindPanic,
		Type:        "*fmt.wrapError > *pgconn.PgError",
		Message:     "status <id>: query failed",
		Stack:       stack,
		Where:       "client_api GET /api/v1/statuses/{id}",
		Fingerprint: "0123456789abcdef0123456789abcdef",
		RequestID:   "req-1", JobID: "", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7",
		Version: "1.2.3", Role: "web", Environment: "staging",
	}
}

// receiver is the httptest intake: it records the requests and answers with the next response.
type receiver struct {
	srv *httptest.Server

	mu        sync.Mutex
	requests  []request
	responses []response
}

type request struct {
	method, path string
	header       http.Header
	body         []byte
}

type response struct {
	status int
	header map[string]string
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.requests = append(r.requests, request{method: req.Method, path: req.URL.Path, header: req.Header.Clone(), body: body})
		resp := response{status: http.StatusOK}
		if len(r.responses) > 0 {
			resp, r.responses = r.responses[0], r.responses[1:]
		}
		r.mu.Unlock()
		for k, v := range resp.header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(`{"id":"0f1e2d3c4b5a69788796a5b4c3d2e1f0"}`))
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) respond(rs ...response) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses = append(r.responses, rs...)
}

func (r *receiver) got() []request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]request(nil), r.requests...)
}

// dsn returns the DSN of the receiver with the given path prefix and project.
func (r *receiver) dsn(prefix string) string {
	return strings.Replace(r.srv.URL, "http://", "http://public:secret@", 1) + prefix + "/17"
}

// clock is a time that the tests move.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newClient(t *testing.T, dsn string, c *clock) *sentry.Client {
	t.Helper()
	client, err := sentry.New(sentry.Options{
		DSN: dsn, Version: "1.2.3", Now: c.Now,
		EventID: func() string { return "9ec79c33ec9942ab8353589fcb2e04dc" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func newClock() *clock { return &clock{now: time.Date(2026, 10, 1, 12, 0, 1, 0, time.UTC)} }

func TestEnvelope(t *testing.T) {
	r := newReceiver(t)
	if err := newClient(t, r.dsn("/prefix"), newClock()).SendError(t.Context(), event()); err != nil {
		t.Fatal(err)
	}
	reqs := r.got()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	req := reqs[0]
	if req.method != http.MethodPost || req.path != "/prefix/api/17/envelope/" {
		t.Errorf("request = %s %s", req.method, req.path)
	}
	if got := req.header.Get("Content-Type"); got != "application/x-sentry-envelope" {
		t.Errorf("Content-Type = %q", got)
	}
	if got, want := req.header.Get("X-Sentry-Auth"), "Sentry sentry_version=7, sentry_client=my/1.2.3, sentry_key=public, sentry_secret=secret"; got != want {
		t.Errorf("X-Sentry-Auth = %q, want %q", got, want)
	}

	if *update {
		if err := os.WriteFile("testdata/envelope.golden", req.body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile("testdata/envelope.golden")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(req.body, golden) {
		t.Errorf("envelope =\n%s\nwant (testdata/envelope.golden)\n%s", req.body, golden)
	}

	// The envelope is three lines: its header, the item header with the event's length, the event.
	lines := bytes.Split(bytes.TrimSuffix(req.body, []byte("\n")), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("envelope has %d lines, want 3", len(lines))
	}
	var item struct {
		Type   string `json:"type"`
		Length int    `json:"length"`
	}
	if err := json.Unmarshal(lines[1], &item); err != nil || item.Type != "event" || item.Length != len(lines[2]) {
		t.Errorf("item header = %s (%v), event length %d", lines[1], err, len(lines[2]))
	}
	var ev map[string]any
	if err := json.Unmarshal(lines[2], &ev); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"server_name", "user", "request", "extra", "modules"} {
		if _, ok := ev[key]; ok {
			t.Errorf("the event has %q", key)
		}
	}
}

// TestNoHostDetails checks that no request carries the host's name or addresses.
func TestNoHostDetails(t *testing.T) {
	r := newReceiver(t)
	if err := newClient(t, r.dsn(""), newClock()).SendError(t.Context(), event()); err != nil {
		t.Fatal(err)
	}
	req := r.got()[0]
	var all bytes.Buffer
	all.Write(req.body)
	for k, vs := range req.header {
		all.WriteString(k + ": " + strings.Join(vs, ",") + "\n")
	}
	var needles []string
	if host, err := os.Hostname(); err == nil && host != "" {
		needles = append(needles, host)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ip, _, err := net.ParseCIDR(a.String()); err == nil && !ip.IsLoopback() {
				needles = append(needles, ip.String())
			}
		}
	}
	for _, n := range needles {
		if strings.Contains(all.String(), n) {
			t.Errorf("the request contains %q", n)
		}
	}
	if strings.Contains(all.String(), "req-1") {
		t.Error("the request contains the request ID")
	}
}

func TestFrames(t *testing.T) {
	r := newReceiver(t)
	if err := newClient(t, r.dsn(""), newClock()).SendError(t.Context(), event()); err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(r.got()[0].body, []byte("\n"))
	var ev struct {
		Exception struct {
			Values []struct {
				Type       string `json:"type"`
				Value      string `json:"value"`
				Stacktrace struct {
					Frames []struct {
						Function string `json:"function"`
						Module   string `json:"module"`
						Filename string `json:"filename"`
						Lineno   int    `json:"lineno"`
						InApp    bool   `json:"in_app"`
					} `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		} `json:"exception"`
	}
	if err := json.Unmarshal(lines[2], &ev); err != nil {
		t.Fatal(err)
	}
	ex := ev.Exception.Values[0]
	if ex.Type != "*pgconn.PgError" || ex.Value != "status <id>: query failed" {
		t.Errorf("exception = %s: %s", ex.Type, ex.Value)
	}
	type want struct {
		function, module, file string
		line                   int
		inApp                  bool
	}
	wants := []want{
		{"(*Server).Serve", "net/http", "net/http/server.go", 3454, false},
		{"HandlerFunc.ServeHTTP", "net/http", "net/http/server.go", 2294, false},
		{"(*Server).getStatus", "github.com/mycroblog/my/internal/interface/clientapi", "github.com/mycroblog/my/internal/interface/clientapi/statuses.go", 120, true},
		{"(*Service).Get", "github.com/mycroblog/my/internal/app/status", "github.com/mycroblog/my/internal/app/status/get.go", 42, true},
	}
	frames := ex.Stacktrace.Frames
	if len(frames) != len(wants) {
		t.Fatalf("frames = %+v", frames)
	}
	for i, w := range wants {
		f := frames[i]
		if f.Function != w.function || f.Module != w.module || f.Filename != w.file || f.Lineno != w.line || f.InApp != w.inApp {
			t.Errorf("frame %d = %+v, want %+v", i, f, w)
		}
	}
}

func TestLevelAndOptionalFields(t *testing.T) {
	r := newReceiver(t)
	ev := event()
	ev.Fatal, ev.TraceID, ev.SpanID, ev.Stack, ev.Type = true, "", "", "not a Go stack", ""
	if err := newClient(t, r.dsn(""), newClock()).SendError(t.Context(), ev); err != nil {
		t.Fatal(err)
	}
	body := string(r.got()[0].body)
	for _, s := range []string{`"level":"fatal"`, `"type":"error"`} {
		if !strings.Contains(body, s) {
			t.Errorf("the envelope lacks %s:\n%s", s, body)
		}
	}
	for _, s := range []string{`"contexts"`, `"stacktrace"`} {
		if strings.Contains(body, s) {
			t.Errorf("the envelope has %s:\n%s", s, body)
		}
	}
}

func TestParseDSN(t *testing.T) {
	d, err := sentry.ParseDSN("https://abc:def@o0.ingest.sentry.io/path/to/42")
	if err != nil {
		t.Fatal(err)
	}
	if d.PublicKey != "abc" || d.SecretKey != "def" || d.ProjectID != "42" || d.EnvelopeURL() != "https://o0.ingest.sentry.io/path/to/api/42/envelope/" {
		t.Errorf("DSN = %+v, %s", d, d.EnvelopeURL())
	}
	d, err = sentry.ParseDSN("https://token@sentry-intake.datadoghq.com/1")
	if err != nil || d.EnvelopeURL() != "https://sentry-intake.datadoghq.com/api/1/envelope/" || d.SecretKey != "" {
		t.Errorf("Datadog DSN = %+v, %v", d, err)
	}
	for _, bad := range []string{"", "ftp://k@h/1", "https://h/1", "https://k@/1", "https://k@h", "https://k@h/", "://"} {
		if _, err := sentry.ParseDSN(bad); err == nil {
			t.Errorf("ParseDSN(%q) = nil error", bad)
		} else if strings.Contains(err.Error(), "k@") {
			t.Errorf("the error of %q repeats the DSN: %v", bad, err)
		}
	}
}

func dropped(err error) bool {
	var d interface{ Dropped() bool }
	return errors.As(err, &d) && d.Dropped()
}

func TestRateLimits(t *testing.T) {
	tests := []struct {
		name string
		resp response
		// err is the outcome of the send that gets resp: "" for nil, "dropped", or "failed".
		err string
		// stop is how long the client stops after resp; 0 for not at all.
		stop time.Duration
	}{
		{name: "429 with Retry-After", resp: response{status: 429, header: map[string]string{"Retry-After": "30"}}, err: "dropped", stop: 30 * time.Second},
		{name: "429 with an HTTP date", resp: response{status: 429, header: map[string]string{"Retry-After": "Thu, 01 Oct 2026 12:00:21 GMT"}}, err: "dropped", stop: 20 * time.Second},
		{name: "429 with nothing", resp: response{status: 429}, err: "dropped", stop: time.Minute},
		{name: "429 with rate limits", resp: response{status: 429, header: map[string]string{"Retry-After": "5", "X-Sentry-Rate-Limits": "120:error:organization"}}, err: "dropped", stop: 2 * time.Minute},
		{name: "200 with an error limit", resp: response{status: 200, header: map[string]string{"X-Sentry-Rate-Limits": "60:transaction;error:project:quota_exceeded"}}, stop: time.Minute},
		{name: "200 with a limit of every category", resp: response{status: 200, header: map[string]string{"X-Sentry-Rate-Limits": "10::organization"}}, stop: 10 * time.Second},
		{name: "200 with the longest of several limits", resp: response{status: 200, header: map[string]string{"X-Sentry-Rate-Limits": "10:error:project, 90:default;error:organization"}}, stop: 90 * time.Second},
		{name: "200 with another category's limit", resp: response{status: 200, header: map[string]string{"X-Sentry-Rate-Limits": "60:transaction:organization"}}},
		{name: "500", resp: response{status: 500}, err: "failed"},
		{name: "400 with an error limit", resp: response{status: 400, header: map[string]string{"X-Sentry-Rate-Limits": "15:error:key"}}, err: "failed", stop: 15 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReceiver(t)
			c := newClock()
			client := newClient(t, r.dsn(""), c)
			r.respond(tt.resp)

			err := client.SendError(t.Context(), event())
			switch got := outcome(err); {
			case got != tt.err:
				t.Fatalf("first send = %v (%s), want %s", err, got, tt.err)
			case len(r.got()) != 1:
				t.Fatalf("got %d requests, want 1: the client never retries", len(r.got()))
			}

			if tt.stop > 0 {
				c.add(tt.stop - time.Second)
				if err := client.SendError(t.Context(), event()); !dropped(err) {
					t.Errorf("send within the limit = %v, want dropped", err)
				}
				if n := len(r.got()); n != 1 {
					t.Errorf("got %d requests within the limit, want 1", n)
				}
				c.add(time.Second)
			}
			if err := client.SendError(t.Context(), event()); err != nil {
				t.Errorf("send after the limit = %v", err)
			}
			if n := len(r.got()); n != 2 {
				t.Errorf("got %d requests, want 2", n)
			}
		})
	}
}

func outcome(err error) string {
	switch {
	case err == nil:
		return ""
	case dropped(err):
		return "dropped"
	default:
		return "failed"
	}
}

func TestSendFailureHidesTheDSN(t *testing.T) {
	r := newReceiver(t)
	dsn := r.dsn("")
	r.srv.Close()
	err := newClient(t, dsn, newClock()).SendError(context.Background(), event())
	if err == nil || dropped(err) {
		t.Fatalf("send to a closed receiver = %v, want a failure", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "public") {
		t.Errorf("the error repeats the DSN: %v", err)
	}
}
