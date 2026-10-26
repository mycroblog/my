// SPDX-License-Identifier: AGPL-3.0-only

// Package livekit is the voice.Transport of voice = livekit (ADR-0102): a LiveKit server,
// self-hosted or LiveKit Cloud, carries the audio of the rooms. The adapter calls the Twirp API
// of its RoomService with net/http and signs the tokens itself (HS256), without LiveKit's SDK.
//
// A room of my is the LiveKit room named by the room's ID, and a session is the LiveKit
// participant whose identity is the session's ID; the account's ID never reaches LiveKit
// (ADR-0102 §3). The server enforces who speaks: a speaker's token may publish the microphone
// only, a listener's nothing, and SetSpeaking changes the permission of the connected
// participant without a reconnect.
package livekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/voice"
)

// Kind is the Grant.Kind of the LiveKit transport.
const Kind = "livekit"

// GrantTTL is how long the access token of a grant lets a client connect: LiveKit checks it
// only when the client connects, and the client asks for a new grant to connect again.
const GrantTTL = 60 * time.Second

// The timeouts of the LiveKit rooms (ADR-0102 §3), short so that a room nobody connects to does
// not stay on the server: EmptyTimeout closes a room nobody joined, as long as a grant lasts and
// then some; DepartureTimeout closes a room after its last participant left, as long as the
// grace of a disconnected host (voice.HostGracePeriod), so that a host who comes back finds it.
// The JobMediaEvent of a room that LiveKit closed while the room of my is open opens it again.
const (
	EmptyTimeout     = 2 * GrantTTL
	DepartureTimeout = voice.HostGracePeriod
)

// apiTokenTTL is how long the token of one call of the API lasts.
const apiTokenTTL = time.Minute

// sourceMicrophone is the only track source a speaker may publish: the rooms carry audio only.
const sourceMicrophone = "microphone"

// Options configure the Transport.
type Options struct {
	// URL is the wss: URL that the browsers connect to (voice_livekit_url).
	URL string
	// APIURL is the https: URL of the API that the server calls (voice_livekit_api_url).
	APIURL string
	// APIKey and APISecret sign the tokens and verify the webhooks.
	APIKey    string
	APISecret string
	// MaxParticipants bounds a room (voice_max_participants); 0 for no limit.
	MaxParticipants int
	// HTTPClient calls the API: the operator configured the server, which may be on a private
	// address, so it is the adapter's own client rather than the SSRF-guarded kernel.HTTPClient
	// (ADR-0102 §1).
	HTTPClient *http.Client
	Clock      kernel.Clock
}

// Transport is the LiveKit transport.
type Transport struct{ o Options }

var (
	_ voice.Transport   = (*Transport)(nil)
	_ voice.EventSource = (*Transport)(nil)
)

// New returns the Transport of o.
func New(o Options) (*Transport, error) {
	switch {
	case o.URL == "", o.APIURL == "":
		return nil, errors.New("livekit: the URL and the API URL are required")
	case o.APIKey == "", o.APISecret == "":
		return nil, errors.New("livekit: the API key and secret are required")
	case o.HTTPClient == nil, o.Clock == nil:
		return nil, errors.New("livekit: an HTTP client and a clock are required")
	}
	o.APIURL = strings.TrimSuffix(o.APIURL, "/")
	return &Transport{o: o}, nil
}

// Capabilities implements voice.Transport: LiveKit enforces who speaks and sends events.
func (*Transport) Capabilities() voice.Capabilities {
	return voice.Capabilities{ServerEnforced: true, Events: true}
}

// Open implements voice.Transport with CreateRoom, which returns the room when it exists.
func (t *Transport) Open(ctx context.Context, room voice.RoomRef) error {
	req := map[string]any{
		"name":              room.ID,
		"empty_timeout":     int(EmptyTimeout / time.Second),
		"departure_timeout": int(DepartureTimeout / time.Second),
		"max_participants":  t.o.MaxParticipants,
	}
	return t.call(ctx, "CreateRoom", room.ID, req, nil)
}

// Grant implements voice.Transport: an access token of GrantTTL for the session's identity in
// the room, which lets a speaker publish the microphone only and a listener nothing.
func (t *Transport) Grant(_ context.Context, room voice.RoomRef, s voice.Session, canSpeak bool) (voice.Grant, error) {
	now := t.o.Clock.Now()
	exp := now.Add(GrantTTL)
	g := &videoGrant{RoomJoin: true, Room: room.ID, CanSubscribe: ptr(true), CanPublish: ptr(canSpeak), CanPublishData: ptr(false)}
	if canSpeak {
		g.CanPublishSources = []string{sourceMicrophone}
	}
	token, err := sign(claims{
		Issuer: t.o.APIKey, Subject: s.ID, ID: s.ID, NotBefore: now.Unix(), ExpiresAt: exp.Unix(), Video: g,
	}, t.o.APISecret)
	if err != nil {
		return voice.Grant{}, err
	}
	return voice.Grant{Kind: Kind, URL: t.o.URL, Token: token, ExpiresAt: exp}, nil
}

// permission is the ParticipantPermission of UpdateParticipant, which replaces the whole
// permission of the participant.
func permission(canSpeak bool) map[string]any {
	p := map[string]any{"can_subscribe": true, "can_publish": canSpeak, "can_publish_data": false}
	if canSpeak {
		p["can_publish_sources"] = []string{strings.ToUpper(sourceMicrophone)}
	}
	return p
}

// SetSpeaking implements voice.Transport with UpdateParticipant: the connected participant
// may publish the microphone, or no longer publishes (LiveKit unpublishes its tracks), without
// a reconnect. A session that has not connected yet would connect with the permission of its
// earlier grant, so it reports a reconnect: the client connects with a new grant.
func (t *Transport) SetSpeaking(ctx context.Context, room voice.RoomRef, s voice.Session, canSpeak bool) (bool, error) {
	err := t.call(ctx, "UpdateParticipant", room.ID, map[string]any{
		"room": room.ID, "identity": s.ID, "permission": permission(canSpeak),
	}, nil)
	if isNotFound(err) {
		return true, nil
	}
	return false, err
}

// participantInfo is the part of a ParticipantInfo that the adapter reads.
type participantInfo struct {
	Identity string      `json:"identity"`
	Tracks   []trackInfo `json:"tracks"`
}

type trackInfo struct {
	SID string `json:"sid"`
	// Type is the TrackType, its name or its number.
	Type  json.RawMessage `json:"type"`
	Muted bool            `json:"muted"`
}

// audio reports an audio track. AUDIO is the zero value of TrackType, which the JSON of the
// API leaves out.
func (tr trackInfo) audio() bool {
	switch string(tr.Type) {
	case "", "null", "0", `"AUDIO"`:
		return true
	}
	return false
}

// Mute implements voice.Transport: GetParticipant finds the participant's audio tracks, and
// MutePublishedTrack mutes each. The participant can turn the microphone on again from its
// client (ADR-0102 §3). A session that is not connected has nothing to mute.
func (t *Transport) Mute(ctx context.Context, room voice.RoomRef, s voice.Session) error {
	var p participantInfo
	err := t.call(ctx, "GetParticipant", room.ID, map[string]any{"room": room.ID, "identity": s.ID}, &p)
	if isNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, tr := range p.Tracks {
		if !tr.audio() || tr.Muted {
			continue
		}
		err := t.call(ctx, "MutePublishedTrack", room.ID, map[string]any{
			"room": room.ID, "identity": s.ID, "track_sid": tr.SID, "muted": true,
		}, nil)
		if err != nil && !isNotFound(err) {
			return err
		}
	}
	return nil
}

// Unmute implements voice.Transport and does nothing: the participant turns the microphone on
// from its client, which LiveKit lets through after a mute by the server (ADR-0102 §3).
func (*Transport) Unmute(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Remove implements voice.Transport with RemoveParticipant. A self-hosted LiveKit does not
// revoke the participant's token, so the participant may connect again; the JobMediaEvent of
// its participant_joined and voice.sweep remove it again (ADR-0102 §4).
func (t *Transport) Remove(ctx context.Context, room voice.RoomRef, s voice.Session) error {
	err := t.call(ctx, "RemoveParticipant", room.ID, map[string]any{"room": room.ID, "identity": s.ID}, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// Close implements voice.Transport with DeleteRoom, which disconnects everyone.
func (t *Transport) Close(ctx context.Context, room voice.RoomRef) error {
	err := t.call(ctx, "DeleteRoom", room.ID, map[string]any{"room": room.ID}, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// Sessions implements voice.Transport with ListParticipants; a room LiveKit does not have has
// none.
func (t *Transport) Sessions(ctx context.Context, room voice.RoomRef) ([]voice.SessionRef, error) {
	var res struct {
		Participants []participantInfo `json:"participants"`
	}
	err := t.call(ctx, "ListParticipants", room.ID, map[string]any{"room": room.ID}, &res)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]voice.SessionRef, 0, len(res.Participants))
	for _, p := range res.Participants {
		out = append(out, voice.SessionRef{ID: p.Identity})
	}
	return out, nil
}

// apiError is an error response of the Twirp API.
type apiError struct {
	Status int
	Code   string `json:"code"`
	Msg    string `json:"msg"`
}

func (e *apiError) Error() string {
	return fmt.Sprintf("livekit: %d %s: %s", e.Status, e.Code, e.Msg)
}

// isNotFound reports an error of a room or a participant that LiveKit does not have.
func isNotFound(err error) bool {
	var e *apiError
	return errors.As(err, &e) && (e.Code == "not_found" || e.Status == http.StatusNotFound)
}

// maxResponseBytes bounds a response of the API that the adapter reads.
const maxResponseBytes = 1 << 20

// call calls method of the RoomService with req, signed for the room, and decodes the response
// into res unless it is nil.
func (t *Transport) call(ctx context.Context, method, room string, req, res any) error {
	now := t.o.Clock.Now()
	token, err := sign(claims{
		Issuer: t.o.APIKey, NotBefore: now.Unix(), ExpiresAt: now.Add(apiTokenTTL).Unix(),
		Video: &videoGrant{RoomCreate: true, RoomList: true, RoomAdmin: true, Room: room},
	}, t.o.APISecret)
	if err != nil {
		return err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, t.o.APIURL+"/twirp/livekit.RoomService/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	resp, err := t.o.HTTPClient.Do(r)
	if err != nil {
		return fmt.Errorf("livekit: %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("livekit: %s: %w", method, err)
	}
	if resp.StatusCode/100 != 2 {
		e := &apiError{Status: resp.StatusCode}
		_ = json.Unmarshal(b, e)
		return fmt.Errorf("livekit: %s: %w", method, e)
	}
	if res == nil {
		return nil
	}
	if err := json.Unmarshal(b, res); err != nil {
		return fmt.Errorf("livekit: %s: decode: %w", method, err)
	}
	return nil
}
