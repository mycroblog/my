// SPDX-License-Identifier: AGPL-3.0-only

package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Defaults of the zero fields of RelayOptions.
const (
	DefaultSendBuffer   = 1024
	DefaultSendTimeout  = 5 * time.Second
	DefaultReadyTimeout = 10 * time.Second
	DefaultIdleTimeout  = 60 * time.Second
)

// The liveness marker of a listening process (ADR-0050 §2): while a Relay listens, it writes
// the key of its process ID in LivenessNamespace every LivenessInterval, with LivenessTTL, so
// that JobSweep keeps the per-process resources of live processes.
const (
	LivenessNamespace = "stream.alive"
	LivenessInterval  = time.Minute
	LivenessTTL       = 5 * time.Minute
)

const (
	// closeTimeout bounds the Close of a listener that the Relay stops by itself (idle or
	// broken), which deletes its resources on the broker.
	closeTimeout = 10 * time.Second
	// dropLogInterval is the least time between two log lines about dropped signals, so that
	// an unreachable broker does not flood the log; the counts in RelayStats are exact.
	dropLogInterval = 10 * time.Second
)

// RelayOptions configure a Relay. Zero fields take the defaults above.
type RelayOptions struct {
	// Hub bounds the subscriptions of this process.
	Hub HubOptions
	// ProcessID identifies this process on the Bus (NewProcessID); the Bus adapter names its
	// per-process resources with the same ID. Empty takes a new one.
	ProcessID string
	// SendBuffer is the number of signals waiting to be sent on the Bus; a signal published
	// while it is full is dropped.
	SendBuffer int
	// SendTimeout bounds each Bus.Send; a signal whose send fails or times out is dropped.
	SendTimeout time.Duration
	// ReadyTimeout bounds the start of the listener, which Subscribe waits for.
	ReadyTimeout time.Duration
	// IdleTimeout is how long the listener stays open after the last subscription closes.
	IdleTimeout time.Duration
	// Liveness, when not nil, holds the liveness marker of this process while it listens: the
	// Bus adapters with a Sweeper need it.
	Liveness kernel.KeyValueStore
	// Logger receives the dropped signals, broken listeners, and failures; nil discards them.
	Logger *slog.Logger
}

// RelayStats counts the signals a Relay dropped instead of sending them on its Bus, since it
// started; a metrics exporter reads them.
type RelayStats struct {
	// DroppedBufferFull counts the signals published while the send buffer was full.
	DroppedBufferFull uint64
	// DroppedSendFailed counts the signals whose Bus.Send failed or timed out, and the ones that
	// could not be encoded.
	DroppedSendFailed uint64
}

// Relay is the broker across processes (ADR-0050 §1): it implements Publisher and Subscriber
// with a Hub for the subscriptions of this process and a Bus that carries the signals between
// processes.
//
//   - Publish hands a signal to the Hub at once and queues it, encoded, for the Bus; it never
//     blocks, and drops the signal (counted in Stats and logged) when the queue is full or the
//     send fails.
//   - A payload received from the Bus goes to the Hub, unless this process sent it (it is
//     already there) or it is of an unknown version or kind.
//   - The Relay listens only while it has subscriptions: the first one starts the listener, and
//     the listener closes IdleTimeout after the last one closes. A process without streams
//     only sends.
//   - Subscribe waits up to ReadyTimeout for the listener and reports ErrUnavailable when it is
//     not ready.
//   - When the listener breaks, every subscription of this process ends, since it may have
//     missed signals; its clients reconnect and read again, and the next Subscribe listens
//     anew.
//
// It is safe for concurrent use.
type Relay struct {
	o   RelayOptions
	hub *Hub
	bus Bus
	log *slog.Logger

	// base parents the starts of listeners and the liveness marker; Close cancels it.
	base   context.Context
	cancel context.CancelFunc

	sendCh   chan []byte
	stopSend chan struct{}
	sendDone chan struct{}
	// flushCtx bounds the sends of the signals still queued at Close; it is set before stopSend
	// is closed.
	flushCtx context.Context

	mu     sync.Mutex
	closed bool
	// active counts the subscriptions that are open or waiting for the listener.
	active int
	// cur is the listener being started or open; nil while the Relay does not listen.
	cur *listening
	// idle closes cur IdleTimeout after active reaches 0; idleToken tells a stopped timer's late
	// call from the current one.
	idle      *time.Timer
	idleToken *struct{}

	isClosed      atomic.Bool
	droppedFull   atomic.Uint64
	droppedFailed atomic.Uint64

	dropMu      sync.Mutex
	lastDropLog time.Time
	unlogged    int
}

var (
	_ Publisher      = (*Relay)(nil)
	_ Subscriber     = (*Relay)(nil)
	_ RoomSubscriber = (*Relay)(nil)
)

// listening is one listener of a Relay, from its start to its close.
type listening struct {
	// ready is closed when Bus.Listen returned, with l or err set.
	ready chan struct{}
	l     Listener
	err   error
	// stop is closed when the Relay stops using the listener; it ends the watch and the
	// liveness marker.
	stop     chan struct{}
	stopOnce sync.Once
}

func (c *listening) halt() { c.stopOnce.Do(func() { close(c.stop) }) }

// NewRelay returns a Relay on bus. Close it on shutdown.
func NewRelay(bus Bus, o RelayOptions) *Relay {
	if o.ProcessID == "" {
		o.ProcessID = NewProcessID()
	}
	if o.SendBuffer <= 0 {
		o.SendBuffer = DefaultSendBuffer
	}
	if o.SendTimeout <= 0 {
		o.SendTimeout = DefaultSendTimeout
	}
	if o.ReadyTimeout <= 0 {
		o.ReadyTimeout = DefaultReadyTimeout
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = DefaultIdleTimeout
	}
	logger := o.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	base, cancel := context.WithCancel(context.Background())
	r := &Relay{
		o: o, hub: NewHub(o.Hub), bus: bus, log: logger.With(slog.String("process_id", o.ProcessID)),
		base: base, cancel: cancel,
		sendCh: make(chan []byte, o.SendBuffer), stopSend: make(chan struct{}), sendDone: make(chan struct{}),
	}
	go r.send()
	return r
}

// ProcessID returns the ID of this process on the Bus.
func (r *Relay) ProcessID() string { return r.o.ProcessID }

// Len returns the number of open subscriptions of this process.
func (r *Relay) Len() int { return r.hub.Len() }

// Stats returns the counts of dropped signals.
func (r *Relay) Stats() RelayStats {
	return RelayStats{DroppedBufferFull: r.droppedFull.Load(), DroppedSendFailed: r.droppedFailed.Load()}
}

// Publish implements Publisher.
func (r *Relay) Publish(e Event) {
	r.hub.Publish(e)
	if r.isClosed.Load() {
		return
	}
	payload, err := encodeSignal(r.o.ProcessID, e)
	if err != nil {
		r.drop(&r.droppedFailed, "encode", err)
		return
	}
	select {
	case r.sendCh <- payload:
	default:
		r.drop(&r.droppedFull, "send buffer full", nil)
	}
}

// send sends the queued signals on the Bus, one at a time, until Close; then it sends what is
// still queued within the flush context.
func (r *Relay) send() {
	defer close(r.sendDone)
	for {
		select {
		case p := <-r.sendCh:
			r.sendOne(context.Background(), p)
		case <-r.stopSend:
			for {
				select {
				case p := <-r.sendCh:
					if err := r.flushCtx.Err(); err != nil {
						r.drop(&r.droppedFailed, "send", err)
						continue
					}
					r.sendOne(r.flushCtx, p)
				default:
					return
				}
			}
		}
	}
}

func (r *Relay) sendOne(parent context.Context, payload []byte) {
	ctx, cancel := context.WithTimeout(parent, r.o.SendTimeout)
	defer cancel()
	if err := r.bus.Send(ctx, payload); err != nil {
		r.drop(&r.droppedFailed, "send", err)
	}
}

// drop counts a dropped signal and logs the drops at most once per dropLogInterval.
func (r *Relay) drop(counter *atomic.Uint64, reason string, err error) {
	counter.Add(1)
	r.dropMu.Lock()
	r.unlogged++
	now := time.Now()
	if !r.lastDropLog.IsZero() && now.Sub(r.lastDropLog) < dropLogInterval {
		r.dropMu.Unlock()
		return
	}
	n := r.unlogged
	r.unlogged, r.lastDropLog = 0, now
	r.dropMu.Unlock()
	r.log.Warn("stream relay dropped signals for other processes", slog.String("reason", reason), slog.Int("count", n), slog.Any("error", err))
}

// receive hands a payload of the Bus to the Hub.
func (r *Relay) receive(payload []byte) {
	origin, e, err := decodeSignal(payload)
	if err != nil {
		r.log.Debug("stream relay dropped a payload", slog.Any("error", err))
		return
	}
	if origin == r.o.ProcessID {
		return // published to the Hub already
	}
	r.hub.Publish(e)
}

// Subscribe implements Subscriber: it starts the listener when none is open and waits for it,
// up to ReadyTimeout. A subscription that joined a start already in progress, which may have
// begun while the broker was unreachable, starts a fresh one once if that start fails.
func (r *Relay) Subscribe(ctx context.Context, accountID string) (Subscription, error) {
	return r.subscribe(ctx, func() (Subscription, error) { return r.hub.Subscribe(ctx, accountID) })
}

// SubscribeRoom implements RoomSubscriber, as Subscribe does.
func (r *Relay) SubscribeRoom(ctx context.Context, roomID string) (Subscription, error) {
	return r.subscribe(ctx, func() (Subscription, error) { return r.hub.SubscribeRoom(ctx, roomID) })
}

// subscribe opens the subscription of open on the Hub once the listener is ready.
func (r *Relay) subscribe(ctx context.Context, open func() (Subscription, error)) (Subscription, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrClosed
	}
	r.active++
	r.stopIdleLocked()
	cur, joined := r.cur, r.cur != nil
	if cur == nil {
		cur = r.startLocked()
	}
	r.mu.Unlock()

	timer := time.NewTimer(r.o.ReadyTimeout)
	defer timer.Stop()
	for {
		select {
		case <-cur.ready:
		case <-ctx.Done():
			r.release()
			return nil, ctx.Err()
		case <-timer.C:
			r.release()
			return nil, fmt.Errorf("%w: the listener was not ready within %s", ErrUnavailable, r.o.ReadyTimeout)
		}

		r.mu.Lock()
		var failure error
		switch {
		case r.closed:
			r.releaseLocked()
			r.mu.Unlock()
			return nil, ErrClosed
		case cur.err != nil:
			failure = cur.err
		case r.cur != cur:
			failure = errors.New("the listener broke")
		}
		if failure == nil {
			break // r.mu stays locked for the subscription below
		}
		if !joined {
			r.releaseLocked()
			r.mu.Unlock()
			return nil, fmt.Errorf("%w: %w", ErrUnavailable, failure)
		}
		// Retry once with a start of this subscription's own.
		joined = false
		if r.cur == nil || r.cur == cur {
			cur = r.startLocked()
		} else {
			cur = r.cur
		}
		r.mu.Unlock()
	}
	defer r.mu.Unlock()
	s, err := open()
	if err != nil {
		r.releaseLocked()
		return nil, err
	}
	return &relaySubscription{Subscription: s, r: r}, nil
}

// startLocked starts a listener and makes it cur; r.mu is held.
func (r *Relay) startLocked() *listening {
	cur := &listening{ready: make(chan struct{}), stop: make(chan struct{})}
	r.cur = cur
	if r.o.Liveness != nil {
		go r.heartbeat(cur) // the marker precedes the resources that Listen creates
	}
	go r.listen(cur)
	return cur
}

func (r *Relay) listen(cur *listening) {
	ctx, cancel := context.WithTimeout(r.base, r.o.ReadyTimeout)
	l, err := r.bus.Listen(ctx, r.receive)
	cancel()

	r.mu.Lock()
	cur.l, cur.err = l, err
	if err != nil {
		if r.cur == cur {
			r.cur = nil // the next Subscribe tries again
		}
		r.mu.Unlock()
		cur.halt()
		close(cur.ready)
		r.log.Warn("stream relay could not listen", slog.Any("error", err))
		return
	}
	current := r.cur == cur
	if current && r.active == 0 {
		r.startIdleLocked()
	}
	r.mu.Unlock()
	close(cur.ready)
	if !current { // the Relay closed meanwhile
		r.closeListener(cur)
		return
	}
	r.watch(cur)
}

// watch ends every subscription when cur's listener breaks while it is current.
func (r *Relay) watch(cur *listening) {
	select {
	case <-cur.stop:
		return
	case <-cur.l.Broken():
	}
	r.mu.Lock()
	current := r.cur == cur
	if current {
		r.cur = nil
		r.stopIdleLocked()
		r.hub.endAll()
	}
	r.mu.Unlock()
	if current {
		r.log.Warn("stream relay listener broke; ending the subscriptions of this process")
	}
	r.closeListener(cur)
}

// closeListener stops using cur and closes its listener, which deletes its resources.
func (r *Relay) closeListener(cur *listening) {
	cur.halt()
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	if err := cur.l.Close(ctx); err != nil {
		r.log.Warn("stream relay could not close its listener", slog.Any("error", err))
	}
}

// heartbeat writes the liveness marker of this process until cur stops.
func (r *Relay) heartbeat(cur *listening) {
	write := func() {
		ctx, cancel := context.WithTimeout(r.base, LivenessInterval/2)
		defer cancel()
		if err := r.o.Liveness.Set(ctx, LivenessNamespace, r.o.ProcessID, []byte("1"), LivenessTTL); err != nil && r.base.Err() == nil {
			r.log.Warn("stream relay could not write its liveness marker", slog.Any("error", err))
		}
	}
	write()
	t := time.NewTicker(LivenessInterval)
	defer t.Stop()
	for {
		select {
		case <-cur.stop:
			return
		case <-t.C:
			write()
		}
	}
}

// release ends a subscription's hold on the listener.
func (r *Relay) release() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releaseLocked()
}

// releaseLocked ends a subscription's hold on the listener and starts the idle timer after the
// last one; r.mu is held.
func (r *Relay) releaseLocked() {
	r.active--
	if r.active == 0 && !r.closed && r.cur != nil && r.cur.l != nil {
		r.startIdleLocked()
	}
}

// startIdleLocked starts the timer that closes the listener; r.mu is held.
func (r *Relay) startIdleLocked() {
	r.stopIdleLocked()
	token := new(struct{})
	r.idleToken = token
	r.idle = time.AfterFunc(r.o.IdleTimeout, func() { r.idleClose(token) })
}

// stopIdleLocked stops the idle timer; r.mu is held.
func (r *Relay) stopIdleLocked() {
	if r.idle != nil {
		r.idle.Stop()
	}
	r.idle, r.idleToken = nil, nil
}

func (r *Relay) idleClose(token *struct{}) {
	r.mu.Lock()
	if r.idleToken != token || r.active > 0 || r.cur == nil || r.cur.l == nil {
		r.mu.Unlock()
		return
	}
	cur := r.cur
	r.cur, r.idle, r.idleToken = nil, nil, nil
	r.mu.Unlock()
	r.closeListener(cur)
}

// Close ends every subscription and refuses new ones, sends the signals still queued, and closes
// the listener, all within ctx; for a shutdown.
func (r *Relay) Close(ctx context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.isClosed.Store(true)
	r.stopIdleLocked()
	cur := r.cur
	r.cur = nil
	r.hub.Close()
	r.mu.Unlock()
	r.cancel()

	var errs []error
	r.flushCtx = ctx
	close(r.stopSend)
	select {
	case <-r.sendDone:
	case <-ctx.Done():
		errs = append(errs, fmt.Errorf("stream: send the queued signals: %w", ctx.Err()))
	}
	if cur != nil {
		select {
		case <-cur.ready:
			if cur.l != nil {
				cur.halt()
				if err := cur.l.Close(ctx); err != nil {
					errs = append(errs, fmt.Errorf("stream: close the listener: %w", err))
				}
			}
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("stream: wait for the listener: %w", ctx.Err()))
		}
	}
	return errors.Join(errs...)
}

// relaySubscription releases its hold on the Relay's listener when it closes.
type relaySubscription struct {
	Subscription
	r    *Relay
	once sync.Once
}

func (s *relaySubscription) Close() {
	s.once.Do(func() {
		s.Subscription.Close()
		s.r.release()
	})
}
