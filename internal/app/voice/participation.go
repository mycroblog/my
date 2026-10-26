// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// previous is the participation of an account in another room, which joining a room ends
// (moved, ADR-0100 §5).
type previous struct {
	Participant
	session string
}

// leaveForJoin reads the participation of accountID in a room other than roomID, which a join
// ends. A host of an open room leaves it first (ErrHostingElsewhere, ADR-0100 §5).
func (s *Service) leaveForJoin(ctx context.Context, accountID, roomID string) (*previous, error) {
	p, err := s.d.Repository.FindParticipant(ctx, nil, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	case p.RoomID == roomID:
		return nil, nil
	}
	rec, err := s.d.Repository.FindRoom(ctx, nil, p.RoomID)
	if err != nil {
		return nil, err
	}
	if rec.Head.State == voice.StateOpen && rec.Head.HostAccountID == accountID {
		return nil, ErrHostingElsewhere
	}
	session, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
	if err != nil {
		return nil, err
	}
	return &previous{Participant: p, session: session}, nil
}

// moveTx ends inside tx the participation of accountID in a room other than roomID (moved),
// before the account joins roomID, and ends that room when nobody is left in it. It never
// relies on the unique participant of an account to tell another room: the participation
// ends first, and a join that races another join of the account fails with ErrConflict.
func (s *Service) moveTx(ctx context.Context, tx uow.Tx, accountID, roomID string, prev *previous, at time.Time) error {
	p, err := s.d.Repository.FindParticipant(ctx, tx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	case p.RoomID == roomID, prev == nil, prev.ParticipationID != p.ParticipationID:
		// The account joined a room meanwhile, whose room this transaction did not lock.
		return ErrConflict
	}
	rec, err := s.d.Repository.FindRoom(ctx, tx, p.RoomID)
	if err != nil {
		return err
	}
	if rec.Head.State == voice.StateOpen && rec.Head.HostAccountID == accountID {
		return ErrHostingElsewhere
	}
	if _, err := s.endParticipationTx(ctx, tx, p, voice.MovedRoom, prev.session, at); err != nil {
		return err
	}
	return s.endIfEmptyTx(ctx, tx, p.RoomID, at)
}

// startParticipationTx records a participation, as a speaker or a listener.
func (s *Service) startParticipationTx(ctx context.Context, tx uow.Tx, part Participation, role voice.Role) error {
	if err := s.d.Repository.InsertParticipation(ctx, tx, part); err != nil {
		return err
	}
	if role == voice.RoleSpeaker {
		c := RoleChange{ID: s.d.IDs.NewID(), ParticipationID: part.ID, Role: role, ChangedByAccountID: part.AccountID, ChangedAt: part.JoinedAt}
		if err := s.d.Repository.InsertRoleChange(ctx, tx, c); err != nil {
			return err
		}
	}
	return tx.Emit(ctx, ParticipationStarted{Participation: part})
}

// joinFailed forgets the session of a participation that did not start and maps a race of
// the one participation of an account to ErrConflict.
func (s *Service) joinFailed(ctx context.Context, participationID, session string, err error) error {
	if stop := s.d.Heartbeats.stop(ctx, participationID, session); stop != nil {
		s.d.Logger.WarnContext(ctx, "could not forget a voice session", slog.Any("error", stop))
	}
	if errors.Is(err, ErrInAnotherRoom) {
		return ErrConflict
	}
	return err
}

// forget forgets the session of a participation that ended, after the commit.
func (s *Service) forget(ctx context.Context, prev *previous) {
	if prev == nil {
		return
	}
	if err := s.d.Heartbeats.stop(ctx, prev.ParticipationID, prev.session); err != nil {
		s.d.Logger.WarnContext(ctx, "could not forget a voice session", slog.Any("error", err))
	}
}

// forgetAll forgets the sessions of participants whose participations ended.
func (s *Service) forgetAll(ctx context.Context, ps []Participant) {
	for _, p := range ps {
		session, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
		if err == nil {
			err = s.d.Heartbeats.stop(ctx, p.ParticipationID, session)
		}
		if err != nil {
			s.d.Logger.WarnContext(ctx, "could not forget a voice session", slog.Any("error", err))
		}
	}
}

// joined returns the room and the grant of a new session.
func (s *Service) joined(ctx context.Context, roomID string, session Session, canSpeak bool) (Joined, error) {
	g, err := s.grant(ctx, roomID, session, canSpeak)
	if err != nil {
		return Joined{}, err
	}
	v, err := s.view(ctx, roomID)
	if err != nil {
		return Joined{}, err
	}
	return Joined{Room: v, Session: session.ID, Grant: g}, nil
}

// Join joins accountID to a room that it sees, as a listener, with a new session (ADR-0100 §5,
// ADR-0101 §6). A participant of the room who joins again, from another device, keeps the
// participation with a new session, and the previous session is disconnected. A participation
// in another room ends (moved), but a host of an open room leaves it first
// (ErrHostingElsewhere). A room as full as the operator allows fails with ErrRoomFull.
func (s *Service) Join(ctx context.Context, accountID, roomID string) (Joined, error) {
	if err := s.enabled(); err != nil {
		return Joined{}, err
	}
	if err := s.evaluate(ctx, roomID, false); err != nil {
		return Joined{}, err
	}
	v, err := s.visibleRoom(ctx, accountID, roomID)
	if err != nil {
		return Joined{}, err
	}
	for _, p := range v.Participants {
		if p.AccountID == accountID {
			return s.rejoin(ctx, p)
		}
	}
	prev, err := s.leaveForJoin(ctx, accountID, roomID)
	if err != nil {
		return Joined{}, err
	}
	capacity := s.d.Config.Capacity
	if capacity > 0 && len(v.Participants) >= capacity {
		return Joined{}, ErrRoomFull
	}
	now := s.now()
	part := Participation{ID: s.d.IDs.NewID(), RoomID: roomID, AccountID: accountID, JoinedAt: now}
	session := newSessionID()
	if err := s.d.Heartbeats.start(ctx, part.ID, session); err != nil {
		return Joined{}, err
	}
	locked := []string{roomID}
	if prev != nil {
		locked = append(locked, prev.RoomID)
	}
	err = s.roomTx(ctx, locked, func(tx uow.Tx, rec RoomRecord) error {
		if rec.Head.State != voice.StateOpen {
			return ErrNotFound
		}
		if capacity > 0 {
			ps, err := s.d.Repository.Participants(ctx, tx, roomID)
			if err != nil {
				return err
			}
			if len(ps) >= capacity {
				return ErrRoomFull
			}
		}
		if err := s.moveTx(ctx, tx, accountID, roomID, prev, now); err != nil {
			return err
		}
		return s.startParticipationTx(ctx, tx, part, voice.RoleListener)
	})
	if err != nil {
		return Joined{}, s.joinFailed(ctx, part.ID, session, err)
	}
	s.forget(ctx, prev)
	return s.joined(ctx, roomID, Session{ID: session, AccountID: accountID}, false)
}

// rejoin gives a participant a new session, which replaces the previous one: the previous
// session is disconnected from the media path (ADR-0101 §2).
func (s *Service) rejoin(ctx context.Context, p Participant) (Joined, error) {
	old, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
	if err != nil {
		return Joined{}, err
	}
	session := newSessionID()
	if err := s.d.Heartbeats.start(ctx, p.ParticipationID, session); err != nil {
		return Joined{}, err
	}
	if old != "" {
		if err := s.d.Heartbeats.kv.Delete(ctx, kvHeartbeats, old); err != nil {
			return Joined{}, err
		}
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return s.enqueueApply(ctx, tx, applyPayload{Op: applyRemove, Room: p.RoomID, Session: old, Account: p.AccountID}, "replace:"+old)
		})
		if err != nil {
			return Joined{}, err
		}
	}
	return s.joined(ctx, p.RoomID, Session{ID: session, AccountID: p.AccountID}, p.Role == voice.RoleSpeaker)
}

// LeaveInput is the choice of a host who leaves a room with participants (ADR-0100 §6): the
// next host, an eligible participant, or ending the room.
type LeaveInput struct {
	NextHostID string
	End        bool
}

// Leave takes accountID out of a room (left). The host of a room with other participants
// chooses the next host among the eligible participants or ends the room
// (ErrHostMustChoose, ErrInvalidNextHost); a room that nobody is left in ends (empty).
func (s *Service) Leave(ctx context.Context, accountID, roomID string, in LeaveInput) error {
	if err := s.enabled(); err != nil {
		return err
	}
	if err := s.evaluate(ctx, roomID, false); err != nil {
		return err
	}
	v, err := s.view(ctx, roomID)
	if err != nil {
		return err
	}
	var me *Participant
	others := 0
	for i, p := range v.Participants {
		if p.AccountID == accountID {
			me = &v.Participants[i]
		} else {
			others++
		}
	}
	if me == nil {
		return ErrNotFound
	}
	session, err := s.d.Heartbeats.current(ctx, me.ParticipationID)
	if err != nil {
		return err
	}
	host := v.Head.HostAccountID == accountID
	var next *Participant
	if host && others > 0 && !in.End {
		if in.NextHostID == "" {
			return ErrHostMustChoose
		}
		if next, err = s.eligibleParticipant(ctx, v, in.NextHostID); err != nil {
			return err
		}
	}
	now := s.now()
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		switch {
		case rec.Head.State != voice.StateOpen:
			return ErrNotFound
		case (rec.Head.HostAccountID == accountID) != host:
			return ErrConflict
		}
		ps, err := s.d.Repository.Participants(ctx, tx, roomID)
		if err != nil {
			return err
		}
		others := 0
		for _, p := range ps {
			if p.AccountID != accountID {
				others++
			}
		}
		switch {
		case host && others == 0:
			// The last one out: the room ends empty, whatever the host chose.
		case host && in.End:
			_, err := s.endRoomTx(ctx, tx, roomID, voice.EndHost, accountID, now)
			return err
		case next != nil:
			p, err := s.d.Repository.FindParticipant(ctx, tx, next.AccountID)
			if errors.Is(err, ErrNotFound) || (err == nil && p.RoomID != roomID) {
				return ErrInvalidNextHost
			}
			if err != nil {
				return err
			}
			if err := s.changeHostTx(ctx, tx, roomID, p, voice.HostLeft, now); err != nil {
				return err
			}
			if _, err := s.d.Repository.DeleteHostGrace(ctx, tx, roomID); err != nil {
				return err
			}
		case host:
			// Someone joined since the host chose to leave alone.
			return ErrHostMustChoose
		}
		if _, err := s.endParticipationTx(ctx, tx, *me, voice.LeftRoom, session, now); err != nil {
			return err
		}
		return s.endIfEmptyTx(ctx, tx, roomID, now)
	})
	if err != nil {
		return err
	}
	s.forget(ctx, &previous{Participant: *me, session: session})
	return nil
}

// eligibleParticipant returns the participant accountID of a room, other than its host, when
// it may host the room; ErrInvalidNextHost otherwise.
func (s *Service) eligibleParticipant(ctx context.Context, v RoomView, accountID string) (*Participant, error) {
	for i, p := range v.Participants {
		if p.AccountID != accountID || accountID == v.Head.HostAccountID {
			continue
		}
		ok, err := s.canHost(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInvalidNextHost
		}
		return &v.Participants[i], nil
	}
	return nil, ErrInvalidNextHost
}

// HandOver makes accountID, an eligible participant, the host of a room that byAccountID
// hosts (handed_over, ADR-0100 §5). The former host stays in the room.
func (s *Service) HandOver(ctx context.Context, byAccountID, roomID, accountID string) (RoomView, error) {
	v, err := s.hostRoom(ctx, byAccountID, roomID, true)
	if err != nil {
		return RoomView{}, err
	}
	if _, err := s.eligibleParticipant(ctx, v, accountID); err != nil {
		return RoomView{}, err
	}
	now := s.now()
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		switch {
		case rec.Head.State != voice.StateOpen:
			return ErrNotFound
		case rec.Head.HostAccountID != byAccountID:
			return ErrNotHost
		}
		p, err := s.d.Repository.FindParticipant(ctx, tx, accountID)
		if errors.Is(err, ErrNotFound) || (err == nil && p.RoomID != roomID) {
			return ErrInvalidNextHost
		}
		if err != nil {
			return err
		}
		if err := s.changeHostTx(ctx, tx, roomID, p, voice.HostHandedOver, now); err != nil {
			return err
		}
		_, err = s.d.Repository.DeleteHostGrace(ctx, tx, roomID)
		return err
	})
	if err != nil {
		return RoomView{}, err
	}
	return s.view(ctx, roomID)
}

// Heartbeat renews the session of accountID in a room (ADR-0100 §6, ADR-0101 §6) and then
// evaluates the expiry of the room. A session that is not the current one of the account's
// participation in the room fails with ErrNotFound: the client joins again.
func (s *Service) Heartbeat(ctx context.Context, accountID, roomID, session string) error {
	if err := s.enabled(); err != nil {
		return err
	}
	p, err := s.d.Repository.FindParticipant(ctx, nil, accountID)
	if err != nil {
		return err
	}
	if p.RoomID != roomID {
		return ErrNotFound
	}
	cur, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
	if err != nil {
		return err
	}
	if cur == "" || cur != session {
		return ErrNotFound
	}
	// The expiry is evaluated first: a heartbeat after the lease does not bring back a
	// participation that expired.
	if err := s.evaluate(ctx, roomID, false); err != nil {
		return err
	}
	again, err := s.d.Repository.FindParticipant(ctx, nil, accountID)
	if errors.Is(err, ErrNotFound) || (err == nil && again.ParticipationID != p.ParticipationID) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.d.Heartbeats.beat(ctx, p.ParticipationID, session); err != nil {
		return err
	}
	// A host who came back in the grace leaves it now.
	return s.evaluate(ctx, roomID, false)
}
