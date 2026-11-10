-- SPDX-License-Identifier: AGPL-3.0-only

-- The interaction policies of statuses (status_interaction_policies, ADR-0155 section 4): read for many
-- statuses at once, and written per status with one statement for its rules. The lists are JSON
-- arrays.

-- name: ListStatusInteractionPolicies :many
SELECT status_id, kind, approval, audience, actor_uri FROM status_interaction_policies
WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)))
ORDER BY status_id, kind, approval, audience, actor_uri;

-- DeleteStatusInteractionPolicyKinds removes the rules of the kinds of a status.
-- name: DeleteStatusInteractionPolicyKinds :exec
DELETE FROM status_interaction_policies
WHERE status_id = sqlc.arg(status_id) AND kind IN (SELECT value FROM json_each(CAST(sqlc.arg(kinds) AS TEXT)));

-- InsertStatusInteractionPolicy inserts the rules of a status; WHERE true keeps ON CONFLICT
-- from reading as a join constraint.
-- name: InsertStatusInteractionPolicy :exec
INSERT INTO status_interaction_policies (status_id, kind, approval, audience, actor_uri)
SELECT CAST(sqlc.arg(status_id) AS TEXT), value ->> '$.kind', value ->> '$.approval', value ->> '$.audience', value ->> '$.actor_uri'
FROM json_each(CAST(sqlc.arg(rules) AS TEXT)) WHERE true
ON CONFLICT (status_id, kind, approval, audience, actor_uri) DO NOTHING;

-- name: PurgeStatusInteractionPolicies :exec
DELETE FROM status_interaction_policies WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- The replies held out of the threads of their parents (held_replies, ADR-0155 section 6).

-- name: InsertHeldReply :exec
INSERT INTO held_replies (status_id, reason, held_at) VALUES (?, ?, ?)
ON CONFLICT (status_id) DO NOTHING;

-- name: HeldReplyExists :one
SELECT EXISTS (SELECT 1 FROM held_replies WHERE status_id = ?);

-- name: PurgeHeldReplies :exec
DELETE FROM held_replies WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));
