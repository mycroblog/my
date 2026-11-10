-- SPDX-License-Identifier: AGPL-3.0-only

-- The due times of the events (gathering_dues, a projection), the reminders that the accounts
-- chose (gathering_reminder_settings and gathering_reminder_offsets, current values), and the
-- recipients of the notifications of the events (ADR-0161 sections 11 and 12).

-- name: GetGatheringDue :one
SELECT next_due_at FROM gathering_dues WHERE gathering_id = ?;

-- name: UpsertGatheringDue :exec
INSERT INTO gathering_dues (gathering_id, next_due_at) VALUES (?, ?)
ON CONFLICT (gathering_id) DO UPDATE SET next_due_at = excluded.next_due_at;

-- name: DeleteGatheringDue :exec
DELETE FROM gathering_dues WHERE gathering_id = ?;

-- name: DeleteAllGatheringDues :exec
DELETE FROM gathering_dues;

-- ListDueGatherings returns the events whose next due time is at or before now, the earliest
-- first.
-- name: ListDueGatherings :many
SELECT gathering_id, next_due_at FROM gathering_dues WHERE next_due_at <= sqlc.arg(now)
ORDER BY next_due_at, gathering_id
LIMIT sqlc.arg(row_limit);

-- ListGatheringAttendeeReminders returns the reminders that the attendees of an event who answer
-- going or maybe chose, each once, the earliest first.
-- name: ListGatheringAttendeeReminders :many
SELECT DISTINCT o.minutes_before FROM gathering_attendees a
JOIN gathering_reminder_offsets o ON o.account_id = a.account_id
WHERE a.gathering_id = ? AND a.answer IN ('going', 'maybe')
ORDER BY o.minutes_before DESC;

-- GatheringHasDefaultReminders reports whether an attendee of an event who answers going or maybe
-- has not chosen its reminders, so that the defaults apply to it.
-- name: GatheringHasDefaultReminders :one
SELECT CAST(EXISTS (SELECT 1 FROM gathering_attendees a
  WHERE a.gathering_id = ? AND a.answer IN ('going', 'maybe')
    AND NOT EXISTS (SELECT 1 FROM gathering_reminder_settings s WHERE s.account_id = a.account_id)) AS INTEGER) AS has_defaults;

-- ListGatheringReminders returns the reminders of the accounts of a JSON array that chose them, a
-- row per reminder, and a row with 0 for an account that chose none.
-- name: ListGatheringReminders :many
SELECT s.account_id, CAST(COALESCE(o.minutes_before, 0) AS INTEGER) AS minutes_before
FROM gathering_reminder_settings s LEFT JOIN gathering_reminder_offsets o ON o.account_id = s.account_id
WHERE s.account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)))
ORDER BY s.account_id, minutes_before DESC;

-- name: UpsertGatheringReminderSetting :exec
INSERT INTO gathering_reminder_settings (account_id, updated_at) VALUES (?, ?)
ON CONFLICT (account_id) DO UPDATE SET updated_at = excluded.updated_at;

-- name: DeleteGatheringReminderSetting :exec
DELETE FROM gathering_reminder_settings WHERE account_id = ?;

-- name: DeleteGatheringReminderOffsets :exec
DELETE FROM gathering_reminder_offsets WHERE account_id = ?;

-- name: InsertGatheringReminderOffset :exec
INSERT INTO gathering_reminder_offsets (account_id, minutes_before) VALUES (?, ?);

-- ListAttendingGatherings returns the events that are scheduled and have not started at now, that
-- an account answers going or maybe, by the nearest start first.
-- name: ListAttendingGatherings :many
SELECT h.gathering_id FROM gathering_attendees a JOIN gathering_heads h ON h.gathering_id = a.gathering_id
WHERE a.account_id = sqlc.arg(account_id) AND a.answer IN ('going', 'maybe') AND h.state = 'scheduled' AND h.starts_at > sqlc.arg(now)
ORDER BY h.starts_at, h.gathering_id
LIMIT sqlc.arg(row_limit);

-- ListGatheringAudience returns the accounts after an ID (or from the first with '') that answer
-- an event going or maybe and, when invitees is 1, those invited to it by an invitation not
-- revoked, each once, in the order of their IDs.
-- name: ListGatheringAudience :many
SELECT CAST(x.account_id AS TEXT) AS account_id FROM (
  SELECT a.account_id AS account_id FROM gathering_attendees a
  WHERE a.gathering_id = sqlc.arg(gathering_id) AND a.answer IN ('going', 'maybe')
  UNION
  SELECT i.invitee_account_id AS account_id FROM gathering_invitations i
  WHERE i.gathering_id = sqlc.arg(gathering_id) AND CAST(sqlc.arg(invitees) AS INTEGER) = 1
    AND NOT EXISTS (SELECT 1 FROM gathering_invitation_revocations x WHERE x.invitation_id = i.id)
) x
WHERE x.account_id > CAST(sqlc.arg(after_id) AS TEXT)
ORDER BY x.account_id
LIMIT sqlc.arg(row_limit);

-- ListGatheringAudienceFacts returns, for each account of a JSON array, whether it has an
-- invitation to an event that was not revoked, its answer ('' for none), whether its answer is
-- approved, and whether it voted in the event's date poll.
-- name: ListGatheringAudienceFacts :many
SELECT CAST(v.value AS TEXT) AS account_id,
  EXISTS (SELECT 1 FROM gathering_invitations i WHERE i.gathering_id = sqlc.arg(gathering_id) AND i.invitee_account_id = v.value
    AND NOT EXISTS (SELECT 1 FROM gathering_invitation_revocations x WHERE x.invitation_id = i.id)) AS invited,
  CAST(COALESCE((SELECT a.answer FROM gathering_attendees a WHERE a.gathering_id = sqlc.arg(gathering_id) AND a.account_id = v.value), '') AS TEXT) AS answer,
  CAST(COALESCE((SELECT a.approved FROM gathering_attendees a WHERE a.gathering_id = sqlc.arg(gathering_id) AND a.account_id = v.value), 0) AS INTEGER) AS approved,
  EXISTS (SELECT 1 FROM gathering_votes o WHERE o.gathering_id = sqlc.arg(gathering_id) AND o.account_id = v.value) AS voted
FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)) v;
