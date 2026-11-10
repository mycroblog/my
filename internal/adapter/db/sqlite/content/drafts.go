// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.DraftRepository = (*StatusRepository)(nil)

// InsertDraft implements content.DraftRepository.
func (r *StatusRepository) InsertDraft(ctx context.Context, tx uow.Tx, d content.Draft) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertDraft(ctx, contentgen.InsertDraftParams{ID: d.ID, AccountID: d.AccountID, CreatedAt: sqlite.ToMillis(d.CreatedAt)})
	return sqlite.Wrap(err, "insert draft")
}

// FindDraft implements content.DraftRepository.
func (r *StatusRepository) FindDraft(ctx context.Context, id string) (content.Draft, error) {
	row, err := r.db.reader().GetDraft(ctx, id)
	if err != nil {
		return content.Draft{}, sqlite.NotFound(err, appcontent.ErrNotFound, "draft "+id)
	}
	return draftFrom(row), nil
}

// LockDraft implements content.DraftRepository. A SQLite transaction holds the database, so the
// read alone serializes.
func (r *StatusRepository) LockDraft(ctx context.Context, tx uow.Tx, id string) (content.Draft, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Draft{}, err
	}
	row, err := q.GetDraftForUpdate(ctx, id)
	if err != nil {
		return content.Draft{}, sqlite.NotFound(err, appcontent.ErrNotFound, "draft "+id)
	}
	return draftFrom(row), nil
}

// SaveDraftContent implements content.DraftRepository.
func (r *StatusRepository) SaveDraftContent(ctx context.Context, tx uow.Tx, c content.DraftContent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := clearDraftContent(ctx, q, c.DraftID); err != nil {
		return err
	}
	err = q.UpsertDraftContent(ctx, contentgen.UpsertDraftContentParams{
		DraftID: c.DraftID, SourceText: c.Text, SpoilerText: c.SpoilerText, Sensitive: sqlite.BoolToInt(c.Sensitive),
		Language: sqlite.ToNullString(c.Language), Visibility: string(c.Visibility),
		InReplyToStatusID: sqlite.ToNullString(c.InReplyToID), QuotedStatusID: sqlite.ToNullString(c.QuotedStatusID),
		UpdatedAt: sqlite.ToMillis(c.UpdatedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "save draft content")
	}
	for _, k := range content.InteractionKinds {
		set, ok := c.InteractionPolicy[k]
		if !ok {
			continue
		}
		rules := []struct {
			approval content.InteractionApproval
			audience content.InteractionAudience
		}{{content.ApprovalAutomatic, set.Automatic}, {content.ApprovalManual, set.Manual}}
		for _, rule := range rules {
			if rule.audience == "" {
				continue
			}
			err := q.InsertDraftInteractionPolicy(ctx, contentgen.InsertDraftInteractionPolicyParams{
				DraftID: c.DraftID, Kind: string(k), Approval: string(rule.approval), Audience: string(rule.audience),
			})
			if err != nil {
				return sqlite.Wrap(err, "insert draft interaction policy")
			}
		}
	}
	for _, a := range c.Attachments {
		err := q.InsertDraftAttachment(ctx, contentgen.InsertDraftAttachmentParams{
			DraftID: c.DraftID, Position: int64(a.Position), MediaID: a.MediaID, Description: sqlite.ToNullString(a.Description),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert draft attachment")
		}
	}
	if p := c.Poll; p != nil {
		err := q.InsertDraftPoll(ctx, contentgen.InsertDraftPollParams{
			DraftID: c.DraftID, Multiple: sqlite.BoolToInt(p.Multiple), HideTotals: sqlite.BoolToInt(p.HideTotals),
			DurationSeconds: int64(p.Duration / time.Second),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert draft poll")
		}
		for i, o := range p.Options {
			err := q.InsertDraftPollOption(ctx, contentgen.InsertDraftPollOptionParams{DraftID: c.DraftID, Position: int64(i), Title: o})
			if err != nil {
				return sqlite.Wrap(err, "insert draft poll option")
			}
		}
	}
	return nil
}

// clearDraftContent deletes the children of a draft's content, which a save writes again.
func clearDraftContent(ctx context.Context, q *contentgen.Queries, draftID string) error {
	return sqlite.RunSteps(ctx, draftID, "clear draft content",
		q.DeleteDraftInteractionPolicies, q.DeleteDraftAttachments, q.DeleteDraftPollOptions, q.DeleteDraftPoll)
}

// FindDraftContent implements content.DraftRepository.
func (r *StatusRepository) FindDraftContent(ctx context.Context, draftID string) (content.DraftContent, error) {
	return draftContentFrom(ctx, r.db.reader(), draftID)
}

// FindDraftContentTx implements content.DraftRepository.
func (r *StatusRepository) FindDraftContentTx(ctx context.Context, tx uow.Tx, draftID string) (content.DraftContent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.DraftContent{}, err
	}
	return draftContentFrom(ctx, q, draftID)
}

func draftContentFrom(ctx context.Context, q *contentgen.Queries, draftID string) (content.DraftContent, error) {
	row, err := q.GetDraftContent(ctx, draftID)
	if err != nil {
		return content.DraftContent{}, sqlite.NotFound(err, appcontent.ErrNotFound, "draft content "+draftID)
	}
	c := content.DraftContent{
		DraftID: row.DraftID, Text: row.SourceText, SpoilerText: row.SpoilerText, Sensitive: row.Sensitive == 1,
		Language: row.Language.String, Visibility: content.Visibility(row.Visibility),
		InReplyToID: row.InReplyToStatusID.String, QuotedStatusID: row.QuotedStatusID.String,
		UpdatedAt: sqlite.FromMillis(row.UpdatedAt),
	}
	policies, err := q.ListDraftInteractionPolicies(ctx, draftID)
	if err != nil {
		return content.DraftContent{}, sqlite.Wrap(err, "list draft interaction policies")
	}
	for _, p := range policies {
		if c.InteractionPolicy == nil {
			c.InteractionPolicy = map[content.InteractionKind]content.InteractionSetting{}
		}
		k := content.InteractionKind(p.Kind)
		set := c.InteractionPolicy[k]
		if content.InteractionApproval(p.Approval) == content.ApprovalManual {
			set.Manual = content.InteractionAudience(p.Audience)
		} else {
			set.Automatic = content.InteractionAudience(p.Audience)
		}
		c.InteractionPolicy[k] = set
	}
	attachments, err := q.ListDraftAttachments(ctx, draftID)
	if err != nil {
		return content.DraftContent{}, sqlite.Wrap(err, "list draft attachments")
	}
	for _, a := range attachments {
		c.Attachments = append(c.Attachments, content.DraftAttachment{Position: int(a.Position), MediaID: a.MediaID, Description: a.Description.String})
	}
	poll, err := q.GetDraftPoll(ctx, draftID)
	switch {
	case err == nil:
		options, err := q.ListDraftPollOptions(ctx, draftID)
		if err != nil {
			return content.DraftContent{}, sqlite.Wrap(err, "list draft poll options")
		}
		c.Poll = &content.DraftPoll{
			Options: options, Duration: time.Duration(poll.DurationSeconds) * time.Second,
			Multiple: poll.Multiple == 1, HideTotals: poll.HideTotals == 1,
		}
	case !errors.Is(err, sql.ErrNoRows):
		return content.DraftContent{}, sqlite.Wrap(err, "get draft poll")
	}
	return c, nil
}

// DeleteDraft implements content.DraftRepository.
func (r *StatusRepository) DeleteDraft(ctx context.Context, tx uow.Tx, id string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	if err := clearDraftContent(ctx, q, id); err != nil {
		return false, err
	}
	err = sqlite.RunSteps(ctx, id, "delete draft",
		q.DeleteDraftContent, q.DeleteScheduledDraft, q.DeleteDraftScheduleOutcomes, q.DeleteDraftSchedules)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteDraft(ctx, id)
	if err != nil {
		return false, sqlite.Wrap(err, "delete draft")
	}
	return n > 0, nil
}

// ListDrafts implements content.DraftRepository.
func (r *StatusRepository) ListDrafts(ctx context.Context, accountID, beforeID string, scheduled *bool, limit int) ([]content.Draft, error) {
	var s sql.NullInt64
	if scheduled != nil {
		s = sql.NullInt64{Int64: sqlite.BoolToInt(*scheduled), Valid: true}
	}
	rows, err := r.db.reader().ListDrafts(ctx, contentgen.ListDraftsParams{
		AccountID: accountID, BeforeID: sqlite.ToNullString(beforeID), Scheduled: s, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list drafts")
	}
	out := make([]content.Draft, 0, len(rows))
	for _, row := range rows {
		out = append(out, draftFrom(row))
	}
	return out, nil
}

// ListDraftIDsOf implements content.DraftRepository.
func (r *StatusRepository) ListDraftIDsOf(ctx context.Context, tx uow.Tx, accountID string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListDraftIDsOf(ctx, accountID)
	return ids, sqlite.Wrap(err, "list draft ids")
}

// CountUnscheduledDrafts implements content.DraftRepository.
func (r *StatusRepository) CountUnscheduledDrafts(ctx context.Context, tx uow.Tx, accountID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountUnscheduledDrafts(ctx, accountID)
	return int(n), sqlite.Wrap(err, "count unscheduled drafts")
}

// CountScheduledDrafts implements content.DraftRepository.
func (r *StatusRepository) CountScheduledDrafts(ctx context.Context, tx uow.Tx, accountID string, from, to time.Time) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	if from.IsZero() && to.IsZero() {
		n, err := q.CountScheduledDrafts(ctx, accountID)
		return int(n), sqlite.Wrap(err, "count scheduled drafts")
	}
	fromMs, toMs := int64(0), int64(1<<62)
	if !from.IsZero() {
		fromMs = sqlite.ToMillis(from)
	}
	if !to.IsZero() {
		toMs = sqlite.ToMillis(to)
	}
	n, err := q.CountScheduledDraftsBetween(ctx, contentgen.CountScheduledDraftsBetweenParams{AccountID: accountID, FromAt: fromMs, ToAt: toMs})
	return int(n), sqlite.Wrap(err, "count scheduled drafts")
}

// InsertDraftSchedule implements content.DraftRepository.
func (r *StatusRepository) InsertDraftSchedule(ctx context.Context, tx uow.Tx, s content.DraftSchedule) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertDraftSchedule(ctx, contentgen.InsertDraftScheduleParams{
		ID: s.ID, DraftID: s.DraftID, PublishAt: sqlite.ToMillis(s.PublishAt), CreatedAt: sqlite.ToMillis(s.CreatedAt),
	})
	return sqlite.Wrap(err, "insert draft schedule")
}

// InsertDraftScheduleOutcome implements content.DraftRepository.
func (r *StatusRepository) InsertDraftScheduleOutcome(ctx context.Context, tx uow.Tx, o content.DraftScheduleOutcome) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertDraftScheduleOutcome(ctx, contentgen.InsertDraftScheduleOutcomeParams{
		ScheduleID: o.ScheduleID, Outcome: string(o.Outcome), FailureReason: sqlite.ToNullString(string(o.FailureReason)),
		OccurredAt: sqlite.ToMillis(o.OccurredAt),
	})
	return sqlite.Wrap(err, "insert draft schedule outcome")
}

// LatestDraftScheduleOutcome implements content.DraftRepository.
func (r *StatusRepository) LatestDraftScheduleOutcome(ctx context.Context, draftID string) (appcontent.DraftScheduleRecord, error) {
	row, err := r.db.reader().GetLatestDraftScheduleOutcome(ctx, draftID)
	if err != nil {
		return appcontent.DraftScheduleRecord{}, sqlite.NotFound(err, appcontent.ErrNotFound, "draft schedule outcome of "+draftID)
	}
	return appcontent.DraftScheduleRecord{
		Schedule: content.DraftSchedule{
			ID: row.ID, DraftID: row.DraftID, PublishAt: sqlite.FromMillis(row.PublishAt), CreatedAt: sqlite.FromMillis(row.CreatedAt),
		},
		Outcome: content.DraftScheduleOutcome{
			ScheduleID: row.ID, Outcome: content.DraftOutcome(row.Outcome), FailureReason: content.DraftFailureReason(row.FailureReason.String),
			OccurredAt: sqlite.FromMillis(row.OccurredAt),
		},
	}, nil
}

// DraftSchedules implements content.DraftRepository.
func (r *StatusRepository) DraftSchedules(ctx context.Context, ids []string) (map[string]appcontent.DraftScheduleRecord, error) {
	out := map[string]appcontent.DraftScheduleRecord{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDraftSchedulesWithOutcomes(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list draft schedules")
	}
	for _, row := range rows {
		rec := appcontent.DraftScheduleRecord{Schedule: content.DraftSchedule{
			ID: row.ID, DraftID: row.DraftID, PublishAt: sqlite.FromMillis(row.PublishAt), CreatedAt: sqlite.FromMillis(row.CreatedAt),
		}}
		if row.Outcome.Valid {
			rec.Outcome = content.DraftScheduleOutcome{
				ScheduleID: row.ID, Outcome: content.DraftOutcome(row.Outcome.String),
				FailureReason: content.DraftFailureReason(row.FailureReason.String), OccurredAt: sqlite.FromMillis(row.OccurredAt.Int64),
			}
		}
		out[row.ID] = rec
	}
	return out, nil
}

// InsertScheduledDraft implements content.DraftRepository.
func (r *StatusRepository) InsertScheduledDraft(ctx context.Context, tx uow.Tx, s content.ScheduledDraft) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertScheduledDraft(ctx, contentgen.InsertScheduledDraftParams{
		DraftID: s.DraftID, ScheduleID: s.ScheduleID, AccountID: s.AccountID, PublishAt: sqlite.ToMillis(s.PublishAt),
	})
	return sqlite.Wrap(err, "insert scheduled draft")
}

// DeleteScheduledDraft implements content.DraftRepository.
func (r *StatusRepository) DeleteScheduledDraft(ctx context.Context, tx uow.Tx, scheduleID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteScheduledDraftBySchedule(ctx, scheduleID)
	if err != nil {
		return false, sqlite.Wrap(err, "delete scheduled draft")
	}
	return n > 0, nil
}

// FindScheduledDraft implements content.DraftRepository.
func (r *StatusRepository) FindScheduledDraft(ctx context.Context, draftID string) (content.ScheduledDraft, error) {
	row, err := r.db.reader().GetScheduledDraft(ctx, draftID)
	if err != nil {
		return content.ScheduledDraft{}, sqlite.NotFound(err, appcontent.ErrNotFound, "schedule of draft "+draftID)
	}
	return scheduledDraftFrom(row), nil
}

// FindScheduledDraftTx implements content.DraftRepository.
func (r *StatusRepository) FindScheduledDraftTx(ctx context.Context, tx uow.Tx, draftID string) (content.ScheduledDraft, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.ScheduledDraft{}, err
	}
	row, err := q.GetScheduledDraft(ctx, draftID)
	if err != nil {
		return content.ScheduledDraft{}, sqlite.NotFound(err, appcontent.ErrNotFound, "schedule of draft "+draftID)
	}
	return scheduledDraftFrom(row), nil
}

// FindScheduledDraftBySchedule implements content.DraftRepository.
func (r *StatusRepository) FindScheduledDraftBySchedule(ctx context.Context, scheduleID string) (content.ScheduledDraft, error) {
	row, err := r.db.reader().GetScheduledDraftBySchedule(ctx, scheduleID)
	if err != nil {
		return content.ScheduledDraft{}, sqlite.NotFound(err, appcontent.ErrNotFound, "scheduled draft "+scheduleID)
	}
	return scheduledDraftFrom(row), nil
}

// RebuildScheduledDrafts implements content.DraftRepository.
func (r *StatusRepository) RebuildScheduledDrafts(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild scheduled drafts",
		(*contentgen.Queries).DeleteAllScheduledDrafts, (*contentgen.Queries).RebuildScheduledDrafts)
}

func draftFrom(row contentgen.Draft) content.Draft {
	return content.Draft{ID: row.ID, AccountID: row.AccountID, CreatedAt: sqlite.FromMillis(row.CreatedAt)}
}

func scheduledDraftFrom(row contentgen.ScheduledDraft) content.ScheduledDraft {
	return content.ScheduledDraft{
		DraftID: row.DraftID, ScheduleID: row.ScheduleID, AccountID: row.AccountID, PublishAt: sqlite.FromMillis(row.PublishAt),
	}
}
