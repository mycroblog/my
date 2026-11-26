// SPDX-License-Identifier: AGPL-3.0-only

// Package nodetest is a simulated browser of voice = sfu for the tests of the media node and of
// the App that runs it: a Pion PeerConnection that connects through the WebSocket of a grant,
// answers the node's offers, and sends its microphone. It lives beside the node because only the
// node's directory may import Pion and the WebSocket library; its signatures name neither.
package nodetest

import (
	"context"
	"encoding/json"
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
	ws      *websocket.Conn
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
}

// Connect connects session to the node at url with ticket.
func Connect(t testing.TB, url, ticket, session string) *Client {
	t.Helper()
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
	ws, res, err := websocket.Dial(t.Context(), url+"?ticket="+ticket, nil)
	if err != nil {
		t.Fatalf("Dial %s: %v", url, err)
	}
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	c.ws = ws
	kernel.Go(t.Context(), "voice.sfu.nodetest.signal", nil, func(context.Context) { c.signal() })
	return c
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

// signal answers the node's offers until the WebSocket closes.
func (c *Client) signal() {
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
			select {
			case c.migrated <- struct{}{}:
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
			select {
			case c.answered <- struct{}{}:
			default:
			}
		}
	}
}

// Send writes a message of the client to the node.
func (c *Client) Send(m node.ClientMessage) {
	c.t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.ws.Write(c.t.Context(), websocket.MessageText, b); err != nil {
		c.t.Fatalf("write %s: %v", m.Type, err)
	}
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
	_ = c.ws.Close(websocket.StatusNormalClosure, "")
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
