-- SPDX-License-Identifier: AGPL-3.0-only

-- Appeals of actions on local accounts (resource), their decisions (event), and their states
-- (projection), data-model 4.3.

-- InsertModerationAppeal inserts nothing when the action already has an appeal.
-- name: InsertModerationAppeal :execrows
INSERT INTO moderation_appeals (id, action_id, account_id, body, submitted_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- InsertModerationAppealDecision inserts nothing when the appeal is already decided.
-- name: InsertModerationAppealDecision :execrows
INSERT INTO moderation_appeal_decisions (appeal_id, decision, public_reason, decided_by_user_id, decided_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (appeal_id) DO NOTHING;

-- name: UpsertAppealState :exec
INSERT INTO appeal_states (appeal_id, state, since) VALUES (?, ?, ?)
ON CONFLICT (appeal_id) DO UPDATE SET state = excluded.state, since = excluded.since;

-- name: GetModerationAppeal :one
SELECT a.id, a.action_id, a.account_id, a.body, a.submitted_at, d.appeal_id AS decision_appeal_id,
  d.decision, d.public_reason, d.decided_by_user_id, d.decided_at
FROM moderation_appeals a LEFT JOIN moderation_appeal_decisions d ON d.appeal_id = a.id
WHERE a.id = ?;

-- ListModerationAppealsOfActions returns the appeals of the actions (a JSON array of their IDs).
-- name: ListModerationAppealsOfActions :many
SELECT a.id, a.action_id, a.account_id, a.body, a.submitted_at, d.appeal_id AS decision_appeal_id,
  d.decision, d.public_reason, d.decided_by_user_id, d.decided_at
FROM moderation_appeals a LEFT JOIN moderation_appeal_decisions d ON d.appeal_id = a.id
WHERE a.action_id IN (SELECT value FROM json_each(CAST(sqlc.arg(action_ids) AS TEXT)));

-- ListModerationAppeals returns the appeals with their decisions, newest first: those in a state
-- (empty for any) before the appeal before (empty for the first page).
-- name: ListModerationAppeals :many
SELECT a.id, a.action_id, a.account_id, a.body, a.submitted_at, d.appeal_id AS decision_appeal_id,
  d.decision, d.public_reason, d.decided_by_user_id, d.decided_at
FROM moderation_appeals a
JOIN appeal_states s ON s.appeal_id = a.id
LEFT JOIN moderation_appeal_decisions d ON d.appeal_id = a.id
WHERE (CAST(sqlc.arg(state) AS TEXT) = '' OR s.state = sqlc.arg(state))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR a.id < sqlc.arg(before))
ORDER BY a.id DESC
LIMIT sqlc.arg(lim);

-- name: CountAppealsInState :one
SELECT COUNT(*) FROM appeal_states WHERE state = ?;
