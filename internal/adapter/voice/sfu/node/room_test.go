// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
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

// TestJoinReadsTheWebSocketWhileTheRoomOffers joins a session while the room is still making the
// slots and offers of other sessions (a burst of joins): the node reads the session's WebSocket
// at once, so that its answer is applied as soon as its own offer is, and not after every offer
// of the burst, which left the clients' connectivity checks unanswered until they gave up.
func TestJoinReadsTheWebSocketWhileTheRoomOffers(t *testing.T) {
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
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	n := New(Options{
		ID: "node-1", URL: url, Capacity: 10, UDPConn: udp, TCPListener: tcp, Loopback: true, Registry: reg,
		Rooms: oneRoom{voice.MediaRoom{Open: true, Sessions: []voice.MediaSession{{ID: "s-1", AccountID: "a-1"}}}},
	})
	srv.Config.Handler = n
	if err := n.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close(context.Background()) })

	// The room is busy with the slots of the joins before this one.
	r, done := n.acquire("room-1")
	r.speakersMu.Lock()
	t.Cleanup(func() { r.speakersMu.Unlock(); done() })

	ticket, err := reg.Issue(t.Context(), sfu.Ticket{Room: "room-1", Session: "s-1", Account: "a-1", Node: "node-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, url+"?ticket="+ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.CloseNow() }()
	if err := ws.Write(ctx, websocket.MessageText, []byte(`{"v":1,"type":"unknown"}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("the node did not read the WebSocket while the room was busy: %v", err)
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil || m.Code != CodeInvalidMessage {
		t.Errorf("message = %s, want the error invalid_message to the message it read", data)
	}
}
