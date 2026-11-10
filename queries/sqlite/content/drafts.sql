-- SPDX-License-Identifier: AGPL-3.0-only

-- Drafts and scheduled posts (data-model 4.4, ADR-0156).

-- name: InsertDraft :exec
INSERT INTO drafts (id, account_id, created_at) VALUES (?, ?, ?);

-- name: GetDraft :one
SELECT id, account_id, created_at FROM drafts WHERE id = ?;

-- name: GetDraftForUpdate :one
SELECT id, account_id, created_at FROM drafts WHERE id = ?;

-- name: UpsertDraftContent :exec
INSERT INTO draft_contents (draft_id, source_text, spoiler_text, sensitive, language, visibility, in_reply_to_status_id, quoted_status_id, updated_at)
VALUES (sqlc.arg(draft_id), sqlc.arg(source_text), sqlc.arg(spoiler_text), sqlc.arg(sensitive), CAST(sqlc.narg(language) AS TEXT), sqlc.arg(visibility),
  CAST(sqlc.narg(in_reply_to_status_id) AS TEXT), CAST(sqlc.narg(quoted_status_id) AS TEXT), sqlc.arg(updated_at))
ON CONFLICT (draft_id) DO UPDATE SET
  source_text = excluded.source_text, spoiler_text = excluded.spoiler_text, sensitive = excluded.sensitive,
  language = excluded.language, visibility = excluded.visibility, in_reply_to_status_id = excluded.in_reply_to_status_id,
  quoted_status_id = excluded.quoted_status_id, updated_at = excluded.updated_at;

-- name: GetDraftContent :one
SELECT draft_id, source_text, spoiler_text, sensitive, language, visibility, in_reply_to_status_id, quoted_status_id, updated_at
FROM draft_contents WHERE draft_id = ?;

-- name: InsertDraftInteractionPolicy :exec
INSERT INTO draft_interaction_policies (draft_id, kind, approval, audience) VALUES (?, ?, ?, ?);

-- name: ListDraftInteractionPolicies :many
SELECT draft_id, kind, approval, audience FROM draft_interaction_policies WHERE draft_id = ? ORDER BY kind, approval;

-- name: DeleteDraftInteractionPolicies :exec
DELETE FROM draft_interaction_policies WHERE draft_id = ?;

-- name: InsertDraftAttachment :exec
INSERT INTO draft_attachments (draft_id, position, media_id, description) VALUES (?, ?, ?, ?);

-- name: ListDraftAttachments :many
SELECT draft_id, position, media_id, description FROM draft_attachments WHERE draft_id = ? ORDER BY position;

-- name: DeleteDraftAttachments :exec
DELETE FROM draft_attachments WHERE draft_id = ?;

-- name: InsertDraftPoll :exec
INSERT INTO draft_polls (draft_id, multiple, hide_totals, duration_seconds) VALUES (?, ?, ?, ?);

-- name: GetDraftPoll :one
SELECT draft_id, multiple, hide_totals, duration_seconds FROM draft_polls WHERE draft_id = ?;

-- name: DeleteDraftPoll :exec
DELETE FROM draft_polls WHERE draft_id = ?;

-- name: InsertDraftPollOption :exec
INSERT INTO draft_poll_options (draft_id, position, title) VALUES (?, ?, ?);

-- name: ListDraftPollOptions :many
SELECT title FROM draft_poll_options WHERE draft_id = ? ORDER BY position;

-- name: DeleteDraftPollOptions :exec
DELETE FROM draft_poll_options WHERE draft_id = ?;

-- name: InsertDraftSchedule :exec
INSERT INTO draft_schedules (id, draft_id, publish_at, created_at) VALUES (?, ?, ?, ?);

-- name: GetDraftSchedule :one
SELECT id, draft_id, publish_at, created_at FROM draft_schedules WHERE id = ?;

-- name: InsertDraftScheduleOutcome :exec
INSERT INTO draft_schedule_outcomes (schedule_id, outcome, failure_reason, occurred_at)
VALUES (sqlc.arg(schedule_id), sqlc.arg(outcome), CAST(sqlc.narg(failure_reason) AS TEXT), sqlc.arg(occurred_at));

-- The newest schedule of a draft that has an outcome, with it: what the author's screens show of
-- a schedule that failed or was cancelled.
-- name: GetLatestDraftScheduleOutcome :one
SELECT s.id, s.draft_id, s.publish_at, s.created_at, o.outcome, o.failure_reason, o.occurred_at
FROM draft_schedules s JOIN draft_schedule_outcomes o ON o.schedule_id = s.id
WHERE s.draft_id = ?
ORDER BY s.id DESC
LIMIT 1;

-- The schedules of ids with their draft and outcome, for the notifications that name them.
-- name: ListDraftSchedulesWithOutcomes :many
SELECT s.id, s.draft_id, s.publish_at, s.created_at, o.outcome, o.failure_reason, o.occurred_at
FROM draft_schedules s LEFT JOIN draft_schedule_outcomes o ON o.schedule_id = s.id
WHERE s.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY s.id;

-- name: InsertScheduledDraft :exec
INSERT INTO scheduled_drafts (draft_id, schedule_id, account_id, publish_at) VALUES (?, ?, ?, ?);

-- name: GetScheduledDraft :one
SELECT draft_id, schedule_id, account_id, publish_at FROM scheduled_drafts WHERE draft_id = ?;

-- name: GetScheduledDraftBySchedule :one
SELECT draft_id, schedule_id, account_id, publish_at FROM scheduled_drafts WHERE schedule_id = ?;

-- name: DeleteScheduledDraftBySchedule :execrows
DELETE FROM scheduled_drafts WHERE schedule_id = ?;

-- name: CountUnscheduledDrafts :one
SELECT count(*) FROM drafts d
WHERE d.account_id = ? AND NOT EXISTS (SELECT 1 FROM scheduled_drafts s WHERE s.draft_id = d.id);

-- name: CountScheduledDrafts :one
SELECT count(*) FROM scheduled_drafts WHERE account_id = ?;

-- name: CountScheduledDraftsBetween :one
SELECT count(*) FROM scheduled_drafts
WHERE account_id = sqlc.arg(account_id) AND publish_at >= sqlc.arg(from_at) AND publish_at < sqlc.arg(to_at);

-- ListDrafts returns up to @row_limit drafts of @account_id, newest first, before the draft ID
-- @before_id (NULL for the first page): every draft, or with @scheduled set the scheduled ones
-- (true) or the others (false).
-- name: ListDrafts :many
SELECT d.id, d.account_id, d.created_at FROM drafts d
WHERE d.account_id = sqlc.arg(account_id)
  AND (CAST(sqlc.narg(before_id) AS TEXT) IS NULL OR d.id < CAST(sqlc.narg(before_id) AS TEXT))
  AND (CAST(sqlc.narg(scheduled) AS INTEGER) IS NULL
    OR CAST(sqlc.narg(scheduled) AS INTEGER) = EXISTS (SELECT 1 FROM scheduled_drafts s WHERE s.draft_id = d.id))
ORDER BY d.id DESC
LIMIT sqlc.arg(row_limit);

-- Deleting a draft deletes every row of it: its content, its schedules with their outcomes, and
-- the schedule in effect.

-- name: DeleteDraftContent :exec
DELETE FROM draft_contents WHERE draft_id = ?;

-- name: DeleteScheduledDraft :exec
DELETE FROM scheduled_drafts WHERE draft_id = ?;

-- name: DeleteDraftScheduleOutcomes :exec
DELETE FROM draft_schedule_outcomes WHERE schedule_id IN (SELECT id FROM draft_schedules WHERE draft_id = ?);

-- name: DeleteDraftSchedules :exec
DELETE FROM draft_schedules WHERE draft_id = ?;

-- name: DeleteDraft :execrows
DELETE FROM drafts WHERE id = ?;

-- name: ListDraftIDsOf :many
SELECT id FROM drafts WHERE account_id = ? ORDER BY id;

-- The projection scheduled_drafts again from the schedules without an outcome.

-- name: DeleteAllScheduledDrafts :exec
DELETE FROM scheduled_drafts;

-- name: RebuildScheduledDrafts :exec
INSERT INTO scheduled_drafts (draft_id, schedule_id, account_id, publish_at)
SELECT s.draft_id, s.id, d.account_id, s.publish_at
FROM draft_schedules s JOIN drafts d ON d.id = s.draft_id
WHERE s.id = (
  SELECT max(m.id) FROM draft_schedules m
  WHERE m.draft_id = s.draft_id AND NOT EXISTS (SELECT 1 FROM draft_schedule_outcomes o WHERE o.schedule_id = m.id));
