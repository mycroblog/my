// SPDX-License-Identifier: MIT

package mybottest_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
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

func newServer(t *testing.T) *mybottest.Server {
	t.Helper()
	srv := mybottest.NewServer()
	t.Cleanup(srv.Close)
	return srv
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
	bot, err := mybot.New(srv.Config())
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
	time.Sleep(50 * time.Millisecond)
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
	// The catch-up after the reconnection lists the first mention again; it is answered once all the same.
	time.Sleep(100 * time.Millisecond)
	if n := len(srv.Posted()); n != 2 {
		t.Errorf("posts = %d, want 2", n)
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
		if err != nil || r.JSON200 == nil {
			t.Fatalf("createStatus = %v %v", r, err)
		}
		ids = append(ids, r.JSON200.Id)
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
	for len(srv.Follows()) == 0 {
		if ctx.Err() != nil {
			t.Fatal("the bot did not follow")
		}
		time.Sleep(10 * time.Millisecond)
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
