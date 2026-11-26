// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The reconciliation of the media path of a media server with the rooms (ADR-0101 §2, ADR-0101
// §13). The room domain is the source of truth: voice.sweep compares what the media path sees
// with the room and puts it right.

// mediaConnectGrace is how long a session that was given a grant may be missing from the media
// path before the reconciliation disconnects it: the client connects within the life of the
// grant, and a sweep may come at any time.
const mediaConnectGrace = 2 * time.Minute

// kvMediaPending keys, by session ID, the sessions within mediaConnectGrace of their grant.
const kvMediaPending = "voice.media_pending"

// applyState applies the room's current state of a participant to its session on the media
// path: whether it may speak. The session is no longer waiting to connect.
func (s *Service) applyState(ctx context.Context, room RoomRef, p Participant, session string) error {
	if err := s.d.Transport.SetSpeaking(ctx, room, Session{ID: session, AccountID: p.AccountID}, p.Role == voice.RoleSpeaker); err != nil {
		return err
	}
	return s.d.Heartbeats.kv.Delete(ctx, kvMediaPending, session)
}

// takeAway ends a session that the media path lost (ADR-0101 §2): its heartbeat and its place as
// the participation's current session go, so that later heartbeats of it fail as for an ended
// participation and the client joins again with a new session and grant, even while the
// participation lasts (a host in the grace).
// The stream of the session reads the room again and ends (removed, disconnected).
func (s *Service) takeAway(ctx context.Context, p Participant, session string) error {
	if err := s.d.Heartbeats.stop(ctx, p.ParticipationID, session); err != nil {
		return err
	}
	s.publishRoom(p.RoomID)
	return nil
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
	room.MaxParticipants = rec.Revision.Settings.Capacity(s.d.Config.Capacity)
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

// reconcile compares the media path of an open room with the room (ADR-0101 §2, ADR-0101 §13),
// on a transport whose media server holds the sessions, with one call of Sessions per room:
//
//   - a media path that sees nobody while live sessions of the room should be on it has likely
//     lost the room (a media server that restarted or closed it): it is opened again, and the
//     live sessions get mediaConnectGrace to connect again;
//   - every live session of the room that the media path sees gets the room's current state
//     again (applyState), which puts right a change that the media server lost or that a
//     client ignored (ADR-0101 §12);
//   - a live session that the media path does not see past mediaConnectGrace is taken away
//     (takeAway), so that the evaluation of voice.sweep disconnects it (a host enters the grace)
//     even if its client still sends heartbeats;
//   - a session on the media path that is not a live session of the room is removed from it.
//
// A room that nobody ever joined, as a scheduled room that waits, has no media path to reconcile.
func (s *Service) reconcile(ctx context.Context, rec RoomRecord) error {
	t := s.d.Transport
	if !s.Enabled() || !t.Capabilities().ServerEnforced || rec.Transport != s.d.Config.Transport || rec.Head.State != voice.StateOpen {
		return nil
	}
	if rec.Head.ParticipantsCount == 0 {
		// A scheduled room that nobody joined yet has no media path (ADR-0100 §16).
		attended, err := s.d.Repository.Attended(ctx, nil, rec.ID)
		if err != nil || !attended {
			return err
		}
	}
	room := RoomRef{ID: rec.ID, MaxParticipants: rec.Revision.Settings.Capacity(s.d.Config.Capacity)}
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
