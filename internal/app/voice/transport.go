// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"time"
)

// Transport carries the audio of the rooms (ADR-0101 §2). The room domain is the source
// of truth; a transport enforces what it is told and reports what it saw.
//
// The calls can be external I/O, so they never run inside a transaction: JobApply calls Open,
// SetSpeaking, Mute, Remove, and Close after the change of the room committed, and the
// requests call Grant and Unmute outside their transactions. The adapters live in
// internal/adapter/voice/<name> and pass voicetest.VoiceTransportContract.
type Transport interface {
	Capabilities() Capabilities
	// Open prepares the media path of a room. Idempotent.
	Open(ctx context.Context, room RoomRef) error
	// Grant returns what the client of a participant session needs to connect.
	// canSpeak is whether the session may publish audio now.
	Grant(ctx context.Context, room RoomRef, session Session, canSpeak bool) (Grant, error)
	// SetSpeaking allows or stops the audio of a session.
	SetSpeaking(ctx context.Context, room RoomRef, session Session, canSpeak bool) error
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
	ServerEnforced bool // false for p2p: speaking is enforced by the relay and the peers (ADR-0101 §4)
}

// RoomRef names a room on a transport: the room's ID, which the adapter maps to the name of
// its media server. MaxParticipants is the room's maximum, the smaller of the host's and the
// operator's, on the calls that may place the room (Open, Grant): the media nodes of voice = sfu
// reserve it (ADR-0101 §14). It is 0 when the caller does not know it.
type RoomRef struct {
	ID              string
	MaxParticipants int
}

// Session is one connection of a participant: a new opaque ID every time a device joins
// (ADR-0101 §2), of the participation of AccountID.
type Session struct {
	ID        string
	AccountID string
}

// SessionRef is a session as the media path sees it. AccountID is empty on a media server that
// knows only the session (ADR-0101 §12).
type SessionRef struct {
	ID        string
	AccountID string
}

// Grant is what the client of a session needs to connect (ADR-0101 §2).
type Grant struct {
	Kind      string      // "p2p" or "sfu"
	URL       string      // the media node's wss: URL for sfu; empty for p2p
	Token     string      // the node's single-use ticket for sfu; empty for p2p
	ICE       []ICEServer // STUN / TURN servers with short-lived credentials
	RelayOnly bool        // iceTransportPolicy = "relay"
	ExpiresAt time.Time   // the client asks for a new grant before this
}

// ICEServer is a STUN or TURN server of a grant, the RTCIceServer of the browser.
type ICEServer struct {
	URLs       []string
	Username   string
	Credential string
}

// MaximumReserver is implemented by a transport that reserves the maximum of each room on its
// media servers (sfu, ADR-0101 §14), so that a server never holds more people than its capacity.
// Update calls ReserveMaximum with the larger maximum (room.MaxParticipants, 0 for none) before a
// change that raises it commits, and refuses the change with ErrTransportUnavailable when it
// fails; a lower maximum is left for the transport to notice on its own.
type MaximumReserver interface {
	ReserveMaximum(ctx context.Context, room RoomRef) error
}
