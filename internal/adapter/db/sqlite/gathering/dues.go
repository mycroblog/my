// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/gatheringgen"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The due times, the reminders, and the recipients of the notifications of the events
// (appgathering.DueRepository).

// flag returns b as the argument of a query that takes a bool: 0 or 1.
func flag(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// FindDue implements appgathering.DueRepository.
func (r *Repository) FindDue(ctx context.Context, tx uow.Tx, gatheringID string) (time.Time, bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return time.Time{}, false, err
	}
	at, err := q.GetGatheringDue(ctx, gatheringID)
	if err != nil {
		err = notFound(err, appgathering.ErrNotFound, "gathering due")
		if errors.Is(err, appgathering.ErrNotFound) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	return fromTime(at), true, nil
}

// SetDue implements appgathering.DueRepository.
func (r *Repository) SetDue(ctx context.Context, tx uow.Tx, gatheringID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertGatheringDue(ctx, gatheringgen.UpsertGatheringDueParams{GatheringID: gatheringID, NextDueAt: toTime(at)}), "upsert gathering due")
}

// DeleteDue implements appgathering.DueRepository.
func (r *Repository) DeleteDue(ctx context.Context, tx uow.Tx, gatheringID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteGatheringDue(ctx, gatheringID), "delete gathering due")
}

// DeleteAllDues implements appgathering.DueRepository.
func (r *Repository) DeleteAllDues(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllGatheringDues(ctx), "delete all gathering dues")
}

// DueGatherings implements appgathering.DueRepository.
func (r *Repository) DueGatherings(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]appgathering.Due, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDueGatherings(ctx, gatheringgen.ListDueGatheringsParams{Now: toTime(now), RowLimit: toInt(limit)})
	if err != nil {
		return nil, wrap(err, "list due gatherings")
	}
	out := make([]appgathering.Due, 0, len(rows))
	for _, row := range rows {
		out = append(out, appgathering.Due{GatheringID: row.GatheringID, At: fromTime(row.NextDueAt)})
	}
	return out, nil
}

// AttendeeReminders implements appgathering.DueRepository.
func (r *Repository) AttendeeReminders(ctx context.Context, tx uow.Tx, gatheringID string) ([]int, bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, false, err
	}
	rows, err := q.ListGatheringAttendeeReminders(ctx, gatheringID)
	if err != nil {
		return nil, false, wrap(err, "list gathering attendee reminders")
	}
	defaults, err := q.GatheringHasDefaultReminders(ctx, gatheringID)
	if err != nil {
		return nil, false, wrap(err, "gathering has default reminders")
	}
	out := make([]int, 0, len(rows))
	for _, m := range rows {
		out = append(out, fromInt(m))
	}
	return out, fromBool(defaults), nil
}

// Reminders implements appgathering.DueRepository.
func (r *Repository) Reminders(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string][]int, error) {
	out := map[string][]int{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	list, err := listOf(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringReminders(ctx, list)
	if err != nil {
		return nil, wrap(err, "list gathering reminders")
	}
	for _, row := range rows {
		m := out[row.AccountID]
		if m == nil {
			m = []int{}
		}
		if row.MinutesBefore > 0 {
			m = append(m, fromInt(row.MinutesBefore))
		}
		out[row.AccountID] = m
	}
	return out, nil
}

// SetReminders implements appgathering.DueRepository.
func (r *Repository) SetReminders(ctx context.Context, tx uow.Tx, accountID string, minutes []int, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.UpsertGatheringReminderSetting(ctx, gatheringgen.UpsertGatheringReminderSettingParams{AccountID: accountID, UpdatedAt: toTime(at)}); err != nil {
		return wrap(err, "upsert gathering reminder setting")
	}
	if err := q.DeleteGatheringReminderOffsets(ctx, accountID); err != nil {
		return wrap(err, "delete gathering reminder offsets")
	}
	for _, m := range minutes {
		if err := q.InsertGatheringReminderOffset(ctx, gatheringgen.InsertGatheringReminderOffsetParams{AccountID: accountID, MinutesBefore: toInt(m)}); err != nil {
			return wrap(err, "insert gathering reminder offset")
		}
	}
	return nil
}

// DeleteReminders implements appgathering.DueRepository.
func (r *Repository) DeleteReminders(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteGatheringReminderOffsets(ctx, accountID); err != nil {
		return wrap(err, "delete gathering reminder offsets")
	}
	return wrap(q.DeleteGatheringReminderSetting(ctx, accountID), "delete gathering reminder setting")
}

// AttendingGatherings implements appgathering.DueRepository.
func (r *Repository) AttendingGatherings(ctx context.Context, tx uow.Tx, accountID string, now time.Time, limit int) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListAttendingGatherings(ctx, gatheringgen.ListAttendingGatheringsParams{AccountID: accountID, Now: toTime(now), RowLimit: toInt(limit)})
	return ids, wrap(err, "list attending gatherings")
}

// Audience implements appgathering.DueRepository.
func (r *Repository) Audience(ctx context.Context, tx uow.Tx, gatheringID string, invitees bool, after string, limit int) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListGatheringAudience(ctx, gatheringgen.ListGatheringAudienceParams{
		GatheringID: gatheringID, Invitees: flag(invitees), AfterID: after, RowLimit: toInt(limit),
	})
	return ids, wrap(err, "list gathering audience")
}

// AudienceFacts implements appgathering.DueRepository.
func (r *Repository) AudienceFacts(ctx context.Context, tx uow.Tx, gatheringID string, accountIDs []string) (map[string]appgathering.ViewerFacts, error) {
	out := make(map[string]appgathering.ViewerFacts, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	list, err := listOf(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringAudienceFacts(ctx, gatheringgen.ListGatheringAudienceFactsParams{GatheringID: gatheringID, AccountIds: list})
	if err != nil {
		return nil, wrap(err, "list gathering audience facts")
	}
	for _, row := range rows {
		out[row.AccountID] = appgathering.ViewerFacts{Invited: row.Invited, Answer: gathering.Answer(row.Answer), Approved: fromBool(row.Approved)}
	}
	return out, nil
}
