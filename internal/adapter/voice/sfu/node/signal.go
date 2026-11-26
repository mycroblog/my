// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

// The signaling of a node over the WebSocket of a grant (ADR-0101 §11): versioned JSON text
// frames, VoiceSFUClientMessage from the client and VoiceSFUNodeMessage from the node in the
// client API's OpenAPI document.

// Version is the version of the messages, their v.
const Version = 1

// The types of the messages.
const (
	TypeOffer     = "offer"
	TypeAnswer    = "answer"
	TypeCandidate = "candidate"
	TypeRestart   = "restart"
	TypeSpeaking  = "speaking"
	TypeMigrate   = "migrate"
	TypeError     = "error"
)

// The codes of an error message.
const (
	CodeInvalidTicket      = "invalid_ticket"
	CodeInvalidMessage     = "invalid_message"
	CodeUnsupportedVersion = "unsupported_version"
	CodeRoomUnavailable    = "room_unavailable"
)

const (
	// maxMessageBytes bounds a message from a client: an answer of 16 KiB with its envelope.
	maxMessageBytes = 32 << 10
	// PingInterval is how often a node pings a client, and PongTimeout how long it waits for the
	// pong before it closes the WebSocket.
	PingInterval = 15 * time.Second
	PongTimeout  = 10 * time.Second
	// outboxLength bounds the messages that wait for a slow client; one more closes it.
	outboxLength = 32
	// writeTimeout bounds the write of one message.
	writeTimeout = 10 * time.Second
)

// ClientMessage is a message from a client: an answer, a candidate, or a restart.
type ClientMessage struct {
	V         int    `json:"v"`
	Type      string `json:"type"`
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// Track maps the mid of a transceiver of an offer to the account whose audio it carries.
type Track struct {
	Mid         string `json:"mid"`
	Participant string `json:"participant"`
}

// Message is a message that a node sends; Type tells which fields it has.
type Message struct {
	V            int      `json:"v"`
	Type         string   `json:"type"`
	SDP          string   `json:"sdp,omitempty"`
	Tracks       []Track  `json:"tracks,omitempty"`
	Candidate    string   `json:"candidate,omitempty"`
	Participants []string `json:"participants,omitempty"`
	Code         string   `json:"code,omitempty"`
	Message      string   `json:"message,omitempty"`
}

// MarshalJSON writes the fields that the type of m requires even when they are empty: the
// tracks of an offer and the participants of speaking.
func (m Message) MarshalJSON() ([]byte, error) {
	type plain Message
	switch m.Type {
	case TypeOffer:
		return json.Marshal(struct {
			V      int     `json:"v"`
			Type   string  `json:"type"`
			SDP    string  `json:"sdp"`
			Tracks []Track `json:"tracks"`
		}{m.V, m.Type, m.SDP, nonNil(m.Tracks)})
	case TypeSpeaking:
		return json.Marshal(struct {
			V            int      `json:"v"`
			Type         string   `json:"type"`
			Participants []string `json:"participants"`
		}{m.V, m.Type, nonNil(m.Participants)})
	case TypeError:
		return json.Marshal(struct {
			V       int    `json:"v"`
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		}{m.V, m.Type, m.Code, m.Message})
	}
	return json.Marshal(plain(m))
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// errorMessage returns the error message of code.
func errorMessage(code, msg string) Message {
	return Message{V: Version, Type: TypeError, Code: code, Message: msg}
}

// wsConn is the WebSocket of a session: its writer sends the messages of its outbox in order, so
// that a slow client delays nobody, and its pinger keeps it alive.
type wsConn struct {
	c      *websocket.Conn
	outbox chan []byte
	// closing carries the close of the WebSocket after the messages before it.
	closing chan closeFrame
	done    chan struct{}
}

type closeFrame struct {
	code   websocket.StatusCode
	reason string
}

func newWSConn(ctx context.Context, c *websocket.Conn, run func(name string, fn func(context.Context))) *wsConn {
	w := &wsConn{c: c, outbox: make(chan []byte, outboxLength), closing: make(chan closeFrame, 1), done: make(chan struct{})}
	run("voice.sfu.ws_write", w.write)
	run("voice.sfu.ws_ping", func(context.Context) { w.ping(ctx) })
	return w
}

// send queues m; a client whose outbox is full is closed.
func (w *wsConn) send(m Message) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	select {
	case <-w.done:
	case w.outbox <- b:
	default:
		w.close(websocket.StatusPolicyViolation, "too slow")
	}
}

// close closes the WebSocket after the messages already queued.
func (w *wsConn) close(code websocket.StatusCode, reason string) {
	select {
	case w.closing <- closeFrame{code: code, reason: reason}:
	default:
	}
}

func (w *wsConn) write(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case b := <-w.outbox:
			if !w.writeOne(ctx, b) {
				_ = w.c.CloseNow()
				return
			}
		case f := <-w.closing:
			if w.flush(ctx) {
				_ = w.c.Close(f.code, f.reason)
			} else {
				_ = w.c.CloseNow()
			}
			return
		case <-ctx.Done():
			_ = w.c.CloseNow()
			return
		}
	}
}

// flush writes the messages still queued and reports whether they all went.
func (w *wsConn) flush(ctx context.Context) bool {
	for {
		select {
		case b := <-w.outbox:
			if !w.writeOne(ctx, b) {
				return false
			}
		default:
			return true
		}
	}
}

func (w *wsConn) writeOne(ctx context.Context, b []byte) bool {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return w.c.Write(ctx, websocket.MessageText, b) == nil
}

func (w *wsConn) ping(ctx context.Context) {
	t := time.NewTicker(PingInterval)
	defer t.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, PongTimeout)
			err := w.c.Ping(pctx)
			cancel()
			if err != nil {
				_ = w.c.CloseNow()
				return
			}
		}
	}
}
