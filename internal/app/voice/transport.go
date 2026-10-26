// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"net/http"
	"time"
)

// Transport carries the audio of the rooms (ADR-0101 §2). The room domain is the source
// of truth; a transport enforces what it is told and reports what it saw.
//
// The calls can be external I/O, so they never run inside a transaction: JobApply calls Open,
// SetSpeaking, Mute, Remove, and Close after the change of the room committed, and the
// requests call Grant and Unmute outside their transactions. The adapters live in
// internal/adapter/voice/<name> and pass testkit.VoiceTransportContract.
type Transport interface {
	Capabilities() Capabilities
	// Open prepares the media path of a room. Idempotent.
	Open(ctx context.Context, room RoomRef) error
	// Grant returns what the client of a participant session needs to connect.
	// canSpeak is whether the session may publish audio now.
	Grant(ctx context.Context, room RoomRef, session Session, canSpeak bool) (Grant, error)
	// SetSpeaking allows or stops the audio of a session. Reconnect reports that the
	// client must connect again with a new grant for the change to take effect.
	SetSpeaking(ctx context.Context, room RoomRef, session Session, canSpeak bool) (reconnect bool, err error)
	// Mute stops the audio of a session; Unmute lifts it when the participant turns the
	// microphone on again.
	Mute(ctx context.Context, room RoomRef, session Session) error
	Unmute(ctx context.Context, room RoomRef, session Session) error
	// Remove disconnects a session from the media path. Idempotent.
	Remove(ctx context.Context, room RoomRef, session Session) error
	// Close ends the media path of a room. Idempotent.
	Close(ctx context.Context, room RoomRef) error
	// Sessions returns the sessions the media path sees now, for reconciliation.
	Sessions(ctx context.Context, room RoomRef) ([]SessionRef, error)
}

// Capabilities tell the room domain how to drive a transport.
type Capabilities struct {
	ServerEnforced  bool // false for p2p: speaking is enforced by the relay and the peers (ADR-0101 §4)
	ConnectCallback bool // the media server asks before every connect (ConnectAuthorizer)
	Events          bool // the media server sends events (EventSource)
}

// RoomRef names a room on a transport: the room's ID, which the adapter maps to the name of
// its media server.
type RoomRef struct{ ID string }

// Session is one connection of a participant: a new opaque ID every time a device joins
// (ADR-0101 §2), of the participation of AccountID.
type Session struct {
	ID        string
	AccountID string
}

// SessionRef is a session as the media path sees it.
type SessionRef struct {
	ID        string
	AccountID string
}

// Grant is what the client of a session needs to connect (ADR-0101 §2).
type Grant struct {
	Kind      string            // "p2p", "livekit", "sora"
	URL       string            // the SFU endpoint; empty for p2p
	Token     string            // the SFU access token; empty for p2p
	Options   map[string]string // method-specific options for the client (ADR-0102)
	ICE       []ICEServer       // STUN / TURN servers with short-lived credentials
	RelayOnly bool              // iceTransportPolicy = "relay"
	ExpiresAt time.Time         // the client asks for a new grant before this
}

// ICEServer is a STUN or TURN server of a grant, the RTCIceServer of the browser.
type ICEServer struct {
	URLs       []string
	Username   string
	Credential string
}

// ConnectAuthorizer is implemented by a transport whose media server asks before every
// connect; decide applies the room domain (bans, roles, one room per account).
type ConnectAuthorizer interface {
	AuthorizeConnect(ctx context.Context, header http.Header, body []byte, decide func(ConnectAttempt) ConnectDecision) (status int, response []byte, err error)
}

// ConnectAttempt is a connection that a media server asks about.
type ConnectAttempt struct {
	Room    RoomRef
	Session string
	// Speaker reports a connection that publishes audio.
	Speaker bool
	// Token is the join value that the client presented, which the adapter verified.
	Token string
}

// ConnectDecision is the answer of the room domain to a ConnectAttempt.
type ConnectDecision struct {
	Allowed bool
	// Reason is why a connection is refused, for the media server's log.
	Reason string
}

// EventSource is implemented by a transport whose media server sends events.
type EventSource interface {
	// ParseEvents verifies and decodes one delivery of events.
	ParseEvents(ctx context.Context, header http.Header, body []byte) ([]Event, error)
}

// EventKind is what a media server reports.
type EventKind string

// The kinds of the events of a media server.
const (
	MediaSessionJoined EventKind = "joined"
	MediaSessionLeft   EventKind = "left"
	MediaRoomFinished  EventKind = "room_finished"
)

// Event is one event of a media server; ID deduplicates it.
type Event struct {
	ID         string
	Kind       EventKind
	Room       RoomRef
	Session    string
	OccurredAt time.Time
}
