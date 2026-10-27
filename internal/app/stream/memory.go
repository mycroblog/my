// SPDX-License-Identifier: AGPL-3.0-only

package stream

import (
	"bytes"
	"context"
	"sync"

	"github.com/mycroblog/my/internal/app/kernel"
)

// memoryListenerBuffer is the number of payloads a MemoryBroker holds for a listener whose
// receive is busy; a listener that falls further behind breaks, as a Valkey subscriber that
// exceeds its output buffer is disconnected.
const memoryListenerBuffer = 1024

// MemoryBroker is an in-process broker for Buses, for tests: each Bus it returns stands for one
// process, and a payload sent on any of them reaches the listeners of all. It passes
// testkit.StreamBusContract, and tests can make it unreachable and break its listeners. It is
// safe for concurrent use.
type MemoryBroker struct {
	mu          sync.Mutex
	listeners   map[*memoryListener]struct{}
	unreachable bool
}

// NewMemoryBroker returns a reachable broker without listeners.
func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{listeners: map[*memoryListener]struct{}{}}
}

// Bus returns a new Bus on b, one process's connection to it.
func (b *MemoryBroker) Bus() Bus { return memoryBus{b: b} }

// Listeners returns the number of open listeners: the per-process resources of the broker.
func (b *MemoryBroker) Listeners() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.listeners)
}

// SetUnreachable makes the broker unreachable, or reachable again: while it is unreachable,
// Send and Listen wait for their context to end and fail, as with a broker that does not
// answer.
func (b *MemoryBroker) SetUnreachable(unreachable bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.unreachable = unreachable
}

// Break breaks every open listener, as a lost connection would: each one stops receiving and
// closes Broken.
func (b *MemoryBroker) Break() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for l := range b.listeners {
		b.breakLocked(l)
	}
}

// breakLocked breaks l and removes it from the broker; b.mu is held.
func (b *MemoryBroker) breakLocked(l *memoryListener) {
	if _, ok := b.listeners[l]; !ok {
		return
	}
	delete(b.listeners, l)
	close(l.broken)
}

type memoryBus struct{ b *MemoryBroker }

// reachable reports whether the broker answers; while it does not, it waits for ctx to end and
// returns its error.
func (m memoryBus) reachable(ctx context.Context) error {
	m.b.mu.Lock()
	unreachable := m.b.unreachable
	m.b.mu.Unlock()
	if !unreachable {
		return ctx.Err()
	}
	<-ctx.Done()
	return ctx.Err()
}

func (m memoryBus) Send(ctx context.Context, payload []byte) error {
	if err := m.reachable(ctx); err != nil {
		return err
	}
	m.b.mu.Lock()
	defer m.b.mu.Unlock()
	for l := range m.b.listeners {
		select {
		case l.payloads <- bytes.Clone(payload):
		default:
			m.b.breakLocked(l)
		}
	}
	return nil
}

func (m memoryBus) Listen(ctx context.Context, receive func(payload []byte)) (Listener, error) {
	if err := m.reachable(ctx); err != nil {
		return nil, err
	}
	l := &memoryListener{
		b: m.b, payloads: make(chan []byte, memoryListenerBuffer),
		broken: make(chan struct{}), stop: make(chan struct{}), stopped: make(chan struct{}),
	}
	m.b.mu.Lock()
	m.b.listeners[l] = struct{}{}
	m.b.mu.Unlock()
	// A test's broker reports nowhere: a panic of receive fails the test as it would without kernel.Go.
	kernel.Go(ctx, "stream.memory.listen", nil, func(context.Context) { l.run(receive) })
	return l, nil
}

type memoryListener struct {
	b        *MemoryBroker
	payloads chan []byte
	broken   chan struct{}
	stop     chan struct{}
	stopped  chan struct{}
	once     sync.Once
}

// run calls receive for each payload until the listener stops or breaks. A broken listener
// delivers nothing more, not even the payloads it held.
func (l *memoryListener) run(receive func(payload []byte)) {
	defer close(l.stopped)
	for {
		select {
		case <-l.stop:
			return
		case <-l.broken:
			return
		case p := <-l.payloads:
			select {
			case <-l.stop:
				return
			case <-l.broken:
				return
			default:
			}
			receive(p)
		}
	}
}

func (l *memoryListener) Broken() <-chan struct{} { return l.broken }

func (l *memoryListener) Close(ctx context.Context) error {
	l.once.Do(func() {
		l.b.mu.Lock()
		delete(l.b.listeners, l)
		l.b.mu.Unlock()
		close(l.stop)
	})
	select {
	case <-l.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
