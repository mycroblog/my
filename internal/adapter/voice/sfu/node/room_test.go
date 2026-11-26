// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

// oneRoom is a RoomSource of one open room.
type oneRoom struct{ room voice.MediaRoom }

func (o oneRoom) MediaRoom(context.Context, string) (voice.MediaRoom, error) { return o.room, nil }

// testNode is a started node on loopback sockets that holds the room st, with its registry,
// meter, and WebSocket URL.
type testNode struct {
	n     *Node
	reg   *sfu.Registry
	meter *testkit.Meter
	url   string
}

func newTestNode(t *testing.T, st voice.MediaRoom) *testNode {
	t.Helper()
	clock := testkit.NewClock(time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC))
	reg := sfu.NewRegistry(testkit.NewKeyValueStore(clock), clock)
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = udp.Close(); _ = tcp.Close() })
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close)
	tn := &testNode{reg: reg, meter: testkit.NewMeter(), url: "ws" + strings.TrimPrefix(srv.URL, "http")}
	tn.n = New(Options{
		ID: "node-1", URL: tn.url, Capacity: 100, UDPConn: udp, TCPListener: tcp, Loopback: true, Registry: reg,
		Rooms: oneRoom{st}, Meter: tn.meter,
	})
	srv.Config.Handler = tn.n
	if err := tn.n.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tn.n.Close(context.Background()) })
	return tn
}

// dial opens the WebSocket of a session of room-1 with a new ticket, with no media.
func (tn *testNode) dial(ctx context.Context, t *testing.T, session, account string) (*websocket.Conn, error) {
	ticket, err := tn.reg.Issue(ctx, sfu.Ticket{Room: "room-1", Session: session, Account: account, Node: "node-1"})
	if err != nil {
		return nil, err
	}
	ws, _, err := websocket.Dial(ctx, tn.url+"?ticket="+ticket, nil)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	return ws, nil
}

// readMessage reads the next message of the node on ws.
func readMessage(ctx context.Context, ws *websocket.Conn) (Message, error) {
	_, data, err := ws.Read(ctx)
	if err != nil {
		return Message{}, err
	}
	var m Message
	err = json.Unmarshal(data, &m)
	return m, err
}

// TestJoinReadsTheWebSocketWhileTheRoomOffers joins a session while the room is still making the
// slots and offers of other sessions (a burst of joins): the node reads the session's WebSocket
// at once, so that its answer is applied as soon as its own offer is, and not after every offer
// of the burst, which left the clients' connectivity checks unanswered until they gave up.
func TestJoinReadsTheWebSocketWhileTheRoomOffers(t *testing.T) {
	tn := newTestNode(t, voice.MediaRoom{Open: true, Sessions: []voice.MediaSession{{ID: "s-1", AccountID: "a-1"}}})

	// The room is busy with the slots of the joins before this one.
	r, done := tn.n.acquire("room-1")
	r.speakersMu.Lock()
	t.Cleanup(func() { r.speakersMu.Unlock(); done() })

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ws, err := tn.dial(ctx, t, "s-1", "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Write(ctx, websocket.MessageText, []byte(`{"v":1,"type":"unknown"}`)); err != nil {
		t.Fatal(err)
	}
	m, err := readMessage(ctx, ws)
	if err != nil {
		t.Fatalf("the node did not read the WebSocket while the room was busy: %v", err)
	}
	if m.Code != CodeInvalidMessage {
		t.Errorf("message = %+v, want the error invalid_message to the message it read", m)
	}
}

// TestBurstOffersWhileAnOfferIsStuck joins 30 sessions into one room at the same moment while the
// negotiation of a session already in it never ends (an offer whose candidates never complete):
// every one of them gets its first offer, since the room makes the offers apart from one
// another and apart from its lock, and the time each waited is recorded in OfferWait.
func TestBurstOffersWhileAnOfferIsStuck(t *testing.T) {
	const burst = 30
	st := voice.MediaRoom{Open: true, Sessions: []voice.MediaSession{{ID: "s-stuck", AccountID: "a-stuck", CanSpeak: true}}}
	for i := range burst {
		st.Sessions = append(st.Sessions, voice.MediaSession{ID: fmt.Sprintf("s-%d", i), AccountID: fmt.Sprintf("a-%d", i)})
	}
	tn := newTestNode(t, st)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	stuck, err := tn.dial(ctx, t, "s-stuck", "a-stuck")
	if err != nil {
		t.Fatal(err)
	}
	if m, err := readMessage(ctx, stuck); err != nil || m.Type != TypeOffer {
		t.Fatalf("the first session's first message = %+v, %v, want an offer", m, err)
	}
	tn.n.mu.Lock()
	p := tn.n.rooms["room-1"].peers["s-stuck"]
	tn.n.mu.Unlock()
	p.mu.Lock()
	t.Cleanup(p.mu.Unlock)

	start := make(chan struct{})
	errs := make([]error, burst)
	var wg sync.WaitGroup
	for i := range burst {
		wg.Go(func() {
			<-start
			ws, err := tn.dial(ctx, t, fmt.Sprintf("s-%d", i), fmt.Sprintf("a-%d", i))
			if err != nil {
				errs[i] = err
				return
			}
			m, err := readMessage(ctx, ws)
			if err == nil && m.Type != TypeOffer {
				err = fmt.Errorf("first message %+v, want an offer", m)
			}
			if err == nil && (len(m.Tracks) != 1 || m.Tracks[0].Participant != "a-stuck") {
				err = fmt.Errorf("offer tracks %+v, want the speaker a-stuck", m.Tracks)
			}
			errs[i] = err
		})
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("session s-%d: %v", i, err)
		}
	}
	if got := len(tn.meter.Measurements(OfferWait.Name, nil)); got != burst+1 {
		t.Errorf("%s has %d measurements, want one per first offer, %d", OfferWait.Name, got, burst+1)
	}
}
