-- SPDX-License-Identifier: AGPL-3.0-only

-- The due times of the events (gathering_dues, a projection), the reminders that the accounts
-- chose (gathering_reminder_settings and gathering_reminder_offsets, current values), and the
-- recipients of the notifications of the events (ADR-0161 sections 11 and 12).

-- name: GetGatheringDue :one
SELECT next_due_at FROM gathering_dues WHERE gathering_id = $1;

-- name: UpsertGatheringDue :exec
INSERT INTO gathering_dues (gathering_id, next_due_at) VALUES ($1, $2)
ON CONFLICT (gathering_id) DO UPDATE SET next_due_at = excluded.next_due_at;

-- name: DeleteGatheringDue :exec
DELETE FROM gathering_dues WHERE gathering_id = $1;

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
WHERE a.gathering_id = $1 AND a.answer IN ('going', 'maybe')
ORDER BY o.minutes_before DESC;

-- GatheringHasDefaultReminders reports whether an attendee of an event who answers going or maybe
-- has not chosen its reminders, so that the defaults apply to it.
-- name: GatheringHasDefaultReminders :one
SELECT EXISTS (SELECT 1 FROM gathering_attendees a
  WHERE a.gathering_id = $1 AND a.answer IN ('going', 'maybe')
    AND NOT EXISTS (SELECT 1 FROM gathering_reminder_settings s WHERE s.account_id = a.account_id)) AS has_defaults;

-- ListGatheringReminders returns the reminders of the accounts of account_ids that chose them, a
-- row per reminder, and a row with 0 for an account that chose none.
-- name: ListGatheringReminders :many
SELECT s.account_id, COALESCE(o.minutes_before, 0)::integer AS minutes_before
FROM gathering_reminder_settings s LEFT JOIN gathering_reminder_offsets o ON o.account_id = s.account_id
WHERE s.account_id = ANY(sqlc.arg(account_ids)::uuid[])
ORDER BY s.account_id, minutes_before DESC;

-- name: UpsertGatheringReminderSetting :exec
INSERT INTO gathering_reminder_settings (account_id, updated_at) VALUES ($1, $2)
ON CONFLICT (account_id) DO UPDATE SET updated_at = excluded.updated_at;

-- name: DeleteGatheringReminderSetting :exec
DELETE FROM gathering_reminder_settings WHERE account_id = $1;

-- name: DeleteGatheringReminderOffsets :exec
DELETE FROM gathering_reminder_offsets WHERE account_id = $1;

-- name: InsertGatheringReminderOffset :exec
INSERT INTO gathering_reminder_offsets (account_id, minutes_before) VALUES ($1, $2);

-- ListAttendingGatherings returns the events that are scheduled and have not started at now, that
-- an account answers going or maybe, by the nearest start first.
-- name: ListAttendingGatherings :many
SELECT h.gathering_id FROM gathering_attendees a JOIN gathering_heads h ON h.gathering_id = a.gathering_id
WHERE a.account_id = sqlc.arg(account_id) AND a.answer IN ('going', 'maybe') AND h.state = 'scheduled' AND h.starts_at > sqlc.arg(now)
ORDER BY h.starts_at, h.gathering_id
LIMIT sqlc.arg(row_limit);

-- ListGatheringAudience returns the accounts after an ID (or from the first with '') that answer
-- an event going or maybe and, when invitees is true, those invited to it by an invitation not
-- revoked, each once, in the order of their IDs.
-- name: ListGatheringAudience :many
SELECT x.account_id::text AS account_id FROM (
  SELECT a.account_id AS account_id FROM gathering_attendees a
  WHERE a.gathering_id = sqlc.arg(gathering_id) AND a.answer IN ('going', 'maybe')
  UNION
  SELECT i.invitee_account_id AS account_id FROM gathering_invitations i
  WHERE i.gathering_id = sqlc.arg(gathering_id) AND sqlc.arg(invitees)::boolean
    AND NOT EXISTS (SELECT 1 FROM gathering_invitation_revocations x WHERE x.invitation_id = i.id)
) x
WHERE x.account_id::text > sqlc.arg(after_id)::text
ORDER BY x.account_id::text
LIMIT sqlc.arg(row_limit);

-- ListGatheringAudienceFacts returns, for each account of account_ids, whether it has an
-- invitation to an event that was not revoked, its answer ('' for none), and whether its answer
-- is approved.
-- name: ListGatheringAudienceFacts :many
SELECT v.account_id::uuid AS account_id,
  EXISTS (SELECT 1 FROM gathering_invitations i WHERE i.gathering_id = sqlc.arg(gathering_id)::uuid AND i.invitee_account_id = v.account_id
    AND NOT EXISTS (SELECT 1 FROM gathering_invitation_revocations x WHERE x.invitation_id = i.id)) AS invited,
  COALESCE((SELECT a.answer FROM gathering_attendees a WHERE a.gathering_id = sqlc.arg(gathering_id)::uuid AND a.account_id = v.account_id), '')::text AS answer,
  COALESCE((SELECT a.approved FROM gathering_attendees a WHERE a.gathering_id = sqlc.arg(gathering_id)::uuid AND a.account_id = v.account_id), false)::boolean AS approved
FROM (SELECT unnest(sqlc.arg(account_ids)::uuid[]) AS account_id) v;
