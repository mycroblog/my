// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/voice"
)

const (
	// RefreshInterval is how often a node reads a room again without a signal, as voice.sweep
	// runs (ADR-0101 §13): a signal that was lost changes the room at the latest then.
	RefreshInterval = time.Minute
	// SpeakingInterval is how often a node tells the clients who makes sound, at most, and only
	// when it changed.
	SpeakingInterval = 250 * time.Millisecond
	// speakingHold is how long a speaker counts as making sound after a loud packet.
	speakingHold = 500 * time.Millisecond
	// resubscribeDelay is the wait before a subscription to the room's signals starts again.
	resubscribeDelay = time.Second
)

// room is a room that has connections on the node. It follows the room state (ADR-0101 §13):
// it reads the room again through the voice module on each voice_room, voice_mute, and voice_mic
// signal of the room and every RefreshInterval, and applies who may speak, who is muted, who is
// gone, and whether the room ended.
type room struct {
	node *Node
	id   string
	fwd  *forwarder
	stop context.CancelFunc
	kick chan struct{}

	mu       sync.Mutex
	peers    map[string]*peer
	joining  int
	closed   bool
	speaking []string

	// speakersMu orders the changes of the slots, so that a later set of speakers is never
	// applied before an earlier one.
	speakersMu sync.Mutex
}

func newRoom(n *Node, id string) *room {
	ctx, stop := context.WithCancel(n.base)
	r := &room{node: n, id: id, fwd: newForwarder(n.stats, time.Now), stop: stop, kick: make(chan struct{}, 1), peers: map[string]*peer{}}
	n.run("voice.sfu.room_refresh", func() { r.refresh(ctx) })
	if n.o.Streams != nil {
		n.run("voice.sfu.room_follow", func() { r.follow(ctx) })
	}
	n.run("voice.sfu.room_speaking", func() { r.tellSpeaking(ctx) })
	return r
}

// join connects a session of a ticket with its WebSocket: the session's live connection when it
// has one, a new one otherwise. It returns the code of the error to answer, "" when it joined.
func (r *room) join(ctx context.Context, session, account string, ws *wsConn) string {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return CodeRoomUnavailable
	}
	if p := r.peers[session]; p != nil {
		r.mu.Unlock()
		p.attach(ws)
		return ""
	}
	r.mu.Unlock()
	st, err := r.node.o.Rooms.MediaRoom(ctx, r.id)
	if err != nil {
		r.node.o.Logger.WarnContext(ctx, "a voice chat media node could not read a room", "error", err)
		return CodeRoomUnavailable
	}
	r.apply(ctx, st)
	ms, ok := st.Session(session)
	if !st.Open || !ok || ms.AccountID != account {
		return CodeRoomUnavailable
	}
	if !r.node.takeSession() {
		// The node holds as many sessions as its capacity: the last line behind the placement.
		return CodeRoomUnavailable
	}
	p, err := newPeer(r, session, account)
	if err != nil {
		r.node.releaseSession()
		r.node.o.Logger.WarnContext(ctx, "a voice chat media node could not make a connection", "error", err)
		return CodeRoomUnavailable
	}
	r.mu.Lock()
	if old := r.peers[session]; r.closed || old != nil {
		r.mu.Unlock()
		p.closed.Store(true)
		close(p.done)
		_ = p.pc.Close()
		r.node.releaseSession()
		if old != nil {
			old.attach(ws)
			return ""
		}
		return CodeRoomUnavailable
	}
	p.canSpeak = ms.CanSpeak
	p.src.allowed.Store(ms.CanSpeak && !ms.Muted)
	r.peers[session] = p
	r.setSubscribersLocked()
	r.mu.Unlock()
	p.attach(ws)
	// The slots and the first offer are made apart from the join, so that the session's
	// WebSocket is read at once: in a burst of joins, each waits its turn for the room's slots,
	// and an answer left unread behind the others' offers kept the ICE-lite node from answering
	// the client's connectivity checks, which a client gives up on within seconds.
	r.node.run("voice.sfu.room_speakers", r.applySpeakers)
	return ""
}

// setSubscribersLocked gives the forwarder the subscribers of the room; r.mu is held.
func (r *room) setSubscribersLocked() {
	subs := make([]*subscriber, 0, len(r.peers))
	for _, p := range r.peers {
		subs = append(subs, p.sub)
	}
	r.fwd.setSubscribers(subs)
}

// leave takes a closed connection out of the room, and the room off the node when it was the
// last.
func (r *room) leave(p *peer) {
	r.mu.Lock()
	if r.peers[p.session] != p {
		r.mu.Unlock()
		return
	}
	delete(r.peers, p.session)
	r.setSubscribersLocked()
	speaker := p.canSpeak
	r.mu.Unlock()
	r.node.releaseSession()
	if speaker {
		r.applySpeakers()
	}
	r.node.dropIfEmpty(r)
}

// peerList returns the connections of the room, by session.
func (r *room) peerList() []*peer {
	r.mu.Lock()
	defer r.mu.Unlock()
	ps := make([]*peer, 0, len(r.peers))
	for _, p := range r.peers {
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(a, b *peer) int { return cmp.Compare(a.session, b.session) })
	return ps
}

// applySpeakers gives each connection a slot for every speaker but itself.
func (r *room) applySpeakers() {
	r.speakersMu.Lock()
	defer r.speakersMu.Unlock()
	ps := r.peerList()
	var speakers []*peer
	r.mu.Lock()
	for _, p := range ps {
		if p.canSpeak {
			speakers = append(speakers, p)
		}
	}
	r.mu.Unlock()
	for _, p := range ps {
		p.setSpeakers(slices.DeleteFunc(slices.Clone(speakers), func(s *peer) bool { return s == p }))
	}
}

// apply applies a state of the room that the voice module returned.
func (r *room) apply(ctx context.Context, st voice.MediaRoom) {
	r.mu.Lock()
	var gone []*peer
	changed := false
	for _, p := range r.peers {
		ms, ok := st.Session(p.session)
		if !st.Open || !ok {
			gone = append(gone, p)
			continue
		}
		p.src.allowed.Store(ms.CanSpeak && !ms.Muted)
		if p.canSpeak != ms.CanSpeak {
			p.canSpeak, changed = ms.CanSpeak, true
		}
	}
	r.mu.Unlock()
	for _, p := range gone {
		p.close(CodeRoomUnavailable, "the session or the room is no longer on the node")
	}
	if changed {
		r.applySpeakers()
	}
	if !st.Open {
		if err := r.node.o.Registry.Release(ctx, r.id); err != nil {
			r.node.o.Logger.WarnContext(ctx, "a voice chat media node could not release a room that ended", "error", err)
		}
	}
}

// signal asks for the room to be read again.
func (r *room) signal() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// refresh reads the room again on each signal and every RefreshInterval.
func (r *room) refresh(ctx context.Context) {
	t := time.NewTicker(RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.kick:
		case <-t.C:
		}
		st, err := r.node.o.Rooms.MediaRoom(ctx, r.id)
		if err != nil {
			if ctx.Err() == nil {
				r.node.o.Logger.WarnContext(ctx, "a voice chat media node could not read a room", "error", err)
			}
			continue
		}
		r.apply(ctx, st)
	}
}

// follow subscribes to the signals of the room, of this process and the others, and asks for a
// read of the room on each that may change it, and once after each subscription since signals
// may have been missed before it.
func (r *room) follow(ctx context.Context) {
	for ctx.Err() == nil {
		sub, err := r.node.o.Streams.SubscribeRoom(ctx, r.id, "")
		if err != nil {
			select {
			case <-ctx.Done():
			case <-time.After(resubscribeDelay):
			}
			continue
		}
		r.signal()
		r.listen(ctx, sub)
		sub.Close()
	}
}

func (r *room) listen(ctx context.Context, sub stream.Subscription) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
			return
		case e := <-sub.Events():
			switch e.Kind {
			case stream.KindVoiceRoom, stream.KindVoiceMute, stream.KindVoiceMic:
				r.signal()
			}
		}
	}
}

// tellSpeaking sends speaking to every connection when the speakers who make sound changed, at
// most every SpeakingInterval.
func (r *room) tellSpeaking(ctx context.Context) {
	t := time.NewTicker(SpeakingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now().UnixNano()
		r.mu.Lock()
		var loud []string
		for _, p := range r.peers {
			if p.src.allowed.Load() && now-p.src.loud.Load() < int64(speakingHold) {
				loud = append(loud, p.account)
			}
		}
		slices.Sort(loud)
		loud = slices.Compact(loud)
		if slices.Equal(loud, r.speaking) {
			r.mu.Unlock()
			continue
		}
		r.speaking = loud
		ps := make([]*peer, 0, len(r.peers))
		for _, p := range r.peers {
			ps = append(ps, p)
		}
		r.mu.Unlock()
		m := Message{V: Version, Type: TypeSpeaking, Participants: loud}
		for _, p := range ps {
			p.send(m)
		}
	}
}

// sessions returns the sessions connected to the room.
func (r *room) sessions() []connected {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]connected, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, connected{session: p.session, account: p.account})
	}
	return out
}

type connected struct{ session, account string }
