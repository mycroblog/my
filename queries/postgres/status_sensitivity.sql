-- SPDX-License-Identifier: AGPL-3.0-only

-- Sensitivity overrides of mark_statuses_sensitive (data-model section 4.4) -----------------------------

-- InsertStatusSensitivityOverride inserts nothing when the action already marks the status.
-- name: InsertStatusSensitivityOverride :exec
INSERT INTO status_sensitivity_overrides (status_id, source_action_id) VALUES ($1, $2)
ON CONFLICT (status_id, source_action_id) DO NOTHING;

-- name: DeleteStatusSensitivityOverridesOfAction :exec
DELETE FROM status_sensitivity_overrides WHERE source_action_id = $1;

-- name: StatusSensitivityOverridden :one
SELECT EXISTS (SELECT 1 FROM status_sensitivity_overrides WHERE status_id = $1);

-- name: PurgeStatusSensitivityOverrides :exec
DELETE FROM status_sensitivity_overrides WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);
