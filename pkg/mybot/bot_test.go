// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// The mentions that arrive while the event stream is down are caught up after the reconnection, and each mention
// reaches the handler once, although both the stream and the catch-up bring some of them.
func TestStreamCatchesUpAfterReconnectingOnce(t *testing.T) {
	f := newFakeServer(t)
	alice := account(1, "alice")
	a := mention(10, alice, "first", mykit.VisibilityPublic)
	b := mention(11, alice, "while the stream was down", mykit.VisibilityPublic)
	c := mention(12, alice, "after the reconnection", mykit.VisibilityPublic)

	var mu sync.Mutex
	seen := map[string]int{}
	handledA := make(chan struct{})
	f.stream = func(w http.ResponseWriter, r *http.Request, n int) {
		switch n {
		case 1:
			// Wait for the catch-up of the first connection to read the (empty) list, so that it misses b.
			eventually(t, "the first catch-up", func() bool {
				f.mu.Lock()
				defer f.mu.Unlock()
				return f.listCalls >= 1
			})
			writeEvent(w, "notification", a)
			<-handledA
			// The stream drops; b arrives before the next connection.
			f.add(a)
			f.add(b)
		default:
			// The next connection brings c on both paths, and a again.
			f.add(c)
			writeEvent(w, "notification", c)
			writeEvent(w, "notification", a)
			<-r.Context().Done()
		}
	}
	cfg := f.config()
	cfg.Mode = Streaming
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	bot.OnMention(func(ctx *Context) error {
		mu.Lock()
		seen[ctx.Notification.Id]++
		mu.Unlock()
		if ctx.Notification.Id == a.Id {
			once.Do(func() { close(handledA) })
		}
		return nil
	})
	stop := runBot(t, bot)
	eventually(t, "the three mentions processed", func() bool { return processed(cfg.Store, a.Id, b.Id, c.Id) })
	// Give a duplicate the time to arrive.
	time.Sleep(100 * time.Millisecond)
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, n := range []mykit.Notification{a, b, c} {
		if seen[n.Id] != 1 {
			t.Errorf("mention %s handled %d times, want once", n.Id, seen[n.Id])
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streamCalls < 2 {
		t.Errorf("stream connections = %d, want a reconnection", f.streamCalls)
	}
	if pos, _ := cfg.Store.Position(context.Background()); pos != c.Id {
		t.Errorf("position = %s, want %s", pos, c.Id)
	}
}

// Handling a notification again, after a restart that lost the record, sends the same Idempotency-Key, so that the
// server posts the reply once.
func TestReplySendsTheSameIdempotencyKeyTwice(t *testing.T) {
	f := newFakeServer(t)
	n := mention(10, account(1, "alice"), "hello", mykit.VisibilityPublic)
	f.add(n)
	for run := range 2 {
		cfg := f.config()
		bot, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		bot.OnMention(func(c *Context) error {
			if _, err := c.Reply("one"); err != nil {
				return err
			}
			_, err := c.Reply("two")
			return err
		})
		stop := runBot(t, bot)
		eventually(t, "the mention processed", func() bool { return processed(cfg.Store, n.Id) })
		if err := stop(); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}
	posts := f.postsSoFar()
	var keys []string
	for _, p := range posts {
		keys = append(keys, p.key)
	}
	want := []string{
		"mybot:reply:" + n.Id + ":1", "mybot:reply:" + n.Id + ":2",
		"mybot:reply:" + n.Id + ":1", "mybot:reply:" + n.Id + ":2",
	}
	if !slices.Equal(keys, want) {
		t.Errorf("Idempotency-Keys = %q, want %q", keys, want)
	}
}

// The manners: no handler for the bot itself, for a bot, or for a sender past the reply limit; replies no wider than
// their post, mentioning the sender alone.
func TestManners(t *testing.T) {
	f := newFakeServer(t)
	self := &mykit.Account{Id: f.me.Id, Username: "bot", Acct: "bot", Bot: true}
	otherBot := account(2, "robot@remote.example")
	otherBot.Bot = true
	alice := account(1, "alice")
	dave := account(3, "dave@remote.example")

	var ns []mykit.Notification
	ns = append(ns, mention(10, self, "me", mykit.VisibilityPublic), mention(11, otherBot, "a bot", mykit.VisibilityPublic))
	// Six mentions of alice: the sixth is past 5 replies in 10 minutes.
	for i := range 6 {
		ns = append(ns, mention(20+i, alice, "again", mykit.VisibilityPublic))
	}
	visibilities := []mykit.Visibility{mykit.VisibilityPrivate, mykit.VisibilityDirect, mykit.VisibilityLocal, mykit.VisibilityUnlisted}
	for i, v := range visibilities {
		ns = append(ns, mention(30+i, dave, "visibility", v))
	}
	for _, n := range ns {
		f.add(n)
	}

	cfg := f.config()
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var handled []string
	bot.OnMention(func(c *Context) error {
		mu.Lock()
		handled = append(handled, c.Notification.Id)
		mu.Unlock()
		_, err := c.Reply("hi")
		return err
	})
	runBot(t, bot)
	var ids []string
	for _, n := range ns {
		ids = append(ids, n.Id)
	}
	eventually(t, "every notification processed", func() bool { return processed(cfg.Store, ids...) })

	mu.Lock()
	defer mu.Unlock()
	for _, skipped := range []string{id(10), id(11), id(25)} {
		if slices.Contains(handled, skipped) {
			t.Errorf("the handler ran for %s", skipped)
		}
	}
	if len(handled) != 5+len(visibilities) {
		t.Errorf("handled %d notifications, want %d: %q", len(handled), 5+len(visibilities), handled)
	}

	posts := f.postsSoFar()
	got := map[string]mykit.Visibility{}
	for _, p := range posts {
		if p.req.InReplyToId == nil {
			t.Fatalf("a reply without in_reply_to_id: %+v", p.req)
		}
		got[*p.req.InReplyToId] = p.req.Visibility
		var to *mykit.Account
		for _, n := range ns {
			if n.Status.Id == *p.req.InReplyToId {
				to = n.Account
			}
		}
		if want := "@" + to.Acct + " hi"; p.req.Text != want {
			t.Errorf("reply text = %q, want %q (the sender alone)", p.req.Text, want)
		}
	}
	if v := got[id(520)]; v != mykit.VisibilityUnlisted {
		t.Errorf("the reply to a public post is %s, want unlisted", v)
	}
	for i, v := range visibilities {
		if got[id(530+i)] != v {
			t.Errorf("the reply to a %s post is %s", v, got[id(530+i)])
		}
	}
}

// RespondToBots and DisableReplyLimits lift the manners that can be lifted.
func TestMannersCanBeLifted(t *testing.T) {
	f := newFakeServer(t)
	robot := account(2, "robot")
	robot.Bot = true
	for i := range 7 {
		f.add(mention(10+i, robot, "again", mykit.VisibilityPublic))
	}
	cfg := f.config()
	cfg.RespondToBots = true
	cfg.DisableReplyLimits = true
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	count := 0
	bot.OnMention(func(c *Context) error {
		mu.Lock()
		count++
		mu.Unlock()
		_, err := c.Reply("hi")
		return err
	})
	runBot(t, bot)
	eventually(t, "every notification processed", func() bool { return processed(cfg.Store, id(10), id(16)) })
	mu.Lock()
	defer mu.Unlock()
	if count != 7 {
		t.Errorf("handled %d, want 7", count)
	}
}

// A rotated refresh token is saved in the Store, and the next start uses it rather than the one of the Config.
func TestRefreshTokenRotationIsSaved(t *testing.T) {
	f := newFakeServer(t)
	f.tokens["rt1"] = "rt2"
	path := filepath.Join(t.TempDir(), "state.json")
	start := func() *Bot {
		store, err := NewFileStore(path)
		if err != nil {
			t.Fatal(err)
		}
		cfg := f.config()
		cfg.TokenSource = nil
		cfg.OAuth = &mykit.OAuthConfig{ClientID: "client", ClientSecret: "secret"}
		cfg.RefreshToken = "rt1"
		cfg.Store = store
		bot, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := bot.start(t.Context()); err != nil {
			t.Fatalf("start: %v", err)
		}
		return bot
	}
	b1 := start()
	if rt, _ := b1.Store().RefreshToken(t.Context()); rt != "rt2" {
		t.Fatalf("saved refresh token = %q, want rt2", rt)
	}
	b2 := start()
	if rt, _ := b2.Store().RefreshToken(t.Context()); rt != "rt2'" {
		t.Errorf("saved refresh token after the second start = %q, want rt2'", rt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if want := []string{"rt1", "rt2"}; !slices.Equal(f.refreshTokens, want) {
		t.Errorf("refresh tokens sent = %q, want %q", f.refreshTokens, want)
	}
}

// An account without the bot flag stops the bot, unless SetBotFlag sets it or AllowNonBot allows it.
func TestBotFlag(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		f := newFakeServer(t)
		f.me.Bot = false
		bot, err := New(f.config())
		if err != nil {
			t.Fatal(err)
		}
		if err := bot.Run(t.Context()); !errors.Is(err, ErrNotBot) {
			t.Fatalf("Run = %v, want ErrNotBot", err)
		}
		if len(f.botUpdates) != 0 {
			t.Errorf("updateCredentials was called: %v", f.botUpdates)
		}
	})
	t.Run("SetBotFlag", func(t *testing.T) {
		f := newFakeServer(t)
		f.me.Bot = false
		cfg := f.config()
		cfg.SetBotFlag = true
		bot, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := bot.start(t.Context()); err != nil {
			t.Fatalf("start: %v", err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if !slices.Equal(f.botUpdates, []bool{true}) {
			t.Errorf("updateCredentials bot = %v, want [true]", f.botUpdates)
		}
		if !bot.me.Bot {
			t.Error("the account has no bot flag after SetBotFlag")
		}
	})
	t.Run("AllowNonBot", func(t *testing.T) {
		f := newFakeServer(t)
		f.me.Bot = false
		cfg := f.config()
		cfg.AllowNonBot = true
		bot, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := bot.start(t.Context()); err != nil {
			t.Fatalf("start: %v", err)
		}
		if len(f.botUpdates) != 0 {
			t.Errorf("updateCredentials was called: %v", f.botUpdates)
		}
	})
}

// scopedToken is a TokenSource of a token with scopes.
type scopedToken string

func (s scopedToken) Token(context.Context) (*mykit.Token, error) {
	return &mykit.Token{AccessToken: "token", Scope: string(s)}, nil
}

// A token without the scopes the bot needs stops it, naming them.
func TestMissingScopesStopTheBot(t *testing.T) {
	f := newFakeServer(t)
	cfg := f.config()
	cfg.TokenSource = scopedToken("read:accounts read:notifications follow")
	cfg.SetBotFlag = true
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = bot.Run(t.Context())
	var se *ScopeError
	if !errors.As(err, &se) {
		t.Fatalf("Run = %v, want a *ScopeError", err)
	}
	if want := []string{"read:statuses", "write:statuses", "write:accounts"}; !slices.Equal(se.Missing, want) {
		t.Errorf("missing = %q, want %q", se.Missing, want)
	}

	cfg.TokenSource = scopedToken("read write:statuses write:accounts")
	bot, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := bot.start(t.Context()); err != nil {
		t.Errorf("start with the broad read scope: %v", err)
	}
}

// Commands match the first word after the bot's mention without case; help lists them; other mentions fall to
// OnMention, and other notifications to OnNotification.
func TestRouting(t *testing.T) {
	f := newFakeServer(t)
	alice := account(1, "alice")
	f.add(mention(10, alice, "PING now please", mykit.VisibilityPublic))
	f.add(mention(11, account(2, "bob"), "help", mykit.VisibilityPublic))
	f.add(mention(12, account(3, "carol"), "just saying hi", mykit.VisibilityPublic))
	f.add(mykit.Notification{Id: id(13), Type: mykit.NotificationTypeFollow, CreatedAt: time.Now(), Account: account(4, "dave")})
	f.add(mykit.Notification{Id: id(14), Type: mykit.NotificationTypePollClosed, CreatedAt: time.Now()})
	cfg := f.config()
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	got := map[string]string{}
	record := func(kind string) Handler {
		return func(c *Context) error {
			mu.Lock()
			defer mu.Unlock()
			got[c.Notification.Id] = kind + ":" + c.Command + ":" + c.Args
			return nil
		}
	}
	bot.OnCommand("ping", "answer pong", record("command"))
	bot.OnMention(record("mention"))
	bot.OnFollow(record("follow"))
	bot.OnNotification(record("notification"))
	runBot(t, bot)
	eventually(t, "every notification processed", func() bool { return processed(cfg.Store, id(10), id(11), id(12), id(13), id(14)) })

	mu.Lock()
	defer mu.Unlock()
	want := map[string]string{
		id(10): "command:ping:now please",
		id(12): "mention::",
		id(13): "follow::",
		id(14): "notification::",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
	posts := f.postsSoFar()
	if len(posts) != 1 || posts[0].req.Text != "@bob help: list the commands\nping: answer pong" {
		t.Errorf("the answer of help = %+v", posts)
	}
}

// A panicking handler is recovered, its notification counts as processed, and the bot goes on.
func TestRecover(t *testing.T) {
	f := newFakeServer(t)
	f.add(mention(10, account(1, "alice"), "boom", mykit.VisibilityPublic))
	f.add(mention(11, account(1, "alice"), "fine", mykit.VisibilityPublic))
	cfg := f.config()
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	bot.OnMention(func(c *Context) error {
		if c.Text == "@bot boom" {
			panic("boom")
		}
		return c.React("👍")
	})
	runBot(t, bot)
	eventually(t, "both processed", func() bool { return processed(cfg.Store, id(10), id(11)) })
	f.mu.Lock()
	defer f.mu.Unlock()
	if want := []string{id(511) + " 👍"}; !slices.Equal(f.reactions, want) {
		t.Errorf("reactions = %q, want %q", f.reactions, want)
	}
}

// A handler still running after the grace period is cancelled, and its notification stays unprocessed for the
// next start.
func TestShutdownCancelsAfterTheGracePeriod(t *testing.T) {
	f := newFakeServer(t)
	f.add(mention(10, account(1, "alice"), "slow", mykit.VisibilityPublic))
	cfg := f.config()
	cfg.ShutdownGrace = 50 * time.Millisecond
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	bot.OnMention(func(c *Context) error {
		close(started)
		<-c.Done()
		close(cancelled)
		return c.Err()
	})
	stop := runBot(t, bot)
	<-started
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler's context was not cancelled")
	}
	time.Sleep(20 * time.Millisecond)
	if processed(cfg.Store, id(10)) {
		t.Error("the cancelled notification was recorded as processed")
	}
	if pos, _ := cfg.Store.Position(context.Background()); pos != "" {
		t.Errorf("position = %q, want none", pos)
	}
}

// The catch-up stops at the position and at the age limit.
func TestCatchUpStopsAtThePositionAndTheLimit(t *testing.T) {
	f := newFakeServer(t)
	old := mention(5, account(1, "alice"), "old", mykit.VisibilityPublic)
	old.CreatedAt = time.Now().Add(-48 * time.Hour)
	f.add(old)
	f.add(mention(6, account(1, "alice"), "before the position", mykit.VisibilityPublic))
	f.add(mention(7, account(1, "alice"), "at the position", mykit.VisibilityPublic))
	f.add(mention(8, account(1, "alice"), "new", mykit.VisibilityPublic))
	cfg := f.config()
	if err := cfg.Store.SetPosition(t.Context(), id(7)); err != nil {
		t.Fatal(err)
	}
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var handled []string
	bot.OnMention(func(c *Context) error {
		mu.Lock()
		defer mu.Unlock()
		handled = append(handled, c.Notification.Id)
		return nil
	})
	runBot(t, bot)
	eventually(t, "the new mention processed", func() bool { return processed(cfg.Store, id(8)) })
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(handled, []string{id(8)}) {
		t.Errorf("handled %q, want only %s", handled, id(8))
	}

	// Without a position, the age limit stops it.
	f2 := newFakeServer(t)
	f2.add(old)
	f2.add(mention(9, account(1, "alice"), "recent", mykit.VisibilityPublic))
	cfg2 := f2.config()
	bot2, err := New(cfg2)
	if err != nil {
		t.Fatal(err)
	}
	bot2.OnMention(func(*Context) error { return nil })
	runBot(t, bot2)
	eventually(t, "the recent mention processed", func() bool { return processed(cfg2.Store, id(9)) })
	if processed(cfg2.Store, id(5)) {
		t.Error("a notification older than the catch-up limit was processed")
	}
}

// The send budget delays writes, and drops those that would wait too long.
func TestSendBudget(t *testing.T) {
	b := newBudget(Rate{N: 2, Per: time.Minute}, DefaultWriteRate)
	now := time.Unix(1_000_000, 0)
	for i := range 2 {
		if w, ok := b.reserve(sendPost, now, time.Minute); !ok || w != 0 {
			t.Fatalf("post %d: wait %v, %v", i, w, ok)
		}
	}
	if w, ok := b.reserve(sendPost, now, time.Minute); !ok || w != 30*time.Second {
		t.Errorf("third post: wait %v, %v; want 30s", w, ok)
	}
	if _, ok := b.reserve(sendPost, now, 10*time.Second); ok {
		t.Error("a post that waits past the limit was reserved")
	}

	// Less than a tenth of the server's limit left: wait for its reset.
	h := http.Header{}
	h.Set("RateLimit-Policy", `"api";q=300;w=300`)
	h.Set("RateLimit", `"api";r=29;t=120`)
	b.observe(h, now)
	if w, ok := b.reserve(sendWrite, now, 10*time.Minute); !ok || w != 2*time.Minute {
		t.Errorf("write after the RateLimit fields: wait %v, %v; want 2m", w, ok)
	}
	h.Set("RateLimit", `"api";r=30;t=120`)
	b2 := newBudget(DefaultPostRate, DefaultWriteRate)
	b2.observe(h, now)
	if w, _ := b2.reserve(sendWrite, now, time.Minute); w != 0 {
		t.Errorf("a tenth left: wait %v, want none", w)
	}
}

func TestReplyLimiter(t *testing.T) {
	l := newReplyLimiter(DefaultReplyLimits)
	now := time.Unix(1_000_000, 0)
	for i := range 5 {
		if !l.take("alice", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("reply %d refused", i)
		}
	}
	if !l.full("alice", now.Add(time.Minute)) || l.full("bob", now) {
		t.Error("the 10-minute limit")
	}
	// Five more after 11 and after 22 minutes make 15 in the hour.
	for _, after := range []time.Duration{11 * time.Minute, 22 * time.Minute} {
		for i := range 5 {
			if !l.take("alice", now.Add(after+time.Duration(i)*time.Second)) {
				t.Fatalf("reply %d after %v refused", i, after)
			}
		}
	}
	if !l.full("alice", now.Add(40*time.Minute)) {
		t.Error("the hourly limit")
	}
	if l.full("alice", now.Add(61*time.Minute)) {
		t.Error("the hourly limit after the hour")
	}
}
