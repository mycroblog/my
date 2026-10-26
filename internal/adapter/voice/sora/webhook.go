// SPDX-License-Identifier: AGPL-3.0-only

package sora

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/voice"
)

// The webhooks of Sora (ADR-0102 §4, §5): the auth webhook, which Sora calls before every
// connection and which answers 2xx with whether it is allowed, and the event and session
// webhooks, whose connection.created, connection.destroyed, and session.destroyed become
// voice.Events. A self-hosted Sora sends them with Basic authentication (webhook_basic_authn);
// Sora Cloud signs them with the header sora-cloud-signature, "t=<unix time>, v1=<hex>", the
// HMAC-SHA256 of "<t>.<body>" with the project's API key.

// ErrWebhookAuth reports a delivery without the Basic authentication or the signature of the
// mode.
var ErrWebhookAuth = errors.New("sora: the webhook's authentication does not verify")

// ErrWebhookBody reports a delivery whose body is not a webhook of Sora.
var ErrWebhookBody = errors.New("sora: the webhook's body is not a webhook of Sora")

// SignatureTolerance is how far the time of a signature of Sora Cloud may be from now: an older
// delivery is a replay. Sora Cloud does not document one; it gives up a delivery after 10
// seconds.
const SignatureTolerance = 5 * time.Minute

// maxReasonBytes is the longest reason of a refusal that Sora accepts.
const maxReasonBytes = 100

// verify checks the authentication of a delivery.
func (t *Transport) verify(header http.Header, body []byte) error {
	if t.o.Mode == ModeCloud {
		return verifySignature(header.Get("sora-cloud-signature"), body, t.o.CloudAPIKey, t.o.Clock.Now())
	}
	r := http.Request{Header: header}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return ErrWebhookAuth
	}
	// Both are compared, whatever the first gives, so that the time tells nothing.
	u := subtle.ConstantTimeCompare([]byte(user), []byte(t.o.WebhookUsername))
	p := subtle.ConstantTimeCompare([]byte(pass), []byte(t.o.WebhookPassword))
	if u&p != 1 {
		return ErrWebhookAuth
	}
	return nil
}

// verifySignature checks a sora-cloud-signature header of body.
func verifySignature(value string, body []byte, key string, now time.Time) error {
	var ts string
	var sigs []string
	for part := range strings.SplitSeq(value, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return ErrWebhookAuth
	}
	if d := now.Sub(time.Unix(sec, 0)); d > SignatureTolerance || d < -SignatureTolerance {
		return ErrWebhookAuth
	}
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(ts + "."))
	h.Write(body)
	want := h.Sum(nil)
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return ErrWebhookAuth
}

// authRequest is the part of a request of the auth webhook that the adapter reads.
type authRequest struct {
	ChannelID    string `json:"channel_id"`
	ConnectionID string `json:"connection_id"`
	Role         string `json:"role"`
	Metadata     *struct {
		Join string `json:"join"`
	} `json:"metadata"`
}

// authResponse is the answer of the auth webhook: an allowed connection gets the session as
// its client_id and audio only, in Opus (ADR-0102 §3).
type authResponse struct {
	Allowed        bool   `json:"allowed"`
	Reason         string `json:"reason,omitempty"`
	ClientID       string `json:"client_id,omitempty"`
	Audio          *bool  `json:"audio,omitempty"`
	AudioCodecType string `json:"audio_codec_type,omitempty"`
	Video          *bool  `json:"video,omitempty"`
}

// The reasons of the refusals that the adapter decides itself.
const (
	reasonBadRequest = "invalid request"
	reasonJoin       = "invalid or expired join value"
	reasonChannel    = "join value of another channel"
	reasonRole       = "role not granted"
)

// AuthorizeConnect implements voice.ConnectAuthorizer for the auth webhook: a delivery that does
// not authenticate gets 401 and ErrWebhookAuth; otherwise the answer is 200 with whether the
// connection is allowed. The adapter refuses a connection without a valid join value of the
// channel, or that asks for a role the grant did not give (sendonly never); decide applies the
// room domain to the others. It makes no call to Sora: the answer has 5 seconds (10 on Sora
// Cloud).
func (t *Transport) AuthorizeConnect(_ context.Context, header http.Header, body []byte, decide func(voice.ConnectAttempt) voice.ConnectDecision) (int, []byte, error) {
	if err := t.verify(header, body); err != nil {
		return http.StatusUnauthorized, nil, err
	}
	refuse := func(reason string) (int, []byte, error) {
		b, err := json.Marshal(authResponse{Reason: truncate(reason, maxReasonBytes)})
		return http.StatusOK, b, err
	}
	var req authRequest
	if err := json.Unmarshal(body, &req); err != nil || req.ChannelID == "" {
		return refuse(reasonBadRequest)
	}
	if req.Metadata == nil || req.Metadata.Join == "" {
		return refuse(reasonJoin)
	}
	v, err := verifyJoin(req.Metadata.Join, t.o.JoinSecret, t.o.Clock.Now())
	if err != nil {
		return refuse(reasonJoin)
	}
	if req.ChannelID != t.channel(v.Room) {
		return refuse(reasonChannel)
	}
	switch {
	case req.Role == roleRecvOnly:
	case req.Role == roleSendRecv && v.Role == roleSendRecv:
	default:
		return refuse(reasonRole)
	}
	d := decide(voice.ConnectAttempt{
		Room: voice.RoomRef{ID: v.Room}, Session: v.Session, Speaker: req.Role == roleSendRecv, Token: req.Metadata.Join,
	})
	if !d.Allowed {
		reason := d.Reason
		if reason == "" {
			reason = "refused"
		}
		return refuse(reason)
	}
	yes, no := true, false
	b, err := json.Marshal(authResponse{Allowed: true, ClientID: v.Session, Audio: &yes, AudioCodecType: "OPUS", Video: &no})
	return http.StatusOK, b, err
}

// truncate cuts s to at most n bytes, on a rune boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// webhookEvent is the part of an event or a session webhook that the adapter reads.
type webhookEvent struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	ChannelID string `json:"channel_id"`
	ClientID  string `json:"client_id"`
	Data      *struct {
		Reason string `json:"reason"`
	} `json:"data"`
}

// ParseEvents implements voice.EventSource: it verifies the authentication of a delivery and
// decodes its event. The destruction of a connection that another one of the same client_id
// replaced (duplicate_client_id) is not a session that left.
func (t *Transport) ParseEvents(_ context.Context, header http.Header, body []byte) ([]voice.Event, error) {
	if err := t.verify(header, body); err != nil {
		return nil, err
	}
	var e webhookEvent
	if err := json.Unmarshal(body, &e); err != nil || e.Type == "" {
		return nil, ErrWebhookBody
	}
	var kind voice.EventKind
	switch e.Type {
	case "connection.created":
		kind = voice.MediaSessionJoined
	case "connection.destroyed":
		if e.Data != nil && e.Data.Reason == "duplicate_client_id" {
			return nil, nil
		}
		kind = voice.MediaSessionLeft
	case "session.destroyed":
		kind = voice.MediaRoomFinished
	default:
		return nil, nil
	}
	room, ok := t.room(e.ChannelID)
	if e.ID == "" || !ok {
		return nil, ErrWebhookBody
	}
	ev := voice.Event{ID: e.ID, Kind: kind, Room: voice.RoomRef{ID: room}}
	if at, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
		ev.OccurredAt = at.UTC()
	}
	if kind != voice.MediaRoomFinished {
		if e.ClientID == "" {
			return nil, ErrWebhookBody
		}
		ev.Session = e.ClientID
	}
	return []voice.Event{ev}, nil
}
