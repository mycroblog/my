// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// A notification handed out again while its handler runs stays pending: a newer one processed meanwhile does not
// move the position past it.
func TestDuplicateHandOutKeepsTheNotificationPending(t *testing.T) {
	b := &Bot{tracker: newTracker(), store: NewMemoryStore(), logger: slog.New(slog.DiscardHandler)}
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan string, 2)
	b.dispatcher = newDispatcher(2, func(j job) {
		if j.notification.Id == id(10) {
			close(started)
			<-release
		}
		b.completed(j.notification.Id)
		done <- j.notification.Id
	})
	older := mention(10, account(1, "alice"), "slow", mykit.VisibilityPublic)
	newer := mention(11, account(2, "bob"), "fast", mykit.VisibilityPublic)

	b.submitNotification(&older)
	<-started
	b.submitNotification(&older) // the catch-up brings it again while it runs
	b.submitNotification(&newer)
	if got := <-done; got != newer.Id {
		t.Fatalf("processed %s first, want %s", got, newer.Id)
	}
	if pos, _ := b.store.Position(t.Context()); pos != "" {
		t.Errorf("position = %s while %s runs, want none", pos, older.Id)
	}
	close(release)
	<-done
	if pos, _ := b.store.Position(t.Context()); pos != newer.Id {
		t.Errorf("position = %q, want %s", pos, newer.Id)
	}
}

// The tracker counts the hand-outs of one notification.
func TestTrackerForgetsARefusedHandOutOnly(t *testing.T) {
	tr := newTracker()
	tr.add("a")
	tr.add("a")
	tr.forget("a")
	tr.add("b")
	if p := tr.complete("b"); p != "" {
		t.Errorf("b moved the position to %q while a is pending", p)
	}
	if p := tr.complete("a"); p != "b" {
		t.Errorf("after a: %q, want b", p)
	}
}

// streamingBot is a bot of f in Mode Streaming on clock that records the IDs of the mentions it handles.
func streamingBot(t *testing.T, f *fakeServer, clock Clock) (*Bot, Config, func() []string) {
	t.Helper()
	cfg := f.config()
	cfg.Mode = Streaming
	cfg.Clock = clock
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
	return bot, cfg, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(handled)
	}
}

// A failed catch-up keeps the position, and the notifications of the stream, until it succeeds: a newer
// notification of the stream never moves the position past those it has yet to find.
func TestFailedCatchUpHoldsThePosition(t *testing.T) {
	f := newFakeServer(t)
	alice := account(1, "alice")
	missed := mention(10, alice, "while the bot was stopped", mykit.VisibilityPublic)
	live := mention(11, alice, "on the stream", mykit.VisibilityPublic)
	f.add(missed)
	f.list = func(n int) bool { return n > 1 } // the first catch-up fails
	sent := make(chan struct{})
	f.stream = func(w http.ResponseWriter, r *http.Request, _ int) {
		eventually(t, "the failed catch-up", func() bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.listCalls >= 1
		})
		f.add(live)
		writeEvent(w, "notification", live)
		close(sent)
		<-r.Context().Done()
	}
	clock := newFakeClock(t, "2026-01-01T00:00:00Z")
	bot, cfg, handled := streamingBot(t, f, clock)
	runBot(t, bot)
	<-sent
	time.Sleep(100 * time.Millisecond)
	if pos, _ := cfg.Store.Position(t.Context()); pos != "" {
		t.Fatalf("position = %s before the catch-up succeeded, want none", pos)
	}
	if got := handled(); len(got) != 0 {
		t.Fatalf("handled %q before the catch-up succeeded", got)
	}
	// The pruning and the retry of the catch-up sleep.
	eventually(t, "the retry of the catch-up waiting", func() bool { return clock.sleepers() >= 2 })
	clock.Advance(retryFirstWait)
	eventually(t, "both mentions processed", func() bool { return processed(cfg.Store, missed.Id, live.Id) })
	eventually(t, "the position", func() bool {
		pos, _ := cfg.Store.Position(context.Background())
		return pos == live.Id
	})
	if got, want := handled(), []string{missed.Id, live.Id}; !slices.Equal(got, want) {
		t.Errorf("handled %q, want %q", got, want)
	}
}

// The notifications of the stream that arrive while the catch-up reads the missed ones are handled after them:
// every notification in the order of time.
func TestStreamWaitsForTheCatchUp(t *testing.T) {
	f := newFakeServer(t)
	alice := account(1, "alice")
	missed := mention(10, alice, "while the bot was stopped", mykit.VisibilityPublic)
	live := mention(11, alice, "on the stream", mykit.VisibilityPublic)
	f.add(missed)
	f.add(live)
	sent := make(chan struct{})
	f.list = func(n int) bool {
		if n == 1 {
			// The stream brings the newer notification before the catch-up has read the older one.
			<-sent
			time.Sleep(100 * time.Millisecond)
		}
		return true
	}
	f.stream = func(w http.ResponseWriter, r *http.Request, _ int) {
		writeEvent(w, "notification", live)
		close(sent)
		<-r.Context().Done()
	}
	bot, cfg, handled := streamingBot(t, f, nil)
	runBot(t, bot)
	eventually(t, "both mentions processed", func() bool { return processed(cfg.Store, missed.Id, live.Id) })
	time.Sleep(50 * time.Millisecond)
	if got, want := handled(), []string{missed.Id, live.Id}; !slices.Equal(got, want) {
		t.Errorf("handled %q, want %q", got, want)
	}
}

// flakyStore is a MemoryStore whose IsProcessed and MarkProcessed fail for the notifications in failRead and
// failMark, once each.
type flakyStore struct {
	*MemoryStore
	mu                 sync.Mutex
	failRead, failMark map[string]bool
}

var errFlaky = errors.New("the store is down")

func (s *flakyStore) fail(m map[string]bool, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m[id] {
		delete(m, id)
		return true
	}
	return false
}

func (s *flakyStore) IsProcessed(ctx context.Context, id string) (bool, error) {
	if s.fail(s.failRead, id) {
		return false, errFlaky
	}
	return s.MemoryStore.IsProcessed(ctx, id)
}

func (s *flakyStore) MarkProcessed(ctx context.Context, id string, at time.Time) error {
	if s.fail(s.failMark, id) {
		return errFlaky
	}
	return s.MemoryStore.MarkProcessed(ctx, id, at)
}

// A notification whose Store calls fail is tried again, without waiting for the server to send it again, and
// holds the position until it is processed.
func TestStoreFailureIsTriedAgain(t *testing.T) {
	f := newFakeServer(t)
	read := mention(10, account(1, "alice"), "read fails", mykit.VisibilityPublic)
	mark := mention(11, account(2, "bob"), "mark fails", mykit.VisibilityPublic)
	fine := mention(12, account(3, "carol"), "fine", mykit.VisibilityPublic)
	f.add(read)
	f.add(mark)
	f.add(fine)
	store := &flakyStore{
		MemoryStore: NewMemoryStore(),
		failRead:    map[string]bool{read.Id: true},
		failMark:    map[string]bool{mark.Id: true},
	}
	clock := newFakeClock(t, "2026-01-01T00:00:00Z")
	cfg := f.config()
	cfg.Store = store
	cfg.Clock = clock
	bot, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	handled := map[string]int{}
	bot.OnMention(func(c *Context) error {
		mu.Lock()
		defer mu.Unlock()
		handled[c.Notification.Id]++
		return nil
	})
	runBot(t, bot)
	eventually(t, "the newest mention processed", func() bool { return processed(store, fine.Id) })
	// The pruning, the polling, and the two retries sleep.
	eventually(t, "the retries waiting", func() bool { return clock.sleepers() >= 4 })
	if pos, _ := store.Position(t.Context()); pos != "" {
		t.Fatalf("position = %s while older notifications are pending, want none", pos)
	}
	clock.Advance(retryFirstWait)
	eventually(t, "every mention processed", func() bool { return processed(store, read.Id, mark.Id, fine.Id) })
	eventually(t, "the position", func() bool {
		pos, _ := store.Position(context.Background())
		return pos == fine.Id
	})
	mu.Lock()
	defer mu.Unlock()
	for _, n := range []mykit.Notification{read, mark, fine} {
		if handled[n.Id] != 1 {
			t.Errorf("mention %s handled %d times, want once", n.Id, handled[n.Id])
		}
	}
}
