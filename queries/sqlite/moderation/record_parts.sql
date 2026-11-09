-- SPDX-License-Identifier: AGPL-3.0-only

-- Moderation records by many IDs at once, one query per part, for the pages of notifications and
-- of the administration. Each reads what the query of one record reads, in the same order within a
-- record. The lists are JSON arrays.

-- name: ListModerationActionRecordsByIDs :many
SELECT a.id, a.target_account_id, a.kind, a.reason_category, a.public_reason, a.private_note,
  a.report_id, a.issuer, a.issued_by_user_id, a.notify_target, a.expires_at, a.issued_at,
  l.action_id AS lift_action_id, l.lifted_by_user_id, l.reason AS lift_reason, l.lifted_at
FROM moderation_actions a
LEFT JOIN moderation_action_lifts l ON l.action_id = a.id
WHERE a.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY a.id;

-- name: ListModerationActionStatusIDsOf :many
SELECT action_id, status_id FROM moderation_action_statuses WHERE action_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY action_id, status_id;

-- name: ListModerationAppealsByIDs :many
SELECT a.id, a.action_id, a.account_id, a.body, a.submitted_at, d.appeal_id AS decision_appeal_id,
  d.decision, d.public_reason, d.decided_by_user_id, d.decided_at
FROM moderation_appeals a LEFT JOIN moderation_appeal_decisions d ON d.appeal_id = a.id
WHERE a.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY a.id;

-- name: ListRelationshipSeverancesOf :many
SELECT s.id, s.domain_action_id, s.action_id, s.account_id, s.followers_lost, s.following_lost, s.occurred_at,
  d.domain AS target_domain, a.target_account_id
FROM relationship_severances s
LEFT JOIN domain_moderation_actions d ON d.id = s.domain_action_id
LEFT JOIN moderation_actions a ON a.id = s.action_id
WHERE s.account_id = sqlc.arg(account_id) AND s.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY s.id;

-- name: ListReportsByIDs :many
SELECT id, uri, reporter_account_id, target_account_id, category, comment, forwarded, filed_at
FROM reports WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;

-- name: ListReportStatusIDsOf :many
SELECT report_id, status_id FROM report_statuses WHERE report_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY report_id, status_id;

-- name: ListReportRuleIDsOf :many
SELECT report_id, rule_id FROM report_rule_citations WHERE report_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY report_id, rule_id;

-- name: ListReportVoiceRoomsOf :many
SELECT report_id, room_id, revision_id FROM report_voice_rooms WHERE report_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY report_id;

-- name: ListReportHookOriginsOf :many
SELECT report_id, hook_decision_id, hook, reason_code FROM report_hook_origins WHERE report_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY report_id;

-- name: ListReportForwardsOf :many
SELECT id, report_id, comment, forwarded_by_user_id, forwarded_at FROM report_forwards WHERE report_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY report_id;

-- name: ListReportStatesOf :many
SELECT report_id, state, outcome, assignee_user_id, since FROM report_states WHERE report_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY report_id;
