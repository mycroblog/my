// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The errors of the scheduled rooms (ADR-0100 §16).
var (
	// ErrInvalidSchedule reports a scheduled room that cannot be opened as asked: no key or a key
	// longer than MaxScheduleKeyLength, a wait outside voice.MinScheduledWait and
	// voice.MaxScheduledWait, more than voice.MaxCohosts co-host candidates, or a start post.
	ErrInvalidSchedule = errors.New("voice: invalid scheduled room")
	// ErrInvalidEndReason reports an end of a scheduled room other than host or moderator.
	ErrInvalidEndReason = errors.New("voice: invalid end reason")
)

// MaxScheduleKeyLength bounds the key of a schedule, in bytes (voice_room_schedules.schedule_key).
const MaxScheduleKeyLength = 200

// ScheduledInvitationBatch is how many invitees one JobInviteScheduled writes (ADR-0100 §16).
const ScheduledInvitationBatch = 500

// JobInviteScheduled writes the invitations that the caller of a scheduled room copies into it
// (ADR-0100 §16): invitations as listeners by the host, without notifications and outside
// InvitationLimit. Its payload names the room and at most ScheduledInvitationBatch accounts; the
// invitations are unique by room and account, so running it again writes nothing twice.
const JobInviteScheduled = "voice.invite_scheduled"

type inviteScheduledPayload struct {
	Room     string   `json:"room"`
	Accounts []string `json:"accounts"`
}

// ScheduledInput is a room that another module opens by its schedule (ADR-0100 §16).
type ScheduledInput struct {
	// HostAccountID is who scheduled it, the host, who is not a participant until they join.
	HostAccountID string
	// Settings are the settings of the room; Settings.Post must be nil, since a scheduled room
	// writes no start post.
	Settings SettingsInput
	// Wait is how long the room waits for its first participant and its host;
	// voice.DefaultScheduledWait when 0.
	Wait time.Duration
	// CohostCandidates are at most voice.MaxCohosts accounts, in the order of their nomination,
	// who become co-hosts as they join.
	CohostCandidates []string
	// Invitees are copied into the invitations of the room by JobInviteScheduled.
	Invitees []string
	// Key is the caller's key of the schedule: opening the same key again returns its room.
	Key string
}

// CheckScheduledHost reports whether accountID may host a scheduled room now: ErrDisabled when
// the voice chat is off and ErrNotEligible when the account may not host (ADR-0100 §2, §16).
// The caller checks it before it keeps a schedule.
func (s *Service) CheckScheduledHost(ctx context.Context, accountID string) error {
	if err := s.enabled(); err != nil {
		return err
	}
	ok, err := s.canHost(ctx, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotEligible
	}
	return nil
}

// OpenScheduled opens the room of a schedule with in.HostAccountID as its host and nobody in it
// (ADR-0100 §16): it waits for in.Wait, during which it does not end empty and its host is in the
// grace of an absent host. It joins nobody, issues no grant, writes no start post, notifies
// nobody (voice_room_started), and leaves the media path to the first join. The same key returns
// the room it opened. It fails as Open does: ErrDisabled, ErrNotEligible, ErrTooManyRooms, and
// ErrHostingElsewhere for a host of another open room (a participation in another room does not
// count), a *voice.SettingsError, and ErrInvalidSchedule.
func (s *Service) OpenScheduled(ctx context.Context, in ScheduledInput) (RoomView, error) {
	if err := s.enabled(); err != nil {
		return RoomView{}, err
	}
	if in.Key == "" || len(in.Key) > MaxScheduleKeyLength || !utf8.ValidString(in.Key) || in.Settings.Post != nil {
		return RoomView{}, ErrInvalidSchedule
	}
	wait := in.Wait
	if wait == 0 {
		wait = voice.DefaultScheduledWait
	}
	if wait < voice.MinScheduledWait || wait > voice.MaxScheduledWait {
		return RoomView{}, ErrInvalidSchedule
	}
	host := in.HostAccountID
	candidates := uniqueIDs(slices.DeleteFunc(slices.Clone(in.CohostCandidates), func(id string) bool { return id == "" || id == host }))
	if len(candidates) > voice.MaxCohosts {
		return RoomView{}, ErrInvalidSchedule
	}
	switch sc, err := s.d.Repository.FindScheduleByKey(ctx, nil, in.Key); {
	case err == nil:
		return s.view(ctx, sc.RoomID)
	case !errors.Is(err, ErrNotFound):
		return RoomView{}, err
	}
	settings, err := s.settings(ctx, in.Settings)
	if err != nil {
		return RoomView{}, err
	}
	ok, err := s.canHost(ctx, host)
	if err != nil {
		return RoomView{}, err
	}
	if !ok {
		return RoomView{}, ErrNotEligible
	}
	now := s.now()
	room := Room{ID: s.d.IDs.NewID(), OpenedByAccountID: host, OpenedAt: now, Transport: s.d.Config.Transport}
	rev := Revision{ID: s.d.IDs.NewID(), RoomID: room.ID, Settings: settings, RevisedByAccountID: host, RevisedAt: now}
	change := HostChange{ID: s.d.IDs.NewID(), RoomID: room.ID, HostAccountID: host, Reason: voice.HostOpened, ChangedAt: now}
	sched := Schedule{RoomID: room.ID, Key: in.Key, WaitEndsAt: now.Add(wait)}
	roomID := room.ID
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		roomID = room.ID
		if err := s.d.Repository.LockOpening(ctx, tx); err != nil {
			return err
		}
		// A job that runs again, or two that race, open the schedule once.
		switch sc, err := s.d.Repository.FindScheduleByKey(ctx, tx, in.Key); {
		case err == nil:
			roomID = sc.RoomID
			return nil
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if err := s.checkOpeningTx(ctx, tx, host); err != nil {
			return err
		}
		if err := s.d.Repository.InsertRoom(ctx, tx, room); err != nil {
			return err
		}
		if err := s.d.Repository.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := s.d.Repository.InsertHostChange(ctx, tx, change); err != nil {
			return err
		}
		if err := tx.Emit(ctx, RoomOpened{Room: room, Revision: rev, Host: change}); err != nil {
			return err
		}
		if err := s.d.Repository.InsertSchedule(ctx, tx, sched); err != nil {
			return err
		}
		// The absent host is in the grace until the end of the wait, as a disconnected host is
		// (ADR-0100 §6, §16).
		if err := s.d.Repository.PutHostGrace(ctx, tx, HostGrace{RoomID: room.ID, EndsAt: sched.WaitEndsAt}); err != nil {
			return err
		}
		if err := s.d.Repository.InsertCohostCandidates(ctx, tx, room.ID, candidates); err != nil {
			return err
		}
		return s.enqueueInviteScheduled(ctx, tx, room.ID, in.Invitees, "open")
	})
	if err != nil {
		return RoomView{}, err
	}
	return s.view(ctx, roomID)
}

// checkOpeningTx checks, under the opening lock, that a room may open for host: fewer rooms than
// the operator allows are open (ErrTooManyRooms) and host hosts none of them
// (ErrHostingElsewhere).
func (s *Service) checkOpeningTx(ctx context.Context, tx uow.Tx, host string) error {
	open, err := s.d.Repository.OpenRooms(ctx, tx, "")
	if err != nil {
		return err
	}
	if limit := s.d.Config.MaxOpenRooms; limit > 0 && len(open) >= limit {
		return ErrTooManyRooms
	}
	if slices.ContainsFunc(open, func(r RoomRecord) bool { return r.Head.HostAccountID == host }) {
		return ErrHostingElsewhere
	}
	return nil
}

// uniqueIDs returns ids without the repeated ones, in the order of their first appearance.
func uniqueIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// AddScheduledInvitees copies more invitees into the room of a schedule, as OpenScheduled copies
// its Invitees (ADR-0100 §16). ErrNotFound for an unknown key; a room that ended takes nobody.
func (s *Service) AddScheduledInvitees(ctx context.Context, key string, accountIDs []string) error {
	sc, err := s.d.Repository.FindScheduleByKey(ctx, nil, key)
	if err != nil {
		return err
	}
	if len(accountIDs) == 0 {
		return nil
	}
	cause := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.enqueueInviteScheduled(ctx, tx, sc.RoomID, accountIDs, cause)
	})
}

// enqueueInviteScheduled enqueues in tx one JobInviteScheduled for each ScheduledInvitationBatch
// of accountIDs; cause tells the calls apart in the idempotency keys.
func (s *Service) enqueueInviteScheduled(ctx context.Context, tx uow.Tx, roomID string, accountIDs []string, cause string) error {
	ids := uniqueIDs(slices.DeleteFunc(slices.Clone(accountIDs), func(id string) bool { return id == "" }))
	for i, batch := range slices.Collect(slices.Chunk(ids, ScheduledInvitationBatch)) {
		b, err := json.Marshal(inviteScheduledPayload{Room: roomID, Accounts: batch})
		if err != nil {
			return err
		}
		if err := s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
			Type: JobInviteScheduled, Payload: b,
			IdempotencyKey: JobInviteScheduled + ":" + roomID + ":" + cause + ":" + strconv.Itoa(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

// InviteScheduled runs JobInviteScheduled.
func (s *Service) InviteScheduled(ctx context.Context, j queue.Job) queue.Result {
	var p inviteScheduledPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("decode %s: %w", JobInviteScheduled, err))
	}
	if err := s.inviteScheduled(ctx, p.Room, p.Accounts); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// inviteScheduled writes the invitations of accountIDs to an open room as listeners, by its host
// now: an account that is not local and active, the host, an account in a block relation with the
// host, and a banned account are left out, as Invite refuses them. An account invited already
// keeps its invitation.
func (s *Service) inviteScheduled(ctx context.Context, roomID string, accountIDs []string) error {
	v, err := s.view(ctx, roomID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	case v.Head.State != voice.StateOpen:
		return nil
	}
	host := v.Head.HostAccountID
	ids := slices.DeleteFunc(slices.Clone(accountIDs), func(id string) bool { return id == host })
	if len(ids) == 0 {
		return nil
	}
	blocked, err := s.d.Relations.BlockedAmong(ctx, host, ids)
	if err != nil {
		return err
	}
	var keep []string
	for _, id := range ids {
		if blocked[id] {
			continue
		}
		m, err := s.d.Members.Member(ctx, id)
		if err != nil {
			return err
		}
		if m.Local && m.Active {
			keep = append(keep, id)
		}
	}
	if len(keep) == 0 {
		return nil
	}
	now := s.now()
	invs := make([]Invitation, len(keep))
	for i, id := range keep {
		invs[i] = Invitation{ID: s.d.IDs.NewID(), RoomID: roomID, InviteeAccountID: id, Role: voice.RoleListener, InvitedAt: now}
	}
	return s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		if rec.Head.State != voice.StateOpen {
			return nil
		}
		facts, err := s.d.Repository.ViewerFacts(ctx, tx, roomID, keep)
		if err != nil {
			return err
		}
		for _, inv := range invs {
			if facts[inv.InviteeAccountID].Banned || inv.InviteeAccountID == rec.Head.HostAccountID {
				continue
			}
			inv.InvitedByAccountID = rec.Head.HostAccountID
			did, err := s.d.Repository.InsertInvitation(ctx, tx, inv)
			if err != nil {
				return err
			}
			if !did {
				continue
			}
			if err := tx.Emit(ctx, Invited{Invitation: inv}); err != nil {
				return err
			}
		}
		return nil
	})
}

// EndScheduledInput ends the room of a schedule that its caller cancelled (ADR-0100 §16).
type EndScheduledInput struct {
	Key string
	// Reason is voice.EndHost or voice.EndModerator.
	Reason voice.EndReason
	// ByAccountID is the account that ended it; ByUserID is the user of a moderator, whom the
	// audit log names.
	ByAccountID string
	ByUserID    string
	// Note is the reason of a moderator, as EndByModerator takes it.
	Note string
}

// EndScheduled ends the room of a schedule: as its host ends it (host), or as a moderator does,
// with the audit log (moderator, EndByModerator). A room that ended already stays as it ended, so
// that a job that runs again ends nothing twice; ErrNotFound for an unknown key, and
// ErrInvalidEndReason for another reason.
func (s *Service) EndScheduled(ctx context.Context, in EndScheduledInput) error {
	if in.Reason != voice.EndHost && in.Reason != voice.EndModerator {
		return ErrInvalidEndReason
	}
	sc, err := s.d.Repository.FindScheduleByKey(ctx, nil, in.Key)
	if err != nil {
		return err
	}
	rec, err := s.d.Repository.FindRoom(ctx, nil, sc.RoomID)
	if err != nil {
		return err
	}
	if rec.Head.State != voice.StateOpen {
		return nil
	}
	if in.Reason == voice.EndModerator {
		err := s.EndByModerator(ctx, ModeratorEndInput{RoomID: sc.RoomID, ByUserID: in.ByUserID, ByAccountID: in.ByAccountID, Note: in.Note})
		if errors.Is(err, ErrNotFound) {
			// The room ended meanwhile.
			return nil
		}
		return err
	}
	ps, err := s.d.Repository.Participants(ctx, nil, sc.RoomID)
	if err != nil {
		return err
	}
	err = s.roomTx(ctx, []string{sc.RoomID}, func(tx uow.Tx, rec RoomRecord) error {
		if rec.Head.State != voice.StateOpen {
			return nil
		}
		_, err := s.endRoomTx(ctx, tx, sc.RoomID, voice.EndHost, in.ByAccountID, s.now())
		return err
	})
	if err != nil {
		return err
	}
	s.forgetAll(ctx, ps)
	return nil
}

// waiting reports whether the open room of rec waits for its absent host (ADR-0100 §16): a room
// opened by a schedule whose host is in no participation of it and in the grace that the opening
// started. It returns whether the wait ran out by now.
func (s *Service) waiting(ctx context.Context, tx uow.Tx, rec RoomRecord, ps []Participant, now time.Time) (wait, over bool, err error) {
	host := rec.Head.HostAccountID
	if slices.ContainsFunc(ps, func(p Participant) bool { return p.AccountID == host }) {
		return false, false, nil
	}
	g, err := s.d.Repository.FindHostGrace(ctx, tx, rec.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		return false, false, nil
	case err != nil:
		return false, false, err
	}
	switch _, err := s.d.Repository.FindSchedule(ctx, tx, rec.ID); {
	case errors.Is(err, ErrNotFound):
		return false, false, nil
	case err != nil:
		return false, false, err
	}
	return true, !now.Before(g.EndsAt), nil
}

// waitTx applies an evaluation to a room that waits for its absent host, in tx (ADR-0100 §16).
// Until the end of the wait, the room keeps its host and ends only when everybody who came left
// (empty). At the end, the earliest eligible co-host in it takes over (host_absent); otherwise
// the room ends: host_absent with participants, empty when everybody who came left, and
// unattended when nobody came.
func (s *Service) waitTx(ctx context.Context, tx uow.Tx, roomID string, e evaluation, now time.Time) error {
	ps, err := s.d.Repository.Participants(ctx, tx, roomID)
	if err != nil {
		return err
	}
	if !e.waitOver {
		if len(ps) > 0 {
			return nil
		}
		attended, err := s.d.Repository.Attended(ctx, tx, roomID)
		if err != nil || !attended {
			return err
		}
		_, err = s.endRoomTx(ctx, tx, roomID, voice.EndEmpty, "", now)
		return err
	}
	did, err := s.d.Repository.DeleteHostGrace(ctx, tx, roomID)
	if err != nil || !did {
		return err
	}
	if next, ok := successorTx(ps, e.host, e.eligible); ok {
		return s.changeHostTx(ctx, tx, roomID, next, voice.HostAbsent, now)
	}
	reason := voice.EndHostAbsent
	if len(ps) == 0 {
		attended, err := s.d.Repository.Attended(ctx, tx, roomID)
		if err != nil {
			return err
		}
		reason = voice.EndUnattended
		if attended {
			reason = voice.EndEmpty
		}
	}
	_, err = s.endRoomTx(ctx, tx, roomID, reason, "", now)
	return err
}

// scheduledJoinTx reports, in the transaction of a join before it records the participation,
// whether the room was opened by a schedule and whether the join is its first.
func (s *Service) scheduledJoinTx(ctx context.Context, tx uow.Tx, roomID string) (scheduled, first bool, err error) {
	switch _, err := s.d.Repository.FindSchedule(ctx, tx, roomID); {
	case errors.Is(err, ErrNotFound):
		return false, false, nil
	case err != nil:
		return false, false, err
	}
	attended, err := s.d.Repository.Attended(ctx, tx, roomID)
	return true, !attended, err
}

// joinScheduledTx applies, in the transaction of a join, what joining a scheduled room does
// (ADR-0100 §16): the first join opens the media path (voice.apply, the same key as Open's), the
// host who comes ends the wait, and a co-host candidate becomes a co-host while fewer than
// voice.MaxCohosts are, unless the account was removed as a co-host of the room once.
func (s *Service) joinScheduledTx(ctx context.Context, tx uow.Tx, rec RoomRecord, part Participation, first bool) error {
	if first {
		if err := s.enqueueApply(ctx, tx, applyPayload{Op: applyOpen, Room: rec.ID}, "open"); err != nil {
			return err
		}
	}
	if part.AccountID == rec.Head.HostAccountID {
		_, err := s.d.Repository.DeleteHostGrace(ctx, tx, rec.ID)
		return err
	}
	c, err := s.d.Repository.FindCohostCandidate(ctx, tx, rec.ID, part.AccountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	case c.Removed:
		return nil
	}
	ps, err := s.d.Repository.Participants(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	cohosts := 0
	for _, p := range ps {
		if p.Cohost() && p.AccountID != rec.Head.HostAccountID {
			cohosts++
		}
	}
	if cohosts >= voice.MaxCohosts {
		return nil
	}
	cc := CohostChange{ID: s.d.IDs.NewID(), RoomID: rec.ID, AccountID: part.AccountID, Change: voice.CohostAssigned, ChangedAt: part.JoinedAt}
	if err := s.d.Repository.InsertCohostChange(ctx, tx, cc); err != nil {
		return err
	}
	return tx.Emit(ctx, CohostChanged{Change: cc})
}
