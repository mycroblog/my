// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The liveness of the participants (ADR-0100 §6, ADR-0101 §6).
const (
	// HeartbeatInterval is how often a participating client sends a heartbeat.
	HeartbeatInterval = 15 * time.Second
	// HeartbeatLease is how long a heartbeat keeps its session alive.
	HeartbeatLease = 45 * time.Second
	// HostGracePeriod is how long a room keeps the host whose heartbeat expired.
	HostGracePeriod = 2 * time.Minute
	// sessionLease is how long the current session of a participation is remembered after its
	// last heartbeat: past the lease and the grace of a host, so that a host who comes back in
	// the grace keeps its session.
	sessionLease = time.Hour
)

// The namespaces of the voice module in the KeyValueStore. The session IDs live only there,
// never in the database (ADR-0101 §2).
const (
	// kvHeartbeats keys the live sessions by session ID; the value is the participation ID,
	// and the entry expires HeartbeatLease after the last heartbeat.
	kvHeartbeats = "voice.heartbeat"
	// kvSessions keys the current session of a participation by participation ID.
	kvSessions = "voice.session"
)

// Heartbeats keeps the sessions of the participations and their heartbeats in a
// KeyValueStore (ADR-0100 §6): one current session per participation, replaced when another
// device joins, alive while it sends heartbeats.
type Heartbeats struct {
	kv   kernel.KeyValueStore
	repo Repository
}

// NewHeartbeats returns the Heartbeats on kv, which read the participants of the rooms from
// repo.
func NewHeartbeats(kv kernel.KeyValueStore, repo Repository) *Heartbeats {
	return &Heartbeats{kv: kv, repo: repo}
}

// newSessionID returns a new opaque, unguessable session ID.
func newSessionID() string { return rand.Text() }

// sessionEnding is the value of kvSessions for a participation whose session an evaluation of
// the expiry claimed, to end the participation (claim): a join from another device does not
// replace it and a heartbeat does not renew it until the claim is released or expires.
const sessionEnding = "-"

// sessionValue is the value of kvSessions that holds session, nil for none.
func sessionValue(session string) []byte {
	if session == "" {
		return nil
	}
	return []byte(session)
}

// start makes sessionID the current, live session of participationID, a participation that
// starts now.
func (h *Heartbeats) start(ctx context.Context, participationID, sessionID string) error {
	if err := h.kv.Set(ctx, kvSessions, participationID, []byte(sessionID), sessionLease); err != nil {
		return err
	}
	return h.kv.Set(ctx, kvHeartbeats, sessionID, []byte(participationID), HeartbeatLease)
}

// replace makes sessionID the current, live session of participationID in place of old ("" for
// none), and reports whether old was still the current session: a join from another device
// that raced another one, or an evaluation that claimed the participation, replaces nothing.
func (h *Heartbeats) replace(ctx context.Context, participationID, old, sessionID string) (bool, error) {
	ok, err := h.kv.CompareAndSwap(ctx, kvSessions, participationID, sessionValue(old), []byte(sessionID), sessionLease)
	if err != nil || !ok {
		return false, err
	}
	if err := h.kv.Set(ctx, kvHeartbeats, sessionID, []byte(participationID), HeartbeatLease); err != nil {
		return false, err
	}
	if old != "" {
		if err := h.kv.Delete(ctx, kvHeartbeats, old); err != nil {
			return false, err
		}
	}
	return true, nil
}

// raw returns the value of kvSessions of a participation, "" without one.
func (h *Heartbeats) raw(ctx context.Context, participationID string) (string, error) {
	b, err := h.kv.Get(ctx, kvSessions, participationID)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return "", nil
	}
	return string(b), err
}

// current returns the current session of a participation, "" without one or while an
// evaluation claims it.
func (h *Heartbeats) current(ctx context.Context, participationID string) (string, error) {
	s, err := h.raw(ctx, participationID)
	if err != nil || s == sessionEnding {
		return "", err
	}
	return s, nil
}

// alive reports whether a session sent a heartbeat within HeartbeatLease.
func (h *Heartbeats) alive(ctx context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, nil
	}
	_, err := h.kv.Get(ctx, kvHeartbeats, sessionID)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, kernel.ErrKeyNotFound):
		return false, nil
	}
	return false, err
}

// beat renews the session of a participation while it is the current one; ErrNotFound for a
// session that another one replaced, that was taken away, or that an evaluation claimed. The
// heartbeat is written first, so that an evaluation that claims the session meanwhile sees it
// alive and releases the claim.
func (h *Heartbeats) beat(ctx context.Context, participationID, sessionID string) error {
	if err := h.kv.Set(ctx, kvHeartbeats, sessionID, []byte(participationID), HeartbeatLease); err != nil {
		return err
	}
	ok, err := h.kv.CompareAndSwap(ctx, kvSessions, participationID, []byte(sessionID), []byte(sessionID), sessionLease)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// stop forgets sessionID, a session of a participation, and its place as the current session
// while it holds it: a session that replaced it stays.
func (h *Heartbeats) stop(ctx context.Context, participationID, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := h.kv.Delete(ctx, kvHeartbeats, sessionID); err != nil {
		return err
	}
	_, err := h.kv.CompareAndSwap(ctx, kvSessions, participationID, []byte(sessionID), nil, 0)
	return err
}

// claim marks the session of a participation that an evaluation read as raw (the value of
// kvSessions) and found expired, so that no join from another device replaces it and no
// heartbeat renews it while the evaluation ends the participation. It reports claimed when the
// mark is the evaluation's, and alive, with nothing claimed, when the session changed or came
// back since the read. A session that another evaluation claimed since the read is neither:
// it is still expired, and that evaluation ends the participation or releases it.
func (h *Heartbeats) claim(ctx context.Context, participationID, raw string) (claimed, alive bool, err error) {
	ok, err := h.kv.CompareAndSwap(ctx, kvSessions, participationID, sessionValue(raw), []byte(sessionEnding), HeartbeatLease)
	if err != nil {
		return false, false, err
	}
	if !ok {
		cur, err := h.raw(ctx, participationID)
		if err != nil {
			return false, false, err
		}
		return false, cur != sessionEnding, nil
	}
	if raw == "" || raw == sessionEnding {
		return true, false, nil
	}
	back, err := h.alive(ctx, raw)
	if err != nil || !back {
		return err == nil, false, err
	}
	// A heartbeat landed since the read: the session stays.
	return false, true, h.release(ctx, participationID, raw)
}

// release gives back the session that claim marked, raw as claim read it.
func (h *Heartbeats) release(ctx context.Context, participationID, raw string) error {
	if raw == sessionEnding {
		return nil // another evaluation claimed it first
	}
	_, err := h.kv.CompareAndSwap(ctx, kvSessions, participationID, []byte(sessionEnding), sessionValue(raw), sessionLease)
	return err
}

// liveness is what the heartbeats tell of the participants of a room at one time.
type liveness struct {
	// session is the current session of each participation read, "" without one.
	session map[string]string
	// raw is the value of kvSessions of each participation read (Heartbeats.raw).
	raw map[string]string
	// alive holds the participations whose session is alive.
	alive map[string]bool
}

// expired reports whether a participation was read and its session is not alive. A
// participation that joined after the read is not expired.
func (l liveness) expired(participationID string) bool {
	_, read := l.session[participationID]
	return read && !l.alive[participationID]
}

// read reads the liveness of participants.
func (h *Heartbeats) read(ctx context.Context, participants []Participant) (liveness, error) {
	l := liveness{session: make(map[string]string, len(participants)), raw: make(map[string]string, len(participants)), alive: make(map[string]bool, len(participants))}
	for _, p := range participants {
		raw, err := h.raw(ctx, p.ParticipationID)
		if err != nil {
			return liveness{}, err
		}
		l.raw[p.ParticipationID] = raw
		s := raw
		if s == sessionEnding {
			s = ""
		}
		ok, err := h.alive(ctx, s)
		if err != nil {
			return liveness{}, err
		}
		l.session[p.ParticipationID], l.alive[p.ParticipationID] = s, ok
	}
	return l, nil
}

// LiveSessions returns the sessions of a room that are alive: the media path of a transport
// without a media server of its own (p2p, ADR-0101 §2).
func (h *Heartbeats) LiveSessions(ctx context.Context, roomID string) ([]SessionRef, error) {
	ps, err := h.repo.Participants(ctx, nil, roomID)
	if err != nil {
		return nil, err
	}
	l, err := h.read(ctx, ps)
	if err != nil {
		return nil, err
	}
	var out []SessionRef
	for _, p := range ps {
		if l.alive[p.ParticipationID] {
			out = append(out, SessionRef{ID: l.session[p.ParticipationID], AccountID: p.AccountID})
		}
	}
	return out, nil
}

// LiveSessionSource is what the p2p transport reads its sessions from: *Heartbeats implements
// it.
type LiveSessionSource interface {
	LiveSessions(ctx context.Context, roomID string) ([]SessionRef, error)
}

var _ LiveSessionSource = (*Heartbeats)(nil)
