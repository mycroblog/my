-- SPDX-License-Identifier: AGPL-3.0-only

-- The sources and the emptying of the projections of moderation, identity, content (the
-- sensitivity overrides and the poll tallies), and timeline (the restrictions) that
-- `my projection rebuild` derives again (data-model section 9). The Rebuild queries state the
-- rules of the commands and projectors in SQL.

-- Moderation ----------------------------------------------------------------------------------

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

-- Identity ------------------------------------------------------------------------------------

-- name: DeleteAllUserRoles :exec
DELETE FROM user_roles;

-- RebuildUserRoles states the rule of the role commands: the grants without a revocation.
-- name: RebuildUserRoles :exec
INSERT INTO user_roles (user_id, role_id, grant_id)
SELECT g.user_id, g.role_id, g.id FROM user_role_grants g
WHERE NOT EXISTS (SELECT 1 FROM user_role_revocations v WHERE v.grant_id = g.id);

-- name: DeleteAllUserRegistrationStates :exec
DELETE FROM user_registration_states;

-- RebuildUserRegistrationStates states the rule of the registration commands: pending since the
-- application, then the latest decision since it was made.
-- name: RebuildUserRegistrationStates :exec
INSERT INTO user_registration_states (user_id, state, since)
SELECT a.user_id,
  COALESCE((SELECT d.decision FROM registration_decisions d WHERE d.application_id = a.id ORDER BY d.id DESC LIMIT 1), 'pending'),
  COALESCE((SELECT d.decided_at FROM registration_decisions d WHERE d.application_id = a.id ORDER BY d.id DESC LIMIT 1), a.submitted_at)
FROM registration_applications a;

-- name: DeleteAllUserPrimaryEmails :exec
DELETE FROM user_primary_emails;

-- RebuildUserPrimaryEmails states the rule of the commands that create users: the first email
-- address of each user is its primary one.
-- name: RebuildUserPrimaryEmails :exec
INSERT INTO user_primary_emails (user_id, email_address_id, address_normalized)
SELECT e.user_id, e.id, e.address_normalized FROM user_email_addresses e
WHERE NOT EXISTS (SELECT 1 FROM user_email_addresses o WHERE o.user_id = e.user_id AND o.id < e.id);

-- name: DeleteAllEmailSuppressions :exec
DELETE FROM email_suppressions;

-- The rule of email_suppressions (data-model 4.1): of the failures of an address after its last
-- lift, the earliest hard bounce, or the earliest complaint when there is no hard bounce.
-- name: RebuildEmailSuppressions :exec
INSERT INTO email_suppressions (email_address_id, kind, failure_id)
SELECT f.email_address_id, f.kind, f.id FROM email_delivery_failures f
WHERE NOT EXISTS (
    SELECT 1 FROM email_suppression_lifts l1 WHERE l1.email_address_id = f.email_address_id AND l1.lifted_at >= f.occurred_at)
  AND (f.kind = 'hard_bounce' OR NOT EXISTS (
    SELECT 1 FROM email_delivery_failures h WHERE h.email_address_id = f.email_address_id AND h.kind = 'hard_bounce'
      AND NOT EXISTS (
        SELECT 1 FROM email_suppression_lifts l2 WHERE l2.email_address_id = h.email_address_id AND l2.lifted_at >= h.occurred_at)))
  AND NOT EXISTS (
    SELECT 1 FROM email_delivery_failures o WHERE o.email_address_id = f.email_address_id AND o.kind = f.kind
      AND (o.occurred_at < f.occurred_at OR (o.occurred_at = f.occurred_at AND o.id < f.id))
      AND NOT EXISTS (
        SELECT 1 FROM email_suppression_lifts l3 WHERE l3.email_address_id = o.email_address_id AND l3.lifted_at >= o.occurred_at));

-- Content -------------------------------------------------------------------------------------

-- name: DeleteAllStatusSensitivityOverrides :exec
DELETE FROM status_sensitivity_overrides;

-- name: DeleteAllPollTallies :exec
DELETE FROM poll_tallies;

-- RebuildPollTallies states the projector's rule: the counts of the newest observation of each
-- poll.
-- name: RebuildPollTallies :exec
INSERT INTO poll_tallies (poll_id, position, votes, voters_count)
SELECT o.poll_id, c.position, c.votes, o.voters_count
FROM poll_tally_observations o JOIN poll_tally_observation_counts c ON c.observation_id = o.id
WHERE NOT EXISTS (SELECT 1 FROM poll_tally_observations n WHERE n.poll_id = o.poll_id AND n.id > o.id);

-- Timeline ------------------------------------------------------------------------------------

-- name: ListAllFeedAuthorRestrictions :many
SELECT account_id, restriction, effective_until FROM feed_author_restrictions ORDER BY account_id, restriction;

-- name: ListAllFeedDomainRestrictions :many
SELECT domain, restriction, effective_until FROM feed_domain_restrictions ORDER BY domain, restriction;

-- name: DeleteAllFeedAuthorRestrictions :exec
DELETE FROM feed_author_restrictions;

-- name: DeleteAllFeedDomainRestrictions :exec
DELETE FROM feed_domain_restrictions;

-- Webhook -------------------------------------------------------------------------------------

-- name: DeleteAllWebhookEndpointHeads :exec
DELETE FROM webhook_endpoint_heads;

-- RebuildWebhookEndpointHeads states the rule of the endpoint commands: the newest revision,
-- state change, and secret of each endpoint, which the commands write together with it.
-- name: RebuildWebhookEndpointHeads :exec
INSERT INTO webhook_endpoint_heads (endpoint_id, revision_id, state, secret_id)
SELECT e.id,
  (SELECT r.id FROM webhook_endpoint_revisions r WHERE r.endpoint_id = e.id ORDER BY r.id DESC LIMIT 1),
  (SELECT c.state FROM webhook_endpoint_state_changes c WHERE c.endpoint_id = e.id ORDER BY c.id DESC LIMIT 1),
  (SELECT s.id FROM webhook_secrets s WHERE s.endpoint_id = e.id ORDER BY s.id DESC LIMIT 1)
FROM webhook_endpoints e;

-- DeleteDerivableWebhookEndpointFailures deletes the runs of failures that the kept attempts
-- derive: those whose first failure is kept, and those that a kept success after their first
-- failure ended. A run whose first failure the purge removed stays.
-- name: DeleteDerivableWebhookEndpointFailures :exec
DELETE FROM webhook_endpoint_failures
WHERE EXISTS (SELECT 1 FROM webhook_delivery_attempts a WHERE a.id = webhook_endpoint_failures.first_failure_attempt_id)
  OR EXISTS (
    SELECT 1 FROM webhook_delivery_attempts a JOIN webhook_deliveries d ON d.id = a.delivery_id
    WHERE d.endpoint_id = webhook_endpoint_failures.endpoint_id AND a.outcome = 'succeeded'
      AND a.id > webhook_endpoint_failures.first_failure_attempt_id
  );

-- RebuildWebhookEndpointFailures states the projector's rule for the endpoints without a run:
-- the failed attempts after the newest succeeded one, from the first of them.
-- name: RebuildWebhookEndpointFailures :exec
INSERT INTO webhook_endpoint_failures (endpoint_id, first_failure_attempt_id, first_failed_at, consecutive_failures)
SELECT g.endpoint_id, g.first_id, f.attempted_at, g.failures
FROM (
  SELECT d.endpoint_id, MIN(a.id) AS first_id, COUNT(*) AS failures
  FROM webhook_delivery_attempts a JOIN webhook_deliveries d ON d.id = a.delivery_id
  WHERE a.outcome <> 'succeeded'
    AND NOT EXISTS (
      SELECT 1 FROM webhook_delivery_attempts s JOIN webhook_deliveries sd ON sd.id = s.delivery_id
      WHERE sd.endpoint_id = d.endpoint_id AND s.outcome = 'succeeded' AND s.id > a.id
    )
  GROUP BY d.endpoint_id
) g JOIN webhook_delivery_attempts f ON f.id = g.first_id
WHERE NOT EXISTS (SELECT 1 FROM webhook_endpoint_failures x WHERE x.endpoint_id = g.endpoint_id);

-- Finance -------------------------------------------------------------------------------------

-- name: DeleteAllCostItemHeads :exec
DELETE FROM cost_item_heads;

-- RebuildCostItemHeads states the rule of the cost item commands: every item that is not
-- canceled, with its newest label revision and end change (no end without one).
-- name: RebuildCostItemHeads :exec
INSERT INTO cost_item_heads (cost_item_id, label_revision_id, ends_on)
SELECT i.id,
  (SELECT l.id FROM cost_item_label_revisions l WHERE l.cost_item_id = i.id ORDER BY l.id DESC LIMIT 1),
  (SELECT e.ends_on FROM cost_item_end_changes e WHERE e.cost_item_id = i.id ORDER BY e.id DESC LIMIT 1)
FROM cost_items i
WHERE NOT EXISTS (SELECT 1 FROM cost_item_cancellations c WHERE c.cost_item_id = i.id);

-- name: DeleteAllImportedCostHeads :exec
DELETE FROM imported_cost_heads;

-- RebuildImportedCostHeads states the rule of the billing imports: the newest revision of each
-- key (the largest id, a UUIDv7 that follows the recording order).
-- name: RebuildImportedCostHeads :exec
INSERT INTO imported_cost_heads (source, billing_month, service, charge_type, revision_id)
SELECT r.source, r.billing_month, r.service, r.charge_type, MAX(r.id)
FROM imported_cost_revisions r
GROUP BY r.source, r.billing_month, r.service, r.charge_type;
