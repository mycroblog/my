// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
)

// defaultStreamTimeout bounds the waits of the stream contracts when the harness sets none.
const defaultStreamTimeout = 5 * time.Second

// streamGrace is how long the stream contracts wait for something that must not arrive.
const streamGrace = 200 * time.Millisecond

// StreamBus is what StreamBusContract needs of a stream.Bus adapter on one broker.
type StreamBus struct {
	// NewBus returns a new Bus on the broker, standing for one process: the Buses it returns
	// share the broker. It registers their cleanup with t.Cleanup. Required.
	NewBus func(t *testing.T) stream.Bus
	// Unreachable, when not nil, returns a Bus whose broker cannot be reached (a closed port, an
	// endpoint that does not answer); Send and Listen on it must fail by their context's
	// deadline.
	Unreachable func(t *testing.T) stream.Bus
	// Break, when not nil, cuts every open listener of the broker from it, as a lost connection
	// or a deleted queue or subscription would.
	Break func(t *testing.T)
	// Resources, when not nil, returns the number of per-process resources on the broker
	// (listening connections, queues, subscriptions); the contract checks that Close deletes
	// its listener's.
	Resources func(t *testing.T) int
	// Timeout bounds each wait for a payload or for Broken; zero means 5 seconds. Adapters that
	// long-poll raise it.
	Timeout time.Duration
}

// StreamBusFactory returns the harness of a new broker, or of a part of a shared one that no
// other test uses (its own channel, topic, or prefix).
type StreamBusFactory func(t *testing.T) StreamBus

// StreamBusContract verifies a stream.Bus adapter: Send without listeners succeeds, a payload
// reaches the listeners of every Bus on the broker, including the sender's, byte for byte; a
// listener outlives the context of Listen; Close stops receiving, deletes the listener's
// resources, and can be called again; Send and Listen return by their deadline when the broker
// cannot be reached; a cut listener closes Broken and a new one can listen again; every payload
// of a burst reaches an open listener; and a burst cut by a break delivers only payloads that
// were sent, byte for byte, none after Close, and leaves a new listener receiving every payload
// of the next burst. The payloads that a cut listener misses or receives twice are the accepted
// semantics of stream.Bus and are not checked.
func StreamBusContract(t *testing.T, newBus StreamBusFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, h StreamBus)
	}{
		{"Broadcasts", busBroadcasts},
		{"ListenOutlivesItsContext", busListenOutlivesItsContext},
		{"Closes", busCloses},
		{"ReturnsWhenUnreachable", busReturnsWhenUnreachable},
		{"Breaks", busBreaks},
		{"DeliversABurst", busDeliversABurst},
		{"BreaksDuringABurst", busBreaksDuringABurst},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newBus(t)
			if h.Timeout <= 0 {
				h.Timeout = defaultStreamTimeout
			}
			tc.run(t, h)
		})
	}
}

// busPayload returns a payload unique to this test: a JSON object like the signals of
// stream.Relay, of exactly size bytes (at least 64), with text that brokers might mangle.
func busPayload(t *testing.T, size int) []byte {
	t.Helper()
	var nonce [8]byte
	_, _ = rand.Read(nonce[:])
	var b strings.Builder
	b.WriteString(`{"v":1,"o":"` + hex.EncodeToString(nonce[:]) + `","k":"home_entry","e":"`)
	const tail = `"}`
	filler := []string{`\\`, `\"`, " ", "☃", "日本語", "<&>", "'", "\t"}
	for i := 0; b.Len()+len(filler[i%len(filler)])+len(tail) <= size; i++ {
		b.WriteString(filler[i%len(filler)])
	}
	for b.Len()+len(tail) < size {
		b.WriteByte(' ')
	}
	b.WriteString(tail)
	return []byte(b.String())
}

// busCollector records the payloads one listener receives.
type busCollector struct {
	mu       sync.Mutex
	payloads [][]byte
	arrived  chan struct{}
}

func newBusCollector() *busCollector { return &busCollector{arrived: make(chan struct{}, 1)} }

func (c *busCollector) receive(p []byte) {
	c.mu.Lock()
	c.payloads = append(c.payloads, p)
	c.mu.Unlock()
	select {
	case c.arrived <- struct{}{}:
	default:
	}
}

func (c *busCollector) has(p []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.ContainsFunc(c.payloads, func(q []byte) bool { return bytes.Equal(p, q) })
}

// wait reports whether p arrives within timeout.
func (c *busCollector) wait(p []byte, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for !c.has(p) {
		select {
		case <-c.arrived:
		case <-deadline.C:
			return c.has(p)
		}
	}
	return true
}

func busListen(t *testing.T, h StreamBus, b stream.Bus, c *busCollector) stream.Listener {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), h.Timeout)
	defer cancel()
	l, err := b.Listen(ctx, c.receive)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), h.Timeout)
		defer cancel()
		_ = l.Close(ctx)
	})
	return l
}

func busSend(t *testing.T, h StreamBus, b stream.Bus, p []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), h.Timeout)
	defer cancel()
	if err := b.Send(ctx, p); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func busClose(t *testing.T, h StreamBus, l stream.Listener) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), h.Timeout)
	defer cancel()
	return l.Close(ctx)
}

func busBroadcasts(t *testing.T, h StreamBus) {
	a, b := h.NewBus(t), h.NewBus(t)
	busSend(t, h, a, busPayload(t, 64)) // nobody listens: lost, not an error

	ca, cb := newBusCollector(), newBusCollector()
	busListen(t, h, a, ca)
	busListen(t, h, b, cb)
	fromA, fromB := busPayload(t, stream.MaxPayloadSize), busPayload(t, 64)
	busSend(t, h, a, fromA)
	busSend(t, h, b, fromB)
	for name, c := range map[string]*busCollector{"a": ca, "b": cb} {
		if !c.wait(fromA, h.Timeout) {
			t.Errorf("listener %s did not receive a's payload, byte for byte", name)
		}
		if !c.wait(fromB, h.Timeout) {
			t.Errorf("listener %s did not receive b's payload, byte for byte", name)
		}
	}
}

func busListenOutlivesItsContext(t *testing.T, h StreamBus) {
	b := h.NewBus(t)
	c := newBusCollector()
	ctx, cancel := context.WithTimeout(t.Context(), h.Timeout)
	l, err := b.Listen(ctx, c.receive)
	cancel()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = busClose(t, h, l) })
	p := busPayload(t, 64)
	busSend(t, h, h.NewBus(t), p)
	if !c.wait(p, h.Timeout) {
		t.Error("a payload sent after Listen returned did not arrive once Listen's context ended")
	}
}

func busCloses(t *testing.T, h StreamBus) {
	a, b := h.NewBus(t), h.NewBus(t)
	ca, cb := newBusCollector(), newBusCollector()
	before := 0
	if h.Resources != nil {
		before = h.Resources(t)
	}
	la := busListen(t, h, a, ca)
	busListen(t, h, b, cb)
	if h.Resources != nil {
		if got := h.Resources(t); got != before+2 {
			t.Errorf("resources with two listeners = %d, want %d", got, before+2)
		}
	}
	if err := busClose(t, h, la); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if h.Resources != nil {
		if got := h.Resources(t); got != before+1 {
			t.Errorf("resources after Close = %d, want %d", got, before+1)
		}
	}
	p := busPayload(t, 64)
	busSend(t, h, b, p)
	if !cb.wait(p, h.Timeout) {
		t.Fatal("the open listener did not receive the payload")
	}
	if ca.wait(p, streamGrace) {
		t.Error("a closed listener received a payload")
	}
	if err := busClose(t, h, la); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func busReturnsWhenUnreachable(t *testing.T, h StreamBus) {
	if h.Unreachable == nil {
		t.Skip("the harness has no unreachable broker")
	}
	b := h.Unreachable(t)
	const deadline, slack = time.Second, 2 * time.Second
	check := func(name string, call func(ctx context.Context) error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), deadline)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- call(ctx) }()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s on an unreachable broker succeeded", name)
			}
		case <-time.After(deadline + slack):
			t.Errorf("%s on an unreachable broker did not return %s after its deadline", name, slack)
		}
	}
	check("Send", func(ctx context.Context) error { return b.Send(ctx, busPayload(t, 64)) })
	check("Listen", func(ctx context.Context) error {
		l, err := b.Listen(ctx, func([]byte) {})
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), h.Timeout)
			defer cancel()
			_ = l.Close(ctx)
		}
		return err
	})
}

func busBreaks(t *testing.T, h StreamBus) {
	if h.Break == nil {
		t.Skip("the harness cannot break listeners")
	}
	b := h.NewBus(t)
	l := busListen(t, h, b, newBusCollector())
	select {
	case <-l.Broken():
		t.Fatal("Broken is closed before the listener broke")
	default:
	}
	h.Break(t)
	select {
	case <-l.Broken():
	case <-time.After(h.Timeout):
		t.Fatal("Broken was not closed after the listener was cut")
	}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = busClose(t, h, l) // what is left may be gone already: an error is fine
	}()
	select {
	case <-closed:
	case <-time.After(2 * h.Timeout):
		t.Fatal("Close of a broken listener did not return")
	}

	// The process can listen again.
	c := newBusCollector()
	busListen(t, h, b, c)
	p := busPayload(t, 64)
	busSend(t, h, b, p)
	if !c.wait(p, h.Timeout) {
		t.Error("a new listener after a break did not receive the payload")
	}
}

// burstSize is the number of payloads of a burst in the stream contracts.
const burstSize = 32

// busBurst returns a burst of distinct payloads; a payload's index is its sequence number.
func busBurst(t *testing.T) [][]byte {
	t.Helper()
	burst := make([][]byte, burstSize)
	for i := range burst {
		burst[i] = busPayload(t, 64+i)
	}
	return burst
}

// busMissing returns the sequence numbers of the payloads of burst that c has not received
// within timeout.
func busMissing(c *busCollector, burst [][]byte, timeout time.Duration) []int {
	deadline := time.Now().Add(timeout)
	var missing []int
	for i, p := range burst {
		if !c.wait(p, max(time.Until(deadline), 0)) {
			missing = append(missing, i)
		}
	}
	return missing
}

// busForeign returns how many payloads c received that are in none of bursts, and how many it
// received in all.
func busForeign(c *busCollector, bursts ...[][]byte) (foreign, received int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.payloads {
		known := false
		for _, burst := range bursts {
			if slices.ContainsFunc(burst, func(q []byte) bool { return bytes.Equal(p, q) }) {
				known = true
				break
			}
		}
		if !known {
			foreign++
		}
	}
	return foreign, len(c.payloads)
}

func busDeliversABurst(t *testing.T, h StreamBus) {
	sender, b := h.NewBus(t), h.NewBus(t)
	c := newBusCollector()
	busListen(t, h, b, c)
	burst := busBurst(t)
	for _, p := range burst {
		busSend(t, h, sender, p)
	}
	if missing := busMissing(c, burst, 2*h.Timeout); len(missing) > 0 {
		t.Errorf("an open listener missed the payloads %v of a burst of %d", missing, len(burst))
	}
	if foreign, _ := busForeign(c, burst); foreign > 0 {
		t.Errorf("the listener received %d payloads that were not sent, or not byte for byte", foreign)
	}
}

func busBreaksDuringABurst(t *testing.T, h StreamBus) {
	if h.Break == nil {
		t.Skip("the harness cannot break listeners")
	}
	sender, b := h.NewBus(t), h.NewBus(t)
	c := newBusCollector()
	l := busListen(t, h, b, c)
	cut := busBurst(t)
	for _, p := range cut[:len(cut)/2] {
		busSend(t, h, sender, p)
	}
	h.Break(t)
	for _, p := range cut[len(cut)/2:] {
		// The sender may still be reconnecting: a payload it cannot send is lost, which the Bus
		// allows.
		ctx, cancel := context.WithTimeout(t.Context(), h.Timeout)
		_ = sender.Send(ctx, p)
		cancel()
	}
	select {
	case <-l.Broken():
	case <-time.After(h.Timeout):
		t.Fatal("Broken was not closed after the listener was cut during a burst")
	}
	_ = busClose(t, h, l) // what is left may be gone already: an error is fine
	foreign, received := busForeign(c, cut)
	if foreign > 0 {
		t.Errorf("the cut listener received %d payloads that were not sent, or not byte for byte", foreign)
	}
	time.Sleep(streamGrace)
	if _, after := busForeign(c, cut); after != received {
		t.Errorf("the cut listener received %d payloads after Close returned", after-received)
	}

	// A new listener receives every payload of the next burst.
	c2 := newBusCollector()
	busListen(t, h, b, c2)
	next := busBurst(t)
	for _, p := range next {
		busSend(t, h, sender, p)
	}
	if missing := busMissing(c2, next, 2*h.Timeout); len(missing) > 0 {
		t.Errorf("a new listener after the break missed the payloads %v of a burst of %d", missing, len(next))
	}
	if foreign, _ := busForeign(c2, cut, next); foreign > 0 {
		t.Errorf("the new listener received %d payloads that were not sent, or not byte for byte", foreign)
	}
}

// StreamBroker is what StreamBrokerContract needs of a stream.Publisher and stream.Subscriber
// pair: stream.Hub, or stream.Relay on a Bus.
type StreamBroker struct {
	// Publisher and Subscriber are the broker of one process. Required.
	Publisher  stream.Publisher
	Subscriber stream.Subscriber
	// Remote, when not nil, publishes from another process on the same broker; its signals
	// reach the Subscriber asynchronously, possibly twice.
	Remote stream.Publisher
	// Break, when not nil, breaks the broker's listener, so that the process may have missed
	// signals.
	Break func(t *testing.T)
	// Timeout bounds each wait for a remote signal or for the end of a subscription; zero means
	// 5 seconds.
	Timeout time.Duration
}

// StreamBrokerFactory returns a new broker bounded by o. It registers its cleanup with
// t.Cleanup.
type StreamBrokerFactory func(t *testing.T, o stream.HubOptions) StreamBroker

// StreamBrokerContract verifies a stream broker: a notification signal reaches the
// subscriptions of its account and a home entry every subscription, once when published in the
// process and also when published in another; the signals of a voice chat room reach the room
// subscriptions of that room only, from this process and from another; the subscriptions per account and per process
// are bounded and Close releases them; a subscriber that falls behind is ended; and a broken
// listener ends every subscription, after which new ones open and receive again.
func StreamBrokerContract(t *testing.T, newBroker StreamBrokerFactory) {
	t.Helper()
	cases := []struct {
		name string
		o    stream.HubOptions
		run  func(t *testing.T, b StreamBroker)
	}{
		{"Routes", stream.HubOptions{}, brokerRoutes},
		{"RoutesRemoteSignals", stream.HubOptions{}, brokerRoutesRemote},
		{"RoutesRoomSignals", stream.HubOptions{}, brokerRoutesRooms},
		{"Limits", stream.HubOptions{MaxPerAccount: 2, MaxTotal: 3}, brokerLimits},
		{"DropsSlowSubscribers", stream.HubOptions{Buffer: 2}, brokerDropsSlow},
		{"EndsOnBroken", stream.HubOptions{}, brokerEndsOnBroken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBroker(t, tc.o)
			if b.Timeout <= 0 {
				b.Timeout = defaultStreamTimeout
			}
			tc.run(t, b)
		})
	}
}

func brokerSubscribe(t *testing.T, b StreamBroker, accountID string) stream.Subscription {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), b.Timeout)
	defer cancel()
	s, err := b.Subscriber.Subscribe(ctx, accountID)
	if err != nil {
		t.Fatalf("Subscribe(%q): %v", accountID, err)
	}
	t.Cleanup(s.Close)
	return s
}

// brokerNext returns the next signal of s within timeout.
func brokerNext(s stream.Subscription, timeout time.Duration) (stream.Event, bool) {
	select {
	case e := <-s.Events():
		return e, true
	default:
	}
	if timeout <= 0 {
		return stream.Event{}, false
	}
	select {
	case e := <-s.Events():
		return e, true
	case <-time.After(timeout):
		return stream.Event{}, false
	}
}

// brokerDone reports whether s is done, or becomes done within timeout.
func brokerDone(s stream.Subscription, timeout time.Duration) bool {
	select {
	case <-s.Done():
		return true
	default:
	}
	if timeout <= 0 {
		return false
	}
	select {
	case <-s.Done():
		return true
	case <-time.After(timeout):
		return false
	}
}

func brokerRoutes(t *testing.T, b StreamBroker) {
	alice, bob := brokerSubscribe(t, b, NamedID("alice")), brokerSubscribe(t, b, NamedID("bob"))

	note := stream.Event{Kind: stream.KindNotifications, AccountID: NamedID("alice")}
	b.Publisher.Publish(note)
	if e, ok := brokerNext(alice, b.Timeout); !ok || e != note {
		t.Errorf("alice received %+v, %v; want %+v", e, ok, note)
	}
	entry := stream.Event{Kind: stream.KindHomeEntry, EntryID: NamedID("s1")}
	b.Publisher.Publish(entry)
	for name, s := range map[string]stream.Subscription{"alice": alice, "bob": bob} {
		if e, ok := brokerNext(s, b.Timeout); !ok || e != entry {
			t.Errorf("%s received %+v, %v; want %+v", name, e, ok, entry)
		}
	}
	// Nothing else arrives: not alice's notification for bob, and no second copy of a signal of
	// this process.
	for name, s := range map[string]stream.Subscription{"alice": alice, "bob": bob} {
		if e, ok := brokerNext(s, streamGrace); ok {
			t.Errorf("%s received %+v more", name, e)
		}
	}
}

func brokerRoutesRooms(t *testing.T, b StreamBroker) {
	rooms, ok := b.Subscriber.(stream.RoomSubscriber)
	if !ok {
		t.Fatalf("the subscriber %T opens no room subscriptions", b.Subscriber)
	}
	open := func(roomID, session string) stream.Subscription {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), b.Timeout)
		defer cancel()
		s, err := rooms.SubscribeRoom(ctx, roomID, session)
		if err != nil {
			t.Fatalf("SubscribeRoom(%q): %v", roomID, err)
		}
		t.Cleanup(s.Close)
		return s
	}
	alice := brokerSubscribe(t, b, NamedID("alice"))
	r1, r2 := open(NamedID("room1"), "s1"), open(NamedID("room2"), "s1")
	other := open(NamedID("room1"), "s2")
	events := []stream.Event{
		{Kind: stream.KindVoiceRoom, RoomID: NamedID("room1")},
		{Kind: stream.KindVoiceSignal, RoomID: NamedID("room1"), MessageID: "m1", To: "s1"},
		{Kind: stream.KindVoiceMic, RoomID: NamedID("room1"), ParticipantID: NamedID("alice"), On: true},
		{Kind: stream.KindVoiceReaction, RoomID: NamedID("room1"), ParticipantID: NamedID("alice"), Emoji: "👏"},
		{Kind: stream.KindVoiceMute, RoomID: NamedID("room1"), SessionID: "s1", By: NamedID("bob")},
	}
	publishers := map[string]stream.Publisher{"this process": b.Publisher}
	if b.Remote != nil {
		publishers["another process"] = b.Remote
	}
	for from, pub := range publishers {
		pub.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: NamedID("s1")})
		for _, e := range events {
			pub.Publish(e)
		}
		// The signals of another process arrive in any order, possibly twice.
		missing := slices.Clone(events)
		deadline := time.Now().Add(b.Timeout)
		for len(missing) > 0 {
			e, ok := brokerNext(r1, time.Until(deadline))
			if !ok {
				t.Fatalf("room1 did not receive %+v from %s", missing, from)
			}
			if !slices.Contains(events, e) {
				t.Errorf("room1 received %+v from %s", e, from)
			}
			missing = slices.DeleteFunc(missing, func(m stream.Event) bool { return m == e })
		}
		if e, ok := brokerNext(r2, streamGrace); ok {
			t.Errorf("room2 received %+v from %s", e, from)
		}
		// The signal and the mute for s1 do not reach the subscription of s2.
		for {
			e, ok := brokerNext(other, streamGrace)
			if !ok {
				break
			}
			if e.Kind == stream.KindVoiceSignal || e.Kind == stream.KindVoiceMute {
				t.Errorf("the subscription of s2 received %+v from %s", e, from)
			}
		}
		for {
			e, ok := brokerNext(alice, streamGrace)
			if !ok {
				break
			}
			if e.Kind != stream.KindHomeEntry {
				t.Errorf("alice's subscription received %+v from %s", e, from)
			}
		}
		for {
			if _, ok := brokerNext(r1, streamGrace); !ok {
				break
			}
		}
	}
}

func brokerRoutesRemote(t *testing.T, b StreamBroker) {
	if b.Remote == nil {
		t.Skip("the broker has no other process")
	}
	alice, bob := brokerSubscribe(t, b, NamedID("alice")), brokerSubscribe(t, b, NamedID("bob"))
	note := stream.Event{Kind: stream.KindNotifications, AccountID: NamedID("alice")}
	entry := stream.Event{Kind: stream.KindHomeEntry, EntryID: NamedID("s1")}
	b.Remote.Publish(note)
	b.Remote.Publish(entry)

	// Signals of another process may arrive in any order and twice.
	want := func(name string, s stream.Subscription, wanted ...stream.Event) {
		t.Helper()
		missing := slices.Clone(wanted)
		deadline := time.Now().Add(b.Timeout)
		for len(missing) > 0 {
			e, ok := brokerNext(s, time.Until(deadline))
			if !ok {
				t.Errorf("%s did not receive %+v", name, missing)
				return
			}
			if !slices.Contains(wanted, e) {
				t.Errorf("%s received %+v", name, e)
			}
			missing = slices.DeleteFunc(missing, func(m stream.Event) bool { return m == e })
		}
		for {
			e, ok := brokerNext(s, streamGrace)
			if !ok {
				return
			}
			if !slices.Contains(wanted, e) {
				t.Errorf("%s received %+v", name, e)
			}
		}
	}
	want("alice", alice, note, entry)
	want("bob", bob, entry)
}

func brokerLimits(t *testing.T, b StreamBroker) {
	first := brokerSubscribe(t, b, NamedID("alice"))
	brokerSubscribe(t, b, NamedID("alice"))
	if _, err := b.Subscriber.Subscribe(t.Context(), NamedID("alice")); !errors.Is(err, stream.ErrTooManySubscriptions) {
		t.Errorf("third subscription of alice: %v, want ErrTooManySubscriptions", err)
	}
	brokerSubscribe(t, b, NamedID("bob"))
	if _, err := b.Subscriber.Subscribe(t.Context(), NamedID("carol")); !errors.Is(err, stream.ErrTooManySubscriptions) {
		t.Errorf("fourth subscription: %v, want ErrTooManySubscriptions", err)
	}
	// Closing releases the slot, once.
	first.Close()
	first.Close()
	if !brokerDone(first, 0) {
		t.Error("a closed subscription is not done")
	}
	brokerSubscribe(t, b, NamedID("alice"))
	if _, err := b.Subscriber.Subscribe(t.Context(), NamedID("carol")); !errors.Is(err, stream.ErrTooManySubscriptions) {
		t.Errorf("a subscription past the limit after Close: %v, want ErrTooManySubscriptions", err)
	}
}

func brokerDropsSlow(t *testing.T, b StreamBroker) {
	slow, other := brokerSubscribe(t, b, NamedID("alice")), brokerSubscribe(t, b, NamedID("bob"))
	for range 3 {
		b.Publisher.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: NamedID("s")})
		if _, ok := brokerNext(other, b.Timeout); !ok {
			t.Fatal("the subscriber that keeps up missed a signal")
		}
	}
	if !brokerDone(slow, b.Timeout) {
		t.Fatal("a subscription with a full buffer is not done")
	}
	if brokerDone(other, 0) {
		t.Error("the subscriber that keeps up was ended")
	}
	// The signals it held stay readable.
	for range 2 {
		if _, ok := brokerNext(slow, 0); !ok {
			t.Error("a buffered signal was lost")
		}
	}
}

func brokerEndsOnBroken(t *testing.T, b StreamBroker) {
	if b.Break == nil {
		t.Skip("the broker has no listener to break")
	}
	alice, bob := brokerSubscribe(t, b, NamedID("alice")), brokerSubscribe(t, b, NamedID("bob"))
	b.Break(t)
	for name, s := range map[string]stream.Subscription{"alice": alice, "bob": bob} {
		if !brokerDone(s, b.Timeout) {
			t.Errorf("%s's subscription did not end when the listener broke", name)
		}
	}
	alice.Close()
	bob.Close()

	// A new subscription listens again and receives.
	again := brokerSubscribe(t, b, NamedID("alice"))
	pub := b.Publisher
	if b.Remote != nil {
		pub = b.Remote
	}
	entry := stream.Event{Kind: stream.KindHomeEntry, EntryID: NamedID("after")}
	pub.Publish(entry)
	if e, ok := brokerNext(again, b.Timeout); !ok || e != entry {
		t.Errorf("after the break, received %+v, %v; want %+v", e, ok, entry)
	}
}
