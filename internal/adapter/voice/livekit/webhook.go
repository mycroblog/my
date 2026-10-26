// SPDX-License-Identifier: AGPL-3.0-only

package livekit

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/voice"
)

// The webhooks of LiveKit (ADR-0102 §4): LiveKit signs each delivery with a JWT of the API key
// in the Authorization header, whose sha256 claim is the base64 SHA-256 of the body. A
// delivery carries one event; the events that tell who is on the media path become
// voice.Events, and the others none.

// ErrWebhookSignature reports a delivery whose token or body hash does not verify.
var ErrWebhookSignature = errors.New("livekit: the webhook's signature does not verify")

// ErrWebhookBody reports a delivery whose body is not an event of LiveKit.
var ErrWebhookBody = errors.New("livekit: the webhook's body is not an event")

// webhookEvent is the part of a WebhookEvent that the adapter reads, in the JSON names of
// LiveKit's webhooks (lowerCamelCase), with the proto names as a fallback.
type webhookEvent struct {
	Event       string          `json:"event"`
	ID          string          `json:"id"`
	CreatedAt   json.RawMessage `json:"createdAt"`
	CreatedAtPB json.RawMessage `json:"created_at"`
	Room        *struct {
		Name string `json:"name"`
	} `json:"room"`
	Participant *struct {
		Identity string `json:"identity"`
	} `json:"participant"`
}

// ParseEvents implements voice.EventSource: it verifies the token of a delivery and the hash of
// its body, and decodes its event.
func (t *Transport) ParseEvents(_ context.Context, header http.Header, body []byte) ([]voice.Event, error) {
	auth := strings.TrimSpace(header.Get("Authorization"))
	auth = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if auth == "" {
		return nil, ErrWebhookSignature
	}
	c, err := verify(auth, t.o.APIKey, t.o.APISecret, t.o.Clock.Now())
	if err != nil {
		return nil, errors.Join(ErrWebhookSignature, err)
	}
	sum := sha256.Sum256(body)
	want := base64.StdEncoding.EncodeToString(sum[:])
	if c.SHA256 == "" || subtle.ConstantTimeCompare([]byte(c.SHA256), []byte(want)) != 1 {
		return nil, ErrWebhookSignature
	}
	var e webhookEvent
	if err := json.Unmarshal(body, &e); err != nil || e.Event == "" {
		return nil, ErrWebhookBody
	}
	var kind voice.EventKind
	switch e.Event {
	case "participant_joined":
		kind = voice.MediaSessionJoined
	case "participant_left", "participant_connection_aborted":
		kind = voice.MediaSessionLeft
	case "room_finished":
		kind = voice.MediaRoomFinished
	default:
		return nil, nil
	}
	if e.ID == "" || e.Room == nil || e.Room.Name == "" {
		return nil, ErrWebhookBody
	}
	ev := voice.Event{ID: e.ID, Kind: kind, Room: voice.RoomRef{ID: e.Room.Name}, OccurredAt: createdAt(e)}
	if kind != voice.MediaRoomFinished {
		if e.Participant == nil || e.Participant.Identity == "" {
			return nil, ErrWebhookBody
		}
		ev.Session = e.Participant.Identity
	}
	return []voice.Event{ev}, nil
}

// createdAt returns the time of an event, in Unix seconds as a number or, from protobuf's JSON,
// a string; the zero time without one.
func createdAt(e webhookEvent) time.Time {
	raw := e.CreatedAt
	if len(raw) == 0 {
		raw = e.CreatedAtPB
	}
	s := strings.Trim(string(raw), `"`)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}
