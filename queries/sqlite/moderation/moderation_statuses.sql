-- SPDX-License-Identifier: AGPL-3.0-only

-- Statuses of actions on statuses (data-model section 4.3) ----------------------------------------------

-- name: InsertModerationActionStatus :exec
INSERT INTO moderation_action_statuses (action_id, status_id) VALUES (?, ?);

-- name: ListModerationActionStatusIDs :many
SELECT status_id FROM moderation_action_statuses WHERE action_id = ? ORDER BY status_id;
