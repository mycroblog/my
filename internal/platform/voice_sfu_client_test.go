// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"

	"github.com/mycroblog/my/internal/adapter/voice/sfu/node"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

// sfuProcess is an App of voice = sfu served over HTTP on loopback, with its media node, if its
// role runs one, started on loopback ports, and the measurements of its metrics.
type sfuProcess struct {
	app   *platform.App
	srv   *httptest.Server
	meter *testkit.Meter
	// url is the node's WebSocket (voice_sfu_public_url), which its grants name.
	url string
}

// newSFUProcess builds the App of role on env with voice = sfu, serves its handler, and starts its
// media node when it has one. env has the database, the stream, and the web origin.
func newSFUProcess(t *testing.T, env map[string]string, role string) *sfuProcess {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "ws://" + l.Addr().String() + config.VoiceSFUPath
	full := map[string]string{
		"MY_ROLE": role, "MY_VOICE": "sfu",
		"MY_VOICE_SFU_UDP_PORT": strconv.Itoa(freePort(t, "udp")), "MY_VOICE_SFU_TCP_PORT": strconv.Itoa(freePort(t, "tcp")),
		"MY_VOICE_SFU_PUBLIC_IP": "127.0.0.1", "MY_VOICE_SFU_PUBLIC_URL": url,
	}
	for k, v := range env {
		full[k] = v
	}
	meter := testkit.NewMeter()
	app, err := platform.NewApp(t.Context(), loadConfig(t, full), platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Meter: meter})
	if err != nil {
		_ = l.Close()
		t.Fatalf("NewApp(%s): %v", role, err)
	}
	t.Cleanup(func() { _ = app.Close() })
	srv := httptest.NewUnstartedServer(app.Handler())
	_ = srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	if app.VoiceNode != nil {
		if err := app.VoiceNode.Start(t.Context()); err != nil {
			t.Fatalf("Start the media node of %s: %v", role, err)
		}
		t.Cleanup(func() { _ = app.VoiceNode.Close(context.Background()) })
	}
	return &sfuProcess{app: app, srv: srv, meter: meter, url: url}
}

// packets returns the RTP packets that the process's node counted with outcome.
func (p *sfuProcess) packets(outcome string) float64 {
	return p.observed(node.Packets.Name, map[string]string{"my.outcome": outcome})
}

// observed collects the gauges and the observable counters of the process's meter and returns
// the last value of the metric name with the labels of match, -1 when it has none.
func (p *sfuProcess) observed(name string, match map[string]string) float64 {
	if err := p.meter.Collect(context.Background()); err != nil {
		return -1
	}
	ms := p.meter.Measurements(name, match)
	if len(ms) == 0 {
		return -1
	}
	return ms[len(ms)-1].Value
}

// sfuClient is a simulated browser of voice = sfu: a Pion PeerConnection that connects through the
// WebSocket of a grant, answers the node's offers, and sends its microphone.
type sfuClient struct {
	t       *testing.T
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

// connectSFU connects session to the node of the grant g.
func connectSFU(t *testing.T, g clientapi.VoiceGrant, session string) *sfuClient {
	t.Helper()
	if g.Kind != clientapi.VoiceGrantKindSfu || g.Url == nil || g.Token == nil {
		t.Fatalf("grant = %+v, want an sfu grant with a URL and a ticket", g)
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
	c := &sfuClient{
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
	ws, res, err := websocket.Dial(t.Context(), *g.Url+"?ticket="+*g.Token, nil)
	if err != nil {
		t.Fatalf("Dial %s: %v", *g.Url, err)
	}
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	c.ws = ws
	go c.signal()
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
func (c *sfuClient) signal() {
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

// send writes a message of the client to the node.
func (c *sfuClient) send(m node.ClientMessage) {
	c.t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.ws.Write(c.t.Context(), websocket.MessageText, b); err != nil {
		c.t.Fatalf("write %s: %v", m.Type, err)
	}
}

// speak sends audio at level (RFC 6464) every 20 ms, as a microphone does, until ctx ends.
func (c *sfuClient) speak(ctx context.Context, level uint8) {
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

// heardFrom returns the packets received of the participant account.
func (c *sfuClient) heardFrom(account string) int64 {
	n, ok := c.heard.Load(account)
	if !ok {
		return 0
	}
	return n.(*atomic.Int64).Load()
}

func (c *sfuClient) connected() bool {
	return c.pc.ConnectionState() == webrtc.PeerConnectionStateConnected
}

// waitConnected fails the test unless every client connects its media.
func waitConnected(t *testing.T, cs ...*sfuClient) {
	t.Helper()
	if !testkit.Eventually(t, 15*time.Second, func() bool {
		for _, c := range cs {
			if !c.connected() {
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

// waitError returns the next error message of c.
func (c *sfuClient) waitError(within time.Duration) (node.Message, bool) {
	select {
	case m := <-c.errors:
		return m, true
	case <-time.After(within):
		return node.Message{}, false
	}
}

// refreshGrant asks the App of e for a new grant of session (POST /voice/rooms/{id}/grant).
func refreshGrant(e *accountsEnv, token, room, session string) clientapi.VoiceGrant {
	e.t.Helper()
	var g clientapi.VoiceGrant
	e.want(e.do(http.MethodPost, "/voice/rooms/"+room+"/grant", token, map[string]any{"session": session}, &g), http.StatusOK, "grant")
	return g
}
