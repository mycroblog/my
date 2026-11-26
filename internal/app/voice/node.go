// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/voice"
)

// MediaRoom is the state of a room that a media node of voice = sfu applies to its connections
// (ADR-0101 §13): the room domain is the source of truth, and the node reads it again on every
// voice_room, voice_mute, and voice_mic signal of the room and once a minute.
type MediaRoom struct {
	// Open is false for a room that ended or that this instance does not have: the node closes
	// every connection of it.
	Open bool
	// MaxParticipants is the room's maximum, the smaller of the host's and the operator's; 0 for
	// none.
	MaxParticipants int
	// Sessions are the current sessions of the participants. A connection whose session is not
	// among them is gone (left, removed, replaced by another device) and is closed.
	Sessions []MediaSession
}

// MediaSession is the current session of a participant of a MediaRoom.
type MediaSession struct {
	ID        string
	AccountID string
	// CanSpeak reports a speaker (ADR-0100 §4): the node forwards the audio of a speaker only.
	CanSpeak bool
	// Muted reports a session that the host or a co-host muted, until the participant turns the
	// microphone on again (ADR-0101 §6): the node drops its audio.
	Muted bool
}

// Session returns the session id of m, found false when it is not current.
func (m MediaRoom) Session(id string) (MediaSession, bool) {
	for _, s := range m.Sessions {
		if s.ID == id {
			return s, true
		}
	}
	return MediaSession{}, false
}

// MediaRoom returns the state of a room for a media node (ADR-0101 §13). A room that is not
// open, of another method, or unknown is returned closed; it fails only when the room cannot be
// read.
func (s *Service) MediaRoom(ctx context.Context, roomID string) (MediaRoom, error) {
	rec, err := s.d.Repository.FindRoom(ctx, nil, roomID)
	switch {
	case errors.Is(err, ErrNotFound):
		return MediaRoom{}, nil
	case err != nil:
		return MediaRoom{}, err
	case rec.Head.State != voice.StateOpen || rec.Transport != s.d.Config.Transport:
		return MediaRoom{}, nil
	}
	ps, err := s.d.Repository.Participants(ctx, nil, roomID)
	if err != nil {
		return MediaRoom{}, err
	}
	out := MediaRoom{Open: true, MaxParticipants: rec.Revision.Settings.Capacity(s.d.Config.Capacity)}
	for _, p := range ps {
		session, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
		if err != nil {
			return MediaRoom{}, err
		}
		if session == "" {
			continue
		}
		_, err = s.d.Heartbeats.kv.Get(ctx, kvMuted, session)
		muted := err == nil
		if err != nil && !errors.Is(err, kernel.ErrKeyNotFound) {
			return MediaRoom{}, err
		}
		out.Sessions = append(out.Sessions, MediaSession{
			ID: session, AccountID: p.AccountID, CanSpeak: p.Role == voice.RoleSpeaker, Muted: muted,
		})
	}
	return out, nil
}
