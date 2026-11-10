// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The voice chat rooms of the events (ADR-0161 §10, ADR-0100 §16). The start of each date of an
// event with a voice place opens, through Rooms, a room that the organizer hosts and that waits
// for its first participant: its settings come from the event, its co-host candidates are the
// co-organizers in the order of their nominations, and its invitations are copied from the
// attendees who answer going or maybe and from the invitees who have not answered. The room writes
// no start post and notifies nobody; event_started is the one notification of the start. A room
// that cannot open leaves the event as it is: the organizer gets event_room_unavailable, and the
// event shows the reason to everyone who sees its voice place. The same checks run when the date
// is confirmed or changed and one day before the start, so that the organizer can fix in time
// what would keep the room shut. Cancelling the event, or removing its voice place, ends its room.

// RoomOpening is the room that the start of a date of an event opens.
type RoomOpening struct {
	// Key is the schedule key of the room (RoomKey), the same for every run of the job.
	Key           string
	HostAccountID string
	// Title is the name of the room.
	Title string
	// Visibility is that of the event, which the room maps to its own (public is local).
	Visibility gathering.Visibility
	Speakers   gathering.Speakers
	// CohostCandidates are the co-organizers in the order of their nominations.
	CohostCandidates []string
	// Invitees are copied into the invitations of the room, without notifying them.
	Invitees []string
}

// RoomEnd ends the room of a schedule key.
type RoomEnd struct {
	Key string
	// Moderator ends it as a moderator does, with the audit log; otherwise as its host does.
	Moderator   bool
	ByAccountID string
	// Note is the reason of a moderator, "" for none.
	Note string
}

// Rooms opens and ends the voice chat rooms of the events through the voice module; the
// composition root implements it.
type Rooms interface {
	// CheckHost reports why accountID could not host a room now: gathering.RoomDisabled,
	// gathering.RoomNotEligible, or "" when it could.
	CheckHost(ctx context.Context, accountID string) (gathering.RoomReason, error)
	// Open opens the room of in, or returns the one that its key opened; reason is why it could
	// not open (RoomDisabled, RoomNotEligible, RoomTooManyRooms, RoomHostingElsewhere), "" when it
	// opened.
	Open(ctx context.Context, in RoomOpening) (roomID string, reason gathering.RoomReason, err error)
	// End ends the room of in.Key; a room that ended already, or a key that opened none, is left.
	End(ctx context.Context, in RoomEnd) error
}

// VoiceRoom is the room that the start of a date of an event opened (gathering_voice_rooms).
type VoiceRoom struct {
	GatheringID    string
	ConfirmationID string
	RoomID         string
	OpenedAt       time.Time
}

// RoomFailure is the room of a date of an event that could not open at its start
// (gathering_voice_room_failures).
type RoomFailure struct {
	GatheringID    string
	ConfirmationID string
	Reason         gathering.RoomReason
	FailedAt       time.Time
}

// RoomRepository is the persistence of the rooms of the events.
type RoomRepository interface {
	// InsertVoiceRoom records the room of a date and reports whether it did: a date keeps its first
	// room.
	InsertVoiceRoom(ctx context.Context, tx uow.Tx, r VoiceRoom) (bool, error)
	// InsertRoomFailure records that the room of a date could not open and reports whether it did:
	// a date keeps its first failure.
	InsertRoomFailure(ctx context.Context, tx uow.Tx, f RoomFailure) (bool, error)
	// VoiceRooms returns the rooms of the dates of gatheringIDs.
	VoiceRooms(ctx context.Context, tx uow.Tx, gatheringIDs []string) ([]VoiceRoom, error)
	// RoomFailures returns the failures of the rooms of the dates of gatheringIDs.
	RoomFailures(ctx context.Context, tx uow.Tx, gatheringIDs []string) ([]RoomFailure, error)
	// FindVoiceRoomByRoom returns the date that opened the room roomID; ErrNotFound for a room that
	// no event opened.
	FindVoiceRoomByRoom(ctx context.Context, tx uow.Tx, roomID string) (VoiceRoom, error)
}

// RoomState is the room of the current date of an event: the room that opened, or why it could
// not open. The zero RoomState is a room that has not opened yet.
type RoomState struct {
	RoomID string
	// Unavailable is why the room could not open at the start, "" otherwise.
	Unavailable gathering.RoomReason
}

// RoomKey returns the schedule key of the room of the date confirmationID of the event id.
func RoomKey(id, confirmationID string) string { return "gathering:" + id + ":" + confirmationID }

// HasVoice reports whether the current settings of the event have a voice place.
func (r Record) HasVoice() bool {
	_, ok := r.Revision.Settings.Place(gathering.PlaceVoice)
	return ok
}

// JobRoom opens the room of a date of an event at its start (checkpoint start), or checks that
// it will open (confirmed, day_before). Its payload names the event, the date, and the
// checkpoint; its idempotency key is the date and the checkpoint.
const JobRoom = "gathering.room"

// JobEndRooms ends the rooms of an event that was cancelled or whose voice place was removed.
const JobEndRooms = "gathering.end_rooms"

// roomPayload is the payload of JobRoom.
type roomPayload struct {
	Gathering  string                   `json:"gathering"`
	Ref        string                   `json:"ref"`
	Checkpoint gathering.RoomCheckpoint `json:"checkpoint"`
}

// endRoomsPayload is the payload of JobEndRooms; Cause tells its jobs apart.
type endRoomsPayload struct {
	Gathering string `json:"gathering"`
	Moderator bool   `json:"moderator,omitempty"`
	By        string `json:"by,omitempty"`
	Note      string `json:"note,omitempty"`
	Cause     string `json:"cause"`
}

// enqueueRoom enqueues a JobRoom for each of ps in tx.
func (s *Service) enqueueRoom(ctx context.Context, tx uow.Tx, ps ...roomPayload) error {
	if s.d.Outbox == nil || len(ps) == 0 {
		return nil
	}
	jobs := make([]queue.NewJob, 0, len(ps))
	for _, p := range ps {
		b, err := json.Marshal(p)
		if err != nil {
			return err
		}
		jobs = append(jobs, queue.NewJob{Type: JobRoom, Payload: b, IdempotencyKey: JobRoom + ":" + p.Ref + ":" + string(p.Checkpoint)})
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, jobs...)
}

// enqueueEndRooms enqueues the JobEndRooms of p in tx.
func (s *Service) enqueueEndRooms(ctx context.Context, tx uow.Tx, p endRoomsPayload) error {
	if s.d.Outbox == nil {
		return nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobEndRooms, Payload: b, IdempotencyKey: JobEndRooms + ":" + p.Gathering + ":" + p.Cause})
}

// enqueueRoomUnavailable enqueues the event_room_unavailable of the room of the date ref of rec,
// to its organizer.
func (s *Service) enqueueRoomUnavailable(ctx context.Context, tx uow.Tx, rec Record, ref string, reason gathering.RoomReason, at gathering.RoomCheckpoint, now time.Time) error {
	return s.enqueueNotify(ctx, tx, notifyPayload{
		Kind: appnotification.EventRoomUnavailable, Gathering: rec.ID, Ref: ref, To: rec.OrganizerAccountID,
		Reason: reason, Checkpoint: at, At: now,
	})
}

// RegisterRoomProjectors registers the projectors of the rooms: a confirmed or changed date of an
// event with a voice place checks that its room will open, and a cancellation, or a revision that
// removes the voice place, ends the rooms of the event.
func (s *Service) RegisterRoomProjectors(r *uow.Projectors) {
	check := func(ctx context.Context, tx uow.Tx, c Confirmation) error {
		rec, err := s.d.Repository.FindGathering(ctx, tx, c.GatheringID)
		if err != nil || !rec.HasVoice() {
			return err
		}
		return s.enqueueRoom(ctx, tx, roomPayload{Gathering: c.GatheringID, Ref: c.ID, Checkpoint: gathering.CheckpointConfirmed})
	}
	r.Register(EventScheduleConfirmed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return check(ctx, tx, e.(ScheduleConfirmed).Confirmation)
	})
	r.Register(EventRescheduled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return check(ctx, tx, e.(Rescheduled).Confirmation)
	})
	r.Register(EventRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(Revised)
		_, had := ev.Previous.Settings.Place(gathering.PlaceVoice)
		_, has := ev.Revision.Settings.Place(gathering.PlaceVoice)
		if !had || has {
			return nil
		}
		// The event stays; only its room goes.
		return s.enqueueEndRooms(ctx, tx, endRoomsPayload{Gathering: ev.Revision.GatheringID, By: ev.Revision.RevisedByAccountID, Cause: ev.Revision.ID})
	})
	r.Register(EventCancelled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(Cancelled).Cancellation
		p := endRoomsPayload{Gathering: c.GatheringID, By: c.CancelledByAccountID, Cause: "cancelled"}
		if c.Reason == gathering.CancelModerator {
			p.Moderator, p.Note = true, c.Note
		}
		return s.enqueueEndRooms(ctx, tx, p)
	})
}

// RegisterRoomJobs registers the handlers of JobRoom and JobEndRooms on reg.
func (s *Service) RegisterRoomJobs(reg *queue.JobHandlerRegistry) {
	reg.Register(JobRoom, func(ctx context.Context, j queue.Job) queue.Result {
		var p roomPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return queue.Fail(fmt.Errorf("decode %s: %w", JobRoom, err))
		}
		if err := s.room(ctx, p); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register(JobEndRooms, func(ctx context.Context, j queue.Job) queue.Result {
		var p endRoomsPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return queue.Fail(fmt.Errorf("decode %s: %w", JobEndRooms, err))
		}
		if err := s.endRooms(ctx, p); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
}

// roomCurrent reports whether the room of the date ref of rec is still to open: the event was
// not cancelled, the date is its current one, and it has a voice place.
func roomCurrent(rec Record, ref string) bool {
	return rec.Cancellation == nil && rec.Confirmation.ID == ref && rec.HasVoice()
}

// room runs JobRoom.
func (s *Service) room(ctx context.Context, p roomPayload) error {
	rec, err := s.d.Repository.FindGathering(ctx, nil, p.Gathering)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil || !roomCurrent(rec, p.Ref) {
		return err
	}
	now := s.now()
	if p.Checkpoint != gathering.CheckpointStart {
		if !now.Before(rec.Confirmation.Schedule.StartsAt) {
			return nil // the start checks it again as it opens the room
		}
		reason := gathering.RoomDisabled
		if s.d.Rooms != nil {
			if reason, err = s.d.Rooms.CheckHost(ctx, rec.OrganizerAccountID); err != nil {
				return err
			}
		}
		if reason == "" {
			return nil
		}
		return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return s.enqueueRoomUnavailable(ctx, tx, rec, p.Ref, reason, p.Checkpoint, now)
		})
	}
	state, err := s.roomStates(ctx, []Record{rec})
	if err != nil {
		return err
	}
	if st, ok := state[rec.ID]; ok && (st.RoomID != "" || st.Unavailable != "") {
		return nil // opened, or failed, by an earlier run
	}
	roomID, reason := "", gathering.RoomDisabled
	if s.d.Rooms != nil {
		in, err := s.opening(ctx, rec)
		if err != nil {
			return err
		}
		if roomID, reason, err = s.d.Rooms.Open(ctx, in); err != nil {
			return err
		}
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockGathering(ctx, tx, rec.ID); err != nil {
			return err
		}
		cur, err := s.d.Repository.FindGathering(ctx, tx, rec.ID)
		if err != nil {
			return err
		}
		if reason != "" {
			ok, err := s.d.Repository.InsertRoomFailure(ctx, tx, RoomFailure{GatheringID: rec.ID, ConfirmationID: p.Ref, Reason: reason, FailedAt: now})
			if err != nil || !ok {
				return err
			}
			return s.enqueueRoomUnavailable(ctx, tx, cur, p.Ref, reason, gathering.CheckpointStart, now)
		}
		if _, err := s.d.Repository.InsertVoiceRoom(ctx, tx, VoiceRoom{GatheringID: rec.ID, ConfirmationID: p.Ref, RoomID: roomID, OpenedAt: now}); err != nil {
			return err
		}
		if roomCurrent(cur, p.Ref) {
			return nil
		}
		// Cancelled, or the voice place removed, while the room opened: the job of that change
		// may have run before the room was recorded, so the room ends now.
		end := endRoomsPayload{Gathering: rec.ID, By: rec.OrganizerAccountID, Cause: "late:" + roomID}
		if c := cur.Cancellation; c != nil && c.Reason == gathering.CancelModerator {
			end.Moderator, end.By, end.Note = true, c.CancelledByAccountID, c.Note
		}
		return s.enqueueEndRooms(ctx, tx, end)
	})
}

// opening returns the room that the start of the current date of rec opens, with its invitees:
// the co-organizers, the attendees who answer going or maybe, and the invitees who have not
// answered.
func (s *Service) opening(ctx context.Context, rec Record) (RoomOpening, error) {
	place, _ := rec.Revision.Settings.Place(gathering.PlaceVoice)
	in := RoomOpening{
		Key: RoomKey(rec.ID, rec.Confirmation.ID), HostAccountID: rec.OrganizerAccountID, Title: rec.Revision.Settings.Title,
		Visibility: rec.Revision.Settings.Visibility, Speakers: place.Speakers,
	}
	for _, c := range rec.Coorganizers {
		in.CohostCandidates = append(in.CohostCandidates, c.AccountID)
	}
	// The co-organizers are invited too, so that they see a room of any visibility and can become
	// its co-hosts as they join.
	in.Invitees = append(in.Invitees, in.CohostCandidates...)
	after := ""
	for {
		ids, err := s.d.Repository.Audience(ctx, nil, rec.ID, true, after, RecipientsPerJob)
		if err != nil {
			return RoomOpening{}, err
		}
		facts, err := s.d.Repository.AudienceFacts(ctx, nil, rec.ID, ids)
		if err != nil {
			return RoomOpening{}, err
		}
		for _, id := range ids {
			if facts[id].Answer != gathering.AnswerDeclined && id != rec.OrganizerAccountID {
				in.Invitees = append(in.Invitees, id)
			}
		}
		if len(ids) < RecipientsPerJob {
			return in, nil
		}
		after = ids[len(ids)-1]
	}
}

// endRooms runs JobEndRooms: it ends every room that the event opened.
func (s *Service) endRooms(ctx context.Context, p endRoomsPayload) error {
	if s.d.Rooms == nil {
		return nil
	}
	rooms, err := s.d.Repository.VoiceRooms(ctx, nil, []string{p.Gathering})
	if err != nil {
		return err
	}
	for _, r := range rooms {
		if err := s.d.Rooms.End(ctx, RoomEnd{Key: RoomKey(r.GatheringID, r.ConfirmationID), Moderator: p.Moderator, ByAccountID: p.By, Note: p.Note}); err != nil {
			return err
		}
	}
	return nil
}

// RoomEnded is what the composition root calls, in the transaction of the end of a voice chat
// room, with the room and whether it ended unattended (nobody came before its wait ended): the
// organizer of the event that opened it, if any, gets event_room_unavailable (ADR-0161 §10).
func (s *Service) RoomEnded(ctx context.Context, tx uow.Tx, roomID string, unattended bool) error {
	if !unattended {
		return nil
	}
	vr, err := s.d.Repository.FindVoiceRoomByRoom(ctx, tx, roomID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	rec, err := s.d.Repository.FindGathering(ctx, tx, vr.GatheringID)
	if err != nil || !roomCurrent(rec, vr.ConfirmationID) {
		return err
	}
	return s.enqueueRoomUnavailable(ctx, tx, rec, vr.ConfirmationID, gathering.RoomUnattended, gathering.CheckpointEnded, s.now())
}

// roomStates returns the rooms of the current dates of those of recs with a voice place, by event.
func (s *Service) roomStates(ctx context.Context, recs []Record) (map[string]RoomState, error) {
	out := map[string]RoomState{}
	var ids []string
	current := map[string]string{}
	for _, r := range recs {
		if r.HasVoice() && r.Scheduled() {
			ids = append(ids, r.ID)
			current[r.ID] = r.Confirmation.ID
			out[r.ID] = RoomState{}
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rooms, err := s.d.Repository.VoiceRooms(ctx, nil, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rooms {
		if current[r.GatheringID] == r.ConfirmationID {
			out[r.GatheringID] = RoomState{RoomID: r.RoomID}
		}
	}
	failures, err := s.d.Repository.RoomFailures(ctx, nil, ids)
	if err != nil {
		return nil, err
	}
	for _, f := range failures {
		if current[f.GatheringID] == f.ConfirmationID && out[f.GatheringID].RoomID == "" {
			out[f.GatheringID] = RoomState{Unavailable: f.Reason}
		}
	}
	return out, nil
}
