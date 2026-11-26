// SPDX-License-Identifier: AGPL-3.0-only

// Package nodetest is a simulated browser of voice = sfu for the tests of the media node and of
// the App that runs it: a Pion PeerConnection that connects through the WebSocket of a grant,
// answers the node's offers, and sends its microphone. It lives beside the node because only the
// node's directory may import Pion and the WebSocket library; its signatures name neither.
package nodetest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"

	"github.com/mycroblog/my/internal/adapter/voice/sfu/node"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// Client is a simulated browser of voice = sfu.
type Client struct {
	t       testing.TB
	pc      *webrtc.PeerConnection
	mic     *webrtc.TrackLocalStaticRTP
	levelID atomic.Uint32
	// heard counts the packets received by participant (an account ID), mapped from the mids of
	// the offers' tracks.
	heard    sync.Map
	mids     sync.Map
	offers   atomic.Int64
	answered chan struct{}
	errors   chan node.Message
	migrated chan struct{}

	// ws is the WebSocket of the signaling, nil once dropped.
	wsMu sync.Mutex
	ws   *websocket.Conn
	// negotiation orders the answers of the signaling loops, one per WebSocket, and holds added:
	// whether the microphone was added.
	negotiation sync.Mutex
	added       bool
}

// Connect connects session to the node at url with ticket, and fails the test when the
// WebSocket does not open.
func Connect(t testing.TB, url, ticket, session string) *Client {
	t.Helper()
	c, err := Dial(t, url, ticket, session)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Dial is Connect returning the error, for a test that connects from several goroutines.
func Dial(t testing.TB, url, ticket, session string) (*Client, error) {
	engine := &webrtc.MediaEngine{}
	if err := node.RegisterCodecs(engine); err != nil {
		return nil, err
	}
	var se webrtc.SettingEngine
	se.SetIncludeLoopbackCandidate(true)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	api := webrtc.NewAPI(webrtc.WithMediaEngine(engine), webrtc.WithSettingEngine(se))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = pc.Close() })
	mic, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "mic", session)
	if err != nil {
		return nil, err
	}
	c := &Client{
		t: t, pc: pc, mic: mic, answered: make(chan struct{}, 16),
		errors: make(chan node.Message, 4), migrated: make(chan struct{}, 1),
	}
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		for {
			if _, _, err := tr.Read(buf); err != nil {
				return
			}
			p, _ := c.mids.Load(receiverMid(pc, tr))
			if s, ok := p.(string); ok {
				n, _ := c.heard.LoadOrStore(s, &atomic.Int64{})
				n.(*atomic.Int64).Add(1)
			}
		}
	})
	if err := c.attach(url, ticket); err != nil {
		return nil, err
	}
	return c, nil
}

// attach opens the WebSocket of url with ticket and answers the node's offers on it.
func (c *Client) attach(url, ticket string) error {
	ws, res, err := websocket.Dial(c.t.Context(), url+"?ticket="+ticket, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", url, err)
	}
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	c.t.Cleanup(func() { _ = ws.CloseNow() })
	c.wsMu.Lock()
	c.ws = ws
	c.wsMu.Unlock()
	kernel.Go(c.t.Context(), "voice.sfu.nodetest.signal", nil, func(context.Context) { c.signal(ws) })
	return nil
}

// receiverMid returns the mid of the transceiver that receives tr.
func receiverMid(pc *webrtc.PeerConnection, tr *webrtc.TrackRemote) string {
	for _, x := range pc.GetTransceivers() {
		if r := x.Receiver(); r != nil && r.Track() == tr {
			return x.Mid()
		}
	}
	return ""
}

// signal answers the node's offers on ws until it closes.
func (c *Client) signal(ws *websocket.Conn) {
	for {
		_, data, err := ws.Read(context.Background())
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
			select {
			case c.errors <- m:
			default:
			}
		case node.TypeMigrate:
			select {
			case c.migrated <- struct{}{}:
			default:
			}
		case node.TypeOffer:
			if !c.answer(ws, m) {
				return
			}
		}
	}
}

// answer answers the offer m on ws, and reports whether the signaling goes on.
func (c *Client) answer(ws *websocket.Conn, m node.Message) bool {
	c.negotiation.Lock()
	defer c.negotiation.Unlock()
	for _, tr := range m.Tracks {
		c.mids.Store(tr.Mid, tr.Participant)
	}
	if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: m.SDP}); err != nil {
		c.t.Logf("SetRemoteDescription: %v", err)
		return false
	}
	if !c.added {
		sender, err := c.pc.AddTrack(c.mic)
		if err != nil {
			c.t.Logf("AddTrack: %v", err)
			return false
		}
		for _, e := range sender.GetParameters().HeaderExtensions {
			if e.URI == sdp.AudioLevelURI {
				c.levelID.Store(uint32(e.ID)) //nolint:gosec // an extension ID is 1 to 255
			}
		}
		c.added = true
	}
	answer, err := c.pc.CreateAnswer(nil)
	if err != nil {
		c.t.Logf("CreateAnswer: %v", err)
		return false
	}
	gathered := webrtc.GatheringCompletePromise(c.pc)
	if err := c.pc.SetLocalDescription(answer); err != nil {
		c.t.Logf("SetLocalDescription: %v", err)
		return false
	}
	<-gathered
	b, _ := json.Marshal(node.ClientMessage{V: node.Version, Type: node.TypeAnswer, SDP: c.pc.LocalDescription().SDP})
	if err := ws.Write(context.Background(), websocket.MessageText, b); err != nil {
		return false
	}
	c.offers.Add(1)
	select {
	case c.answered <- struct{}{}:
	default:
	}
	return true
}

// Send writes a message of the client to the node.
func (c *Client) Send(m node.ClientMessage) {
	c.t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatal(err)
	}
	c.wsMu.Lock()
	ws := c.ws
	c.wsMu.Unlock()
	if ws == nil {
		c.t.Fatalf("write %s: the WebSocket was dropped", m.Type)
	}
	if err := ws.Write(c.t.Context(), websocket.MessageText, b); err != nil {
		c.t.Fatalf("write %s: %v", m.Type, err)
	}
}

// DropSignal drops the WebSocket without a close handshake, as a network that went away does,
// and keeps the PeerConnection.
func (c *Client) DropSignal() {
	c.wsMu.Lock()
	ws := c.ws
	c.ws = nil
	c.wsMu.Unlock()
	if ws != nil {
		_ = ws.CloseNow()
	}
}

// Reattach drops the WebSocket, if the client still has one, and opens a new one to the node at
// url with ticket for the same PeerConnection, as a client does with the ticket of a new grant
// after its WebSocket dropped (ADR-0101 §11).
func (c *Client) Reattach(url, ticket string) error {
	c.DropSignal()
	return c.attach(url, ticket)
}

// Speak sends audio at level (RFC 6464) every 20 ms, as a microphone does, until ctx ends.
func (c *Client) Speak(ctx context.Context, level uint8) {
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

// HeardFrom returns the packets received of the participant account.
func (c *Client) HeardFrom(account string) int64 {
	n, ok := c.heard.Load(account)
	if !ok {
		return 0
	}
	return n.(*atomic.Int64).Load()
}

// Offers returns the offers of the node that the client has answered.
func (c *Client) Offers() int64 { return c.offers.Load() }

// Migrated is signalled when the node sends migrate.
func (c *Client) Migrated() <-chan struct{} { return c.migrated }

// Connected reports whether the client's media is connected.
func (c *Client) Connected() bool {
	return c.pc.ConnectionState() == webrtc.PeerConnectionStateConnected
}

// Close closes the WebSocket and the PeerConnection, as a client that leaves does.
func (c *Client) Close() {
	c.wsMu.Lock()
	ws := c.ws
	c.ws = nil
	c.wsMu.Unlock()
	if ws != nil {
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}
	_ = c.pc.Close()
}

// WaitConnected fails the test unless every client connects its media.
func WaitConnected(t *testing.T, cs ...*Client) {
	t.Helper()
	if !testkit.Eventually(t, 15*time.Second, func() bool {
		for _, c := range cs {
			if !c.Connected() {
				return false
			}
		}
		return true
	}) {
		for i, c := range cs {
			t.Logf("client %d: %s", i, c.pc.ConnectionState())
		}
		t.Fatal("the clients did not connect")
	}
}

// WaitError returns the next error message of c.
func (c *Client) WaitError(within time.Duration) (node.Message, bool) {
	select {
	case m := <-c.errors:
		return m, true
	case <-time.After(within):
		return node.Message{}, false
	}
}

// DialRaw opens the WebSocket of the node at url with ticket, with no media. It returns a function
// that reads until the node closes the connection or sends a message, and one that closes it.
func DialRaw(ctx context.Context, url, ticket string) (read func() error, closeFn func(), err error) {
	ws, res, err := websocket.Dial(ctx, url+"?ticket="+ticket, nil)
	if err != nil {
		return nil, nil, err
	}
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	return func() error { _, _, err := ws.Read(ctx); return err }, func() { _ = ws.CloseNow() }, nil
}
