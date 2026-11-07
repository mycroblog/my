-- SPDX-License-Identifier: AGPL-3.0-only

-- The sources and the emptying of the projections of moderation, identity, content (the
-- sensitivity overrides and the poll tallies), and timeline (the restrictions) that
-- `my projection rebuild` derives again (data-model section 9). The Rebuild queries state the
-- rules of the commands and projectors in SQL.

-- ListUnliftedModerationActions returns the actions on accounts without a lift, expired ones
-- included, by ID.
-- name: ListUnliftedModerationActions :many
SELECT a.id, a.target_account_id, a.kind, a.expires_at FROM moderation_actions a
WHERE NOT EXISTS (SELECT 1 FROM moderation_action_lifts l WHERE l.action_id = a.id)
ORDER BY a.id;

-- ListUnliftedDomainModerationActions returns the actions on domains without a lift, expired
-- ones included, by ID.
-- name: ListUnliftedDomainModerationActions :many
SELECT a.id, a.domain, a.kind, a.expires_at FROM domain_moderation_actions a
WHERE NOT EXISTS (SELECT 1 FROM domain_moderation_action_lifts l WHERE l.action_id = a.id)
ORDER BY a.id;

-- ListUnliftedModerationActionStatuses returns the statuses of the actions of a kind without a
-- lift, by action and status.
-- name: ListUnliftedModerationActionStatuses :many
SELECT s.action_id, s.status_id FROM moderation_action_statuses s
JOIN moderation_actions a ON a.id = s.action_id
WHERE a.kind = sqlc.arg(kind)
  AND NOT EXISTS (SELECT 1 FROM moderation_action_lifts l WHERE l.action_id = a.id)
ORDER BY s.action_id, s.status_id;

-- name: ListAllAccountRestrictionRows :many
SELECT account_id, restriction, source_action_id, effective_until FROM account_restrictions
ORDER BY account_id, restriction, source_action_id;

-- name: ListAllDomainRestrictionRows :many
SELECT domain, restriction, source_action_id, effective_until FROM domain_restrictions
ORDER BY domain, restriction, source_action_id;

-- name: DeleteAllAccountRestrictions :exec
DELETE FROM account_restrictions;

-- name: DeleteAllDomainRestrictions :exec
DELETE FROM domain_restrictions;

-- name: DeleteAllReportStates :exec
DELETE FROM report_states;

-- RebuildReportStates states the rule of the commands on reports: a report is open when filed;
-- a resolution (only of an open report) resolves it with its outcome, and a reopening (only of
-- a resolved one) opens it again, so it is resolved when it has more resolutions than
-- reopenings; since is the time of the latest of these; the assignee is that of the latest
-- assignment.
-- name: RebuildReportStates :exec
INSERT INTO report_states (report_id, state, outcome, assignee_user_id, since)
SELECT x.id,
  CASE WHEN x.resolutions > x.reopenings THEN 'resolved' ELSE 'open' END,
  CASE WHEN x.resolutions > x.reopenings THEN
    (SELECT s.outcome FROM report_resolutions s WHERE s.report_id = x.id ORDER BY s.id DESC LIMIT 1)
  END,
  (SELECT g.assignee_user_id FROM report_assignments g WHERE g.report_id = x.id ORDER BY g.id DESC LIMIT 1),
  CASE WHEN x.resolutions > x.reopenings THEN
    (SELECT s.resolved_at FROM report_resolutions s WHERE s.report_id = x.id ORDER BY s.id DESC LIMIT 1)
  ELSE
    COALESCE((SELECT o.reopened_at FROM report_reopenings o WHERE o.report_id = x.id ORDER BY o.id DESC LIMIT 1), x.filed_at)
  END
FROM (
  SELECT r.id, r.filed_at,
    (SELECT COUNT(*) FROM report_resolutions s WHERE s.report_id = r.id) AS resolutions,
    (SELECT COUNT(*) FROM report_reopenings o WHERE o.report_id = r.id) AS reopenings
  FROM reports r
) x;

-- name: DeleteAllAppealStates :exec
DELETE FROM appeal_states;

-- RebuildAppealStates states the rule of the commands on appeals: pending since it was filed,
-- then the decision since it was made.
-- name: RebuildAppealStates :exec
INSERT INTO appeal_states (appeal_id, state, since)
SELECT a.id, COALESCE(d.decision, 'pending'), COALESCE(d.decided_at, a.submitted_at)
FROM moderation_appeals a LEFT JOIN moderation_appeal_decisions d ON d.appeal_id = a.id;
