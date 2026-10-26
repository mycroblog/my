// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// kvStreams keys the one open room stream of a session by session ID (ADR-0101 §3): the
// value is the token of the stream that opened last, and a stream whose token is gone ends.
const kvStreams = "voice.stream"

// snapshotReadTimeout bounds a read of a room that the streams of this process share, which
// does not end with the stream that started it.
const snapshotReadTimeout = 10 * time.Second

// snapshotKeep is how long a shared read of a room stays after it was made, for the streams
// that ask for one made after an earlier time.
const snapshotKeep = time.Minute

// publish hands a signal to the room streams of every process; it never blocks.
func (s *Service) publish(e stream.Event) {
	if s.d.Streams != nil {
		s.d.Streams.Publish(e)
	}
}

// publishRoom signals that a room or its participants changed.
func (s *Service) publishRoom(roomID string) {
	s.publish(stream.Event{Kind: stream.KindVoiceRoom, RoomID: roomID})
}

// RegisterCommitHooks signals every change of a room to its streams once the command that
// made it commits (ADR-0101 §3): the signal voice_room names the room, and each stream reads
// the room again.
func (s *Service) RegisterCommitHooks(r *uow.Projectors) {
	rooms := map[string]func(uow.Event) string{
		EventRoomOpened:           func(e uow.Event) string { return e.(RoomOpened).Room.ID },
		EventRoomRevised:          func(e uow.Event) string { return e.(RoomRevised).Revision.RoomID },
		EventHostChanged:          func(e uow.Event) string { return e.(HostChanged).Change.RoomID },
		EventCohostChanged:        func(e uow.Event) string { return e.(CohostChanged).Change.RoomID },
		EventParticipationStarted: func(e uow.Event) string { return e.(ParticipationStarted).Participation.RoomID },
		EventParticipationEnded:   func(e uow.Event) string { return e.(ParticipationEnded).Participation.RoomID },
		EventRoleChanged:          func(e uow.Event) string { return e.(RoleChanged).Participation.RoomID },
		EventHandRaiseChanged:     func(e uow.Event) string { return e.(HandRaiseChanged).Participation.RoomID },
		EventSpeakGrantChanged:    func(e uow.Event) string { return e.(SpeakGrantChanged).Grant.RoomID },
		EventRoomEnded:            func(e uow.Event) string { return e.(RoomEnded).Ending.RoomID },
	}
	for eventType, room := range rooms {
		r.OnCommit(eventType, func(_ context.Context, e uow.Event) { s.publishRoom(room(e)) })
	}
}

// RoomStream is the open room stream of a session (ADR-0101 §3).
type RoomStream struct {
	RoomID          string
	AccountID       string
	ParticipationID string
	Session         string
	// token tells this stream from a later one of the same session.
	token string
}

// OpenStream opens the room stream of session, the current live session of accountID in a
// room; ErrNotFound for any other session. The stream is the one stream of the session: one
// opened before, in any process, ends at its next check (StreamCurrent).
func (s *Service) OpenStream(ctx context.Context, accountID, roomID, session string) (RoomStream, error) {
	if err := s.enabled(); err != nil {
		return RoomStream{}, err
	}
	if err := s.evaluate(ctx, roomID, false); err != nil {
		return RoomStream{}, err
	}
	p, err := s.liveParticipant(ctx, accountID, roomID, session)
	if err != nil {
		return RoomStream{}, err
	}
	st := RoomStream{RoomID: roomID, AccountID: accountID, ParticipationID: p.ParticipationID, Session: session, token: rand.Text()}
	if err := s.d.Heartbeats.kv.Set(ctx, kvStreams, session, []byte(st.token), sessionLease); err != nil {
		return RoomStream{}, err
	}
	return st, nil
}

// liveParticipant returns the participant of accountID in a room whose current session is
// session and alive; ErrNotFound otherwise.
func (s *Service) liveParticipant(ctx context.Context, accountID, roomID, session string) (Participant, error) {
	p, err := s.d.Repository.FindParticipant(ctx, nil, accountID)
	if err != nil {
		return Participant{}, err
	}
	if p.RoomID != roomID || session == "" {
		return Participant{}, ErrNotFound
	}
	cur, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
	if err != nil {
		return Participant{}, err
	}
	if cur != session {
		return Participant{}, ErrNotFound
	}
	alive, err := s.d.Heartbeats.alive(ctx, session)
	if err != nil {
		return Participant{}, err
	}
	if !alive {
		return Participant{}, ErrNotFound
	}
	return p, nil
}

// StreamCurrent reports whether st is still the one stream of its session.
func (s *Service) StreamCurrent(ctx context.Context, st RoomStream) (bool, error) {
	b, err := s.d.Heartbeats.kv.Get(ctx, kvStreams, st.Session)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return false, nil
	}
	return err == nil && string(b) == st.token, err
}

// CloseStream forgets st as the one stream of its session, unless a later stream replaced it.
func (s *Service) CloseStream(ctx context.Context, st RoomStream) {
	if ok, err := s.StreamCurrent(ctx, st); err == nil && ok {
		_ = s.d.Heartbeats.kv.Delete(ctx, kvStreams, st.Session)
	}
}

// StreamEnd is why a room stream ends.
type StreamEnd string

// The ends of a room stream (ADR-0101 §3).
const (
	// StreamEnded reports a room that ended; the reason is its voice.EndReason.
	StreamEnded StreamEnd = "ended"
	// StreamRemoved reports a participation that ended; the reason is its
	// voice.ParticipationEndReason.
	StreamRemoved StreamEnd = "removed"
	// StreamReplaced reports a session that another one of the participation replaced, when
	// the account joined from another device; the reason is ReasonReplaced.
	StreamReplaced StreamEnd = "replaced"
)

// ReasonReplaced is the reason of StreamReplaced.
const ReasonReplaced = "replaced"

// StreamState is what a room stream sends (ADR-0101 §3): the room as its participant sees it,
// or why the stream ends.
type StreamState struct {
	// End is why the stream ends, "" while it stays open, with its Reason.
	End    StreamEnd
	Reason string
	Room   RoomView
	// Sessions are the live sessions of the participants, by account ID, which the clients
	// connect to each other (ADR-0101 §4).
	Sessions map[string]string
	// Blocked holds the participants in a block relation, either way, with the viewer
	// (ADR-0100 §10).
	Blocked map[string]bool
}

// StreamState reads the room of st as its viewer sees it, from a read of the room that started
// no earlier than notBefore: the streams of this process that ask at about the same time share
// one read (ADR-0101 §3).
func (s *Service) StreamState(ctx context.Context, st RoomStream, notBefore time.Time) (StreamState, error) {
	snap, err := s.snapshots.get(ctx, st.RoomID, notBefore, s.readSnapshot)
	if err != nil {
		return StreamState{}, err
	}
	v := snap.view
	if v.Head.State == voice.StateEnded {
		reason := string(voice.EndHost)
		if v.Ending != nil {
			reason = string(v.Ending.Reason)
		}
		return StreamState{End: StreamEnded, Reason: reason, Room: v}, nil
	}
	mine := false
	ids := make([]string, 0, len(v.Participants))
	for _, p := range v.Participants {
		if p.ParticipationID == st.ParticipationID {
			mine = true
		} else {
			ids = append(ids, p.AccountID)
		}
	}
	if !mine {
		reason := string(voice.LeftRoom)
		end, err := s.d.Repository.FindParticipationEnd(ctx, nil, st.ParticipationID)
		switch {
		case err == nil:
			reason = string(end.Reason)
		case !errors.Is(err, ErrNotFound):
			return StreamState{}, err
		}
		return StreamState{End: StreamRemoved, Reason: reason, Room: v}, nil
	}
	if snap.live.session[st.ParticipationID] != st.Session {
		return StreamState{End: StreamReplaced, Reason: ReasonReplaced, Room: v}, nil
	}
	blocked, err := s.blockedAmong(ctx, st.AccountID, ids)
	if err != nil {
		return StreamState{}, err
	}
	sessions := make(map[string]string, len(v.Participants))
	for _, p := range v.Participants {
		if snap.live.alive[p.ParticipationID] {
			sessions[p.AccountID] = snap.live.session[p.ParticipationID]
		}
	}
	return StreamState{Room: v, Sessions: sessions, Blocked: blocked}, nil
}

// BlockedAmong returns which of the accounts ids are in a block relation, either way, with
// accountID: the mark of the participant list (ADR-0100 §10).
func (s *Service) BlockedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	if err := s.enabled(); err != nil {
		return nil, err
	}
	return s.blockedAmong(ctx, accountID, ids)
}

func (s *Service) blockedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	others := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != accountID {
			others = append(others, id)
		}
	}
	if len(others) == 0 {
		return map[string]bool{}, nil
	}
	return s.d.Relations.BlockedAmong(ctx, accountID, others)
}

// Blocked reports a block relation, either way, between two accounts, for a reaction whose
// sender a stream's last state did not list.
func (s *Service) Blocked(ctx context.Context, a, b string) (bool, error) {
	if a == b {
		return false, nil
	}
	return s.d.Relations.Blocked(ctx, a, b)
}

// snapshot is one read of a room for its streams: the room, its participants, and their
// sessions.
type snapshot struct {
	view RoomView
	live liveness
}

func (s *Service) readSnapshot(ctx context.Context, roomID string) (snapshot, error) {
	v, err := s.view(ctx, roomID)
	if err != nil {
		return snapshot{}, err
	}
	l, err := s.d.Heartbeats.read(ctx, v.Participants)
	if err != nil {
		return snapshot{}, err
	}
	return snapshot{view: v, live: l}, nil
}

// snapshots shares the reads of the rooms among the streams of this process: a stream that
// asks for a read started no earlier than a time takes the newest read that did, or waits for
// the one in progress, or starts one.
type snapshots struct {
	mu    sync.Mutex
	reads map[string]*snapshotRead
}

type snapshotRead struct {
	started time.Time
	done    chan struct{}
	snap    snapshot
	err     error
}

func newSnapshots() *snapshots { return &snapshots{reads: map[string]*snapshotRead{}} }

func (c *snapshots) get(ctx context.Context, roomID string, notBefore time.Time, read func(context.Context, string) (snapshot, error)) (snapshot, error) {
	now := time.Now()
	c.mu.Lock()
	for id, r := range c.reads {
		if now.Sub(r.started) > snapshotKeep {
			delete(c.reads, id)
		}
	}
	r := c.reads[roomID]
	if r == nil || r.started.Before(notBefore) {
		r = &snapshotRead{started: now, done: make(chan struct{})}
		c.reads[roomID] = r
		c.mu.Unlock()
		// The read outlives the stream that started it, for the others that wait for it.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), snapshotReadTimeout)
		r.snap, r.err = read(rctx, roomID)
		cancel()
		close(r.done)
		if r.err != nil {
			c.mu.Lock()
			if c.reads[roomID] == r {
				delete(c.reads, roomID)
			}
			c.mu.Unlock()
		}
		return r.snap, r.err
	}
	c.mu.Unlock()
	select {
	case <-r.done:
		return r.snap, r.err
	case <-ctx.Done():
		return snapshot{}, ctx.Err()
	}
}
