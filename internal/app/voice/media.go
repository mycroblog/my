// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The events of a media server and the reconciliation of its media path with the rooms
// (ADR-0101 §2, ADR-0102 §4). The room domain is the source of truth: the events only make
// the reconciliation of voice.sweep come sooner, and nothing assumes they arrive.

// JobMediaEvent applies one verified event of a media server (ADR-0102 §4): a session that
// joined the media path and is not a live session of the room is removed from it, and a live one
// loses its older connections (SessionDeduplicator) and gets the room's current state; a session of the room that left, and is not connected again
// with another connection, is taken away and disconnected as by ADR-0100 §6; a media room
// that finished while the room is open is opened again. Its idempotency key is the event's ID.
const JobMediaEvent = "voice.media_event"

// MediaWebhookMaxBodyBytes bounds the body of a delivery of a media server's events, read
// before it is verified.
const MediaWebhookMaxBodyBytes = 256 << 10

// MediaWebhookIPLimit bounds the deliveries of events from one client IP that do not verify
// (ADR-0019 §5); MediaConnectIPLimit those of the requests that authorize a connection. Only a
// request that does not verify counts, so that the traffic of a media server, however busy,
// never makes it refuse a connection or end a session; a flood of forged requests gets 429.
var (
	MediaWebhookIPLimit = kernel.Limit{Name: "voice.webhook.ip", Max: 300, Window: 5 * time.Minute}
	MediaConnectIPLimit = kernel.Limit{Name: "voice.connect.ip", Max: 300, Window: 5 * time.Minute}
)

// limitRejected counts a request from ip that did not verify against lim and reports how long
// the client waits when it sent too many.
func (s *Service) limitRejected(ctx context.Context, lim kernel.Limit, ip string) (time.Duration, bool, error) {
	if s.d.RateLimiter == nil {
		return 0, false, nil
	}
	d, err := s.d.RateLimiter.Take(ctx, lim, kernel.ClientKey(ip))
	if err != nil || d.Allowed {
		return 0, false, err
	}
	return d.RetryAfter, true, nil
}

// mediaConnectGrace is how long a session that was given a grant may be missing from the media
// path before the reconciliation disconnects it: the client connects within the life of the
// grant, and a sweep may come at any time.
const mediaConnectGrace = 2 * time.Minute

// kvMediaPending keys, by session ID, the sessions within mediaConnectGrace of their grant.
const kvMediaPending = "voice.media_pending"

// MediaWebhook is one delivery of a media server's events.
type MediaWebhook struct {
	// Transport is the method the delivery is for, from the path of its endpoint.
	Transport string
	Header    http.Header
	Body      []byte
	ClientIP  string
}

// MediaWebhookOutcome is what became of a delivery.
type MediaWebhookOutcome string

// The outcomes of a delivery.
const (
	// MediaWebhookAccepted: the events were verified and queued (or there were none to queue).
	MediaWebhookAccepted MediaWebhookOutcome = "accepted"
	// MediaWebhookNotFound: the voice chat does not run on the delivery's method (404).
	MediaWebhookNotFound MediaWebhookOutcome = "not_found"
	// MediaWebhookRejected: the delivery did not verify (400).
	MediaWebhookRejected MediaWebhookOutcome = "rejected"
	// MediaWebhookRateLimited: the client IP sent more requests that do not verify than
	// MediaWebhookIPLimit or MediaConnectIPLimit allow (429).
	MediaWebhookRateLimited MediaWebhookOutcome = "rate_limited"
)

// MediaWebhookResult is the outcome of ReceiveMediaEvents.
type MediaWebhookResult struct {
	Outcome MediaWebhookOutcome
	// RetryAfter is how long a rate-limited client waits.
	RetryAfter time.Duration
	// Reason is why a delivery was rejected, for the log; it never holds the delivery.
	Reason error
}

type mediaPayload struct {
	Transport string    `json:"transport"`
	ID        string    `json:"id"`
	Kind      EventKind `json:"kind"`
	Room      string    `json:"room"`
	Session   string    `json:"session,omitempty"`
	// Connection is the media server's ID of the event's connection.
	Connection string `json:"connection,omitempty"`
}

// ReceiveMediaEvents verifies a delivery of the media server of the voice chat's method and
// queues a JobMediaEvent for each of its events. The events are not recorded otherwise
// (ADR-0102 §4).
func (s *Service) ReceiveMediaEvents(ctx context.Context, req MediaWebhook) (MediaWebhookResult, error) {
	src, ok := s.d.Transport.(EventSource)
	if !s.Enabled() || !ok || string(s.d.Config.Transport) != req.Transport {
		return MediaWebhookResult{Outcome: MediaWebhookNotFound}, nil
	}
	evs, err := src.ParseEvents(ctx, req.Header, req.Body)
	if err != nil {
		wait, limited, lerr := s.limitRejected(ctx, MediaWebhookIPLimit, req.ClientIP)
		switch {
		case lerr != nil:
			return MediaWebhookResult{}, lerr
		case limited:
			return MediaWebhookResult{Outcome: MediaWebhookRateLimited, RetryAfter: wait}, nil
		}
		return MediaWebhookResult{Outcome: MediaWebhookRejected, Reason: err}, nil
	}
	if len(evs) == 0 {
		return MediaWebhookResult{Outcome: MediaWebhookAccepted}, nil
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, e := range evs {
			if e.ID == "" || e.Room.ID == "" {
				continue
			}
			b, err := json.Marshal(mediaPayload{Transport: req.Transport, ID: e.ID, Kind: e.Kind, Room: e.Room.ID, Session: e.Session, Connection: e.Connection})
			if err != nil {
				return err
			}
			err = s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
				Type: JobMediaEvent, Payload: b, IdempotencyKey: JobMediaEvent + ":" + req.Transport + ":" + e.ID,
				OrderingKey: "voice_room:" + e.Room.ID,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MediaWebhookResult{}, err
	}
	return MediaWebhookResult{Outcome: MediaWebhookAccepted}, nil
}

// MediaEvent runs JobMediaEvent. An event of another method than the voice chat's now is
// dropped.
func (s *Service) MediaEvent(ctx context.Context, j queue.Job) queue.Result {
	var p mediaPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("decode %s: %w", JobMediaEvent, err))
	}
	if !s.Enabled() || string(s.d.Config.Transport) != p.Transport {
		return queue.Done()
	}
	room := RoomRef{ID: p.Room}
	var err error
	switch p.Kind {
	case MediaSessionJoined:
		err = s.mediaJoined(ctx, room, p.Session)
	case MediaSessionLeft:
		err = s.mediaLeft(ctx, room, p.Session, p.Connection)
	case MediaRoomFinished:
		err = s.reopen(ctx, room)
	default:
		return queue.Done()
	}
	if err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// mediaJoined handles a session that joined the media path: one that is not a live session of
// the room is removed (banned, gone, replaced by another device, or never joined: a
// self-hosted LiveKit lets a removed participant connect again with its token, ADR-0102 §4);
// a live one gets the room's current state applied, since its grant may predate a change.
func (s *Service) mediaJoined(ctx context.Context, room RoomRef, session string) error {
	p, live, err := s.liveParticipant(ctx, room.ID, session)
	if err != nil {
		return err
	}
	if !live {
		return s.d.Transport.Remove(ctx, room, Session{ID: session})
	}
	if d, ok := s.d.Transport.(SessionDeduplicator); ok {
		// A session that connected again, to change its role on Sora, keeps its new
		// connection (ADR-0102 §3).
		if err := d.Deduplicate(ctx, room, Session{ID: session, AccountID: p.AccountID}); err != nil {
			return err
		}
	}
	return s.applyState(ctx, room, p, session)
}

// mediaLeft handles a session that left the media path. A session that is still connected,
// with another connection than the one that left (a full reconnect, or the same identity twice),
// stays. Otherwise the session is taken away (takeAway) and the room evaluated: a participant is
// disconnected, and a host enters the grace (ADR-0100 §6).
func (s *Service) mediaLeft(ctx context.Context, room RoomRef, session, connection string) error {
	p, live, err := s.liveParticipant(ctx, room.ID, session)
	if err != nil || !live {
		return err
	}
	media, err := s.d.Transport.Sessions(ctx, room)
	if err != nil {
		return err
	}
	for _, m := range media {
		if m.ID == session && (connection == "" || m.Connection == "" || m.Connection != connection) {
			return nil
		}
	}
	if err := s.takeAway(ctx, p, session); err != nil {
		return err
	}
	return s.evaluate(ctx, room.ID, false)
}

// applyState applies the room's current state of a participant to its session on the media
// path: whether it may speak. The session is no longer waiting to connect.
func (s *Service) applyState(ctx context.Context, room RoomRef, p Participant, session string) error {
	if _, err := s.d.Transport.SetSpeaking(ctx, room, Session{ID: session, AccountID: p.AccountID}, p.Role == voice.RoleSpeaker); err != nil {
		return err
	}
	return s.d.Heartbeats.kv.Delete(ctx, kvMediaPending, session)
}

// takeAway ends a session that the media path lost (ADR-0101 §2): its heartbeat and its place as
// the participation's current session go, so that later heartbeats of it fail as for an ended
// participation and the client joins again with a new session and grant, even while the
// participation lasts (a host in the grace).
func (s *Service) takeAway(ctx context.Context, p Participant, session string) error {
	return s.d.Heartbeats.stop(ctx, p.ParticipationID, session)
}

// reopen opens the media path of a room again when the media server closed it while the room
// is open, for its participants to connect again.
func (s *Service) reopen(ctx context.Context, room RoomRef) error {
	rec, err := s.d.Repository.FindRoom(ctx, nil, room.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	case rec.Head.State != voice.StateOpen || rec.Transport != s.d.Config.Transport:
		return nil
	}
	return s.d.Transport.Open(ctx, room)
}

// liveParticipant returns the participant of a room whose current, live session is session.
func (s *Service) liveParticipant(ctx context.Context, roomID, session string) (Participant, bool, error) {
	if session == "" {
		return Participant{}, false, nil
	}
	b, err := s.d.Heartbeats.kv.Get(ctx, kvHeartbeats, session)
	switch {
	case errors.Is(err, kernel.ErrKeyNotFound):
		return Participant{}, false, nil
	case err != nil:
		return Participant{}, false, err
	}
	participationID := string(b)
	cur, err := s.d.Heartbeats.current(ctx, participationID)
	if err != nil || cur != session {
		return Participant{}, false, err
	}
	ps, err := s.d.Repository.Participants(ctx, nil, roomID)
	if err != nil {
		return Participant{}, false, err
	}
	for _, p := range ps {
		if p.ParticipationID == participationID {
			return p, true, nil
		}
	}
	return Participant{}, false, nil
}

// markPending gives a session that was just granted mediaConnectGrace to connect to the media
// path before the reconciliation disconnects it.
func (s *Service) markPending(ctx context.Context, session string) {
	if err := s.d.Heartbeats.kv.Set(ctx, kvMediaPending, session, []byte{1}, mediaConnectGrace); err != nil {
		s.d.Logger.WarnContext(ctx, "could not keep the grace of a voice session", slog.Any("error", err))
	}
}

func (s *Service) pending(ctx context.Context, session string) (bool, error) {
	_, err := s.d.Heartbeats.kv.Get(ctx, kvMediaPending, session)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, kernel.ErrKeyNotFound):
		return false, nil
	}
	return false, err
}

// kvMediaReopened keys, by room ID, the rooms whose media path the reconciliation opened again,
// for 2 × mediaConnectGrace: a room that stays empty after that is not opened again until then.
const kvMediaReopened = "voice.media_reopened"

// reconcile compares the media path of an open room with the room (ADR-0101 §2, ADR-0102 §4),
// on a transport whose media server holds the sessions, with one call of Sessions per room:
//
//   - a media path that sees nobody while live sessions of the room should be on it has likely
//     lost the room (a media server that restarted or closed it): it is opened again, and the
//     live sessions get mediaConnectGrace to connect again;
//   - every live session of the room that the media path sees gets the room's current state
//     again (applyState), which puts right a change that the media server lost or that a
//     client ignored (ADR-0102 §3);
//   - a live session that the media path does not see past mediaConnectGrace is taken away
//     (takeAway), so that the evaluation of voice.sweep disconnects it (a host enters the grace)
//     even if its client still sends heartbeats;
//   - a session on the media path that is not a live session of the room is removed from it.
func (s *Service) reconcile(ctx context.Context, rec RoomRecord) error {
	t := s.d.Transport
	if !s.Enabled() || !t.Capabilities().ServerEnforced || rec.Transport != s.d.Config.Transport || rec.Head.State != voice.StateOpen {
		return nil
	}
	room := RoomRef{ID: rec.ID}
	media, err := t.Sessions(ctx, room)
	if err != nil {
		return err
	}
	ps, err := s.d.Repository.Participants(ctx, nil, rec.ID)
	if err != nil {
		return err
	}
	l, err := s.d.Heartbeats.read(ctx, ps)
	if err != nil {
		return err
	}
	type liveSession struct {
		p       Participant
		id      string
		pending bool
	}
	var lives []liveSession
	for _, p := range ps {
		if !l.alive[p.ParticipationID] {
			continue
		}
		session := l.session[p.ParticipationID]
		wait, err := s.pending(ctx, session)
		if err != nil {
			return err
		}
		lives = append(lives, liveSession{p: p, id: session, pending: wait})
	}
	if len(media) == 0 && slices.ContainsFunc(lives, func(ls liveSession) bool { return !ls.pending }) {
		reopened, err := s.d.Heartbeats.kv.Get(ctx, kvMediaReopened, rec.ID)
		switch {
		case errors.Is(err, kernel.ErrKeyNotFound):
			reopened = nil
		case err != nil:
			return err
		}
		if reopened == nil {
			if err := t.Open(ctx, room); err != nil {
				return err
			}
			if err := s.d.Heartbeats.kv.Set(ctx, kvMediaReopened, rec.ID, []byte{1}, 2*mediaConnectGrace); err != nil {
				return err
			}
			for _, ls := range lives {
				s.markPending(ctx, ls.id)
			}
			return nil
		}
	}
	seen := make(map[string]bool, len(media))
	for _, m := range media {
		seen[m.ID] = true
	}
	live := map[string]bool{}
	var errs []error
	for _, ls := range lives {
		live[ls.id] = true
		var err error
		switch {
		case seen[ls.id]:
			err = s.applyState(ctx, room, ls.p, ls.id)
		case !seen[ls.id] && !ls.pending:
			err = s.takeAway(ctx, ls.p, ls.id)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	for _, m := range media {
		if live[m.ID] {
			continue
		}
		if err := t.Remove(ctx, room, Session{ID: m.ID}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
