// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The writes of the lifecycle of a room. Every one is conditional: a participation ends once,
// a room ends once, and the grace of a host ends once, so that the evaluations that race (the
// requests of the room and voice.sweep) apply each change once (ADR-0100 §6, ADR-0004). Only
// the evaluation that wrote a change emits its event and enqueues its JobApply.

// endParticipationTx ends the participation of p for reason and reports whether it did. The
// session, when known, is removed from the media path after the commit.
func (s *Service) endParticipationTx(ctx context.Context, tx uow.Tx, p Participant, reason voice.ParticipationEndReason, session string, at time.Time) (bool, error) {
	end := ParticipationEnd{ParticipationID: p.ParticipationID, Reason: reason, EndedAt: at}
	did, err := s.d.Repository.InsertParticipationEnd(ctx, tx, end)
	if err != nil || !did {
		return false, err
	}
	part := Participation{ID: p.ParticipationID, RoomID: p.RoomID, AccountID: p.AccountID}
	if err := tx.Emit(ctx, ParticipationEnded{Participation: part, End: end}); err != nil {
		return false, err
	}
	if session == "" {
		return true, nil
	}
	return true, s.enqueueApply(ctx, tx, applyPayload{Op: applyRemove, Room: p.RoomID, Session: session, Account: p.AccountID}, "end:"+p.ParticipationID)
}

// endRoomTx ends a room for reason: every participation ends with room_ended, the room ends,
// its host grace goes, and its media path closes after the commit. It reports whether it
// ended the room; a room that ended already stays as it ended.
func (s *Service) endRoomTx(ctx context.Context, tx uow.Tx, roomID string, reason voice.EndReason, by string, at time.Time) (bool, error) {
	ps, err := s.d.Repository.Participants(ctx, tx, roomID)
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		if _, err := s.endParticipationTx(ctx, tx, p, voice.RoomEnded, "", at); err != nil {
			return false, err
		}
	}
	ending := Ending{RoomID: roomID, Reason: reason, EndedByAccountID: by, EndedAt: at}
	did, err := s.d.Repository.InsertEnding(ctx, tx, ending)
	if err != nil || !did {
		return false, err
	}
	if _, err := s.d.Repository.DeleteHostGrace(ctx, tx, roomID); err != nil {
		return false, err
	}
	if err := tx.Emit(ctx, RoomEnded{Ending: ending}); err != nil {
		return false, err
	}
	return true, s.enqueueApply(ctx, tx, applyPayload{Op: applyClose, Room: roomID}, "close")
}

// endIfEmptyTx ends a room that nobody is left in (empty).
func (s *Service) endIfEmptyTx(ctx context.Context, tx uow.Tx, roomID string, at time.Time) error {
	ps, err := s.d.Repository.Participants(ctx, tx, roomID)
	if err != nil || len(ps) > 0 {
		return err
	}
	_, err = s.endRoomTx(ctx, tx, roomID, voice.EndEmpty, "", at)
	return err
}

// changeHostTx makes accountID the host of a room for reason. A co-host who becomes the host
// is no longer a co-host.
func (s *Service) changeHostTx(ctx context.Context, tx uow.Tx, roomID string, next Participant, reason voice.HostChangeReason, at time.Time) error {
	c := HostChange{ID: s.d.IDs.NewID(), RoomID: roomID, HostAccountID: next.AccountID, Reason: reason, ChangedAt: at}
	if err := s.d.Repository.InsertHostChange(ctx, tx, c); err != nil {
		return err
	}
	if err := tx.Emit(ctx, HostChanged{Change: c}); err != nil {
		return err
	}
	if !next.Cohost() {
		return nil
	}
	cc := CohostChange{ID: s.d.IDs.NewID(), RoomID: roomID, AccountID: next.AccountID, Change: voice.CohostRemoved, ChangedAt: at}
	if err := s.d.Repository.InsertCohostChange(ctx, tx, cc); err != nil {
		return err
	}
	return tx.Emit(ctx, CohostChanged{Change: cc})
}

// successorTx returns the co-host who takes over a room from its host: the eligible one among
// eligible, read before the transaction, who was nominated first (voice.NextHost) and is still
// in the room.
func successorTx(ps []Participant, host string, eligible map[string]bool) (Participant, bool) {
	cohosts := make([]Participant, 0, len(ps))
	for _, p := range ps {
		if p.Cohost() && p.AccountID != host {
			cohosts = append(cohosts, p)
		}
	}
	slices.SortFunc(cohosts, func(a, b Participant) int {
		switch {
		case a.CohostChangeID < b.CohostChangeID:
			return -1
		case a.CohostChangeID > b.CohostChangeID:
			return 1
		}
		return 0
	})
	cands := make([]voice.Candidate, len(cohosts))
	for i, p := range cohosts {
		cands[i] = voice.Candidate{AccountID: p.AccountID, Eligible: eligible[p.AccountID]}
	}
	id, ok := voice.NextHost(cands)
	if !ok {
		return Participant{}, false
	}
	i := slices.IndexFunc(cohosts, func(p Participant) bool { return p.AccountID == id })
	return cohosts[i], true
}

// eligibleCohosts reads which co-hosts of a room may host it, before a transaction.
func (s *Service) eligibleCohosts(ctx context.Context, ps []Participant, host string, skip func(Participant) bool) (map[string]bool, error) {
	out := map[string]bool{}
	for _, p := range ps {
		if !p.Cohost() || p.AccountID == host || (skip != nil && skip(p)) {
			continue
		}
		ok, err := s.d.Repository.Banned(ctx, nil, p.RoomID, p.AccountID)
		if err != nil {
			return nil, err
		}
		if ok {
			continue
		}
		if out[p.AccountID], err = s.canHost(ctx, p.AccountID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// evaluation is what an evaluation of a room changes, decided before its transaction.
type evaluation struct {
	sweep bool
	// endReason ends the room (time_limit, disabled), "" otherwise.
	endReason voice.EndReason
	live      liveness
	eligible  map[string]bool
	// host is the host that the evaluation read before its transaction.
	host string
}

// roomTx runs fn in a transaction that first locks the rooms roomIDs, in the order of their IDs
// (LockRoom), and then reads the first of them, so that every command and evaluation of a room
// decides on what the others committed and applies once (ADR-0100 §6).
func (s *Service) roomTx(ctx context.Context, roomIDs []string, fn func(tx uow.Tx, rec RoomRecord) error) error {
	ids := slices.Clone(roomIDs)
	ids = slices.DeleteFunc(ids, func(id string) bool { return id == "" })
	slices.Sort(ids)
	ids = slices.Compact(ids)
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, id := range ids {
			if err := s.d.Repository.LockRoom(ctx, tx, id); err != nil {
				return err
			}
		}
		rec, err := s.d.Repository.FindRoom(ctx, tx, roomIDs[0])
		if err != nil {
			return err
		}
		return fn(tx, rec)
	})
}

// evaluate evaluates the expiry of a room (ADR-0100 §6): the participants whose heartbeat
// expired leave (disconnected); a host whose heartbeat expired enters the grace, and past it
// the earliest eligible co-host takes over, or the room ends (host_disconnected); a host who
// came back leaves the grace; and a room that nobody is left in ends (empty). voice.sweep also
// ends the rooms of a voice chat that is off or runs another method (disabled) and those open
// longer than the operator allows (time_limit). The heartbeats are read before the
// transaction, which opens only when something changes.
func (s *Service) evaluate(ctx context.Context, roomID string, sweep bool) error {
	rec, err := s.d.Repository.FindRoom(ctx, nil, roomID)
	if err != nil || rec.Head.State != voice.StateOpen {
		return err
	}
	now := s.now()
	e := evaluation{sweep: sweep, host: rec.Head.HostAccountID}
	if sweep {
		cfg := s.d.Config
		switch {
		case !s.Enabled() || rec.Transport != cfg.Transport:
			e.endReason = voice.EndDisabled
		case cfg.MaxRoomDuration > 0 && !now.Before(rec.OpenedAt.Add(cfg.MaxRoomDuration)):
			e.endReason = voice.EndTimeLimit
		}
	}
	ps, err := s.d.Repository.Participants(ctx, nil, roomID)
	if err != nil {
		return err
	}
	if e.endReason == "" {
		if e.live, err = s.d.Heartbeats.read(ctx, ps); err != nil {
			return err
		}
		host := rec.Head.HostAccountID
		// A host who is in no participation of the room hands it over at once, as after the
		// grace.
		hostExpired := !slices.ContainsFunc(ps, func(p Participant) bool { return p.AccountID == host })
		changes := len(ps) == 0 || hostExpired
		for _, p := range ps {
			if !e.live.expired(p.ParticipationID) {
				continue
			}
			changes = true
			hostExpired = hostExpired || p.AccountID == host
		}
		switch _, err := s.d.Repository.FindHostGrace(ctx, nil, roomID); {
		case err == nil:
			changes = true // a grace that runs out, or a host who came back
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if !changes {
			return nil
		}
		if hostExpired {
			if e.eligible, err = s.eligibleCohosts(ctx, ps, host, func(p Participant) bool { return e.live.expired(p.ParticipationID) }); err != nil {
				return err
			}
		}
	}
	return s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error { return s.evaluateTx(ctx, tx, rec, e, now) })
}

// evaluateTx applies an evaluation to a room locked in tx. A room whose host changed since the
// evaluation read it is left to the next evaluation, which reads the new host's co-hosts.
func (s *Service) evaluateTx(ctx context.Context, tx uow.Tx, rec RoomRecord, e evaluation, now time.Time) error {
	roomID := rec.ID
	if rec.Head.State != voice.StateOpen || (e.endReason == "" && rec.Head.HostAccountID != e.host) {
		return nil
	}
	if e.endReason != "" {
		_, err := s.endRoomTx(ctx, tx, roomID, e.endReason, "", now)
		return err
	}
	ps, err := s.d.Repository.Participants(ctx, tx, roomID)
	if err != nil {
		return err
	}
	host := rec.Head.HostAccountID
	var hostP *Participant
	for i, p := range ps {
		if p.AccountID == host {
			hostP = &ps[i]
			continue
		}
		if e.live.expired(p.ParticipationID) {
			if _, err := s.endParticipationTx(ctx, tx, p, voice.Disconnected, e.live.session[p.ParticipationID], now); err != nil {
				return err
			}
		}
	}
	switch {
	case hostP == nil:
		// The host is in no participation of the room: the room goes on to the next host
		// at once, as after the grace.
		if err := s.succeedTx(ctx, tx, roomID, host, e, now); err != nil {
			return err
		}
	case e.live.expired(hostP.ParticipationID):
		if err := s.graceTx(ctx, tx, roomID, *hostP, e, now); err != nil {
			return err
		}
	default:
		if _, err := s.d.Repository.DeleteHostGrace(ctx, tx, roomID); err != nil {
			return err
		}
	}
	return s.endIfEmptyTx(ctx, tx, roomID, now)
}

// graceTx starts the grace of a host whose heartbeat expired, or ends it once it ran out: the
// host leaves (disconnected) for the next host, or the room ends (host_disconnected).
func (s *Service) graceTx(ctx context.Context, tx uow.Tx, roomID string, host Participant, e evaluation, now time.Time) error {
	g, err := s.d.Repository.FindHostGrace(ctx, tx, roomID)
	switch {
	case errors.Is(err, ErrNotFound):
		return s.d.Repository.PutHostGrace(ctx, tx, HostGrace{RoomID: roomID, EndsAt: now.Add(HostGracePeriod)})
	case err != nil:
		return err
	case now.Before(g.EndsAt):
		return nil
	}
	did, err := s.d.Repository.DeleteHostGrace(ctx, tx, roomID)
	if err != nil || !did {
		return err
	}
	if err := s.succeedTx(ctx, tx, roomID, host.AccountID, e, now); err != nil {
		return err
	}
	_, err = s.endParticipationTx(ctx, tx, host, voice.Disconnected, e.live.session[host.ParticipationID], now)
	return err
}

// succeedTx hands a room whose host disconnected to the earliest eligible co-host, or ends it
// (host_disconnected).
func (s *Service) succeedTx(ctx context.Context, tx uow.Tx, roomID, host string, e evaluation, now time.Time) error {
	ps, err := s.d.Repository.Participants(ctx, tx, roomID)
	if err != nil {
		return err
	}
	if next, ok := successorTx(ps, host, e.eligible); ok {
		return s.changeHostTx(ctx, tx, roomID, next, voice.HostDisconnected, now)
	}
	_, err = s.endRoomTx(ctx, tx, roomID, voice.EndHostDisconnected, "", now)
	return err
}

// hostIneligible moves a room whose host lost the eligibility (ADR-0100 §2) to the earliest
// eligible co-host, or ends it (host_ineligible). A host who is suspended or deleted also
// leaves (suspended, §11); otherwise the former host stays in the room.
func (s *Service) hostIneligible(ctx context.Context, roomID, host string, suspended bool) error {
	ps, err := s.d.Repository.Participants(ctx, nil, roomID)
	if err != nil {
		return err
	}
	eligible, err := s.eligibleCohosts(ctx, ps, host, nil)
	if err != nil {
		return err
	}
	sessions := map[string]string{}
	for _, p := range ps {
		if p.AccountID == host {
			if sessions[p.ParticipationID], err = s.d.Heartbeats.current(ctx, p.ParticipationID); err != nil {
				return err
			}
		}
	}
	now := s.now()
	return s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		switch {
		case rec.Head.State != voice.StateOpen:
			return nil
		case rec.Head.HostAccountID != host:
			// Another command changed the host since the read: decide again on the new one.
			return ErrConflict
		}
		ps, err := s.d.Repository.Participants(ctx, tx, roomID)
		if err != nil {
			return err
		}
		next, ok := successorTx(ps, host, eligible)
		if !ok {
			_, err := s.endRoomTx(ctx, tx, roomID, voice.EndHostIneligible, "", now)
			return err
		}
		if err := s.changeHostTx(ctx, tx, roomID, next, voice.HostIneligible, now); err != nil {
			return err
		}
		if _, err := s.d.Repository.DeleteHostGrace(ctx, tx, roomID); err != nil {
			return err
		}
		if !suspended {
			return nil
		}
		for _, p := range ps {
			if p.AccountID == host {
				if _, err := s.endParticipationTx(ctx, tx, p, voice.SuspendedAccount, sessions[p.ParticipationID], now); err != nil {
					return err
				}
			}
		}
		return s.endIfEmptyTx(ctx, tx, roomID, now)
	})
}
