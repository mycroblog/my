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
// moderator and the reason, and the audit log records voice_room_ended in the same transaction.
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
			TargetLabel: rec.Revision.Settings.Name, SourceEventID: in.RoomID, OccurredAt: now,
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

// PurgeRecords erases the records of the participants of the rooms that ended longer than
// retention_voice_participants ago (ADR-0100 §13), and returns how many rooms it erased them
// of. It keeps the records of a room that a report points at, until the moderation module
// erases the report after its retention, and those of a room whose records name an account
// that a retention hold preserves. Each batch is one short transaction that locks its rooms;
// one run erases a bounded number of rooms, and the next run continues. It is safe to repeat.
// Without Deps.Retention it erases nothing.
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
	after, total := "", 0
	for range recordPurgeMaxBatches {
		ids, err := repo.RoomsToPurge(ctx, nil, before, after, RecordPurgeBatch)
		if err != nil || len(ids) == 0 {
			return total, err
		}
		after = ids[len(ids)-1]
		reported, err := r.ReportedVoiceRooms(ctx, ids)
		if err != nil {
			return total, err
		}
		purge := make([]string, 0, len(ids))
		for _, id := range ids {
			if reported[id] {
				continue
			}
			held, err := s.namesHeld(ctx, id, heldIDs)
			if err != nil {
				return total, err
			}
			if !held {
				purge = append(purge, id)
			}
		}
		if len(purge) > 0 {
			err := s.roomTx(ctx, purge, func(tx uow.Tx, _ RoomRecord) error {
				return repo.PurgeParticipantRecords(ctx, tx, purge)
			})
			if err != nil {
				return total, fmt.Errorf("voice: purge the records of the rooms: %w", err)
			}
			total += len(purge)
		}
		if len(ids) < RecordPurgeBatch {
			break
		}
	}
	return total, nil
}

// namesHeld reports whether the records of the participants of a room name one of heldIDs.
func (s *Service) namesHeld(ctx context.Context, roomID string, heldIDs []string) (bool, error) {
	if len(heldIDs) == 0 {
		return false, nil
	}
	ids, err := s.d.Repository.RecordAccounts(ctx, nil, roomID)
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
