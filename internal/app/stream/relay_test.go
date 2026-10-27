// SPDX-License-Identifier: AGPL-3.0-only

package stream_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/stream"
)

// The routing, limits, slow subscribers, and broken listeners of Relay are verified by
// testkit.StreamBrokerContract (pkg/testkit/stream_test.go); these tests cover what is its
// own, on a fake clock.

func closeRelay(t *testing.T, r *stream.Relay) {
	t.Helper()
	if err := r.Close(context.Background()); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestRelayListensLazily(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := stream.NewMemoryBroker()
		r := stream.NewRelay(broker.Bus(), stream.RelayOptions{})
		defer closeRelay(t, r)

		r.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: entryID})
		synctest.Wait()
		if got := broker.Listeners(); got != 0 {
			t.Fatalf("listeners without subscriptions = %d, want 0", got)
		}

		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatal(err)
		}
		if got := broker.Listeners(); got != 1 {
			t.Fatalf("listeners with a subscription = %d, want 1", got)
		}
		s.Close()
		time.Sleep(stream.DefaultIdleTimeout - time.Second)
		synctest.Wait()
		if got := broker.Listeners(); got != 1 {
			t.Fatalf("listeners before the idle timeout = %d, want 1", got)
		}

		// A subscription within the idle timeout keeps the listener and restarts the timeout.
		s, err = r.Subscribe(t.Context(), bobID)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
		s.Close()
		time.Sleep(stream.DefaultIdleTimeout - time.Second)
		synctest.Wait()
		if got := broker.Listeners(); got != 1 {
			t.Fatalf("listeners before the restarted idle timeout = %d, want 1", got)
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if got := broker.Listeners(); got != 0 {
			t.Fatalf("listeners after the idle timeout = %d, want 0", got)
		}
	})
}

func TestRelayUnavailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := stream.NewMemoryBroker()
		broker.SetUnreachable(true)
		r := stream.NewRelay(broker.Bus(), stream.RelayOptions{})
		defer closeRelay(t, r)

		start := time.Now()
		if _, err := r.Subscribe(t.Context(), aliceID); !errors.Is(err, stream.ErrUnavailable) {
			t.Fatalf("Subscribe on an unreachable broker: %v, want ErrUnavailable", err)
		}
		if waited := time.Since(start); waited != stream.DefaultReadyTimeout {
			t.Errorf("Subscribe waited %s, want %s", waited, stream.DefaultReadyTimeout)
		}
		// A subscription that gives up leaves the start of the listener in progress, still
		// waiting for the unreachable broker.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if _, err := r.Subscribe(ctx, aliceID); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Subscribe with a shorter context: %v, want its error", err)
		}
		synctest.Wait()
		if got := r.Len(); got != 0 {
			t.Errorf("Len = %d, want 0", got)
		}

		// Once the broker answers again, the next subscription listens, although it joins that
		// start, which fails.
		broker.SetUnreachable(false)
		start = time.Now()
		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatalf("Subscribe on a reachable broker: %v", err)
		}
		if waited := time.Since(start); waited >= stream.DefaultReadyTimeout {
			t.Errorf("Subscribe waited %s, want less than %s", waited, stream.DefaultReadyTimeout)
		}
		s.Close()
	})
}

// syncBuffer is a bytes.Buffer safe for concurrent writes, for a log handler.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestRelayDropsWhatItCannotSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := stream.NewMemoryBroker()
		broker.SetUnreachable(true)
		var log syncBuffer
		r := stream.NewRelay(broker.Bus(), stream.RelayOptions{
			SendBuffer: 2, SendTimeout: time.Second, Logger: slog.New(slog.NewTextHandler(&log, nil)),
		})
		defer closeRelay(t, r)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 10 {
				r.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: entryID})
			}
		}()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("Publish blocked on an unreachable broker")
		}
		if got := r.Stats().DroppedBufferFull; got < 7 {
			t.Errorf("DroppedBufferFull = %d, want at least 7", got)
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if st := r.Stats(); st.DroppedBufferFull+st.DroppedSendFailed != 10 {
			t.Errorf("Stats = %+v, want 10 dropped in all", st)
		}
		if !strings.Contains(log.String(), "dropped signals") {
			t.Errorf("the drops were not logged: %s", log.String())
		}
	})
}

func TestRelayDecodesOnlyKnownSignals(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := stream.NewMemoryBroker()
		r := stream.NewRelay(broker.Bus(), stream.RelayOptions{})
		defer closeRelay(t, r)
		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		other := broker.Bus()
		for _, p := range []string{
			`{"v":2,"o":"other","k":"home_entry","e":"from a newer process"}`,
			`{"v":1,"o":"other","k":"reaction","e":"unknown kind"}`,
			`not a signal`,
			`{"v":1,"o":"` + r.ProcessID() + `","k":"home_entry","e":"from this process"}`,
			`{"v":1,"o":"other","k":"home_entry","e":"0199f0a8-0000-7000-8000-000000000001"}`,
		} {
			if err := other.Send(t.Context(), []byte(p)); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		want := stream.Event{Kind: stream.KindHomeEntry, EntryID: entryID}
		select {
		case e := <-s.Events():
			if e != want {
				t.Errorf("received %+v, want %+v", e, want)
			}
		default:
			t.Fatal("the valid signal did not arrive")
		}
		select {
		case e := <-s.Events():
			t.Errorf("received %+v more", e)
		default:
		}
	})
}

func TestRelayCloseSendsWhatIsQueued(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := stream.NewMemoryBroker()
		sender := stream.NewRelay(broker.Bus(), stream.RelayOptions{})
		receiver := stream.NewRelay(broker.Bus(), stream.RelayOptions{})
		defer closeRelay(t, receiver)
		s, err := receiver.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatal(err)
		}

		local, err := sender.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatal(err)
		}
		e := stream.Event{Kind: stream.KindNotifications, AccountID: aliceID}
		sender.Publish(e)
		closeRelay(t, sender)
		if !isDone(local) {
			t.Error("Close left a subscription open")
		}
		if _, err := sender.Subscribe(t.Context(), aliceID); !errors.Is(err, stream.ErrClosed) {
			t.Errorf("Subscribe after Close: %v, want ErrClosed", err)
		}
		synctest.Wait()
		select {
		case got := <-s.Events():
			if got != e {
				t.Errorf("received %+v, want %+v", got, e)
			}
		default:
			t.Error("the signal queued at Close was not sent")
		}
		if got := broker.Listeners(); got != 1 {
			t.Errorf("listeners = %d, want the receiver's only", got)
		}
		s.Close()
	})
}

// kvStore is a minimal kernel.KeyValueStore on the bubble's clock.
type kvStore struct {
	mu      sync.Mutex
	entries map[string]time.Time // namespace/key → expiry
	sets    int
	fail    error // when not nil, Set fails with it
}

func newKVStore() *kvStore { return &kvStore{entries: map[string]time.Time{}} }

func (s *kvStore) setFail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

func (s *kvStore) Get(_ context.Context, ns, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.entries[ns+"/"+key]
	if !ok || !time.Now().Before(exp) {
		return nil, kernel.ErrKeyNotFound
	}
	return []byte("1"), nil
}

func (s *kvStore) Set(_ context.Context, ns, key string, _ []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.entries[ns+"/"+key] = time.Now().Add(ttl)
	s.sets++
	return nil
}

func (s *kvStore) Delete(_ context.Context, ns, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, ns+"/"+key)
	return nil
}

func (s *kvStore) IncrBy(context.Context, string, string, int64, time.Duration) (int64, error) {
	return 0, errors.ErrUnsupported
}

func (s *kvStore) CompareAndSwap(context.Context, string, string, []byte, []byte, time.Duration) (bool, error) {
	panic("unused")
}

func (s *kvStore) DeleteExpired(context.Context, int) (int64, error) { return 0, nil }

func (s *kvStore) setCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sets
}

func TestRelayWritesItsLivenessMarker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newKVStore()
		broker := stream.NewMemoryBroker()
		r := stream.NewRelay(broker.Bus(), stream.RelayOptions{Liveness: kv})
		defer closeRelay(t, r)
		alive := func() bool {
			ok, err := stream.Alive(t.Context(), kv, r.ProcessID())
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		if alive() {
			t.Fatal("a process that does not listen is alive")
		}

		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if !alive() {
			t.Fatal("a listening process has no liveness marker")
		}
		time.Sleep(3*stream.LivenessInterval + time.Second)
		synctest.Wait()
		if got := kv.setCount(); got != 4 {
			t.Errorf("marker writes after 3 intervals = %d, want 4", got)
		}

		// Once the listener closes, the marker is no longer renewed and expires.
		s.Close()
		time.Sleep(stream.DefaultIdleTimeout + time.Second)
		synctest.Wait()
		writes := kv.setCount()
		time.Sleep(stream.LivenessTTL)
		synctest.Wait()
		if got := kv.setCount(); got != writes {
			t.Errorf("marker writes after the listener closed = %d, want %d", got, writes)
		}
		if alive() {
			t.Error("the marker outlived its TTL after the listener closed")
		}
	})
}

// A Relay that cannot write its liveness marker does not listen, so that no sweep takes its
// resources for the leftovers of an ended process.
func TestRelayDoesNotListenWithoutItsLivenessMarker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newKVStore()
		kv.setFail(errors.New("kv down"))
		broker := stream.NewMemoryBroker()
		r := stream.NewRelay(broker.Bus(), stream.RelayOptions{Liveness: kv})
		defer closeRelay(t, r)

		if _, err := r.Subscribe(t.Context(), aliceID); !errors.Is(err, stream.ErrUnavailable) {
			t.Fatalf("Subscribe without a marker: %v, want ErrUnavailable", err)
		}
		synctest.Wait()
		if got := broker.Listeners(); got != 0 {
			t.Errorf("listeners without a marker = %d, want 0", got)
		}

		kv.setFail(nil)
		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatalf("Subscribe once the marker can be written: %v", err)
		}
		defer s.Close()
		if ok, err := stream.Alive(t.Context(), kv, r.ProcessID()); err != nil || !ok {
			t.Errorf("Alive = %v, %v; want true", ok, err)
		}
	})
}

// A Relay whose marker writes keep failing until the marker may expire ends its subscriptions
// and closes its listener, whose resources a sweep may delete; a single failure does not.
func TestRelayBreaksWhenItsLivenessMarkerLapses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newKVStore()
		broker := stream.NewMemoryBroker()
		r := stream.NewRelay(broker.Bus(), stream.RelayOptions{Liveness: kv})
		defer closeRelay(t, r)
		s, err := r.Subscribe(t.Context(), aliceID)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		kv.setFail(errors.New("kv down"))
		time.Sleep(stream.LivenessInterval + time.Second)
		synctest.Wait()
		kv.setFail(nil)
		time.Sleep(stream.LivenessInterval)
		synctest.Wait()
		select {
		case <-s.Done():
			t.Fatal("one failed marker write ended the subscription")
		default:
		}

		kv.setFail(errors.New("kv down"))
		lapse := stream.LivenessTTL - stream.LivenessInterval
		time.Sleep(lapse - stream.LivenessInterval)
		synctest.Wait()
		select {
		case <-s.Done():
			t.Fatal("the subscription ended while the marker was still live past the next write")
		default:
		}
		time.Sleep(stream.LivenessInterval)
		synctest.Wait()
		select {
		case <-s.Done():
		default:
			t.Fatal("the subscription outlived a marker that may have expired")
		}
		if got := broker.Listeners(); got != 0 {
			t.Errorf("listeners after the marker lapsed = %d, want 0", got)
		}
	})
}

// fakeSweeper reports the processes of its resources as alive or not.
type fakeSweeper struct {
	processes []string
	minAge    time.Duration
	swept     []string
	err       error
}

func (s *fakeSweeper) Sweep(ctx context.Context, minAge time.Duration, alive func(context.Context, string) (bool, error)) (int, error) {
	s.minAge = minAge
	for _, p := range s.processes {
		ok, err := alive(ctx, p)
		if err != nil {
			return len(s.swept), err
		}
		if !ok {
			s.swept = append(s.swept, p)
		}
	}
	return len(s.swept), s.err
}

func TestSweepJob(t *testing.T) {
	kv := newKVStore()
	if err := kv.Set(t.Context(), stream.LivenessNamespace, "live", []byte("1"), stream.LivenessTTL); err != nil {
		t.Fatal(err)
	}
	sw := &fakeSweeper{processes: []string{"live", "gone"}}
	reg := &queue.JobHandlerRegistry{}
	sched := queue.NewScheduler(nil, nil, nil)
	stream.RegisterSweepJob(reg, sched, sw, kv, nil)

	if s := sched.Schedules(); len(s) != 1 || s[0].Job.Type != stream.JobSweep {
		t.Fatalf("schedules = %+v, want the sweep", s)
	}
	h, _, ok := reg.Lookup(stream.JobSweep)
	if !ok {
		t.Fatal("no handler for the sweep")
	}
	if res := h(t.Context(), queue.Job{Type: stream.JobSweep}); !res.IsDone() {
		t.Fatalf("sweep = %+v, want done", res)
	}
	if sw.minAge != stream.SweepMinAge || len(sw.swept) != 1 || sw.swept[0] != "gone" {
		t.Errorf("swept %v older than %s, want [gone] older than %s", sw.swept, sw.minAge, stream.SweepMinAge)
	}

	sw.err = errors.New("broker down")
	if res := h(t.Context(), queue.Job{Type: stream.JobSweep}); !res.IsRetry() {
		t.Errorf("a failed sweep = %+v, want retry", res)
	}
}
