// SPDX-License-Identifier: AGPL-3.0-only

package stream

import (
	"context"
	"sync"
)

// Defaults of the zero fields of HubOptions.
const (
	DefaultMaxPerAccount = 8
	DefaultMaxTotal      = 1000
	DefaultBuffer        = 64
)

// HubOptions bound a Hub. Zero fields take the defaults above.
type HubOptions struct {
	// MaxPerAccount is the number of subscriptions one account may hold open at once (tabs and
	// devices).
	MaxPerAccount int
	// MaxTotal is the number of subscriptions the process holds open at once.
	MaxTotal int
	// Buffer is the number of signals a subscription holds before its subscriber reads them.
	// A subscription whose buffer is full when a signal arrives ends, so that one slow
	// subscriber never delays the others or the publisher.
	Buffer int
}

// Hub is the in-process broker: it implements Publisher and Subscriber for the subscribers of
// one process. A signal published in another process (another web replica, or a worker that
// processes the inbox) never reaches it. It is safe for concurrent use.
type Hub struct {
	o HubOptions

	mu   sync.Mutex
	subs map[*subscription]struct{}
	// accounts holds the account subscriptions, byAccount them by account, and byRoom the room
	// subscriptions by room.
	accounts  map[*subscription]struct{}
	byAccount map[string]map[*subscription]struct{}
	byRoom    map[string]map[*subscription]struct{}
	closed    bool
}

var (
	_ Publisher      = (*Hub)(nil)
	_ Subscriber     = (*Hub)(nil)
	_ RoomSubscriber = (*Hub)(nil)
)

// NewHub returns an empty Hub bounded by o.
func NewHub(o HubOptions) *Hub {
	if o.MaxPerAccount <= 0 {
		o.MaxPerAccount = DefaultMaxPerAccount
	}
	if o.MaxTotal <= 0 {
		o.MaxTotal = DefaultMaxTotal
	}
	if o.Buffer <= 0 {
		o.Buffer = DefaultBuffer
	}
	return &Hub{
		o: o, subs: map[*subscription]struct{}{}, accounts: map[*subscription]struct{}{},
		byAccount: map[string]map[*subscription]struct{}{}, byRoom: map[string]map[*subscription]struct{}{},
	}
}

// Subscribe implements Subscriber. A Hub is always ready, so it waits for nothing.
func (h *Hub) Subscribe(ctx context.Context, accountID string) (Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	if len(h.subs) >= h.o.MaxTotal || len(h.byAccount[accountID]) >= h.o.MaxPerAccount {
		return nil, ErrTooManySubscriptions
	}
	s := &subscription{hub: h, accountID: accountID, events: make(chan Event, h.o.Buffer), done: make(chan struct{})}
	h.subs[s] = struct{}{}
	h.accounts[s] = struct{}{}
	add(h.byAccount, accountID, s)
	return s, nil
}

// SubscribeRoom implements RoomSubscriber. A Hub is always ready, so it waits for nothing.
func (h *Hub) SubscribeRoom(ctx context.Context, roomID, session string) (Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	if len(h.subs) >= h.o.MaxTotal {
		return nil, ErrTooManySubscriptions
	}
	s := &subscription{hub: h, roomID: roomID, session: session, events: make(chan Event, h.o.Buffer), done: make(chan struct{})}
	h.subs[s] = struct{}{}
	add(h.byRoom, roomID, s)
	return s, nil
}

// add adds s to the set of key in m.
func add(m map[string]map[*subscription]struct{}, key string, s *subscription) {
	if m[key] == nil {
		m[key] = map[*subscription]struct{}{}
	}
	m[key][s] = struct{}{}
}

// remove removes s from the set of key in m, and the set once it is empty.
func remove(m map[string]map[*subscription]struct{}, key string, s *subscription) {
	if set := m[key]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(m, key)
		}
	}
}

// Publish implements Publisher. A signal of KindNotifications goes to the subscriptions of its
// account, one of KindHomeEntry or KindAnnouncement to every account subscription, and a voice
// signal to the subscriptions of its room.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	targets := h.accounts
	switch {
	case e.Kind == KindNotifications:
		targets = h.byAccount[e.AccountID]
	case e.Kind.IsVoice():
		targets = h.byRoom[e.RoomID]
	}
	for s := range targets {
		if !s.wants(e) {
			continue
		}
		select {
		case s.events <- e:
		default:
			h.removeLocked(s) // the subscriber fell behind; it reconnects and reads again
		}
	}
}

// Len returns the number of open subscriptions.
func (h *Hub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Close ends every subscription and refuses new ones, for a shutdown: the open streams return,
// so that the HTTP server's graceful shutdown does not wait for them.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		h.removeLocked(s)
	}
}

// endAll ends every subscription but keeps accepting new ones, for a Relay whose listener
// broke: the open streams may have missed signals, so they end and their clients read again.
func (h *Hub) endAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		h.removeLocked(s)
	}
}

// removeLocked ends s; h.mu is held.
func (h *Hub) removeLocked(s *subscription) {
	if _, ok := h.subs[s]; !ok {
		return
	}
	delete(h.subs, s)
	if s.roomID != "" {
		remove(h.byRoom, s.roomID, s)
	} else {
		delete(h.accounts, s)
		remove(h.byAccount, s.accountID, s)
	}
	close(s.done)
}

type subscription struct {
	hub *Hub
	// accountID is the account of an account subscription, roomID the room of a room
	// subscription.
	accountID string
	roomID    string
	// session is the session of a room subscription.
	session string
	events  chan Event
	done    chan struct{}
}

// wants reports whether a room subscription takes a voice signal: not one for another session.
func (s *subscription) wants(e Event) bool {
	switch e.Kind {
	case KindVoiceSignal:
		return e.To == "" || e.To == s.session
	case KindVoiceMute:
		return e.SessionID == s.session
	}
	return true
}

func (s *subscription) Events() <-chan Event  { return s.events }
func (s *subscription) Done() <-chan struct{} { return s.done }

func (s *subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.removeLocked(s)
}
