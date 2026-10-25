// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// fakeClock is a Clock that moves when the test advances it.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []clockWaiter
}

type clockWaiter struct {
	until time.Time
	ch    chan struct{}
}

func newFakeClock(t *testing.T, at string) *fakeClock { return &fakeClock{now: mustTime(t, at)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	c.mu.Lock()
	w := clockWaiter{until: c.now.Add(d), ch: make(chan struct{})}
	c.waiters = append(c.waiters, w)
	c.mu.Unlock()
	select {
	case <-w.ch:
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		defer c.mu.Unlock()
		for i, o := range c.waiters {
			if o.ch == w.ch {
				c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
				break
			}
		}
		return ctx.Err()
	}
}

// Advance moves the clock on by d and wakes the sleepers whose time came.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if w.until.After(c.now) {
			kept = append(kept, w)
		} else {
			close(w.ch)
		}
	}
	c.waiters = kept
}

// sleepers is how many goroutines sleep.
func (c *fakeClock) sleepers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

// newScheduledBot is a bot of f on clock with a job "greet" of expr that posts "hello".
func newScheduledBot(t *testing.T, f *fakeServer, clock Clock, store Store, expr string, opts EveryOptions) (*Bot, *[]time.Time) {
	t.Helper()
	cfg := f.config()
	cfg.Clock = clock
	cfg.Store = store
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	runs := &[]time.Time{}
	if opts.Name == "" {
		opts.Name = "greet"
	}
	if err := bot.Every(expr, func(c *Context) error {
		mu.Lock()
		*runs = append(*runs, c.ScheduledAt)
		mu.Unlock()
		_, err := c.Post("hello")
		return err
	}, opts); err != nil {
		t.Fatal(err)
	}
	return bot, runs
}

// With CatchUp Latest, of the times missed while the bot was stopped, the newest alone runs at the start; with
// the default Skip, none does.
func TestEveryCatchUpLatestRunsOnce(t *testing.T) {
	for _, tc := range []struct {
		name    string
		catchUp CatchUp
		want    []string
	}{
		{"Latest", Latest, []string{"2026-10-25T09:00:00Z"}},
		{"Skip", Skip, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeServer(t)
			clock := newFakeClock(t, "2026-10-25T12:00:00Z")
			store := NewMemoryStore()
			// The last run was three days ago: three times were missed.
			if err := store.SetLastRun(context.Background(), "greet", mustTime(t, "2026-10-22T09:00:00Z")); err != nil {
				t.Fatal(err)
			}
			bot, runs := newScheduledBot(t, f, clock, store, "0 9 * * *", EveryOptions{CatchUp: tc.catchUp})
			stop := runBot(t, bot)
			// The poll, the prune, and the job sleep once the start is over.
			eventually(t, "the bot to sleep", func() bool { return clock.sleepers() >= 3 })
			if tc.want != nil {
				eventually(t, "the catch-up run recorded", func() bool {
					at, _ := store.LastRun(context.Background(), "greet")
					return at.Equal(mustTime(t, tc.want[0]))
				})
			}
			time.Sleep(50 * time.Millisecond)
			if err := stop(); err != nil {
				t.Fatal(err)
			}
			if len(*runs) != len(tc.want) {
				t.Fatalf("runs %v, want %v", *runs, tc.want)
			}
			posts := f.postsSoFar()
			if len(posts) != len(tc.want) {
				t.Fatalf("posted %d times, want %d", len(posts), len(tc.want))
			}
			for i, w := range tc.want {
				if got := (*runs)[i].UTC().Format(time.RFC3339); got != w {
					t.Errorf("ran %s, want %s", got, w)
				}
				if want := "mybot:every:greet:" + w; posts[i].key != want {
					t.Errorf("Idempotency-Key %q, want %q", posts[i].key, want)
				}
				if posts[i].req.Visibility != mykit.VisibilityUnlisted {
					t.Errorf("visibility %s, want unlisted", posts[i].req.Visibility)
				}
			}
		})
	}
}

// Two processes of one bot that post at the same time send the same Idempotency-Key, so that the server keeps one
// post; the clock reaching the time runs the job, and the next time follows.
func TestEveryTwoProcessesSendTheSameKey(t *testing.T) {
	f := newFakeServer(t)
	clock := newFakeClock(t, "2026-10-25T08:59:30Z")
	var bots []*Bot
	for range 2 {
		b, _ := newScheduledBot(t, f, clock, NewMemoryStore(), "0 9 * * *", EveryOptions{TimeZone: "Asia/Tokyo", Visibility: mykit.VisibilityPublic})
		bots = append(bots, b)
	}
	for _, b := range bots {
		runBot(t, b)
	}
	eventually(t, "the bots to sleep", func() bool { return clock.sleepers() >= 6 })
	// 09:00 in Tokyo is 00:00 UTC of the next day.
	clock.Advance(15*time.Hour + time.Minute)
	eventually(t, "two posts", func() bool { return len(f.postsSoFar()) == 2 })
	posts := f.postsSoFar()
	want := "mybot:every:greet:2026-10-26T00:00:00Z"
	for _, p := range posts {
		if p.key != want {
			t.Errorf("Idempotency-Key %q, want %q", p.key, want)
		}
		if p.req.Visibility != mykit.VisibilityPublic {
			t.Errorf("visibility %s, want public", p.req.Visibility)
		}
	}
	// The next day's time runs next.
	eventually(t, "the jobs to sleep again", func() bool { return clock.sleepers() >= 6 })
	clock.Advance(24 * time.Hour)
	eventually(t, "four posts", func() bool { return len(f.postsSoFar()) == 4 })
	if got := f.postsSoFar()[3].key; got != "mybot:every:greet:2026-10-27T00:00:00Z" {
		t.Errorf("the next key %q", got)
	}
}

// RunOnce handles the notifications that came and the scheduled jobs that are due, once, and returns; a second
// RunOnce on the same Store has nothing to do.
func TestRunOnce(t *testing.T) {
	f := newFakeServer(t)
	alice := account(1, "alice")
	m := mention(10, alice, "ping", mykit.VisibilityPublic)
	f.add(m)
	clock := newFakeClock(t, time.Now().UTC().Format(time.RFC3339))
	store := NewMemoryStore()
	newBot := func() (*Bot, *[]time.Time, *int) {
		bot, runs := newScheduledBot(t, f, clock, store, "*/5 * * * *", EveryOptions{})
		mentions := new(int)
		bot.OnMention(func(c *Context) error {
			*mentions++
			_, err := c.Reply("pong")
			return err
		})
		// A job of the default CatchUp runs in RunOnce too.
		if err := bot.Every("0 0 1 1 *", func(*Context) error { return nil }, EveryOptions{Name: "new year"}); err != nil {
			t.Fatal(err)
		}
		return bot, runs, mentions
	}

	bot, runs, mentions := newBot()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := bot.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if *mentions != 1 || !processed(store, m.Id) {
		t.Errorf("handled the mention %d times (processed %v), want once", *mentions, processed(store, m.Id))
	}
	if len(*runs) != 1 {
		t.Fatalf("ran the job %d times, want once", len(*runs))
	}
	if last, _ := store.LastRun(context.Background(), "greet"); !last.Equal((*runs)[0]) {
		t.Errorf("the last run %s, want %s", last, (*runs)[0])
	}
	if last, _ := store.LastRun(context.Background(), "new year"); last.IsZero() {
		t.Error("the job of CatchUp Skip did not run in RunOnce")
	}
	if n := len(f.postsSoFar()); n != 2 {
		t.Errorf("posted %d times, want the reply and the scheduled post", n)
	}

	again, runs2, mentions2 := newBot()
	if err := again.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if *mentions2 != 0 || len(*runs2) != 0 {
		t.Errorf("the second RunOnce handled %d mentions and %d runs, want none", *mentions2, len(*runs2))
	}
	if err := again.RunOnce(ctx); !errors.Is(err, ErrRunning) {
		t.Errorf("a second RunOnce of one bot: %v, want ErrRunning", err)
	}
}

// CatchUpLimit zero turns the catch-up of the notifications off (ADR-0094 §3).
func TestCatchUpLimitZeroTurnsTheCatchUpOff(t *testing.T) {
	f := newFakeServer(t)
	f.add(mention(10, account(1, "alice"), "ping", mykit.VisibilityPublic))
	cfg := f.config()
	cfg.CatchUpLimit = new(time.Duration(0))
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	handled := 0
	bot.OnMention(func(*Context) error { handled++; return nil })
	if err := bot.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if handled != 0 {
		t.Errorf("handled %d mentions with the catch-up off", handled)
	}
}

func TestEveryRefuses(t *testing.T) {
	f := newFakeServer(t)
	bot, err := New(f.config())
	if err != nil {
		t.Fatal(err)
	}
	h := func(*Context) error { return nil }
	if err := bot.Every("0 9 * * *", h, EveryOptions{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		expr string
		opts EveryOptions
	}{
		"a name in use":       {"0 10 * * *", EveryOptions{Name: "a"}},
		"seconds":             {"0 0 9 * * *", EveryOptions{}},
		"an alias":            {"@daily", EveryOptions{}},
		"an unknown zone":     {"0 9 * * *", EveryOptions{TimeZone: "Nowhere/Else"}},
		"a direct visibility": {"0 9 * * *", EveryOptions{Name: "b", Visibility: mykit.VisibilityDirect}},
		"an unknown CatchUp":  {"0 9 * * *", EveryOptions{Name: "c", CatchUp: CatchUp(9)}},
	} {
		if err := bot.Every(tc.expr, h, tc.opts); err == nil {
			t.Errorf("Every took %s", name)
		}
	}
}

func TestPostOutsideAScheduledJob(t *testing.T) {
	c := &Context{Context: context.Background()}
	if _, err := c.Post("x"); !errors.Is(err, ErrNotScheduled) {
		t.Errorf("Post: %v, want ErrNotScheduled", err)
	}
}

func TestEveryIdempotencyKey(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if got := EveryIdempotencyKey("greet", time.Date(2026, 10, 26, 9, 0, 0, 0, tokyo)); got != "mybot:every:greet:2026-10-26T00:00:00Z" {
		t.Errorf("key %q", got)
	}
}
