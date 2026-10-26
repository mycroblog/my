// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The P2P signaling relay (ADR-0101 §4).
const (
	// MaxSignalSize bounds the session description and the candidate of a signal, in bytes.
	MaxSignalSize = 16 << 10
	// MessageTTL is how long a message for a session waits in the KeyValueStore for the
	// stream of its session: a message that no stream read in time is lost, and the clients
	// restart ICE.
	MessageTTL = 30 * time.Second
	// kvMessages keys the messages for the sessions by message ID.
	kvMessages = "voice.message"
)

// SignalLimit bounds the signals of each session (ADR-0101 §4).
var SignalLimit = kernel.Limit{Name: "voice.signals", Max: 20, Window: time.Second}

// SignalKind is the kind of a signal (ADR-0101 §4).
type SignalKind string

// The kinds of a signal.
const (
	SignalOffer     SignalKind = "offer"
	SignalAnswer    SignalKind = "answer"
	SignalCandidate SignalKind = "candidate"
	SignalRestart   SignalKind = "restart"
)

// Valid reports whether k is a known kind.
func (k SignalKind) Valid() bool {
	switch k {
	case SignalOffer, SignalAnswer, SignalCandidate, SignalRestart:
		return true
	}
	return false
}

// ErrInvalidSignal matches a *SignalError.
var ErrInvalidSignal = errors.New("voice: the signal is not relayed")

// SignalError reports a signal that the relay does not pass on (ADR-0101 §4), by the field of
// the request that is wrong: "kind", "to", "sdp", or "candidate".
type SignalError struct {
	Field string
	Err   error
}

func (e *SignalError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("voice: the signal is not relayed: %s: %v", e.Field, e.Err)
	}
	return "voice: the signal is not relayed: " + e.Field
}

// Is makes errors.Is(err, ErrInvalidSignal) hold.
func (e *SignalError) Is(target error) bool { return target == ErrInvalidSignal }

func (e *SignalError) Unwrap() error { return e.Err }

// SignalInput is a signal from the session Session to the session To.
type SignalInput struct {
	Session   string
	To        string
	Kind      SignalKind
	SDP       string
	Candidate string
}

// Signal is a signal as its receiver sees it: the session that sent it, and its content.
type Signal struct {
	From      string     `json:"from"`
	Kind      SignalKind `json:"kind"`
	SDP       string     `json:"sdp,omitempty"`
	Candidate string     `json:"candidate,omitempty"`
}

// GrantMessage is a new grant for a session that has to connect again (ADR-0101 §2).
type GrantMessage struct {
	Grant     Grant `json:"grant"`
	Reconnect bool  `json:"reconnect"`
}

// Message is a message for one session of a room, which waits in the KeyValueStore for the
// stream of the session: a signal, or a grant.
type Message struct {
	Room   string        `json:"room"`
	To     string        `json:"to"`
	Signal *Signal       `json:"signal,omitempty"`
	Grant  *GrantMessage `json:"grant,omitempty"`
}

// Signal relays a signal of P2P (ADR-0101 §4) from in.Session, the current live session of
// accountID in a room, to in.To, another live session of the room, when one of the two is a
// speaker. The session description of an offer or an answer has audio sections only and, from
// a participant who may not speak, receives only. It fails with ErrNotFound for a session that
// is not the account's, with a *SignalError for a signal that the relay does not pass on, and
// with a *kernel.RateLimitedError past SignalLimit.
func (s *Service) Signal(ctx context.Context, accountID, roomID string, in SignalInput) error {
	if err := s.enabled(); err != nil {
		return err
	}
	if err := checkSignal(in); err != nil {
		return err
	}
	from, err := s.liveParticipant(ctx, accountID, roomID, in.Session)
	if err != nil {
		return err
	}
	d, err := s.d.RateLimiter.Take(ctx, SignalLimit, in.Session)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	v, err := s.view(ctx, roomID)
	if err != nil {
		return err
	}
	to, err := s.sessionParticipant(ctx, v, in.To)
	if err != nil {
		return err
	}
	if to.ParticipationID == from.ParticipationID {
		return &SignalError{Field: "to"}
	}
	for _, p := range v.Participants {
		if p.ParticipationID == from.ParticipationID {
			from = p
		}
	}
	// Two sessions connect when at least one of them is a speaker (P1).
	if from.Role != voice.RoleSpeaker && to.Role != voice.RoleSpeaker {
		return &SignalError{Field: "to"}
	}
	if in.Kind == SignalOffer || in.Kind == SignalAnswer {
		mayPublish, err := s.mayPublish(ctx, v, from)
		if err != nil {
			return err
		}
		if err := voice.CheckSDP(in.SDP, mayPublish); err != nil {
			return &SignalError{Field: "sdp", Err: err}
		}
	}
	sig := Signal{From: in.Session, Kind: in.Kind, SDP: in.SDP, Candidate: in.Candidate}
	return s.send(ctx, Message{Room: roomID, To: in.To, Signal: &sig})
}

// checkSignal checks what a signal holds by itself.
func checkSignal(in SignalInput) error {
	switch {
	case !in.Kind.Valid():
		return &SignalError{Field: "kind"}
	case in.To == "":
		return &SignalError{Field: "to"}
	case len(in.SDP)+len(in.Candidate) > MaxSignalSize:
		if in.SDP != "" {
			return &SignalError{Field: "sdp"}
		}
		return &SignalError{Field: "candidate"}
	case (in.Kind == SignalOffer || in.Kind == SignalAnswer) && in.SDP == "":
		return &SignalError{Field: "sdp"}
	case in.Kind == SignalCandidate && in.Candidate == "":
		return &SignalError{Field: "candidate"}
	}
	return nil
}

// sessionParticipant returns the participant of v whose current session is session and alive;
// a *SignalError for the field to otherwise.
func (s *Service) sessionParticipant(ctx context.Context, v RoomView, session string) (Participant, error) {
	b, err := s.d.Heartbeats.kv.Get(ctx, kvHeartbeats, session)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return Participant{}, &SignalError{Field: "to"}
	}
	if err != nil {
		return Participant{}, err
	}
	for _, p := range v.Participants {
		if p.ParticipationID != string(b) {
			continue
		}
		cur, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
		if err != nil {
			return Participant{}, err
		}
		if cur != session {
			break
		}
		return p, nil
	}
	return Participant{}, &SignalError{Field: "to"}
}

// mayPublish reports whether a participant may send audio: a speaker, or one who may speak
// freely (ADR-0100 §4, ADR-0101 §4).
func (s *Service) mayPublish(ctx context.Context, v RoomView, p Participant) (bool, error) {
	if p.Role == voice.RoleSpeaker {
		return true, nil
	}
	viewer, m, err := s.viewer(ctx, p.AccountID)
	if err != nil {
		return false, err
	}
	f, err := s.facts(ctx, v, viewer, m)
	if err != nil {
		return false, err
	}
	return voice.CanSpeakFreely(policyRoom(v), viewer, f), nil
}

// send puts a message for a session in the KeyValueStore for MessageTTL and signals its ID to
// the streams of the room (ADR-0101 §4, B1).
func (s *Service) send(ctx context.Context, m Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	id := rand.Text()
	if err := s.d.Heartbeats.kv.Set(ctx, kvMessages, id, b, MessageTTL); err != nil {
		return err
	}
	s.publish(stream.Event{Kind: stream.KindVoiceSignal, RoomID: m.Room, MessageID: id})
	return nil
}

// Message returns the message id when it is for the session of st; false for a message of
// another session, or one that is gone. The streams of this process share the reads.
func (s *Service) Message(ctx context.Context, st RoomStream, id string) (Message, bool, error) {
	m, ok, err := s.messages.get(ctx, id, func(ctx context.Context) ([]byte, error) {
		return s.d.Heartbeats.kv.Get(ctx, kvMessages, id)
	})
	if err != nil || !ok || m.Room != st.RoomID || m.To != st.Session {
		return Message{}, false, err
	}
	return m, true, nil
}

// messages keeps the messages that the streams of this process read, until they expire.
type messages struct {
	mu   sync.Mutex
	read map[string]cachedMessage
}

type cachedMessage struct {
	m       Message
	ok      bool
	expires time.Time
}

func newMessages() *messages { return &messages{read: map[string]cachedMessage{}} }

func (c *messages) get(ctx context.Context, id string, read func(context.Context) ([]byte, error)) (Message, bool, error) {
	now := time.Now()
	c.mu.Lock()
	if m, ok := c.read[id]; ok && now.Before(m.expires) {
		c.mu.Unlock()
		return m.m, m.ok, nil
	}
	for k, m := range c.read {
		if !now.Before(m.expires) {
			delete(c.read, k)
		}
	}
	c.mu.Unlock()
	b, err := read(ctx)
	var m cachedMessage
	switch {
	case errors.Is(err, kernel.ErrKeyNotFound):
	case err != nil:
		return Message{}, false, err
	default:
		if err := json.Unmarshal(b, &m.m); err != nil {
			return Message{}, false, fmt.Errorf("decode the voice message %s: %w", id, err)
		}
		m.ok = true
	}
	m.expires = now.Add(MessageTTL)
	c.mu.Lock()
	c.read[id] = m
	c.mu.Unlock()
	return m.m, m.ok, nil
}
