-- SPDX-License-Identifier: AGPL-3.0-only

-- Statuses of actions on statuses (data-model section 4.3) ----------------------------------------------

-- name: InsertModerationActionStatuses :exec
INSERT INTO moderation_action_statuses (action_id, status_id)
SELECT CAST(sqlc.arg(action_id) AS TEXT), value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT));

-- name: ListModerationActionStatusIDs :many
SELECT status_id FROM moderation_action_statuses WHERE action_id = ? ORDER BY status_id;
