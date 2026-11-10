-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The moderation of the events and the retention of their records (ADR-0161, section 14): the revisions
-- that the moderators read from a report, the upcoming events of an organizer who was suspended
-- or deleted, and the purge of the answers and the invitations.

-- ListGatheringRevisions returns the revisions of an event, oldest first.
-- name: ListGatheringRevisions :many
SELECT id, gathering_id, title, description, visibility, time_zone, place_disclosure, capacity, revised_by_account_id, revised_at
FROM gathering_revisions WHERE gathering_id = sqlc.arg(gathering_id)
ORDER BY id;

-- ListOrganizedUpcomingGatherings returns at most row_limit events, in ID order, that an account
-- organizes, that were not cancelled, and that have not started at now: the scheduled events whose
-- start is ahead, and every unscheduled event, whose date poll has no date yet.
-- name: ListOrganizedUpcomingGatherings :many
SELECT g.id FROM gatherings g
JOIN gathering_heads h ON h.gathering_id = g.id
WHERE g.organizer_account_id = sqlc.arg(account_id)
  AND (h.state = 'unscheduled' OR (h.state = 'scheduled' AND h.starts_at > sqlc.arg(now)))
ORDER BY g.id
LIMIT sqlc.arg(row_limit);

-- ListGatheringsToPurge returns at most row_limit events, in ID order after the event after (''
-- for the first), that ended or were cancelled before ended_before, whichever came first, and
-- still have answers, votes, or invitations. The end of an unscheduled event is the end of the span
-- of its candidates.
-- name: ListGatheringsToPurge :many
SELECT h.gathering_id FROM gathering_heads h
LEFT JOIN gathering_cancellations x ON x.gathering_id = h.gathering_id
WHERE (sqlc.arg(after)::text = '' OR h.gathering_id::text > sqlc.arg(after)::text)
  AND (h.ends_at < sqlc.arg(ended_before) OR x.cancelled_at < sqlc.arg(ended_before))
  AND (EXISTS (SELECT 1 FROM gathering_rsvps r WHERE r.gathering_id = h.gathering_id)
    OR EXISTS (SELECT 1 FROM gathering_rsvp_approvals a WHERE a.gathering_id = h.gathering_id)
    OR EXISTS (SELECT 1 FROM gathering_votes v WHERE v.gathering_id = h.gathering_id)
    OR EXISTS (SELECT 1 FROM gathering_invitations i WHERE i.gathering_id = h.gathering_id))
ORDER BY h.gathering_id
LIMIT sqlc.arg(row_limit);

-- ListGatheringRecordAccounts returns the accounts that the answers, the approvals, the votes, and
-- the invitations of an event name.
-- name: ListGatheringRecordAccounts :many
SELECT r.account_id FROM gathering_rsvps r WHERE r.gathering_id = sqlc.arg(gathering_id)
UNION SELECT a.account_id FROM gathering_rsvp_approvals a WHERE a.gathering_id = sqlc.arg(gathering_id)
UNION SELECT a.changed_by_account_id FROM gathering_rsvp_approvals a WHERE a.gathering_id = sqlc.arg(gathering_id)
UNION SELECT o.account_id FROM gathering_votes o WHERE o.gathering_id = sqlc.arg(gathering_id)
UNION SELECT i.invitee_account_id FROM gathering_invitations i WHERE i.gathering_id = sqlc.arg(gathering_id)
UNION SELECT i.invited_by_account_id FROM gathering_invitations i WHERE i.gathering_id = sqlc.arg(gathering_id)
UNION SELECT v.revoked_by_account_id FROM gathering_invitation_revocations v
  JOIN gathering_invitations i ON i.id = v.invitation_id WHERE i.gathering_id = sqlc.arg(gathering_id);

-- The purge of the records of gathering_ids.

-- name: PurgeGatheringRSVPApprovals :exec
DELETE FROM gathering_rsvp_approvals WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[]);

-- name: PurgeGatheringRSVPs :exec
DELETE FROM gathering_rsvps WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[]);

-- name: PurgeGatheringVotes :exec
DELETE FROM gathering_votes WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[]);

-- name: PurgeGatheringAttendees :exec
DELETE FROM gathering_attendees WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[]);

-- name: PurgeGatheringInvitationRevocations :exec
DELETE FROM gathering_invitation_revocations
WHERE invitation_id IN (SELECT id FROM gathering_invitations WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[]));

-- name: PurgeGatheringInvitations :exec
DELETE FROM gathering_invitations WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[]);
