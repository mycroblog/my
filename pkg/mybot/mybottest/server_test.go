// SPDX-License-Identifier: MIT

package mybottest_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mybot"
	"github.com/mycroblog/my/pkg/mybot/mybottest"
	"github.com/mycroblog/my/pkg/mykit"
)

// run runs bot until the test ends, and returns a function that stops it and returns the error of Run.
func run(t *testing.T, bot *mybot.Bot) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()
	stopped := false
	var err error
	stop := func() error {
		if !stopped {
			stopped = true
			cancel()
			select {
			case err = <-done:
			case <-time.After(15 * time.Second):
				t.Fatal("Run did not return")
			}
		}
		return err
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

// botLog is a slog.Handler that records the bot's log at every level, for a test to wait for the debug lines
// that mybot.Config.Logger documents. It drops the attributes of WithAttrs and the groups.
type botLog struct {
	mu    sync.Mutex
	lines []slog.Record
}

func (l *botLog) Enabled(context.Context, slog.Level) bool { return true }

func (l *botLog) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, r.Clone())
	return nil
}

func (l *botLog) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *botLog) WithGroup(string) slog.Handler      { return l }

func (l *botLog) has(msg, key, value string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.lines {
		found := false
		r.Attrs(func(a slog.Attr) bool {
			found = a.Key == key && a.Value.String() == value
			return !found
		})
		if r.Message == msg && found {
			return true
		}
	}
	return false
}

// wait waits for a line with the message msg whose attribute key is value, failing t when ctx ends first.
func (l *botLog) wait(ctx context.Context, t *testing.T, msg, key, value string) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !l.has(msg, key, value) {
		select {
		case <-ctx.Done():
			t.Fatalf("the bot did not log %q with %s=%s", msg, key, value)
		case <-tick.C:
		}
	}
}

func newServer(t *testing.T) *mybottest.Server {
	t.Helper()
	srv := mybottest.NewServer()
	t.Cleanup(srv.Close)
	return srv
}

func TestNewServerClock(t *testing.T) {
	before := time.Now().Add(-time.Second)
	if now := newServer(t).Clock().Now(); now.Before(before) || now.After(time.Now()) {
		t.Errorf("Clock().Now() = %s, want the current time", now)
	}

	// A time in the past stays settable on any day.
	at := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	srv := mybottest.NewServer(mybottest.WithClockAt(at))
	t.Cleanup(srv.Close)
	if now := srv.Clock().Now(); !now.Equal(at) {
		t.Errorf("Clock().Now() = %s, want %s", now, at)
	}
	if created := srv.Me().CreatedAt; !created.Equal(at) {
		t.Errorf("Me().CreatedAt = %s, want %s", created, at)
	}
	srv.Clock().Set(at.Add(time.Hour))
	if now := srv.Clock().Now(); !now.Equal(at.Add(time.Hour)) {
		t.Errorf("Clock().Now() after Set = %s, want %s", now, at.Add(time.Hour))
	}
}

func waitCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// A notification that comes while the stream is down reaches the bot by its catch-up after it connects again,
// and is answered once.
func TestDisconnectCatchUp(t *testing.T) {
	srv := newServer(t)
	cfg := srv.Config()
	log := &botLog{}
	cfg.Logger = slog.New(log)
	bot, err := mybot.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	bot.OnMention(func(c *mybot.Context) error {
		_, err := c.Reply("seen")
		return err
	})
	run(t, bot)
	ctx := waitCtx(t)
	alice := srv.Account("alice")
	srv.Inject(srv.Mention(alice, "one", mykit.VisibilityPublic))
	if _, err := srv.WaitPosted(ctx, 1); err != nil {
		t.Fatal(err)
	}

	srv.Disconnect()
	missed := srv.Inject(srv.Mention(alice, "two", mykit.VisibilityPublic))
	if err := srv.WaitHeld(ctx, 1); err != nil {
		t.Fatalf("the bot's stream was not held: %v", err)
	}
	if n := len(srv.Posted()); n != 1 {
		t.Fatalf("posts while disconnected = %d, want 1", n)
	}
	srv.Reconnect()
	posts, err := srv.WaitPosted(ctx, 2)
	if err != nil {
		t.Fatalf("no reply to the missed mention: %v", err)
	}
	if got, want := posts[1].IdempotencyKey, mybot.ReplyIdempotencyKey(missed.Id, 1); got != want {
		t.Errorf("reply key = %q, want %q", got, want)
	}
	if err := srv.WaitStreams(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// The first mention is answered once all the same: once the catch-up after the reconnection is over,
	// nothing more is handed out.
	log.wait(ctx, t, "mybot: every notification handed out is processed", "position", missed.Id)
	if n := len(srv.Posted()); n != 2 {
		t.Errorf("posts = %d, want 2", n)
	}
}

// An event stream that the bot sees open is subscribed already: a notification added the moment it opens, after
// the catch-up has read the list, comes on the stream, however late the server goroutine runs.
func TestStreamIsSubscribedWhenItOpens(t *testing.T) {
	srv := newServer(t)
	// The server goroutine of the stream goes on only once the bot has seen the stream open and the notification
	// was added.
	added := make(chan struct{})
	mybottest.SetOpened(srv, func() {
		select {
		case <-added:
		case <-t.Context().Done():
		}
	})
	api, err := mykit.New(mykit.Config{Server: srv.URL(), TokenSource: mykit.StaticToken(mybottest.Token)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var injected mykit.Notification
	got := make(chan string, 1)
	var once sync.Once
	err = api.Stream(ctx, mykit.StreamOptions{
		OnOpen: func(bool) {
			injected = srv.Inject(srv.Mention(srv.Account("alice"), "now", mykit.VisibilityPublic))
			once.Do(func() { close(added) })
		},
		OnEvent: func(e mykit.StreamEvent) {
			if n, ok := e.(*mykit.NotificationEvent); ok {
				got <- n.Notification.Id
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case id := <-got:
		if id != injected.Id {
			t.Errorf("stream sent %s, want %s", id, injected.Id)
		}
	default:
		t.Fatal("the notification added when the stream opened did not come on it")
	}
}

// The server posts once per Idempotency-Key, as the real one does.
func TestCreateStatusIdempotencyKey(t *testing.T) {
	srv := newServer(t)
	api, err := mykit.New(mykit.Config{Server: srv.URL(), TokenSource: mykit.StaticToken(mybottest.Token)})
	if err != nil {
		t.Fatal(err)
	}
	key := "k1"
	var ids []string
	for range 2 {
		r, err := api.CreateStatusWithResponse(t.Context(), &mykit.CreateStatusParams{IdempotencyKey: &key},
			mykit.CreateStatusRequest{Text: "hi", Visibility: mykit.VisibilityUnlisted})
		if err != nil || r.JSON201 == nil {
			t.Fatalf("createStatus = %v %v", r, err)
		}
		ids = append(ids, r.JSON201.Id)
	}
	if ids[0] != ids[1] {
		t.Errorf("the second request with the key posted %s, want the first post %s", ids[1], ids[0])
	}
	if n := len(srv.Posted()); n != 1 {
		t.Errorf("posts = %d, want 1", n)
	}

	_, err = api.CreateStatusWithResponse(t.Context(), &mykit.CreateStatusParams{}, mykit.CreateStatusRequest{Text: "no visibility"})
	if p, ok := errors.AsType[*mykit.Problem](err); !ok || p.Status != http.StatusUnprocessableEntity {
		t.Errorf("createStatus without visibility = %v, want a 422 problem", err)
	}
}

// The helpers of the Context reach the server: React, Favourite, and Follow.
func TestHelpers(t *testing.T) {
	srv := newServer(t)
	bot, err := mybot.New(srv.Config())
	if err != nil {
		t.Fatal(err)
	}
	bot.OnMention(func(c *mybot.Context) error {
		if err := c.React("👍"); err != nil {
			return err
		}
		if err := c.Favourite(); err != nil {
			return err
		}
		return c.Follow()
	})
	run(t, bot)
	alice := srv.Account("alice")
	n := srv.Inject(srv.Mention(alice, "hello", mykit.VisibilityPublic))
	ctx := waitCtx(t)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for len(srv.Follows()) == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("the bot did not follow")
		case <-tick.C:
		}
	}
	if got, want := srv.Reactions(), []mybottest.Reaction{{StatusID: n.Status.Id, Emoji: "👍"}}; !slices.Equal(got, want) {
		t.Errorf("reactions = %v, want %v", got, want)
	}
	if got := srv.Favourites(); !slices.Equal(got, []string{n.Status.Id}) {
		t.Errorf("favourites = %v, want %s", got, n.Status.Id)
	}
	if got := srv.Follows(); !slices.Equal(got, []string{alice.Id}) {
		t.Errorf("follows = %v, want %s", got, alice.Id)
	}
}

// Without the bot flag, the bot refuses to start, or sets it with SetBotFlag.
func TestBotFlag(t *testing.T) {
	srv := newServer(t)
	srv.SetBot(false)
	bot, err := mybot.New(srv.Config())
	if err != nil {
		t.Fatal(err)
	}
	if err := bot.Run(t.Context()); !errors.Is(err, mybot.ErrNotBot) {
		t.Fatalf("Run = %v, want ErrNotBot", err)
	}

	cfg := srv.Config()
	cfg.SetBotFlag = true
	bot, err = mybot.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stop := run(t, bot)
	ctx := waitCtx(t)
	if err := srv.WaitStreams(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if !srv.Me().Bot {
		t.Error("the bot flag is off after SetBotFlag")
	}
}

// A request without an access token is a 401 problem, and an operation that the fake does not serve a 404.
func TestProblems(t *testing.T) {
	srv := newServer(t)
	for _, tc := range []struct {
		auth, path string
		want       int
	}{
		{"", "/api/v1/accounts/verify_credentials", http.StatusUnauthorized},
		{"Bearer " + mybottest.Token, "/api/v1/timelines/home", http.StatusNotFound},
	} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL()+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		resp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want || resp.Header.Get("Content-Type") != "application/problem+json" {
			t.Errorf("GET %s = %d %s, want %d application/problem+json", tc.path, resp.StatusCode, resp.Header.Get("Content-Type"), tc.want)
		}
	}
}
