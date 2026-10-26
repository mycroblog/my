// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
// joined the media path and is not a live session of the room is removed from it; a session of
// the room that left loses its heartbeat and is disconnected as by ADR-0100 §6; a media room
// that finished while the room is open is opened again. Its idempotency key is the event's ID.
const JobMediaEvent = "voice.media_event"

// MediaWebhookMaxBodyBytes bounds the body of a delivery of a media server's events, read
// before it is verified.
const MediaWebhookMaxBodyBytes = 256 << 10

// MediaWebhookIPLimit bounds the deliveries from one client IP (ADR-0019 §5): far above what a
// media server sends, so that it only stops a flood of forged requests.
var MediaWebhookIPLimit = kernel.Limit{Name: "voice.webhook.ip", Max: 3000, Window: 5 * time.Minute}

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
	// MediaWebhookRateLimited: the client IP sent more than MediaWebhookIPLimit (429).
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
}

// ReceiveMediaEvents verifies a delivery of the media server of the voice chat's method and
// queues a JobMediaEvent for each of its events. The events are not recorded otherwise
// (ADR-0102 §4).
func (s *Service) ReceiveMediaEvents(ctx context.Context, req MediaWebhook) (MediaWebhookResult, error) {
	src, ok := s.d.Transport.(EventSource)
	if !s.Enabled() || !ok || string(s.d.Config.Transport) != req.Transport {
		return MediaWebhookResult{Outcome: MediaWebhookNotFound}, nil
	}
	if s.d.RateLimiter != nil {
		d, err := s.d.RateLimiter.Take(ctx, MediaWebhookIPLimit, kernel.ClientKey(req.ClientIP))
		if err != nil {
			return MediaWebhookResult{}, err
		}
		if !d.Allowed {
			return MediaWebhookResult{Outcome: MediaWebhookRateLimited, RetryAfter: d.RetryAfter}, nil
		}
	}
	evs, err := src.ParseEvents(ctx, req.Header, req.Body)
	if err != nil {
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
			b, err := json.Marshal(mediaPayload{Transport: req.Transport, ID: e.ID, Kind: e.Kind, Room: e.Room.ID, Session: e.Session})
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
		var live bool
		if live, err = s.liveSession(ctx, p.Room, p.Session); err == nil && !live {
			// Banned, gone, replaced by another device, or never joined: a self-hosted
			// LiveKit lets a removed participant connect again with its token (ADR-0102 §4).
			err = s.d.Transport.Remove(ctx, room, Session{ID: p.Session})
		}
	case MediaSessionLeft:
		var live bool
		if live, err = s.liveSession(ctx, p.Room, p.Session); err == nil && live {
			if err = s.d.Heartbeats.kv.Delete(ctx, kvHeartbeats, p.Session); err == nil {
				err = s.evaluate(ctx, p.Room, false)
			}
		}
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

// liveSession reports whether a session is the current, live session of a participant of a
// room.
func (s *Service) liveSession(ctx context.Context, roomID, session string) (bool, error) {
	if session == "" {
		return false, nil
	}
	b, err := s.d.Heartbeats.kv.Get(ctx, kvHeartbeats, session)
	switch {
	case errors.Is(err, kernel.ErrKeyNotFound):
		return false, nil
	case err != nil:
		return false, err
	}
	participationID := string(b)
	cur, err := s.d.Heartbeats.current(ctx, participationID)
	if err != nil || cur != session {
		return false, err
	}
	ps, err := s.d.Repository.Participants(ctx, nil, roomID)
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		if p.ParticipationID == participationID {
			return true, nil
		}
	}
	return false, nil
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

// reconcile compares the media path of an open room with the room (ADR-0101 §2, ADR-0102 §4),
// on a transport whose media server holds the sessions: a live session of the room that the
// media path does not see past mediaConnectGrace loses its heartbeat, so that the evaluation of
// voice.sweep disconnects it (a host enters the grace); a session on the media path that is not
// a live session of the room is removed from it. One call of Sessions per room.
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
	seen := make(map[string]bool, len(media))
	for _, m := range media {
		seen[m.ID] = true
	}
	live := map[string]bool{}
	var errs []error
	for _, p := range ps {
		session := l.session[p.ParticipationID]
		if !l.alive[p.ParticipationID] {
			continue
		}
		live[session] = true
		if seen[session] {
			continue
		}
		wait, err := s.pending(ctx, session)
		if err == nil && !wait {
			err = s.d.Heartbeats.kv.Delete(ctx, kvHeartbeats, session)
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
