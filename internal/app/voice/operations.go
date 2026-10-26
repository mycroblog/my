// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The errors of the participant operations (ADR-0100 §5).
var (
	// ErrNotHostOrCohost reports an operation of the host and the co-hosts by another
	// participant.
	ErrNotHostOrCohost = errors.New("voice: only the host or a co-host may do this")
	// ErrCannotSpeak reports a listener who may not become a speaker without a grant
	// (voice.CanSpeakFreely).
	ErrCannotSpeak = errors.New("voice: the participant may not speak without a grant")
	// ErrInvalidParticipant reports an operation on an account that is not a participant it
	// applies to: one not in the room, the one who operates, the host, or a co-host whom the
	// operation does not apply to.
	ErrInvalidParticipant = errors.New("voice: the operation does not apply to this participant")
	// ErrTooManyCohosts reports a room that has voice.MaxCohosts co-hosts already.
	ErrTooManyCohosts = errors.New("voice: the room has as many co-hosts as it may")
)

// participation returns the participation of p, for the events.
func participation(p Participant) Participation {
	return Participation{ID: p.ParticipationID, RoomID: p.RoomID, AccountID: p.AccountID}
}

// inRoom returns a room that accountID sees and is in, after evaluating its expiry, with the
// account's participant; ErrNotFound otherwise.
func (s *Service) inRoom(ctx context.Context, accountID, roomID string) (RoomView, Participant, error) {
	if err := s.enabled(); err != nil {
		return RoomView{}, Participant{}, err
	}
	if err := s.evaluate(ctx, roomID, false); err != nil {
		return RoomView{}, Participant{}, err
	}
	v, err := s.visibleRoom(ctx, accountID, roomID)
	if err != nil {
		return RoomView{}, Participant{}, err
	}
	for _, p := range v.Participants {
		if p.AccountID == accountID {
			return v, p, nil
		}
	}
	return RoomView{}, Participant{}, ErrNotFound
}

// moderating returns a room that accountID is in as its host or a co-host;
// ErrNotHostOrCohost for another participant.
func (s *Service) moderating(ctx context.Context, accountID, roomID string) (RoomView, error) {
	v, p, err := s.inRoom(ctx, accountID, roomID)
	if err != nil {
		return RoomView{}, err
	}
	if v.Head.HostAccountID != accountID && !p.Cohost() {
		return RoomView{}, ErrNotHostOrCohost
	}
	return v, nil
}

// operand returns the participant accountID of v that byAccountID operates on: neither
// byAccountID nor the host, since nobody operates the host (ADR-0100 §5);
// ErrInvalidParticipant otherwise.
func operand(v RoomView, byAccountID, accountID string) (Participant, error) {
	if accountID == byAccountID || accountID == v.Head.HostAccountID {
		return Participant{}, ErrInvalidParticipant
	}
	for _, p := range v.Participants {
		if p.AccountID == accountID {
			return p, nil
		}
	}
	return Participant{}, ErrInvalidParticipant
}

// authorityTx checks in tx that a room is open and that byAccountID is still its host, or one
// of its co-hosts when cohost is true: ErrNotFound for a room that ended, ErrNotHost or
// ErrNotHostOrCohost otherwise.
func (s *Service) authorityTx(ctx context.Context, tx uow.Tx, roomID, byAccountID string, cohost bool) error {
	rec, err := s.d.Repository.FindRoom(ctx, tx, roomID)
	switch {
	case err != nil:
		return err
	case rec.Head.State != voice.StateOpen:
		return ErrNotFound
	case rec.Head.HostAccountID == byAccountID:
		return nil
	case !cohost:
		return ErrNotHost
	}
	p, err := s.d.Repository.FindParticipant(ctx, tx, byAccountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return ErrNotHostOrCohost
	case err != nil:
		return err
	case p.RoomID != roomID || !p.Cohost():
		return ErrNotHostOrCohost
	}
	return nil
}

// currentTx reads p again in tx, and fails with gone when its participation ended or another
// one replaced it.
func (s *Service) currentTx(ctx context.Context, tx uow.Tx, p Participant, gone error) (Participant, error) {
	cur, err := s.d.Repository.FindParticipant(ctx, tx, p.AccountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return Participant{}, gone
	case err != nil:
		return Participant{}, err
	case cur.ParticipationID != p.ParticipationID:
		return Participant{}, gone
	}
	return cur, nil
}

// roleTx makes p a speaker or a listener, changed by byAccountID, and allows or stops the audio
// of its session, when known, after the commit (ADR-0101 §2). The raised hand of a new speaker
// goes down. A participant in the role already stays as it is.
func (s *Service) roleTx(ctx context.Context, tx uow.Tx, p Participant, role voice.Role, byAccountID, session string, at time.Time) error {
	if p.Role == role {
		return nil
	}
	c := RoleChange{ID: s.d.IDs.NewID(), ParticipationID: p.ParticipationID, Role: role, ChangedByAccountID: byAccountID, ChangedAt: at}
	if err := s.d.Repository.InsertRoleChange(ctx, tx, c); err != nil {
		return err
	}
	if err := tx.Emit(ctx, RoleChanged{Participation: participation(p), Change: c}); err != nil {
		return err
	}
	if role == voice.RoleSpeaker && p.HandRaised {
		if err := s.handTx(ctx, tx, p, voice.HandLowered, at); err != nil {
			return err
		}
	}
	if session == "" {
		return nil
	}
	return s.enqueueApply(ctx, tx, applyPayload{
		Op: applySpeak, Room: p.RoomID, Session: session, Account: p.AccountID, Speak: role == voice.RoleSpeaker,
	}, "role:"+c.ID)
}

// handTx records a change of the raised hand of p.
func (s *Service) handTx(ctx context.Context, tx uow.Tx, p Participant, change voice.HandRaiseChange, at time.Time) error {
	h := HandRaise{ID: s.d.IDs.NewID(), ParticipationID: p.ParticipationID, Change: change, ChangedAt: at}
	if err := s.d.Repository.InsertHandRaise(ctx, tx, h); err != nil {
		return err
	}
	return tx.Emit(ctx, HandRaiseChanged{Participation: participation(p), Change: h})
}

// speakGrantTx grants or revokes the explicit permission to speak of accountID in a room; a
// permission granted or revoked already stays as it is.
func (s *Service) speakGrantTx(ctx context.Context, tx uow.Tx, roomID, accountID string, change voice.SpeakGrantChange, byAccountID string, at time.Time) error {
	granted, err := s.d.Repository.SpeakGranted(ctx, tx, roomID, accountID)
	if err != nil || granted == (change == voice.SpeakGranted) {
		return err
	}
	g := SpeakGrant{ID: s.d.IDs.NewID(), RoomID: roomID, AccountID: accountID, Change: change, ChangedByAccountID: byAccountID, ChangedAt: at}
	if err := s.d.Repository.InsertSpeakGrant(ctx, tx, g); err != nil {
		return err
	}
	return tx.Emit(ctx, SpeakGrantChanged{Grant: g})
}

// changeOwn runs change on the participant of accountID in a room, read again in tx, and
// returns the room after the commit.
func (s *Service) changeOwn(ctx context.Context, accountID, roomID string, change func(tx uow.Tx, p Participant, session string, at time.Time) error) (RoomView, error) {
	_, p, err := s.inRoom(ctx, accountID, roomID)
	if err != nil {
		return RoomView{}, err
	}
	session, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
	if err != nil {
		return RoomView{}, err
	}
	now := s.now()
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		if rec.Head.State != voice.StateOpen {
			return ErrNotFound
		}
		cur, err := s.currentTx(ctx, tx, p, ErrConflict)
		if err != nil {
			return err
		}
		return change(tx, cur, session, now)
	})
	if err != nil {
		return RoomView{}, err
	}
	return s.view(ctx, roomID)
}

// Speak makes accountID, a listener in a room, a speaker when voice.CanSpeakFreely holds for it
// (ADR-0100 §4, §5): ErrCannotSpeak otherwise. A speaker stays one. The relations that other
// modules own (the follows of the host, a limit) are read before the transaction, whose ports
// take none; the transaction reads again what the room owns (the host, the settings, the
// co-host, the explicit permission) and decides there, so that a revocation that commits first
// holds. A host that changed meanwhile fails with ErrConflict.
func (s *Service) Speak(ctx context.Context, accountID, roomID string) (RoomView, error) {
	v, p, err := s.inRoom(ctx, accountID, roomID)
	if err != nil {
		return RoomView{}, err
	}
	if p.Role == voice.RoleSpeaker {
		return v, nil
	}
	viewer, m, err := s.viewer(ctx, accountID)
	if err != nil {
		return RoomView{}, err
	}
	f, err := s.facts(ctx, v, viewer, m)
	if err != nil {
		return RoomView{}, err
	}
	session, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
	if err != nil {
		return RoomView{}, err
	}
	now := s.now()
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		switch {
		case rec.Head.State != voice.StateOpen:
			return ErrNotFound
		case rec.Head.HostAccountID != v.Head.HostAccountID:
			return ErrConflict
		}
		cur, err := s.currentTx(ctx, tx, p, ErrConflict)
		if err != nil {
			return err
		}
		facts := f
		facts.Cohost = cur.Cohost()
		if facts.SpeakGranted, err = s.d.Repository.SpeakGranted(ctx, tx, roomID, accountID); err != nil {
			return err
		}
		if !voice.CanSpeakFreely(policyRoom(RoomView{RoomRecord: rec}), viewer, facts) {
			return ErrCannotSpeak
		}
		return s.roleTx(ctx, tx, cur, voice.RoleSpeaker, accountID, session, now)
	})
	if err != nil {
		return RoomView{}, err
	}
	return s.view(ctx, roomID)
}

// Listen makes accountID, a speaker in a room, a listener again (ADR-0100 §5). The explicit
// permission to speak stays, so the participant may speak again.
func (s *Service) Listen(ctx context.Context, accountID, roomID string) (RoomView, error) {
	return s.changeOwn(ctx, accountID, roomID, func(tx uow.Tx, p Participant, session string, at time.Time) error {
		return s.roleTx(ctx, tx, p, voice.RoleListener, accountID, session, at)
	})
}

// RaiseHand raises the hand of accountID, a listener in a room, to ask to speak (ADR-0100
// §5). A raised hand stays up, and a speaker raises none.
func (s *Service) RaiseHand(ctx context.Context, accountID, roomID string) (RoomView, error) {
	return s.changeOwn(ctx, accountID, roomID, func(tx uow.Tx, p Participant, _ string, at time.Time) error {
		if p.HandRaised || p.Role == voice.RoleSpeaker {
			return nil
		}
		return s.handTx(ctx, tx, p, voice.HandRaised, at)
	})
}

// LowerHand lowers the raised hand of accountID in a room.
func (s *Service) LowerHand(ctx context.Context, accountID, roomID string) (RoomView, error) {
	return s.changeOwn(ctx, accountID, roomID, func(tx uow.Tx, p Participant, _ string, at time.Time) error {
		if !p.HandRaised {
			return nil
		}
		return s.handTx(ctx, tx, p, voice.HandLowered, at)
	})
}

// operate runs change, by byAccountID, the host or a co-host of a room, on the participant
// accountID, read again in tx, and returns the room after the commit.
func (s *Service) operate(ctx context.Context, byAccountID, roomID, accountID string, change func(tx uow.Tx, p Participant, session string, at time.Time) error) (RoomView, error) {
	v, err := s.moderating(ctx, byAccountID, roomID)
	if err != nil {
		return RoomView{}, err
	}
	t, err := operand(v, byAccountID, accountID)
	if err != nil {
		return RoomView{}, err
	}
	session, err := s.d.Heartbeats.current(ctx, t.ParticipationID)
	if err != nil {
		return RoomView{}, err
	}
	now := s.now()
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, _ RoomRecord) error {
		if err := s.authorityTx(ctx, tx, roomID, byAccountID, true); err != nil {
			return err
		}
		cur, err := s.currentTx(ctx, tx, t, ErrInvalidParticipant)
		if err != nil {
			return err
		}
		return change(tx, cur, session, now)
	})
	if err != nil {
		return RoomView{}, err
	}
	return s.view(ctx, roomID)
}

// Grant gives accountID, a participant of a room, the explicit permission to speak, by
// byAccountID, the host or a co-host (ADR-0100 §4, §5), with or without a raised hand: a raised
// hand is granted. The participant becomes a speaker on its own (Speak).
func (s *Service) Grant(ctx context.Context, byAccountID, roomID, accountID string) (RoomView, error) {
	return s.operate(ctx, byAccountID, roomID, accountID, func(tx uow.Tx, p Participant, _ string, at time.Time) error {
		if err := s.speakGrantTx(ctx, tx, roomID, p.AccountID, voice.SpeakGranted, byAccountID, at); err != nil {
			return err
		}
		if !p.HandRaised {
			return nil
		}
		return s.handTx(ctx, tx, p, voice.HandGranted, at)
	})
}

// Dismiss dismisses the raised hand of accountID in a room, by byAccountID, the host or a
// co-host (ADR-0100 §5). A hand that is not raised stays down.
func (s *Service) Dismiss(ctx context.Context, byAccountID, roomID, accountID string) (RoomView, error) {
	return s.operate(ctx, byAccountID, roomID, accountID, func(tx uow.Tx, p Participant, _ string, at time.Time) error {
		if !p.HandRaised {
			return nil
		}
		return s.handTx(ctx, tx, p, voice.HandDismissed, at)
	})
}

// Revoke makes accountID, a participant of a room, a listener again and takes back its
// explicit permission to speak, by byAccountID, the host or a co-host (ADR-0100 §5).
func (s *Service) Revoke(ctx context.Context, byAccountID, roomID, accountID string) (RoomView, error) {
	return s.operate(ctx, byAccountID, roomID, accountID, func(tx uow.Tx, p Participant, session string, at time.Time) error {
		if err := s.roleTx(ctx, tx, p, voice.RoleListener, byAccountID, session, at); err != nil {
			return err
		}
		return s.speakGrantTx(ctx, tx, roomID, p.AccountID, voice.SpeakRevoked, byAccountID, at)
	})
}

// Mute stops the audio of the session of accountID in a room, by byAccountID, the host or a
// co-host (ADR-0100 §5): after the commit, JobApply mutes the session that the participation
// has then, so that a participant who joined again from another device is muted there too. The
// participant may turn the microphone on again. Nothing of it is recorded.
func (s *Service) Mute(ctx context.Context, byAccountID, roomID, accountID string) error {
	_, err := s.operate(ctx, byAccountID, roomID, accountID, func(tx uow.Tx, p Participant, _ string, _ time.Time) error {
		return s.enqueueApply(ctx, tx, applyPayload{
			Op: applyMute, Room: roomID, Participation: p.ParticipationID, Account: p.AccountID,
		}, "mute:"+s.d.IDs.NewID())
	})
	return err
}

// Remove takes accountID out of a room (removed) and bans it from the room, by byAccountID, the
// host or a co-host (ADR-0100 §5): the account neither sees nor joins the room again. The host
// and the co-hosts do not remove each other (ErrInvalidParticipant).
func (s *Service) Remove(ctx context.Context, byAccountID, roomID, accountID string) error {
	var removed *previous
	_, err := s.operate(ctx, byAccountID, roomID, accountID, func(tx uow.Tx, p Participant, session string, at time.Time) error {
		if p.Cohost() {
			return ErrInvalidParticipant
		}
		b := Ban{ID: s.d.IDs.NewID(), RoomID: roomID, AccountID: p.AccountID, BannedByAccountID: byAccountID, BannedAt: at}
		did, err := s.d.Repository.InsertBan(ctx, tx, b)
		if err != nil {
			return err
		}
		if did {
			if err := tx.Emit(ctx, Banned{Ban: b}); err != nil {
				return err
			}
		}
		if _, err := s.endParticipationTx(ctx, tx, p, voice.RemovedFromRoom, session, at); err != nil {
			return err
		}
		removed = &previous{Participant: p, session: session}
		return s.endIfEmptyTx(ctx, tx, roomID, at)
	})
	if err != nil {
		return err
	}
	s.forget(ctx, removed)
	return nil
}

// AssignCohost makes accountID, a participant of a room that byAccountID hosts, a co-host
// (ADR-0100 §5): at most voice.MaxCohosts (ErrTooManyCohosts). The participants in a block
// relation, either way, with the new co-host leave (blocked, §11), as the transaction reads
// them; a participant who joined after the blocks were read fails it with ErrConflict. A co-host
// stays one.
func (s *Service) AssignCohost(ctx context.Context, byAccountID, roomID, accountID string) (RoomView, error) {
	v, err := s.hostRoom(ctx, byAccountID, roomID, true)
	if err != nil {
		return RoomView{}, err
	}
	t, err := operand(v, byAccountID, accountID)
	if err != nil {
		return RoomView{}, err
	}
	if t.Cohost() {
		return v, nil
	}
	if cohosts(v.Participants, byAccountID) >= voice.MaxCohosts {
		return RoomView{}, ErrTooManyCohosts
	}
	blocked, err := s.d.Relations.Blocked(ctx, byAccountID, accountID)
	if err != nil {
		return RoomView{}, err
	}
	if blocked {
		return RoomView{}, ErrInvalidParticipant
	}
	// The blocks are read for the participants now; the transaction fails with ErrConflict
	// for one who joined meanwhile.
	blocks := map[string]bool{}
	sessions := map[string]string{}
	for _, p := range v.Participants {
		if p.AccountID == byAccountID || p.AccountID == accountID {
			continue
		}
		if blocks[p.AccountID], err = s.d.Relations.Blocked(ctx, p.AccountID, accountID); err != nil {
			return RoomView{}, err
		}
		if !blocks[p.AccountID] {
			continue
		}
		if sessions[p.ParticipationID], err = s.d.Heartbeats.current(ctx, p.ParticipationID); err != nil {
			return RoomView{}, err
		}
	}
	now := s.now()
	var left []previous
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, _ RoomRecord) error {
		left = left[:0]
		if err := s.authorityTx(ctx, tx, roomID, byAccountID, false); err != nil {
			return err
		}
		cur, err := s.currentTx(ctx, tx, t, ErrInvalidParticipant)
		if err != nil || cur.Cohost() {
			return err
		}
		ps, err := s.d.Repository.Participants(ctx, tx, roomID)
		if err != nil {
			return err
		}
		if cohosts(ps, byAccountID) >= voice.MaxCohosts {
			return ErrTooManyCohosts
		}
		c := CohostChange{ID: s.d.IDs.NewID(), RoomID: roomID, AccountID: accountID, Change: voice.CohostAssigned, ChangedAt: now}
		if err := s.d.Repository.InsertCohostChange(ctx, tx, c); err != nil {
			return err
		}
		if err := tx.Emit(ctx, CohostChanged{Change: c}); err != nil {
			return err
		}
		for _, p := range ps {
			if p.AccountID == byAccountID || p.AccountID == accountID {
				continue
			}
			blocked, read := blocks[p.AccountID]
			switch {
			case !read:
				return ErrConflict
			case !blocked:
				continue
			}
			l := previous{Participant: p, session: sessions[p.ParticipationID]}
			did, err := s.endParticipationTx(ctx, tx, p, voice.BlockedInRoom, l.session, now)
			if err != nil {
				return err
			}
			if did {
				left = append(left, l)
			}
		}
		return nil
	})
	if err != nil {
		return RoomView{}, err
	}
	for i := range left {
		s.forget(ctx, &left[i])
	}
	return s.view(ctx, roomID)
}

// RemoveCohost makes accountID, a co-host of a room that byAccountID hosts, a participant
// without the role of a co-host (ADR-0100 §5). A participant who is not a co-host stays as it
// is.
func (s *Service) RemoveCohost(ctx context.Context, byAccountID, roomID, accountID string) (RoomView, error) {
	v, err := s.hostRoom(ctx, byAccountID, roomID, true)
	if err != nil {
		return RoomView{}, err
	}
	t, err := operand(v, byAccountID, accountID)
	if err != nil {
		return RoomView{}, err
	}
	if !t.Cohost() {
		return v, nil
	}
	now := s.now()
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, _ RoomRecord) error {
		if err := s.authorityTx(ctx, tx, roomID, byAccountID, false); err != nil {
			return err
		}
		cur, err := s.currentTx(ctx, tx, t, ErrInvalidParticipant)
		if err != nil || !cur.Cohost() {
			return err
		}
		c := CohostChange{ID: s.d.IDs.NewID(), RoomID: roomID, AccountID: accountID, Change: voice.CohostRemoved, ChangedAt: now}
		if err := s.d.Repository.InsertCohostChange(ctx, tx, c); err != nil {
			return err
		}
		return tx.Emit(ctx, CohostChanged{Change: c})
	})
	if err != nil {
		return RoomView{}, err
	}
	return s.view(ctx, roomID)
}

// guards returns the IDs of the host and the co-hosts of v other than accountID: the accounts
// whose blocks with accountID keep it out of the room (ADR-0100 §4, §11).
func guards(v RoomView, accountID string) []string {
	host := v.Head.HostAccountID
	var ids []string
	if host != accountID {
		ids = append(ids, host)
	}
	for _, p := range v.Participants {
		if p.Cohost() && p.AccountID != host && p.AccountID != accountID {
			ids = append(ids, p.AccountID)
		}
	}
	return ids
}

// readBlocks reads, before a transaction, whether accountID is in a block relation, either
// way, with each of the host and the co-hosts of v. The ports of the relations take no
// transaction, so guardTx checks the result against the host and the co-hosts that the
// transaction reads.
func (s *Service) readBlocks(ctx context.Context, v RoomView, accountID string) (map[string]bool, error) {
	ids := guards(v, accountID)
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		blocked, err := s.d.Relations.Blocked(ctx, accountID, id)
		if err != nil {
			return nil, err
		}
		out[id] = blocked
	}
	return out, nil
}

// guardTx reports, in tx, whether accountID is in a block relation with the host or a co-host
// of the room of rec as the transaction reads them, from blocks of readBlocks; ErrConflict for a
// host or a co-host whose block was not read (one that changed meanwhile).
func (s *Service) guardTx(ctx context.Context, tx uow.Tx, rec RoomRecord, accountID string, blocks map[string]bool) (bool, error) {
	ps, err := s.d.Repository.Participants(ctx, tx, rec.ID)
	if err != nil {
		return false, err
	}
	for _, id := range guards(RoomView{RoomRecord: rec, Participants: ps}, accountID) {
		blocked, read := blocks[id]
		if !read {
			return false, ErrConflict
		}
		if blocked {
			return true, nil
		}
	}
	return false, nil
}

// cohosts counts the co-hosts among ps, other than the host.
func cohosts(ps []Participant, host string) int {
	n := 0
	for _, p := range ps {
		if p.Cohost() && p.AccountID != host {
			n++
		}
	}
	return n
}
