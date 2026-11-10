-- SPDX-License-Identifier: AGPL-3.0-only

-- The events of the local users (ADR-0161 section 13).

-- name: InsertGathering :exec
INSERT INTO gatherings (id, organizer_account_id, created_at) VALUES (?, ?, ?);

-- name: InsertGatheringRevision :exec
INSERT INTO gathering_revisions (id, gathering_id, title, description, visibility, time_zone, place_disclosure, capacity, revised_by_account_id, revised_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertGatheringRevisionPlace :exec
INSERT INTO gathering_revision_places (revision_id, position, kind, name, address, url, speakers) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: InsertGatheringScheduleConfirmation :exec
INSERT INTO gathering_schedule_confirmations (id, gathering_id, starts_at, ends_at, confirmed_by_account_id, confirmed_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertGatheringInvitation :execrows
INSERT INTO gathering_invitations (id, gathering_id, invitee_account_id, invited_by_account_id, invited_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (gathering_id, invitee_account_id) DO NOTHING;

-- name: InsertGatheringInvitationRevocation :execrows
INSERT INTO gathering_invitation_revocations (invitation_id, revoked_by_account_id, revoked_at) VALUES (?, ?, ?)
ON CONFLICT (invitation_id) DO NOTHING;

-- name: InsertGatheringRSVP :exec
INSERT INTO gathering_rsvps (id, gathering_id, account_id, answer, answered_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertGatheringRSVPApproval :exec
INSERT INTO gathering_rsvp_approvals (id, gathering_id, account_id, change, changed_by_account_id, changed_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertGatheringCoorganizerChange :exec
INSERT INTO gathering_coorganizer_changes (id, gathering_id, account_id, change, changed_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertGatheringCancellation :execrows
INSERT INTO gathering_cancellations (gathering_id, reason, cancelled_by_account_id, note, cancelled_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (gathering_id) DO NOTHING;

-- GetGathering returns an event with its head, its current revision and date, its cancellation
-- (NULL columns for none), and how many revisions it has.
-- name: GetGathering :one
SELECT g.id, g.organizer_account_id, g.created_at,
  h.revision_id, h.confirmation_id, h.state, h.starts_at, h.ends_at, h.going_count,
  v.title, v.description, v.visibility, v.time_zone, v.place_disclosure, v.capacity, v.revised_by_account_id, v.revised_at,
  c.confirmed_by_account_id, c.confirmed_at,
  x.reason AS cancel_reason, x.cancelled_by_account_id, x.note AS cancel_note, x.cancelled_at,
  (SELECT COUNT(*) FROM gathering_revisions n WHERE n.gathering_id = g.id) AS revision_count
FROM gatherings g
JOIN gathering_heads h ON h.gathering_id = g.id
JOIN gathering_revisions v ON v.id = h.revision_id
JOIN gathering_schedule_confirmations c ON c.id = h.confirmation_id
LEFT JOIN gathering_cancellations x ON x.gathering_id = g.id
WHERE g.id = ?;

-- ListUpcomingGatherings returns at most row_limit events that are scheduled and have not ended at
-- now, by the nearest start first and then by ID, after the position (after_starts_at, after_id)
-- unless after_id is ''; the columns are those of GetGathering.
-- name: ListUpcomingGatherings :many
SELECT g.id, g.organizer_account_id, g.created_at,
  h.revision_id, h.confirmation_id, h.state, h.starts_at, h.ends_at, h.going_count,
  v.title, v.description, v.visibility, v.time_zone, v.place_disclosure, v.capacity, v.revised_by_account_id, v.revised_at,
  c.confirmed_by_account_id, c.confirmed_at,
  x.reason AS cancel_reason, x.cancelled_by_account_id, x.note AS cancel_note, x.cancelled_at,
  (SELECT COUNT(*) FROM gathering_revisions n WHERE n.gathering_id = g.id) AS revision_count
FROM gathering_heads h
JOIN gatherings g ON g.id = h.gathering_id
JOIN gathering_revisions v ON v.id = h.revision_id
JOIN gathering_schedule_confirmations c ON c.id = h.confirmation_id
LEFT JOIN gathering_cancellations x ON x.gathering_id = g.id
WHERE h.state = 'scheduled' AND h.ends_at > sqlc.arg(now)
  AND (CAST(sqlc.arg(after_id) AS TEXT) = '' OR h.starts_at > sqlc.arg(after_starts_at)
    OR (h.starts_at = sqlc.arg(after_starts_at) AND h.gathering_id > CAST(sqlc.arg(after_id) AS TEXT)))
ORDER BY h.starts_at, h.gathering_id
LIMIT sqlc.arg(row_limit);

-- ListGatheringRevisionPlaces returns the places of revision_ids, a JSON array.
-- name: ListGatheringRevisionPlaces :many
SELECT revision_id, position, kind, name, address, url, speakers FROM gathering_revision_places
WHERE revision_id IN (SELECT value FROM json_each(CAST(sqlc.arg(revision_ids) AS TEXT)))
ORDER BY revision_id, position;

-- ListGatheringCoorganizers returns the co-organizers of gathering_ids, a JSON array, in the order
-- of their nominations.
-- name: ListGatheringCoorganizers :many
SELECT gathering_id, account_id, change_id FROM gathering_coorganizers
WHERE gathering_id IN (SELECT value FROM json_each(CAST(sqlc.arg(gathering_ids) AS TEXT)))
ORDER BY gathering_id, change_id;

-- name: GetGatheringAttendee :one
SELECT gathering_id, account_id, rsvp_id, answer, approved FROM gathering_attendees WHERE gathering_id = ? AND account_id = ?;

-- ListGatheringAttendees returns the attendees of an event who answer going or maybe, in the order
-- of their answers, after the answer of an ID (or from the first with '').
-- name: ListGatheringAttendees :many
SELECT gathering_id, account_id, rsvp_id, answer, approved FROM gathering_attendees
WHERE gathering_id = sqlc.arg(gathering_id) AND answer IN ('going', 'maybe') AND rsvp_id > CAST(sqlc.arg(after_id) AS TEXT)
ORDER BY rsvp_id
LIMIT sqlc.arg(row_limit);

-- GetGatheringInvitation returns the invitation of an account to an event and whether it was
-- revoked.
-- name: GetGatheringInvitation :one
SELECT i.id, i.gathering_id, i.invitee_account_id, i.invited_by_account_id, i.invited_at,
  CAST(EXISTS (SELECT 1 FROM gathering_invitation_revocations x WHERE x.invitation_id = i.id) AS INTEGER) AS revoked
FROM gathering_invitations i
WHERE i.gathering_id = ? AND i.invitee_account_id = ?;

-- ListGatheringInvitations returns the invitations of an event that were not revoked, after the
-- invitation of an ID (or from the first with ''), in the order they were made.
-- name: ListGatheringInvitations :many
SELECT i.id, i.gathering_id, i.invitee_account_id, i.invited_by_account_id, i.invited_at
FROM gathering_invitations i
WHERE i.gathering_id = sqlc.arg(gathering_id) AND i.id > CAST(sqlc.arg(after_id) AS TEXT)
  AND NOT EXISTS (SELECT 1 FROM gathering_invitation_revocations x WHERE x.invitation_id = i.id)
ORDER BY i.id
LIMIT sqlc.arg(row_limit);

-- ListGatheringViewerFacts returns, for each event of a JSON array, whether the account has an
-- invitation that was not revoked, whether it answered, and whether its answer is approved.
-- name: ListGatheringViewerFacts :many
SELECT CAST(v.value AS TEXT) AS gathering_id,
  EXISTS (SELECT 1 FROM gathering_invitations i WHERE i.gathering_id = v.value AND i.invitee_account_id = sqlc.arg(account_id)
    AND NOT EXISTS (SELECT 1 FROM gathering_invitation_revocations x WHERE x.invitation_id = i.id)) AS invited,
  CAST(COALESCE((SELECT a.answer FROM gathering_attendees a WHERE a.gathering_id = v.value AND a.account_id = sqlc.arg(account_id)), '') AS TEXT) AS answer,
  CAST(COALESCE((SELECT a.approved FROM gathering_attendees a WHERE a.gathering_id = v.value AND a.account_id = sqlc.arg(account_id)), 0) AS INTEGER) AS approved
FROM json_each(CAST(sqlc.arg(gathering_ids) AS TEXT)) v;

-- ListGatheringRevisionsToErase returns at most row_limit revisions that an account wrote and whose
-- title, description, or places the erasure has not removed yet.
-- name: ListGatheringRevisionsToErase :many
SELECT v.id FROM gathering_revisions v
WHERE v.revised_by_account_id = sqlc.arg(account_id)
  AND (v.title <> '' OR v.description <> '' OR EXISTS (SELECT 1 FROM gathering_revision_places p WHERE p.revision_id = v.id))
ORDER BY v.id
LIMIT sqlc.arg(row_limit);

-- name: EraseGatheringRevision :exec
UPDATE gathering_revisions SET title = '', description = '' WHERE id = ?;

-- name: DeleteGatheringRevisionPlaces :exec
DELETE FROM gathering_revision_places WHERE revision_id = ?;

-- Projections ---------------------------------------------------------------------------------
-- The rules of gathering_heads, gathering_attendees, and gathering_coorganizers. The refreshes of
-- one event or one account and the rebuild state the same rules. gathering_heads counts the
-- attendees, so the attendees are derived first.

-- name: DeleteGatheringHead :exec
DELETE FROM gathering_heads WHERE gathering_id = ?;

-- InsertGatheringHead derives the head of an event: its newest revision and confirmation,
-- cancelled when it has a cancellation, and its attendees who answer going.
-- name: InsertGatheringHead :exec
INSERT INTO gathering_heads (gathering_id, organizer_account_id, revision_id, confirmation_id, state, starts_at, ends_at, going_count)
SELECT g.id, g.organizer_account_id,
  (SELECT v.id FROM gathering_revisions v WHERE v.gathering_id = g.id ORDER BY v.id DESC LIMIT 1),
  c.id,
  CASE WHEN EXISTS (SELECT 1 FROM gathering_cancellations x WHERE x.gathering_id = g.id) THEN 'cancelled' ELSE 'scheduled' END,
  c.starts_at, c.ends_at,
  (SELECT COUNT(*) FROM gathering_attendees a WHERE a.gathering_id = g.id AND a.answer = 'going')
FROM gatherings g
JOIN gathering_schedule_confirmations c ON c.id = (
  SELECT c2.id FROM gathering_schedule_confirmations c2 WHERE c2.gathering_id = g.id ORDER BY c2.id DESC LIMIT 1
)
WHERE g.id = ?;

-- name: DeleteAllGatheringHeads :exec
DELETE FROM gathering_heads;

-- name: RebuildGatheringHeads :exec
INSERT INTO gathering_heads (gathering_id, organizer_account_id, revision_id, confirmation_id, state, starts_at, ends_at, going_count)
SELECT g.id, g.organizer_account_id,
  (SELECT v.id FROM gathering_revisions v WHERE v.gathering_id = g.id ORDER BY v.id DESC LIMIT 1),
  c.id,
  CASE WHEN EXISTS (SELECT 1 FROM gathering_cancellations x WHERE x.gathering_id = g.id) THEN 'cancelled' ELSE 'scheduled' END,
  c.starts_at, c.ends_at,
  (SELECT COUNT(*) FROM gathering_attendees a WHERE a.gathering_id = g.id AND a.answer = 'going')
FROM gatherings g
JOIN gathering_schedule_confirmations c ON c.id = (
  SELECT c2.id FROM gathering_schedule_confirmations c2 WHERE c2.gathering_id = g.id ORDER BY c2.id DESC LIMIT 1
);

-- name: DeleteGatheringAttendee :exec
DELETE FROM gathering_attendees WHERE gathering_id = ? AND account_id = ?;

-- InsertGatheringAttendee derives the attendee row of an account of an event from its newest
-- answer: approved when the answer is not declined and the newest approval change of the account
-- is an approval that no answer of declined followed.
-- name: InsertGatheringAttendee :exec
INSERT INTO gathering_attendees (gathering_id, account_id, rsvp_id, answer, approved)
SELECT r.gathering_id, r.account_id, r.id, r.answer,
  CASE WHEN r.answer <> 'declined' AND EXISTS (
    SELECT 1 FROM gathering_rsvp_approvals a
    WHERE a.id = (
      SELECT a2.id FROM gathering_rsvp_approvals a2 WHERE a2.gathering_id = r.gathering_id AND a2.account_id = r.account_id ORDER BY a2.id DESC LIMIT 1
    ) AND a.change = 'approved'
      AND NOT EXISTS (
        SELECT 1 FROM gathering_rsvps d WHERE d.gathering_id = r.gathering_id AND d.account_id = r.account_id AND d.answer = 'declined' AND d.id > a.id
      )
  ) THEN 1 ELSE 0 END
FROM gathering_rsvps r
WHERE r.gathering_id = sqlc.arg(gathering_id) AND r.account_id = sqlc.arg(account_id)
  AND r.id = (SELECT r2.id FROM gathering_rsvps r2 WHERE r2.gathering_id = r.gathering_id AND r2.account_id = r.account_id ORDER BY r2.id DESC LIMIT 1);

-- name: DeleteAllGatheringAttendees :exec
DELETE FROM gathering_attendees;

-- name: RebuildGatheringAttendees :exec
INSERT INTO gathering_attendees (gathering_id, account_id, rsvp_id, answer, approved)
SELECT r.gathering_id, r.account_id, r.id, r.answer,
  CASE WHEN r.answer <> 'declined' AND EXISTS (
    SELECT 1 FROM gathering_rsvp_approvals a
    WHERE a.id = (
      SELECT a2.id FROM gathering_rsvp_approvals a2 WHERE a2.gathering_id = r.gathering_id AND a2.account_id = r.account_id ORDER BY a2.id DESC LIMIT 1
    ) AND a.change = 'approved'
      AND NOT EXISTS (
        SELECT 1 FROM gathering_rsvps d WHERE d.gathering_id = r.gathering_id AND d.account_id = r.account_id AND d.answer = 'declined' AND d.id > a.id
      )
  ) THEN 1 ELSE 0 END
FROM gathering_rsvps r
WHERE r.id = (SELECT r2.id FROM gathering_rsvps r2 WHERE r2.gathering_id = r.gathering_id AND r2.account_id = r.account_id ORDER BY r2.id DESC LIMIT 1);

-- name: DeleteGatheringCoorganizers :exec
DELETE FROM gathering_coorganizers WHERE gathering_id = ?;

-- InsertGatheringCoorganizers derives the co-organizers of an event: the accounts whose newest
-- co-organizer change is an assignment, with that change.
-- name: InsertGatheringCoorganizers :exec
INSERT INTO gathering_coorganizers (gathering_id, account_id, change_id)
SELECT c.gathering_id, c.account_id, c.id FROM gathering_coorganizer_changes c
WHERE c.gathering_id = ? AND c.change = 'assigned'
  AND c.id = (
    SELECT c2.id FROM gathering_coorganizer_changes c2 WHERE c2.gathering_id = c.gathering_id AND c2.account_id = c.account_id ORDER BY c2.id DESC LIMIT 1
  );

-- name: DeleteAllGatheringCoorganizers :exec
DELETE FROM gathering_coorganizers;

-- name: RebuildGatheringCoorganizers :exec
INSERT INTO gathering_coorganizers (gathering_id, account_id, change_id)
SELECT c.gathering_id, c.account_id, c.id FROM gathering_coorganizer_changes c
WHERE c.change = 'assigned'
  AND c.id = (
    SELECT c2.id FROM gathering_coorganizer_changes c2 WHERE c2.gathering_id = c.gathering_id AND c2.account_id = c.account_id ORDER BY c2.id DESC LIMIT 1
  );
