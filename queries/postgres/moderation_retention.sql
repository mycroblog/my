-- SPDX-License-Identifier: AGPL-3.0-only

-- Retention of the moderation records and the holds that stop it (events), data-model 4.3, 8,
-- ADR-0038. A hold is in effect while it has no release.

-- name: InsertRetentionHold :exec
INSERT INTO retention_holds (id, account_id, reason, created_by_user_id, created_at) VALUES ($1, $2, $3, $4, $5);

-- InsertRetentionHoldRelease inserts nothing when the hold is already released.
-- name: InsertRetentionHoldRelease :execrows
INSERT INTO retention_hold_releases (hold_id, released_by_user_id, released_at) VALUES ($1, $2, $3)
ON CONFLICT (hold_id) DO NOTHING;

-- name: GetRetentionHold :one
SELECT h.id, h.account_id, h.reason, h.created_by_user_id, h.created_at, r.hold_id AS release_hold_id,
  r.released_by_user_id, r.released_at
FROM retention_holds h LEFT JOIN retention_hold_releases r ON r.hold_id = h.id
WHERE h.id = $1;

-- ListRetentionHolds returns the holds of an account (of every account when account_id is
-- empty) with their releases, newest first.
-- name: ListRetentionHolds :many
SELECT h.id, h.account_id, h.reason, h.created_by_user_id, h.created_at, r.hold_id AS release_hold_id,
  r.released_by_user_id, r.released_at
FROM retention_holds h LEFT JOIN retention_hold_releases r ON r.hold_id = h.id
WHERE sqlc.narg(account_id)::uuid IS NULL OR h.account_id = sqlc.narg(account_id)::uuid
ORDER BY h.id DESC;

-- ListHeldAccountIDs returns the accounts that a hold in effect preserves.
-- name: ListHeldAccountIDs :many
SELECT DISTINCT h.account_id FROM retention_holds h
WHERE NOT EXISTS (SELECT 1 FROM retention_hold_releases r WHERE r.hold_id = h.id)
ORDER BY h.account_id;

-- StatusRetained reports whether a report or an action on statuses points at the status, or a
-- hold in effect preserves its author.
-- name: StatusRetained :one
SELECT (
  EXISTS (SELECT 1 FROM report_statuses rs WHERE rs.status_id = sqlc.arg(status_id))
  OR EXISTS (SELECT 1 FROM moderation_action_statuses ms WHERE ms.status_id = sqlc.arg(status_id))
  OR EXISTS (
    SELECT 1 FROM retention_holds h WHERE h.account_id = sqlc.arg(author_account_id)
      AND NOT EXISTS (SELECT 1 FROM retention_hold_releases r WHERE r.hold_id = h.id))
)::boolean AS retained;

-- ListModerationActionsToPurge returns, oldest first and below @id_before, the actions on
-- accounts that ended before @ended_before: an action that imposes nothing lasting (warn,
-- delete_statuses) when it was issued, another at the earlier of its lift and its expiry (never
-- for one with neither), and an appealed action not before the decision on the appeal. Actions
-- on an account that a hold preserves stay.
-- name: ListModerationActionsToPurge :many
SELECT a.id FROM moderation_actions a
LEFT JOIN moderation_action_lifts l ON l.action_id = a.id
LEFT JOIN moderation_appeals ap ON ap.action_id = a.id
LEFT JOIN moderation_appeal_decisions d ON d.appeal_id = ap.id
WHERE a.id < sqlc.arg(id_before)::uuid
  AND (CASE
    WHEN a.kind IN ('warn', 'delete_statuses') THEN a.issued_at
    WHEN l.lifted_at IS NOT NULL AND a.expires_at IS NOT NULL THEN LEAST(l.lifted_at, a.expires_at)
    ELSE COALESCE(l.lifted_at, a.expires_at)
  END) < sqlc.arg(ended_before)::timestamptz
  AND (ap.id IS NULL OR d.decided_at < sqlc.arg(ended_before)::timestamptz)
  AND NOT EXISTS (
    SELECT 1 FROM retention_holds h WHERE h.account_id = a.target_account_id
      AND NOT EXISTS (SELECT 1 FROM retention_hold_releases r WHERE r.hold_id = h.id))
ORDER BY a.id
LIMIT sqlc.arg(batch_limit)::integer;

-- The PurgeModerationAction* statements delete the actions (@ids) with their
-- children: the appeals with their decisions and states, the statuses, the lifts, and the
-- restriction rows.

-- name: PurgeModerationAppealStates :exec
DELETE FROM appeal_states WHERE appeal_id IN (
  SELECT ap.id FROM moderation_appeals ap WHERE ap.action_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeModerationAppealDecisions :exec
DELETE FROM moderation_appeal_decisions WHERE appeal_id IN (
  SELECT ap.id FROM moderation_appeals ap WHERE ap.action_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeModerationAppeals :exec
DELETE FROM moderation_appeals WHERE action_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeModerationActionStatuses :exec
DELETE FROM moderation_action_statuses WHERE action_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeModerationActionLifts :exec
DELETE FROM moderation_action_lifts WHERE action_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeModerationActionRestrictions :exec
DELETE FROM account_restrictions WHERE source_action_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeModerationActions :exec
DELETE FROM moderation_actions WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- ListReportsToPurge returns, oldest first and below @id_before, the resolved reports whose
-- state dates from before @resolved_before, that no action follows from, and whose target and
-- reporter no hold in effect preserves.
-- name: ListReportsToPurge :many
SELECT r.id FROM reports r JOIN report_states s ON s.report_id = r.id
WHERE r.id < sqlc.arg(id_before)::uuid
  AND s.state = 'resolved' AND s.since < sqlc.arg(resolved_before)::timestamptz
  AND NOT EXISTS (SELECT 1 FROM moderation_actions a WHERE a.report_id = r.id)
  AND NOT EXISTS (
    SELECT 1 FROM retention_holds h WHERE h.account_id IN (r.target_account_id, r.reporter_account_id)
      AND NOT EXISTS (SELECT 1 FROM retention_hold_releases rr WHERE rr.hold_id = h.id))
ORDER BY r.id
LIMIT sqlc.arg(batch_limit)::integer;

-- The PurgeReport* statements delete the reports (@ids) with their children.

-- name: PurgeReportStatuses :exec
DELETE FROM report_statuses WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReportRuleCitations :exec
DELETE FROM report_rule_citations WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReportAssignments :exec
DELETE FROM report_assignments WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReportResolutions :exec
DELETE FROM report_resolutions WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReportReopenings :exec
DELETE FROM report_reopenings WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReportNotes :exec
DELETE FROM report_notes WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReportVoiceRooms :exec
DELETE FROM report_voice_rooms WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReportStates :exec
DELETE FROM report_states WHERE report_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeReports :exec
DELETE FROM reports WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- ListSignupBlocksToPurge returns, oldest first and below @id_before, the sign-up blocks that
-- ended before @ended_before, at the earlier of their lift and their expiry (never for one with
-- neither). A block that a ban made stays while a hold in effect preserves the banned account.
-- name: ListSignupBlocksToPurge :many
SELECT b.id FROM signup_blocks b
LEFT JOIN signup_block_lifts l ON l.block_id = b.id
WHERE b.id < sqlc.arg(id_before)::uuid
  AND (CASE
    WHEN l.lifted_at IS NOT NULL AND b.expires_at IS NOT NULL THEN LEAST(l.lifted_at, b.expires_at)
    ELSE COALESCE(l.lifted_at, b.expires_at)
  END) < sqlc.arg(ended_before)::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM moderation_actions a JOIN retention_holds h ON h.account_id = a.target_account_id
    WHERE a.id = b.moderation_action_id
      AND NOT EXISTS (SELECT 1 FROM retention_hold_releases r WHERE r.hold_id = h.id))
ORDER BY b.id
LIMIT sqlc.arg(batch_limit)::integer;

-- The PurgeSignupBlock* statements delete the blocks (@ids) with their lifts.

-- name: PurgeSignupBlockLifts :exec
DELETE FROM signup_block_lifts WHERE block_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeSignupBlocks :exec
DELETE FROM signup_blocks WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- DeleteAuditLogEntriesBefore deletes at most @batch_limit entries below @id_before recorded
-- before @occurred_before, the oldest first, except those on an account that a hold in effect
-- preserves and on a report or an appeal of such an account.
-- name: DeleteAuditLogEntriesBefore :execrows
DELETE FROM audit_log_entries WHERE id IN (
  SELECT e.id FROM audit_log_entries e
  WHERE e.id < sqlc.arg(id_before)::uuid AND e.occurred_at < sqlc.arg(occurred_before)::timestamptz
    AND NOT EXISTS (
      SELECT 1 FROM retention_holds h
      WHERE NOT EXISTS (SELECT 1 FROM retention_hold_releases rr WHERE rr.hold_id = h.id)
        AND (
          (e.target_kind = 'account' AND e.target_id = h.account_id::text)
          OR (e.target_kind = 'report' AND EXISTS (
            SELECT 1 FROM reports r WHERE r.id::text = e.target_id
              AND h.account_id IN (r.target_account_id, r.reporter_account_id)))
          OR (e.target_kind = 'appeal' AND EXISTS (
            SELECT 1 FROM moderation_appeals ap WHERE ap.id::text = e.target_id AND ap.account_id = h.account_id))))
  ORDER BY e.id
  LIMIT sqlc.arg(batch_limit)::integer
);

-- name: ListReportAssignments :many
SELECT id, report_id, assignee_user_id, assigned_by_user_id, assigned_at FROM report_assignments
WHERE report_id = $1 ORDER BY id;

-- name: ListReportResolutions :many
SELECT id, report_id, outcome, resolved_by_user_id, resolved_at FROM report_resolutions
WHERE report_id = $1 ORDER BY id;

-- name: ListReportReopenings :many
SELECT id, report_id, reopened_by_user_id, reopened_at FROM report_reopenings
WHERE report_id = $1 ORDER BY id;
