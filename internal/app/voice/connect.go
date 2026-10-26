// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The authorization of a connection that a media server asks about before it connects
// (ConnectAuthorizer, ADR-0102 §4): Sora calls it on every connection and waits at most 5
// seconds (10 on Sora Cloud), so the decision only reads the projections and the
// KeyValueStore, outside any transaction, and makes no external call.

// MediaConnectResult is the outcome of AuthorizeMediaConnect.
type MediaConnectResult struct {
	// Outcome is MediaWebhookAccepted for an answer of the transport (allowed or not),
	// MediaWebhookRejected for a request that did not authenticate, MediaWebhookNotFound, or
	// MediaWebhookRateLimited.
	Outcome MediaWebhookOutcome
	// Status and Body are the answer of the transport to the media server.
	Status int
	Body   []byte
	// RetryAfter is how long a rate-limited client waits.
	RetryAfter time.Duration
	// Reason is why a request was rejected, for the log.
	Reason error
}

// The reasons of the refusals of a connection, for the media server's log (at most 100 bytes
// on Sora).
const (
	connectReasonRoom        = "room not open"
	connectReasonSession     = "session not live"
	connectReasonParticipant = "not a participant"
	connectReasonBanned      = "banned from the room"
	connectReasonSpeaker     = "not a speaker"
	connectReasonFull        = "room full"
	connectReasonUnavailable = "temporarily unavailable"
)

// AuthorizeMediaConnect answers the request of the media server of the voice chat's method
// that asks whether a connection may connect: the transport verifies the request and its join
// value, and the room decides (decideConnect).
func (s *Service) AuthorizeMediaConnect(ctx context.Context, req MediaWebhook) (MediaConnectResult, error) {
	ca, ok := s.d.Transport.(ConnectAuthorizer)
	if !s.Enabled() || !ok || string(s.d.Config.Transport) != req.Transport {
		return MediaConnectResult{Outcome: MediaWebhookNotFound}, nil
	}
	if s.d.RateLimiter != nil {
		d, err := s.d.RateLimiter.Take(ctx, MediaWebhookIPLimit, kernel.ClientKey(req.ClientIP))
		if err != nil {
			return MediaConnectResult{}, err
		}
		if !d.Allowed {
			return MediaConnectResult{Outcome: MediaWebhookRateLimited, RetryAfter: d.RetryAfter}, nil
		}
	}
	status, body, err := ca.AuthorizeConnect(ctx, req.Header, req.Body, func(a ConnectAttempt) ConnectDecision {
		d, err := s.decideConnect(ctx, a)
		if err != nil {
			s.d.Logger.WarnContext(ctx, "could not decide a voice connection", slog.String("room", a.Room.ID), slog.Any("error", err))
			return ConnectDecision{Reason: connectReasonUnavailable}
		}
		return d
	})
	if err != nil {
		return MediaConnectResult{Outcome: MediaWebhookRejected, Status: status, Body: body, Reason: err}, nil
	}
	return MediaConnectResult{Outcome: MediaWebhookAccepted, Status: status, Body: body}, nil
}

// decideConnect applies the room to a connection (ADR-0102 §4): the room is open on the
// method, the session is the current, live session of a participant who is not banned, a
// connection that publishes audio is a speaker's, and the participant is within the capacity
// of the room, in the order the participants joined.
func (s *Service) decideConnect(ctx context.Context, a ConnectAttempt) (ConnectDecision, error) {
	refuse := func(reason string) (ConnectDecision, error) { return ConnectDecision{Reason: reason}, nil }
	rec, err := s.d.Repository.FindRoom(ctx, nil, a.Room.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		return refuse(connectReasonRoom)
	case err != nil:
		return ConnectDecision{}, err
	case rec.Head.State != voice.StateOpen || rec.Transport != s.d.Config.Transport:
		return refuse(connectReasonRoom)
	}
	b, err := s.d.Heartbeats.kv.Get(ctx, kvHeartbeats, a.Session)
	switch {
	case errors.Is(err, kernel.ErrKeyNotFound):
		return refuse(connectReasonSession)
	case err != nil:
		return ConnectDecision{}, err
	}
	participationID := string(b)
	cur, err := s.d.Heartbeats.current(ctx, participationID)
	if err != nil {
		return ConnectDecision{}, err
	}
	if cur != a.Session {
		return refuse(connectReasonSession)
	}
	ps, err := s.d.Repository.Participants(ctx, nil, a.Room.ID)
	if err != nil {
		return ConnectDecision{}, err
	}
	at := -1
	for i, p := range ps {
		if p.ParticipationID == participationID {
			at = i
			break
		}
	}
	if at < 0 {
		return refuse(connectReasonParticipant)
	}
	p := ps[at]
	banned, err := s.d.Repository.Banned(ctx, nil, a.Room.ID, p.AccountID)
	switch {
	case err != nil:
		return ConnectDecision{}, err
	case banned:
		return refuse(connectReasonBanned)
	case a.Speaker && p.Role != voice.RoleSpeaker:
		return refuse(connectReasonSpeaker)
	case s.d.Config.Capacity > 0 && at >= s.d.Config.Capacity:
		return refuse(connectReasonFull)
	}
	return ConnectDecision{Allowed: true}, nil
}
