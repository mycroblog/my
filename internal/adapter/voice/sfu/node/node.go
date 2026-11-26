// SPDX-License-Identifier: AGPL-3.0-only

// Package node is the media node of voice = sfu (ADR-0101 §10 to §16): an audio-only SFU on
// Pion. A client connects to it over the WebSocket of its grant with a single-use ticket; the
// node makes every offer, on one PeerConnection per session, with a transceiver that receives
// the user's microphone and one slot per speaker that sends that speaker. It forwards the Opus
// of every speaker who makes sound and drops the rest (ADR-0101 §12), follows the room state of
// the voice module (§13), and keeps its lease, its reservations, and the assignments of its rooms
// in the registry of package sfu (§14). Nothing of it runs unless the server runs voice = sfu.
package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/voice"
)

const (
	// drainPoll is how often a draining node tries again to move its rooms and checks whether its
	// clients left.
	drainPoll = time.Second
	// leaseTimeout bounds one renewal of the leases.
	leaseTimeout = 5 * time.Second
)

// RoomSource reads the state of a room from the voice module: *voice.Service implements it.
type RoomSource interface {
	MediaRoom(ctx context.Context, roomID string) (voice.MediaRoom, error)
}

// Options configure a Node.
type Options struct {
	// ID names the node in the registry; unique per process.
	ID string
	// URL is the wss: URL that the clients connect to (voice_sfu_public_url).
	URL string
	// Capacity is how many sessions the node holds (voice_sfu_node_capacity).
	Capacity int
	// UDPPort and TCPPort are the ports of the media (voice_sfu_udp_port, voice_sfu_tcp_port),
	// and PublicIP the address that the node gives as its own (voice_sfu_public_ip).
	UDPPort, TCPPort int
	PublicIP         string
	// UDPConn and TCPListener, when both set, are the sockets of the media instead of the
	// ports; Loopback makes the node offer its loopback addresses. Tests set them.
	UDPConn     net.PacketConn
	TCPListener net.Listener
	Loopback    bool
	// OriginPatterns are the hosts of the web pages that may open the WebSocket besides the
	// node's own (web_origin).
	OriginPatterns []string
	Registry       *sfu.Registry
	Rooms          RoomSource
	// Streams carries the signals of the rooms; nil leaves the rooms to RefreshInterval.
	Streams  stream.RoomSubscriber
	Meter    kernel.Meter
	Logger   *slog.Logger
	Reporter kernel.ErrorReporter
}

// Node is a media node.
type Node struct {
	o     Options
	media *media
	stats *stats
	base  context.Context
	stop  context.CancelFunc

	draining atomic.Bool
	sessions atomic.Int64

	mu    sync.Mutex
	rooms map[string]*room
	held  []sfu.HeldRoom
}

// New returns the Node of o; Start starts it.
func New(o Options) *Node {
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	o.Logger = o.Logger.With(slog.String("component", "voice.sfu"))
	return &Node{o: o, rooms: map[string]*room{}}
}

// ID returns the ID of the node in the registry.
func (n *Node) ID() string { return n.o.ID }

// Start opens the media sockets, registers the node, and keeps its leases until Close.
func (n *Node) Start(ctx context.Context) error {
	m, err := newMedia(ctx, n.o, n.o.Logger)
	if err != nil {
		return err
	}
	n.media = m
	n.stats = newStats(n.o.Meter)
	n.base, n.stop = context.WithCancel(context.WithoutCancel(ctx))
	n.registerGauges()
	if err := n.renew(ctx); err != nil {
		n.stop()
		m.close()
		return fmt.Errorf("register the media node: %w", err)
	}
	n.run("voice.sfu.lease", n.keepLeases)
	return nil
}

// run runs fn in a goroutine whose panic is reported (kernel.Go).
func (n *Node) run(name string, fn func()) {
	kernel.Go(n.base, name, n.o.Reporter, func(context.Context) { fn() })
}

func (n *Node) registerGauges() {
	m := kernel.MeterOrNop(n.o.Meter)
	gauge := func(i kernel.Instrument, v func() int) {
		m.Gauge(i, func(_ context.Context, o kernel.GaugeObserver) error {
			o.Observe(float64(v()), kernel.Labels{})
			return nil
		})
	}
	gauge(Rooms, n.Rooms)
	gauge(Sessions, n.Sessions)
	gauge(CapacityConnected, n.Sessions)
	gauge(CapacityReserved, n.Reserved)
}

// Rooms returns how many rooms have connections on the node.
func (n *Node) Rooms() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.rooms)
}

// Sessions returns how many sessions are connected to the node.
func (n *Node) Sessions() int { return int(n.sessions.Load()) }

// Reserved returns the sum of the maximums of the rooms placed on the node, when it last renewed.
func (n *Node) Reserved() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	sum := 0
	for _, h := range n.held {
		sum += h.Max
	}
	return sum
}

// Totals returns the totals of the packet path since the node started.
func (n *Node) Totals() Totals { return n.stats.totals() }

// takeSession counts a new session, false when the node holds its capacity.
func (n *Node) takeSession() bool {
	for {
		c := n.sessions.Load()
		if n.o.Capacity > 0 && c >= int64(n.o.Capacity) {
			return false
		}
		if n.sessions.CompareAndSwap(c, c+1) {
			return true
		}
	}
}

func (n *Node) releaseSession() { n.sessions.Add(-1) }

// keepLeases renews the leases every sfu.LeaseRenewal, and checks once a minute that the rooms
// placed on the node without connections are still open.
func (n *Node) keepLeases() {
	t := time.NewTicker(sfu.LeaseRenewal)
	defer t.Stop()
	check := time.NewTicker(RefreshInterval)
	defer check.Stop()
	for {
		select {
		case <-n.base.Done():
			return
		case <-t.C:
			if err := n.renew(n.base); err != nil && n.base.Err() == nil {
				n.o.Logger.Warn("a voice chat media node could not renew its lease", "error", err)
			}
		case <-check.C:
			n.releaseEnded(n.base)
		}
	}
}

// renew renews the node's lease, reservations and assignments with the sessions of its rooms.
func (n *Node) renew(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, leaseTimeout)
	defer cancel()
	n.mu.Lock()
	rooms := make([]*room, 0, len(n.rooms))
	for _, r := range n.rooms {
		rooms = append(rooms, r)
	}
	n.mu.Unlock()
	sessions := make(map[string][]sfu.ConnectedSession, len(rooms))
	for _, r := range rooms {
		for _, c := range r.sessions() {
			sessions[r.id] = append(sessions[r.id], sfu.ConnectedSession{ID: c.session, AccountID: c.account})
		}
	}
	held, err := n.o.Registry.Renew(ctx, sfu.NodeLease{
		ID: n.o.ID, URL: n.o.URL, Capacity: n.o.Capacity, Connected: n.Sessions(), Draining: n.draining.Load(),
	}, sessions)
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.held = held
	n.mu.Unlock()
	return nil
}

// releaseEnded releases the rooms placed on the node that have no connection and ended, which a
// Close of the transport that never ran would leave reserved.
func (n *Node) releaseEnded(ctx context.Context) {
	n.mu.Lock()
	var idle []string
	for _, h := range n.held {
		if n.rooms[h.ID] == nil {
			idle = append(idle, h.ID)
		}
	}
	n.mu.Unlock()
	for _, id := range idle {
		st, err := n.o.Rooms.MediaRoom(ctx, id)
		if err != nil || st.Open {
			continue
		}
		if err := n.o.Registry.Release(ctx, id); err != nil {
			n.o.Logger.WarnContext(ctx, "a voice chat media node could not release a room that ended", "error", err)
		}
	}
}

// acquire returns the room of id on the node, made when it has none, counted as joining until
// done is called.
func (n *Node) acquire(id string) (r *room, done func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r = n.rooms[id]
	if r == nil {
		r = newRoom(n, id)
		n.rooms[id] = r
	}
	r.mu.Lock()
	r.joining++
	r.mu.Unlock()
	return r, func() {
		r.mu.Lock()
		r.joining--
		r.mu.Unlock()
		n.dropIfEmpty(r)
	}
}

// dropIfEmpty takes a room without connections and joins off the node.
func (n *Node) dropIfEmpty(r *room) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r.mu.Lock()
	empty := len(r.peers) == 0 && r.joining == 0 && !r.closed
	if empty {
		r.closed = true
	}
	r.mu.Unlock()
	if empty {
		r.stop()
		if n.rooms[r.id] == r {
			delete(n.rooms, r.id)
		}
	}
}

// ServeHTTP serves the WebSocket of the grants (config.VoiceSFUPath): it consumes the ticket of
// the query parameter ticket, connects the session, and reads the client's messages until the
// WebSocket closes. A bad ticket, a room or session no longer on the node, a message the node
// does not understand, or another version gets an error message and the WebSocket closes.
func (n *Node) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The WebSocket outlives the deadlines of the HTTP server's request.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: n.o.OriginPatterns})
	if err != nil {
		return // Accept answered
	}
	c.SetReadLimit(maxMessageBytes)
	ws := newWSConn(n.base, c, func(name string, fn func(context.Context)) {
		kernel.Go(n.base, name, n.o.Reporter, fn)
	})
	t, err := n.o.Registry.Consume(r.Context(), r.URL.Query().Get("ticket"), n.o.ID)
	if err != nil {
		reason := RefusalInvalid
		if errors.Is(err, sfu.ErrOtherNode) {
			reason = RefusalOtherNode
		}
		n.stats.refusals.Add(n.base, 1, kernel.NewLabels("my.voice.sfu.reason", reason))
		ws.send(errorMessage(CodeInvalidTicket, "the ticket expired, was used, or is for another node; ask for a new grant"))
		ws.close(websocket.StatusPolicyViolation, "invalid ticket")
		n.discard(c)
		return
	}
	rm, done := n.acquire(t.Room)
	code := rm.join(n.base, t.Session, t.Account, ws)
	done()
	if code != "" {
		ws.send(errorMessage(code, "the session or the room is no longer on the node"))
		ws.close(websocket.StatusNormalClosure, "")
		n.discard(c)
		return
	}
	rm.mu.Lock()
	p := rm.peers[t.Session]
	rm.mu.Unlock()
	if p == nil {
		ws.close(websocket.StatusNormalClosure, "")
		n.discard(c)
		return
	}
	n.serve(c, ws, p)
}

// discard reads a WebSocket that is closing, so that its close handshake completes.
func (n *Node) discard(c *websocket.Conn) {
	for {
		if _, _, err := c.Read(n.base); err != nil {
			return
		}
	}
}

// serve reads the messages of a session's client until its WebSocket closes.
func (n *Node) serve(c *websocket.Conn, ws *wsConn, p *peer) {
	defer p.detach(ws)
	for {
		typ, data, err := c.Read(n.base)
		if err != nil {
			return
		}
		m, code := parseClientMessage(typ, data)
		if code != "" {
			ws.send(errorMessage(code, "the node does not understand the message"))
			ws.close(websocket.StatusPolicyViolation, code)
			n.discard(c)
			return
		}
		switch m.Type {
		case TypeAnswer:
			p.answer(ws, m.SDP)
		case TypeCandidate:
			p.candidate(ws, m.Candidate)
		case TypeRestart:
			p.restartICE()
		}
	}
}

// parseClientMessage decodes a message of a client, or returns the code of the error to answer.
func parseClientMessage(typ websocket.MessageType, data []byte) (ClientMessage, string) {
	var m ClientMessage
	if typ != websocket.MessageText || json.Unmarshal(data, &m) != nil {
		return m, CodeInvalidMessage
	}
	if m.V != Version {
		return m, CodeUnsupportedVersion
	}
	switch m.Type {
	case TypeAnswer:
		if m.SDP == "" {
			return m, CodeInvalidMessage
		}
	case TypeCandidate, TypeRestart:
	default:
		return m, CodeInvalidMessage
	}
	return m, ""
}

// Drain stops the node taking rooms and moves its rooms to other nodes (ADR-0101 §14): it marks
// itself draining, places each of its rooms on another node, sends migrate to the clients of
// each room that moved, and waits for them to leave until ctx ends. A room that no other node
// has room for stays and is served until it ends, or ctx does; Drain tries it again meanwhile.
func (n *Node) Drain(ctx context.Context) {
	if n.base == nil {
		return
	}
	n.draining.Store(true)
	for {
		if err := n.renew(ctx); err != nil && ctx.Err() == nil {
			n.o.Logger.WarnContext(ctx, "a draining voice chat media node could not renew its lease", "error", err)
		}
		n.moveRooms(ctx)
		if n.Sessions() == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(drainPoll):
		}
	}
}

// moveRooms places the rooms of the node on other nodes, and sends migrate to the clients of
// each room that is not on this node any more.
func (n *Node) moveRooms(ctx context.Context) {
	n.mu.Lock()
	held := append([]sfu.HeldRoom(nil), n.held...)
	rooms := make([]*room, 0, len(n.rooms))
	for _, r := range n.rooms {
		rooms = append(rooms, r)
	}
	n.mu.Unlock()
	for _, h := range held {
		if _, err := n.o.Registry.Place(ctx, h.ID, h.Max, n.o.ID); err != nil && !errors.Is(err, sfu.ErrNoCapacity) {
			n.o.Logger.WarnContext(ctx, "a draining voice chat media node could not move a room", "error", err)
		}
	}
	for _, r := range rooms {
		a, ok, err := n.o.Registry.Assignment(ctx, r.id)
		if err != nil || (ok && a.Node == n.o.ID) {
			continue
		}
		for _, p := range r.peerList() {
			if p.migrate() {
				n.stats.migrations.Add(n.base, 1, kernel.Labels{})
			}
		}
	}
}

// Close closes every connection and the media sockets, and takes the node out of the registry.
func (n *Node) Close(ctx context.Context) error {
	if n.base == nil {
		return nil
	}
	n.mu.Lock()
	rooms := make([]*room, 0, len(n.rooms))
	for _, r := range n.rooms {
		rooms = append(rooms, r)
	}
	n.mu.Unlock()
	for _, r := range rooms {
		for _, p := range r.peerList() {
			p.close("", "")
		}
	}
	n.stop()
	n.media.close()
	return n.o.Registry.Deregister(ctx, n.o.ID)
}
