-- SPDX-License-Identifier: AGPL-3.0-only

-- Sensitivity overrides of mark_statuses_sensitive (data-model section 4.4) -----------------------------

-- InsertStatusSensitivityOverride inserts nothing when the action already marks the status.
-- name: InsertStatusSensitivityOverride :exec
INSERT INTO status_sensitivity_overrides (status_id, source_action_id) VALUES (?, ?)
ON CONFLICT (status_id, source_action_id) DO NOTHING;

-- name: DeleteStatusSensitivityOverridesOfAction :exec
DELETE FROM status_sensitivity_overrides WHERE source_action_id = ?;

-- name: StatusSensitivityOverridden :one
SELECT EXISTS (SELECT 1 FROM status_sensitivity_overrides WHERE status_id = ?);

-- name: PurgeStatusSensitivityOverrides :exec
DELETE FROM status_sensitivity_overrides WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- ListExistingStatusIDs returns the statuses among ids that have a row, which the retention of the
-- hook decisions asks (ADR-0147 6).
-- name: ListExistingStatusIDs :many
SELECT id FROM statuses WHERE id IN (sqlc.slice(ids)) ORDER BY id;
