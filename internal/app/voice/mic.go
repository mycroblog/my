// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/domain/voice"
)

// kvMuted keys the sessions that the host or a co-host muted (ADR-0100 §5), until the
// participant turns the microphone on again, by session ID.
const kvMuted = "voice.muted"

// mutedLease is how long a muted session is remembered: past the longest call, so that the
// microphone turned on again lifts the mute of the transport.
const mutedLease = 24 * time.Hour

// ReactionLimit bounds the reactions of each participation (ADR-0100 §10).
var ReactionLimit = kernel.Limit{Name: "voice.reactions", Max: 10, Window: 10 * time.Second}

// ErrInvalidEmoji reports a reaction with an emoji that is neither a Unicode emoji nor an
// enabled custom emoji of this instance (ADR-0100 §10).
var ErrInvalidEmoji = errors.New("voice: the emoji is not one to react with")

// session returns the participant of accountID in a room that it sees, after evaluating the
// expiry of the room, when session is its current live session; ErrNotFound otherwise.
func (s *Service) session(ctx context.Context, accountID, roomID, session string) (RoomView, Participant, error) {
	v, p, err := s.inRoom(ctx, accountID, roomID)
	if err != nil {
		return RoomView{}, Participant{}, err
	}
	if _, err := s.liveParticipant(ctx, accountID, roomID, session); err != nil {
		return RoomView{}, Participant{}, err
	}
	return v, p, nil
}

// Mic turns the microphone of session, the current session of accountID in a room, on or off
// (ADR-0101 §6): nothing of it is recorded, and the streams of the room hear of it. Only a
// speaker turns it on (ErrCannotSpeak); a session that the host or a co-host muted is unmuted
// on the transport first.
func (s *Service) Mic(ctx context.Context, accountID, roomID, session string, on bool) error {
	if err := s.enabled(); err != nil {
		return err
	}
	_, p, err := s.session(ctx, accountID, roomID, session)
	if err != nil {
		return err
	}
	if on {
		if p.Role != voice.RoleSpeaker {
			return ErrCannotSpeak
		}
		if err := s.unmute(ctx, roomID, Session{ID: session, AccountID: accountID}); err != nil {
			return err
		}
	}
	s.publish(stream.Event{Kind: stream.KindVoiceMic, RoomID: roomID, ParticipantID: accountID, On: on})
	return nil
}

// unmute lifts the mute of a session that the host or a co-host muted.
func (s *Service) unmute(ctx context.Context, roomID string, session Session) error {
	_, err := s.d.Heartbeats.kv.Get(ctx, kvMuted, session.ID)
	switch {
	case errors.Is(err, kernel.ErrKeyNotFound):
		return nil
	case err != nil:
		return err
	}
	if err := s.d.Transport.Unmute(ctx, RoomRef{ID: roomID}, session); err != nil {
		s.d.Logger.WarnContext(ctx, "the voice transport did not unmute a session", slog.String("room", roomID), slog.Any("error", err))
		return errors.Join(ErrTransportUnavailable, err)
	}
	return s.d.Heartbeats.kv.Delete(ctx, kvMuted, session.ID)
}

// muted remembers a session that byAccountID muted and tells its stream (ADR-0101 §4).
func (s *Service) muted(ctx context.Context, roomID, session, byAccountID string) {
	if err := s.d.Heartbeats.kv.Set(ctx, kvMuted, session, []byte(byAccountID), mutedLease); err != nil {
		s.d.Logger.WarnContext(ctx, "could not remember a muted voice session", slog.Any("error", err))
	}
	s.publish(stream.Event{Kind: stream.KindVoiceMute, RoomID: roomID, SessionID: session, By: byAccountID})
}

// React sends a reaction of accountID, a participant of a room, to the participants (ADR-0100
// §10): a Unicode emoji or an enabled custom emoji of this instance, which nothing records. It
// fails with ErrInvalidEmoji, and with a *kernel.RateLimitedError past ReactionLimit.
func (s *Service) React(ctx context.Context, accountID, roomID, emoji string) error {
	_, p, err := s.inRoom(ctx, accountID, roomID)
	if err != nil {
		return err
	}
	e, err := s.reactionEmoji(ctx, emoji)
	if err != nil {
		return err
	}
	d, err := s.d.RateLimiter.Take(ctx, ReactionLimit, p.ParticipationID)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	s.publish(stream.Event{Kind: stream.KindVoiceReaction, RoomID: roomID, ParticipantID: accountID, Emoji: e})
	return nil
}

// reactionEmoji returns the emoji to react with, normalized.
func (s *Service) reactionEmoji(ctx context.Context, name string) (string, error) {
	var e string
	if s.d.Emojis != nil {
		var err error
		e, err = s.d.Emojis.ReactionEmoji(ctx, name)
		if errors.Is(err, content.ErrInvalidReaction) {
			return "", fmt.Errorf("%w: %w", ErrInvalidEmoji, err)
		}
		if err != nil {
			return "", err
		}
	} else {
		u, ok := content.NormalizeUnicodeEmoji(name)
		if !ok {
			return "", ErrInvalidEmoji
		}
		e = u
	}
	if e == "" || len(e) > stream.MaxEmojiSize {
		return "", ErrInvalidEmoji
	}
	return e, nil
}

// RefreshGrant returns a new grant for session, the current session of accountID in a room
// (ADR-0101 §5): the client asks for it before the grant it holds expires.
func (s *Service) RefreshGrant(ctx context.Context, accountID, roomID, session string) (Grant, error) {
	if err := s.enabled(); err != nil {
		return Grant{}, err
	}
	_, p, err := s.session(ctx, accountID, roomID, session)
	if err != nil {
		return Grant{}, err
	}
	return s.grant(ctx, roomID, Session{ID: session, AccountID: accountID}, p.Role == voice.RoleSpeaker)
}
