// SPDX-License-Identifier: AGPL-3.0-only

// Package sora is the voice.Transport of voice = sora (ADR-0102): a Sora server of Shiguredo,
// self-hosted or Sora Cloud, carries the audio of the rooms. The adapter calls Sora's HTTP API
// with net/http (POST with the header x-sora-target), without an SDK, and it is tested only
// against an httptest stand-in that follows Sora's documents, never against a real Sora server.
//
// A room of my is the Sora channel named by the room's ID (on Sora Cloud "<room>@<project>"),
// and a session is the Sora client_id that the auth webhook gives the connection; the account's
// ID never reaches Sora (ADR-0102 §3). Sora asks the auth webhook before every connection
// (voice.ConnectAuthorizer), which checks the join value of the grant that the client sends in
// the metadata of its connect message and the room.
//
// The role of a Sora connection (sendrecv or recvonly) cannot change: a listener connects
// recvonly, and SetSpeaking always asks the client to connect again with a new grant. A
// revoke and a mute block the audio of the session's connections at once with the channel's
// forwarding filter, which the adapter owns and in whose metadata it keeps the two apart, so
// that an Unmute never lifts a revoke.
package sora

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/voice"
)

// Kind is the Grant.Kind of the Sora transport.
const Kind = "sora"

// GrantTTL is how long the join value of a grant lets a client connect: the auth webhook checks
// it only when the client connects, and the client asks for a new grant to connect again.
const GrantTTL = 60 * time.Second

// The modes of voice_sora_mode.
const (
	ModeSelfHosted = "self_hosted"
	ModeCloud      = "cloud"
)

// CloudAPIURL is the endpoint of the Sora API of Sora Cloud.
const CloudAPIURL = "https://api.sora-cloud.shiguredo.app/sora-api"

// The roles of a Sora connection.
const (
	roleSendRecv = "sendrecv"
	roleRecvOnly = "recvonly"
)

// The x-sora-target of each call of the API.
const (
	targetDisconnectConnection   = "Sora_20151104.DisconnectConnection"
	targetListChannelConnections = "Sora_20201013.ListChannelConnections"
	targetTerminateSession       = "Sora_20230628.TerminateSession"
	targetListForwardingFilters  = "Sora_20230628.ListForwardingFilters"
	targetCreateChannelFilter    = "Sora_20230628.CreateChannelForwardingFilter"
	targetUpdateChannelFilter    = "Sora_20230628.UpdateChannelForwardingFilter"
	targetDeleteChannelFilter    = "Sora_20230628.DeleteChannelForwardingFilter"
)

// Options configure the Transport.
type Options struct {
	// Mode is ModeSelfHosted or ModeCloud (voice_sora_mode).
	Mode string
	// SignalingURLs are the wss: URLs that the browsers connect to (voice_sora_signaling_urls).
	SignalingURLs []string
	// APIURL is the URL of the API that the server calls: the operator's reverse proxy of a
	// self-hosted Sora (voice_sora_api_url), CloudAPIURL when empty on Sora Cloud.
	APIURL string
	// APIHeaderName and APIHeaderValue authenticate the server to the reverse proxy of a
	// self-hosted Sora; empty sends none.
	APIHeaderName  string
	APIHeaderValue string
	// WebhookUsername and WebhookPassword are the Basic authentication of the webhooks of a
	// self-hosted Sora.
	WebhookUsername string
	WebhookPassword string
	// CloudProjectID and CloudAPIKey are the project of Sora Cloud: the key signs the access
	// tokens, authenticates the calls of the API, and verifies the webhooks.
	CloudProjectID string
	CloudAPIKey    string
	// JoinSecret signs the join values of the grants (voice_join_secret).
	JoinSecret string
	// HTTPClient calls the API: the operator configured the server, which may be on a private
	// address, so it is the adapter's own client rather than the SSRF-guarded kernel.HTTPClient
	// (ADR-0102 §1).
	HTTPClient *http.Client
	Clock      kernel.Clock
}

// Transport is the Sora transport.
type Transport struct{ o Options }

var (
	_ voice.Transport           = (*Transport)(nil)
	_ voice.ConnectAuthorizer   = (*Transport)(nil)
	_ voice.EventSource         = (*Transport)(nil)
	_ voice.SessionDeduplicator = (*Transport)(nil)
)

// New returns the Transport of o.
func New(o Options) (*Transport, error) {
	switch o.Mode {
	case ModeSelfHosted:
		if o.APIURL == "" || o.WebhookUsername == "" || o.WebhookPassword == "" {
			return nil, errors.New("sora: self_hosted needs the API URL and the webhook's user name and password")
		}
		if (o.APIHeaderName == "") != (o.APIHeaderValue == "") {
			return nil, errors.New("sora: the API header needs a name and a value")
		}
	case ModeCloud:
		if o.CloudProjectID == "" || o.CloudAPIKey == "" {
			return nil, errors.New("sora: cloud needs the project ID and the API key")
		}
		if o.APIURL == "" {
			o.APIURL = CloudAPIURL
		}
	default:
		return nil, fmt.Errorf("sora: unknown mode %q", o.Mode)
	}
	switch {
	case len(o.SignalingURLs) == 0:
		return nil, errors.New("sora: the signaling URLs are required")
	case o.JoinSecret == "":
		return nil, errors.New("sora: the join secret is required")
	case o.HTTPClient == nil, o.Clock == nil:
		return nil, errors.New("sora: an HTTP client and a clock are required")
	}
	return &Transport{o: o}, nil
}

// Capabilities implements voice.Transport: Sora enforces who speaks, asks before every
// connection, and sends events.
func (*Transport) Capabilities() voice.Capabilities {
	return voice.Capabilities{ServerEnforced: true, ConnectCallback: true, Events: true}
}

// channel returns the Sora channel of a room.
func (t *Transport) channel(room string) string {
	if t.o.Mode == ModeCloud {
		return room + "@" + t.o.CloudProjectID
	}
	return room
}

// room returns the room of a Sora channel, false for a channel of another project.
func (t *Transport) room(channel string) (string, bool) {
	if t.o.Mode != ModeCloud {
		return channel, channel != ""
	}
	room, ok := strings.CutSuffix(channel, "@"+t.o.CloudProjectID)
	return room, ok && room != ""
}

func role(canSpeak bool) string {
	if canSpeak {
		return roleSendRecv
	}
	return roleRecvOnly
}

// Open implements voice.Transport and does nothing: Sora makes the session of a channel when
// its first connection is authorized.
func (*Transport) Open(context.Context, voice.RoomRef) error { return nil }

// Grant implements voice.Transport: a join value of GrantTTL for the session in the room with
// its role, which the client sends as the "join" of the metadata of its connect message; on
// Sora Cloud also the access token, its "access_token". Options name the channel_id and the
// role of the connect message, and all the signaling URLs, comma-separated.
func (t *Transport) Grant(_ context.Context, room voice.RoomRef, s voice.Session, canSpeak bool) (voice.Grant, error) {
	exp := t.o.Clock.Now().Add(GrantTTL)
	channel := t.channel(room.ID)
	value, err := signJoin(joinValue{Room: room.ID, Session: s.ID, Role: role(canSpeak), ExpiresAt: exp.Unix()}, t.o.JoinSecret)
	if err != nil {
		return voice.Grant{}, err
	}
	opts := map[string]string{
		"channel_id":     channel,
		"role":           role(canSpeak),
		"signaling_urls": strings.Join(t.o.SignalingURLs, ","),
	}
	if t.o.Mode == ModeCloud {
		token, err := signAccessToken(accessClaims{
			ChannelID: channel, Role: role(canSpeak), NotBefore: t.o.Clock.Now().Unix(), ExpiresAt: exp.Unix(),
		}, t.o.CloudAPIKey)
		if err != nil {
			return voice.Grant{}, err
		}
		opts["access_token"] = token
	}
	return voice.Grant{Kind: Kind, URL: t.o.SignalingURLs[0], Token: value, Options: opts, ExpiresAt: exp}, nil
}

// SetSpeaking implements voice.Transport: the role of a connection does not change, so the
// client always connects again with a new grant (ADR-0102 §3). A revoke blocks the audio of the
// session's connections at once with the channel's forwarding filter; when the session is
// already revoked, its sendrecv connections are disconnected, for a client that does not
// connect again recvonly keeps none (JobMediaEvent and voice.sweep apply the role of every live
// session again). An allow lifts the revoke, not a mute.
func (t *Transport) SetSpeaking(ctx context.Context, room voice.RoomRef, s voice.Session, canSpeak bool) (bool, error) {
	err := t.change(ctx, room, func(st *filterState, conns []connection) []string {
		if canSpeak {
			st.Revoked = remove(st.Revoked, s.ID)
			return nil
		}
		was := slices.Contains(st.Revoked, s.ID)
		st.Revoked = add(st.Revoked, s.ID)
		if !was {
			return nil
		}
		var drop []string
		for _, c := range conns {
			if c.ClientID == s.ID && c.Role != roleRecvOnly {
				drop = append(drop, c.ConnectionID)
			}
		}
		return drop
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// Mute implements voice.Transport: the channel's forwarding filter blocks the audio of the
// session's connections, and of those it connects later, until Unmute.
func (t *Transport) Mute(ctx context.Context, room voice.RoomRef, s voice.Session) error {
	return t.change(ctx, room, func(st *filterState, _ []connection) []string {
		st.Muted = add(st.Muted, s.ID)
		return nil
	})
}

// Unmute implements voice.Transport: it lifts the mute of the session; the audio of a session
// whose permission to speak was revoked stays blocked.
func (t *Transport) Unmute(ctx context.Context, room voice.RoomRef, s voice.Session) error {
	return t.change(ctx, room, func(st *filterState, _ []connection) []string {
		st.Muted = remove(st.Muted, s.ID)
		return nil
	})
}

func add(ids []string, id string) []string {
	if slices.Contains(ids, id) {
		return ids
	}
	ids = append(ids, id)
	slices.Sort(ids)
	return ids
}

func remove(ids []string, id string) []string {
	return slices.DeleteFunc(ids, func(x string) bool { return x == id })
}

// Remove implements voice.Transport with DisconnectConnection, for every connection of the
// session; a connection that is gone already counts as disconnected.
func (t *Transport) Remove(ctx context.Context, room voice.RoomRef, s voice.Session) error {
	conns, err := t.connections(ctx, room)
	if err != nil {
		return err
	}
	var errs []error
	for _, c := range conns {
		if c.ClientID == s.ID {
			errs = append(errs, t.disconnect(ctx, room, c.ConnectionID))
		}
	}
	return errors.Join(errs...)
}

// Close implements voice.Transport with TerminateSession, which disconnects everyone. A
// channel without connections has no session, or one that ends by itself, and a session that
// is gone already counts as terminated.
func (t *Transport) Close(ctx context.Context, room voice.RoomRef) error {
	conns, err := t.connections(ctx, room)
	if err != nil || len(conns) == 0 {
		return err
	}
	err = t.call(ctx, targetTerminateSession, map[string]any{"channel_id": t.channel(room.ID)}, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// Sessions implements voice.Transport with ListChannelConnections. A session with several
// connections, which Sora allows with default_duplicate_client_id = allow, keeps its newest
// connection: the others are disconnected (ADR-0102 §3).
func (t *Transport) Sessions(ctx context.Context, room voice.RoomRef) ([]voice.SessionRef, error) {
	conns, err := t.connections(ctx, room)
	if err != nil {
		return nil, err
	}
	if err := t.dedupe(ctx, room, conns, ""); err != nil {
		return nil, err
	}
	var out []voice.SessionRef
	for id, c := range newestBySession(conns) {
		out = append(out, voice.SessionRef{ID: id, Connection: c.ConnectionID})
	}
	slices.SortFunc(out, func(a, b voice.SessionRef) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// Deduplicate implements voice.SessionDeduplicator: of the connections of a session, the newest
// stays and the others are disconnected.
func (t *Transport) Deduplicate(ctx context.Context, room voice.RoomRef, s voice.Session) error {
	conns, err := t.connections(ctx, room)
	if err != nil {
		return err
	}
	return t.dedupe(ctx, room, conns, s.ID)
}

// dedupe disconnects the connections of each session but its newest, of session only unless it
// is "".
func (t *Transport) dedupe(ctx context.Context, room voice.RoomRef, conns []connection, session string) error {
	newest := newestBySession(conns)
	for _, c := range conns {
		if c.ClientID == "" || (session != "" && c.ClientID != session) || newest[c.ClientID].ConnectionID == c.ConnectionID {
			continue
		}
		if err := t.disconnect(ctx, room, c.ConnectionID); err != nil {
			return err
		}
	}
	return nil
}

// newestBySession returns the newest connection of each session of conns.
func newestBySession(conns []connection) map[string]connection {
	newest := map[string]connection{}
	for _, c := range conns {
		if c.ClientID == "" {
			continue
		}
		if n, ok := newest[c.ClientID]; !ok || c.newer(n) {
			newest[c.ClientID] = c
		}
	}
	return newest
}

// connection is the part of a connection of ListChannelConnections that the adapter reads.
type connection struct {
	ClientID     string `json:"client_id"`
	ConnectionID string `json:"connection_id"`
	Role         string `json:"role"`
	// CreatedTimestamp is the time of the connection in RFC 3339.
	CreatedTimestamp string `json:"created_timestamp"`
}

// newer reports whether c connected after o.
func (c connection) newer(o connection) bool {
	ct, err1 := time.Parse(time.RFC3339Nano, c.CreatedTimestamp)
	ot, err2 := time.Parse(time.RFC3339Nano, o.CreatedTimestamp)
	if err1 != nil || err2 != nil || ct.Equal(ot) {
		return c.ConnectionID > o.ConnectionID
	}
	return ct.After(ot)
}

// connections returns the connections of the room's channel.
func (t *Transport) connections(ctx context.Context, room voice.RoomRef) ([]connection, error) {
	var conns []connection
	err := t.call(ctx, targetListChannelConnections, map[string]any{"channel_id": t.channel(room.ID)}, &conns)
	return conns, err
}

// disconnect disconnects a connection; one that is gone already counts as disconnected.
func (t *Transport) disconnect(ctx context.Context, room voice.RoomRef, connectionID string) error {
	err := t.call(ctx, targetDisconnectConnection, map[string]any{"channel_id": t.channel(room.ID), "connection_id": connectionID}, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// rule is a rule of a forwarding filter.
type rule struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

// filter is a channel forwarding filter of ListForwardingFilters.
type filter struct {
	Name     string          `json:"name"`
	Action   string          `json:"action"`
	Version  string          `json:"version"`
	Rules    [][]rule        `json:"rules"`
	Metadata json.RawMessage `json:"metadata"`
}

// filterState is what the adapter keeps in the metadata of the channel's forwarding filter: the
// sessions whose audio is blocked, apart for a mute (which Unmute lifts) and a revoke of the
// permission to speak (which only SetSpeaking lifts). The rules block the sendrecv connections
// of these sessions, so a session that connects again stays blocked once the filter is written
// again (SetSpeaking of JobMediaEvent and voice.sweep).
type filterState struct {
	Muted   []string `json:"my_muted,omitempty"`
	Revoked []string `json:"my_revoked,omitempty"`
}

// noConnection is the value of the rule of a filter that blocks no connection: Sora refuses a
// filter without rules.
const noConnection = "-"

// state returns the state that the adapter kept in a filter. The adapter owns the channel's
// filter: one it did not write holds no state and is written over.
func (f filter) state() filterState {
	var st filterState
	if len(f.Metadata) == 0 || json.Unmarshal(f.Metadata, &st) != nil {
		return filterState{}
	}
	return st
}

// change applies fn to the state of the channel's forwarding filter (ADR-0102 §3) and writes the
// filter again: one rule whose connection_id, the senders, are the sendrecv connections of the
// blocked sessions, AND the kind audio. A recvonly connection sends no audio, so it is never
// blocked; a revoked session without a connection is forgotten, as the room applies the role
// again when it connects. fn may return connections to disconnect after the filter is written.
// The filter's version makes two changes that race fail, for the job to try again.
func (t *Transport) change(ctx context.Context, room voice.RoomRef, fn func(st *filterState, conns []connection) []string) error {
	conns, err := t.connections(ctx, room)
	if err != nil {
		return err
	}
	channel := t.channel(room.ID)
	var list struct {
		Filters []filter `json:"channel_forwarding_filters"`
	}
	if err := t.call(ctx, targetListForwardingFilters, map[string]any{"channel_id": channel}, &list); err != nil {
		return err
	}
	var cur *filter
	var st filterState
	if len(list.Filters) > 0 {
		cur = &list.Filters[0]
		st = cur.state()
	}
	before := st
	before.Muted, before.Revoked = slices.Clone(st.Muted), slices.Clone(st.Revoked)
	drop := fn(&st, conns)

	connected := map[string]bool{}
	for _, c := range conns {
		connected[c.ClientID] = true
	}
	st.Revoked = slices.DeleteFunc(st.Revoked, func(id string) bool { return !connected[id] })
	var ids []string
	for _, c := range conns {
		if c.Role == roleRecvOnly || slices.Contains(drop, c.ConnectionID) {
			continue
		}
		if slices.Contains(st.Muted, c.ClientID) || slices.Contains(st.Revoked, c.ClientID) {
			ids = append(ids, c.ConnectionID)
		}
	}
	slices.Sort(ids)
	var errs []error
	if err := t.write(ctx, channel, cur, before, st, ids); err != nil {
		return err
	}
	for _, id := range drop {
		errs = append(errs, t.disconnect(ctx, room, id))
	}
	return errors.Join(errs...)
}

// write writes the channel's forwarding filter of st blocking ids, unless it is so already.
func (t *Transport) write(ctx context.Context, channel string, cur *filter, before, st filterState, ids []string) error {
	empty := len(st.Muted) == 0 && len(st.Revoked) == 0
	switch {
	case empty && cur == nil:
		return nil
	case empty:
		return t.call(ctx, targetDeleteChannelFilter, map[string]any{"channel_id": channel}, nil)
	}
	values := ids
	if len(values) == 0 {
		values = []string{noConnection}
	}
	if cur != nil && slices.Equal(before.Muted, st.Muted) && slices.Equal(before.Revoked, st.Revoked) {
		blocked := cur.blocked()
		slices.Sort(blocked)
		if slices.Equal(blocked, values) {
			return nil
		}
	}
	rules := [][]rule{{
		{Field: "connection_id", Operator: "is_in", Values: values},
		{Field: "kind", Operator: "is_in", Values: []string{"audio"}},
	}}
	version := rand.Text()
	req := map[string]any{"channel_id": channel, "action": "block", "rules": rules, "metadata": st}
	if cur == nil {
		req["version"] = version
		return t.call(ctx, targetCreateChannelFilter, req, nil)
	}
	req["desired_version"] = version
	if cur.Version != "" {
		req["expected_version"] = cur.Version
	}
	return t.call(ctx, targetUpdateChannelFilter, req, nil)
}

// blocked returns the connections whose audio the filter blocks, as the adapter writes it: one
// rule of the senders' connection_id and the kind audio.
func (f filter) blocked() []string {
	var out []string
	for _, and := range f.Rules {
		var ids []string
		audio := false
		for _, r := range and {
			switch {
			case r.Field == "connection_id" && r.Operator == "is_in":
				ids = r.Values
			case r.Field == "kind" && r.Operator == "is_in" && slices.Equal(r.Values, []string{"audio"}):
				audio = true
			}
		}
		if audio {
			out = append(out, ids...)
		}
	}
	return out
}

// APIError is an error response of the Sora API.
type APIError struct {
	Target string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("sora: %s: %d %s", e.Target, e.Status, e.Body)
}

// isNotFound reports an error response of Sora for a session or a connection that it does not
// have: 400 or 404 with an error_type of the form "...-NOT-FOUND" (Sora documents the form of
// its error types, such as INVALID-VERSION, but not the not-found ones; ADR-0102 §2).
func isNotFound(err error) bool {
	var e *APIError
	if !errors.As(err, &e) || (e.Status != http.StatusBadRequest && e.Status != http.StatusNotFound) {
		return false
	}
	var body struct {
		ErrorType string `json:"error_type"`
	}
	return json.Unmarshal([]byte(e.Body), &body) == nil && strings.HasSuffix(body.ErrorType, "-NOT-FOUND")
}

// maxResponseBytes bounds a response of the API that the adapter reads.
const maxResponseBytes = 1 << 20

// maxErrorBytes bounds the body of an error response kept in an APIError.
const maxErrorBytes = 256

// call calls target with req, authenticated for the mode, and decodes the response into res
// unless it is nil.
func (t *Transport) call(ctx context.Context, target string, req, res any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, t.o.APIURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("x-sora-target", target)
	r.Header.Set("Content-Type", "application/json")
	switch {
	case t.o.Mode == ModeCloud:
		r.Header.Set("Authorization", "Bearer "+t.o.CloudAPIKey)
	case t.o.APIHeaderName != "":
		r.Header.Set(t.o.APIHeaderName, t.o.APIHeaderValue)
	}
	resp, err := t.o.HTTPClient.Do(r)
	if err != nil {
		return fmt.Errorf("sora: %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("sora: %s: %w", target, err)
	}
	if resp.StatusCode/100 != 2 {
		if len(b) > maxErrorBytes {
			b = b[:maxErrorBytes]
		}
		return &APIError{Target: target, Status: resp.StatusCode, Body: string(b)}
	}
	if res == nil {
		return nil
	}
	if err := json.Unmarshal(b, res); err != nil {
		return fmt.Errorf("sora: %s: decode: %w", target, err)
	}
	return nil
}
