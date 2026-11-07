-- SPDX-License-Identifier: AGPL-3.0-only

-- Statuses of actions on statuses (data-model section 4.3) ----------------------------------------------

-- name: InsertModerationActionStatus :exec
INSERT INTO moderation_action_statuses (action_id, status_id) VALUES ($1, $2);

-- name: ListModerationActionStatusIDs :many
SELECT status_id FROM moderation_action_statuses WHERE action_id = $1 ORDER BY status_id;
