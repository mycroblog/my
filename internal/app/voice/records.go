// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The moderation of the rooms and the retention of their records (ADR-0100 §11, §13): the
// target of a report of a room, the end of a room by a moderator with its audit log entry, the
// records that the moderators read from a report, the hourly JobPurgeRecords that erases the
// records of the participants after retention_voice_participants, and the erasure of the names
// and descriptions of the revisions that a deleted account wrote.

const (
	// JobPurgeRecords erases the records of the participants of the rooms that ended longer
	// than retention_voice_participants ago, except the rooms that a report points at and the
	// records that name an account a retention hold preserves (ADR-0100 §13); no payload.
	JobPurgeRecords = "voice.purge_records"
	// RecordPurgeInterval is the period of the purge's schedule.
	RecordPurgeInterval = time.Hour
	// RecordPurgeBatch is the number of rooms one transaction of the purge erases the records
	// of; recordPurgeMaxBatches bounds the batches of one run, and the next run continues.
	RecordPurgeBatch      = 100
	recordPurgeMaxBatches = 20
	// MaxEndNote is the number of characters of the reason that a moderator gives for ending a
	// room (ADR-0100 §11).
	MaxEndNote = 500
)

// ErrInvalidEndNote reports a reason for ending a room that is longer than MaxEndNote
// characters or is not valid text.
var ErrInvalidEndNote = errors.New("voice: the reason for ending the room is too long or not valid text")

// Retention reads what the purge of the records keeps (ADR-0100 §13); the composition root
// implements it on the instance and moderation modules, so that a change applies at the next
// run.
type Retention interface {
	// ParticipantRetention is the instance setting retention_voice_participants.
	ParticipantRetention(ctx context.Context) (time.Duration, error)
	// ReportedVoiceRooms returns which of roomIDs a report points at.
	ReportedVoiceRooms(ctx context.Context, roomIDs []string) (map[string]bool, error)
	// HeldAccountIDs returns the accounts that a retention hold in effect preserves.
	HeldAccountIDs(ctx context.Context) ([]string, error)
}

// ParticipationRecord is a participation with its end, nil while it has not ended.
type ParticipationRecord struct {
	Participation
	End *ParticipationEnd
}

// RoomRecords are what the moderators read of a reported room (ADR-0100 §11): the room, its
// ending, every revision of its settings, and the records of its participants while the
// retention keeps them.
type RoomRecords struct {
	RoomRecord
	// Revisions are the revisions of the settings, oldest first.
	Revisions []Revision
	// Participations are the participations, in the order they started.
	Participations []ParticipationRecord
	// Bans are the accounts removed and banned from the room, in the order they were.
	Bans []Ban
}

// RecordsRepository is the part of Repository that the moderators' reads, the purge, and the
// erasure use.
type RecordsRepository interface {
	// Revisions returns the revisions of a room with their hashtags, oldest first.
	Revisions(ctx context.Context, tx uow.Tx, roomID string) ([]Revision, error)
	// Participations returns the participations of a room with their ends, in the order they
	// started.
	Participations(ctx context.Context, tx uow.Tx, roomID string) ([]ParticipationRecord, error)
	// Bans returns the bans of a room in the order they were made.
	Bans(ctx context.Context, tx uow.Tx, roomID string) ([]Ban, error)
	// RoomsToPurge returns at most limit rooms, in ID order after the room after ("" for the
	// first), that ended before endedBefore and still have records of their participants.
	RoomsToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, after string, limit int) ([]string, error)
	// RecordAccounts returns the accounts that the records of the participants of a room name,
	// as the participant or as the one who acted.
	RecordAccounts(ctx context.Context, tx uow.Tx, roomID string) ([]string, error)
	// PurgeParticipantRecords deletes the records of the participants of the rooms: their
	// participations with the ends, role changes, and raised hands, the speak grants, the
	// invitations with their revocations, and the bans.
	PurgeParticipantRecords(ctx context.Context, tx uow.Tx, roomIDs []string) error
	// RoomsRevisedBy returns at most limit rooms, in ID order, with revisions that accountID
	// wrote whose name or description is not empty.
	RoomsRevisedBy(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]string, error)
	// EraseRevisionTexts empties the names and descriptions of the revisions that accountID
	// wrote in the rooms.
	EraseRevisionTexts(ctx context.Context, tx uow.Tx, accountID string, roomIDs []string) error
}

// ReportTarget is what a report of a room points at: the room, its current revision, and its
// host, whom the report targets (ADR-0100 §11).
type ReportTarget struct {
	RoomID        string
	RevisionID    string
	HostAccountID string
}

// ReportTarget returns the target of a report of a room that accountID sees (voice.CanSee),
// after evaluating its expiry; ErrNotFound for any other room, an ended one included.
func (s *Service) ReportTarget(ctx context.Context, accountID, roomID string) (ReportTarget, error) {
	v, err := s.Get(ctx, accountID, roomID)
	if err != nil {
		return ReportTarget{}, err
	}
	return ReportTarget{RoomID: v.ID, RevisionID: v.Head.RevisionID, HostAccountID: v.Head.HostAccountID}, nil
}

// ModeratorEndInput is a moderator ending a room.
type ModeratorEndInput struct {
	RoomID string
	// ByUserID and ByAccountID are the moderator's user, whom the audit log names, and local
	// account, whom the ending names.
	ByUserID    string
	ByAccountID string
	// Note is the reason, plain text of at most MaxEndNote characters, "" for none.
	Note string
}

// EndByModerator ends an open room for a holder of manage_reports, whom the caller checked
// (moderator, ADR-0100 §11): every participant leaves (room_ended), the ending records the
// moderator and the reason, and the audit log records voice_room_ended in the same transaction,
// labelled with the room's ID: the name goes with the erasure of the account that wrote it,
// and the moderators read it from the report.
// It works with the voice chat off too. An unknown or ended room fails with ErrNotFound, and a
// reason that does not hold with ErrInvalidEndNote.
func (s *Service) EndByModerator(ctx context.Context, in ModeratorEndInput) error {
	note := strings.TrimSpace(normalizeLines(in.Note))
	if !utf8.ValidString(note) || strings.ContainsRune(note, 0) || utf8.RuneCountInString(note) > MaxEndNote {
		return ErrInvalidEndNote
	}
	rec, err := s.d.Repository.FindRoom(ctx, nil, in.RoomID)
	if err != nil {
		return err
	}
	if rec.Head.State != voice.StateOpen {
		return ErrNotFound
	}
	ps, err := s.d.Repository.Participants(ctx, nil, in.RoomID)
	if err != nil {
		return err
	}
	now := s.now()
	actor, user := moderation.AuditActor(in.ByUserID)
	auditID := s.d.IDs.NewID()
	err = s.roomTx(ctx, []string{in.RoomID}, func(tx uow.Tx, rec RoomRecord) error {
		if rec.Head.State != voice.StateOpen {
			return ErrNotFound
		}
		ending := Ending{RoomID: in.RoomID, Reason: voice.EndModerator, EndedByAccountID: in.ByAccountID, EndedAt: now, Note: note}
		did, err := s.endRoomWithTx(ctx, tx, ending)
		if err != nil {
			return err
		}
		if !did {
			return ErrNotFound
		}
		return tx.Emit(ctx, moderation.AuditRecorded{Entry: moderation.AuditEntry{
			ID: auditID, ActorKind: actor, ActorUserID: user, Action: moderation.AuditVoiceRoomEnded,
			TargetKind: moderation.AuditTargetVoiceRoom, TargetID: in.RoomID,
			TargetLabel: in.RoomID, SourceEventID: in.RoomID, OccurredAt: now,
		}})
	})
	if err != nil {
		return err
	}
	s.forgetAll(ctx, ps)
	return nil
}

// RoomRecords returns the records of a room for the moderators who read a report of it
// (ADR-0100 §11), with the voice chat off too; ErrNotFound for an unknown room. The records of
// the participants are those that the purge has not erased yet.
func (s *Service) RoomRecords(ctx context.Context, roomID string) (RoomRecords, error) {
	rec, err := s.d.Repository.FindRoom(ctx, nil, roomID)
	if err != nil {
		return RoomRecords{}, err
	}
	out := RoomRecords{RoomRecord: rec}
	if out.Revisions, err = s.d.Repository.Revisions(ctx, nil, roomID); err != nil {
		return RoomRecords{}, err
	}
	if out.Participations, err = s.d.Repository.Participations(ctx, nil, roomID); err != nil {
		return RoomRecords{}, err
	}
	if out.Bans, err = s.d.Repository.Bans(ctx, nil, roomID); err != nil {
		return RoomRecords{}, err
	}
	return out, nil
}

// kvPurge keys the cursor of JobPurgeRecords: the last room that a run looked at, so that the
// next run goes on after it however many rooms the earlier runs kept. It lasts purgeCursorLease;
// a lost cursor starts again from the first room.
const (
	kvPurge          = "voice.purge"
	kvPurgeCursor    = "cursor"
	purgeCursorLease = 30 * 24 * time.Hour
)

// PurgeRecords erases the records of the participants of the rooms that ended longer than
// retention_voice_participants ago (ADR-0100 §13), and returns how many rooms it erased them
// of. It keeps the records of a room that a report points at, until the moderation module
// erases the report after its retention, and those of a room whose records name an account
// that a retention hold preserves. Each batch is one short transaction that locks its rooms and
// reads the holds and the reports again before it deletes. One run looks at a bounded number
// of rooms in ID order from where the last run stopped (the cursor in the KeyValueStore),
// and wraps around to the first room at the end, so the rooms that it keeps never starve the
// others. It is safe to repeat. Without Deps.Retention it erases nothing.
func (s *Service) PurgeRecords(ctx context.Context) (int, error) {
	r := s.d.Retention
	if r == nil {
		return 0, nil
	}
	period, err := r.ParticipantRetention(ctx)
	if err != nil {
		return 0, err
	}
	heldIDs, err := r.HeldAccountIDs(ctx)
	if err != nil {
		return 0, err
	}
	before := s.now().Add(-period)
	repo := s.d.Repository
	start, err := s.purgeCursor(ctx)
	if err != nil {
		return 0, err
	}
	after, wrapped, total := start, start == "", 0
	for range recordPurgeMaxBatches {
		ids, err := repo.RoomsToPurge(ctx, nil, before, after, RecordPurgeBatch)
		if err != nil {
			return total, err
		}
		if len(ids) > 0 {
			after = ids[len(ids)-1]
			n, err := s.purgeBatch(ctx, ids, heldIDs)
			total += n
			if err != nil {
				return total, err
			}
		}
		if len(ids) < RecordPurgeBatch {
			// The end of the rooms: the next batch, or the next run, starts from the first.
			after = ""
			if wrapped {
				break
			}
			wrapped = true
		}
	}
	return total, s.savePurgeCursor(ctx, after)
}

// purgeBatch erases the records of the rooms ids that no report points at and whose records
// name none of heldIDs, read before the transaction, and checks both again in it after locking
// the rooms, so that a hold or a report made meanwhile keeps its room.
func (s *Service) purgeBatch(ctx context.Context, ids, heldIDs []string) (int, error) {
	r, repo := s.d.Retention, s.d.Repository
	candidates, err := s.purgeable(ctx, nil, ids, heldIDs)
	if err != nil || len(candidates) == 0 {
		return 0, err
	}
	n := 0
	err = s.roomTx(ctx, candidates, func(tx uow.Tx, _ RoomRecord) error {
		held, err := r.HeldAccountIDs(ctx)
		if err != nil {
			return err
		}
		purge, err := s.purgeable(ctx, tx, candidates, held)
		if err != nil || len(purge) == 0 {
			n = 0
			return err
		}
		n = len(purge)
		return repo.PurgeParticipantRecords(ctx, tx, purge)
	})
	if err != nil {
		return 0, fmt.Errorf("voice: purge the records of the rooms: %w", err)
	}
	return n, nil
}

// purgeable returns the rooms of ids that no report points at and whose records name none of
// heldIDs, reading the records in tx (outside any transaction when tx is nil).
func (s *Service) purgeable(ctx context.Context, tx uow.Tx, ids, heldIDs []string) ([]string, error) {
	reported, err := s.d.Retention.ReportedVoiceRooms(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if reported[id] {
			continue
		}
		held, err := s.namesHeld(ctx, tx, id, heldIDs)
		if err != nil {
			return nil, err
		}
		if !held {
			out = append(out, id)
		}
	}
	return out, nil
}

// purgeCursor returns the room after which JobPurgeRecords goes on, "" for the first.
func (s *Service) purgeCursor(ctx context.Context) (string, error) {
	if s.d.Heartbeats == nil {
		return "", nil
	}
	b, err := s.d.Heartbeats.kv.Get(ctx, kvPurge, kvPurgeCursor)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return "", nil
	}
	return string(b), err
}

// savePurgeCursor keeps the room after which the next run of JobPurgeRecords goes on.
func (s *Service) savePurgeCursor(ctx context.Context, after string) error {
	if s.d.Heartbeats == nil {
		return nil
	}
	if after == "" {
		return s.d.Heartbeats.kv.Delete(ctx, kvPurge, kvPurgeCursor)
	}
	return s.d.Heartbeats.kv.Set(ctx, kvPurge, kvPurgeCursor, []byte(after), purgeCursorLease)
}

// namesHeld reports whether the records of the participants of a room name one of heldIDs.
func (s *Service) namesHeld(ctx context.Context, tx uow.Tx, roomID string, heldIDs []string) (bool, error) {
	if len(heldIDs) == 0 {
		return false, nil
	}
	ids, err := s.d.Repository.RecordAccounts(ctx, tx, roomID)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(ids, func(id string) bool { return slices.Contains(heldIDs, id) }), nil
}

// registerPurgeJob registers the JobPurgeRecords handler and its hourly schedule.
func (s *Service) registerPurgeJob(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeRecords, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.PurgeRecords(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	if sched != nil {
		sched.Add(queue.Schedule{Name: JobPurgeRecords, Every: RecordPurgeInterval, Job: queue.NewJob{Type: JobPurgeRecords}})
	}
}

// Eraser empties the names and descriptions of the revisions that a deleted local account
// wrote (data-model §8, ADR-0100 §13); the rooms, the rest of their revisions, and their hosts
// stay.
type Eraser struct {
	uow  uow.UnitOfWork
	repo Repository
}

// NewEraser returns the voice module's eraser.
func NewEraser(u uow.UnitOfWork, r Repository) *Eraser { return &Eraser{uow: u, repo: r} }

// EraseAccountData implements account.DataEraser: one batch of the rooms with revisions that
// the account wrote, in one transaction that locks them.
func (e *Eraser) EraseAccountData(ctx context.Context, t account.ErasureTarget, _ string, limit int) (account.ErasureStep, error) {
	ids, err := e.repo.RoomsRevisedBy(ctx, nil, t.AccountID, limit)
	if err != nil || len(ids) == 0 {
		return account.ErasureStep{Done: err == nil}, err
	}
	err = lockedTx(ctx, e.uow, e.repo, ids, func(tx uow.Tx, _ RoomRecord) error {
		return e.repo.EraseRevisionTexts(ctx, tx, t.AccountID, ids)
	})
	return account.ErasureStep{Done: len(ids) < limit}, err
}
