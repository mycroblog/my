-- SPDX-License-Identifier: AGPL-3.0-only

-- The interaction policies of statuses (status_interaction_policies, ADR-0155 section 4): read for many
-- statuses at once, and written per status with one statement for its rules (the unnests of a
-- select list advance together).

-- name: ListStatusInteractionPolicies :many
SELECT status_id, kind, approval, audience, actor_uri FROM status_interaction_policies
WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[])
ORDER BY status_id, kind, approval, audience, actor_uri;

-- DeleteStatusInteractionPolicyKinds removes the rules of the kinds of a status.
-- name: DeleteStatusInteractionPolicyKinds :exec
DELETE FROM status_interaction_policies WHERE status_id = sqlc.arg(status_id)::uuid AND kind = ANY(sqlc.arg(kinds)::text[]);

-- name: InsertStatusInteractionPolicy :exec
INSERT INTO status_interaction_policies (status_id, kind, approval, audience, actor_uri)
SELECT sqlc.arg(status_id)::uuid, unnest(sqlc.arg(kinds)::text[]), unnest(sqlc.arg(approvals)::text[]),
  unnest(sqlc.arg(audiences)::text[]), unnest(sqlc.arg(actor_uris)::text[])
ON CONFLICT (status_id, kind, approval, audience, actor_uri) DO NOTHING;

-- name: PurgeStatusInteractionPolicies :exec
DELETE FROM status_interaction_policies WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);
