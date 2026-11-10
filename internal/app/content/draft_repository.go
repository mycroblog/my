// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// DraftScheduleRecord is a schedule of a draft with its outcome, whose Outcome is "" while the
// schedule is in effect.
type DraftScheduleRecord struct {
	Schedule content.DraftSchedule
	Outcome  content.DraftScheduleOutcome
}

// DraftRepository is the persistence port of drafts and scheduled posts (data-model §4.4,
// ADR-0156). The database adapters' status repositories implement it and pass
// contenttest.DraftRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type DraftRepository interface {
	// InsertDraft inserts a draft.
	InsertDraft(ctx context.Context, tx uow.Tx, d content.Draft) error
	// FindDraft returns a draft.
	FindDraft(ctx context.Context, id string) (content.Draft, error)
	// LockDraft returns a draft inside tx, holding it until tx ends on PostgreSQL, so that the
	// saves and schedules of one draft run one after another.
	LockDraft(ctx context.Context, tx uow.Tx, id string) (content.Draft, error)
	// SaveDraftContent replaces the content of a draft with its interaction policy, attachments,
	// and poll.
	SaveDraftContent(ctx context.Context, tx uow.Tx, c content.DraftContent) error
	// FindDraftContent returns the content of a draft.
	FindDraftContent(ctx context.Context, draftID string) (content.DraftContent, error)
	// FindDraftContentTx is FindDraftContent inside tx.
	FindDraftContentTx(ctx context.Context, tx uow.Tx, draftID string) (content.DraftContent, error)
	// DeleteDraft deletes every row of a draft: its content, its schedules with their outcomes,
	// and its schedule in effect. It reports whether the draft existed.
	DeleteDraft(ctx context.Context, tx uow.Tx, id string) (bool, error)
	// ListDrafts returns up to limit drafts of an account, newest first, before the draft ID
	// beforeID ("" for the first page): every draft with a nil scheduled, else the scheduled
	// ones (true) or the others (false).
	ListDrafts(ctx context.Context, accountID, beforeID string, scheduled *bool, limit int) ([]content.Draft, error)
	// ListDraftIDsOf returns the IDs of every draft of an account inside tx.
	ListDraftIDsOf(ctx context.Context, tx uow.Tx, accountID string) ([]string, error)
	// CountUnscheduledDrafts counts the drafts of an account without a schedule in effect.
	CountUnscheduledDrafts(ctx context.Context, tx uow.Tx, accountID string) (int, error)
	// CountScheduledDrafts counts the schedules in effect of an account whose publish_at is in
	// [from, to); zero times leave that end open.
	CountScheduledDrafts(ctx context.Context, tx uow.Tx, accountID string, from, to time.Time) (int, error)

	// InsertDraftSchedule inserts a schedule.
	InsertDraftSchedule(ctx context.Context, tx uow.Tx, s content.DraftSchedule) error
	// InsertDraftScheduleOutcome records the end of a schedule.
	InsertDraftScheduleOutcome(ctx context.Context, tx uow.Tx, o content.DraftScheduleOutcome) error
	// LatestDraftScheduleOutcome returns the newest schedule of a draft that has ended, with its
	// outcome.
	LatestDraftScheduleOutcome(ctx context.Context, draftID string) (DraftScheduleRecord, error)
	// DraftSchedules returns the schedules of ids that exist, with their outcomes, by ID, with
	// one query.
	DraftSchedules(ctx context.Context, ids []string) (map[string]DraftScheduleRecord, error)

	// The projection scheduled_drafts, which this module writes in the transactions that record
	// schedules and their outcomes.

	// InsertScheduledDraft records the schedule in effect of a draft.
	InsertScheduledDraft(ctx context.Context, tx uow.Tx, s content.ScheduledDraft) error
	// DeleteScheduledDraft deletes the row of the schedule scheduleID and reports whether it was
	// in effect; of two concurrent calls, one alone reports true.
	DeleteScheduledDraft(ctx context.Context, tx uow.Tx, scheduleID string) (bool, error)
	// FindScheduledDraft returns the schedule in effect of a draft.
	FindScheduledDraft(ctx context.Context, draftID string) (content.ScheduledDraft, error)
	// FindScheduledDraftTx is FindScheduledDraft inside tx.
	FindScheduledDraftTx(ctx context.Context, tx uow.Tx, draftID string) (content.ScheduledDraft, error)
	// FindScheduledDraftBySchedule returns the row of the schedule scheduleID while it is in
	// effect.
	FindScheduledDraftBySchedule(ctx context.Context, scheduleID string) (content.ScheduledDraft, error)
	// RebuildScheduledDrafts empties scheduled_drafts and derives it again from the newest
	// schedule without an outcome of each draft.
	RebuildScheduledDrafts(ctx context.Context, tx uow.Tx) error
}
