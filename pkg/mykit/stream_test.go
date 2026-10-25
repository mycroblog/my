// SPDX-License-Identifier: MIT

package mykit

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// streamServer answers GET /api/v1/streaming with the handler of the n-th connection (from 0); connections
// past the last get the last handler. It records the headers of every request.
type streamServer struct {
	*httptest.Server
	mu      sync.Mutex
	headers []http.Header
}

func newStreamServer(t *testing.T, handlers ...func(w http.ResponseWriter, r *http.Request)) *streamServer {
	t.Helper()
	s := &streamServer{}
	var n atomic.Int32
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/streaming" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		s.mu.Lock()
		s.headers = append(s.headers, r.Header.Clone())
		s.mu.Unlock()
		i := min(int(n.Add(1))-1, len(handlers)-1)
		handlers[i](w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// sse writes an event stream: the chunks in order, flushed one by one. With hold it then waits for the client to
// go away instead of ending the stream.
func sse(hold bool, chunks ...string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f := w.(http.Flusher)
		for _, c := range chunks {
			_, _ = io.WriteString(w, c)
			f.Flush()
		}
		if hold {
			<-r.Context().Done()
		}
	}
}

// refusal answers with a problem of status and type, and Retry-After when it is not empty.
func refusal(status int, typ, retryAfter string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"type":"`+typ+`","title":"t","status":0}`)
	}
}

// streamRecorder collects what Stream reports, and ends the stream at the open number stopAt (from 0).
type streamRecorder struct {
	mu     sync.Mutex
	opens  []bool
	events []StreamEvent
	errs   []error
}

func (rec *streamRecorder) options(cancel context.CancelFunc, stopAt int) StreamOptions {
	return StreamOptions{
		OnOpen: func(reconnected bool) {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			rec.opens = append(rec.opens, reconnected)
			if len(rec.opens) > stopAt {
				cancel()
			}
		},
		OnEvent: func(e StreamEvent) {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			rec.events = append(rec.events, e)
		},
		OnError: func(err error) {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			rec.errs = append(rec.errs, err)
		},
	}
}

func runStream(t *testing.T, api *API, opts StreamOptions, ctx context.Context) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- api.Stream(ctx, opts) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Stream did not return")
		return nil
	}
}

func TestStreamDeliversTypedEventsAndReconnects(t *testing.T) {
	srv := newStreamServer(t,
		sse(false,
			": connected\n\n",
			"event: update\ndata: {\"id\":\"s1\"}\n\n",
			"event: notification\ndata: {\"id\":\"n1\",\"type\":\"follow\"}\n\n",
			": heartbeat\n\n",
			"event: announcement\ndata: {\"id\":\"a1\"}\n\n",
			"event: announcement_removed\ndata: {\"id\":\"a2\"}\n\n",
			"event: poll\ndata: {\"id\":\"p1\"}\ndata: more\n\n",
			"event: update\ndata: not json\n\n",
		),
		sse(true, ": connected\n\n"),
	)
	api, waits := newTestAPI(t, srv.Server, Config{TokenSource: StaticToken("a0")})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rec streamRecorder
	if err := runStream(t, api, rec.options(cancel, 1), ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream = %v, want context.Canceled", err)
	}

	if !slices.Equal(rec.opens, []bool{false, true}) {
		t.Errorf("opens = %v, want [false true]", rec.opens)
	}
	if len(rec.events) != 5 {
		t.Fatalf("events = %#v, want 5", rec.events)
	}
	if e, ok := rec.events[0].(*UpdateEvent); !ok || e.Status.Id != "s1" {
		t.Errorf("events[0] = %#v, want the update of s1", rec.events[0])
	}
	if e, ok := rec.events[1].(*NotificationEvent); !ok || e.Notification.Id != "n1" {
		t.Errorf("events[1] = %#v, want the notification n1", rec.events[1])
	}
	if e, ok := rec.events[2].(*AnnouncementEvent); !ok || e.Announcement.Id != "a1" {
		t.Errorf("events[2] = %#v, want the announcement a1", rec.events[2])
	}
	if e, ok := rec.events[3].(*AnnouncementRemovedEvent); !ok || e.ID != "a2" {
		t.Errorf("events[3] = %#v, want the removal of a2", rec.events[3])
	}
	if e, ok := rec.events[4].(*UnknownEvent); !ok || e.Name != "poll" || e.Data != "{\"id\":\"p1\"}\nmore" || e.EventName() != "poll" {
		t.Errorf("events[4] = %#v, want the unknown event poll", rec.events[4])
	}
	// The update whose data is no Status is skipped and reported, and the stream goes on.
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), `"update"`) {
		t.Errorf("errs = %v, want the unreadable update", rec.errs)
	}
	// A stream that ended normally waits about a second before connecting again.
	if len(*waits) != 1 || (*waits)[0] < 500*time.Millisecond || (*waits)[0] > time.Second {
		t.Errorf("waits = %v, want one of 0.5s to 1s", *waits)
	}
	for _, h := range srv.headers {
		if h.Get("Authorization") != "Bearer a0" || h.Get("Accept") != "text/event-stream" || h.Get("Last-Event-ID") != "" {
			t.Errorf("request headers = %v", h)
		}
	}
}

func TestStreamReconnectsWhenHeartbeatsStop(t *testing.T) {
	// The first stream says it is connected and then goes silent.
	srv := newStreamServer(t, sse(true, ": connected\n\n"), sse(true, ": connected\n\n"))
	api, _ := newTestAPI(t, srv.Server, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rec streamRecorder
	opts := rec.options(cancel, 1)
	opts.HeartbeatTimeout = 50 * time.Millisecond
	if err := runStream(t, api, opts, ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream = %v, want context.Canceled", err)
	}
	if !slices.Equal(rec.opens, []bool{false, true}) {
		t.Errorf("opens = %v, want [false true]", rec.opens)
	}
	if len(rec.errs) != 1 || !errors.Is(rec.errs[0], ErrHeartbeatTimeout) {
		t.Errorf("errs = %v, want ErrHeartbeatTimeout", rec.errs)
	}
}

func TestStreamKeepsAStreamWithHeartbeats(t *testing.T) {
	// Heartbeats more often than the timeout keep the stream, which then ends normally.
	srv := newStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		_, _ = io.WriteString(w, ": connected\n\n")
		f.Flush()
		for range 6 {
			time.Sleep(20 * time.Millisecond)
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			f.Flush()
		}
		_, _ = io.WriteString(w, "event: announcement_removed\ndata: {\"id\":\"a1\"}\n\n")
	}, sse(true, ": connected\n\n"))
	api, _ := newTestAPI(t, srv.Server, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rec streamRecorder
	opts := rec.options(cancel, 1)
	opts.HeartbeatTimeout = 300 * time.Millisecond
	if err := runStream(t, api, opts, ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream = %v, want context.Canceled", err)
	}
	if len(rec.errs) != 0 || len(rec.events) != 1 {
		t.Errorf("errs = %v, events = %v; want no error and the one event", rec.errs, rec.events)
	}
}

func TestStreamWaitsForRetryAfter(t *testing.T) {
	srv := newStreamServer(t,
		refusal(http.StatusTooManyRequests, "/api/v1/problems/too-many-streams", "7"),
		refusal(http.StatusServiceUnavailable, ProblemTypeAboutBlank, "5"),
		refusal(http.StatusBadGateway, ProblemTypeAboutBlank, ""),
		sse(true, ": connected\n\n"),
	)
	api, waits := newTestAPI(t, srv.Server, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rec streamRecorder
	if err := runStream(t, api, rec.options(cancel, 0), ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream = %v, want context.Canceled", err)
	}
	if len(*waits) != 3 || (*waits)[0] != 7*time.Second || (*waits)[1] != 5*time.Second {
		t.Fatalf("waits = %v, want 7s, 5s, then a backoff", *waits)
	}
	// The third failure in a row backs off from 8s, with jitter over its upper half.
	if w := (*waits)[2]; w < 4*time.Second || w > 8*time.Second {
		t.Errorf("backoff = %v, want 4s to 8s", w)
	}
	var p *Problem
	if len(rec.errs) != 3 || !errors.As(rec.errs[0], &p) || p.Type != "/api/v1/problems/too-many-streams" {
		t.Errorf("errs = %v, want the 429 first", rec.errs)
	}
	if !slices.Equal(rec.opens, []bool{false}) {
		t.Errorf("opens = %v, want [false]", rec.opens)
	}
}

// refreshingSource hands out a<n> and renews it to a<n+1> on Refresh.
type refreshingSource struct {
	mu        sync.Mutex
	n         int
	refreshes int
}

func (s *refreshingSource) Token(context.Context) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &Token{AccessToken: "a" + string(rune('0'+s.n))}, nil
}

func (s *refreshingSource) Refresh(context.Context, *Token) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	s.refreshes++
	return &Token{AccessToken: "a" + string(rune('0'+s.n))}, nil
}

func TestStreamRefreshesTheTokenOnceAfterA401(t *testing.T) {
	srv := newStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer a1" {
			refusal(http.StatusUnauthorized, "/api/v1/problems/invalid-token", "")(w, r)
			return
		}
		sse(true, ": connected\n\n")(w, r)
	})
	source := &refreshingSource{}
	api, _ := newTestAPI(t, srv.Server, Config{TokenSource: source})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rec streamRecorder
	if err := runStream(t, api, rec.options(cancel, 0), ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream = %v, want context.Canceled", err)
	}
	if source.refreshes != 1 || len(rec.errs) != 0 {
		t.Errorf("refreshes = %d, errs = %v; want one refresh and no error", source.refreshes, rec.errs)
	}
}

func TestStreamStopsWhenRefusedForGood(t *testing.T) {
	srv := newStreamServer(t, refusal(http.StatusForbidden, "/api/v1/problems/insufficient-scope", ""))
	api, waits := newTestAPI(t, srv.Server, Config{TokenSource: StaticToken("a0")})
	var rec streamRecorder
	err := runStream(t, api, rec.options(func() {}, 0), context.Background())
	var p *Problem
	if !errors.As(err, &p) || p.Status != http.StatusForbidden {
		t.Fatalf("Stream = %v, want the 403 problem", err)
	}
	if len(*waits) != 0 || len(rec.opens) != 0 {
		t.Errorf("waits = %v, opens = %v; want none", *waits, rec.opens)
	}
}

func TestStreamRetriesAnAnswerThatIsNoEventStream(t *testing.T) {
	srv := newStreamServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<p>maintenance</p>")
	}, sse(true, ": connected\n\n"))
	api, _ := newTestAPI(t, srv.Server, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rec streamRecorder
	if err := runStream(t, api, rec.options(cancel, 0), ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream = %v, want context.Canceled", err)
	}
	if len(rec.errs) != 1 || !errors.Is(rec.errs[0], errNotEventStream) || !slices.Equal(rec.opens, []bool{false}) {
		t.Errorf("errs = %v, opens = %v", rec.errs, rec.opens)
	}
}

func TestEventParserFollowsTheStandard(t *testing.T) {
	input := "\uFEFFevent: a\r\ndata:one\rdata:  two\n\n" + // CRLF, CR, a colon without a space, two data lines
		": comment\n" +
		"data\n\n" + // a field without a colon: empty data
		"event: b\nid: 7\nretry: 10\n\n" + // no data: not dispatched
		"data: x\n\n" +
		"data: unfinished\n" // dropped at the end
	sc := bufio.NewScanner(strings.NewReader(input))
	sc.Split(scanLines)
	var p eventParser
	var got []rawEvent
	for sc.Scan() {
		if ev, ok := p.line(sc.Text()); ok {
			got = append(got, ev)
		}
	}
	want := []rawEvent{{"a", "one\n two"}, {"message", ""}, {"message", "x"}}
	if !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestStreamDelayBacksOffWithJitter(t *testing.T) {
	for failures, limit := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
		for range 20 {
			if d := streamDelay(failures); d < limit/2 || d > limit {
				t.Fatalf("streamDelay(%d) = %v, want %v to %v", failures, d, limit/2, limit)
			}
		}
	}
	if d := streamDelay(30); d > time.Minute {
		t.Errorf("streamDelay(30) = %v, want at most a minute", d)
	}
}
