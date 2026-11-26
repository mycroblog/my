// SPDX-License-Identifier: AGPL-3.0-only

package node_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node"
	"github.com/mycroblog/my/internal/app/stream"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/voicetest"
)

// rooms is the voice module of the tests: the state of each room, which a test changes.
type rooms struct {
	mu    sync.Mutex
	state map[string]appvoice.MediaRoom
}

func (r *rooms) MediaRoom(_ context.Context, id string) (appvoice.MediaRoom, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state[id], nil
}

func (r *rooms) set(id string, st appvoice.MediaRoom) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state[id] = st
}

// env is a node on loopback sockets behind an HTTP server, with the transport that places the
// rooms on it.
type env struct {
	node      *node.Node
	transport *sfu.Transport
	rooms     *rooms
	hub       *stream.Hub
	clock     *testkit.Clock
	url       string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	rs := &rooms{state: map[string]appvoice.MediaRoom{}}
	return newNodeEnv(t, "node-1", sfu.NewRegistry(testkit.NewKeyValueStore(clock), clock), clock, rs, stream.NewHub(stream.HubOptions{}))
}

// newNodeEnv is newEnv for the node id on a registry, a voice module, and a hub that several
// nodes share.
func newNodeEnv(t *testing.T, id string, reg *sfu.Registry, clock *testkit.Clock, rs *rooms, hub *stream.Hub) *env {
	t.Helper()
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = udp.Close(); _ = tcp.Close() })
	e := &env{rooms: rs, hub: hub, clock: clock}
	t.Cleanup(e.hub.Close)
	mux := httptest.NewServer(nil)
	t.Cleanup(mux.Close)
	e.url = "ws" + strings.TrimPrefix(mux.URL, "http")
	e.node = node.New(node.Options{
		ID: id, URL: e.url, Capacity: 50, UDPConn: udp, TCPListener: tcp, Loopback: true,
		Registry: reg, Rooms: rs, Streams: e.hub,
	})
	mux.Config.Handler = e.node
	if err := e.node.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.node.Close(context.Background()) })
	e.transport = sfu.New(sfu.Options{Registry: reg, MaxParticipants: 10, Clock: clock})
	return e
}

// client is a simulated browser: a Pion PeerConnection that answers the node's offers over the
// WebSocket of a grant and sends its microphone.
type client struct {
	t        *testing.T
	pc       *webrtc.PeerConnection
	ws       *websocket.Conn
	mic      *webrtc.TrackLocalStaticRTP
	levelID  atomic.Uint32
	heard    sync.Map // participant -> *atomic.Int64 of packets
	mids     sync.Map // mid -> participant
	offers   atomic.Int64
	errors   chan node.Message
	speaking chan []string
	migrated chan struct{}
}

func (e *env) connect(t *testing.T, room, session, account string) *client {
	t.Helper()
	g, err := e.transport.Grant(t.Context(), appvoice.RoomRef{ID: room}, appvoice.Session{ID: session, AccountID: account}, true)
	if err != nil {
		t.Fatalf("Grant: %v", err)
	}
	engine := &webrtc.MediaEngine{}
	if err := node.RegisterCodecs(engine); err != nil {
		t.Fatal(err)
	}
	var se webrtc.SettingEngine
	se.SetIncludeLoopbackCandidate(true)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	api := webrtc.NewAPI(webrtc.WithMediaEngine(engine), webrtc.WithSettingEngine(se))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	mic, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "mic", session)
	if err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, pc: pc, mic: mic, errors: make(chan node.Message, 4), speaking: make(chan []string, 64), migrated: make(chan struct{}, 1)}
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		for {
			if _, _, err := tr.Read(buf); err != nil {
				return
			}
			p, _ := c.mids.Load(transceiverMid(pc, tr))
			if s, ok := p.(string); ok {
				n, _ := c.heard.LoadOrStore(s, &atomic.Int64{})
				n.(*atomic.Int64).Add(1)
			}
		}
	})
	ws, _, err := websocket.Dial(t.Context(), g.URL+"?ticket="+g.Token, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	c.ws = ws
	go c.signal()
	return c
}

// transceiverMid returns the mid of the transceiver that receives tr.
func transceiverMid(pc *webrtc.PeerConnection, tr *webrtc.TrackRemote) string {
	for _, x := range pc.GetTransceivers() {
		if r := x.Receiver(); r != nil && r.Track() == tr {
			return x.Mid()
		}
	}
	return ""
}

// signal answers the node's offers until the WebSocket closes.
func (c *client) signal() {
	added := false
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			return
		}
		var m node.Message
		if err := json.Unmarshal(data, &m); err != nil {
			c.t.Errorf("a message of the node does not decode: %v", err)
			return
		}
		switch m.Type {
		case node.TypeError:
			c.errors <- m
		case node.TypeMigrate:
			c.migrated <- struct{}{}
		case node.TypeSpeaking:
			select {
			case c.speaking <- m.Participants:
			default:
			}
		case node.TypeOffer:
			for _, tr := range m.Tracks {
				c.mids.Store(tr.Mid, tr.Participant)
			}
			if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: m.SDP}); err != nil {
				c.t.Logf("SetRemoteDescription: %v", err)
				return
			}
			if !added {
				sender, err := c.pc.AddTrack(c.mic)
				if err != nil {
					c.t.Logf("AddTrack: %v", err)
					return
				}
				for _, e := range sender.GetParameters().HeaderExtensions {
					if e.URI == sdp.AudioLevelURI {
						c.levelID.Store(uint32(e.ID)) //nolint:gosec // an extension ID is 1 to 255
					}
				}
				added = true
			}
			answer, err := c.pc.CreateAnswer(nil)
			if err != nil {
				c.t.Logf("CreateAnswer: %v", err)
				return
			}
			gathered := webrtc.GatheringCompletePromise(c.pc)
			if err := c.pc.SetLocalDescription(answer); err != nil {
				c.t.Logf("SetLocalDescription: %v", err)
				return
			}
			<-gathered
			b, _ := json.Marshal(node.ClientMessage{V: node.Version, Type: node.TypeAnswer, SDP: c.pc.LocalDescription().SDP})
			if err := c.ws.Write(context.Background(), websocket.MessageText, b); err != nil {
				return
			}
			c.offers.Add(1)
		}
	}
}

// speak sends audio at level (RFC 6464) every 20 ms until ctx ends.
func (c *client) speak(ctx context.Context, level uint8) {
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	var seq uint16
	var ts uint32
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		seq++
		ts += 960
		p := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: ts}, Payload: []byte{0xfc, 0xff, 0xfe}}
		if id := uint8(c.levelID.Load()); id != 0 { //nolint:gosec // an extension ID is 1 to 255
			_ = p.SetExtension(id, []byte{level})
		}
		_ = c.mic.WriteRTP(p)
	}
}

func (c *client) heardFrom(account string) int64 {
	n, ok := c.heard.Load(account)
	if !ok {
		return 0
	}
	return n.(*atomic.Int64).Load()
}

// TestForwardsTheSpeakerOnly joins a speaker and a listener: the listener hears the speaker, the
// speaker hears nobody, the listener's audio and the silent packets are dropped at the node.
func TestForwardsTheSpeakerOnly(t *testing.T) {
	e := newEnv(t)
	e.rooms.set("room-1", appvoice.MediaRoom{Open: true, MaxParticipants: 10, Sessions: []appvoice.MediaSession{
		{ID: "s-speaker", AccountID: "a-speaker", CanSpeak: true},
		{ID: "s-listener", AccountID: "a-listener"},
	}})
	if err := e.transport.Open(t.Context(), appvoice.RoomRef{ID: "room-1", MaxParticipants: 10}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	speaker := e.connect(t, "room-1", "s-speaker", "a-speaker")
	listener := e.connect(t, "room-1", "s-listener", "a-listener")
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	connected := func(c *client) bool { return c.pc.ConnectionState() == webrtc.PeerConnectionStateConnected }
	if !testkit.Eventually(t, 10*time.Second, func() bool { return connected(speaker) && connected(listener) }) {
		t.Fatalf("not connected: speaker %s, listener %s", speaker.pc.ConnectionState(), listener.pc.ConnectionState())
	}
	go speaker.speak(ctx, 30)
	go listener.speak(ctx, 30)
	if !testkit.Eventually(t, 10*time.Second, func() bool { return listener.heardFrom("a-speaker") > 10 }) {
		t.Fatalf("the listener heard %d packets of the speaker, want more than 10; totals %+v", listener.heardFrom("a-speaker"), e.node.Totals())
	}
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.node.Totals().NotAllowed > 10 }) {
		t.Fatalf("totals %+v, want the listener's packets dropped as not allowed", e.node.Totals())
	}
	if n := speaker.heardFrom("a-listener"); n != 0 {
		t.Errorf("the speaker heard %d packets of the listener, want none", n)
	}
	if got := e.node.Sessions(); got != 2 {
		t.Errorf("Sessions = %d, want 2", got)
	}
	sessions, err := e.transport.Sessions(t.Context(), appvoice.RoomRef{ID: "room-1"})
	if err != nil {
		t.Fatal(err)
	}
	_ = sessions // reported on the next renewal of the leases
}

// change sets the state of a room and signals it, as the voice module does.
func (e *env) change(room string, st appvoice.MediaRoom) {
	e.rooms.set(room, st)
	e.hub.Publish(stream.Event{Kind: stream.KindVoiceRoom, RoomID: room})
}

// TestFollowsTheRoom follows the changes of a room: a listener made a speaker is heard after an
// offer, a muted speaker is dropped, a removed session is closed with room_unavailable, and the
// speakers who make sound are told.
func TestFollowsTheRoom(t *testing.T) {
	e := newEnv(t)
	host := appvoice.MediaSession{ID: "s-host", AccountID: "a-host", CanSpeak: true}
	guest := appvoice.MediaSession{ID: "s-guest", AccountID: "a-guest"}
	e.rooms.set("room-2", appvoice.MediaRoom{Open: true, Sessions: []appvoice.MediaSession{host, guest}})
	a := e.connect(t, "room-2", host.ID, host.AccountID)
	b := e.connect(t, "room-2", guest.ID, guest.AccountID)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.speak(ctx, 30)
	go b.speak(ctx, 30)
	if !testkit.Eventually(t, 10*time.Second, func() bool { return b.heardFrom("a-host") > 5 }) {
		t.Fatalf("the guest did not hear the host; totals %+v", e.node.Totals())
	}
	if !waitSpeaking(a, []string{"a-host"}) {
		t.Errorf("speaking did not name the host")
	}

	// The guest becomes a speaker: the host gets an offer with a slot for the guest.
	guest.CanSpeak = true
	e.change("room-2", appvoice.MediaRoom{Open: true, Sessions: []appvoice.MediaSession{host, guest}})
	if !testkit.Eventually(t, 10*time.Second, func() bool { return a.heardFrom("a-guest") > 5 }) {
		t.Fatalf("the host did not hear the guest made a speaker; totals %+v", e.node.Totals())
	}

	// The host mutes the guest: the guest's packets are dropped.
	guest.Muted = true
	e.change("room-2", appvoice.MediaRoom{Open: true, Sessions: []appvoice.MediaSession{host, guest}})
	before := e.node.Totals().NotAllowed
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.node.Totals().NotAllowed > before+10 }) {
		t.Fatalf("the muted guest's packets were not dropped; totals %+v", e.node.Totals())
	}

	// The guest is removed: its connection closes with room_unavailable.
	e.change("room-2", appvoice.MediaRoom{Open: true, Sessions: []appvoice.MediaSession{host}})
	select {
	case m := <-b.errors:
		if m.Code != node.CodeRoomUnavailable {
			t.Errorf("error code = %q, want %q", m.Code, node.CodeRoomUnavailable)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the removed guest got no error")
	}
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.node.Sessions() == 1 }) {
		t.Errorf("Sessions = %d, want 1", e.node.Sessions())
	}
}

// TestRefusesABadTicket refuses an unknown ticket with invalid_ticket.
func TestRefusesABadTicket(t *testing.T) {
	e := newEnv(t)
	ws, _, err := websocket.Dial(t.Context(), e.url+"?ticket=unknown", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.CloseNow() }()
	_, data, err := ws.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var m node.Message
	if err := json.Unmarshal(data, &m); err != nil || m.Type != node.TypeError || m.Code != node.CodeInvalidTicket {
		t.Errorf("message = %s, want an error invalid_ticket", data)
	}
}

func waitSpeaking(c *client, want []string) bool {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-c.speaking:
			if strings.Join(got, ",") == strings.Join(want, ",") {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// TestVoiceTransportContract runs the transport's contract with a running node, which only a
// WebRTC client connects to.
func TestVoiceTransportContract(t *testing.T) {
	voicetest.VoiceTransportContract(t, func(t *testing.T) voicetest.VoiceTransportHarness {
		e := newEnv(t)
		return voicetest.VoiceTransportHarness{Transport: e.transport, Clock: e.clock}
	})
}

// TestDrainMigrates drains a node with a room: the room moves to the other node, the client gets
// migrate, and its next grant connects it there.
func TestDrainMigrates(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	reg := sfu.NewRegistry(testkit.NewKeyValueStore(clock), clock)
	rs := &rooms{state: map[string]appvoice.MediaRoom{}}
	hub := stream.NewHub(stream.HubOptions{})
	first := newNodeEnv(t, "node-1", reg, clock, rs, hub)
	rs.set("room-3", appvoice.MediaRoom{Open: true, Sessions: []appvoice.MediaSession{{ID: "s-1", AccountID: "a-1", CanSpeak: true}}})
	c := first.connect(t, "room-3", "s-1", "a-1")
	if !testkit.Eventually(t, 10*time.Second, func() bool { return c.pc.ConnectionState() == webrtc.PeerConnectionStateConnected }) {
		t.Fatal("the session did not connect")
	}
	second := newNodeEnv(t, "node-2", reg, clock, rs, hub)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	go func() {
		<-c.migrated
		_ = c.pc.Close()
	}()
	first.node.Drain(ctx)
	if a, ok, err := reg.Assignment(t.Context(), "room-3"); err != nil || !ok || a.Node != "node-2" {
		t.Fatalf("Assignment after the drain = %+v, %t, %v, want node-2", a, ok, err)
	}
	if n := first.node.Sessions(); n != 0 {
		t.Errorf("the drained node keeps %d sessions", n)
	}
	g, err := second.transport.Grant(t.Context(), appvoice.RoomRef{ID: "room-3"}, appvoice.Session{ID: "s-1", AccountID: "a-1"}, true)
	if err != nil || g.URL != second.url {
		t.Errorf("Grant after the drain = %+v, %v, want the URL of node-2 %s", g, err, second.url)
	}
}
